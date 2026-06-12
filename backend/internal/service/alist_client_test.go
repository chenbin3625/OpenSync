package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestFileListEntryMetadataUsesHashInfoMD5(t *testing.T) {
	entry := FileListEntry{
		Name: "video.mkv",
		Size: 1024,
		HashInfo: map[string]interface{}{
			"md5": "ABCDEF0123456789",
		},
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

func TestGetClientByIDCoalescesConcurrentLoads(t *testing.T) {
	alistClientListMu.Lock()
	oldList := alistClientList
	alistClientList = make(map[int64]*AlistClient)
	alistClientListMu.Unlock()
	oldGet := getAlistByID
	oldNew := newAlistClient
	defer func() {
		alistClientListMu.Lock()
		alistClientList = oldList
		alistClientListMu.Unlock()
		getAlistByID = oldGet
		newAlistClient = oldNew
	}()

	var loads atomic.Int64
	getAlistByID = func(alistID int64) (map[string]interface{}, error) {
		return map[string]interface{}{
			"url":   "https://example.test",
			"token": "token",
		}, nil
	}
	newAlistClient = func(alistURL string, token string, alistID int64) (*AlistClient, error) {
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
