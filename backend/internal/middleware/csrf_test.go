package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCSRFProtectionRejectsCrossOriginCookieMutation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CSRFProtection())
	router.POST("/svr/job", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	req := httptest.NewRequest(http.MethodPost, "http://example.test/svr/job", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: "session"})
	req.Header.Set("Origin", "https://attacker.test")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestCSRFProtectionAllowsSameOriginCookieMutation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CSRFProtection())
	router.POST("/svr/job", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	req := httptest.NewRequest(http.MethodPost, "http://example.test/svr/job", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: "session"})
	req.Header.Set("Origin", "http://example.test")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
}
