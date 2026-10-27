package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetryDirectoryScanFailureDoesNotBecomeMkdirSuccess(t *testing.T) {
	oldDelay := scanListRetryDelay
	scanListRetryDelay = func(int) time.Duration { return 0 }
	defer func() { scanListRetryDelay = oldDelay }()

	for _, isSrc := range []bool{false, true} {
		name := "destination"
		if isSrc {
			name = "source"
		}
		t.Run(name, func(t *testing.T) {
			var persisted []map[string]interface{}
			restorePersist := stubPersistJobTaskItems(t, &persisted, nil)
			defer restorePersist()

			var mkdirCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/api/fs/mkdir" {
					mkdirCalls.Add(1)
					_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{}}`))
					return
				}
				_, _ = w.Write([]byte(`{"code":403,"message":"password is incorrect or you have no permission","data":null}`))
			}))
			defer server.Close()

			jt := scanTestTask(server.URL, server.Client(), map[string]interface{}{})
			if _, err := jt.listDir("/protected/", true, nil, "/protected/", isSrc); err == nil {
				t.Fatal("listDir() succeeded, want permission failure")
			}
			if err := jt.flushPersistBuffer(); err != nil {
				t.Fatal(err)
			}
			if len(persisted) != 1 {
				t.Fatalf("scan failures = %d, want 1", len(persisted))
			}
			item := persisted[0]
			originalError := *item["errMsg"].(*string)
			// The mapper returns persisted error messages as strings.
			item["errMsg"] = originalError
			persisted = nil

			retry := scanTestTask(server.URL, server.Client(), map[string]interface{}{})
			retry.retryTaskItem(item)
			if err := retry.flushPersistBuffer(); err != nil {
				t.Fatal(err)
			}
			if mkdirCalls.Load() != 0 {
				t.Fatalf("mkdir calls = %d, want no mkdir for a failed directory scan", mkdirCalls.Load())
			}
			if len(persisted) != 1 || persisted[0]["status"] != taskStatusFailed.Int() {
				t.Fatalf("retry result = %#v, want one failed item", persisted)
			}
			errMsg := *persisted[0]["errMsg"].(*string)
			if !strings.Contains(errMsg, originalError) || !strings.Contains(errMsg, "重新执行完整任务") {
				t.Fatalf("retry error = %q, want original error and full-task recovery guidance", errMsg)
			}
			if len(retry.Waiting.snapshot()) != 0 {
				t.Fatal("failed directory scan queued file operations")
			}
		})
	}
}

func TestRetryMkdirWithBothPathsStillRuns(t *testing.T) {
	var persisted []map[string]interface{}
	restorePersist := stubPersistJobTaskItems(t, &persisted, nil)
	defer restorePersist()

	var mkdirCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/fs/mkdir" {
			t.Errorf("unexpected request %s", r.URL.Path)
		}
		mkdirCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{}}`))
	}))
	defer server.Close()

	jt := scanTestTask(server.URL, server.Client(), map[string]interface{}{})
	jt.retryTaskItem(map[string]interface{}{
		"type": taskItemTypeCopy.Int(), "isPath": taskItemPath.Int(),
		"srcPath": "/src/", "dstPath": "/dst/",
	})
	if err := jt.flushPersistBuffer(); err != nil {
		t.Fatal(err)
	}
	if mkdirCalls.Load() != 1 || len(persisted) != 1 || persisted[0]["status"] != taskStatusSuccess.Int() {
		t.Fatalf("mkdir calls = %d, results = %#v, want existing mkdir retry behavior", mkdirCalls.Load(), persisted)
	}
}
