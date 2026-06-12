package main

import (
	"bytes"
	"context"
	"database/sql"
	"net"
	"net/http"
	"net/http/httptest"
	"opensync/internal/config"
	"opensync/internal/mapper"
	"opensync/internal/model"
	"opensync/pkg/crypto"
	"regexp"
	"strings"
	"testing"

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
