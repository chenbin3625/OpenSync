package middleware

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"opensync/internal/config"
	"opensync/internal/mapper"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
)

func withAuthTestConfig(t *testing.T) {
	t.Helper()

	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error: %v", err)
	}
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir(%q) error: %v", tempDir, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldCwd)
	})

	if err := os.MkdirAll("data", 0755); err != nil {
		t.Fatalf("MkdirAll(data) error: %v", err)
	}
	InitSecureCookie()
}

func TestCookiePayloadOmitsPasswordHash(t *testing.T) {
	withAuthTestConfig(t)

	user := map[string]interface{}{
		"id":       int64(7),
		"userName": "admin",
		"passwd":   "stored-password-hash",
	}
	cookieUser := NewCookieUser(user)
	payload, err := json.Marshal(cookieUser)
	if err != nil {
		t.Fatalf("Marshal(cookieUser) error: %v", err)
	}

	if strings.Contains(string(payload), "stored-password-hash") {
		t.Fatalf("cookie payload leaked password hash: %s", payload)
	}
	if strings.Contains(string(payload), "passwd") {
		t.Fatalf("cookie payload contains passwd field: %s", payload)
	}
	if !CookieUserMatches(cookieUser, user) {
		t.Fatalf("CookieUserMatches() = false, want true for unchanged user")
	}

	changedUser := map[string]interface{}{
		"id":       int64(7),
		"userName": "admin",
		"passwd":   "changed-password-hash",
	}
	if CookieUserMatches(cookieUser, changedUser) {
		t.Fatalf("CookieUserMatches() = true, want false after password change")
	}
}

func TestSetAuthCookieUsesHttpOnlyCookie(t *testing.T) {
	withAuthTestConfig(t)
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "https://example.test/", nil)

	SetAuthCookie(c, map[string]interface{}{
		"id":       int64(7),
		"userName": "admin",
		"passwd":   "stored-password-hash",
	})

	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("SetAuthCookie() wrote %d cookies, want 1", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != cookieName {
		t.Fatalf("cookie name = %q, want %q", cookie.Name, cookieName)
	}
	if !cookie.HttpOnly {
		t.Fatalf("cookie HttpOnly = false, want true")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie SameSite = %v, want Lax", cookie.SameSite)
	}
	if !cookie.Secure {
		t.Fatalf("cookie Secure = false on HTTPS request, want true")
	}
}

func TestSetAuthCookieIgnoresForwardedProtoFromRemoteClient(t *testing.T) {
	withAuthTestConfig(t)
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	c.Request.RemoteAddr = "203.0.113.10:12345"
	c.Request.Header.Set("X-Forwarded-Proto", "https")

	SetAuthCookie(c, map[string]interface{}{
		"id":       int64(7),
		"userName": "admin",
		"passwd":   "stored-password-hash",
	})

	if cookie := w.Result().Cookies()[0]; cookie.Secure {
		t.Fatalf("cookie Secure = true for untrusted forwarded proto, want false")
	}
}

func TestSetAuthCookieTrustsForwardedProtoFromLoopbackProxy(t *testing.T) {
	withAuthTestConfig(t)
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	c.Request.RemoteAddr = "127.0.0.1:12345"
	c.Request.Header.Set("X-Forwarded-Proto", "https")

	SetAuthCookie(c, map[string]interface{}{
		"id":       int64(7),
		"userName": "admin",
		"passwd":   "stored-password-hash",
	})

	if cookie := w.Result().Cookies()[0]; !cookie.Secure {
		t.Fatalf("cookie Secure = false for loopback forwarded proto, want true")
	}
}

