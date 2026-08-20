package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"io"
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

func TestGzipCompressionIsAppliedToJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(maybeCompress())
	router.GET("/svr/data", func(c *gin.Context) {
		c.Header("Content-Type", "application/json; charset=utf-8")
		c.String(200, strings.Repeat("OpenSync performance payload ", 500))
	})

	req := httptest.NewRequest(http.MethodGet, "/svr/data", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if !strings.Contains(w.Header().Get("Vary"), "Accept-Encoding") {
		t.Fatalf("Vary = %q, want Accept-Encoding", w.Header().Get("Vary"))
	}
	if w.Header().Get("Content-Length") != "" {
		t.Fatalf("Content-Length must be dropped for gzip body, got %q", w.Header().Get("Content-Length"))
	}
	zr, err := gzip.NewReader(w.Result().Body)
	if err != nil {
		t.Fatalf("gzip.NewReader error: %v", err)
	}
	defer zr.Close()
	body, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("read gzip body error: %v", err)
	}
	if !strings.Contains(string(body), "OpenSync performance payload") {
		t.Fatalf("decompressed body missing payload: %q", string(body))
	}
}

func TestGzipCompressionIsNotAppliedWithoutAcceptEncoding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(maybeCompress())
	router.GET("/svr/plain", func(c *gin.Context) {
		c.String(200, "plain response")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/svr/plain", nil)
	router.ServeHTTP(w, req)

	if w.Header().Get("Content-Encoding") != "" {
		t.Fatalf("Content-Encoding = %q, want empty", w.Header().Get("Content-Encoding"))
	}
	if body := w.Body.String(); body != "plain response" {
		t.Fatalf("body = %q, want uncompressed plain response", body)
	}
}

func TestGzipCompressionSkipsSSEStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(maybeCompress())
	router.GET("/svr/job/stream", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		c.Writer.WriteString("data: hello\n\n")
		c.Writer.Flush()
	})

	req := httptest.NewRequest(http.MethodGet, "/svr/job/stream", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Header().Get("Content-Encoding") != "" {
		t.Fatalf("SSE must not be gzipped, got Content-Encoding=%q", w.Header().Get("Content-Encoding"))
	}
	if body := w.Body.String(); body != "data: hello\n\n" {
		t.Fatalf("SSE body = %q, want raw stream", body)
	}
}

func TestGzipCompressionSkipsRangeRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(maybeCompress())
	router.HEAD("/assets/app.js", func(c *gin.Context) {
		c.Header("Content-Length", "12345")
		c.Header("Content-Type", "application/javascript")
		c.Status(http.StatusPartialContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Range", "bytes=0-1023")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Header().Get("Content-Encoding") != "" {
		t.Fatalf("range response must not be gzipped, got Content-Encoding=%q", w.Header().Get("Content-Encoding"))
	}
}

func TestStaticAssetsGetImmutableCacheHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(cachedStaticFiles())
	router.GET("/assets/app-hash123.js", func(c *gin.Context) {
		c.String(200, "bundle")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/assets/app-hash123.js", nil)
	router.ServeHTTP(w, req)

	if got := w.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("Cache-Control = %q, want immutable", got)
	}
}

func TestIndexHTMLGetsNoCacheHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/", noCacheHTML(), func(c *gin.Context) {
		c.String(200, "<html>OpenSync</html>")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	router.ServeHTTP(w, req)

	if got := w.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("Cache-Control = %q, want no-cache", got)
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
	dataDir := t.TempDir()
	t.Setenv("OPENSYNC_DATA_DIR", dataDir)
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
	if !strings.Contains(output, "reset-credentials.txt") {
		t.Fatalf("CLI output missing credential file path: %s", output)
	}
	credBytes, err := os.ReadFile(filepath.Join(dataDir, "reset-credentials.txt"))
	if err != nil {
		t.Fatalf("read credential file: %v", err)
	}
	credOutput := string(credBytes)

	newPassword := matchFirstGroup(t, credOutput, `新密码:\s+(\S+)`)
	newRecoveryKey := matchFirstGroup(t, credOutput, `恢复密钥:\s+(\S+)`)
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
