package handler

import (
	"fmt"
	"net/http"
	"opensync/internal/model"
	"opensync/internal/msg"
	"opensync/internal/service"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// StreamJobCurrent handles GET /svr/job/stream as Server-Sent Events.
func StreamJobCurrent(c *gin.Context) {
	idStr := c.Query("id")
	if idStr == "" {
		c.JSON(http.StatusOK, model.Error(msg.LostPart))
		return
	}
	jobID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || jobID <= 0 {
		c.JSON(http.StatusOK, model.Error(msg.LostPart))
		return
	}

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, model.Error("streaming unsupported"))
		return
	}

	updates := service.SubscribeJobProgress(jobID)
	if updates == nil {
		c.JSON(http.StatusTooManyRequests, model.Error("too many progress streams"))
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	defer service.UnsubscribeJobProgress(jobID, updates)

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	writeEvent := func(payload []byte) bool {
		if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", payload); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	service.TouchJobWatching(jobID)
	if payload, err := service.BuildJobProgressStreamPayload(jobID); err == nil {
		if !writeEvent(payload) {
			return
		}
	}

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case payload, ok := <-updates:
			if !ok {
				return
			}
			service.TouchJobWatching(jobID)
			if !writeEvent(payload) {
				return
			}
		case <-heartbeat.C:
			service.TouchJobWatching(jobID)
			if _, err := fmt.Fprintf(c.Writer, ": heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
