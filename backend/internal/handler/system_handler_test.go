package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"opensync/internal/config"
	"opensync/internal/mapper"
	"opensync/internal/middleware"
	"opensync/internal/model"
	"opensync/pkg/crypto"

	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
)

func TestLoginFailuresDoNotLockOutDifferentClientScope(t *testing.T) {
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
	hash, err := crypto.HashPassword("correct-password")
	if err != nil {
		t.Fatalf("HashPassword() error: %v", err)
	}
	recoveryHash, err := crypto.HashPassword("login-test-recovery-key")
	if err != nil {
		t.Fatalf("HashPassword(recovery) error: %v", err)
	}
	if _, err := db.Exec("INSERT INTO user_list(id, userName, passwd, recoveryKey) VALUES (1, 'admin', ?, ?)", hash, recoveryHash); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	restoreDB := mapper.SetDBForTest(db)
	t.Cleanup(restoreDB)
	oldConfig := config.GetConfig()
	config.SetConfigForTest(&config.Config{
		Server: config.ServerConfig{
			Expires:   7,
			PasswdStr: "test-login-secret",
		},
	})
	t.Cleanup(func() {
		config.SetConfigForTest(oldConfig)
	})
	middleware.InitSecureCookie()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		c.JSON(http.StatusInternalServerError, model.Error(fmt.Sprintf("%v", recovered)))
	}))
	router.POST("/svr/noAuth/login", Login)

	for i := 0; i < 4; i++ {
		w := performLogin(router, "192.0.2.10:1234", "admin", "bad-password")
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("bad login %d status = %d, want recovery status %d", i+1, w.Code, http.StatusInternalServerError)
		}
	}

	w := performLogin(router, "192.0.2.11:1234", "admin", "correct-password")
	if w.Code != http.StatusOK {
		t.Fatalf("valid login from another client status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"code":200`) {
		t.Fatalf("valid login response = %s, want success code", w.Body.String())
	}
}

func TestInitializeReturnsRecoveryKeyOnce(t *testing.T) {
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

	restoreDB := mapper.SetDBForTest(db)
	t.Cleanup(restoreDB)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/svr/noAuth/init", Initialize)

	body := `{"userName":"admin","passwd":"correct-password"}`
	req := httptest.NewRequest(http.MethodPost, "/svr/noAuth/init", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("initialize status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			UserName    string `json:"userName"`
			RecoveryKey string `json:"recoveryKey"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Code != 200 {
		t.Fatalf("response code = %d, want 200", resp.Code)
	}
	if resp.Data.UserName != "admin" {
		t.Fatalf("response userName = %q, want admin", resp.Data.UserName)
	}
	if len(resp.Data.RecoveryKey) != 24 {
		t.Fatalf("response recoveryKey length = %d, want 24", len(resp.Data.RecoveryKey))
	}

	var storedRecoveryHash string
	if err := db.QueryRow("SELECT recoveryKey FROM user_list WHERE userName='admin'").Scan(&storedRecoveryHash); err != nil {
		t.Fatalf("read recoveryKey hash: %v", err)
	}
	if storedRecoveryHash == resp.Data.RecoveryKey {
		t.Fatalf("stored recovery key was plaintext")
	}
	if !crypto.CheckPassword(resp.Data.RecoveryKey, storedRecoveryHash) {
		t.Fatalf("stored recovery key hash does not verify response recovery key")
	}
}

func TestResetPasswordUsesRecoveryKeyAndReturnsReplacement(t *testing.T) {
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
	passwordHash, err := crypto.HashPassword("old-password")
	if err != nil {
		t.Fatalf("HashPassword(password) error: %v", err)
	}
	recoveryHash, err := crypto.HashPassword("web-recovery-key")
	if err != nil {
		t.Fatalf("HashPassword(recovery) error: %v", err)
	}
	if _, err := db.Exec("INSERT INTO user_list(id, userName, passwd, recoveryKey) VALUES (1, 'admin', ?, ?)", passwordHash, recoveryHash); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	restoreDB := mapper.SetDBForTest(db)
	t.Cleanup(restoreDB)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PUT("/svr/noAuth/login", ResetPassword)

	body := `{"userName":"admin","recoveryKey":"web-recovery-key","passwd":"new-password"}`
	req := httptest.NewRequest(http.MethodPut, "/svr/noAuth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("reset status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp struct {
		Code int    `json:"code"`
		Data string `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Code != 200 {
		t.Fatalf("response code = %d, want 200", resp.Code)
	}
	if len(resp.Data) != 24 {
		t.Fatalf("new recovery key length = %d, want 24", len(resp.Data))
	}

	var storedPasswordHash, storedRecoveryHash string
	if err := db.QueryRow("SELECT passwd, recoveryKey FROM user_list WHERE id=1").Scan(&storedPasswordHash, &storedRecoveryHash); err != nil {
		t.Fatalf("read updated hashes: %v", err)
	}
	if !crypto.CheckPassword("new-password", storedPasswordHash) {
		t.Fatalf("stored password hash does not verify new password")
	}
	if !crypto.CheckPassword(resp.Data, storedRecoveryHash) {
		t.Fatalf("stored recovery hash does not verify response recovery key")
	}
	if crypto.CheckPassword("web-recovery-key", storedRecoveryHash) {
		t.Fatalf("old recovery key still verifies after reset")
	}
}

func TestResetPasswordRejectsOldSecretKeyField(t *testing.T) {
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
	passwordHash, err := crypto.HashPassword("old-password")
	if err != nil {
		t.Fatalf("HashPassword(password) error: %v", err)
	}
	recoveryHash, err := crypto.HashPassword("real-recovery-key")
	if err != nil {
		t.Fatalf("HashPassword(recovery) error: %v", err)
	}
	if _, err := db.Exec("INSERT INTO user_list(id, userName, passwd, recoveryKey) VALUES (1, 'admin', ?, ?)", passwordHash, recoveryHash); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	restoreDB := mapper.SetDBForTest(db)
	t.Cleanup(restoreDB)
	oldConfig := config.GetConfig()
	config.SetConfigForTest(&config.Config{
		Server: config.ServerConfig{PasswdStr: "data-secret-key"},
	})
	t.Cleanup(func() {
		config.SetConfigForTest(oldConfig)
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PUT("/svr/noAuth/login", ResetPassword)

	body := `{"userName":"admin","key":"data-secret-key","passwd":"new-password"}`
	req := httptest.NewRequest(http.MethodPut, "/svr/noAuth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("reset with legacy key field status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if strings.Contains(w.Body.String(), `"code":200`) {
		t.Fatalf("reset with legacy key field succeeded unexpectedly: %s", w.Body.String())
	}
	var storedPasswordHash string
	if err := db.QueryRow("SELECT passwd FROM user_list WHERE id=1").Scan(&storedPasswordHash); err != nil {
		t.Fatalf("read password hash: %v", err)
	}
	if !crypto.CheckPassword("old-password", storedPasswordHash) {
		t.Fatalf("password hash changed after legacy key reset attempt")
	}
}

func performLogin(router http.Handler, remoteAddr, userName, passwd string) *httptest.ResponseRecorder {
	body := fmt.Sprintf(`{"userName":%q,"passwd":%q}`, userName, passwd)
	req := httptest.NewRequest(http.MethodPost, "/svr/noAuth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remoteAddr
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}
