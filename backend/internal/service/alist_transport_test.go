package service

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAlistCheckRedirectProtectsAuthorizationHeader(t *testing.T) {
	makeReq := func(rawURL, auth string) *http.Request {
		req, err := http.NewRequest(http.MethodGet, rawURL, nil)
		if err != nil {
			t.Fatalf("NewRequest(%q) error: %v", rawURL, err)
		}
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		return req
	}

	t.Run("same origin preserves Authorization", func(t *testing.T) {
		prev := makeReq("https://alist.example.com/api/me", "secret-token")
		next := makeReq("https://alist.example.com/alist/api/me", "")
		if err := alistCheckRedirect(next, []*http.Request{prev}); err != nil {
			t.Fatalf("alistCheckRedirect() error: %v", err)
		}
		if got := next.Header.Get("Authorization"); got != "secret-token" {
			t.Fatalf("Authorization = %q, want secret-token", got)
		}
	})

	t.Run("same host http to https upgrade preserves Authorization", func(t *testing.T) {
		prev := makeReq("http://alist.example.com/api/me", "secret-token")
		next := makeReq("https://alist.example.com/api/me", "")
		if err := alistCheckRedirect(next, []*http.Request{prev}); err != nil {
			t.Fatalf("alistCheckRedirect() error: %v", err)
		}
		if got := next.Header.Get("Authorization"); got != "secret-token" {
			t.Fatalf("Authorization = %q, want secret-token", got)
		}
	})

	t.Run("cross origin strips Authorization", func(t *testing.T) {
		cases := []struct {
			name string
			from string
			to   string
		}{
			{"different host", "https://alist.example.com/api/me", "https://evil.example.org/steal"},
			{"subdomain", "https://example.com/api/me", "https://sub.example.com/steal"},
			{"different port", "https://alist.example.com:5244/api/me", "https://alist.example.com:9443/steal"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				prev := makeReq(tc.from, "secret-token")
				next := makeReq(tc.to, "secret-token")
				if err := alistCheckRedirect(next, []*http.Request{prev}); err != nil {
					t.Fatalf("alistCheckRedirect() error: %v", err)
				}
				if got := next.Header.Get("Authorization"); got != "" {
					t.Fatalf("Authorization = %q, want stripped on %s redirect", got, tc.name)
				}
			})
		}
	})

	t.Run("https to http downgrade refused and Authorization stripped", func(t *testing.T) {
		prev := makeReq("https://alist.example.com/api/me", "secret-token")
		next := makeReq("http://alist.example.com/api/me", "secret-token")
		err := alistCheckRedirect(next, []*http.Request{prev})
		if !errors.Is(err, errRefusedHTTPSDowngrade) {
			t.Fatalf("alistCheckRedirect() error = %v, want errRefusedHTTPSDowngrade", err)
		}
		if got := next.Header.Get("Authorization"); got != "" {
			t.Fatalf("Authorization = %q, want stripped on HTTPS downgrade", got)
		}
	})

	t.Run("newAlistHTTPClient wires CheckRedirect end-to-end", func(t *testing.T) {
		var dstAuth atomic.Value
		dstServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			dstAuth.Store(r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusOK)
		}))
		defer dstServer.Close()

		srcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, dstServer.URL+"/steal", http.StatusFound)
		}))
		defer srcServer.Close()

		client := newAlistHTTPClient()
		defer func() {
			if c, ok := client.Transport.(io.Closer); ok {
				_ = c.Close()
			}
		}()

		req, err := http.NewRequest(http.MethodGet, srcServer.URL+"/api/me", nil)
		if err != nil {
			t.Fatalf("NewRequest() error: %v", err)
		}
		req.Header.Set("Authorization", "top-secret")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("client.Do() error: %v", err)
		}
		defer resp.Body.Close()

		if got, _ := dstAuth.Load().(string); got != "" {
			t.Fatalf("cross-origin redirect target received Authorization = %q, want empty", got)
		}
	})
}

type mockH3Transport struct {
	roundTripFn  func(*http.Request) (*http.Response, error)
	closedIdle   atomic.Int32
	closedSocket atomic.Int32
}

