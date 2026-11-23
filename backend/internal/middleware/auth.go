package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"opensync/internal/config"
	"opensync/internal/mapper"
	"opensync/internal/model"
	"opensync/internal/msg"
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

const (
	authUserCacheTTL        = 15 * time.Second
	maxAuthUserCacheEntries = 1024
)

type authUserCacheEntry struct {
	user      map[string]interface{}
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
	ID             int64  `json:"id"`
	UserName       string `json:"userName"`
	SessionVersion int64  `json:"sessionVersion"`
	Session        string `json:"session"`
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
	_, _ = mac.Write([]byte(fmt.Sprintf("|%d", userSessionVersion(user))))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func userSessionVersion(user map[string]interface{}) int64 {
	version := util.ToInt64(user["sessionVersion"])
	if version <= 0 {
		return 1
	}
	return version
}

// NewCookieUser builds the non-sensitive auth payload for a user record.
func NewCookieUser(user map[string]interface{}) CookieUser {
	return CookieUser{
		ID:             util.ToInt64(user["id"]),
		UserName:       fmt.Sprintf("%v", user["userName"]),
		SessionVersion: userSessionVersion(user),
		Session:        authSessionSignature(user),
	}
}

// CookieUserMatches verifies a cookie payload against the current user record.
func CookieUserMatches(cookieUser CookieUser, user map[string]interface{}) bool {
	if cookieUser.ID != util.ToInt64(user["id"]) ||
		cookieUser.UserName != fmt.Sprintf("%v", user["userName"]) ||
		cookieUser.SessionVersion != userSessionVersion(user) {
		return false
	}
	want := authSessionSignature(user)
	return hmac.Equal([]byte(cookieUser.Session), []byte(want))
}

// RevokeAuthSession invalidates the current user's cookies server-side. The
// cookie is decoded and verified against the current user record before bumping
// sessionVersion so stale or already-revoked cookies cannot force-logout active
// sessions.
func RevokeAuthSession(c *gin.Context) error {
	cookieVal, err := c.Cookie(cookieName)
	if err != nil || cookieVal == "" {
		return nil
	}
	var decoded string
	if err := currentSecureCookie().Decode(cookieName, cookieVal, &decoded); err != nil {
		return nil
	}
	var cookieUser CookieUser
	if err := json.Unmarshal([]byte(decoded), &cookieUser); err != nil || cookieUser.ID <= 0 {
		return nil
	}
	trueUser, err := mapper.GetUserByID(cookieUser.ID)
	if err != nil {
		if errors.Is(err, mapper.ErrUserNotFound) || errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if !CookieUserMatches(cookieUser, trueUser) {
		return nil
	}
	if err := mapper.BumpSessionVersion(cookieUser.ID); err != nil {
		return err
	}
	ClearAuthUserCache()
	return nil
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

func cachedAuthUser(cookieUser CookieUser) (map[string]interface{}, bool) {
	key := authUserCacheKey(cookieUser)
	now := time.Now()

	authUserCacheMu.RLock()
	entry, ok := authUserCache[key]
	authUserCacheMu.RUnlock()
	if !ok {
		return nil, false
	}
	if now.After(entry.expiresAt) {
		authUserCacheMu.Lock()
		if current, ok := authUserCache[key]; ok && now.After(current.expiresAt) {
			delete(authUserCache, key)
		}
		authUserCacheMu.Unlock()
		return nil, false
	}

	return copyAuthUserMap(entry.user), true
}

func cacheAuthUser(cookieUser CookieUser, trueUser map[string]interface{}) {
	key := authUserCacheKey(cookieUser)
	now := time.Now()
	authUserCacheMu.Lock()
	if len(authUserCache) >= maxAuthUserCacheEntries {
		for k, v := range authUserCache {
			if now.After(v.expiresAt) {
				delete(authUserCache, k)
			}
		}
		if len(authUserCache) >= maxAuthUserCacheEntries {
			count := 0
			for k := range authUserCache {
				delete(authUserCache, k)
				count++
				if count >= maxAuthUserCacheEntries/2 {
					break
				}
			}
		}
	}
	authUserCache[key] = authUserCacheEntry{
		user:      publicAuthUser(trueUser),
		expiresAt: now.Add(authUserCacheTTL),
	}
	authUserCacheMu.Unlock()
}

func ClearAuthUserCache() {
	authUserCacheMu.Lock()
	authUserCache = make(map[string]authUserCacheEntry)
	authUserCacheMu.Unlock()
}

// SetAuthCookie sets the signed auth cookie. An error is returned so callers
// can fail the login instead of responding with code 200 but no cookie, which
// leaves the client logged out on the very next request.
func SetAuthCookie(c *gin.Context, user map[string]interface{}) error {
	cfg := config.GetConfig()
	cookieData := NewCookieUser(user)
	jsonData, _ := json.Marshal(cookieData)
	encoded, err := currentSecureCookie().Encode(cookieName, string(jsonData))
	if err != nil {
		log.Printf("Failed to encode cookie: %v", err)
		return err
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
	return nil
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
	// Only honor X-Forwarded-Proto from loopback or explicitly configured
	// trusted proxies, otherwise a remote attacker could forge it and force the
	// cookie to be marked Secure. Operators behind a TLS-terminating reverse
	// proxy configure trusted_proxies so the Secure attribute is set correctly.
	if !config.GetConfig().Server.IsTrustedProxy(c.Request.RemoteAddr) {
		return false
	}
	if strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
		return true
	}
	return strings.EqualFold(c.GetHeader("X-Forwarded-Ssl"), "on")
}

func isStateChangingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch:
		return true
	default:
		return false
	}
}

func normalizeHostPort(hostPort, defaultScheme string) (string, string) {
	hostPort = strings.TrimSpace(hostPort)
	host, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		host = strings.Trim(hostPort, "[]")
		port = ""
	}
	host = strings.ToLower(host)
	if port == "" {
		switch strings.ToLower(defaultScheme) {
		case "https":
			port = "443"
		case "http":
			port = "80"
		}
	}
	return host, port
}

