package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"opensync/internal/config"
	"opensync/internal/msg"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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
		if r.URL.Path == "/api/fs/get" {
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"name":"movie.mkv","size":8589934592,"is_dir":false,"hash_info":{"md5":"0123456789abcdef0123456789abcdef"}}}`))
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

// A source file excluded by the size filter must not be deleted from the
// destination: the filter decides what gets copied, never what gets deleted.
func TestFullSyncPlanKeepsSizeFilteredDestinationFile(t *testing.T) {
	small := FileMetadata{Size: 500 * 1024}
	src := &fullSyncSnapshot{
		root: "/src/",
		dirs: map[string]FileListResult{"": {"small.txt": small}},
	}
	dst := &fullSyncSnapshot{
		root: "/dst/",
		dirs: map[string]FileListResult{"": {"small.txt": small}},
	}
	plan := newFullSyncPlan(src, dst)
	plan.build(map[string]interface{}{
		"method":      1,
		"minFileSize": int64(1024 * 1024),
	})

	if len(plan.extraDeletes) != 0 {
		t.Fatalf("extraDeletes = %#v, want none: the file still exists at the source", plan.extraDeletes)
	}
	if len(plan.extraFiles) != 0 {
		t.Fatalf("extraFiles = %#v, want none", plan.extraFiles)
	}
	if len(plan.newFiles) != 0 || len(plan.changed) != 0 {
		t.Fatalf("newFiles=%#v changed=%#v, want none: the file is filtered out of copying", plan.newFiles, plan.changed)
	}
}

// A destination file with no source counterpart is still deleted when a size
// filter is configured, so the filter fix does not disable mirror deletes.
func TestFullSyncPlanStillDeletesUnmatchedDestinationFileWithSizeFilter(t *testing.T) {
	src := &fullSyncSnapshot{
		root: "/src/",
		dirs: map[string]FileListResult{"": {}},
	}
	dst := &fullSyncSnapshot{
		root: "/dst/",
		dirs: map[string]FileListResult{"": {"orphan.txt": {Size: 500 * 1024}}},
	}
	plan := newFullSyncPlan(src, dst)
	plan.build(map[string]interface{}{
		"method":      1,
		"minFileSize": int64(1024 * 1024),
	})

	if len(plan.extraDeletes) != 1 {
		t.Fatalf("extraDeletes = %#v, want the orphaned destination file", plan.extraDeletes)
	}
}

// Issue 5: blocker lookup must use exact match and skip already-matched destination entries.
func TestFullSyncPlanBlockerLookupUsesExactMatchAndSkipsMatchedEntries(t *testing.T) {
	meta := FileMetadata{Size: 42}
	src := &fullSyncSnapshot{
		root: "/src/",
		dirs: map[string]FileListResult{
			"":     {"Q%26A": meta, "Q&A/": {}},
			"Q&A/": {"inside.txt": meta},
		},
	}
	dst := &fullSyncSnapshot{
		root: "/dst/",
		dirs: map[string]FileListResult{
			"":     {"Q&amp;A": meta, "Q&A/": {}},
			"Q&A/": {"inside.txt": meta},
		},
	}
	plan := newFullSyncPlan(src, dst)
	plan.build(map[string]interface{}{"method": 1})

	if len(plan.blockers) != 0 {
		t.Fatalf("blockers = %#v, want 0 (matched fuzzy file and dir must not collide)", plan.blockers)
	}
	if len(plan.extraDeletes) != 0 {
		t.Fatalf("extraDeletes = %#v, want 0", plan.extraDeletes)
	}
	if len(plan.newFiles) != 0 || len(plan.changed) != 0 {
		t.Fatalf("newFiles=%#v changed=%#v, want none", plan.newFiles, plan.changed)
	}
}

// Issue 6: unmatched destination directories with excluded descendants must NOT be deleted wholesale.
func TestFullSyncPlanProtectsExcludedDescendantsInUnmatchedDirectory(t *testing.T) {
	src := &fullSyncSnapshot{
		root: "/src/",
		dirs: map[string]FileListResult{"": {}},
	}
	dst := &fullSyncSnapshot{
		root: "/dst/",
		dirs: map[string]FileListResult{
			"":              {"orphan/": {}, "clean/": {}},
			"orphan/":       {"drop.txt": {Size: 10}, "sub/": {}, "empty/": {}},
			"orphan/sub/":   {"drop2.txt": {Size: 20}},
			"orphan/empty/": {"bye.txt": {Size: 5}},
			"clean/":        {"gone.txt": {Size: 15}},
		},
		excludedDirs: map[string]bool{
			"orphan/sub/": true, // excluded file (e.g. keep.bak) lived in orphan/sub/
		},
	}

	plan := newFullSyncPlan(src, dst)
	plan.build(map[string]interface{}{"method": 1})

	got := make(map[string]bool)
	for _, del := range plan.extraDeletes {
		got[del.object] = true
	}

	// orphan/ and orphan/sub/ must NOT be deleted because orphan/sub/ has an excluded entry.
	if got["/dst/orphan"] || got["/dst/orphan/sub"] {
		t.Fatalf("protected parent dir was slated for deletion: %#v", plan.extraDeletes)
	}
	// Non-excluded siblings inside orphan/ and clean/ SHOULD be deleted.
	for _, want := range []string{"/dst/orphan/drop.txt", "/dst/orphan/sub/drop2.txt", "/dst/orphan/empty", "/dst/clean"} {
		if !got[want] {
			t.Fatalf("missing expected non-excluded delete %q in %#v", want, plan.extraDeletes)
		}
	}
}

// Issue 7: Full Sync defers extraDeletes until AFTER queued copies succeed, and skips them on copy failure.
func TestFullSyncExecutesExtraDeletesAfterQueuedCopiesComplete(t *testing.T) {
	testDB := newServiceTaskStatusTestDB(t)
	oldDB := mapperDBForServiceTest(testDB)
	defer oldDB()

	oldConfig := config.GetConfig()
	defer config.SetConfigForTest(oldConfig)
	config.SetConfigForTest(&config.Config{Server: config.ServerConfig{
		CopyConcurrency: 1,
		ScanConcurrency: 1,
		MaxRetries:      0,
	}})

	var mu sync.Mutex
	var events []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch r.URL.Path {
		case "/api/fs/list":
			path := body["path"].(string)
			if path == "/src/" {
				_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[{"name":"new.txt","size":10,"is_dir":false}]}}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[{"name":"old.txt","size":5,"is_dir":false}]}}`))
		case "/api/fs/copy":
			time.Sleep(40 * time.Millisecond)
			mu.Lock()
			events = append(events, "copy")
			mu.Unlock()
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"tasks":[]}}`))
		case "/api/fs/get":
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"name":"new.txt","size":10,"is_dir":false}}`))
		case "/api/fs/remove":
			mu.Lock()
			events = append(events, "remove")
			mu.Unlock()
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{}}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := &JobClient{JobID: 1, Job: map[string]interface{}{"id": int64(1), "enable": 1, "isCron": 2}}
	client.tryMarkDoing()
	jt := scanTestTask(server.URL, server.Client(), map[string]interface{}{
		"method":        1,
		"srcPath":       "/src/",
		"dstPath":       "/dst/",
		"useCacheS":     0,
		"useCacheT":     0,
		"scanIntervalS": 0,
		"scanIntervalT": 0,
	})
	jt.TaskID = 10
	jt.JobClient = client
	client.setCurrentTask(jt)

	jt.Start()
	if !client.waitUntilIdle(5 * time.Second) {
		t.Fatal("task did not finish")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 2 || events[0] != "copy" || events[1] != "remove" {
		t.Fatalf("event order = %#v, want [copy remove]", events)
	}
}

