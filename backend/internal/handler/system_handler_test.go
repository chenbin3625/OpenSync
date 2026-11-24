package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

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
	req.RemoteAddr = "127.0.0.1:1234"
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

func TestInitializeRejectsLoopbackWithForwardingHeadersWhenTrustedProxiesEmpty(t *testing.T) {
	oldConfig := config.GetConfig()
	config.SetConfigForTest(&config.Config{
		Server: config.ServerConfig{
			AllowRemoteInit: false,
			TrustedProxies:  nil,
			PasswdStr:       "test-init-secret",
		},
	})
	t.Cleanup(func() { config.SetConfigForTest(oldConfig) })

	gin.SetMode(gin.TestMode)
	router := gin.New()
	_ = router.SetTrustedProxies(nil)
	router.POST("/svr/noAuth/init", Initialize)

	for _, tc := range []struct {
		header string
		value  string
	}{
		{"X-Forwarded-For", "203.0.113.9"},
		{"X-Real-IP", "203.0.113.9"},
		{"Forwarded", "for=203.0.113.9"},
	} {
		t.Run(tc.header, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/svr/noAuth/init", strings.NewReader(`{"userName":"admin","passwd":"correct-password"}`))
			req.RemoteAddr = "127.0.0.1:4321"
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(tc.header, tc.value)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusForbidden, w.Body.String())
			}
		})
	}
}

func TestEditPasswordBumpsSessionVersionAndReissuesCookie(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`CREATE TABLE user_list(
		id integer primary key autoincrement,
		userName text,
		passwd text,
		recoveryKey text,
		sessionVersion integer NOT NULL DEFAULT 1,
		sqlVersion integer,
		createTime integer DEFAULT 1
	)`); err != nil {
		t.Fatalf("create user_list: %v", err)
	}
	oldHash, err := crypto.HashPassword("old-password")
	if err != nil {
		t.Fatalf("HashPassword() error: %v", err)
	}
	if _, err := db.Exec("INSERT INTO user_list(id, userName, passwd, recoveryKey, sessionVersion) VALUES (1, 'admin', ?, 'rk', 1)", oldHash); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	restoreDB := mapper.SetDBForTest(db)
	t.Cleanup(restoreDB)
	oldConfig := config.GetConfig()
	config.SetConfigForTest(&config.Config{
		Server: config.ServerConfig{
			Expires:   7,
			PasswdStr: "test-edit-pwd-secret",
		},
	})
	t.Cleanup(func() {
		config.SetConfigForTest(oldConfig)
		middleware.InitSecureCookie()
	})
	middleware.InitSecureCookie()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.AuthRequired())
	router.PUT("/svr/user", EditPassword)
	router.GET("/svr/user", GetUser)

	// Mint initial session cookie.
	wInit := httptest.NewRecorder()
	initCtx, _ := gin.CreateTestContext(wInit)
	initCtx.Request = httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	if err := middleware.SetAuthCookie(initCtx, map[string]interface{}{
		"id":             int64(1),
		"userName":       "admin",
		"passwd":         oldHash,
		"sessionVersion": int64(1),
	}); err != nil {
		t.Fatalf("SetAuthCookie() error: %v", err)
	}
	oldCookie := wInit.Result().Cookies()[0]

	req := httptest.NewRequest(http.MethodPut, "/svr/user", strings.NewReader(`{"oldPasswd":"old-password","passwd":"brand-new-password"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(oldCookie)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"code":200`) {
		t.Fatalf("EditPassword status=%d body=%s, want 200 success", w.Code, w.Body.String())
	}

	var version int64
	if err := db.QueryRow("SELECT sessionVersion FROM user_list WHERE id=1").Scan(&version); err != nil {
		t.Fatalf("query sessionVersion: %v", err)
	}
	if version != 2 {
		t.Fatalf("sessionVersion = %d, want 2 after password edit", version)
	}

	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatalf("EditPassword did not re-issue fresh auth cookie")
	}
	freshCookie := cookies[len(cookies)-1]

	// Old cookie from another session must be rejected.
	wOld := httptest.NewRecorder()
	reqOld := httptest.NewRequest(http.MethodGet, "/svr/user", nil)
	reqOld.AddCookie(oldCookie)
	router.ServeHTTP(wOld, reqOld)
	if wOld.Code != http.StatusUnauthorized {
		t.Fatalf("old cookie status = %d, want %d", wOld.Code, http.StatusUnauthorized)
	}

	// Re-issued cookie must keep the current caller seamlessly authenticated.
	wFresh := httptest.NewRecorder()
	reqFresh := httptest.NewRequest(http.MethodGet, "/svr/user", nil)
	reqFresh.AddCookie(freshCookie)
	router.ServeHTTP(wFresh, reqFresh)
	if wFresh.Code != http.StatusOK {
		t.Fatalf("fresh cookie status = %d, want %d; body=%s", wFresh.Code, http.StatusOK, wFresh.Body.String())
	}
}