func checkSameOriginCSRF(r *http.Request) bool {
	if r == nil || !isStateChangingMethod(r.Method) {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")), "cross-site") {
		return false
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	if strings.EqualFold(origin, "null") {
		return false
	}
	originURL, err := url.Parse(origin)
	if err != nil || originURL.Host == "" {
		return false
	}
	if strings.EqualFold(originURL.Host, r.Host) {
		return true
	}
	originHost, originPort := normalizeHostPort(originURL.Host, originURL.Scheme)
	reqHost, reqPort := normalizeHostPort(r.Host, originURL.Scheme)
	return originHost != "" && originHost == reqHost && originPort == reqPort
}

// AuthRequired is the Gin middleware for authentication
func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		// Frontend routes are public entry points; authentication is enforced by
		// the API routes and the client-side guards. The noAuth match is anchored
		// so a hypothetical future /svr/noAuthXxx route cannot sneak past auth.
		if (path != "/svr" && !strings.HasPrefix(path, "/svr/")) ||
			path == "/svr/noAuth" || strings.HasPrefix(path, "/svr/noAuth/") {
			c.Next()
			return
		}

		if !checkSameOriginCSRF(c.Request) {
			c.JSON(http.StatusForbidden, model.Error("禁止跨站请求"))
			c.Abort()
			return
		}

		cookieVal, err := c.Cookie(cookieName)
		if err != nil || cookieVal == "" {
			ClearAuthCookie(c)
			c.JSON(http.StatusUnauthorized, model.Unauthorized(msg.SignIn))
			c.Abort()
			return
		}

		var decoded string
		if err := currentSecureCookie().Decode(cookieName, cookieVal, &decoded); err != nil {
			ClearAuthCookie(c)
			c.JSON(http.StatusUnauthorized, model.Unauthorized(msg.LoginExpired))
			c.Abort()
			return
		}

		var cUser CookieUser
		if err := json.Unmarshal([]byte(decoded), &cUser); err != nil {
			ClearAuthCookie(c)
			c.JSON(http.StatusUnauthorized, model.Unauthorized(msg.LoginExpired))
			c.Abort()
			return
		}

		if user, ok := cachedAuthUser(cUser); ok {
			c.Set("user", user)
			c.Next()
			return
		}

		// Verify user still exists and matches
		trueUser, err := mapper.GetUserByID(cUser.ID)
		if err != nil {
			if errors.Is(err, mapper.ErrUserNotFound) || errors.Is(err, sql.ErrNoRows) {
				ClearAuthCookie(c)
				c.JSON(http.StatusUnauthorized, model.Unauthorized(msg.LoginExpired))
				c.Abort()
				return
			}
			log.Printf("AuthRequired failed to query user %d: %v", cUser.ID, err)
			c.JSON(http.StatusServiceUnavailable, model.Error("服务暂时不可用，请稍后重试"))
			c.Abort()
			return
		}

		if !CookieUserMatches(cUser, trueUser) {
			ClearAuthCookie(c)
			c.JSON(http.StatusUnauthorized, model.Unauthorized(msg.LoginExpired))
			c.Abort()
			return
		}

		cacheAuthUser(cUser, trueUser)
		// Store user info in context (without passwd/sqlVersion)
		c.Set("user", publicAuthUser(trueUser))

		c.Next()
	}
}
