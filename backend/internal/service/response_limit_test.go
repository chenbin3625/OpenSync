package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAlistClientRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", maxResponseBytes+1)))
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
