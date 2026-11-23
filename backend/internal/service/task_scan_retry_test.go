package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"opensync/internal/msg"
)

func TestListDirRetriesTransientAListFailures(t *testing.T) {
	oldDelay := scanListRetryDelay
	scanListRetryDelay = func(int) time.Duration { return 0 }
	defer func() { scanListRetryDelay = oldDelay }()

	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if attempts.Add(1) <= maxScanListRetries {
			_, _ = w.Write([]byte(`{"code":500,"message":"failed get dir: object not found","data":null}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[{"name":"file.txt","is_dir":false,"size":10}]}}`))
	}))
	defer server.Close()

	jt := newScanRetryTestTask(server)
	files, err := jt.listDir("/dst/", true, nil, "/dst/", false)
	if err != nil {
		t.Fatalf("listDir() error: %v", err)
	}
	if attempts.Load() != maxScanListRetries+1 {
		t.Fatalf("list attempts = %d, want %d", attempts.Load(), maxScanListRetries+1)
	}
	metadata, ok := files["file.txt"]
	if !ok || metadata.Size != 10 {
		t.Fatalf("listed file metadata = %#v, want size 10", files["file.txt"])
	}
}

func TestListDirDoesNotRetryAuthenticationFailure(t *testing.T) {
	oldDelay := scanListRetryDelay
	scanListRetryDelay = func(int) time.Duration { return 0 }
	defer func() { scanListRetryDelay = oldDelay }()

	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":401,"message":"unauthorized","data":null}`))
	}))
	defer server.Close()

	var persisted []map[string]interface{}
	restorePersist := stubPersistJobTaskItems(t, &persisted, nil)
	defer restorePersist()

	jt := newScanRetryTestTask(server)
	if _, err := jt.listDir("/dst/", true, nil, "/dst/", false); err == nil {
		t.Fatal("listDir() error = nil, want authentication error")
	}
	if err := jt.flushPersistBuffer(); err != nil {
		t.Fatalf("flushPersistBuffer() error: %v", err)
	}
	if attempts.Load() != 1 {
		t.Fatalf("list attempts = %d, want 1", attempts.Load())
	}
	if len(persisted) != 1 {
		t.Fatalf("persisted failures = %d, want 1", len(persisted))
	}
}

func TestShouldRetryScanListHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if shouldRetryScanList(ctx, errors.New("temporary failure")) {
		t.Fatal("shouldRetryScanList() = true for canceled context")
	}
	if shouldRetryScanList(context.Background(), errors.New(msg.AlistUnAuth)) {
		t.Fatal("shouldRetryScanList() = true for authentication failure")
	}
	if !shouldRetryScanList(context.Background(), errors.New("temporary failure")) {
		t.Fatal("shouldRetryScanList() = false for transient failure")
	}
}

