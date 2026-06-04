package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"opensync/internal/config"
	"opensync/internal/i18n"
	"opensync/internal/mapper"
	"opensync/internal/model"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/securecookie"
)

const cookieName = "open_sync"

var sc *securecookie.SecureCookie

// InitSecureCookie initializes the secure cookie encoder
func InitSecureCookie() {
	cfg := config.GetConfig()
	hashKey := deriveCookieKey(cfg.Server.PasswdStr, "hash", 32)
	blockKey := deriveCookieKey(cfg.Server.PasswdStr, "block", 16)
	sc = securecookie.New(hashKey, blockKey)
}

// CookieUser represents the user data stored in cookie
type CookieUser struct {
	ID       int64  `json:"id"`
	UserName string `json:"userName"`
	Session  string `json:"session"`
}

func deriveCookieKey(secret, label string, size int) []byte {
	sum := sha256.Sum256([]byte(label + ":" + secret))
	return sum[:size]
}

func authSessionSignature(user map[string]interface{}) string {
	cfg := config.GetConfig()
	mac := hmac.New(sha256.New, []byte(cfg.Server.PasswdStr))
	_, _ = mac.Write([]byte(fmt.Sprintf("%d|%s|%s",
		toInt64(user["id"]),
		fmt.Sprintf("%v", user["userName"]),
		fmt.Sprintf("%v", user["passwd"]),
	)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// NewCookieUser builds the non-sensitive auth payload for a user record.
func NewCookieUser(user map[string]interface{}) CookieUser {
	return CookieUser{
		ID:       toInt64(user["id"]),
		UserName: fmt.Sprintf("%v", user["userName"]),
		Session:  authSessionSignature(user),
	}
}

// CookieUserMatches verifies a cookie payload against the current user record.
func CookieUserMatches(cookieUser CookieUser, user map[string]interface{}) bool {
	if cookieUser.ID != toInt64(user["id"]) ||
		cookieUser.UserName != fmt.Sprintf("%v", user["userName"]) {
		return false
	}
	want := authSessionSignature(user)
	return hmac.Equal([]byte(cookieUser.Session), []byte(want))
}

// SetAuthCookie sets the signed auth cookie
func SetAuthCookie(c *gin.Context, user map[string]interface{}) {
	cfg := config.GetConfig()
	cookieData := NewCookieUser(user)
	jsonData, _ := json.Marshal(cookieData)
	encoded, err := sc.Encode(cookieName, string(jsonData))
	if err != nil {
		log.Printf("Failed to encode cookie: %v", err)
		return
	}

	expires := time.Now().Add(time.Duration(cfg.Server.Expires) * 24 * time.Hour)
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     cookieName,
		Value:    encoded,
		Path:     "/",
		MaxAge:   cfg.Server.Expires * 86400,
		Expires:  expires,
		HttpOnly: true,
		Secure:   isSecureRequest(c),
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearAuthCookie removes the auth cookie
func ClearAuthCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isSecureRequest(c),
		SameSite: http.SameSiteLaxMode,
	})
}

func isSecureRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	if c.Request.TLS != nil {
		return true
	}
	if strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
		return true
	}
	return strings.EqualFold(c.GetHeader("X-Forwarded-Ssl"), "on")
}

// AuthRequired is the Gin middleware for authentication
func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		// Skip auth for login APIs and embedded frontend assets.
		if strings.HasPrefix(path, "/svr/noAuth") ||
			path == "/" ||
			strings.HasPrefix(path, "/assets/") ||
			path == "/favicon.svg" ||
			path == "/icons.svg" {
			c.Next()
			return
		}

		cookieVal, err := c.Cookie(cookieName)
		if err != nil || cookieVal == "" {
			ClearAuthCookie(c)
			c.JSON(http.StatusOK, model.Unauthorized(i18n.G("sign_in")))
			c.Abort()
			return
		}

		var decoded string
		if err := sc.Decode(cookieName, cookieVal, &decoded); err != nil {
			ClearAuthCookie(c)
			c.JSON(http.StatusOK, model.Unauthorized(i18n.G("login_expired")))
			c.Abort()
			return
		}

		var cUser CookieUser
		if err := json.Unmarshal([]byte(decoded), &cUser); err != nil {
			ClearAuthCookie(c)
			c.JSON(http.StatusOK, model.Unauthorized(i18n.G("login_expired")))
			c.Abort()
			return
		}

		// Verify user still exists and matches
		trueUser, err := mapper.GetUserByID(cUser.ID)
		if err != nil {
			ClearAuthCookie(c)
			c.JSON(http.StatusOK, model.Unauthorized(i18n.G("login_expired")))
			c.Abort()
			return
		}

		if !CookieUserMatches(cUser, trueUser) {
			ClearAuthCookie(c)
			c.JSON(http.StatusOK, model.Unauthorized(i18n.G("login_expired")))
			c.Abort()
			return
		}

		// Store user info in context (without passwd/sqlVersion)
		c.Set("user", map[string]interface{}{
			"id":         toInt64(trueUser["id"]),
			"userName":   fmt.Sprintf("%v", trueUser["userName"]),
			"createTime": trueUser["createTime"],
		})
		c.Set("userFull", trueUser)

		c.Next()
	}
}

func toInt64(v interface{}) int64 {
	switch val := v.(type) {
	case int64:
		return val
	case int:
		return int64(val)
	case float64:
		return int64(val)
	default:
		return 0
	}
}
