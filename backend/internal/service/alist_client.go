package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"opensync/internal/config"
	"opensync/internal/msg"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// defaultMaxListResponseBytes is the cap for a single AList list/API
	// response. Overridden by OPENSYNC_MAX_LIST_BYTES (must be >= 1MB).
	defaultMaxListResponseBytes = 32 << 20 // 32MB
	maxUndoneResponseBytes      = 8 << 20  // 8MB; task snapshots should be small
	maxFileListEntries          = 100000
	fileListPageSize            = 500
	maxFileListPages            = 10000
	maxAlistWaitBuckets         = 1024
	alistValidationTimeout      = 30 * time.Second
)

// maxResponseBytes is the current response-size cap. Kept as a mutable package
// variable so tests can lower it; production uses loadMaxListResponseBytes().
var maxResponseBytes = loadMaxListResponseBytes()

func loadMaxListResponseBytes() int64 {
	if raw := strings.TrimSpace(os.Getenv("OPENSYNC_MAX_LIST_BYTES")); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n >= (1<<20) {
			return n
		}
	}
	return defaultMaxListResponseBytes
}

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
	return NewAlistClientContext(context.Background(), alistURL, token, alistID)
}

// NewAlistClientContext creates a new AList client and validates it with ctx.
func NewAlistClientContext(ctx context.Context, alistURL string, token string, alistID int64) (*AlistClient, error) {
	normalizedURL, err := normalizeAlistBaseURL(alistURL)
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		MaxConnsPerHost:       32,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}
	if !config.GetConfig().Server.AllowInternalAlist {
		// Mirror notify's SSRF protection for AList connections. Enabled by
		// default is NOT harmful here (internal AList is the normal deployment);
		// operators who run OpenSync on the public internet can turn it on.
		transport.DialContext = ssrfSafeDialContext(&net.Dialer{Timeout: 15 * time.Second}, func() bool {
			return config.GetConfig().Server.AllowInternalAlist
		})
	}
	c := &AlistClient{
		URL:     normalizedURL,
		Token:   token,
		AlistID: alistID,
		waits:   make(map[string]time.Time),
		client: &http.Client{
			Timeout:   300 * time.Second,
			Transport: transport,
		},
	}
	if err := c.getUserContext(ctx); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

// Close releases idle HTTP connections held by this client.
func (c *AlistClient) Close() {
	if c == nil || c.client == nil || c.client.Transport == nil {
		return
	}
	if transport, ok := c.client.Transport.(interface{ CloseIdleConnections() }); ok {
		transport.CloseIdleConnections()
	}
}

func (c *AlistClient) doRequestContext(ctx context.Context, method, apiPath string, data interface{}, params map[string]string) (json.RawMessage, error) {
	return c.doRequestContextLimit(ctx, method, apiPath, data, params, maxResponseBytes)
}

func (c *AlistClient) doRequestContextLimit(ctx context.Context, method, apiPath string, data interface{}, params map[string]string, responseLimit int64) (json.RawMessage, error) {
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

	reqURL, err := c.requestURL(apiPath, params)
	if err != nil {
		return nil, errors.New(msg.AddressIncorrect)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return nil, errors.New(msg.AddressIncorrect)
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
			return nil, errors.New(msg.AlistConnectFail)
		}
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := readAllWithLimit(resp.Body, responseLimit)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s (HTTP %d)", msg.CodeNot200, resp.StatusCode)
	}

	var res alistResponse
	if err := json.Unmarshal(respBody, &res); err != nil {
		return nil, err
	}

	if res.Code == 401 {
		if c.AlistID > 0 {
			removeCachedAlistClient(c.AlistID)
		}
		return nil, errors.New(msg.AlistUnAuth)
	}
	if res.Code != 200 {
		return nil, errors.New(msg.AlistFailCodeReason(res.Code, res.Message))
	}

	return res.Data, nil
}

func normalizeAlistBaseURL(rawURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New(msg.AlistURLInvalid)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", errors.New(msg.AlistURLInvalid)
	}
	u.Scheme = scheme
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = ""
	return u.String(), nil
}

