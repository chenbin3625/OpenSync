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
