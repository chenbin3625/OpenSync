package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDecodeFileListResponseStreamsEntriesAndSkipsEnvelopeNoise(t *testing.T) {
	raw := `{"code":200,"message":"ok","extra":{"nested":[1,2,{"k":"v"}]},"data":{"readme":"x","content":[{"name":"dir","is_dir":true},{"name":"a.mkv","is_dir":false,"size":9,"hash_info":{"sha1":"ff","md5":"AB"}}],"total":2}}`
	result := FileListResult{}
	n, total, code, message, err := decodeFileListResponse(strings.NewReader(raw), 1<<20, result)
	if err != nil {
		t.Fatalf("decodeFileListResponse() error: %v", err)
	}
	if code != 200 || message != "ok" || n != 2 || total != 2 {
		t.Fatalf("code=%d message=%q n=%d total=%d", code, message, n, total)
	}
	if _, ok := result["dir/"]; !ok {
		t.Fatalf("missing dir/, got %#v", result)
	}
	if result["a.mkv"].Size != 9 || result["a.mkv"].MD5 != "ab" {
		t.Fatalf("file metadata = %#v", result["a.mkv"])
	}
}

func TestCapReaderRejectsOversizedStream(t *testing.T) {
	r := &capReader{r: strings.NewReader(strings.Repeat("a", 32)), limit: 8}
	buf := make([]byte, 32)
	n, err := r.Read(buf)
	if n != 8 || err != nil {
		t.Fatalf("first Read n=%d err=%v, want 8 nil", n, err)
	}
	n, err = r.Read(buf)
	if n != 0 || err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("second Read n=%d err=%v, want exceeds", n, err)
	}

	result := FileListResult{}
	_, _, _, _, err = decodeFileListResponse(strings.NewReader(`{"code":200,"data":{"content":[],"total":0}}`), 8, result)
	if err == nil {
		t.Fatal("decodeFileListResponse() error = nil, want size cap on oversized envelope")
	}
}

func TestFileListApiContextPostsTypedBodyAndOverlapsRemainingPages(t *testing.T) {
	origPageSize := fileListPageSize
	fileListPageSize = 2
	defer func() { fileListPageSize = origPageSize }()

	var mu sync.Mutex
	inflight := 0
	maxInflight := 0
	seen := map[int]alistListRequest{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/fs/list" {
			t.Fatalf("path = %s, want /api/fs/list", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Fatalf("Content-Type = %q", ct)
		}
		var req alistListRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode list request: %v", err)
		}
		if req.Path != "/tv" || req.PerPage != 2 || !req.Refresh {
			t.Fatalf("list request = %#v", req)
		}

		mu.Lock()
		inflight++
		if inflight > maxInflight {
			maxInflight = inflight
		}
		seen[req.Page] = req
		mu.Unlock()
		time.Sleep(50 * time.Millisecond)
		mu.Lock()
		inflight--
		mu.Unlock()

		base := (req.Page - 1) * 2
		items := `{"name":"a` + strconv.Itoa(base) + `.mkv","is_dir":false,"size":1,"hash_info":{"md5":"Aa"}},{"name":"a` + strconv.Itoa(base+1) + `.mkv","is_dir":false,"size":2}`
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[` + items + `],"total":6}}`))
	}))
	defer server.Close()

	client := &AlistClient{URL: server.URL, client: server.Client()}
	files, err := client.FileListApiContext(context.Background(), "/tv", 0, 0)
	if err != nil {
		t.Fatalf("FileListApiContext() error: %v", err)
	}
	if len(files) != 6 {
		t.Fatalf("len(files) = %d, want 6 (%#v)", len(files), files)
	}
	if files["a0.mkv"].MD5 != "aa" || files["a1.mkv"].Size != 2 {
		t.Fatalf("metadata = %#v", files)
	}
	if len(seen) != 3 {
		t.Fatalf("pages = %#v, want 1,2,3", seen)
	}
	if maxInflight < 2 {
		t.Fatalf("max inflight = %d, want overlapping remaining-page fetches", maxInflight)
	}
}
