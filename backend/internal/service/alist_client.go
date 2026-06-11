package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"opensync/internal/i18n"
	"strings"
	"sync"
	"time"
)

const maxResponseBytes = 10 << 20 // 10MB
const maxAlistWaitBuckets = 1024

// AlistClient represents an AList HTTP client
type AlistClient struct {
	URL     string
	Token   string
	User    string
	AlistID int64
	waits   map[string]time.Time
	mu      sync.Mutex
	client  *http.Client
}

// alistResponse represents AList API response
type alistResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// NewAlistClient creates a new AList client
func NewAlistClient(alistURL string, token string, alistID int64) (*AlistClient, error) {
	c := &AlistClient{
		URL:     strings.TrimRight(alistURL, "/"),
		Token:   token,
		AlistID: alistID,
		waits:   make(map[string]time.Time),
		client: &http.Client{
			Timeout: 300 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
	if err := c.getUser(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *AlistClient) doRequestContext(ctx context.Context, method, apiPath string, data interface{}, params map[string]string) (json.RawMessage, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	var body io.Reader
	if data != nil {
		jsonData, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(jsonData)
	}

	reqURL := c.URL + apiPath
	if len(params) > 0 {
		q := url.Values{}
		for k, v := range params {
			q.Set(k, v)
		}
		reqURL += "?" + q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return nil, errors.New(i18n.G("address_incorrect"))
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", c.Token)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "no such host") {
			return nil, errors.New(i18n.G("alist_connect_fail"))
		}
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := readAllWithLimit(resp.Body, maxResponseBytes)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, errors.New(i18n.G("code_not_200"))
	}

	var res alistResponse
	if err := json.Unmarshal(respBody, &res); err != nil {
		return nil, err
	}

	if res.Code == 401 {
		return nil, errors.New(i18n.G("alist_un_auth"))
	}
	if res.Code != 200 {
		msg := i18n.G("alist_fail_code_reason")
		msg = strings.Replace(msg, "{}", fmt.Sprintf("%d", res.Code), 1)
		msg = strings.Replace(msg, "{}", res.Message, 1)
		return nil, errors.New(msg)
	}

	return res.Data, nil
}

func (c *AlistClient) PostContext(ctx context.Context, apiPath string, data interface{}, params map[string]string) (json.RawMessage, error) {
	return c.doRequestContext(ctx, "POST", apiPath, data, params)
}

func (c *AlistClient) GetContext(ctx context.Context, apiPath string, params map[string]string) (json.RawMessage, error) {
	return c.doRequestContext(ctx, "GET", apiPath, nil, params)
}

func (c *AlistClient) getUser() error {
	data, err := c.GetContext(context.Background(), "/api/me", nil)
	if err != nil {
		return err
	}
	var result struct {
		Username string `json:"username"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return err
	}
	c.User = result.Username
	return nil
}

func (c *AlistClient) CheckWaitContext(ctx context.Context, path string, scanInterval int) error {
	if scanInterval <= 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	parts := strings.SplitN(path, "/", 3)
	var pathFirst string
	if len(parts) > 1 {
		pathFirst = parts[1]
	}
	if pathFirst == "" {
		return nil
	}

	now := time.Now()
	interval := time.Duration(scanInterval) * time.Second
	waitUntil := now

	c.mu.Lock()
	if c.waits == nil {
		c.waits = make(map[string]time.Time)
	}
	c.pruneWaitsLocked(now.Add(-interval))
	if lastTime, ok := c.waits[pathFirst]; ok && now.Sub(lastTime) < interval {
		waitUntil = lastTime.Add(interval)
	}
	c.waits[pathFirst] = waitUntil
	c.enforceMaxWaitBucketsLocked()
	c.mu.Unlock()

	if waitUntil.After(now) {
		timer := time.NewTimer(time.Until(waitUntil))
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (c *AlistClient) pruneWaitsLocked(cutoff time.Time) {
	for pathFirst, lastTime := range c.waits {
		if !lastTime.After(cutoff) {
			delete(c.waits, pathFirst)
		}
	}
}

func (c *AlistClient) enforceMaxWaitBucketsLocked() {
	for len(c.waits) > maxAlistWaitBuckets {
		var oldestPath string
		var oldestTime time.Time
		first := true
		for pathFirst, lastTime := range c.waits {
			if first || lastTime.Before(oldestTime) {
				oldestPath = pathFirst
				oldestTime = lastTime
				first = false
			}
		}
		if oldestPath == "" {
			return
		}
		delete(c.waits, oldestPath)
	}
}

// FileListResponse represents a file list entry
type FileListEntry struct {
	Name     string                 `json:"name"`
	IsDir    bool                   `json:"is_dir"`
	Size     int64                  `json:"size"`
	HashInfo map[string]interface{} `json:"hash_info"`
	Hashinfo string                 `json:"hashinfo"`
}

// FileMetadata contains lightweight comparison data from AList list results.
type FileMetadata struct {
	Size int64
	MD5  string
}

func (e FileListEntry) metadata() FileMetadata {
	return FileMetadata{
		Size: e.Size,
		MD5:  normalizeMD5(firstMD5(e.HashInfo, e.Hashinfo)),
	}
}

func firstMD5(hashInfo map[string]interface{}, hashinfo string) string {
	if md5 := hashValue(hashInfo, "md5"); md5 != "" {
		return md5
	}
	if md5 := hashValue(hashInfo, "MD5"); md5 != "" {
		return md5
	}
	if hashinfo == "" {
		return ""
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(hashinfo), &parsed); err == nil {
		if md5 := hashValue(parsed, "md5"); md5 != "" {
			return md5
		}
		return hashValue(parsed, "MD5")
	}
	return ""
}

func hashValue(hashInfo map[string]interface{}, key string) string {
	if hashInfo == nil {
		return ""
	}
	value, ok := hashInfo[key]
	if !ok || value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

func normalizeMD5(md5 string) string {
	return strings.ToLower(strings.TrimSpace(md5))
}

// FileListResult maps filename -> metadata (for files) or empty map (for dirs)
// Directories have key ending with "/"
type FileListResult = map[string]interface{}

func (c *AlistClient) FileListApiContext(ctx context.Context, path string, useCache int, scanInterval int) (FileListResult, error) {
	if err := c.CheckWaitContext(ctx, path, scanInterval); err != nil {
		return nil, err
	}

	data, err := c.PostContext(ctx, "/api/fs/list", map[string]interface{}{
		"path":    path,
		"refresh": useCache != 1,
	}, nil)
	if err != nil {
		return nil, err
	}

	var content struct {
		Content []FileListEntry `json:"content"`
	}
	if err := json.Unmarshal(data, &content); err != nil {
		return nil, err
	}

	result := make(FileListResult)
	if content.Content != nil {
		for _, item := range content.Content {
			if item.IsDir {
				result[item.Name+"/"] = map[string]interface{}{}
			} else {
				result[item.Name] = item.metadata()
			}
		}
	}
	return result, nil
}

// FilePathList gets subdirectory list for path selector
func (c *AlistClient) FilePathList(ctx context.Context, path string) ([]map[string]string, error) {
	data, err := c.PostContext(ctx, "/api/fs/list", map[string]interface{}{
		"path":    path,
		"refresh": true,
	}, nil)
	if err != nil {
		return nil, err
	}

	var content struct {
		Content []FileListEntry `json:"content"`
	}
	if err := json.Unmarshal(data, &content); err != nil {
		return nil, err
	}

	var result []map[string]string
	if content.Content != nil {
		for _, item := range content.Content {
			if item.IsDir {
				result = append(result, map[string]string{"path": item.Name})
			}
		}
	}
	if result == nil {
		result = []map[string]string{}
	}
	return result, nil
}

func (c *AlistClient) MkdirContext(ctx context.Context, path string, scanInterval int) error {
	if err := c.CheckWaitContext(ctx, path, scanInterval); err != nil {
		return err
	}
	_, err := c.PostContext(ctx, "/api/fs/mkdir", map[string]interface{}{
		"path": path,
	}, nil)
	return err
}

func (c *AlistClient) DeleteFileContext(ctx context.Context, path string, names []string, scanInterval int) error {
	if err := c.CheckWaitContext(ctx, path, scanInterval); err != nil {
		return err
	}
	_, err := c.PostContext(ctx, "/api/fs/remove", map[string]interface{}{
		"names": names,
		"dir":   path,
	}, nil)
	return err
}

func (c *AlistClient) copyOrMoveFileContext(ctx context.Context, apiPath, srcDir, dstDir, name string) (string, error) {
	data, err := c.PostContext(ctx, apiPath, map[string]interface{}{
		"src_dir":   srcDir,
		"dst_dir":   dstDir,
		"overwrite": true,
		"names":     []string{name},
	}, nil)
	if err != nil {
		return "", fmt.Errorf("%s request failed: %w", apiPath, err)
	}
	var result struct {
		Tasks []struct {
			ID string `json:"id"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("%s response decode failed: %w", apiPath, err)
	}
	if len(result.Tasks) > 0 {
		return result.Tasks[0].ID, nil
	}
	return "", nil
}

func (c *AlistClient) CopyFileContext(ctx context.Context, srcDir, dstDir, name string) (string, error) {
	return c.copyOrMoveFileContext(ctx, "/api/fs/copy", srcDir, dstDir, name)
}

func (c *AlistClient) MoveFileContext(ctx context.Context, srcDir, dstDir, name string) (string, error) {
	return c.copyOrMoveFileContext(ctx, "/api/fs/move", srcDir, dstDir, name)
}

func alistTaskGroup(copyType taskItemType) string {
	if copyType == taskItemTypeMove {
		return "move"
	}
	return "copy"
}

func (c *AlistClient) taskActionContext(ctx context.Context, taskID string, copyType taskItemType, action string) (json.RawMessage, error) {
	apiPath := fmt.Sprintf("/api/admin/task/%s/%s", alistTaskGroup(copyType), action)
	return c.PostContext(ctx, apiPath, nil, map[string]string{"tid": taskID})
}

func (c *AlistClient) TaskInfoContext(ctx context.Context, taskID string, copyType taskItemType) (map[string]interface{}, error) {
	data, err := c.taskActionContext(ctx, taskID, copyType, "info")
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// CopyTaskDone gets completed copy tasks
func (c *AlistClient) CopyTaskDone() (json.RawMessage, error) {
	return c.GetContext(context.Background(), "/api/admin/task/copy/done", nil)
}

// CopyTaskUnDone gets uncompleted copy tasks
func (c *AlistClient) CopyTaskUnDone() (json.RawMessage, error) {
	return c.GetContext(context.Background(), "/api/admin/task/copy/undone", nil)
}

// CopyTaskRetry retries a copy task
func (c *AlistClient) CopyTaskRetry(taskID string) error {
	_, err := c.PostContext(context.Background(), "/api/admin/task/copy/retry", nil, map[string]string{"tid": taskID})
	return err
}

// CopyTaskClearSucceeded clears completed copy tasks
func (c *AlistClient) CopyTaskClearSucceeded() error {
	_, err := c.PostContext(context.Background(), "/api/admin/task/copy/clear_succeeded", nil, nil)
	return err
}

func (c *AlistClient) CopyTaskDeleteContext(ctx context.Context, taskID string) error {
	return c.TaskDeleteContext(ctx, taskID, taskItemTypeCopy)
}

func (c *AlistClient) CopyTaskCancelContext(ctx context.Context, taskID string) error {
	return c.TaskCancelContext(ctx, taskID, taskItemTypeCopy)
}

func (c *AlistClient) TaskDeleteContext(ctx context.Context, taskID string, copyType taskItemType) error {
	_, err := c.taskActionContext(ctx, taskID, copyType, "delete")
	return err
}

func (c *AlistClient) TaskCancelContext(ctx context.Context, taskID string, copyType taskItemType) error {
	_, err := c.taskActionContext(ctx, taskID, copyType, "cancel")
	return err
}
