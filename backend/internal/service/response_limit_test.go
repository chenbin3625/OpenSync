package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestAlistClientRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", int(maxResponseBytes+1))))
	}))
	defer server.Close()

	client := &AlistClient{
		URL:    server.URL,
		client: server.Client(),
	}

	_, err := client.GetContext(context.Background(), "/api/fs/list", nil)
	if err == nil {
		t.Fatalf("GetContext() error = nil, want response size error")
	}
	if !errors.Is(err, ErrResponseBodyTooLarge) {
		t.Fatalf("GetContext() error = %v, want ErrResponseBodyTooLarge", err)
	}
	if !strings.Contains(err.Error(), "response body exceeds") {
		t.Fatalf("GetContext() error = %q, want response size error", err.Error())
	}
}

func TestCopyOrMoveFileContextIncludesOperationInRequestError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "failure", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := &AlistClient{
		URL:    server.URL,
		client: server.Client(),
	}

	tests := []struct {
		name    string
		apiPath string
		call    func() (string, error)
	}{
		{
			name:    "copy",
			apiPath: "/api/fs/copy",
			call: func() (string, error) {
				return client.CopyFileContext(context.Background(), "/src", "/dst", "file.txt")
			},
		},
		{
			name:    "move",
			apiPath: "/api/fs/move",
			call: func() (string, error) {
				return client.MoveFileContext(context.Background(), "/src", "/dst", "file.txt")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.call()
			if err == nil {
				t.Fatalf("%s error = nil, want request error", tt.name)
			}
			if !strings.Contains(err.Error(), tt.apiPath) {
				t.Fatalf("%s error = %q, want api path context %q", tt.name, err.Error(), tt.apiPath)
			}
		})
	}
}

func TestSendNotifyRequestRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", maxNotifyResponseBytes+1)))
	}))
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatalf("sendNotifyRequest() panic = nil, want response size panic")
		}
		if !strings.Contains(strings.ToLower(recovered.(string)), "response body exceeds") {
			t.Fatalf("sendNotifyRequest() panic = %q, want response size panic", recovered)
		}
	}()

	sendNotifyRequest(server.Client(), req)
}

type chunkedTestReader struct {
	chunks [][]byte
	idx    int
}

func (r *chunkedTestReader) Read(p []byte) (int, error) {
	if r.idx >= len(r.chunks) {
		return 0, io.EOF
	}
	n := copy(p, r.chunks[r.idx])
	if n == len(r.chunks[r.idx]) {
		r.idx++
	} else {
		r.chunks[r.idx] = r.chunks[r.idx][n:]
	}
	return n, nil
}

func TestCapReaderExactLimitBoundaries(t *testing.T) {
	const limit = 8

	readWithBuf := func(src io.Reader, bufSize int) ([]byte, error) {
		cr := &capReader{r: src, limit: limit}
		buf := make([]byte, bufSize)
		var out bytes.Buffer
		for {
			n, err := cr.Read(buf)
			if n > 0 {
				out.Write(buf[:n])
			}
			if err == io.EOF {
				return out.Bytes(), nil
			}
			if err != nil {
				return out.Bytes(), err
			}
		}
	}

	tests := []struct {
		name      string
		payload   string
		wantError bool
	}{
		{name: "limit-1", payload: strings.Repeat("a", limit-1), wantError: false},
		{name: "exact limit", payload: strings.Repeat("b", limit), wantError: false},
		{name: "limit+1", payload: strings.Repeat("c", limit+1), wantError: true},
	}

	for _, tt := range tests {
		for _, bufSize := range []int{32, 8, 3, 1} {
			t.Run(tt.name+"_single_reader_buf_"+strconv.Itoa(bufSize), func(t *testing.T) {
				got, err := readWithBuf(strings.NewReader(tt.payload), bufSize)
				if tt.wantError {
					if !errors.Is(err, ErrResponseBodyTooLarge) {
						t.Fatalf("Read(%s, buf=%d) err = %v, want ErrResponseBodyTooLarge", tt.name, bufSize, err)
					}
					return
				}
				if err != nil {
					t.Fatalf("Read(%s, buf=%d) unexpected error: %v", tt.name, bufSize, err)
				}
				if string(got) != tt.payload {
					t.Fatalf("Read(%s, buf=%d) = %q, want %q", tt.name, bufSize, string(got), tt.payload)
				}
			})
		}

		t.Run(tt.name+"_chunked_reader", func(t *testing.T) {
			var chunks [][]byte
			for i := 0; i < len(tt.payload); i += 3 {
				end := i + 3
				if end > len(tt.payload) {
					end = len(tt.payload)
				}
				chunks = append(chunks, []byte(tt.payload[i:end]))
			}
			got, err := readWithBuf(&chunkedTestReader{chunks: chunks}, 16)
			if tt.wantError {
				if !errors.Is(err, ErrResponseBodyTooLarge) {
					t.Fatalf("chunked Read(%s) err = %v, want ErrResponseBodyTooLarge", tt.name, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("chunked Read(%s) unexpected error: %v", tt.name, err)
			}
			if string(got) != tt.payload {
				t.Fatalf("chunked Read(%s) = %q, want %q", tt.name, string(got), tt.payload)
			}
		})
	}
}

func TestCapReaderWithJSONDecoderAtExactLimit(t *testing.T) {
	payload := `{"code":200,"data":{"content":[],"total":0}}`
	exactLimit := int64(len(payload))

	var dst map[string]interface{}
	if err := json.NewDecoder(&capReader{r: strings.NewReader(payload), limit: exactLimit}).Decode(&dst); err != nil {
		t.Fatalf("Decode() at exact limit error: %v", err)
	}

	result := FileListResult{}
	if _, _, code, _, err := decodeFileListResponse(strings.NewReader(payload), exactLimit, result); err != nil || code != 200 {
		t.Fatalf("decodeFileListResponse() at exact limit = code %d err %v, want 200 nil", code, err)
	}

	overPayload := `{"code":200, "data":{"content":[],"total":0}}`
	if _, _, _, _, err := decodeFileListResponse(strings.NewReader(overPayload), exactLimit, result); !errors.Is(err, ErrResponseBodyTooLarge) {
		t.Fatalf("decodeFileListResponse() at limit+1 err = %v, want ErrResponseBodyTooLarge", err)
	}
}

func TestResponseReadAllWithLimitExactBoundaries(t *testing.T) {
	const limit = 8
	for _, size := range []int{limit - 1, limit} {
		want := strings.Repeat("z", size)
		got, err := readAllWithLimit(strings.NewReader(want), limit)
		if err != nil {
			t.Fatalf("readAllWithLimit(%d) error: %v", size, err)
		}
		if string(got) != want {
			t.Fatalf("readAllWithLimit(%d) = %q, want %q", size, string(got), want)
		}
	}
	if _, err := readAllWithLimit(strings.NewReader(strings.Repeat("z", limit+1)), limit); !errors.Is(err, ErrResponseBodyTooLarge) {
		t.Fatalf("readAllWithLimit(%d) error = %v, want ErrResponseBodyTooLarge", limit+1, err)
	}
}