func TestAuthRequiredDoesNotExposeFullUserInContext(t *testing.T) {
	withAuthTestConfig(t)
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(AuthRequired())
	router.GET("/svr/user", func(c *gin.Context) {
		if _, ok := c.Get("userFull"); ok {
			t.Fatalf("userFull should not be stored in request context")
		}
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	cookieCtx, _ := gin.CreateTestContext(w)
	user := map[string]interface{}{
		"id":         int64(7),
		"userName":   "admin",
		"passwd":     "stored-password-hash",
		"createTime": int64(1),
	}
	cookieCtx.Request = httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	SetAuthCookie(cookieCtx, user)
	cookie := w.Result().Cookies()[0]

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE user_list(
		id integer primary key,
		userName text,
		passwd text,
		createTime integer
	)`); err != nil {
		t.Fatalf("create user_list: %v", err)
	}
	if _, err := db.Exec("INSERT INTO user_list(id, userName, passwd, createTime) VALUES (?, ?, ?, ?)",
		user["id"], user["userName"], user["passwd"], user["createTime"]); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	restoreDB := mapper.SetDBForTest(db)
	t.Cleanup(restoreDB)

	req := httptest.NewRequest(http.MethodGet, "/svr/user", nil)
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusNoContent, resp.Body.String())
	}
}

func TestInitSecureCookieUsesConfiguredMaxAge(t *testing.T) {
	oldConfig := config.GetConfig()
	config.SetConfigForTest(&config.Config{
		Server: config.ServerConfig{
			Expires:   2,
			PasswdStr: "test-cookie-secret",
		},
	})
	t.Cleanup(func() {
		config.SetConfigForTest(oldConfig)
		InitSecureCookie()
	})

	InitSecureCookie()

	got := reflect.ValueOf(sc).Elem().FieldByName("maxAge").Int()
	want := int64(2 * 86400)
	if got != want {
		t.Fatalf("secure cookie maxAge = %d, want %d", got, want)
	}
}

func TestAuthRequiredReturnsHTTP401WhenCookieMissing(t *testing.T) {
	withAuthTestConfig(t)
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(AuthRequired())
	router.GET("/svr/user", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/svr/user", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthRequiredAllowsFrontendRoutesWithoutCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(AuthRequired())
	router.GET("/engine", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/engine", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("frontend route status = %d, want %d; body=%s", w.Code, http.StatusNoContent, w.Body.String())
	}
}

func TestAuthRequiredReusesRecentlyVerifiedCookieUser(t *testing.T) {
	oldConfig := config.GetConfig()
	config.SetConfigForTest(&config.Config{
		Server: config.ServerConfig{
			Expires:   7,
			PasswdStr: "test-auth-cache-secret",
		},
	})
	t.Cleanup(func() {
		config.SetConfigForTest(oldConfig)
		InitSecureCookie()
	})
	InitSecureCookie()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	if _, err := db.Exec(`CREATE TABLE user_list(
		id integer primary key,
		userName text,
		passwd text,
		createTime integer
	)`); err != nil {
		t.Fatalf("create user_list: %v", err)
	}
	user := map[string]interface{}{
		"id":         int64(7),
		"userName":   "admin",
		"passwd":     "stored-password-hash",
		"createTime": int64(1),
	}
	if _, err := db.Exec("INSERT INTO user_list(id, userName, passwd, createTime) VALUES (?, ?, ?, ?)",
		user["id"], user["userName"], user["passwd"], user["createTime"]); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	restoreDB := mapper.SetDBForTest(db)
	t.Cleanup(restoreDB)

	cookieWriter := httptest.NewRecorder()
	cookieCtx, _ := gin.CreateTestContext(cookieWriter)
	cookieCtx.Request = httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	SetAuthCookie(cookieCtx, user)
	cookies := cookieWriter.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("SetAuthCookie() wrote %d cookies, want 1", len(cookies))
	}

	router := gin.New()
	router.Use(AuthRequired())
	router.GET("/svr/user", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	first := httptest.NewRecorder()
	firstReq := httptest.NewRequest(http.MethodGet, "/svr/user", nil)
	firstReq.AddCookie(cookies[0])
	router.ServeHTTP(first, firstReq)
	if first.Code != http.StatusNoContent {
		t.Fatalf("first authenticated status = %d, want %d; body=%s", first.Code, http.StatusNoContent, first.Body.String())
	}

	if _, err := db.Exec("DELETE FROM user_list WHERE id=?", user["id"]); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	second := httptest.NewRecorder()
	secondReq := httptest.NewRequest(http.MethodGet, "/svr/user", nil)
	secondReq.AddCookie(cookies[0])
	router.ServeHTTP(second, secondReq)
	if second.Code != http.StatusNoContent {
		t.Fatalf("second authenticated status = %d, want cached success %d; body=%s", second.Code, http.StatusNoContent, second.Body.String())
	}
}

func TestRevokeAuthSessionIgnoresStaleCookieAndRevokesCurrentCookie(t *testing.T) {
	withAuthTestConfig(t)
	gin.SetMode(gin.TestMode)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE user_list(
		id integer primary key,
		userName text,
		passwd text,
		sessionVersion integer NOT NULL DEFAULT 1,
		createTime integer DEFAULT 1
	)`); err != nil {
		t.Fatalf("create user_list: %v", err)
	}
	if _, err := db.Exec("INSERT INTO user_list(id, userName, passwd, sessionVersion) VALUES (7, 'admin', 'hash', 2)"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	restoreDB := mapper.SetDBForTest(db)
	t.Cleanup(restoreDB)

	mintCookie := func(sessionVersion int64) *http.Cookie {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
		if err := SetAuthCookie(c, map[string]interface{}{
			"id":             int64(7),
			"userName":       "admin",
			"passwd":         "hash",
			"sessionVersion": sessionVersion,
		}); err != nil {
			t.Fatalf("SetAuthCookie() error: %v", err)
		}
		return w.Result().Cookies()[0]
	}

	staleCookie := mintCookie(1)
	staleCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	staleCtx.Request = httptest.NewRequest(http.MethodDelete, "/svr/noAuth/login", nil)
	staleCtx.Request.AddCookie(staleCookie)
	if err := RevokeAuthSession(staleCtx); err != nil {
		t.Fatalf("RevokeAuthSession(stale) error: %v", err)
	}
	var version int64
	if err := db.QueryRow("SELECT sessionVersion FROM user_list WHERE id=7").Scan(&version); err != nil {
		t.Fatalf("query sessionVersion: %v", err)
	}
	if version != 2 {
		t.Fatalf("stale cookie bumped sessionVersion to %d, want unchanged 2", version)
	}

	activeCookie := mintCookie(2)
	activeCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	activeCtx.Request = httptest.NewRequest(http.MethodDelete, "/svr/noAuth/login", nil)
	activeCtx.Request.AddCookie(activeCookie)
	if err := RevokeAuthSession(activeCtx); err != nil {
		t.Fatalf("RevokeAuthSession(active) error: %v", err)
	}
	if err := db.QueryRow("SELECT sessionVersion FROM user_list WHERE id=7").Scan(&version); err != nil {
		t.Fatalf("query sessionVersion: %v", err)
	}
	if version != 3 {
		t.Fatalf("active cookie sessionVersion = %d, want 3", version)
	}
}

func TestAuthRequiredPreservesCookieOnTransientDBError(t *testing.T) {
	withAuthTestConfig(t)
	gin.SetMode(gin.TestMode)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	// Close DB immediately to trigger a non-ErrNoRows transient DB error.
	_ = db.Close()
	restoreDB := mapper.SetDBForTest(db)
	t.Cleanup(restoreDB)

	wCookie := httptest.NewRecorder()
	cookieCtx, _ := gin.CreateTestContext(wCookie)
	cookieCtx.Request = httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	if err := SetAuthCookie(cookieCtx, map[string]interface{}{
		"id":       int64(7),
		"userName": "admin",
		"passwd":   "hash",
	}); err != nil {
		t.Fatalf("SetAuthCookie() error: %v", err)
	}
	cookie := wCookie.Result().Cookies()[0]

	router := gin.New()
	router.Use(AuthRequired())
	router.GET("/svr/user", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/svr/user", nil)
	req.AddCookie(cookie)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusServiceUnavailable)
	}
	for _, c := range resp.Result().Cookies() {
		if c.Name == cookieName && c.MaxAge < 0 {
			t.Fatalf("transient DB error cleared auth cookie: %+v", c)
		}
	}
}

func TestAuthRequiredCSRFDefense(t *testing.T) {
	withAuthTestConfig(t)
	gin.SetMode(gin.TestMode)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE user_list(
		id integer primary key,
		userName text,
		passwd text,
		createTime integer
	)`); err != nil {
		t.Fatalf("create user_list: %v", err)
	}
	user := map[string]interface{}{
		"id":         int64(7),
		"userName":   "admin",
		"passwd":     "hash",
		"createTime": int64(1),
	}
	if _, err := db.Exec("INSERT INTO user_list(id, userName, passwd, createTime) VALUES (7, 'admin', 'hash', 1)"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	restoreDB := mapper.SetDBForTest(db)
	t.Cleanup(restoreDB)

	wCookie := httptest.NewRecorder()
	cookieCtx, _ := gin.CreateTestContext(wCookie)
	cookieCtx.Request = httptest.NewRequest(http.MethodGet, "https://example.test/", nil)
	_ = SetAuthCookie(cookieCtx, user)
	cookie := wCookie.Result().Cookies()[0]

	router := gin.New()
	router.Use(AuthRequired())
	router.GET("/svr/job", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.POST("/svr/job", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.PUT("/svr/job", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.DELETE("/svr/job", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	cases := []struct {
		name         string
		method       string
		host         string
		origin       string
		secFetchSite string
		wantCode     int
	}{
		{"cross-site Sec-Fetch-Site POST", http.MethodPost, "example.test", "", "cross-site", http.StatusForbidden},
		{"cross-origin Origin PUT", http.MethodPut, "example.test", "https://evil.example", "", http.StatusForbidden},
		{"null Origin DELETE", http.MethodDelete, "example.test", "null", "", http.StatusForbidden},
		{"same-origin Origin POST", http.MethodPost, "example.test", "https://example.test", "same-origin", http.StatusNoContent},
		{"same-origin default port match", http.MethodPost, "example.test:443", "https://example.test", "same-origin", http.StatusNoContent},
		{"no browser headers POST", http.MethodPost, "example.test", "", "", http.StatusNoContent},
		{"safe GET with cross-site header", http.MethodGet, "example.test", "", "cross-site", http.StatusNoContent},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "https://"+tc.host+"/svr/job", nil)
			req.Host = tc.host
			req.AddCookie(cookie)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.secFetchSite != "" {
				req.Header.Set("Sec-Fetch-Site", tc.secFetchSite)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d; body=%s", w.Code, tc.wantCode, w.Body.String())
			}
		})
	}
}
