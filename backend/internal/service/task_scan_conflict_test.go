package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFullSyncDeletesConflictingDestinationDirectoryBeforeQueueingFile(t *testing.T) {
	var persisted []map[string]interface{}
	restorePersist := stubPersistJobTaskItems(t, &persisted, nil)
	defer restorePersist()

	var removeCalls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/fs/list":
			var req struct {
				Path string `json:"path"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode list request: %v", err)
			}
			switch req.Path {
			case "/src/":
				_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[{"name":"foo","is_dir":false,"size":10}]}}`))
			case "/dst/":
				_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[{"name":"foo","is_dir":true,"size":0}]}}`))
			case "/dst/foo/":
				// Full sync walks the destination tree, so the conflicting
				// directory is listed too. It is empty.
				_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[]}}`))
			default:
				t.Fatalf("unexpected list path %q", req.Path)
			}
		case "/api/fs/remove":
			var req struct {
				Dir   string   `json:"dir"`
				Names []string `json:"names"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode remove request: %v", err)
			}
			if len(req.Names) != 1 {
				t.Fatalf("remove names = %#v, want one name", req.Names)
			}
			removeCalls = append(removeCalls, req.Dir+req.Names[0]+"/")
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{}}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	// Mirror (method=1) jobs run through syncFull, so drive the job entry point
	// rather than the incremental scanner: the dir-vs-file conflict is resolved
	// by the full-sync plan's blockers, which run before anything is queued.
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

	if len(removeCalls) != 1 || removeCalls[0] != "/dst/foo/" {
		t.Fatalf("removeCalls = %#v, want /dst/foo/", removeCalls)
	}
	if err := jt.flushPersistBuffer(); err != nil {
		t.Fatalf("flushPersistBuffer() error: %v", err)
	}
	if len(persisted) != 1 {
		t.Fatalf("persisted len = %d, want one delete record", len(persisted))
	}
	if persisted[0]["type"] != taskItemTypeDelete.Int() || persisted[0]["isPath"] != taskItemPath.Int() {
		t.Fatalf("persisted delete item = %#v, want directory delete", persisted[0])
	}
	waiting := jt.Waiting.snapshot()
	if len(waiting) != 1 {
		t.Fatalf("waiting len = %d, want one queued copy", len(waiting))
	}
	if waiting[0].FileName != "foo" {
		t.Fatalf("queued file = %q, want foo", waiting[0].FileName)
	}
}

func TestFullSyncSkipsEquivalentEscapedDestinationFileNameWithSameSize(t *testing.T) {
	var persisted []map[string]interface{}
	restorePersist := stubPersistJobTaskItems(t, &persisted, nil)
	defer restorePersist()

	var removeCalls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/fs/list":
			var req struct {
				Path string `json:"path"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode list request: %v", err)
			}
			switch req.Path {
			case "/src/":
				_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[{"name":"Q&A.mp4","is_dir":false,"size":10}]}}`))
			case "/dst/":
				_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[{"name":"Q&amp;A.mp4","is_dir":false,"size":10}]}}`))
			default:
				t.Fatalf("unexpected list path %q", req.Path)
			}
		case "/api/fs/remove":
			var req struct {
				Dir   string   `json:"dir"`
				Names []string `json:"names"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode remove request: %v", err)
			}
			removeCalls = append(removeCalls, req.Dir+req.Names[0])
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{}}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	jt := scanTestTask(server.URL, server.Client(), map[string]interface{}{"method": 1})
	jt.syncWithHave(scanWork{
		SrcPath:     "/src/",
		DstPath:     "/dst/",
		SrcRootPath: "/src/",
		DstRootPath: "/dst/",
		FirstDst:    true,
		Mode:        scanWorkCompare,
	}, nil)

	if len(removeCalls) != 0 {
		t.Fatalf("removeCalls = %#v, want none", removeCalls)
	}
	if len(persisted) != 0 {
		t.Fatalf("persisted len = %d, want no delete records", len(persisted))
	}
	if waiting := jt.Waiting.snapshot(); len(waiting) != 0 {
		t.Fatalf("waiting len = %d, want no queued copy", len(waiting))
	}
}

func TestFullSyncRecopiesEquivalentEscapedDestinationFileNameWhenSizeDiffers(t *testing.T) {
	var persisted []map[string]interface{}
	restorePersist := stubPersistJobTaskItems(t, &persisted, nil)
	defer restorePersist()

	var removeCalls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/fs/list":
			var req struct {
				Path string `json:"path"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode list request: %v", err)
			}
			switch req.Path {
			case "/src/":
				_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[{"name":"Q&A.mp4","is_dir":false,"size":11}]}}`))
			case "/dst/":
				_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[{"name":"Q&amp;A.mp4","is_dir":false,"size":10}]}}`))
			default:
				t.Fatalf("unexpected list path %q", req.Path)
			}
		case "/api/fs/remove":
			var req struct {
				Dir   string   `json:"dir"`
				Names []string `json:"names"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode remove request: %v", err)
			}
			removeCalls = append(removeCalls, req.Dir+req.Names[0])
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{}}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	jt := scanTestTask(server.URL, server.Client(), map[string]interface{}{"method": 1})
	jt.syncWithHave(scanWork{
		SrcPath:     "/src/",
		DstPath:     "/dst/",
		SrcRootPath: "/src/",
		DstRootPath: "/dst/",
		FirstDst:    true,
		Mode:        scanWorkCompare,
	}, nil)

	if len(removeCalls) != 0 {
		t.Fatalf("removeCalls = %#v, want none", removeCalls)
	}
	if len(persisted) != 0 {
		t.Fatalf("persisted len = %d, want no delete records", len(persisted))
	}
	waiting := jt.Waiting.snapshot()
	if len(waiting) != 1 {
		t.Fatalf("waiting len = %d, want one queued copy", len(waiting))
	}
	if waiting[0].FileName != "Q&A.mp4" {
		t.Fatalf("queued file = %q, want Q&A.mp4", waiting[0].FileName)
	}
}

func TestFullSyncDoesNotFuzzyMatchWhenSourceHasCanonicalNameCollision(t *testing.T) {
	var persisted []map[string]interface{}
	restorePersist := stubPersistJobTaskItems(t, &persisted, nil)
	defer restorePersist()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/fs/list":
			var req struct {
				Path string `json:"path"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode list request: %v", err)
			}
			switch req.Path {
			case "/src/":
				_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[{"name":"Q&A.mp4","is_dir":false,"size":10},{"name":"Q&amp;A.mp4","is_dir":false,"size":10}]}}`))
			case "/dst/":
				_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[{"name":"Q&amp;A.mp4","is_dir":false,"size":10}]}}`))
			default:
				t.Fatalf("unexpected list path %q", req.Path)
			}
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	jt := scanTestTask(server.URL, server.Client(), map[string]interface{}{"method": 1})
	jt.syncWithHave(scanWork{
		SrcPath:     "/src/",
		DstPath:     "/dst/",
		SrcRootPath: "/src/",
		DstRootPath: "/dst/",
		FirstDst:    true,
		Mode:        scanWorkCompare,
	}, nil)

	if len(persisted) != 0 {
		t.Fatalf("persisted len = %d, want no delete records", len(persisted))
	}
	waiting := jt.Waiting.snapshot()
	if len(waiting) != 1 {
		t.Fatalf("waiting len = %d, want one queued copy", len(waiting))
	}
	if waiting[0].FileName != "Q&A.mp4" {
		t.Fatalf("queued file = %q, want Q&A.mp4", waiting[0].FileName)
	}
}