func TestFullSyncSkipsDeferredDeletesWhenQueuedCopyFails(t *testing.T) {
	testDB := newServiceTaskStatusTestDB(t)
	oldDB := mapperDBForServiceTest(testDB)
	defer oldDB()

	oldConfig := config.GetConfig()
	defer config.SetConfigForTest(oldConfig)
	config.SetConfigForTest(&config.Config{Server: config.ServerConfig{
		CopyConcurrency: 1,
		ScanConcurrency: 1,
		MaxRetries:      0,
	}})

	var removeCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch r.URL.Path {
		case "/api/fs/list":
			path := body["path"].(string)
			if path == "/src/" {
				_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[{"name":"new.txt","size":10,"is_dir":false}]}}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[{"name":"old.txt","size":5,"is_dir":false}]}}`))
		case "/api/fs/copy":
			_, _ = w.Write([]byte(`{"code":500,"message":"copy failed","data":null}`))
		case "/api/fs/remove":
			removeCalls.Add(1)
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{}}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := &JobClient{JobID: 1, Job: map[string]interface{}{"id": int64(1), "enable": 1, "isCron": 2}}
	client.tryMarkDoing()
	jt := scanTestTask(server.URL, server.Client(), map[string]interface{}{
		"method":        1,
		"srcPath":       "/src/",
		"dstPath":       "/dst/",
		"useCacheS":     0,
		"useCacheT":     0,
		"scanIntervalS": 0,
		"scanIntervalT": 0,
	})
	jt.TaskID = 10
	jt.JobClient = client
	client.setCurrentTask(jt)

	jt.Start()
	if !client.waitUntilIdle(5 * time.Second) {
		t.Fatal("task did not finish")
	}
	if got := removeCalls.Load(); got != 0 {
		t.Fatalf("removeCalls = %d, want 0 when queued copy failed", got)
	}
}

// Issue 8: missing destination root on first run must not retry 3x or emit failed job_task_item.
func TestFullSyncMissingDestinationRootDoesNotRetryOrRecordFailure(t *testing.T) {
	oldDelay := scanListRetryDelay
	scanListRetryDelay = func(int) time.Duration { return 0 }
	defer func() { scanListRetryDelay = oldDelay }()

	var dstListBeforeMkdir atomic.Int32
	var dstListCalls atomic.Int32
	var mkdirCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch r.URL.Path {
		case "/api/fs/list":
			path := body["path"].(string)
			if path == "/src/" {
				_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[{"name":"a.txt","size":10,"is_dir":false}]}}`))
				return
			}
			if mkdirCalls.Load() == 0 {
				dstListBeforeMkdir.Add(1)
			}
			if dstListCalls.Add(1) == 1 {
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

	if got := dstListBeforeMkdir.Load(); got != 1 {
		t.Fatalf("dst root list calls before mkdir = %d, want 1 (no 3x retry on missing root)", got)
	}
	if got := dstListCalls.Load(); got != 2 {
		t.Fatalf("dst root total list calls = %d, want 2 (1 initial miss + 1 post-mkdir rescan)", got)
	}
	if got := mkdirCalls.Load(); got != 1 {
		t.Fatalf("mkdir calls = %d, want 1", got)
	}
	for _, item := range persisted {
		if item["status"] == taskStatusFailed.Int() {
			t.Fatalf("unexpected failed task item on missing dst root: %#v", item)
		}
	}
	if waiting := jt.Waiting.snapshot(); len(waiting) != 1 || waiting[0].FileName != "a.txt" {
		t.Fatalf("waiting copies = %#v, want [a.txt]", waiting)
	}
}

