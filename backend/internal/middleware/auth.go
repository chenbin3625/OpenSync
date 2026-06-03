package middleware

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"taosync/internal/config"
	"taosync/internal/i18n"
	"taosync/internal/mapper"
	"taosync/internal/model"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/securecookie"
)

const cookieName = "tao_sync"

var sc *securecookie.SecureCookie

// InitSecureCookie initializes the secure cookie encoder
func InitSecureCookie() {
	cfg := config.GetConfig()
	// Use first 32 bytes of secret key as hash key, first 16 as block key
	hashKey := []byte(cfg.Server.PasswdStr)
	if len(hashKey) > 32 {
		hashKey = hashKey[:32]
	}
	blockKey := []byte(cfg.Server.PasswdStr)
	if len(blockKey) > 16 {
		blockKey = blockKey[:16]
	}
	sc = securecookie.New(hashKey, blockKey)
}

// CookieUser represents the user data stored in cookie
type CookieUser struct {
	ID       int64  `json:"id"`
	UserName string `json:"userName"`
	Passwd   string `json:"passwd"`
}

// SetAuthCookie sets the signed auth cookie
func SetAuthCookie(c *gin.Context, user map[string]interface{}) {
	cfg := config.GetConfig()
	cookieData := CookieUser{
		ID:       toInt64(user["id"]),
		UserName: fmt.Sprintf("%v", user["userName"]),
		Passwd:   fmt.Sprintf("%v", user["passwd"]),
	}
	jsonData, _ := json.Marshal(cookieData)
	encoded, err := sc.Encode(cookieName, string(jsonData))
	if err != nil {
		log.Printf("Failed to encode cookie: %v", err)
		return
	}

	expires := time.Now().Add(time.Duration(cfg.Server.Expires) * 24 * time.Hour)
	c.SetCookie(cookieName, encoded, cfg.Server.Expires*86400, "/", "", false, false)
	c.SetSameSite(http.SameSiteLaxMode)
	// Also set with explicit expiry
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     cookieName,
		Value:    encoded,
		Path:     "/",
		Expires:  expires,
		HttpOnly: false,
	})
}

// ClearAuthCookie removes the auth cookie
func ClearAuthCookie(c *gin.Context) {
	c.SetCookie(cookieName, "", -1, "/", "", false, false)
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

		if fmt.Sprintf("%v", trueUser["passwd"]) != cUser.Passwd ||
			fmt.Sprintf("%v", trueUser["userName"]) != cUser.UserName {
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
