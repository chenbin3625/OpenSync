package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"opensync/internal/config"
	"opensync/internal/i18n"
	"opensync/internal/mapper"
	"opensync/internal/model"
	"opensync/pkg/util"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/securecookie"
)

const cookieName = "open_sync"

var sc *securecookie.SecureCookie
var scMu sync.RWMutex

const authUserCacheTTL = 15 * time.Second

type authUserCacheEntry struct {
	user      map[string]interface{}
	userFull  map[string]interface{}
	expiresAt time.Time
}

var (
	authUserCache   = make(map[string]authUserCacheEntry)
	authUserCacheMu sync.RWMutex
)

// InitSecureCookie initializes the secure cookie encoder
func InitSecureCookie() {
	cfg := config.GetConfig()
	codec := newSecureCookie(cfg)
	scMu.Lock()
	sc = codec
	scMu.Unlock()
	ClearAuthUserCache()
}

func newSecureCookie(cfg *config.Config) *securecookie.SecureCookie {
	hashKey := deriveCookieKey(cfg.Server.PasswdStr, "hash", 32)
	blockKey := deriveCookieKey(cfg.Server.PasswdStr, "block", 16)
	codec := securecookie.New(hashKey, blockKey)
	codec.MaxAge(cfg.Server.Expires * 86400)
	return codec
}

func currentSecureCookie() *securecookie.SecureCookie {
	scMu.RLock()
	codec := sc
	scMu.RUnlock()
	if codec != nil {
		return codec
	}
	InitSecureCookie()
	scMu.RLock()
	codec = sc
	scMu.RUnlock()
	return codec
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
		util.ToInt64(user["id"]),
		fmt.Sprintf("%v", user["userName"]),
		fmt.Sprintf("%v", user["passwd"]),
	)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// NewCookieUser builds the non-sensitive auth payload for a user record.
func NewCookieUser(user map[string]interface{}) CookieUser {
	return CookieUser{
		ID:       util.ToInt64(user["id"]),
		UserName: fmt.Sprintf("%v", user["userName"]),
		Session:  authSessionSignature(user),
	}
}

// CookieUserMatches verifies a cookie payload against the current user record.
func CookieUserMatches(cookieUser CookieUser, user map[string]interface{}) bool {
	if cookieUser.ID != util.ToInt64(user["id"]) ||
		cookieUser.UserName != fmt.Sprintf("%v", user["userName"]) {
		return false
	}
	want := authSessionSignature(user)
	return hmac.Equal([]byte(cookieUser.Session), []byte(want))
}

func authUserCacheKey(cookieUser CookieUser) string {
	return fmt.Sprintf("%d|%s|%s", cookieUser.ID, cookieUser.UserName, cookieUser.Session)
}

func copyAuthUserMap(src map[string]interface{}) map[string]interface{} {
	dst := make(map[string]interface{}, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func publicAuthUser(user map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"id":         util.ToInt64(user["id"]),
		"userName":   fmt.Sprintf("%v", user["userName"]),
		"createTime": user["createTime"],
	}
}

func cachedAuthUser(cookieUser CookieUser) (map[string]interface{}, map[string]interface{}, bool) {
	key := authUserCacheKey(cookieUser)
	now := time.Now()

	authUserCacheMu.RLock()
	entry, ok := authUserCache[key]
	authUserCacheMu.RUnlock()
	if !ok {
		return nil, nil, false
	}
	if now.After(entry.expiresAt) {
		authUserCacheMu.Lock()
		if current, ok := authUserCache[key]; ok && now.After(current.expiresAt) {
			delete(authUserCache, key)
		}
		authUserCacheMu.Unlock()
		return nil, nil, false
	}

	return copyAuthUserMap(entry.user), copyAuthUserMap(entry.userFull), true
}

func cacheAuthUser(cookieUser CookieUser, trueUser map[string]interface{}) {
	key := authUserCacheKey(cookieUser)
	authUserCacheMu.Lock()
	authUserCache[key] = authUserCacheEntry{
		user:      publicAuthUser(trueUser),
		userFull:  copyAuthUserMap(trueUser),
		expiresAt: time.Now().Add(authUserCacheTTL),
	}
	authUserCacheMu.Unlock()
}

func ClearAuthUserCache() {
	authUserCacheMu.Lock()
	authUserCache = make(map[string]authUserCacheEntry)
	authUserCacheMu.Unlock()
}

// SetAuthCookie sets the signed auth cookie
func SetAuthCookie(c *gin.Context, user map[string]interface{}) {
	cfg := config.GetConfig()
	cookieData := NewCookieUser(user)
	jsonData, _ := json.Marshal(cookieData)
	encoded, err := currentSecureCookie().Encode(cookieName, string(jsonData))
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
			c.JSON(http.StatusUnauthorized, model.Unauthorized(i18n.G("sign_in")))
			c.Abort()
			return
		}

		var decoded string
		if err := currentSecureCookie().Decode(cookieName, cookieVal, &decoded); err != nil {
			ClearAuthCookie(c)
			c.JSON(http.StatusUnauthorized, model.Unauthorized(i18n.G("login_expired")))
			c.Abort()
			return
		}

		var cUser CookieUser
		if err := json.Unmarshal([]byte(decoded), &cUser); err != nil {
			ClearAuthCookie(c)
			c.JSON(http.StatusUnauthorized, model.Unauthorized(i18n.G("login_expired")))
			c.Abort()
			return
		}

		if user, userFull, ok := cachedAuthUser(cUser); ok {
			c.Set("user", user)
			c.Set("userFull", userFull)
			c.Next()
			return
		}

		// Verify user still exists and matches
		trueUser, err := mapper.GetUserByID(cUser.ID)
		if err != nil {
			ClearAuthCookie(c)
			c.JSON(http.StatusUnauthorized, model.Unauthorized(i18n.G("login_expired")))
			c.Abort()
			return
		}

		if !CookieUserMatches(cUser, trueUser) {
			ClearAuthCookie(c)
			c.JSON(http.StatusUnauthorized, model.Unauthorized(i18n.G("login_expired")))
			c.Abort()
			return
		}

		cacheAuthUser(cUser, trueUser)
		// Store user info in context (without passwd/sqlVersion)
		c.Set("user", publicAuthUser(trueUser))
		c.Set("userFull", trueUser)

		c.Next()
	}
}
