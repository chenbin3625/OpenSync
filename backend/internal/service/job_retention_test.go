package service

import (
	"log"
	"math"
	"opensync/internal/mapper"
	"testing"
	"time"
)

func TestTaskRetentionCutoffDisabledWhenTaskSaveIsZero(t *testing.T) {
	_, ok := taskRetentionCutoff(time.Unix(1_700_000_000, 0), 0)
	if ok {
		t.Fatalf("taskRetentionCutoff(..., 0) ok = true, want false")
	}
}

func TestTaskRetentionCutoffUsesDays(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	cutoff, ok := taskRetentionCutoff(now, 7)
	if !ok {
		t.Fatalf("taskRetentionCutoff(..., 7) ok = false, want true")
	}

	want := now.Add(-7 * 24 * time.Hour).Unix()
	if cutoff != want {
		t.Fatalf("cutoff = %d, want %d", cutoff, want)
	}
}

func TestTaskRetentionCutoffPreventsIntegerOverflow(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	if cutoff, ok := taskRetentionCutoff(now, math.MaxInt); ok || cutoff != 0 {
		t.Fatalf("taskRetentionCutoff(MaxInt) = (%d, %v), want (0, false)", cutoff, ok)
	}
	if cutoff, ok := taskRetentionCutoff(now, 100000); ok || cutoff != 0 {
		t.Fatalf("taskRetentionCutoff(100000) = (%d, %v), want (0, false)", cutoff, ok)
	}
}

func TestCleanupExpiredTasksAndRemoveTaskProtectActiveAndRetrySourceTasks(t *testing.T) {
	testDB := newServiceTaskStatusTestDB(t)
	oldDB := mapperDBForServiceTest(testDB)
	defer oldDB()
	resetJobClientsForTest()
	defer resetJobClientsForTest()

	now := time.Unix(1_700_000_000, 0)
	expiredVeryOld := now.Add(-30 * 24 * time.Hour).Unix()
	retrySourceRunTime := now.Add(-15 * 24 * time.Hour).Unix()
	activeRunTime := now.Add(-10 * 24 * time.Hour).Unix()

	if _, err := testDB.Exec("DELETE FROM job_task"); err != nil {
		t.Fatalf("clear job_task: %v", err)
	}
	if _, err := testDB.Exec("INSERT INTO job_task(id, jobId, status, runTime) VALUES (1, 1, 2, ?)", expiredVeryOld); err != nil {
		t.Fatalf("insert task 1: %v", err)
	}
	if _, err := testDB.Exec("INSERT INTO job_task(id, jobId, status, runTime) VALUES (2, 1, 3, ?)", retrySourceRunTime); err != nil {
		t.Fatalf("insert task 2: %v", err)
	}
	if _, err := testDB.Exec("INSERT INTO job_task(id, jobId, status, runTime) VALUES (3, 1, 1, ?)", activeRunTime); err != nil {
		t.Fatalf("insert task 3: %v", err)
	}

	activeTask := &JobTask{
		TaskID:            3,
		RunTime:           activeRunTime,
		RetrySourceTaskID: 2,
	}
	client := &JobClient{JobID: 1}
	client.setCurrentTask(activeTask)
	jobClientListMu.Lock()
	jobClientList[1] = client
	jobClientListMu.Unlock()

	// RemoveTask must reject both active task (3) and active retry source task (2).
	requirePublicPanic(t, func() { RemoveTask(3) })
	requirePublicPanic(t, func() { RemoveTask(2) })

	// CleanupExpiredTasks with 7-day retention must delete task 1 (older than retrySourceRunTime),
	// while preserving task 2 (retry source) and task 3 (active).
	CleanupExpiredTasks(log.Default(), 7, now)

	if _, err := mapper.GetJobTaskByID(1); err == nil {
		t.Fatal("expired task 1 was not deleted")
	}
	if _, err := mapper.GetJobTaskByID(2); err != nil {
		t.Fatalf("protected retry source task 2 was deleted: %v", err)
	}
	if _, err := mapper.GetJobTaskByID(3); err != nil {
		t.Fatalf("protected active task 3 was deleted: %v", err)
	}
}