// Issue 8: deep subdirectory 404 in Full Sync must NOT trigger root Mkdir/rescan and must record scan failure without mutating dst.
func TestFullSyncDeepSubdirNotFoundDoesNotTriggerRootMkdir(t *testing.T) {
	oldDelay := scanListRetryDelay
	scanListRetryDelay = func(int) time.Duration { return 0 }
	defer func() { scanListRetryDelay = oldDelay }()

	var mkdirCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch r.URL.Path {
		case "/api/fs/list":
			path := body["path"].(string)
			switch {
			case path == "/src/":
				_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[]}}`))
			case path == "/dst/":
				_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[{"name":"sub","is_dir":true}]}}`))
			case strings.HasPrefix(path, "/dst/sub"):
				_, _ = w.Write([]byte(`{"code":500,"message":"failed get dir: object not found","data":null}`))
			default:
				_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[]}}`))
			}
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

	if got := mkdirCalls.Load(); got != 0 {
		t.Fatalf("mkdirCalls = %d, want 0 for deep subdir 404", got)
	}
	if waiting := jt.Waiting.snapshot(); len(waiting) != 0 {
		t.Fatalf("waiting copies = %#v, want 0 after deep subdir scan failure", waiting)
	}
	foundSubdirFail := false
	for _, item := range persisted {
		if item["status"] == taskStatusFailed.Int() && item["dstPath"] == "/dst/sub/" {
			foundSubdirFail = true
			break
		}
	}
	if !foundSubdirFail {
		t.Fatalf("expected failed scan item for /dst/sub/, got %#v", persisted)
	}
}

func TestFullSyncAbortsWithoutDeletingWhenSourceListReturnsNullOrMissingData(t *testing.T) {
	oldDelay := scanListRetryDelay
	scanListRetryDelay = func(int) time.Duration { return 0 }
	defer func() { scanListRetryDelay = oldDelay }()

	cases := []struct {
		name string
		body string
	}{
		{name: "null data", body: `{"code":200,"message":"ok","data":null}`},
		{name: "missing data", body: `{"code":200,"message":"ok"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var removeCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				var body map[string]interface{}
				_ = json.NewDecoder(r.Body).Decode(&body)
				switch r.URL.Path {
				case "/api/fs/list":
					if body["path"].(string) == "/src/" {
						_, _ = w.Write([]byte(tc.body))
						return
					}
					_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[{"name":"important.txt","size":100,"is_dir":false}]}}`))
				case "/api/fs/remove":
					removeCalls.Add(1)
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
			if got := removeCalls.Load(); got != 0 {
				t.Fatalf("removeCalls = %d, want 0 when source list returned %s", got, tc.name)
			}
			if len(jt.deferredDeletes) != 0 {
				t.Fatalf("deferredDeletes = %#v, want empty", jt.deferredDeletes)
			}
			foundScanFailure := false
			for _, item := range persisted {
				if item["status"] == taskStatusFailed.Int() && item["srcPath"] == "/src/" {
					foundScanFailure = true
					break
				}
			}
			if !foundScanFailure {
				t.Fatalf("expected failed source scan record, got %#v", persisted)
			}
		})
	}
}

func TestFullSyncSkipsDeferredDeletesWhenRemoteCopyCanceled(t *testing.T) {
	testDB := newServiceTaskStatusTestDB(t)
	oldDB := mapperDBForServiceTest(testDB)
	defer oldDB()

	oldConfig := config.GetConfig()
	defer config.SetConfigForTest(oldConfig)
	config.SetConfigForTest(&config.Config{Server: config.ServerConfig{
		CopyConcurrency: 1,
		ScanConcurrency: 1,
		MaxRetries:      0,
	}})

	var removeCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch r.URL.Path {
		case "/api/fs/list":
			if body["path"].(string) == "/src/" {
				_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[{"name":"new.txt","size":10,"is_dir":false}]}}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[{"name":"old.txt","size":5,"is_dir":false}]}}`))
		case "/api/fs/copy":
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"tasks":[{"id":"remote-cancel-1"}]}}`))
		case "/api/admin/task/copy/undone":
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":[]}`))
		case "/api/admin/task/copy/info":
			// AList tache.State 4 = canceled -> taskStatusStopped.
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"id":"remote-cancel-1","state":4,"progress":20,"error":"canceled in alist"}}`))
		case "/api/admin/task/copy/delete":
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{}}`))
		case "/api/fs/remove":
			removeCalls.Add(1)
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{}}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := &JobClient{JobID: 1, Job: map[string]interface{}{"id": int64(1), "enable": 1, "isCron": 2}}
	client.tryMarkDoing()
	jt := scanTestTask(server.URL, server.Client(), map[string]interface{}{
		"method":        1,
		"srcPath":       "/src/",
		"dstPath":       "/dst/",
		"useCacheS":     0,
		"useCacheT":     0,
		"scanIntervalS": 0,
		"scanIntervalT": 0,
	})
	jt.TaskID = 10
	jt.JobClient = client
	jt.LastWatching.Store(time.Now().Unix())
	client.setCurrentTask(jt)

	jt.Start()
	if !client.waitUntilIdle(5 * time.Second) {
		t.Fatal("task did not finish")
	}
	if got := removeCalls.Load(); got != 0 {
		t.Fatalf("removeCalls = %d, want 0 when remote AList copy was canceled", got)
	}
}

