package handler

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"opensync/internal/model"
	"opensync/internal/msg"
	"opensync/internal/service"
	"strings"

	"github.com/gin-gonic/gin"
)

type flexibleID struct {
	set   bool
	valid bool
	value int64
}

func (f *flexibleID) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		f.set = false
		f.valid = false
		f.value = 0
		return nil
	}
	f.set = true
	f.valid = false
	f.value = 0

	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return nil
		}
		id, err := parseRequiredID(strings.TrimSpace(s), "id")
		if err != nil {
			return nil
		}
		f.valid = true
		f.value = id
		return nil
	}

	id, err := parseRequiredID(string(trimmed), "id")
	if err != nil {
		return nil
	}
	f.valid = true
	f.value = id
	return nil
}

func hasPresentJobID(raw interface{}) bool {
	if raw == nil {
		return false
	}
	switch v := raw.(type) {
	case string:
		s := strings.TrimSpace(v)
		return s != "" && s != "0"
	case float64:
		return !math.IsNaN(v) && !math.IsInf(v, 0) && v != 0
	case int:
		return v != 0
	case int64:
		return v != 0
	case json.Number:
		s := strings.TrimSpace(v.String())
		return s != "" && s != "0"
	default:
		return true
	}
}

// GetJob handles GET /svr/job
func GetJob(c *gin.Context) {
	idStr := c.Query("id")
	taskIDStr := c.Query("taskId")

	if idStr != "" {
		id, err := parseRequiredID(idStr, "id")
		if err != nil {
			c.JSON(http.StatusOK, model.Error(msg.LostPart))
			return
		}
		// Check for current (real-time progress)
		current := c.Query("current")
		if current != "" {
			req := map[string]interface{}{
				"status":   c.Query("status"),
				"pageSize": c.Query("pageSize"),
				"pageNum":  c.Query("pageNum"),
			}
			removeEmptyStringValues(req)
			result := service.GetJobCurrent(id, req)
			c.JSON(http.StatusOK, model.Success(result))
			return
		}
		// Task list for this job
		req := map[string]interface{}{
			"id":        id,
			"pageSize":  c.Query("pageSize"),
			"pageNum":   c.Query("pageNum"),
			"status":    c.Query("status"),
			"keyword":   c.Query("keyword"),
			"startTime": c.Query("startTime"),
			"endTime":   c.Query("endTime"),
		}
		if statusIn := c.QueryArray("statusIn"); len(statusIn) > 0 {
			req["statusIn"] = statusIn
		}
		removeEmptyStringValues(req)
		result := service.GetTaskList(req)
		c.JSON(http.StatusOK, model.Success(result))
		return
	}

	if taskIDStr != "" {
		taskID, err := parseRequiredID(taskIDStr, "taskId")
		if err != nil {
			c.JSON(http.StatusOK, model.Error(err.Error()))
			return
		}
		// Task item list
		req := map[string]interface{}{
			"taskId":   taskID,
			"pageSize": c.Query("pageSize"),
			"pageNum":  c.Query("pageNum"),
			"status":   c.Query("status"),
			"type":     c.Query("type"),
			"isPath":   c.Query("isPath"),
			"hasError": c.Query("hasError"),
			"keyword":  c.Query("keyword"),
		}
		// Remove empty params
		removeEmptyStringValues(req)
		ctx := c.Request.Context()
		result := service.GetTaskItemList(ctx, req)
		if ctx.Err() != nil {
			return
		}
		c.JSON(http.StatusOK, model.Success(result))
		return
	}

	// Job list (paginated)
	req := map[string]interface{}{
		"pageSize": c.Query("pageSize"),
		"pageNum":  c.Query("pageNum"),
	}
	removeEmptyStringValues(req)
	result := service.GetJobList(req)
	c.JSON(http.StatusOK, model.Success(result))
}

func removeEmptyStringValues(req map[string]interface{}) {
	for k, v := range req {
		if v == "" {
			delete(req, k)
		}
	}
}

// AddJob handles POST /svr/job
func AddJob(c *gin.Context) {
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil || req == nil {
		c.JSON(http.StatusOK, model.Error(msg.LostPart))
		return
	}
	// Check if it's an edit (has a non-empty, non-zero 'id') or add
	if hasPresentJobID(req["id"]) {
		service.EditJobClient(req)
	} else {
		delete(req, "id")
		service.AddJobClient(req, false)
	}
	c.JSON(http.StatusOK, model.Success(nil))
}

type updateJobRequest struct {
	ID     flexibleID `json:"id"`
	TaskID flexibleID `json:"taskId"`
	Action string     `json:"action"`
	Pause  *bool      `json:"pause"`
	Abort  *bool      `json:"abort"`
}

// UpdateJob handles PUT /svr/job
func UpdateJob(c *gin.Context) {
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
	var req updateJobRequest
	if err := json.Unmarshal(trimmed, &req); err != nil {
		c.JSON(http.StatusOK, model.Error(msg.LostPart))
		return
	}

	if req.TaskID.set {
		if !req.TaskID.valid {
			c.JSON(http.StatusOK, model.Error(msg.LostPart))
			return
		}
		switch req.Action {
		case "stop":
			service.StopTask(req.TaskID.value)
		case "retry":
			service.RetryFailedTask(req.TaskID.value)
		default:
			c.JSON(http.StatusOK, model.Error(msg.LostPart))
			return
		}
		c.JSON(http.StatusOK, model.Success(nil))
		return
	}

	if req.Pause == nil {
		// Manual execution
		if req.ID.set {
			if !req.ID.valid {
				c.JSON(http.StatusOK, model.Error(msg.LostPart))
				return
			}
			service.DoJobManual(req.ID.value)
		} else {
			service.DoAllJobManual()
		}
	} else if *req.Pause {
		// Disable or abort
		if !req.ID.set || !req.ID.valid {
			c.JSON(http.StatusOK, model.Error(msg.LostPart))
			return
		}
		if req.Abort != nil && *req.Abort {
			service.AbortJob(req.ID.value)
		} else {
			service.PauseJob(req.ID.value)
		}
	} else {
		// Enable
		if !req.ID.set || !req.ID.valid {
			c.JSON(http.StatusOK, model.Error(msg.LostPart))
			return
		}
		service.ContinueJob(req.ID.value)
	}
	c.JSON(http.StatusOK, model.Success(nil))
}

// DeleteJob handles DELETE /svr/job
func DeleteJob(c *gin.Context) {
	idStr := c.Query("id")
	taskIDStr := c.Query("taskId")

	if idStr != "" {
		id, err := parseRequiredID(idStr, "id")
		if err != nil {
			c.JSON(http.StatusOK, model.Error(err.Error()))
			return
		}
		service.RemoveJobClient(id)
	} else if taskIDStr != "" {
		taskID, err := parseRequiredID(taskIDStr, "taskId")
		if err != nil {
			c.JSON(http.StatusOK, model.Error(err.Error()))
			return
		}
		service.RemoveTask(taskID)
	} else {
		c.JSON(http.StatusOK, model.Error(msg.LostPart))
		return
	}
	c.JSON(http.StatusOK, model.Success(nil))
}
