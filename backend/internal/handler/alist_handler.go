package handler

import (
	"net/http"
	"strconv"
	"opensync/internal/i18n"
	"opensync/internal/model"
	"opensync/internal/service"

	"github.com/gin-gonic/gin"
)

// GetAlist handles GET /svr/alist
func GetAlist(c *gin.Context) {
	// Check if it's a path select request
	alistIDStr := c.Query("alistId")
	path := c.Query("path")
	if alistIDStr != "" && path != "" {
		alistID, err := strconv.ParseInt(alistIDStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusOK, model.Error(i18n.G("lost_part")))
			return
		}
		result := service.GetChildPath(alistID, path)
		c.JSON(http.StatusOK, model.Success(result))
		return
	}
	// Return client list
	result := service.GetClientList()
	c.JSON(http.StatusOK, model.Success(result))
}

// AddAlist handles POST /svr/alist
func AddAlist(c *gin.Context) {
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, model.Error(i18n.G("lost_part")))
		return
	}
	service.AddClient(req)
	c.JSON(http.StatusOK, model.Success(nil))
}

// UpdateAlist handles PUT /svr/alist
func UpdateAlist(c *gin.Context) {
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, model.Error(i18n.G("lost_part")))
		return
	}
	service.UpdateClient(req)
	c.JSON(http.StatusOK, model.Success(nil))
}

// DeleteAlist handles DELETE /svr/alist
func DeleteAlist(c *gin.Context) {
	idStr := c.Query("id")
	if idStr == "" {
		// Try from body/form
		idStr = c.PostForm("id")
	}
	if idStr == "" {
		c.JSON(http.StatusOK, model.Error(i18n.G("lost_part")))
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, model.Error(i18n.G("lost_part")))
		return
	}
	service.RemoveClient(id)
	c.JSON(http.StatusOK, model.Success(nil))
}
