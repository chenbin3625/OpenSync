package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"opensync/internal/mapper"
)

func TestFileListEntryMetadataUsesHashInfoMD5(t *testing.T) {
	entry := FileListEntry{
		Name:     "video.mkv",
		Size:     1024,
		HashInfo: json.RawMessage(`{"sha1":"deadbeef","md5":"ABCDEF0123456789"}`),
	}

	metadata := entry.metadata()

	if metadata.Size != 1024 {
		t.Fatalf("metadata.Size = %d, want 1024", metadata.Size)
	}
	if metadata.MD5 != "abcdef0123456789" {
		t.Fatalf("metadata.MD5 = %q, want lowercase md5", metadata.MD5)
	}
}

func TestFileListEntryMetadataParsesHashinfoString(t *testing.T) {
	entry := FileListEntry{
		Name:     "photo.jpg",
		Size:     2048,
		Hashinfo: `{"md5":"00112233445566778899aabbccddeeff"}`,
	}

	metadata := entry.metadata()

	if metadata.Size != 2048 {
		t.Fatalf("metadata.Size = %d, want 2048", metadata.Size)
	}
	if metadata.MD5 != "00112233445566778899aabbccddeeff" {
		t.Fatalf("metadata.MD5 = %q, want parsed md5", metadata.MD5)
	}
}

func TestMD5FromRawReadsUppercaseKeyWithoutAllocatingHashMap(t *testing.T) {
	raw := []byte(`{"sha256":"ff","MD5":"00112233445566778899aabbccddeeff"}`)
	if got := md5FromRaw(raw); got != "00112233445566778899aabbccddeeff" {
		t.Fatalf("md5FromRaw() = %q, want uppercase-key md5", got)
	}
	if got := md5FromRaw([]byte(`null`)); got != "" {
		t.Fatalf("md5FromRaw(null) = %q, want empty", got)
	}
}

func TestFileListApiContextStoresTypedMetadataWithoutEmptyDirMaps(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[{"name":"clips","is_dir":true,"size":0},{"name":"a.mkv","is_dir":false,"size":4096,"hash_info":{"md5":"Aa"}}],"total":2}}`))
	}))
	defer server.Close()

	client := &AlistClient{URL: server.URL, client: server.Client()}
	files, err := client.FileListApiContext(context.Background(), "/media", 0, 0)
	if err != nil {
		t.Fatalf("FileListApiContext() error: %v", err)
	}

	dir, ok := files["clips/"]
	if !ok {
		t.Fatalf("missing directory key clips/, got %#v", files)
	}
	if dir != (FileMetadata{}) {
		t.Fatalf("directory metadata = %#v, want zero value", dir)
	}

	file := files["a.mkv"]
	if file.Size != 4096 || file.MD5 != "aa" {
		t.Fatalf("file metadata = %#v, want size 4096 md5 aa", file)
	}
	if _, exists := files["clips"]; exists {
		t.Fatal("directory stored without trailing slash")
	}

	paths, err := client.FilePathList(context.Background(), "/media")
	if err != nil {
		t.Fatalf("FilePathList() error: %v", err)
	}
	if len(paths) != 1 || paths[0]["path"] != "clips" {
		t.Fatalf("FilePathList() = %#v, want clips", paths)
	}
}

func TestGetClientByIDCoalescesConcurrentLoads(t *testing.T) {
	alistClientListMu.Lock()
	oldList := alistClientList
	alistClientList = make(map[int64]*AlistClient)
	alistClientListMu.Unlock()
	oldGet := getAlistByID
	oldNew := newAlistClientContext
	defer func() {
		alistClientListMu.Lock()
		alistClientList = oldList
		alistClientListMu.Unlock()
		getAlistByID = oldGet
		newAlistClientContext = oldNew
	}()

	var loads atomic.Int64
	getAlistByID = func(alistID int64) (map[string]interface{}, error) {
		return map[string]interface{}{
			"url":   "https://example.test",
			"token": "token",
		}, nil
	}
	newAlistClientContext = func(ctx context.Context, alistURL string, token string, alistID int64) (*AlistClient, error) {
		loads.Add(1)
		time.Sleep(20 * time.Millisecond)
		return &AlistClient{URL: alistURL, Token: token, AlistID: alistID}, nil
	}

	const workers = 16
	var wg sync.WaitGroup
	wg.Add(workers)
	clients := make([]*AlistClient, workers)
	for i := 0; i < workers; i++ {
		i := i
		go func() {
			defer wg.Done()
			clients[i] = GetClientByID(7)
		}()
	}
	wg.Wait()

	if loads.Load() != 1 {
		t.Fatalf("newAlistClient called %d times, want 1", loads.Load())
	}
	for i, client := range clients {
		if client == nil {
			t.Fatalf("clients[%d] = nil", i)
		}
		if client != clients[0] {
			t.Fatalf("clients[%d] = different pointer, want shared cached client", i)
		}
	}
}

func TestGetClientByIDContextPassesCancellationToInitialLoad(t *testing.T) {
	alistClientListMu.Lock()
	oldList := alistClientList
	oldLoads := alistClientLoads
	alistClientList = make(map[int64]*AlistClient)
	alistClientLoads = make(map[int64]*alistClientLoad)
	alistClientListMu.Unlock()
	oldGet := getAlistByID
	oldNew := newAlistClientContext
	defer func() {
		alistClientListMu.Lock()
		alistClientList = oldList
		alistClientLoads = oldLoads
		alistClientListMu.Unlock()
		getAlistByID = oldGet
		newAlistClientContext = oldNew
	}()

	getAlistByID = func(alistID int64) (map[string]interface{}, error) {
		return map[string]interface{}{
			"url":   "https://example.test",
			"token": "token",
		}, nil
	}
	newAlistClientContext = func(ctx context.Context, alistURL string, token string, alistID int64) (*AlistClient, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatalf("GetClientByIDContext() panic = nil, want cancellation error panic")
		}
	}()
	GetClientByIDContext(ctx, 7)
}

func TestGetContextDoesNotSendContentTypeWithoutBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "" {
			t.Fatalf("Content-Type = %q, want empty for GET without body", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"username":"admin"}}`))
	}))
	defer server.Close()

	client := &AlistClient{
		URL:    server.URL,
		client: server.Client(),
	}

	if _, err := client.GetContext(context.Background(), "/api/me", nil); err != nil {
		t.Fatalf("GetContext() error: %v", err)
	}
}

