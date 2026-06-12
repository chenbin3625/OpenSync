package handler

import (
	"net/http"
	"opensync/internal/i18n"
	"opensync/internal/model"
	"opensync/internal/service"
	"opensync/pkg/util"

	"github.com/gin-gonic/gin"
)

// GetNotify handles GET /svr/notify
func GetNotify(c *gin.Context) {
	result := service.GetNotifyList()
	c.JSON(http.StatusOK, model.Success(result))
}

// AddNotify handles POST /svr/notify
func AddNotify(c *gin.Context) {
	var req struct {
		Notify *map[string]interface{} `json:"notify"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Notify == nil {
		c.JSON(http.StatusOK, model.Error(i18n.G("lost_part")))
		return
	}

	notify := *req.Notify
	if _, hasEnable := notify["enable"]; hasEnable {
		service.AddNewNotify(notify)
	} else {
		service.TestNotify(notify)
	}
	c.JSON(http.StatusOK, model.Success(nil))
}

// UpdateNotify handles PUT /svr/notify
func UpdateNotify(c *gin.Context) {
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, model.Error(i18n.G("lost_part")))
		return
	}

	if notifyIDStr, ok := req["notifyId"]; ok {
		// Update status
		notifyID, err := parseRequiredID(util.StringValue(notifyIDStr), "notifyId")
		if err != nil {
			c.JSON(http.StatusOK, model.Error(err.Error()))
			return
		}
		enable := util.ToInt(req["enable"])
		service.UpdateNotifyStatus(notifyID, enable)
	} else if notify, ok := req["notify"]; ok {
		// Edit notify
		if nMap, ok := notify.(map[string]interface{}); ok {
			service.EditNotify(nMap)
		}
	}
	c.JSON(http.StatusOK, model.Success(nil))
}

// DeleteNotify handles DELETE /svr/notify
func DeleteNotify(c *gin.Context) {
	notifyIDStr := c.Query("notifyId")
	if notifyIDStr == "" {
		c.JSON(http.StatusOK, model.Error(i18n.G("lost_part")))
		return
	}
	notifyID, err := parseRequiredID(notifyIDStr, "notifyId")
	if err != nil {
		c.JSON(http.StatusOK, model.Error(err.Error()))
		return
	}
	service.DeleteNotify(notifyID)
	c.JSON(http.StatusOK, model.Success(nil))
}
