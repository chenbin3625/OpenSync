package handler

import (
	"bytes"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"opensync/internal/config"
	"opensync/internal/mapper"
	"opensync/internal/middleware"
	"opensync/internal/model"
	"opensync/internal/msg"
	"opensync/internal/service"
	"strings"

	"github.com/gin-gonic/gin"
)

var runTaskRetentionCleanup = service.RunTaskRetentionCleanup

func hasForwardingHeaders(r *http.Request) bool {
	if r == nil {
		return false
	}
	for _, header := range []string{"X-Forwarded-For", "Forwarded", "X-Real-IP"} {
		if strings.TrimSpace(r.Header.Get(header)) != "" {
			return true
		}
	}
	return false
}

// Login handles POST /svr/noAuth/login
func Login(c *gin.Context) {
	var req struct {
		UserName string `json:"userName" form:"userName"`
		Passwd   string `json:"passwd" form:"passwd"`
	}
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusOK, model.Error(msg.LostPart))
		return
	}
	user := service.CheckPwdScoped(0, req.Passwd, req.UserName, c.ClientIP())
	if err := middleware.SetAuthCookie(c, user); err != nil {
		c.JSON(http.StatusOK, model.Error(msg.SetCookieFail))
		return
	}
	// Return user info without passwd and sqlVersion
	userReturn := map[string]interface{}{
		"id":         user["id"],
		"userName":   user["userName"],
		"createTime": user["createTime"],
	}
	c.JSON(http.StatusOK, model.Success(userReturn))
}

// GetInitStatus handles GET /svr/noAuth/init
func GetInitStatus(c *gin.Context) {
	c.JSON(http.StatusOK, model.Success(map[string]bool{
		"initialized": service.IsInitialized(),
	}))
}

// Initialize handles POST /svr/noAuth/init
func Initialize(c *gin.Context) {
	serverCfg := config.GetConfig().Server
	if !serverCfg.AllowRemoteInit {
		clientIP := net.ParseIP(strings.TrimSpace(c.ClientIP()))
		if clientIP == nil || !clientIP.IsLoopback() {
			c.JSON(http.StatusForbidden, model.Error(msg.InitialSetupLocalOnly))
			return
		}
		if len(serverCfg.TrustedProxies) == 0 && hasForwardingHeaders(c.Request) {
			c.JSON(http.StatusForbidden, model.Error(msg.InitialSetupLocalOnly))
			return
		}
	}
	var req struct {
		UserName string `json:"userName" form:"userName"`
		Passwd   string `json:"passwd" form:"passwd"`
	}
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusOK, model.Error(msg.LostPart))
		return
	}
	user, recoveryKey := service.InitializeUser(req.UserName, req.Passwd)
	if err := middleware.SetAuthCookie(c, user); err != nil {
		c.JSON(http.StatusOK, model.Error(msg.SetCookieFail))
		return
	}
	userReturn := map[string]interface{}{
		"id":          user["id"],
		"userName":    user["userName"],
		"createTime":  user["createTime"],
		"recoveryKey": recoveryKey,
	}
	c.JSON(http.StatusOK, model.Success(userReturn))
}

// ResetPassword handles PUT /svr/noAuth/login
func ResetPassword(c *gin.Context) {
	var req struct {
		UserName    string `json:"userName" form:"userName"`
		RecoveryKey string `json:"recoveryKey" form:"recoveryKey"`
		Passwd      string `json:"passwd" form:"passwd"`
	}
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusOK, model.Error(msg.LostPart))
		return
	}
	if strings.TrimSpace(req.UserName) == "" || strings.TrimSpace(req.RecoveryKey) == "" || strings.TrimSpace(req.Passwd) == "" {
		c.JSON(http.StatusOK, model.Error(msg.LostPart))
		return
	}
	result := service.ResetPasswd(req.UserName, req.RecoveryKey, req.Passwd)
	middleware.ClearAuthUserCache()
	c.JSON(http.StatusOK, model.Success(result))
}

// Logout handles DELETE /svr/noAuth/login
func Logout(c *gin.Context) {
	if err := middleware.RevokeAuthSession(c); err != nil {
		c.JSON(http.StatusInternalServerError, model.Error("退出登录失败，请重试"))
		return
	}
	middleware.ClearAuthCookie(c)
	c.JSON(http.StatusOK, model.Success(nil))
}

// GetUser handles GET /svr/user
func GetUser(c *gin.Context) {
	user, _ := c.Get("user")
	c.JSON(http.StatusOK, model.Success(user))
}

// EditPassword handles PUT /svr/user
func EditPassword(c *gin.Context) {
	var req struct {
		Passwd    string `json:"passwd" form:"passwd"`
		OldPasswd string `json:"oldPasswd" form:"oldPasswd"`
	}
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusOK, model.Error(msg.LostPart))
		return
	}
	user, _ := c.Get("user")
	userMap := user.(map[string]interface{})
	userID := userMap["id"].(int64)
	service.EditPasswd(userID, req.Passwd, req.OldPasswd)
	if err := mapper.BumpSessionVersion(userID); err != nil {
		middleware.ClearAuthUserCache()
		c.JSON(http.StatusInternalServerError, model.Error(msg.SetCookieFail))
		return
	}
	middleware.ClearAuthUserCache()
	updatedUser := service.GetUser(userID, "")
	if err := middleware.SetAuthCookie(c, updatedUser); err != nil {
		c.JSON(http.StatusOK, model.Error(msg.SetCookieFail))
		return
	}
	c.JSON(http.StatusOK, model.Success(nil))
}

// GetSystemConfig handles GET /svr/system/config
func GetSystemConfig(c *gin.Context) {
	c.JSON(http.StatusOK, model.Success(config.GetSystemSettings()))
}

type updateSystemConfigRequest struct {
	Expires         *int `json:"expires"`
	TaskTimeout     *int `json:"taskTimeout"`
	TaskSave        *int `json:"taskSave"`
	CopyConcurrency *int `json:"copyConcurrency"`
	ScanConcurrency *int `json:"scanConcurrency"`
	MaxRetries      *int `json:"maxRetries"`
}

// UpdateSystemConfig handles PUT /svr/system/config
func UpdateSystemConfig(c *gin.Context) {
	var raw json.RawMessage
	if err := c.ShouldBindJSON(&raw); err != nil {
		c.JSON(http.StatusOK, model.Error(msg.LostPart))
		return
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		c.JSON(http.StatusOK, model.Error(msg.LostPart))
		return
	}
	var req updateSystemConfigRequest
	if err := json.Unmarshal(trimmed, &req); err != nil {
		c.JSON(http.StatusOK, model.Error(msg.LostPart))
		return
	}
	updated, err := config.PatchSystemSettings(config.SystemSettingsPatch{
		Expires:         req.Expires,
		TaskTimeout:     req.TaskTimeout,
		TaskSave:        req.TaskSave,
		CopyConcurrency: req.CopyConcurrency,
		ScanConcurrency: req.ScanConcurrency,
		MaxRetries:      req.MaxRetries,
	})
	if err != nil {
		c.JSON(http.StatusOK, model.Error(err.Error()))
		return
	}
	middleware.InitSecureCookie()
	cleanupFn := runTaskRetentionCleanup
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("async task retention cleanup panic: %v", r)
			}
		}()
		cleanupFn()
	}()
	c.JSON(http.StatusOK, model.Success(updated))
}
