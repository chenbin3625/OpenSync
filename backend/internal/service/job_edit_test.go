package service

import (
	"opensync/internal/config"
	"opensync/internal/mapper"
	"path/filepath"
	"testing"
)

func TestEditEnabledJobClientUpdatesNextRunWithoutBreakingCurrentTask(t *testing.T) {
	oldConfig := config.GetConfig()
	config.SetConfigForTest(&config.Config{
		DB: config.DBConfig{DBName: filepath.Join(t.TempDir(), "opensync-test.db")},
		Server: config.ServerConfig{
			Timeout:               0,
			CopyConcurrency:       1,
			ScanConcurrency:       1,
			RealtimeFinishedItems: 100,
			PasswdStr:             "test-secret",
		},
	})
	defer config.SetConfigForTest(oldConfig)

	mapper.InitSQL()
	resetJobClientsForTest()

	AddJobClient(map[string]interface{}{
		"enable":        1,
		"remark":        "old",
		"srcPath":       []string{"/old-src"},
		"dstPath":       []string{"/old-dst"},
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
	defer resetJobClientsForTest()

	runningTask := &JobTask{TaskID: 99, Job: client.Job}
	runningTask.initRuntime()
	client.setCurrentTask(runningTask)
	if !client.tryMarkDoing() {
		t.Fatalf("tryMarkDoing() = false, want true")
	}
	defer func() {
		client.markDone()
		client.clearCurrentTask(runningTask)
	}()

	EditJobClient(map[string]interface{}{
		"id":            client.JobID,
		"enable":        1,
		"remark":        "edited",
		"srcPath":       []string{"/new-src"},
		"dstPath":       []string{"/new-dst"},
		"alistId":       1,
		"useCacheT":     0,
		"scanIntervalT": 0,
		"useCacheS":     0,
		"scanIntervalS": 0,
		"method":        0,
		"interval":      120,
		"isCron":        0,
		"minFileSize":   0,
		"maxFileSize":   0,
	})

	if runningTask.isBreak() {
		t.Fatalf("EditJobClient() requested break on the currently running task")
	}
	if got := client.currentTask(); got != runningTask {
		t.Fatalf("currentTask() = %#v, want existing running task", got)
	}
	if !client.isDoing() {
		t.Fatalf("client.isDoing() = false, want current task to remain running")
	}
	if got := client.Job["remark"]; got != "edited" {
		t.Fatalf("client.Job remark = %v, want edited", got)
	}
	if got := runningTask.Job["remark"]; got != "old" {
		t.Fatalf("running task job remark = %v, want old config for current run", got)
	}

	stored, err := mapper.GetJobByID(client.JobID)
	if err != nil {
		t.Fatalf("GetJobByID() error: %v", err)
	}
	if got := stored["remark"]; got != "edited" {
		t.Fatalf("stored job remark = %v, want edited", got)
	}
}

func resetJobClientsForTest() {
	jobClientListMu.Lock()
	for _, client := range jobClientList {
		if client.Scheduler != nil {
			client.Scheduler.Stop()
		}
	}
	jobClientList = make(map[int64]*JobClient)
	jobClientListMu.Unlock()
}

func onlyJobClientForTest(t *testing.T) *JobClient {
	t.Helper()
	jobClientListMu.RLock()
	defer jobClientListMu.RUnlock()
	if len(jobClientList) != 1 {
		t.Fatalf("jobClientList len = %d, want 1", len(jobClientList))
	}
	for _, client := range jobClientList {
		return client
	}
	return nil
}
