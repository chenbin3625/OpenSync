package service

import (
	"net/http"
	"net/http/httptest"
	"opensync/internal/config"
	"opensync/internal/mapper"
	"opensync/internal/msg"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func setupRetryFailedTaskTest(t *testing.T, enable int) (*JobClient, int64) {
	t.Helper()
	oldConfig := config.GetConfig()
	config.SetConfigForTest(&config.Config{
		DB: config.DBConfig{DBName: filepath.Join(t.TempDir(), "opensync-test.db")},
		Server: config.ServerConfig{
			Timeout:         0,
			CopyConcurrency: 1,
			ScanConcurrency: 1,
			PasswdStr:       "test-secret",
		},
	})
	t.Cleanup(func() {
		config.SetConfigForTest(oldConfig)
		resetJobClientsForTest()
		mapper.CloseDB()
	})

	mapper.CloseDB()
	mapper.InitSQL()
	resetJobClientsForTest()

	// AddJobClient validates that the engine exists before inserting the job.
	oldGetAlist := getAlistByID
	getAlistByID = func(alistID int64) (map[string]interface{}, error) {
		return map[string]interface{}{"id": alistID, "url": "https://alist.test", "token": "t"}, nil
	}
	t.Cleanup(func() { getAlistByID = oldGetAlist })

	AddJobClient(map[string]interface{}{
		"enable":        enable,
		"remark":        "retry-test",
		"srcPath":       []string{"/src"},
		"dstPath":       []string{"/dst"},
		"alistId":       1,
		"useCacheT":     0,
		"scanIntervalT": 0,
		"useCacheS":     0,
		"scanIntervalS": 0,
		"method":        0,
		"interval":      60,
		"isCron":        0,
		"minFileSize":   0,
		"maxFileSize":   0,
	}, false)
	client := onlyJobClientForTest(t)

	taskID, err := mapper.AddJobTask(client.JobID, time.Now().Unix())
	if err != nil {
		t.Fatalf("AddJobTask: %v", err)
	}
	return client, taskID
}

func addRetryTestItem(t *testing.T, taskID int64, name string, status int) {
	t.Helper()
	if err := mapper.AddJobTaskItemMany([]map[string]interface{}{
		{
			"taskId":     taskID,
			"srcPath":    "/src",
			"dstPath":    "/dst",
			"isPath":     0,
			"fileName":   name,
			"fileSize":   int64(1),
			"type":       0,
			"status":     status,
			"createTime": time.Now().Unix(),
		},
	}); err != nil {
		t.Fatalf("AddJobTaskItemMany: %v", err)
	}
}

func TestRetryFailedTaskPanicsWhenNoUnfinishedItems(t *testing.T) {
	_, taskID := setupRetryFailedTaskTest(t, 1)
	addRetryTestItem(t, taskID, "ok.txt", taskStatusSuccess.Int())

	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatalf("RetryFailedTask did not panic when there are no unfinished items")
		}
	}()
	RetryFailedTask(taskID)
}

func TestRetryFailedTaskPanicsWhenJobBusy(t *testing.T) {
	client, taskID := setupRetryFailedTaskTest(t, 1)
	addRetryTestItem(t, taskID, "bad.txt", taskStatusFailed.Int())
	if !client.tryMarkDoing() {
		t.Fatalf("tryMarkDoing() = false, want true")
	}
	t.Cleanup(func() { client.markDone() })

	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatalf("RetryFailedTask did not panic when job is busy")
		}
	}()
	RetryFailedTask(taskID)
}

func TestRetryFailedTaskPanicsWhenJobDisabled(t *testing.T) {
	_, taskID := setupRetryFailedTaskTest(t, 0)
	addRetryTestItem(t, taskID, "bad.txt", taskStatusFailed.Int())

	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatalf("RetryFailedTask did not panic when job is disabled")
		}
	}()
	RetryFailedTask(taskID)
}