func (m *mockH3Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if m.roundTripFn != nil {
		return m.roundTripFn(req)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("h3")),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func (m *mockH3Transport) CloseIdleConnections() {
	m.closedIdle.Add(1)
}

func (m *mockH3Transport) Close() error {
	m.closedSocket.Add(1)
	return nil
}

func TestAlistTransportCloseClosesHTTP3Transport(t *testing.T) {
	h12 := &closeTrackingTransport{}
	h3 := &mockH3Transport{}
	rt := wrapAlistRoundTripper(h12, h3)

	closer, ok := rt.(io.Closer)
	if !ok {
		t.Fatal("wrapAlistRoundTripper() does not implement io.Closer")
	}

	if err := closer.Close(); err != nil {
		t.Fatalf("first Close() error: %v", err)
	}
	if err := closer.Close(); err != nil {
		t.Fatalf("second Close() error: %v", err)
	}

	if !h12.closed.Load() {
		t.Fatal("Close() did not close idle connections on h12 transport")
	}
	if got := h3.closedSocket.Load(); got != 1 {
		t.Fatalf("h3.Close() called %d times, want 1", got)
	}

	// Verify real http3.Transport closes cleanly via AlistClient.Close().
	realClient := &AlistClient{client: newAlistHTTPClient()}
	realClient.Close()
}

func TestAlistTransportDoesNotBypassProxyForHTTP3(t *testing.T) {
	var h12Calls, h3Calls atomic.Int32
	h12 := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		h12Calls.Add(1)
		h := make(http.Header)
		h.Set("Alt-Svc", `h3=":443"; ma=2592000`)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("h12")),
			Header:     h,
			Request:    req,
		}, nil
	})
	h3 := &mockH3Transport{
		roundTripFn: func(req *http.Request) (*http.Response, error) {
			h3Calls.Add(1)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("h3")),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		},
	}

	proxyURL, _ := url.Parse("http://proxy.internal:8080")
	proxiedTripper := wrapAlistRoundTripper(h12, h3).(*protocolTripper)
	proxiedTripper.proxyFunc = func(*http.Request) (*url.URL, error) {
		return proxyURL, nil
	}

	for i := 0; i < 3; i++ {
		req, _ := http.NewRequest(http.MethodGet, "https://alist.example.com/api/me", nil)
		resp, err := proxiedTripper.RoundTrip(req)
		if err != nil {
			t.Fatalf("proxied RoundTrip #%d error: %v", i, err)
		}
		_ = resp.Body.Close()
	}
	if h3Calls.Load() != 0 {
		t.Fatalf("h3 called %d times while proxy is configured, want 0", h3Calls.Load())
	}
	if h12Calls.Load() != 3 {
		t.Fatalf("h12 called %d times while proxy is configured, want 3", h12Calls.Load())
	}

	// Without a proxy, Alt-Svc upgrades subsequent requests to HTTP/3.
	h12Calls.Store(0)
	h3Calls.Store(0)
	directTripper := wrapAlistRoundTripper(h12, h3).(*protocolTripper)
	directTripper.proxyFunc = func(*http.Request) (*url.URL, error) {
		return nil, nil
	}

	for i := 0; i < 2; i++ {
		req, _ := http.NewRequest(http.MethodGet, "https://alist.example.com/api/me", nil)
		resp, err := directTripper.RoundTrip(req)
		if err != nil {
			t.Fatalf("direct RoundTrip #%d error: %v", i, err)
		}
		_ = resp.Body.Close()
	}
	if h12Calls.Load() != 1 || h3Calls.Load() != 1 {
		t.Fatalf("direct calls h12=%d h3=%d, want 1 and 1", h12Calls.Load(), h3Calls.Load())
	}
}

func TestIsReplayableAlistRequestHandlesSubpathsAndMutations(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   bool
	}{
		{http.MethodGet, "/api/me", true},
		{http.MethodGet, "/alist/api/me", true},
		{http.MethodPost, "/api/fs/list", true},
		{http.MethodPost, "/alist/api/fs/list", true},
		{http.MethodPost, "/nested/sub/api/fs/get", true},
		{http.MethodPost, "/api/fs/copy", false},
		{http.MethodPost, "/alist/api/fs/copy", false},
		{http.MethodPost, "/alist/api/fs/copy/", false},
		{http.MethodPost, "/alist//api/fs/move", false},
		{http.MethodPost, "/alist/api/fs/remove", false},
		{http.MethodPost, "/api/fs/mkdir", false},
		{http.MethodPost, "/alist/api/fs/mkdir", false},
		{http.MethodPost, "/api/admin/task/copy/delete", false},
		{http.MethodPost, "/alist/api/admin/task/copy/cancel", false},
		{http.MethodPost, "/alist/api/fs/rename", false},
		{http.MethodDelete, "/alist/api/fs/list", false},
	}

	for _, tt := range tests {
		req, err := http.NewRequest(tt.method, "https://alist.example.com"+tt.path, nil)
		if err != nil {
			t.Fatalf("NewRequest(%s %s) error: %v", tt.method, tt.path, err)
		}
		if got := isReplayableAlistRequest(req); got != tt.want {
			t.Errorf("isReplayableAlistRequest(%s %s) = %v, want %v", tt.method, tt.path, got, tt.want)
		}
	}
}

func TestAlistTransportDoesNotReplaySubpathCopyAfterHTTP3Failure(t *testing.T) {
	var h12Calls atomic.Int32
	h12 := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		h12Calls.Add(1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"code":200}`)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})
	h3Err := errors.New("quic transport reset")
	h3 := &mockH3Transport{
		roundTripFn: func(req *http.Request) (*http.Response, error) {
			return nil, h3Err
		},
	}

	tripper := wrapAlistRoundTripper(h12, h3).(*protocolTripper)
	tripper.proxyFunc = func(*http.Request) (*url.URL, error) { return nil, nil }
	tripper.markHTTP3("alist.example.com")

	copyReq, _ := http.NewRequest(http.MethodPost, "https://alist.example.com/alist/api/fs/copy", bytes.NewReader([]byte(`{}`)))
	if _, err := tripper.RoundTrip(copyReq); !errors.Is(err, h3Err) {
		t.Fatalf("RoundTrip(subpath copy) error = %v, want %v", err, h3Err)
	}
	if h12Calls.Load() != 0 {
		t.Fatalf("subpath /alist/api/fs/copy was replayed to h12 (%d calls), want 0", h12Calls.Load())
	}

	// Read-only subpath /alist/api/fs/list SHOULD replay to h12 after HTTP/3 failure.
	tripper2 := wrapAlistRoundTripper(h12, h3).(*protocolTripper)
	tripper2.proxyFunc = func(*http.Request) (*url.URL, error) { return nil, nil }
	tripper2.markHTTP3("alist.example.com")

	listReq, _ := http.NewRequest(http.MethodPost, "https://alist.example.com/alist/api/fs/list", bytes.NewReader([]byte(`{"path":"/"}`)))
	resp, err := tripper2.RoundTrip(listReq)
	if err != nil {
		t.Fatalf("RoundTrip(subpath list) error = %v, want replay success", err)
	}
	_ = resp.Body.Close()
	if h12Calls.Load() != 1 {
		t.Fatalf("subpath /alist/api/fs/list h12 calls = %d, want 1", h12Calls.Load())
	}
}
