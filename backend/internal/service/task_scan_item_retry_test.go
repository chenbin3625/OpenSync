package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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
	var listCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/fs/mkdir":
			mkdirCalls.Add(1)
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{}}`))
		case "/api/fs/list":
			listCalls.Add(1)
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[],"total":0}}`))
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			_, _ = w.Write([]byte(`{"code":500,"message":"unexpected"}`))
		}
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
	if mkdirCalls.Load() != 1 || listCalls.Load() < 1 || len(persisted) != 1 || persisted[0]["status"] != taskStatusSuccess.Int() {
		t.Fatalf("mkdir calls = %d, list calls = %d, results = %#v, want mkdir retry and subtree scan", mkdirCalls.Load(), listCalls.Load(), persisted)
	}
}

func TestRetryTaskItemDoesNotRecordFailureAfterDeadline(t *testing.T) {
	var persisted []map[string]interface{}
	restorePersist := stubPersistJobTaskItems(t, &persisted, nil)
	defer restorePersist()

	jt := &JobTask{Job: map[string]interface{}{}}
	jt.BreakFlag.Store(true)
	jt.retryTaskItem(map[string]interface{}{
		"type": taskItemTypeCopy.Int(), "isPath": taskItemPath.Int(),
		"srcPath": "/src/", "dstPath": "",
	})
	jt.retryTaskItem(map[string]interface{}{
		"type": 99, "isPath": taskItemFile.Int(),
		"srcPath": "/src/", "dstPath": "/dst/", "fileName": "file.txt",
	})

	if err := jt.flushPersistBuffer(); err != nil {
		t.Fatalf("flushPersistBuffer() error: %v", err)
	}
	if len(persisted) != 0 {
		t.Fatalf("persisted failures = %#v, want none after deadline", persisted)
	}
}

func TestRetryFailedMkdirRebuildsMissingSubtreeAndRespectsExclude(t *testing.T) {
	var persisted []map[string]interface{}
	restorePersist := stubPersistJobTaskItems(t, &persisted, nil)
	defer restorePersist()

	var failMkdir atomic.Bool
	failMkdir.Store(true)
	var mu sync.Mutex
	var mkdirPaths []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch r.URL.Path {
		case "/api/fs/mkdir":
			p := body["path"].(string)
			if failMkdir.Load() {
				_, _ = w.Write([]byte(`{"code":500,"message":"mkdir failed","data":null}`))
				return
			}
			mu.Lock()
			mkdirPaths = append(mkdirPaths, p)
			mu.Unlock()
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{}}`))
		case "/api/fs/list":
			switch body["path"].(string) {
			case "/src/sub/":
				_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[{"name":"keep.txt","size":10,"is_dir":false},{"name":"skip.tmp","size":5,"is_dir":false},{"name":"deep","is_dir":true}]}}`))
			case "/src/sub/deep/":
				_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[{"name":"nested.txt","size":20,"is_dir":false}]}}`))
			case "/dst/sub/":
				_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[]}}`))
			default:
				t.Fatalf("unexpected list path %v", body["path"])
			}
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	jobCfg := map[string]interface{}{
		"method":  0,
		"srcPath": `["/src"]`,
		"dstPath": `["/dst"]`,
		"exclude": "*.tmp",
	}

	// Step 1: initial incremental sync fails to create /dst/sub/ before scanning its children.
	initial := scanTestTask(server.URL, server.Client(), jobCfg)
	initial.syncWithoutHave(scanWork{
		SrcPath:     "/src/sub/",
		DstPath:     "/dst/sub/",
		SrcRootPath: "/src/",
		DstRootPath: "/dst/",
		FirstDst:    true,
		Mode:        scanWorkMissingDst,
	}, compileJobExcludeSpec(jobCfg))
	if err := initial.flushPersistBuffer(); err != nil {
		t.Fatal(err)
	}
	if len(persisted) != 1 || persisted[0]["status"] != taskStatusFailed.Int() {
		t.Fatalf("initial persisted = %#v, want 1 failed mkdir item", persisted)
	}
	if len(initial.Waiting.snapshot()) != 0 {
		t.Fatalf("initial queued files = %d, want 0 after mkdir failure", len(initial.Waiting.snapshot()))
	}

	// Step 2: retry the failed mkdir item once AList recovers.
	failedItem := persisted[0]
	persisted = nil
	failMkdir.Store(false)

	retry := scanTestTask(server.URL, server.Client(), jobCfg)
	retry.retryTaskItem(failedItem)
	if err := retry.flushPersistBuffer(); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	gotMkdirs := append([]string(nil), mkdirPaths...)
	mu.Unlock()
	if len(gotMkdirs) != 2 || gotMkdirs[0] != "/dst/sub/" || gotMkdirs[1] != "/dst/sub/deep/" {
		t.Fatalf("mkdirPaths = %v, want [/dst/sub/ /dst/sub/deep/]", gotMkdirs)
	}

	queued := make(map[string]bool)
	for _, item := range retry.Waiting.snapshot() {
		queued[item.DstPath+item.FileName] = true
	}
	if !queued["/dst/sub/keep.txt"] || !queued["/dst/sub/deep/nested.txt"] {
		t.Fatalf("queued copies = %#v, want both keep.txt and deep/nested.txt", queued)
	}
	if queued["/dst/sub/skip.tmp"] {
		t.Fatalf("excluded file skip.tmp was queued: %#v", queued)
	}
}
