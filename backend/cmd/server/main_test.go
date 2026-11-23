package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"opensync/internal/config"
	"opensync/internal/mapper"
	"opensync/internal/model"
	"opensync/pkg/crypto"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
)

func TestErrorRecoveryReturnsHTTP500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(errorRecovery())
	router.GET("/panic", func(c *gin.Context) {
		panic("boom")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestErrorRecoveryDoesNotExposeInternalPanicText(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(errorRecovery())
	router.GET("/panic", func(c *gin.Context) {
		panic("database locked: SELECT * FROM user_list")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	router.ServeHTTP(w, req)

	body := w.Body.String()
	for _, leaked := range []string{"database locked", "SELECT * FROM user_list"} {
		if strings.Contains(body, leaked) {
			t.Fatalf("response body leaked %q: %s", leaked, body)
		}
	}
}

func TestErrorRecoveryExposesPublicErrorMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(errorRecovery())
	router.GET("/business-error", func(c *gin.Context) {
		panic(model.PublicError("最小文件大小不能大于最大文件大小"))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/business-error", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(w.Body.String(), "最小文件大小不能大于最大文件大小") {
		t.Fatalf("response body = %s, want public error message", w.Body.String())
	}
}

func TestSecurityHeadersAreApplied(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(securityHeaders())
	router.GET("/", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	router.ServeHTTP(w, req)

	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := w.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options = %q, want DENY", got)
	}
	if got := w.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("Referrer-Policy = %q, want no-referrer", got)
	}
}

func TestSPAFallbackServesIndexForClientRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	webDist := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>OpenSync</html>")},
	}
	router := gin.New()
	router.NoRoute(func(c *gin.Context) {
		serveSPAFallback(c, webDist)
	})

	for _, route := range []string{"/login", "/home", "/home/task/detail", "/engine", "/notify", "/setting"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, route, nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("GET %s status = %d, want %d", route, w.Code, http.StatusOK)
		}
		if !strings.Contains(w.Body.String(), "OpenSync") {
			t.Errorf("GET %s body = %q, want frontend index", route, w.Body.String())
		}
	}
}

func TestSPAFallbackKeepsAPIAndAssetMissesAs404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	webDist := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>OpenSync</html>")},
	}
	router := gin.New()
	router.NoRoute(func(c *gin.Context) {
		serveSPAFallback(c, webDist)
	})

	for _, route := range []string{"/svr/missing", "/assets/missing.js", "/favicon.ico"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, route, nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("GET %s status = %d, want %d", route, w.Code, http.StatusNotFound)
		}
	}
}

func newAssetTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	assets := fstest.MapFS{
		"app.js": &fstest.MapFile{Data: bytes.Repeat([]byte("console.log('opensync');\n"), 64)},
	}
	handler := serveAssetFile(http.StripPrefix("/assets", http.FileServer(http.FS(assets))))
	router := gin.New()
	router.GET("/assets/*filepath", handler)
	router.HEAD("/assets/*filepath", handler)
	return router
}

func TestServeAssetFileCompressesAndCachesImmutably(t *testing.T) {
	router := newAssetTestRouter()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if got := w.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("Cache-Control = %q, want immutable asset cache", got)
	}
	if got := w.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if got := w.Header().Get("Vary"); !strings.Contains(got, "Accept-Encoding") {
		t.Fatalf("Vary = %q, want Accept-Encoding", got)
	}
	if got := w.Header().Get("Content-Length"); got != "" {
		t.Fatalf("Content-Length = %q, want empty for a compressed body", got)
	}

	reader, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatalf("gzip.NewReader() error: %v", err)
	}
	decompressed, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read gzip body: %v", err)
	}
	if !bytes.Contains(decompressed, []byte("console.log('opensync')")) {
		t.Fatalf("decompressed body lost the asset content")
	}
}

func TestServeAssetFileSkipsCompressionWithoutGzipSupport(t *testing.T) {
	router := newAssetTestRouter()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if got := w.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty", got)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("console.log('opensync')")) {
		t.Fatalf("body lost the asset content")
	}
}

func TestServeAssetFileDoesNotCacheMissesImmutably(t *testing.T) {
	router := newAssetTestRouter()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
	if got := w.Header().Get("Cache-Control"); got != "" {
		t.Fatalf("Cache-Control = %q, want no immutable caching for a miss", got)
	}
}