func TestFullSyncSkipsDeferredDeletesWhenNewDirMkdirFails(t *testing.T) {
	testDB := newServiceTaskStatusTestDB(t)
	oldDB := mapperDBForServiceTest(testDB)
	defer oldDB()

	oldConfig := config.GetConfig()
	defer config.SetConfigForTest(oldConfig)
	config.SetConfigForTest(&config.Config{Server: config.ServerConfig{
		CopyConcurrency: 1,
		ScanConcurrency: 1,
		MaxRetries:      0,
	}})

	var removeCalls atomic.Int32
	var copyCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch r.URL.Path {
		case "/api/fs/list":
			switch body["path"].(string) {
			case "/src/":
				_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[{"name":"newdir","is_dir":true}]}}`))
			case "/src/newdir/":
				_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[{"name":"moved.txt","size":10,"is_dir":false}]}}`))
			case "/dst/":
				_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[{"name":"old.txt","size":10,"is_dir":false}]}}`))
			default:
				t.Fatalf("unexpected list path %v", body["path"])
			}
		case "/api/fs/mkdir":
			_, _ = w.Write([]byte(`{"code":500,"message":"permission denied","data":null}`))
		case "/api/fs/copy":
			copyCalls.Add(1)
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"tasks":[]}}`))
		case "/api/fs/remove":
			removeCalls.Add(1)
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{}}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := &JobClient{JobID: 1, Job: map[string]interface{}{"id": int64(1), "enable": 1, "isCron": 2}}
	client.tryMarkDoing()
	jt := scanTestTask(server.URL, server.Client(), map[string]interface{}{
		"method":        1,
		"srcPath":       "/src/",
		"dstPath":       "/dst/",
		"useCacheS":     0,
		"useCacheT":     0,
		"scanIntervalS": 0,
		"scanIntervalT": 0,
	})
	jt.TaskID = 10
	jt.JobClient = client
	client.setCurrentTask(jt)

	jt.Start()
	if !client.waitUntilIdle(5 * time.Second) {
		t.Fatal("task did not finish")
	}
	if got := copyCalls.Load(); got != 0 {
		t.Fatalf("copyCalls = %d, want 0 when newdir mkdir failed", got)
	}
	if got := removeCalls.Load(); got != 0 {
		t.Fatalf("removeCalls = %d, want 0 when newdir mkdir failed", got)
	}
}

