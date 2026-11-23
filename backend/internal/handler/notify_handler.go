package handler

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"opensync/internal/model"
	"opensync/internal/msg"
	"opensync/internal/service"
	"strings"

	"github.com/gin-gonic/gin"
)

func parseNotifyIDValue(value interface{}, field string) (int64, error) {
	switch v := value.(type) {
	case string:
		return parseRequiredID(strings.TrimSpace(v), field)
	case json.Number:
		return parseRequiredID(strings.TrimSpace(v.String()), field)
	case int:
		if v <= 0 {
			return 0, errors.New(msg.LostPart)
		}
		return int64(v), nil
	case int64:
		if v <= 0 {
			return 0, errors.New(msg.LostPart)
		}
		return v, nil
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 || math.Trunc(v) != v || v > float64(1<<53) {
			return 0, errors.New(msg.LostPart)
		}
		return int64(v), nil
	default:
		return 0, errors.New(msg.LostPart)
	}
}

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
	if err := c.ShouldBindJSON(&req); err != nil || req.Notify == nil || *req.Notify == nil {
		c.JSON(http.StatusOK, model.Error(msg.LostPart))
		return
	}

	notify := *req.Notify
	if _, hasEnable := notify["enable"]; !hasEnable {
		// A create request without `enable` is ambiguous; test sends belong on
		// POST /svr/notify/test so this cannot silently fire a real message.
		c.JSON(http.StatusOK, model.Error(msg.LostPart))
		return
	}
	service.AddNewNotify(notify)
	c.JSON(http.StatusOK, model.Success(nil))
}

// TestNotify handles POST /svr/notify/test
func TestNotify(c *gin.Context) {
	var req struct {
		Notify *map[string]interface{} `json:"notify"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Notify == nil || *req.Notify == nil {
		c.JSON(http.StatusOK, model.Error(msg.LostPart))
		return
	}
	service.TestNotify(*req.Notify)
	c.JSON(http.StatusOK, model.Success(nil))
}

// UpdateNotify handles PUT /svr/notify
func UpdateNotify(c *gin.Context) {
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil || req == nil {
		c.JSON(http.StatusOK, model.Error(msg.LostPart))
		return
	}

	if notifyIDRaw, ok := req["notifyId"]; ok {
		// Update status
		notifyID, err := parseNotifyIDValue(notifyIDRaw, "notifyId")
		if err != nil {
			c.JSON(http.StatusOK, model.Error(err.Error()))
			return
		}
		enableRaw, ok := req["enable"]
		if !ok {
			c.JSON(http.StatusOK, model.Error(msg.LostPart))
			return
		}
		enable, err := parseEnableValue(enableRaw)
		if err != nil {
			c.JSON(http.StatusOK, model.Error(err.Error()))
			return
		}
		service.UpdateNotifyStatus(notifyID, enable)
	} else if notify, ok := req["notify"]; ok {
		// Edit notify
		if nMap, ok := notify.(map[string]interface{}); ok && nMap != nil {
			service.EditNotify(nMap)
		} else {
			c.JSON(http.StatusOK, model.Error(msg.LostPart))
			return
		}
	} else {
		c.JSON(http.StatusOK, model.Error(msg.LostPart))
		return
	}
	c.JSON(http.StatusOK, model.Success(nil))
}

// DeleteNotify handles DELETE /svr/notify
func DeleteNotify(c *gin.Context) {
	notifyIDStr := c.Query("notifyId")
	if notifyIDStr == "" {
		c.JSON(http.StatusOK, model.Error(msg.LostPart))
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
