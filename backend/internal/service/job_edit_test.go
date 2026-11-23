package service

import (
	"opensync/internal/config"
	"opensync/internal/mapper"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestEditEnabledJobClientUpdatesNextRunWithoutBreakingCurrentTask(t *testing.T) {
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
	defer config.SetConfigForTest(oldConfig)

	mapper.InitSQL()
	resetJobClientsForTest()

	// AddJobClient validates that the engine exists before inserting the job.
	oldGetAlist := getAlistByID
	getAlistByID = func(alistID int64) (map[string]interface{}, error) {
		return map[string]interface{}{"id": alistID, "url": "https://alist.test", "token": "t"}, nil
	}
	defer func() { getAlistByID = oldGetAlist }()

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

func TestBrickedExistingJobLoadsDisabledWithoutPanicAndRemainsEditable(t *testing.T) {
	oldConfig := config.GetConfig()
	config.SetConfigForTest(&config.Config{
		DB: config.DBConfig{DBName: filepath.Join(t.TempDir(), "opensync-bricked.db")},
		Server: config.ServerConfig{
			Timeout:         0,
			CopyConcurrency: 1,
			ScanConcurrency: 1,
			PasswdStr:       "test-secret",
		},
	})
	defer config.SetConfigForTest(oldConfig)

	mapper.CloseDB()
	mapper.InitSQL()
	defer mapper.CloseDB()
	resetJobClientsForTest()
	defer resetJobClientsForTest()

	oldGetAlist := getAlistByID
	getAlistByID = func(alistID int64) (map[string]interface{}, error) {
		return map[string]interface{}{"id": alistID, "url": "https://alist.test", "token": "t"}, nil
	}
	defer func() { getAlistByID = oldGetAlist }()

	jobID, err := mapper.AddJob(map[string]interface{}{
		"enable":        1,
		"remark":        "bricked",
		"srcPath":       `["/src"]`,
		"dstPath":       `["/dst"]`,
		"alistId":       1,
		"useCacheT":     0,
		"scanIntervalT": 0,
		"useCacheS":     0,
		"scanIntervalS": 0,
		"method":        0,
		"interval":      0,
		"isCron":        1, // cron mode with no cron fields => invalid schedule
		"minFileSize":   0,
		"maxFileSize":   0,
	})
	if err != nil {
		t.Fatalf("mapper.AddJob() error: %v", err)
	}

	// Loading via GetJobClientByID must NOT panic; it must auto-disable in place.
	client := GetJobClientByID(jobID)
	if client == nil {
		t.Fatal("GetJobClientByID() returned nil")
	}
	if client.enabled() {
		t.Fatal("bricked job should be auto-disabled on load")
	}

	// Now user can repair it via EditJobClient.
	EditJobClient(map[string]interface{}{
		"id":            jobID,
		"enable":        1,
		"remark":        "repaired",
		"srcPath":       []string{"/src"},
		"dstPath":       []string{"/dst"},
		"alistId":       1,
		"useCacheT":     0,
		"scanIntervalT": 0,
		"useCacheS":     0,
		"scanIntervalS": 0,
		"method":        0,
		"interval":      30,
		"isCron":        0,
		"minFileSize":   0,
		"maxFileSize":   0,
	})
	if !client.enabled() {
		t.Fatal("repaired job should be enabled")
	}

	// And can delete it cleanly.
	RemoveJobClient(jobID)
	if _, ok := getJobClientIfExists(jobID); ok {
		t.Fatal("removed job should no longer exist in registry")
	}
}

func TestJobLifecycleConcurrentEditPauseContinueAbortRemoveRace(t *testing.T) {
	oldConfig := config.GetConfig()
	config.SetConfigForTest(&config.Config{
		DB: config.DBConfig{DBName: filepath.Join(t.TempDir(), "opensync-race.db")},
		Server: config.ServerConfig{
			Timeout:         0,
			CopyConcurrency: 1,
			ScanConcurrency: 1,
			PasswdStr:       "test-secret",
		},
	})
	defer config.SetConfigForTest(oldConfig)

	mapper.CloseDB()
	mapper.InitSQL()
	defer mapper.CloseDB()
	resetJobClientsForTest()
	defer resetJobClientsForTest()

	oldGetAlist := getAlistByID
	getAlistByID = func(alistID int64) (map[string]interface{}, error) {
		return map[string]interface{}{"id": alistID, "url": "https://alist.test", "token": "t"}, nil
	}
	defer func() { getAlistByID = oldGetAlist }()

	AddJobClient(map[string]interface{}{
		"enable":        1,
		"remark":        "race",
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
	jobID := client.idSnapshot()

	safeCall := func(fn func()) {
		defer func() { _ = recover() }()
		fn()
	}

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				safeCall(func() {
					EditJobClient(map[string]interface{}{
						"id":            jobID,
						"enable":        1,
						"remark":        "edited",
						"srcPath":       []string{"/src"},
						"dstPath":       []string{"/dst"},
						"alistId":       1,
						"useCacheT":     0,
						"scanIntervalT": 0,
						"useCacheS":     0,
						"scanIntervalS": 0,
						"method":        0,
						"interval":      60 + idx + j,
						"isCron":        0,
						"minFileSize":   0,
						"maxFileSize":   0,
					})
				})
				safeCall(func() { PauseJob(jobID) })
				safeCall(func() { ContinueJob(jobID) })
				safeCall(func() { AbortJob(jobID) })
			}
		}(i)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(15 * time.Millisecond)
		safeCall(func() { RemoveJobClient(jobID) })
	}()
	wg.Wait()
}

func TestDoAllJobManualSkipsBusyAndDeletedJobsAndRunsAvailable(t *testing.T) {
	resetJobClientsForTest()
	defer resetJobClientsForTest()

	busyClient := &JobClient{
		JobID: 1,
		Job:   map[string]interface{}{"id": int64(1), "enable": 1, "isCron": 2},
	}
	if !busyClient.tryMarkDoing() {
		t.Fatal("failed to mark busy client")
	}
	defer busyClient.markDone()

	availClient := &JobClient{
		JobID: 2,
		Job:   map[string]interface{}{"id": int64(2), "enable": 1, "isCron": 2},
	}

	jobClientListMu.Lock()
	jobClientList[1] = busyClient
	jobClientList[2] = availClient
	jobClientListMu.Unlock()

	oldGetEnable := getEnableJobList
	getEnableJobList = func() ([]map[string]interface{}, error) {
		return []map[string]interface{}{
			{"id": int64(1)},
			{"id": int64(999)}, // missing job -> skipped cleanly
			{"id": int64(2)},
		}, nil
	}
	defer func() { getEnableJobList = oldGetEnable }()

	// DoAllJobManual must skip busy job 1 and missing job 999, start job 2, and not panic.
	DoAllJobManual()
	if !availClient.waitUntilIdle(2 * time.Second) {
		t.Fatal("available client did not return to idle")
	}
}

func TestJobClientReleaseTaskDoesNotDetachNewerTask(t *testing.T) {
	client := &JobClient{
		JobID: 7,
		Job:   map[string]interface{}{"id": int64(7), "enable": 1, "isCron": 2},
	}
	if !client.tryStartRun() {
		t.Fatal("tryStartRun() = false, want true")
	}
	task1 := &JobTask{TaskID: 1}
	task2 := &JobTask{TaskID: 2}
	if !client.attachTaskIfActive(task1) {
		t.Fatal("attachTaskIfActive(task1) = false, want true")
	}

	// Simulate task1 completing normally and task2 starting immediately after.
	client.releaseTask(task1)
	if !client.tryStartRun() {
		t.Fatal("second tryStartRun() = false, want true")
	}
	if !client.attachTaskIfActive(task2) {
		t.Fatal("attachTaskIfActive(task2) = false, want true")
	}

	// A stale/duplicate releaseTask(task1) must NOT clear task2 or mark client idle.
	client.releaseTask(task1)
	if got := client.currentTask(); got != task2 {
		t.Fatalf("currentTask() = %#v, want task2", got)
	}
	if !client.isDoing() {
		t.Fatal("client.isDoing() = false, want true while task2 is active")
	}

	client.releaseTask(task2)
	if client.isDoing() || client.currentTask() != nil {
		t.Fatal("client should be idle after releaseTask(task2)")
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
