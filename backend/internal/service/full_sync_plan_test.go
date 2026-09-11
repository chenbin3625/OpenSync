package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"opensync/internal/config"
	"sync"
	"testing"
)

func TestFullSyncPlanMatchesMovedFileByStrongFingerprint(t *testing.T) {
	plan := movedFileFullSyncPlan(FileMetadata{
		Size: 8 << 30,
		MD5:  "0123456789abcdef0123456789abcdef",
	})

	relocations := plan.relocations()
	if len(relocations) != 1 {
		t.Fatalf("relocations len = %d, want 1", len(relocations))
	}
	got := relocations[0]
	if got.source.dir != "/dst/old/archive/" || got.target.dstDir != "/dst/new/archive/" {
		t.Fatalf("relocation = %#v, want destination-side old/archive -> new/archive", got)
	}
	if got.source.name != "movie.mkv" || got.target.name != "movie.mkv" {
		t.Fatalf("relocation names = %q -> %q, want movie.mkv", got.source.name, got.target.name)
	}
}

func TestFullSyncPlanDoesNotInferMoveWithoutMD5(t *testing.T) {
	plan := movedFileFullSyncPlan(FileMetadata{Size: 8 << 30})

	if relocations := plan.relocations(); len(relocations) != 0 {
		t.Fatalf("relocations = %#v, want none when only file size matches", relocations)
	}
}

func TestFullSyncPlanExecutesDestinationMoveBeforeDeletingOldTree(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode %s request: %v", r.URL.Path, err)
		}
		mu.Lock()
		switch r.URL.Path {
		case "/api/fs/list":
			mu.Unlock()
			writeMovedFileListResponse(t, w, body["path"].(string))
			return
		case "/api/fs/mkdir":
			calls = append(calls, "mkdir:"+body["path"].(string))
		case "/api/fs/move":
			names := body["names"].([]interface{})
			calls = append(calls, "move:"+body["src_dir"].(string)+"->"+body["dst_dir"].(string)+names[0].(string))
		case "/api/fs/get":
			calls = append(calls, "get:"+body["path"].(string))
		case "/api/fs/remove":
			names := body["names"].([]interface{})
			calls = append(calls, "remove:"+body["dir"].(string)+names[0].(string))
		default:
			mu.Unlock()
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		mu.Unlock()

		if r.URL.Path == "/api/fs/move" {
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"tasks":[]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{}}`))
	}))
	defer server.Close()

	var persisted []map[string]interface{}
	restorePersist := stubPersistJobTaskItems(t, &persisted, nil)
	defer restorePersist()

	jt := scanTestTask(server.URL, server.Client(), map[string]interface{}{
		"method":        1,
		"srcPath":       "/src/",
		"dstPath":       "/dst/",
		"useCacheS":     0,
		"useCacheT":     0,
		"scanIntervalS": 0,
		"scanIntervalT": 0,
	})

	jt.sync()
	if err := jt.flushPersistBuffer(); err != nil {
		t.Fatalf("flushPersistBuffer() error: %v", err)
	}
	if !jt.ScanFinish.Load() {
		t.Fatal("sync did not mark scanning complete")
	}

	if waiting := jt.Waiting.snapshot(); len(waiting) != 0 {
		t.Fatalf("waiting copies = %#v, want none after successful target-side move", waiting)
	}
	wantCalls := []string{
		"mkdir:/dst/new/",
		"mkdir:/dst/new/archive/",
		"move:/dst/old/archive/->/dst/new/archive/movie.mkv",
		"get:/dst/new/archive/movie.mkv",
		"remove:/dst/old",
	}
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != len(wantCalls) {
		t.Fatalf("calls = %#v, want %#v", calls, wantCalls)
	}
	for i := range wantCalls {
		if calls[i] != wantCalls[i] {
			t.Fatalf("calls[%d] = %q, want %q (all calls: %#v)", i, calls[i], wantCalls[i], calls)
		}
	}

	moveRecords := 0
	for _, item := range persisted {
		if item["type"] == taskItemTypeMove.Int() {
			moveRecords++
			if item["srcPath"] != "/dst/old/archive/" || item["dstPath"] != "/dst/new/archive/" {
				t.Fatalf("persisted move = %#v", item)
			}
		}
	}
	if moveRecords != 1 {
		t.Fatalf("move records = %d, want 1 (all records: %#v)", moveRecords, persisted)
	}
}

func TestFullSyncPlanPreservesOldTreeWhenDestinationMoveFails(t *testing.T) {
	oldConfig := config.GetConfig()
	defer config.SetConfigForTest(oldConfig)
	config.SetConfigForTest(&config.Config{Server: config.ServerConfig{
		CopyConcurrency: 1,
		ScanConcurrency: 1,
		MaxRetries:      0,
	}})

	removeCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/fs/mkdir":
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{}}`))
		case "/api/fs/move":
			_, _ = w.Write([]byte(`{"code":500,"message":"move failed","data":null}`))
		case "/api/fs/remove":
			removeCalled = true
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{}}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	var persisted []map[string]interface{}
	restorePersist := stubPersistJobTaskItems(t, &persisted, nil)
	defer restorePersist()

	jt := scanTestTask(server.URL, server.Client(), map[string]interface{}{
		"method":        1,
		"scanIntervalT": 0,
	})
	plan := movedFileFullSyncPlan(FileMetadata{
		Size: 8 << 30,
		MD5:  "0123456789abcdef0123456789abcdef",
	})

	jt.executeFullSyncPlan(plan)
	if err := jt.flushPersistBuffer(); err != nil {
		t.Fatalf("flushPersistBuffer() error: %v", err)
	}
	if removeCalled {
		t.Fatal("old destination tree was deleted after its relocation failed")
	}
	waiting := jt.Waiting.snapshot()
	if len(waiting) != 1 || waiting[0].SrcPath != "/src/new/archive/" || waiting[0].DstPath != "/dst/new/archive/" {
		t.Fatalf("fallback copy queue = %#v, want source new/archive -> destination new/archive", waiting)
	}
	moveFailures := 0
	for _, item := range persisted {
		if item["type"] == taskItemTypeMove.Int() && item["status"] == taskStatusFailed.Int() {
			moveFailures++
		}
	}
	if moveFailures != 1 {
		t.Fatalf("failed move records = %d, want 1 (all records: %#v)", moveFailures, persisted)
	}
}