func TestAlistRequestURLPreservesConfiguredBasePath(t *testing.T) {
	client := &AlistClient{URL: "https://alist.example.test/root/"}
	got, err := client.requestURL("/api/me", map[string]string{"page": "2"})
	if err != nil {
		t.Fatalf("requestURL() error: %v", err)
	}
	if got != "https://alist.example.test/root/api/me?page=2" {
		t.Fatalf("requestURL() = %q, want base path and query preserved", got)
	}
}

func TestValidateAlistURLRejectsQueryAndFragment(t *testing.T) {
	for _, raw := range []string{
		"https://alist.example.test/root?token=secret",
		"https://alist.example.test/root#fragment",
	} {
		if err := validateAlistURL(raw); err == nil {
			t.Fatalf("validateAlistURL(%q) error = nil, want rejection", raw)
		}
	}
}

func TestTaskUndoneListContextUsesOperationSpecificTaskEndpoint(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":[{"id":"task-1","state":1,"progress":50}]}`))
	}))
	defer server.Close()

	client := &AlistClient{
		URL:    server.URL,
		client: server.Client(),
	}

	tasks, err := client.TaskUndoneListContext(context.Background(), taskItemTypeCopy)
	if err != nil {
		t.Fatalf("TaskUndoneListContext() error: %v", err)
	}
	if len(tasks) != 1 || tasks[0].idString() != "task-1" {
		t.Fatalf("tasks = %#v, want one undone task", tasks)
	}
	if len(paths) != 1 || paths[0] != "/api/admin/task/copy/undone" {
		t.Fatalf("paths = %v, want [/api/admin/task/copy/undone]", paths)
	}
}

func TestTaskInfoContextUsesOperationSpecificTaskEndpoint(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"state":2,"progress":100}}`))
	}))
	defer server.Close()

	client := &AlistClient{
		URL:    server.URL,
		client: server.Client(),
	}

	if _, err := client.TaskInfoContext(context.Background(), "copy-task", taskItemTypeCopy); err != nil {
		t.Fatalf("copy TaskInfoContext() error: %v", err)
	}
	if _, err := client.TaskInfoContext(context.Background(), "move-task", taskItemTypeMove); err != nil {
		t.Fatalf("move TaskInfoContext() error: %v", err)
	}

	want := []string{"/api/admin/task/copy/info", "/api/admin/task/move/info"}
	if len(paths) != len(want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("paths[%d] = %q, want %q", i, paths[i], want[i])
		}
	}
}

func TestCheckWaitContextBoundsTrackedPathBuckets(t *testing.T) {
	client := &AlistClient{
		waits: make(map[string]time.Time),
	}

	for i := 0; i < maxAlistWaitBuckets+128; i++ {
		if err := client.CheckWaitContext(context.Background(), "/bucket-"+strconv.Itoa(i)+"/file.txt", 1); err != nil {
			t.Fatalf("CheckWaitContext() error: %v", err)
		}
	}

	if got := len(client.waits); got > maxAlistWaitBuckets {
		t.Fatalf("tracked wait buckets = %d, want <= %d", got, maxAlistWaitBuckets)
	}
}

type closeTrackingTransport struct {
	closed atomic.Bool
}

func (t *closeTrackingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"code":200,"message":"ok","data":{}}`)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func (t *closeTrackingTransport) CloseIdleConnections() {
	t.closed.Store(true)
}