// Issue 8: missing destination root in listSrcAndDst auto-creates dir without 3x retry or failed item.
func TestListSrcAndDstAutoCreatesMissingDstRootWithoutFailedItem(t *testing.T) {
	oldDelay := scanListRetryDelay
	scanListRetryDelay = func(int) time.Duration { return 0 }
	defer func() { scanListRetryDelay = oldDelay }()

	var dstListCalls atomic.Int32
	var dstListBeforeMkdir atomic.Int32
	var mkdirCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch r.URL.Path {
		case "/api/fs/list":
			if body["path"].(string) == "/src/" {
				_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[{"name":"a.txt","size":10,"is_dir":false}]}}`))
				return
			}
			dstListCalls.Add(1)
			if mkdirCalls.Load() == 0 {
				dstListBeforeMkdir.Add(1)
				_, _ = w.Write([]byte(`{"code":500,"message":"failed get dir: object not found","data":null}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[]}}`))
		case "/api/fs/mkdir":
			mkdirCalls.Add(1)
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{}}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	var persisted []map[string]interface{}
	restorePersist := stubPersistJobTaskItems(t, &persisted, nil)
	defer restorePersist()

	jt := newScanRetryTestTask(server)
	srcFiles, dstFiles, err := jt.listSrcAndDst("/src/", "/dst/", nil, "/src/", "/dst/", true)
	if err != nil {
		t.Fatalf("listSrcAndDst() error: %v", err)
	}
	if err := jt.flushPersistBuffer(); err != nil {
		t.Fatalf("flushPersistBuffer() error: %v", err)
	}
	if len(srcFiles) != 1 || len(dstFiles) != 0 {
		t.Fatalf("srcFiles=%d dstFiles=%d, want 1/0", len(srcFiles), len(dstFiles))
	}
	if got := dstListBeforeMkdir.Load(); got != 1 {
		t.Fatalf("dstListBeforeMkdir = %d, want 1 (no 3x retry before mkdir)", got)
	}
	if got := dstListCalls.Load(); got != 2 {
		t.Fatalf("dstListCalls = %d, want 2 (1 initial + 1 after mkdir)", got)
	}
	if got := mkdirCalls.Load(); got != 1 {
		t.Fatalf("mkdirCalls = %d, want 1", got)
	}
	if len(persisted) != 0 {
		t.Fatalf("persisted failed items = %#v, want none", persisted)
	}
}

// Issue 9: retryTaskItem validates current job scope, excludes, method, and source existence before Delete.
func TestRetryTaskItemValidatesScopeMethodExcludesAndSourceExistence(t *testing.T) {
	var removeCalls atomic.Int32
	var sourceExists atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/fs/get":
			if sourceExists.Load() {
				_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"name":"reappeared.txt","size":10,"is_dir":false}}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":500,"message":"object not found","data":null}`))
		case "/api/fs/remove":
			removeCalls.Add(1)
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{}}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	restorePersist := stubPersistJobTaskItems(t, nil, nil)
	defer restorePersist()

	jt := newScanRetryTestTask(server)
	jt.Job = map[string]interface{}{
		"srcPath": `["/src"]`,
		"dstPath": `["/dst"]`,
		"method":  1, // Full Sync
		"exclude": "*.tmp",
	}

	// 1. Out-of-scope srcPath/dstPath -> skipped
	jt.retryTaskItem(map[string]interface{}{
		"type":     taskItemTypeCopy.Int(),
		"srcPath":  "/other-src/",
		"dstPath":  "/dst/",
		"fileName": "a.txt",
		"fileSize": int64(10),
	})
	if jt.Waiting.len() != 0 {
		t.Fatalf("out-of-scope copy was queued: %d", jt.Waiting.len())
	}

	// 2. Newly excluded file -> skipped
	jt.retryTaskItem(map[string]interface{}{
		"type":     taskItemTypeCopy.Int(),
		"srcPath":  "/src/",
		"dstPath":  "/dst/",
		"fileName": "ignore.tmp",
		"fileSize": int64(10),
	})
	if jt.Waiting.len() != 0 {
		t.Fatalf("excluded file copy was queued: %d", jt.Waiting.len())
	}

	// 3. Delete when source object reappeared -> NOT deleted!
	sourceExists.Store(true)
	jt.retryTaskItem(map[string]interface{}{
		"type":     taskItemTypeDelete.Int(),
		"dstPath":  "/dst/",
		"fileName": "reappeared.txt",
		"fileSize": int64(10),
		"isPath":   taskItemFile.Int(),
	})
	if removeCalls.Load() != 0 {
		t.Fatalf("removeCalls = %d, want 0 when source object exists again", removeCalls.Load())
	}

	// 4. Delete when source object is truly absent -> executed
	sourceExists.Store(false)
	jt.retryTaskItem(map[string]interface{}{
		"type":     taskItemTypeDelete.Int(),
		"dstPath":  "/dst/",
		"fileName": "gone.txt",
		"fileSize": int64(10),
		"isPath":   taskItemFile.Int(),
	})
	if removeCalls.Load() != 1 {
		t.Fatalf("removeCalls = %d, want 1 when source object is absent", removeCalls.Load())
	}

	// 5. Delete when job was edited to non-Full-Sync (method=0) -> skipped
	jt.Job["method"] = 0
	jt.retryTaskItem(map[string]interface{}{
		"type":     taskItemTypeDelete.Int(),
		"dstPath":  "/dst/",
		"fileName": "gone2.txt",
		"fileSize": int64(10),
		"isPath":   taskItemFile.Int(),
	})
	if removeCalls.Load() != 1 {
		t.Fatalf("removeCalls = %d, want 1 (Delete skipped when method != 1)", removeCalls.Load())
	}
}

func newScanRetryTestTask(server *httptest.Server) *JobTask {
	client := server.Client()
	client.Timeout = time.Second
	jt := &JobTask{
		TaskID:  42,
		Job:     map[string]interface{}{},
		Waiting: newCopyQueue(),
		AlistClient: &AlistClient{
			URL:    server.URL,
			client: client,
		},
	}
	jt.initRuntime()
	return jt
}
