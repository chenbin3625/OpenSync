package handler

import (
	"errors"
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

	if c.Request.Context().Err() != nil {
		return
	}

	// Validate the job, subscribe, and build the first frame atomically BEFORE
	// switching the response to SSE, so an unknown job still returns a normal
	// JSON error and no terminal update can slip between snapshot and subscribe.
	updates, initialPayload, err := service.SubscribeJobProgressWithSnapshot(jobID)
	if err != nil {
		if errors.Is(err, service.ErrTooManyProgressStreams) {
			c.JSON(http.StatusTooManyRequests, model.Error("too many progress streams"))
			return
		}
		c.JSON(http.StatusInternalServerError, model.Error("failed to build progress payload"))
		return
	}
	defer service.UnsubscribeJobProgress(jobID, updates)

	if c.Request.Context().Err() != nil {
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)
	flusher.Flush()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	ctx := c.Request.Context()
	writeEvent := func(payload []byte) bool {
		if ctx.Err() != nil {
			return false
		}
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
		case <-ctx.Done():
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
			if ctx.Err() != nil {
				return
			}
			service.TouchJobWatching(jobID)
			if _, err := fmt.Fprintf(c.Writer, ": heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