func (c *AlistClient) requestURL(apiPath string, params map[string]string) (string, error) {
	base, err := url.Parse(c.URL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return "", errors.New(msg.AddressIncorrect)
	}
	endpoint, err := url.Parse(apiPath)
	if err != nil || endpoint.IsAbs() || endpoint.Host != "" {
		return "", errors.New(msg.AddressIncorrect)
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/" + strings.TrimLeft(endpoint.Path, "/")
	base.RawPath = ""
	query := base.Query()
	for key, value := range endpoint.Query() {
		for _, item := range value {
			query.Add(key, item)
		}
	}
	for key, value := range params {
		query.Set(key, value)
	}
	base.RawQuery = query.Encode()
	return base.String(), nil
}

func (c *AlistClient) PostContext(ctx context.Context, apiPath string, data interface{}, params map[string]string) (json.RawMessage, error) {
	return c.doRequestContext(ctx, "POST", apiPath, data, params)
}

func (c *AlistClient) GetContext(ctx context.Context, apiPath string, params map[string]string) (json.RawMessage, error) {
	return c.doRequestContext(ctx, "GET", apiPath, nil, params)
}

func (c *AlistClient) getUser() error {
	return c.getUserContext(context.Background())
}

func (c *AlistClient) getUserContext(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, alistValidationTimeout)
	defer cancel()
	data, err := c.GetContext(ctx, "/api/me", nil)
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
	Modified int64                  `json:"modified"`
	HashInfo map[string]interface{} `json:"hash_info"`
	Hashinfo string                 `json:"hashinfo"`
}

// FileMetadata contains lightweight comparison data from AList list results.
type FileMetadata struct {
	Size     int64
	MD5      string
	Modified int64
}

func (e FileListEntry) metadata() FileMetadata {
	return FileMetadata{
		Size:     e.Size,
		MD5:      normalizeMD5(firstMD5(e.HashInfo, e.Hashinfo)),
		Modified: e.Modified,
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

	result := make(FileListResult)
	readTotal := 0
	for page := 1; page <= maxFileListPages; page++ {
		data, err := c.PostContext(ctx, "/api/fs/list", map[string]interface{}{
			"path":     path,
			"refresh":  useCache != 1,
			"page":     page,
			"per_page": fileListPageSize,
		}, nil)
		if err != nil {
			return nil, err
		}

		var content struct {
			Content []FileListEntry `json:"content"`
			Total   int             `json:"total"`
		}
		if err := json.Unmarshal(data, &content); err != nil {
			return nil, err
		}
		readTotal += len(content.Content)
		for _, item := range content.Content {
			if item.Name == "" {
				continue
			}
			if item.IsDir {
				result[item.Name+"/"] = map[string]interface{}{}
			} else {
				result[item.Name] = item.metadata()
			}
		}
		if len(result) >= maxFileListEntries {
			return nil, fmt.Errorf("AList directory contains more than %d entries", maxFileListEntries)
		}
		if len(content.Content) == 0 || content.Total <= readTotal || len(content.Content) < fileListPageSize {
			break
		}
		if page == maxFileListPages {
			return nil, fmt.Errorf("AList directory listing exceeded %d pages", maxFileListPages)
		}
	}
	return result, nil
}

// FilePathList gets subdirectory list for path selector
func (c *AlistClient) FilePathList(ctx context.Context, path string) ([]map[string]string, error) {
	files, err := c.FileListApiContext(ctx, path, 0, 0)
	if err != nil {
		return nil, err
	}

	var result []map[string]string
	for name := range files {
		if strings.HasSuffix(name, "/") {
			result = append(result, map[string]string{"path": strings.TrimSuffix(name, "/")})
		}
	}
	if result == nil {
		result = []map[string]string{}
	}
	return result, nil
}

// FileExistsContext reports whether a direct child named `name` exists in `dir`.
func (c *AlistClient) FileExistsContext(ctx context.Context, dir, name string) (bool, error) {
	return c.FileGetContext(ctx, dir, name)
}

// FileGetContext checks a single file via /api/fs/get (cheaper than listing the directory).
func (c *AlistClient) FileGetContext(ctx context.Context, dir, name string) (bool, error) {
	dir = strings.TrimRight(strings.TrimSpace(dir), "/")
	filePath := dir + "/" + strings.TrimSpace(name)
	_, err := c.PostContext(ctx, "/api/fs/get", map[string]interface{}{
		"path": filePath,
	}, nil)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "404") || strings.Contains(strings.ToLower(errMsg), "not found") {
			return false, nil
		}
		return false, err
	}
	return true, nil
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

func (c *AlistClient) TaskUndoneListContext(ctx context.Context, copyType taskItemType) ([]map[string]interface{}, error) {
	apiPath := fmt.Sprintf("/api/admin/task/%s/undone", alistTaskGroup(copyType))
	data, err := c.doRequestContextLimit(ctx, http.MethodPost, apiPath, map[string]interface{}{}, nil, maxUndoneResponseBytes)
	if err != nil {
		return nil, err
	}
	var tasks []map[string]interface{}
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, err
	}
	if tasks == nil {
		tasks = []map[string]interface{}{}
	}
	return tasks, nil
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

func (c *AlistClient) TaskDeleteContext(ctx context.Context, taskID string, copyType taskItemType) error {
	_, err := c.taskActionContext(ctx, taskID, copyType, "delete")
	return err
}

func (c *AlistClient) TaskCancelContext(ctx context.Context, taskID string, copyType taskItemType) error {
	_, err := c.taskActionContext(ctx, taskID, copyType, "cancel")
	return err
}