func TestAlistClientCloseClosesIdleTransportConnections(t *testing.T) {
	transport := &closeTrackingTransport{}
	client := &AlistClient{
		client: &http.Client{Transport: transport},
	}

	client.Close()

	if !transport.closed.Load() {
		t.Fatalf("Close() did not close idle transport connections")
	}
}

func TestStoreAlistClientClosesReplacedClient(t *testing.T) {
	oldTransport := &closeTrackingTransport{}
	oldClient := &AlistClient{
		AlistID: 42,
		client:  &http.Client{Transport: oldTransport},
	}
	newClient := &AlistClient{AlistID: 42}

	alistClientListMu.Lock()
	previousList := alistClientList
	alistClientList = map[int64]*AlistClient{42: oldClient}
	alistClientListMu.Unlock()
	defer func() {
		alistClientListMu.Lock()
		alistClientList = previousList
		alistClientListMu.Unlock()
	}()

	storeAlistClient(42, newClient)

	if !oldTransport.closed.Load() {
		t.Fatalf("storeAlistClient() did not close the replaced client")
	}
	if got := GetClientByID(42); got != newClient {
		t.Fatalf("cached client = %#v, want new client", got)
	}
}

func TestUpdateClientKeepsCachedClientWhenDatabaseUpdateFails(t *testing.T) {
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	defer testDB.Close()
	if _, err := testDB.Exec(`CREATE TABLE alist_list(
		id integer primary key autoincrement,
		remark text,
		url text UNIQUE,
		userName text,
		token text
	)`); err != nil {
		t.Fatalf("create alist_list: %v", err)
	}
	if _, err := testDB.Exec("INSERT INTO alist_list(id, remark, url, userName, token) VALUES (1, '', 'https://old.test', 'old', 'old-token')"); err != nil {
		t.Fatalf("insert old alist: %v", err)
	}
	if _, err := testDB.Exec("INSERT INTO alist_list(id, remark, url, userName, token) VALUES (2, '', 'https://dupe.test', 'dupe', 'dupe-token')"); err != nil {
		t.Fatalf("insert duplicate alist: %v", err)
	}
	restoreDB := mapper.SetDBForTest(testDB)
	defer restoreDB()

	oldNew := newAlistClient
	newTransport := &closeTrackingTransport{}
	newClient := &AlistClient{AlistID: 1, URL: "https://dupe.test", client: &http.Client{Transport: newTransport}}
	newAlistClient = func(alistURL string, token string, alistID int64) (*AlistClient, error) {
		return newClient, nil
	}
	defer func() {
		newAlistClient = oldNew
	}()

	oldClient := &AlistClient{AlistID: 1, URL: "https://old.test"}
	alistClientListMu.Lock()
	previousList := alistClientList
	alistClientList = map[int64]*AlistClient{1: oldClient}
	alistClientListMu.Unlock()
	defer func() {
		alistClientListMu.Lock()
		alistClientList = previousList
		alistClientListMu.Unlock()
	}()

	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatalf("UpdateClient() did not panic on database constraint failure")
		}
		if got := GetClientByID(1); got != oldClient {
			t.Fatalf("cached client changed after failed update")
		}
		if !newTransport.closed.Load() {
			t.Fatalf("new client was not closed after failed update")
		}
	}()

	UpdateClient(map[string]interface{}{
		"id":     int64(1),
		"url":    "https://dupe.test",
		"token":  "new-token",
		"remark": "new",
	})
}

func TestValidateAlistURLAcceptsHTTPAndHTTPS(t *testing.T) {
	if err := validateAlistURL("alist.example.com"); err == nil {
		t.Fatalf("validateAlistURL() accepted URL without scheme")
	}
	if err := validateAlistURL("http://alist.example.com"); err != nil {
		t.Fatalf("validateAlistURL(http) error: %v", err)
	}
	if err := validateAlistURL("http://localhost:5244"); err != nil {
		t.Fatalf("validateAlistURL(localhost) error: %v", err)
	}
	if err := validateAlistURL("https://alist.example.com"); err != nil {
		t.Fatalf("validateAlistURL(https) error: %v", err)
	}
	if err := validateAlistURL("ftp://alist.example.com"); err == nil {
		t.Fatalf("validateAlistURL() accepted unsupported scheme")
	}
}

func TestNormalizeAlistTokenTrimsAndRejectsMissingRequiredToken(t *testing.T) {
	alist := map[string]interface{}{"token": "  token-value \n"}
	token, ok := normalizeAlistToken(alist, true)
	if !ok {
		t.Fatalf("normalizeAlistToken() ok = false, want true")
	}
	if token != "token-value" {
		t.Fatalf("token = %q, want trimmed token", token)
	}
	if alist["token"] != "token-value" {
		t.Fatalf("stored token = %q, want trimmed token", alist["token"])
	}

	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatalf("normalizeAlistToken() panic = nil, want missing required token panic")
		}
	}()
	normalizeAlistToken(map[string]interface{}{}, true)
}
