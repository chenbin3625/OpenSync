package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type scriptedTripper struct {
	calls atomic.Int64
	fn    func(call int, req *http.Request) (*http.Response, error)
}

func (s *scriptedTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	call := int(s.calls.Add(1))
	return s.fn(call, req)
}

func jsonResponse(status int, header http.Header, body string) *http.Response {
	if header == nil {
		header = make(http.Header)
	}
	header.Set("Content-Type", "application/json")
	return &http.Response{
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestNewAlistHTTPTransportKeepsHTTP2WithCustomDial(t *testing.T) {
	for _, allowInternal := range []bool{true, false} {
		tr := newAlistHTTPTransport(allowInternal)
		if !tr.ForceAttemptHTTP2 {
			t.Fatalf("allowInternal=%v ForceAttemptHTTP2 = false, want true so DialContext does not disable h2", allowInternal)
		}
		if tr.DialContext == nil {
			t.Fatalf("allowInternal=%v DialContext = nil", allowInternal)
		}
		if tr.TLSClientConfig == nil || len(tr.TLSClientConfig.NextProtos) == 0 || tr.TLSClientConfig.NextProtos[0] != "h2" {
			t.Fatalf("allowInternal=%v TLS NextProtos = %#v, want h2 first", allowInternal, tr.TLSClientConfig)
		}
	}
}

func TestProtocolTripperUpgradesToHTTP3AfterAltSvcAndStopsAfterFailure(t *testing.T) {
	var h12, h3 atomic.Int64
	h12Trip := &scriptedTripper{fn: func(call int, req *http.Request) (*http.Response, error) {
		h12.Add(1)
		header := make(http.Header)
		header.Set("Alt-Svc", `h3=":443"; ma=86400`)
		return jsonResponse(200, header, `{"ok":true}`), nil
	}}
	h3Trip := &scriptedTripper{fn: func(call int, req *http.Request) (*http.Response, error) {
		h3.Add(1)
		if call == 1 {
			return nil, errors.New("quic handshake timeout")
		}
		return jsonResponse(200, nil, `{"ok":true}`), nil
	}}

	tripper := wrapAlistRoundTripper(h12Trip, h3Trip)
	req := httptest.NewRequest(http.MethodGet, "https://alist.example.test/api/me", nil)

	if _, err := tripper.RoundTrip(req); err != nil {
		t.Fatalf("first RoundTrip error: %v", err)
	}
	if h12.Load() != 1 || h3.Load() != 0 {
		t.Fatalf("first trip h12=%d h3=%d, want HTTP/2 until Alt-Svc", h12.Load(), h3.Load())
	}

	if _, err := tripper.RoundTrip(req.Clone(context.Background())); err != nil {
		t.Fatalf("second RoundTrip error: %v", err)
	}
	if h3.Load() != 1 || h12.Load() != 2 {
		t.Fatalf("after Alt-Svc h3=%d h12=%d, want one failed HTTP/3 then HTTP/2 fallback", h3.Load(), h12.Load())
	}

	if _, err := tripper.RoundTrip(req.Clone(context.Background())); err != nil {
		t.Fatalf("third RoundTrip error: %v", err)
	}
	if h3.Load() != 1 {
		t.Fatalf("HTTP/3 retries after failure: h3=%d, want sticky HTTP/2", h3.Load())
	}
}

func TestProtocolTripperDoesNotUseHTTP3ForCleartext(t *testing.T) {
	var h3 atomic.Int64
	h12Trip := &scriptedTripper{fn: func(call int, req *http.Request) (*http.Response, error) {
		header := make(http.Header)
		header.Set("Alt-Svc", `h3=":443"; ma=86400`)
		return jsonResponse(200, header, `{}`), nil
	}}
	h3Trip := &scriptedTripper{fn: func(call int, req *http.Request) (*http.Response, error) {
		h3.Add(1)
		return jsonResponse(200, nil, `{}`), nil
	}}
	tripper := wrapAlistRoundTripper(h12Trip, h3Trip)
	req := httptest.NewRequest(http.MethodGet, "http://alist.lan/api/me", nil)
	if _, err := tripper.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip error: %v", err)
	}
	if _, err := tripper.RoundTrip(req.Clone(context.Background())); err != nil {
		t.Fatalf("second RoundTrip error: %v", err)
	}
	if h3.Load() != 0 {
		t.Fatalf("cleartext HTTP used HTTP/3 %d times", h3.Load())
	}
}

func TestAltSvcOffersHTTP3(t *testing.T) {
	if !altSvcOffersHTTP3(`h3=":443"; ma=86400`) {
		t.Fatal("expected h3 advertisement")
	}
	if !altSvcOffersHTTP3(`h2=":443", h3=":443"; ma=2592000`) {
		t.Fatal("expected h3 among mixed Alt-Svc")
	}
	if altSvcOffersHTTP3(`h2=":443"; ma=86400`) {
		t.Fatal("h2-only Alt-Svc should not enable HTTP/3")
	}
}

func TestCloneHTTPRequestReplaysBytesBody(t *testing.T) {
	payload := []byte(`{"path":"/media"}`)
	req, err := http.NewRequest(http.MethodPost, "https://alist.example.test/api/fs/list", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	clone, err := cloneHTTPRequest(req)
	if err != nil {
		t.Fatalf("cloneHTTPRequest() error: %v", err)
	}
	got, err := io.ReadAll(clone.Body)
	if err != nil {
		t.Fatalf("read clone body: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("clone body = %q, want %q", got, payload)
	}
}
