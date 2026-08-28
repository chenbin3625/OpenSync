package handler

import (
	"fmt"
	"net/http"
	"opensync/internal/model"
	"opensync/internal/msg"
	"opensync/internal/service"

	"time"

	"github.com/gin-gonic/gin"
)

// StreamJobCurrent handles GET /svr/job/stream as Server-Sent Events.
func StreamJobCurrent(c *gin.Context) {
	jobID, err := parseRequiredID(c.Query("id"), "id")
	if err != nil {
		c.JSON(http.StatusOK, model.Error(msg.LostPart))
		return
	}

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, model.Error("streaming unsupported"))
		return
	}

	// Validate the job and build the first frame BEFORE switching the response
	// to SSE, so an unknown job still returns a normal JSON error instead of a
	// JSON body glued onto an event-stream response.
	initialPayload, err := service.BuildJobProgressStreamPayload(jobID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.Error("failed to build progress payload"))
		return
	}

	updates := service.SubscribeJobProgress(jobID)
	if updates == nil {
		c.JSON(http.StatusTooManyRequests, model.Error("too many progress streams"))
		return
	}
	defer service.UnsubscribeJobProgress(jobID, updates)

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

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
	if !writeEvent(initialPayload) {
		return
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
