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
	maxFileListEntries          = 100000
	maxFileListPages            = 10000
	maxAlistWaitBuckets         = 1024
	alistValidationTimeout      = 30 * time.Second
)

// fileListPageSize is the AList /api/fs/list page size. Mutable in tests so
// parallel remaining-page fetches can be proven without 500-entry fixtures.
var fileListPageSize = 500

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
	// baseURL is parsed once when the client is created. The URL is immutable
	// for the lifetime of a cached client, so requestURL can copy this small
	// value instead of reparsing the base string on every AList call.
	baseURL url.URL
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
	parsedURL, err := url.Parse(normalizedURL)
	if err != nil {
		return nil, errors.New(msg.AlistURLInvalid)
	}
	c := &AlistClient{
		URL:     normalizedURL,
		Token:   token,
		AlistID: alistID,
		waits:   make(map[string]time.Time),
		client:  newAlistHTTPClient(),
		baseURL: *parsedURL,
	}
	if err := c.getUserContext(ctx); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

// Close releases idle HTTP connections held by this client, including any
// HTTP/3 UDP sockets created after an Alt-Svc upgrade.
func (c *AlistClient) Close() {
	if c == nil || c.client == nil || c.client.Transport == nil {
		return
	}
	switch transport := c.client.Transport.(type) {
	case io.Closer:
		_ = transport.Close()
	case interface{ CloseIdleConnections() }:
		transport.CloseIdleConnections()
	}
}

func (c *AlistClient) doRequestContext(ctx context.Context, method, apiPath string, data interface{}, params map[string]string) (json.RawMessage, error) {
	return c.doRequestContextLimit(ctx, method, apiPath, data, params, maxResponseBytes)
}

func (c *AlistClient) startRequest(ctx context.Context, method, apiPath string, data interface{}, params map[string]string) (*http.Response, error) {
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
	return resp, nil
}

