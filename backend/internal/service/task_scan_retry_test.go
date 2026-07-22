package service

import (
	"context"
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
	metadata, ok := files["file.txt"].(FileMetadata)
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

func newScanRetryTestTask(server *httptest.Server) *JobTask {
	client := server.Client()
	client.Timeout = time.Second
	jt := &JobTask{
		TaskID: 42,
		Job:    map[string]interface{}{},
		AlistClient: &AlistClient{
			URL:    server.URL,
			client: client,
		},
	}
	jt.initRuntime()
	return jt
}