func TestRetryFailedTaskBlockedAfterSwitchingJobEngine(t *testing.T) {
	defer resetAlistClientCacheForTest(t)()

	oldConfig := config.GetConfig()
	config.SetConfigForTest(&config.Config{
		DB: config.DBConfig{DBName: filepath.Join(t.TempDir(), "opensync-engine-switch.db")},
		Server: config.ServerConfig{
			Timeout:         0,
			CopyConcurrency: 1,
			ScanConcurrency: 1,
			PasswdStr:       "test-secret",
		},
	})
	t.Cleanup(func() {
		config.SetConfigForTest(oldConfig)
		resetJobClientsForTest()
		mapper.CloseDB()
	})

	mapper.CloseDB()
	mapper.InitSQL()
	resetJobClientsForTest()

	var engineBRequests atomic.Int32
	serverA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"username":"userA"}}`))
	}))
	defer serverA.Close()
	serverB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		engineBRequests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"username":"userB"}}`))
	}))
	defer serverB.Close()

	alistA, err := mapper.AddAlist("A", serverA.URL, "userA", "tokA")
	if err != nil {
		t.Fatalf("AddAlist(A): %v", err)
	}
	alistB, err := mapper.AddAlist("B", serverB.URL, "userB", "tokB")
	if err != nil {
		t.Fatalf("AddAlist(B): %v", err)
	}

	AddJobClient(map[string]interface{}{
		"enable":        1,
		"remark":        "mirror-job",
		"srcPath":       []string{"/src"},
		"dstPath":       []string{"/dst"},
		"alistId":       alistA,
		"useCacheT":     0,
		"scanIntervalT": 0,
		"useCacheS":     0,
		"scanIntervalS": 0,
		"method":        1,
		"interval":      60,
		"isCron":        2,
		"minFileSize":   0,
		"maxFileSize":   0,
	}, false)
	client := onlyJobClientForTest(t)

	// Task 1: modern task recorded with alistA.
	taskModern, err := mapper.AddJobTaskWithAlist(client.JobID, alistA, time.Now().Unix())
	if err != nil {
		t.Fatalf("AddJobTaskWithAlist: %v", err)
	}
	// Task 2: simulated pre-migration task with alistId = 0.
	taskLegacy, err := mapper.AddJobTaskWithAlist(client.JobID, 0, time.Now().Unix())
	if err != nil {
		t.Fatalf("AddJobTaskWithAlist(legacy): %v", err)
	}
	for _, tid := range []int64{taskModern, taskLegacy} {
		if err := mapper.AddJobTaskItemMany([]map[string]interface{}{{
			"taskId":     tid,
			"srcPath":    "",
			"dstPath":    "/dst/",
			"isPath":     0,
			"fileName":   "victim.txt",
			"fileSize":   int64(10),
			"type":       taskItemTypeDelete.Int(),
			"status":     taskStatusFailed.Int(),
			"createTime": time.Now().Unix(),
		}}); err != nil {
			t.Fatalf("AddJobTaskItemMany(%d): %v", tid, err)
		}
	}

	// Switch the job from Engine A to Engine B while keeping the same src/dst paths.
	EditJobClient(map[string]interface{}{
		"id":            client.JobID,
		"enable":        1,
		"remark":        "mirror-job",
		"srcPath":       []string{"/src"},
		"dstPath":       []string{"/dst"},
		"alistId":       alistB,
		"useCacheT":     0,
		"scanIntervalT": 0,
		"useCacheS":     0,
		"scanIntervalS": 0,
		"method":        1,
		"interval":      60,
		"isCron":        2,
		"minFileSize":   0,
		"maxFileSize":   0,
	})

	// Legacy task must have been backfilled with old engine A before switching.
	legacyRow, err := mapper.GetJobTaskByID(taskLegacy)
	if err != nil {
		t.Fatalf("GetJobTaskByID(legacy): %v", err)
	}
	if got := legacyRow["alistId"]; got != alistA {
		t.Fatalf("legacy task backfilled alistId = %v, want %d", got, alistA)
	}

	// Retrying either Engine A task must be rejected without touching Engine B.
	for _, tc := range []struct {
		name   string
		taskID int64
	}{
		{"modern task", taskModern},
		{"backfilled legacy task", taskLegacy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var recovered interface{}
			func() {
				defer func() { recovered = recover() }()
				RetryFailedTask(tc.taskID)
			}()
			if recovered == nil {
				t.Fatalf("RetryFailedTask(%d) did not panic after engine switch", tc.taskID)
			}
			if got := recovered.(error).Error(); got != msg.RetryTaskEngineChanged {
				t.Fatalf("RetryFailedTask(%d) panic = %q, want %q", tc.taskID, got, msg.RetryTaskEngineChanged)
			}
		})
	}
	if got := engineBRequests.Load(); got != 0 {
		t.Fatalf("engine B received %d requests, want 0", got)
	}
}
