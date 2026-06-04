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
	"strings"
	"sync"
	"opensync/internal/i18n"
	"time"
)

// AlistClient represents an AList HTTP client
type AlistClient struct {
	URL     string
	Token   string
	User    string
	AlistID int64
	waits   map[string]float64
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
		waits:   make(map[string]float64),
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

func (c *AlistClient) doRequest(method, apiPath string, data interface{}, params map[string]string) (json.RawMessage, error) {
	return c.doRequestContext(context.Background(), method, apiPath, data, params)
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
	req.Header.Set("Content-Type", "application/json")
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

	respBody, err := io.ReadAll(resp.Body)
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

// Post sends a POST request
func (c *AlistClient) Post(apiPath string, data interface{}, params map[string]string) (json.RawMessage, error) {
	return c.doRequest("POST", apiPath, data, params)
}

func (c *AlistClient) PostContext(ctx context.Context, apiPath string, data interface{}, params map[string]string) (json.RawMessage, error) {
	return c.doRequestContext(ctx, "POST", apiPath, data, params)
}

// Get sends a GET request
func (c *AlistClient) Get(apiPath string, params map[string]string) (json.RawMessage, error) {
	return c.doRequest("GET", apiPath, nil, params)
}

func (c *AlistClient) GetContext(ctx context.Context, apiPath string, params map[string]string) (json.RawMessage, error) {
	return c.doRequestContext(ctx, "GET", apiPath, nil, params)
}

func (c *AlistClient) getUser() error {
	data, err := c.Get("/api/me", nil)
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

// CheckWait checks if we need to wait based on scan interval
func (c *AlistClient) CheckWait(path string, scanInterval int) {
	_ = c.CheckWaitContext(context.Background(), path, scanInterval)
}

func (c *AlistClient) CheckWaitContext(ctx context.Context, path string, scanInterval int) error {
	if scanInterval == 0 {
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

	now := float64(time.Now().UnixNano()) / float64(time.Second)
	waitUntil := now

	c.mu.Lock()
	if lastTime, ok := c.waits[pathFirst]; ok && now-lastTime < float64(scanInterval) {
		waitUntil = lastTime + float64(scanInterval)
	}
	c.waits[pathFirst] = waitUntil
	c.mu.Unlock()

	if waitUntil > now {
		timer := time.NewTimer(time.Duration((waitUntil - now) * float64(time.Second)))
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// FileListResponse represents a file list entry
type FileListEntry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

// FileListResult maps filename -> size (for files) or empty map (for dirs)
// Directories have key ending with "/"
type FileListResult = map[string]interface{}

// FileListApi gets directory listing
func (c *AlistClient) FileListApi(path string, useCache int, scanInterval int) (FileListResult, error) {
	return c.FileListApiContext(context.Background(), path, useCache, scanInterval)
}

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
				result[item.Name] = item.Size
			}
		}
	}
	return result, nil
}

// FilePathList gets subdirectory list for path selector
func (c *AlistClient) FilePathList(path string) ([]map[string]string, error) {
	data, err := c.Post("/api/fs/list", map[string]interface{}{
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

// Mkdir creates a directory
func (c *AlistClient) Mkdir(path string, scanInterval int) error {
	return c.MkdirContext(context.Background(), path, scanInterval)
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

// DeleteFile deletes files/directories
func (c *AlistClient) DeleteFile(path string, names []string, scanInterval int) error {
	return c.DeleteFileContext(context.Background(), path, names, scanInterval)
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

// CopyFile copies a file and returns the task ID
func (c *AlistClient) CopyFile(srcDir, dstDir, name string) (string, error) {
	return c.CopyFileContext(context.Background(), srcDir, dstDir, name)
}

func (c *AlistClient) CopyFileContext(ctx context.Context, srcDir, dstDir, name string) (string, error) {
	data, err := c.PostContext(ctx, "/api/fs/copy", map[string]interface{}{
		"src_dir":   srcDir,
		"dst_dir":   dstDir,
		"overwrite": true,
		"names":     []string{name},
	}, nil)
	if err != nil {
		return "", err
	}
	var result struct {
		Tasks []struct {
			ID string `json:"id"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", err
	}
	if len(result.Tasks) > 0 {
		return result.Tasks[0].ID, nil
	}
	return "", nil
}

// MoveFile moves a file and returns the task ID
func (c *AlistClient) MoveFile(srcDir, dstDir, name string) (string, error) {
	data, err := c.Post("/api/fs/move", map[string]interface{}{
		"src_dir":   srcDir,
		"dst_dir":   dstDir,
		"overwrite": true,
		"names":     []string{name},
	}, nil)
	if err != nil {
		return "", err
	}
	var result struct {
		Tasks []struct {
			ID string `json:"id"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", err
	}
	if len(result.Tasks) > 0 {
		return result.Tasks[0].ID, nil
	}
	return "", nil
}

// TaskInfo gets task details
func (c *AlistClient) TaskInfo(taskID string) (map[string]interface{}, error) {
	return c.TaskInfoContext(context.Background(), taskID)
}

func (c *AlistClient) TaskInfoContext(ctx context.Context, taskID string) (map[string]interface{}, error) {
	data, err := c.PostContext(ctx, "/api/admin/task/copy/info", nil, map[string]string{"tid": taskID})
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
	return c.Get("/api/admin/task/copy/done", nil)
}

// CopyTaskUnDone gets uncompleted copy tasks
func (c *AlistClient) CopyTaskUnDone() (json.RawMessage, error) {
	return c.Get("/api/admin/task/copy/undone", nil)
}

// CopyTaskRetry retries a copy task
func (c *AlistClient) CopyTaskRetry(taskID string) error {
	_, err := c.Post("/api/admin/task/copy/retry", nil, map[string]string{"tid": taskID})
	return err
}

// CopyTaskClearSucceeded clears completed copy tasks
func (c *AlistClient) CopyTaskClearSucceeded() error {
	_, err := c.Post("/api/admin/task/copy/clear_succeeded", nil, nil)
	return err
}

// CopyTaskDelete deletes a copy task record
func (c *AlistClient) CopyTaskDelete(taskID string) error {
	return c.CopyTaskDeleteContext(context.Background(), taskID)
}

func (c *AlistClient) CopyTaskDeleteContext(ctx context.Context, taskID string) error {
	_, err := c.PostContext(ctx, "/api/admin/task/copy/delete", nil, map[string]string{"tid": taskID})
	return err
}

// CopyTaskCancel cancels a copy task
func (c *AlistClient) CopyTaskCancel(taskID string) error {
	return c.CopyTaskCancelContext(context.Background(), taskID)
}

func (c *AlistClient) CopyTaskCancelContext(ctx context.Context, taskID string) error {
	_, err := c.PostContext(ctx, "/api/admin/task/copy/cancel", nil, map[string]string{"tid": taskID})
	return err
}
