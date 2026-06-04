package handler

import (
	"net/http"
	"strconv"
	"taosync/internal/i18n"
	"taosync/internal/model"
	"taosync/internal/service"

	"github.com/gin-gonic/gin"
)

// GetJob handles GET /svr/job
func GetJob(c *gin.Context) {
	idStr := c.Query("id")
	taskIDStr := c.Query("taskId")

	if idStr != "" {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusOK, model.Error(i18n.G("lost_part")))
			return
		}
		// Check for current (real-time progress)
		current := c.Query("current")
		if current != "" {
			status := c.Query("status")
			var statusPtr *string
			if status != "" {
				statusPtr = &status
			}
			result := service.GetJobCurrent(id, statusPtr)
			c.JSON(http.StatusOK, model.Success(result))
			return
		}
		// Task list for this job
		req := map[string]interface{}{
			"id":       id,
			"pageSize": c.Query("pageSize"),
			"pageNum":  c.Query("pageNum"),
		}
		result := service.GetTaskList(req)
		c.JSON(http.StatusOK, model.Success(result))
		return
	}

	if taskIDStr != "" {
		// Task item list
		req := map[string]interface{}{
			"taskId":   taskIDStr,
			"pageSize": c.Query("pageSize"),
			"pageNum":  c.Query("pageNum"),
			"status":   c.Query("status"),
			"type":     c.Query("type"),
		}
		// Remove empty params
		for k, v := range req {
			if v == "" {
				delete(req, k)
			}
		}
		result := service.GetTaskItemList(req)
		c.JSON(http.StatusOK, model.Success(result))
		return
	}

	// Job list (paginated)
	req := map[string]interface{}{
		"pageSize": c.Query("pageSize"),
		"pageNum":  c.Query("pageNum"),
	}
	for k, v := range req {
		if v == "" {
			delete(req, k)
		}
	}
	result := service.GetJobList(req)
	c.JSON(http.StatusOK, model.Success(result))
}

// AddJob handles POST /svr/job
func AddJob(c *gin.Context) {
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, model.Error(i18n.G("lost_part")))
		return
	}
	// Check if it's an edit (has 'id') or add
	if _, hasID := req["id"]; hasID {
		service.EditJobClient(req)
	} else {
		service.AddJobClient(req, false)
	}
	c.JSON(http.StatusOK, model.Success(nil))
}

// UpdateJob handles PUT /svr/job
func UpdateJob(c *gin.Context) {
	var req struct {
		ID    *string `json:"id"`
		Pause *bool   `json:"pause"`
		Abort *bool   `json:"abort"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, model.Error(i18n.G("lost_part")))
		return
	}

	if req.Pause == nil {
		// Manual execution
		if req.ID != nil {
			id, err := parseRequiredID(*req.ID, "id")
			if err != nil {
				c.JSON(http.StatusOK, model.Error(err.Error()))
				return
			}
			service.DoJobManual(id)
		} else {
			service.DoAllJobManual()
		}
	} else if *req.Pause {
		// Disable or abort
		if req.ID == nil {
			c.JSON(http.StatusOK, model.Error(i18n.G("lost_part")))
			return
		}
		id, err := parseRequiredID(*req.ID, "id")
		if err != nil {
			c.JSON(http.StatusOK, model.Error(err.Error()))
			return
		}
		if req.Abort != nil && *req.Abort {
			service.AbortJob(id)
		} else {
			service.PauseJob(id)
		}
	} else {
		// Enable
		if req.ID == nil {
			c.JSON(http.StatusOK, model.Error(i18n.G("lost_part")))
			return
		}
		id, err := parseRequiredID(*req.ID, "id")
		if err != nil {
			c.JSON(http.StatusOK, model.Error(err.Error()))
			return
		}
		service.ContinueJob(id)
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
		c.JSON(http.StatusOK, model.Error(i18n.G("lost_part")))
		return
	}
	c.JSON(http.StatusOK, model.Success(nil))
}