func scanTestTask(serverURL string, client *http.Client, job map[string]interface{}) *JobTask {
	jt := &JobTask{
		TaskID:  42,
		Job:     job,
		Waiting: newCopyQueue(),
		AlistClient: &AlistClient{
			URL:    serverURL,
			client: client,
		},
	}
	jt.initRuntime()
	return jt
}

// A destination root that does not exist yet must be created rather than
// failing the whole job with "destination scan failed".
func TestSyncCreatesMissingDestinationRoot(t *testing.T) {
	oldDelay := scanListRetryDelay
	scanListRetryDelay = func(int) time.Duration { return 0 }
	defer func() { scanListRetryDelay = oldDelay }()

	var persisted []map[string]interface{}
	restorePersist := stubPersistJobTaskItems(t, &persisted, nil)
	defer restorePersist()

	var mkdirCalls []string
	dstCreated := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var req struct {
			Path string `json:"path"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		switch r.URL.Path {
		case "/api/fs/list":
			switch req.Path {
			case "/src/":
				_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[{"name":"a.txt","is_dir":false,"size":10}]}}`))
			case "/dst/":
				if !dstCreated {
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"code":500,"message":"failed get objs: failed get dir: object not found","data":null}`))
					return
				}
				_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[]}}`))
			default:
				t.Errorf("unexpected list path %q", req.Path)
			}
		case "/api/fs/mkdir":
			mkdirCalls = append(mkdirCalls, req.Path)
			dstCreated = true
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{}}`))
		case "/api/fs/copy":
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"tasks":[]}}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	jt := scanTestTask(server.URL, server.Client(), map[string]interface{}{
		"method":        0,
		"srcPath":       "/src/",
		"dstPath":       "/dst/",
		"scanIntervalS": 0,
		"scanIntervalT": 0,
	})

	jt.sync()

	if len(mkdirCalls) != 1 || mkdirCalls[0] != "/dst/" {
		t.Fatalf("mkdirCalls = %#v, want one /dst/", mkdirCalls)
	}
	waiting := jt.Waiting.snapshot()
	if len(waiting) != 1 || waiting[0].FileName != "a.txt" {
		t.Fatalf("waiting = %#v, want the source file queued after the root was created", waiting)
	}
}
