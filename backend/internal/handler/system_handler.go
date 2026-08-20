package handler

import (
	"net/http"
	"opensync/internal/config"
	"opensync/internal/middleware"
	"opensync/internal/model"
	"opensync/internal/msg"
	"opensync/internal/service"
	"strings"

	"github.com/gin-gonic/gin"
)

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
	middleware.SetAuthCookie(c, user)
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
	if service.IsInitialized() {
		c.JSON(http.StatusOK, model.Success(map[string]bool{
			"initialized": true,
		}))
		return
	}
	token := strings.TrimSpace(c.Query("setupToken"))
	if token == "" {
		token = strings.TrimSpace(c.GetHeader("X-Setup-Token"))
	}
	if config.ValidateSetupToken(token) {
		c.JSON(http.StatusOK, model.Success(map[string]bool{
			"initialized": false,
		}))
		return
	}
	c.JSON(http.StatusOK, model.Success(map[string]interface{}{
		"initialized":          false,
		"setupTokenRequired": true,
	}))
}

// Initialize handles POST /svr/noAuth/init
func Initialize(c *gin.Context) {
	var req struct {
		UserName   string `json:"userName" form:"userName"`
		Passwd     string `json:"passwd" form:"passwd"`
		SetupToken string `json:"setupToken" form:"setupToken"`
	}
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusOK, model.Error(msg.LostPart))
		return
	}
	if !service.IsInitialized() {
		if !config.ValidateSetupToken(req.SetupToken) {
			c.JSON(http.StatusOK, model.Error(msg.SetupTokenInvalid))
			return
		}
	}
	user, recoveryKey := service.InitializeUser(req.UserName, req.Passwd)
	config.RemoveSetupToken()
	middleware.SetAuthCookie(c, user)
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
	middleware.ClearAuthUserCache()
	c.JSON(http.StatusOK, model.Success(nil))
}

// GetSystemConfig handles GET /svr/system/config
func GetSystemConfig(c *gin.Context) {
	c.JSON(http.StatusOK, model.Success(config.GetSystemSettings()))
}

// UpdateSystemConfig handles PUT /svr/system/config
func UpdateSystemConfig(c *gin.Context) {
	var req config.SystemSettings
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, model.Error(msg.LostPart))
		return
	}
	if err := config.UpdateSystemSettings(req); err != nil {
		c.JSON(http.StatusOK, model.Error(err.Error()))
		return
	}
	middleware.InitSecureCookie()
	service.RunTaskRetentionCleanup()
	c.JSON(http.StatusOK, model.Success(config.GetSystemSettings()))
}