func (c *AlistClient) doRequestContextLimit(ctx context.Context, method, apiPath string, data interface{}, params map[string]string, responseLimit int64) (json.RawMessage, error) {
	resp, err := c.startRequest(ctx, method, apiPath, data, params)
	if err != nil {
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
	if err := c.checkAlistCode(res.Code, res.Message); err != nil {
		return nil, err
	}
	return res.Data, nil
}

func (c *AlistClient) checkAlistCode(code int, message string) error {
	if code == 401 {
		if c.AlistID > 0 {
			removeCachedAlistClient(c.AlistID)
		}
		return errors.New(msg.AlistUnAuth)
	}
	if code != 200 {
		return errors.New(msg.AlistFailCodeReason(code, message))
	}
	return nil
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
	base := c.baseURL
	if base.Scheme == "" || base.Host == "" {
		parsed, err := url.Parse(c.URL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return "", errors.New(msg.AddressIncorrect)
		}
		base = *parsed
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

	pathFirst := firstAlistPathSegment(path)
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

// firstAlistPathSegment preserves the historical SplitN(path, "/", 3)[1]
// behavior while avoiding a temporary slice on every rate-limited AList
// operation. AList paths normally start with '/', but the no-leading-slash
// form is kept compatible for callers and tests.
func firstAlistPathSegment(path string) string {
	firstSlash := strings.IndexByte(path, '/')
	if firstSlash < 0 {
		return ""
	}
	rest := path[firstSlash+1:]
	if nextSlash := strings.IndexByte(rest, '/'); nextSlash >= 0 {
		return rest[:nextSlash]
	}
	return rest
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
	Name     string      `json:"name"`
	IsDir    bool        `json:"is_dir"`
	Size     int64       `json:"size"`
	Modified int64       `json:"modified"`
	HashInfo interface{} `json:"hash_info"`
	Hashinfo string      `json:"hashinfo"`
}

func (e *FileListEntry) UnmarshalJSON(data []byte) error {
	var raw struct {
		Name     string          `json:"name"`
		IsDir    bool            `json:"is_dir"`
		Size     int64           `json:"size"`
		Modified int64           `json:"modified"`
		HashInfo json.RawMessage `json:"hash_info"`
		Hashinfo string          `json:"hashinfo"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	e.Name, e.IsDir, e.Size, e.Modified, e.Hashinfo = raw.Name, raw.IsDir, raw.Size, raw.Modified, raw.Hashinfo
	if len(raw.HashInfo) > 0 && string(raw.HashInfo) != "null" {
		e.HashInfo = raw.HashInfo
	}
	return nil
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
		MD5:      normalizeMD5(md5FromHashFields(e.HashInfo, e.Hashinfo)),
		Modified: e.Modified,
	}
}

func md5FromHashFields(hashInfo interface{}, hashinfo string) string {
	switch value := hashInfo.(type) {
	case json.RawMessage:
		if md5 := md5FromRaw(value); md5 != "" {
			return md5
		}
	case map[string]interface{}:
		if md5, ok := value["md5"].(string); ok {
			return md5
		}
		if md5, ok := value["MD5"].(string); ok {
			return md5
		}
	}
	if hashinfo != "" {
		return md5FromRaw([]byte(hashinfo))
	}
	return ""
}

// md5FromRaw pulls the md5/MD5 string out of a small JSON object without
// allocating a map[string]interface{} per file. AList list pages are 500
// entries; the old decoder boxed every hash key on the scan hot path.
func md5FromRaw(raw []byte) string {
	if value := jsonQuotedString(raw, `"md5"`); value != "" {
		return normalizeMD5(value)
	}
	return normalizeMD5(jsonQuotedString(raw, `"MD5"`))
}

func jsonQuotedString(raw []byte, quotedKey string) string {
	idx := bytes.Index(raw, []byte(quotedKey))
	if idx < 0 {
		return ""
	}
	rest := bytes.TrimSpace(raw[idx+len(quotedKey):])
	if len(rest) == 0 || rest[0] != ':' {
		return ""
	}
	rest = bytes.TrimSpace(rest[1:])
	if len(rest) < 2 || rest[0] != '"' {
		return ""
	}
	rest = rest[1:]
	end := 0
	for end < len(rest) {
		if rest[end] == '\\' {
			end += 2
			if end > len(rest) {
				return ""
			}
			continue
		}
		if rest[end] == '"' {
			return string(rest[:end])
		}
		end++
	}
	return ""
}

func normalizeMD5(md5 string) string {
	return strings.ToLower(strings.TrimSpace(md5))
}

// FileListResult maps filename -> comparison metadata.
// Directories use a key ending with "/" and a zero FileMetadata value so a
// 10k-entry listing does not allocate 10k empty maps or box every file in
// interface{}.
type FileListResult = map[string]FileMetadata

// FilePathList gets subdirectory list for path selector
func (c *AlistClient) FilePathList(ctx context.Context, path string) ([]map[string]string, error) {
	files, err := c.FileListApiContext(ctx, path, 0, 0)
	if err != nil {
		return nil, err
	}

	result := make([]map[string]string, 0, len(files))
	for name := range files {
		if strings.HasSuffix(name, "/") {
			result = append(result, map[string]string{"path": strings.TrimSuffix(name, "/")})
		}
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
	_, err := c.PostContext(ctx, "/api/fs/get", alistPathRequest{Path: filePath}, nil)
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
	_, err := c.PostContext(ctx, "/api/fs/mkdir", alistPathRequest{Path: path}, nil)
	return err
}

func (c *AlistClient) DeleteFileContext(ctx context.Context, path string, names []string, scanInterval int) error {
	if err := c.CheckWaitContext(ctx, path, scanInterval); err != nil {
		return err
	}
	_, err := c.PostContext(ctx, "/api/fs/remove", alistRemoveRequest{Names: names, Dir: path}, nil)
	return err
}

func (c *AlistClient) copyOrMoveFileContext(ctx context.Context, apiPath, srcDir, dstDir, name string) (string, error) {
	data, err := c.PostContext(ctx, apiPath, alistCopyMoveRequest{
		SrcDir:    srcDir,
		DstDir:    dstDir,
		Overwrite: true,
		Names:     []string{name},
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
	data, err := c.PostContext(ctx, apiPath, map[string]interface{}{}, nil)
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
