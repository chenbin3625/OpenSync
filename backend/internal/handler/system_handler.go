package handler

import (
	"net/http"
	"taosync/internal/i18n"
	"taosync/internal/middleware"
	"taosync/internal/model"
	"taosync/internal/service"

	"github.com/gin-gonic/gin"
)

// Login handles POST /svr/noAuth/login
func Login(c *gin.Context) {
	var req struct {
		UserName string `json:"userName" form:"userName"`
		Passwd   string `json:"passwd" form:"passwd"`
	}
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusOK, model.Error(i18n.G("lost_part")))
		return
	}
	user := service.CheckPwd(0, req.Passwd, req.UserName)
	middleware.SetAuthCookie(c, user)
	// Return user info without passwd and sqlVersion
	userReturn := map[string]interface{}{
		"id":         user["id"],
		"userName":   user["userName"],
		"createTime": user["createTime"],
	}
	c.JSON(http.StatusOK, model.Success(userReturn))
}

// ResetPassword handles PUT /svr/noAuth/login
func ResetPassword(c *gin.Context) {
	var req struct {
		UserName string `json:"userName" form:"userName"`
		Key      string `json:"key" form:"key"`
		Passwd   string `json:"passwd" form:"passwd"`
	}
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusOK, model.Error(i18n.G("lost_part")))
		return
	}
	result := service.ResetPasswd(req.UserName, req.Key, req.Passwd)
	if result != "" {
		c.JSON(http.StatusOK, model.Success(result))
	} else {
		c.JSON(http.StatusOK, model.Success(nil))
	}
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
		c.JSON(http.StatusOK, model.Error(i18n.G("lost_part")))
		return
	}
	user, _ := c.Get("user")
	userMap := user.(map[string]interface{})
	userID := userMap["id"].(int64)
	service.EditPasswd(userID, req.Passwd, req.OldPasswd)
	c.JSON(http.StatusOK, model.Success(nil))
}

// GetLanguage handles GET /svr/language
func GetLanguage(c *gin.Context) {
	c.JSON(http.StatusOK, model.Success(i18n.GetLanguage()))
}

// SetLanguage handles POST /svr/language
func SetLanguage(c *gin.Context) {
	var req struct {
		Language string `json:"language" form:"language"`
	}
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusOK, model.Error(i18n.G("lost_part")))
		return
	}
	if err := i18n.SetLanguage(req.Language); err != nil {
		c.JSON(http.StatusOK, model.Error(err.Error()))
		return
	}
	c.JSON(http.StatusOK, model.Success(nil))
}