func TestServeAssetFileDoesNotCompressHead(t *testing.T) {
	router := newAssetTestRouter()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodHead, "/assets/app.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if got := w.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("HEAD Content-Encoding = %q, want empty", got)
	}
	if w.Body.Len() != 0 {
		t.Fatalf("HEAD body length = %d, want 0", w.Body.Len())
	}
}

func TestSPAFallbackDisablesIndexCaching(t *testing.T) {
	gin.SetMode(gin.TestMode)
	webDist := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>OpenSync</html>")},
	}
	router := gin.New()
	router.NoRoute(func(c *gin.Context) {
		serveSPAFallback(c, webDist)
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/home", nil))

	if got := w.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("Cache-Control = %q, want no-cache for index.html", got)
	}
}

func TestConfigureTrustedProxiesHonorsConfiguredProxies(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rTrusted := gin.New()
	if err := configureTrustedProxies(rTrusted, []string{"127.0.0.1"}); err != nil {
		t.Fatalf("configureTrustedProxies() error: %v", err)
	}
	var gotTrustedIP string
	rTrusted.GET("/ip", func(c *gin.Context) {
		gotTrustedIP = c.ClientIP()
		c.Status(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "127.0.0.1:4321"
	req.Header.Set("X-Forwarded-For", "203.0.113.55")
	rTrusted.ServeHTTP(httptest.NewRecorder(), req)
	if gotTrustedIP != "203.0.113.55" {
		t.Fatalf("ClientIP with trusted proxy = %q, want 203.0.113.55", gotTrustedIP)
	}

	rUntrusted := gin.New()
	if err := configureTrustedProxies(rUntrusted, nil); err != nil {
		t.Fatalf("configureTrustedProxies(nil) error: %v", err)
	}
	var gotUntrustedIP string
	rUntrusted.GET("/ip", func(c *gin.Context) {
		gotUntrustedIP = c.ClientIP()
		c.Status(http.StatusOK)
	})
	req2 := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req2.RemoteAddr = "127.0.0.1:4321"
	req2.Header.Set("X-Forwarded-For", "203.0.113.55")
	rUntrusted.ServeHTTP(httptest.NewRecorder(), req2)
	if gotUntrustedIP != "127.0.0.1" {
		t.Fatalf("ClientIP without trusted proxies = %q, want 127.0.0.1", gotUntrustedIP)
	}
}

func TestRunHTTPServerReturnsNilWhenContextCancelled(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	}

	if err := runHTTPServer(ctx, server, ln); err != nil {
		t.Fatalf("runHTTPServer() error = %v, want nil", err)
	}
}

func TestRunHTTPServerCancelsActiveStreamImmediatelyOnShutdown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error: %v", err)
	}
	streamStarted := make(chan struct{})
	streamCancelled := make(chan struct{})

	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			close(streamStarted)
			<-r.Context().Done()
			close(streamCancelled)
		}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runHTTPServer(ctx, server, ln)
	}()

	reqDone := make(chan struct{})
	go func() {
		defer close(reqDone)
		resp, err := http.Get("http://" + ln.Addr().String() + "/svr/job/stream")
		if err == nil && resp != nil {
			_, _ = io.ReadAll(resp.Body)
			_ = resp.Body.Close()
		}
	}()

	select {
	case <-streamStarted:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("timed out waiting for SSE stream to start")
	}

	start := time.Now()
	cancel()

	select {
	case <-streamCancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("SSE request context was not cancelled on shutdown")
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runHTTPServer() error = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runHTTPServer() hung waiting for SSE stream")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("shutdown took %v, expected immediate BaseContext cancellation", elapsed)
	}
	<-reqDone
}

func TestRunHTTPServerRejectsHalfConfiguredTLS(t *testing.T) {
	for _, tc := range []struct {
		name string
		cert string
		key  string
	}{
		{"cert only", "/tmp/cert.pem", ""},
		{"key only", "", "/tmp/key.pem"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatalf("Listen() error: %v", err)
			}
			server := &http.Server{Handler: http.NewServeMux()}
			if err := runHTTPServer(context.Background(), server, ln, tc.cert, tc.key); err == nil {
				t.Fatalf("runHTTPServer() error = nil for %s, want fail-fast error", tc.name)
			}
		})
	}
}

