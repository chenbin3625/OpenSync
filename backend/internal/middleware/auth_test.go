package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
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
