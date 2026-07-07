package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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

	jt := &JobTask{
		TaskID:  42,
		Job:     map[string]interface{}{"method": 1},
		Waiting: newCopyQueue(),
		AlistClient: &AlistClient{
			URL:    server.URL,
			client: server.Client(),
		},
	}
	jt.initRuntime()

	jt.syncWithHave(scanWork{
		SrcPath:     "/src/",
		DstPath:     "/dst/",
		SrcRootPath: "/src/",
		DstRootPath: "/dst/",
		FirstDst:    true,
		Mode:        scanWorkCompare,
	}, nil)

	if len(removeCalls) != 1 || removeCalls[0] != "/dst/foo/" {
		t.Fatalf("removeCalls = %#v, want /dst/foo/", removeCalls)
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