func writeSelfSignedTestCert(t *testing.T) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return certFile, keyFile
}

func TestRunHTTPServerWithTLSAndHTTP3GracefulShutdown(t *testing.T) {
	certFile, keyFile := writeSelfSignedTestCert(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error: %v", err)
	}
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runHTTPServer(ctx, server, ln, certFile, keyFile)
	}()

	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	var resp *http.Response
	for i := 0; i < 20; i++ {
		resp, err = client.Get("https://" + ln.Addr().String() + "/")
		if err == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err != nil {
		cancel()
		t.Fatalf("HTTPS request failed: %v", err)
	}
	_ = resp.Body.Close()

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runHTTPServer(TLS+H3) shutdown error = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runHTTPServer(TLS+H3) shutdown timed out")
	}
}

func TestRunCLIResetPasswordUpdatesStoredCredentials(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	if _, err := db.Exec(`CREATE TABLE user_list(
		id integer primary key autoincrement,
		userName text,
		passwd text,
		recoveryKey text,
		sqlVersion integer,
		createTime integer DEFAULT 1
	)`); err != nil {
		t.Fatalf("create user_list: %v", err)
	}
	oldPasswordHash, err := crypto.HashPassword("old-password")
	if err != nil {
		t.Fatalf("HashPassword(old-password) error: %v", err)
	}
	oldRecoveryHash, err := crypto.HashPassword("old-recovery-key")
	if err != nil {
		t.Fatalf("HashPassword(old-recovery-key) error: %v", err)
	}
	if _, err := db.Exec("INSERT INTO user_list(id, userName, passwd, recoveryKey, sqlVersion) VALUES (1, 'admin', ?, ?, 260612)", oldPasswordHash, oldRecoveryHash); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	restoreDB := mapper.SetDBForTest(db)
	t.Cleanup(restoreDB)
	oldConfig := config.GetConfig()
	config.SetConfigForTest(&config.Config{
		Server: config.ServerConfig{PasswdStr: "test-cookie-secret"},
	})
	t.Cleanup(func() {
		config.SetConfigForTest(oldConfig)
	})

	var out bytes.Buffer
	handled, err := runCLI([]string{"reset-password", "--user", "admin"}, &out)
	if err != nil {
		t.Fatalf("runCLI(reset-password) error: %v", err)
	}
	if !handled {
		t.Fatalf("runCLI(reset-password) handled = false, want true")
	}
	output := out.String()
	if strings.Contains(output, "secret.key") {
		t.Fatalf("CLI output exposed secret.key: %s", output)
	}

	newPassword := matchFirstGroup(t, output, `新密码:\s+(\S+)`)
	newRecoveryKey := matchFirstGroup(t, output, `恢复密钥:\s+(\S+)`)
	if newPassword == "" || newRecoveryKey == "" {
		t.Fatalf("CLI output missing generated credentials: %s", output)
	}
	if len(newRecoveryKey) != 24 {
		t.Fatalf("new recovery key length = %d, want 24", len(newRecoveryKey))
	}

	var storedPasswordHash, storedRecoveryHash string
	if err := db.QueryRow("SELECT passwd, recoveryKey FROM user_list WHERE id=1").Scan(&storedPasswordHash, &storedRecoveryHash); err != nil {
		t.Fatalf("read updated hashes: %v", err)
	}
	if !crypto.CheckPassword(newPassword, storedPasswordHash) {
		t.Fatalf("stored password hash does not verify CLI password")
	}
	if !crypto.CheckPassword(newRecoveryKey, storedRecoveryHash) {
		t.Fatalf("stored recovery hash does not verify CLI recovery key")
	}
	if crypto.CheckPassword("old-recovery-key", storedRecoveryHash) {
		t.Fatalf("stored recovery hash still verifies old recovery key")
	}
}

func matchFirstGroup(t *testing.T, s string, pattern string) string {
	t.Helper()
	matches := regexp.MustCompile(pattern).FindStringSubmatch(s)
	if len(matches) != 2 {
		t.Fatalf("output %q did not match %s", s, pattern)
	}
	return matches[1]
}
