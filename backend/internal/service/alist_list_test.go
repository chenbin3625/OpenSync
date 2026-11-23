package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"opensync/internal/msg"
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
	if n != 0 || err == nil || !errors.Is(err, ErrResponseBodyTooLarge) || !strings.Contains(err.Error(), "exceeds") {
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
		wantRefresh := req.Page == 1
		if req.Path != "/tv" || req.PerPage != 2 || req.Refresh != wantRefresh {
			t.Fatalf("list request page %d = %#v, want refresh=%v", req.Page, req, wantRefresh)
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
	if !seen[1].Refresh || seen[2].Refresh || seen[3].Refresh {
		t.Fatalf("refresh flags = page1:%v page2:%v page3:%v, want true/false/false", seen[1].Refresh, seen[2].Refresh, seen[3].Refresh)
	}
	if maxInflight < 2 {
		t.Fatalf("max inflight = %d, want overlapping remaining-page fetches", maxInflight)
	}
}

func TestFileListLimitExceededAllowsExactLimit(t *testing.T) {
	if fileListLimitExceeded(maxFileListEntries - 1) {
		t.Fatal("entry count below limit was rejected")
	}
	if fileListLimitExceeded(maxFileListEntries) {
		t.Fatal("entry count exactly at limit was rejected")
	}
	if !fileListLimitExceeded(maxFileListEntries + 1) {
		t.Fatal("entry count above limit was accepted")
	}
}

// Entry names come from the AList server, which may proxy untrusted third-party
// storage. Names that are not plain file names are dropped rather than being
// concatenated onto a scan path.
func TestAddFileListEntryRejectsUnsafeNames(t *testing.T) {
	unsafe := []string{"", ".", "..", "../escape", "a/b", "a\\b", "/abs"}
	for _, name := range unsafe {
		result := make(FileListResult)
		addFileListEntry(result, FileListEntry{Name: name, Size: 10})
		if len(result) != 0 {
			t.Errorf("addFileListEntry(%q) kept %#v, want dropped", name, result)
		}
	}

	result := make(FileListResult)
	addFileListEntry(result, FileListEntry{Name: "normal file.txt", Size: 10})
	addFileListEntry(result, FileListEntry{Name: "dir", IsDir: true})
	if _, ok := result["normal file.txt"]; !ok {
		t.Errorf("plain file name was dropped: %#v", result)
	}
	if _, ok := result["dir/"]; !ok {
		t.Errorf("plain directory name was dropped: %#v", result)
	}
}

func TestFileListApiContextGuardsIntegerOverflowOnHugeTotal(t *testing.T) {
	origPageSize := fileListPageSize
	fileListPageSize = 2
	defer func() { fileListPageSize = origPageSize }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[{"name":"a.txt","size":1},{"name":"b.txt","size":2}],"total":` + strconv.Itoa(math.MaxInt) + `}}`))
	}))
	defer server.Close()

	client := &AlistClient{URL: server.URL, client: server.Client()}
	_, err := client.FileListApiContext(context.Background(), "/huge", 0, 0)
	if err == nil {
		t.Fatal("FileListApiContext() error = nil, want page limit error on math.MaxInt total")
	}
	if !strings.Contains(err.Error(), "exceeded") {
		t.Fatalf("FileListApiContext() error = %q, want exceeded pages error", err.Error())
	}
}

func TestFileListApiContextEnforcesMaxEntriesOnSinglePage(t *testing.T) {
	origMax := maxFileListEntries
	origPageSize := fileListPageSize
	maxFileListEntries = 3
	fileListPageSize = 10
	defer func() {
		maxFileListEntries = origMax
		fileListPageSize = origPageSize
	}()

	makePayload := func(count int) string {
		var items []string
		for i := 0; i < count; i++ {
			items = append(items, `{"name":"f`+strconv.Itoa(i)+`.txt","size":1}`)
		}
		return `{"code":200,"message":"ok","data":{"content":[` + strings.Join(items, ",") + `],"total":` + strconv.Itoa(count) + `}}`
	}

	var returnCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(makePayload(int(returnCount.Load()))))
	}))
	defer server.Close()

	client := &AlistClient{URL: server.URL, client: server.Client()}

	returnCount.Store(3)
	files, err := client.FileListApiContext(context.Background(), "/exact", 0, 0)
	if err != nil {
		t.Fatalf("FileListApiContext(exact single-page limit) error: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("len(files) = %d, want 3", len(files))
	}

	returnCount.Store(4)
	if _, err := client.FileListApiContext(context.Background(), "/over", 0, 0); err == nil || !strings.Contains(err.Error(), "more than") {
		t.Fatalf("FileListApiContext(single-page over limit) error = %v, want max entries error", err)
	}
}

func TestFileListApiContextHandlesDuplicatesAndUnsafeNamesAcrossPages(t *testing.T) {
	origPageSize := fileListPageSize
	fileListPageSize = 2
	defer func() { fileListPageSize = origPageSize }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req alistListRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		var content string
		switch req.Page {
		case 1:
			content = `{"name":"a.txt","size":1},{"name":"../escape","size":2}`
		case 2:
			content = `{"name":"a.txt","size":1},{"name":"b.txt","size":3}`
		case 3:
			content = `{"name":"c.txt","size":4},{"name":"sub/bad","size":5}`
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[` + content + `],"total":6}}`))
	}))
	defer server.Close()

	client := &AlistClient{URL: server.URL, client: server.Client()}
	files, err := client.FileListApiContext(context.Background(), "/mixed", 0, 0)
	if err != nil {
		t.Fatalf("FileListApiContext() unexpected error with duplicates/unsafe names: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("len(files) = %d, want 3 sanitized unique entries (%#v)", len(files), files)
	}
	for _, want := range []string{"a.txt", "b.txt", "c.txt"} {
		if _, ok := files[want]; !ok {
			t.Fatalf("missing %q in %#v", want, files)
		}
	}

	// Verify genuinely truncated page still fails completeness validation.
	incompleteServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req alistListRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		content := `{"name":"a.txt","size":1},{"name":"b.txt","size":2}`
		if req.Page == 2 {
			content = `{"name":"c.txt","size":3}`
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"content":[` + content + `],"total":4}}`))
	}))
	defer incompleteServer.Close()

	incompleteClient := &AlistClient{URL: incompleteServer.URL, client: incompleteServer.Client()}
	if _, err := incompleteClient.FileListApiContext(context.Background(), "/short", 0, 0); err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("FileListApiContext(incomplete) error = %v, want incomplete error", err)
	}
}

type drainTrackingBody struct {
	r      *strings.Reader
	eof    bool
	closed bool
}

func (b *drainTrackingBody) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	if err == io.EOF {
		b.eof = true
	}
	return n, err
}

func (b *drainTrackingBody) Close() error {
	b.closed = true
	return nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestFileListApiContextDrainsResponseBodyForKeepAlive(t *testing.T) {
	t.Run("200 with trailing newline", func(t *testing.T) {
		body := &drainTrackingBody{
			r: strings.NewReader(`{"code":200,"message":"ok","data":{"content":[],"total":0}}` + "\r\n   \n"),
		}
		client := &AlistClient{
			URL: "https://alist.example.test",
			client: &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       body,
						Header:     make(http.Header),
						Request:    req,
					}, nil
				}),
			},
		}
		if _, err := client.FileListApiContext(context.Background(), "/", 0, 0); err != nil {
			t.Fatalf("FileListApiContext() error: %v", err)
		}
		if !body.eof || !body.closed {
			t.Fatalf("body eof=%v closed=%v, want both true for keep-alive reuse", body.eof, body.closed)
		}
	})

	t.Run("non-200 error response drained", func(t *testing.T) {
		body := &drainTrackingBody{
			r: strings.NewReader(`<html><body>502 Bad Gateway</body></html>`),
		}
		client := &AlistClient{
			URL: "https://alist.example.test",
			client: &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusBadGateway,
						Body:       body,
						Header:     make(http.Header),
						Request:    req,
					}, nil
				}),
			},
		}
		if _, err := client.FileListApiContext(context.Background(), "/", 0, 0); err == nil {
			t.Fatal("FileListApiContext() error = nil, want 502 error")
		}
		if !body.eof || !body.closed {
			t.Fatalf("error body eof=%v closed=%v, want both true for keep-alive reuse", body.eof, body.closed)
		}
	})
}

func TestFileListApiContextNonObjectDataStillChecksAlistCode(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		wantNotFound bool
		wantUnAuth   bool
		wantErrPart  string
	}{
		{
			name:         "500 object not found with string data",
			body:         `{"code":500,"message":"failed get dir: object not found","data":"string-detail"}`,
			wantNotFound: true,
		},
		{
			name:       "401 with array data before code",
			body:       `{"data":["extra",1],"code":401,"message":"token expired"}`,
			wantUnAuth: true,
		},
		{
			name:         "404 with null data",
			body:         `{"code":404,"message":"object not found","data":null}`,
			wantNotFound: true,
		},
		{
			name:        "200 with non-object data still fails",
			body:        `{"code":200,"message":"ok","data":"unexpected"}`,
			wantErrPart: "not an object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			client := &AlistClient{URL: server.URL, client: server.Client()}
			_, err := client.FileListApiContext(context.Background(), "/test", 0, 0)
			if err == nil {
				t.Fatalf("FileListApiContext() error = nil, want error")
			}
			if tt.wantNotFound && !isAlistObjectNotFound(err) {
				t.Fatalf("isAlistObjectNotFound(%v) = false, want true", err)
			}
			if tt.wantUnAuth && err.Error() != msg.AlistUnAuth {
				t.Fatalf("error = %q, want %q", err.Error(), msg.AlistUnAuth)
			}
			if tt.wantErrPart != "" && !strings.Contains(err.Error(), tt.wantErrPart) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantErrPart)
			}
		})
	}
}
