package handler

import (
	"net/http"
	"strconv"
	"taosync/internal/i18n"
	"taosync/internal/model"
	"taosync/internal/service"

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
		notifyID, _ := strconv.ParseInt(toStr(notifyIDStr), 10, 64)
		enable := toIntI(req["enable"])
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
	notifyID, _ := strconv.ParseInt(notifyIDStr, 10, 64)
	service.DeleteNotify(notifyID)
	c.JSON(http.StatusOK, model.Success(nil))
}

func toStr(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(val, 10)
	default:
		return ""
	}
}

func toIntI(v interface{}) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	default:
		return 0
	}
}