func writeMovedFileListResponse(t *testing.T, w http.ResponseWriter, dir string) {
	t.Helper()
	responses := map[string]string{
		"/src/":             `{"code":200,"message":"ok","data":{"content":[{"name":"new","is_dir":true}]}}`,
		"/src/new/":         `{"code":200,"message":"ok","data":{"content":[{"name":"archive","is_dir":true}]}}`,
		"/src/new/archive/": `{"code":200,"message":"ok","data":{"content":[{"name":"movie.mkv","size":8589934592,"hash_info":{"md5":"0123456789abcdef0123456789abcdef"}}]}}`,
		"/dst/":             `{"code":200,"message":"ok","data":{"content":[{"name":"old","is_dir":true}]}}`,
		"/dst/old/":         `{"code":200,"message":"ok","data":{"content":[{"name":"archive","is_dir":true}]}}`,
		"/dst/old/archive/": `{"code":200,"message":"ok","data":{"content":[{"name":"movie.mkv","size":8589934592,"hash_info":{"md5":"0123456789abcdef0123456789abcdef"}}]}}`,
	}
	response, ok := responses[dir]
	if !ok {
		t.Fatalf("unexpected list path %q", dir)
	}
	_, _ = w.Write([]byte(response))
}

func movedFileFullSyncPlan(metadata FileMetadata) *fullSyncPlan {
	src := &fullSyncSnapshot{
		root: "/src/",
		dirs: map[string]FileListResult{
			"":             {"new/": {}},
			"new/":         {"archive/": {}},
			"new/archive/": {"movie.mkv": metadata},
		},
	}
	dst := &fullSyncSnapshot{
		root: "/dst/",
		dirs: map[string]FileListResult{
			"":             {"old/": {}},
			"old/":         {"archive/": {}},
			"old/archive/": {"movie.mkv": metadata},
		},
	}
	plan := newFullSyncPlan(src, dst)
	plan.build(map[string]interface{}{"method": 1})
	return plan
}
