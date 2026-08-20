package middleware

import (
	"net/http"
	"net/url"
	"opensync/internal/model"
	"strings"

	"github.com/gin-gonic/gin"
)

// CSRFProtection requires a same-origin signal for state-changing requests
// made with the browser auth cookie. The X-Requested-With fallback supports
// non-browser clients while still requiring a non-simple header that a
// cross-site form cannot set.
func CSRFProtection() gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions || !hasAuthCookie(c) {
			c.Next()
			return
		}

		if origin := strings.TrimSpace(c.GetHeader("Origin")); origin != "" {
			if !sameOrigin(c, origin) {
				c.JSON(http.StatusForbidden, model.Error("invalid request origin"))
				c.Abort()
				return
			}
			c.Next()
			return
		}
		if referer := strings.TrimSpace(c.GetHeader("Referer")); referer != "" {
			if !sameOrigin(c, referer) {
				c.JSON(http.StatusForbidden, model.Error("invalid request origin"))
				c.Abort()
				return
			}
			c.Next()
			return
		}
		if strings.EqualFold(c.GetHeader("X-Requested-With"), "XMLHttpRequest") {
			c.Next()
			return
		}

		c.JSON(http.StatusForbidden, model.Error("request origin required"))
		c.Abort()
	}
}

func hasAuthCookie(c *gin.Context) bool {
	cookie, err := c.Request.Cookie(cookieName)
	return err == nil && cookie.Value != ""
}

func sameOrigin(c *gin.Context, raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil {
		return false
	}
	return strings.EqualFold(u.Host, c.Request.Host)
}
