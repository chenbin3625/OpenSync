package service

import (
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

const (
	alistHTTP3HandshakeTimeout = 2 * time.Second
	maxAlistRedirects          = 10
)

var (
	errRequestBodyNotReplayable = errors.New("alist: request body cannot be retried")
	errRefusedHTTPSDowngrade    = errors.New("alist: refusing redirect from HTTPS to insecure scheme")
)

// newAlistHTTPClient builds the outbound AList client. HTTPS uses HTTP/2
// (ForceAttemptHTTP2 stays on even with a custom dialer / TLS config; Go
// otherwise silently falls back to HTTP/1.1 and the 6-connection cap). HTTP/3
// is opportunistic: only after the origin advertises Alt-Svc, so a LAN AList
// without QUIC does not pay a handshake timeout on every job.
func newAlistHTTPClient() *http.Client {
	return &http.Client{
		Timeout:       300 * time.Second,
		Transport:     newAlistRoundTripper(),
		CheckRedirect: alistCheckRedirect,
	}
}

// alistCheckRedirect protects the AList Authorization header across redirects:
//   - Refuses HTTPS -> non-HTTPS downgrades so credentials cannot leak over cleartext.
//   - Preserves Authorization on same-origin redirects and safe same-host HTTP -> HTTPS upgrades.
//   - Strips Authorization on cross-origin redirects (including subdomains and port changes).
func alistCheckRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxAlistRedirects {
		return errors.New("stopped after 10 redirects")
	}
	if len(via) == 0 {
		return nil
	}
	prev := via[len(via)-1]
	if prev.URL != nil && req.URL != nil &&
		strings.EqualFold(prev.URL.Scheme, "https") && !strings.EqualFold(req.URL.Scheme, "https") {
		req.Header.Del("Authorization")
		return errRefusedHTTPSDowngrade
	}
	if isSameAlistOrigin(prev.URL, req.URL) || isSafeHTTPSUpgrade(prev.URL, req.URL) {
		if auth := prev.Header.Get("Authorization"); auth != "" && req.Header.Get("Authorization") == "" {
			req.Header.Set("Authorization", auth)
		}
		return nil
	}
	req.Header.Del("Authorization")
	return nil
}

func normalizedURLPort(u *url.URL) string {
	if u == nil {
		return ""
	}
	if port := u.Port(); port != "" {
		return port
	}
	switch strings.ToLower(u.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

func isSameAlistOrigin(a, b *url.URL) bool {
	if a == nil || b == nil {
		return false
	}
	return strings.EqualFold(a.Scheme, b.Scheme) &&
		strings.EqualFold(a.Hostname(), b.Hostname()) &&
		normalizedURLPort(a) == normalizedURLPort(b)
}

func isSafeHTTPSUpgrade(prev, next *url.URL) bool {
	if prev == nil || next == nil {
		return false
	}
	return strings.EqualFold(prev.Scheme, "http") &&
		strings.EqualFold(next.Scheme, "https") &&
		strings.EqualFold(prev.Hostname(), next.Hostname()) &&
		normalizedURLPort(prev) == "80" &&
		normalizedURLPort(next) == "443"
}

func newAlistRoundTripper() http.RoundTripper {
	return wrapAlistRoundTripper(newAlistHTTPTransport(), newAlistHTTP3Transport())
}

func newAlistHTTPTransport() *http.Transport {
	dialer := &net.Dialer{
		Timeout:   15 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
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
	proxyFunc := http.ProxyFromEnvironment
	if tr, ok := h12.(*http.Transport); ok && tr.Proxy != nil {
		proxyFunc = tr.Proxy
	}
	return &protocolTripper{
		h12:       h12,
		h3:        h3,
		proxyFunc: proxyFunc,
		h3Hosts:   make(map[string]struct{}),
		h3Failed:  make(map[string]struct{}),
	}
}

// protocolTripper sends HTTPS over HTTP/2 (or HTTP/1.1) until Alt-Svc names
// HTTP/3, then sticks to QUIC for that host (unless a proxy is configured for
// the request). A failed HTTP/3 attempt is remembered so UDP-blocked NAS paths
// do not retry a 2s handshake forever.
type protocolTripper struct {
	h12       http.RoundTripper
	h3        http.RoundTripper
	proxyFunc func(*http.Request) (*url.URL, error)
	mu        sync.Mutex
	h3Hosts   map[string]struct{}
	h3Failed  map[string]struct{}
	closeOnce sync.Once
	closeErr  error
}

func (t *protocolTripper) usesProxy(req *http.Request) bool {
	if req == nil || req.URL == nil {
		return false
	}
	proxyFn := t.proxyFunc
	if proxyFn == nil {
		proxyFn = http.ProxyFromEnvironment
	}
	proxyURL, err := proxyFn(req)
	return err != nil || proxyURL != nil
}

func (t *protocolTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	proxied := t.usesProxy(req)
	if req.URL.Scheme == "https" && !proxied && t.preferHTTP3(req.URL.Host) {
		resp, err := t.h3.RoundTrip(req)
		if err == nil {
			return resp, nil
		}
		t.markHTTP3Failed(req.URL.Host)
		if !isReplayableAlistRequest(req) {
			// Do not transparently resend a mutating request over a new
			// protocol: the server may have already executed it. h3Failed is
			// recorded, so the caller's bounded retry (or the next request)
			// goes over HTTP/1.1 or HTTP/2 instead.
			return nil, err
		}
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
	if req.URL.Scheme == "https" && !proxied && altSvcOffersHTTP3(resp.Header.Get("Alt-Svc")) {
		t.markHTTP3(req.URL.Host)
	}
	return resp, nil
}

// replayableAlistPostPaths lists the AList POST endpoints that are strictly
// read-only and therefore safe to replay over HTTP/1.1 or HTTP/2 after an
// HTTP/3 transport failure. All other POST endpoints (copy, move, remove,
// mkdir, admin task mutations, etc.) are treated as non-idempotent even when
// AList is deployed behind a subpath such as /alist/api/fs/copy.
var replayableAlistPostPaths = map[string]bool{
	"/api/fs/list": true,
	"/api/fs/get":  true,
}

func normalizeAlistAPIPath(rawPath string) string {
	cleaned := path.Clean("/" + strings.TrimSpace(rawPath))
	if idx := strings.LastIndex(cleaned, "/api/"); idx >= 0 {
		return cleaned[idx:]
	}
	return cleaned
}

func isReplayableAlistRequest(req *http.Request) bool {
	if req == nil || req.URL == nil {
		return false
	}
	switch req.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	case http.MethodPost:
		return replayableAlistPostPaths[normalizeAlistAPIPath(req.URL.Path)]
	default:
		return false
	}
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

// Close releases idle TCP connections and closes the HTTP/3 transport so its
// underlying UDP socket and QUIC goroutines do not leak when clients are
// evicted or replaced.
func (t *protocolTripper) Close() error {
	t.CloseIdleConnections()
	t.closeOnce.Do(func() {
		if closer, ok := t.h3.(io.Closer); ok {
			t.closeErr = closer.Close()
		}
		if closer, ok := t.h12.(io.Closer); ok {
			if err := closer.Close(); t.closeErr == nil {
				t.closeErr = err
			}
		}
	})
	return t.closeErr
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
		if strings.HasPrefix(proto, "h3=") {
			return true
		}
	}
	return false
}