func TestFullSyncFileDirConflictProtectsExcludedDescendants(t *testing.T) {
	for _, nested := range []bool{false, true} {
		name := "direct excluded child"
		if nested {
			name = "nested excluded descendant"
		}
		t.Run(name, func(t *testing.T) {
			var removeCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				var body map[string]interface{}
				_ = json.NewDecoder(r.Body).Decode(&body)
				switch r.URL.Path {
				case "/api/fs/list":
					switch body["path"].(string) {
					case "/src/":
						_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[{"name":"report","size":12,"is_dir":false}]}}`))
					case "/dst/":
						_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[{"name":"report","is_dir":true}]}}`))
					case "/dst/report/":
						if nested {
							_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[{"name":"sub","is_dir":true}]}}`))
						} else {
							_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[{"name":"keep.bak","size":5,"is_dir":false}]}}`))
						}
					case "/dst/report/sub/":
						_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[{"name":"keep.bak","size":5,"is_dir":false}]}}`))
					default:
						t.Fatalf("unexpected list path %v", body["path"])
					}
				case "/api/fs/remove":
					removeCalls.Add(1)
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
				"srcPath":       "/src/",
				"dstPath":       "/dst/",
				"exclude":       "*.bak",
				"useCacheS":     0,
				"useCacheT":     0,
				"scanIntervalS": 0,
				"scanIntervalT": 0,
			})

			jt.sync()
			if err := jt.flushPersistBuffer(); err != nil {
				t.Fatalf("flushPersistBuffer() error: %v", err)
			}
			if got := removeCalls.Load(); got != 0 {
				t.Fatalf("removeCalls = %d, want 0 when conflicting dir contains excluded descendant", got)
			}
			if waiting := jt.Waiting.snapshot(); len(waiting) != 0 {
				t.Fatalf("waiting copies = %#v, want 0 because conflicting dir was protected", waiting)
			}
			foundProtectedErr := false
			for _, item := range persisted {
				if item["type"] == taskItemTypeDelete.Int() && item["status"] == taskStatusFailed.Int() {
					if errPtr, ok := item["errMsg"].(*string); ok && errPtr != nil && *errPtr == msg.DirDeleteProtectedByExclude {
						foundProtectedErr = true
					}
				}
			}
			if !foundProtectedErr {
				t.Fatalf("expected protected delete failure %q, got %#v", msg.DirDeleteProtectedByExclude, persisted)
			}
		})
	}
}