func TestUpdateSystemConfigPreservesOmittedFieldsAndRunsCleanupAsync(t *testing.T) {
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir() error: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if err := os.MkdirAll("data", 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}

	oldConfig := config.GetConfig()
	config.SetConfigForTest(&config.Config{
		Server: config.ServerConfig{
			Bind:            "127.0.0.1",
			Port:            8023,
			Expires:         7,
			Timeout:         48,
			TaskSave:        30,
			CopyConcurrency: 5,
			ScanConcurrency: 8,
			MaxRetries:      4,
			PasswdStr:       "test-config-secret",
		},
	})
	t.Cleanup(func() {
		config.SetConfigForTest(oldConfig)
		middleware.InitSecureCookie()
	})

	cleanupStarted := make(chan struct{})
	releaseCleanup := make(chan struct{})
	origCleanup := runTaskRetentionCleanup
	runTaskRetentionCleanup = func() {
		close(cleanupStarted)
		<-releaseCleanup
		panic("simulated cleanup panic")
	}
	t.Cleanup(func() {
		runTaskRetentionCleanup = origCleanup
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PUT("/svr/system/config", UpdateSystemConfig)

	// Send only expires; omitted zero-minimum fields (taskTimeout, taskSave, maxRetries)
	// must NOT be reset to 0, and HTTP response must NOT block on retention cleanup.
	req := httptest.NewRequest(http.MethodPut, "/svr/system/config", strings.NewReader(`{"expires":14}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		router.ServeHTTP(w, req)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		close(releaseCleanup)
		t.Fatal("UpdateSystemConfig blocked waiting for RunTaskRetentionCleanup")
	}

	select {
	case <-cleanupStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("async retention cleanup did not start")
	}
	close(releaseCleanup)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	got := config.GetSystemSettings()
	if got.Expires != 14 {
		t.Fatalf("Expires = %d, want 14", got.Expires)
	}
	if got.TaskTimeout != 48 || got.TaskSave != 30 || got.MaxRetries != 4 {
		t.Fatalf("omitted zero-min fields were overwritten: %+v", got)
	}
}

func TestUpdateSystemConfigConcurrentDisjointRequestsPreserveBoth(t *testing.T) {
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir() error: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if err := os.MkdirAll("data", 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}

	oldConfig := config.GetConfig()
	config.SetConfigForTest(&config.Config{
		Server: config.ServerConfig{
			Bind:            "127.0.0.1",
			Port:            8023,
			Expires:         7,
			Timeout:         48,
			TaskSave:        30,
			CopyConcurrency: 4,
			ScanConcurrency: 4,
			MaxRetries:      3,
			PasswdStr:       "test-config-secret",
		},
	})
	t.Cleanup(func() {
		config.SetConfigForTest(oldConfig)
		middleware.InitSecureCookie()
	})

	origCleanup := runTaskRetentionCleanup
	runTaskRetentionCleanup = func() {}
	t.Cleanup(func() { runTaskRetentionCleanup = origCleanup })

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PUT("/svr/system/config", UpdateSystemConfig)

	payloads := []string{
		`{"copyConcurrency":9}`,
		`{"taskSave":45}`,
		`{"scanConcurrency":12}`,
		`{"maxRetries":6}`,
		`{"taskTimeout":72}`,
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan string, len(payloads))
	for _, payload := range payloads {
		payload := payload
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			req := httptest.NewRequest(http.MethodPut, "/svr/system/config", strings.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"code":200`) {
				errs <- fmt.Sprintf("%s -> status=%d body=%s", payload, w.Code, w.Body.String())
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for msg := range errs {
		t.Fatal(msg)
	}

	got := config.GetSystemSettings()
	if got.CopyConcurrency != 9 || got.TaskSave != 45 || got.ScanConcurrency != 12 || got.MaxRetries != 6 || got.TaskTimeout != 72 || got.Expires != 7 {
		t.Fatalf("concurrent disjoint updates lost fields: got %+v", got)
	}
}

func TestModelSensitiveFieldsAreOmittedFromJSON(t *testing.T) {
	uBytes, err := json.Marshal(model.User{
		ID:             1,
		UserName:       "admin",
		Passwd:         "secret-bcrypt-hash",
		RecoveryKey:    "secret-recovery-hash",
		SessionVersion: 3,
		SQLVersion:     260615,
		CreateTime:     100,
	})
	if err != nil {
		t.Fatalf("Marshal(User) error: %v", err)
	}
	for _, secret := range []string{"secret-bcrypt-hash", "secret-recovery-hash", "Passwd", "RecoveryKey", "SessionVersion"} {
		if strings.Contains(string(uBytes), secret) {
			t.Fatalf("User JSON leaked %q: %s", secret, string(uBytes))
		}
	}

	aBytes, err := json.Marshal(model.Alist{
		ID:         1,
		Remark:     "main",
		URL:        "https://alist.example.test",
		UserName:   "admin",
		Token:      "alist-secret-token",
		CreateTime: 100,
	})
	if err != nil {
		t.Fatalf("Marshal(Alist) error: %v", err)
	}
	for _, secret := range []string{"alist-secret-token", "Token"} {
		if strings.Contains(string(aBytes), secret) {
			t.Fatalf("Alist JSON leaked %q: %s", secret, string(aBytes))
		}
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
