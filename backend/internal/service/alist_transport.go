package service

import (
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"opensync/internal/config"
)

const alistHTTP3HandshakeTimeout = 2 * time.Second

var errRequestBodyNotReplayable = errors.New("alist: request body cannot be retried")

// newAlistHTTPClient builds the outbound AList client. HTTPS uses HTTP/2
// (ForceAttemptHTTP2 stays on even with a custom dialer / TLS config; Go
// otherwise silently falls back to HTTP/1.1 and the 6-connection cap). HTTP/3
// is opportunistic: only after the origin advertises Alt-Svc, so a LAN AList
// without QUIC does not pay a handshake timeout on every job.
func newAlistHTTPClient(allowInternalAlist bool) *http.Client {
	return &http.Client{
		Timeout:   300 * time.Second,
		Transport: newAlistRoundTripper(allowInternalAlist),
	}
}

func newAlistRoundTripper(allowInternalAlist bool) http.RoundTripper {
	return wrapAlistRoundTripper(newAlistHTTPTransport(allowInternalAlist), newAlistHTTP3Transport())
}

func newAlistHTTPTransport(allowInternalAlist bool) *http.Transport {
	dialer := &net.Dialer{
		Timeout:   15 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	dial := dialer.DialContext
	if !allowInternalAlist {
		dial = ssrfSafeDialContext(dialer, func() bool {
			return config.GetConfig().Server.AllowInternalAlist
		})
	}
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dial,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   32,
		MaxConnsPerHost:       32,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		WriteBufferSize:       32 << 10,
		ReadBufferSize:        32 << 10,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			NextProtos: []string{"h2", "http/1.1"},
		},
	}
}

func newAlistHTTP3Transport() *http3.Transport {
	return &http3.Transport{
		QUICConfig: &quic.Config{HandshakeIdleTimeout: alistHTTP3HandshakeTimeout},
	}
}

func wrapAlistRoundTripper(h12 http.RoundTripper, h3 http.RoundTripper) http.RoundTripper {
	return &protocolTripper{
		h12:      h12,
		h3:       h3,
		h3Hosts:  make(map[string]struct{}),
		h3Failed: make(map[string]struct{}),
	}
}

// protocolTripper sends HTTPS over HTTP/2 (or HTTP/1.1) until Alt-Svc names
// HTTP/3, then sticks to QUIC for that host. A failed HTTP/3 attempt is
// remembered so UDP-blocked NAS paths do not retry a 2s handshake forever.
type protocolTripper struct {
	h12      http.RoundTripper
	h3       http.RoundTripper
	mu       sync.Mutex
	h3Hosts  map[string]struct{}
	h3Failed map[string]struct{}
}

func (t *protocolTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme == "https" && t.preferHTTP3(req.URL.Host) {
		resp, err := t.h3.RoundTrip(req)
		if err == nil {
			return resp, nil
		}
		t.markHTTP3Failed(req.URL.Host)
		retry, retryErr := cloneHTTPRequest(req)
		if retryErr != nil {
			return nil, err
		}
		req = retry
	}

	resp, err := t.h12.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	if req.URL.Scheme == "https" && altSvcOffersHTTP3(resp.Header.Get("Alt-Svc")) {
		t.markHTTP3(req.URL.Host)
	}
	return resp, nil
}

func (t *protocolTripper) preferHTTP3(host string) bool {
	if t.h3 == nil || host == "" {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	_, ok := t.h3Hosts[host]
	return ok
}

func (t *protocolTripper) markHTTP3(host string) {
	if t.h3 == nil || host == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, failed := t.h3Failed[host]; failed {
		return
	}
	if t.h3Hosts == nil {
		t.h3Hosts = make(map[string]struct{})
	}
	t.h3Hosts[host] = struct{}{}
}

func (t *protocolTripper) markHTTP3Failed(host string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.h3Hosts, host)
	if t.h3Failed == nil {
		t.h3Failed = make(map[string]struct{})
	}
	t.h3Failed[host] = struct{}{}
}

func (t *protocolTripper) CloseIdleConnections() {
	if closer, ok := t.h12.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
	if closer, ok := t.h3.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

func (t *protocolTripper) Close() error {
	t.CloseIdleConnections()
	if closer, ok := t.h3.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

func cloneHTTPRequest(req *http.Request) (*http.Request, error) {
	clone := req.Clone(req.Context())
	if req.Body == nil || req.Body == http.NoBody {
		return clone, nil
	}
	if req.GetBody == nil {
		return nil, errRequestBodyNotReplayable
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, err
	}
	clone.Body = body
	return clone, nil
}

func altSvcOffersHTTP3(header string) bool {
	for _, part := range strings.Split(header, ",") {
		proto := strings.TrimSpace(strings.ToLower(part))
		if strings.HasPrefix(proto, "h3=") || strings.HasPrefix(proto, "h3=\"") {
			return true
		}
	}
	return false
}
