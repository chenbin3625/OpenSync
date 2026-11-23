package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"opensync/internal/config"
	"opensync/internal/mapper"
	"opensync/internal/service"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func setupJobStreamTestJob(t *testing.T) int64 {
	t.Helper()
	gin.SetMode(gin.TestMode)

	oldConfig := config.GetConfig()
	config.SetConfigForTest(&config.Config{
		DB: config.DBConfig{DBName: filepath.Join(t.TempDir(), "opensync-stream.db")},
		Server: config.ServerConfig{
			Timeout:         0,
			CopyConcurrency: 1,
			ScanConcurrency: 1,
			PasswdStr:       "test-secret",
		},
	})
	t.Cleanup(func() {
		config.SetConfigForTest(oldConfig)
		mapper.CloseDB()
	})

	mapper.CloseDB()
	mapper.InitSQL()

	jobID, err := mapper.AddJob(map[string]interface{}{
		"enable":        1,
		"remark":        "stream-test",
		"srcPath":       `["/src"]`,
		"dstPath":       `["/dst"]`,
		"alistId":       1,
		"useCacheT":     0,
		"scanIntervalT": 0,
		"useCacheS":     0,
		"scanIntervalS": 0,
		"method":        0,
		"interval":      60,
		"isCron":        2,
		"minFileSize":   0,
		"maxFileSize":   0,
	})
	if err != nil {
		t.Fatalf("mapper.AddJob() error: %v", err)
	}
	t.Cleanup(func() {
		defer func() { _ = recover() }()
		service.RemoveJobClient(jobID)
	})
	return jobID
}

func TestStreamJobCurrentEmitsSSEHeadersInitialFrameAndExitsOnCancel(t *testing.T) {
	jobID := setupJobStreamTestJob(t)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/svr/job/stream?id="+strconv.FormatInt(jobID, 10), nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	done := make(chan struct{})
	go func() {
		defer close(done)
		StreamJobCurrent(c)
	}()

	time.Sleep(40 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("StreamJobCurrent did not exit after request context canceled")
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}
	body := rec.Body.String()
	if !strings.HasPrefix(body, "data: ") || !strings.Contains(body, `"data":null`) {
		t.Fatalf("SSE body = %q, want initial null event", body)
	}
}

func TestStreamJobCurrentReturns429WhenMaxSubscribersReached(t *testing.T) {
	jobID := setupJobStreamTestJob(t)

	var subs []<-chan []byte
	for i := 0; i < 8; i++ {
		ch := service.SubscribeJobProgress(jobID)
		if ch == nil {
			t.Fatalf("SubscribeJobProgress(%d) returned nil unexpectedly", i)
		}
		subs = append(subs, ch)
	}
	defer func() {
		for _, ch := range subs {
			service.UnsubscribeJobProgress(jobID, ch)
		}
	}()

	req := httptest.NewRequest(http.MethodGet, "/svr/job/stream?id="+strconv.FormatInt(jobID, 10), nil)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	StreamJobCurrent(c)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 Too Many Requests", rec.Code)
	}
}
