package middleware

import (
	"net/http"
	"opensync/internal/model"
	"strings"

	"github.com/gin-gonic/gin"
)

// NoAuthCSRFProtection applies same-origin checks to unauthenticated state-changing routes.
func NoAuthCSRFProtection() gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
			c.Next()
			return
		}
		if !requestOriginAllowed(c) {
			c.JSON(http.StatusForbidden, model.Error("invalid request origin"))
			c.Abort()
			return
		}
		c.Next()
	}
}

func requestOriginAllowed(c *gin.Context) bool {
	if origin := strings.TrimSpace(c.GetHeader("Origin")); origin != "" {
		return sameOrigin(c, origin)
	}
	if referer := strings.TrimSpace(c.GetHeader("Referer")); referer != "" {
		return sameOrigin(c, referer)
	}
	return strings.EqualFold(c.GetHeader("X-Requested-With"), "XMLHttpRequest")
}
