package mapper

import (
	"database/sql"
	"opensync/internal/msg"
	"path/filepath"
	"testing"
	"time"
)

func openRetentionTestDB(t *testing.T, journalMode string) *sql.DB {
	t.Helper()
	testDB, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "retention.db")+"?_pragma=journal_mode("+journalMode+")&_pragma=busy_timeout(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = testDB.Close() })
	t.Cleanup(SetDBForTest(testDB))
	for _, query := range []string{
		"CREATE TABLE job_task (id INTEGER PRIMARY KEY, runTime INTEGER, createTime INTEGER, status INTEGER)",
		"CREATE TABLE job_task_item (id INTEGER PRIMARY KEY, taskId INTEGER)",
		"INSERT INTO job_task VALUES (1, 20, 10, 2), (2, 20, 10, 0), (3, 20, 10, 1), (4, 60, 10, 2), (5, 50, 10, 2), (6, 0, 20, 3), (7, 0, 60, 2)",
		"INSERT INTO job_task_item VALUES (1, 1), (2, 2), (3, 3), (4, 4), (5, 5), (6, 6), (7, 7)",
	} {
		if _, err := testDB.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	return testDB
}

func TestDeleteJobTaskByRunTimeRetriesSQLiteContention(t *testing.T) {
	for _, journalMode := range []string{"WAL", "DELETE"} {
		t.Run(journalMode, func(t *testing.T) {
			testDB := openRetentionTestDB(t, journalMode)
			blocker, err := testDB.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer blocker.Rollback()
			if journalMode == "WAL" {
				if _, err := blocker.Exec("UPDATE job_task SET status=status WHERE id=1"); err != nil {
					t.Fatal(err)
				}
			} else {
				var count int
				if err := blocker.QueryRow("SELECT COUNT(*) FROM job_task").Scan(&count); err != nil {
					t.Fatal(err)
				}
			}
			released := make(chan error, 1)
			go func() {
				time.Sleep(150 * time.Millisecond)
				released <- blocker.Rollback()
			}()
			err = DeleteJobTaskByRunTime(50)
			if releaseErr := <-released; releaseErr != nil {
				t.Fatal(releaseErr)
			}
			if err != nil {
				t.Fatalf("transient lock failed retention cleanup: %v", err)
			}
			for _, table := range []string{"job_task", "job_task_item"} {
				var count int
				if err := testDB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 5 {
					t.Fatalf("%s rows = %d, err = %v; want 5 active or unexpired rows", table, count, err)
				}
				if err := testDB.QueryRow("SELECT COUNT(*) FROM " + table + " WHERE id IN (1, 6)").Scan(&count); err != nil || count != 0 {
					t.Fatalf("%s expired rows = %d, err = %v; want 0", table, count, err)
				}
			}
		})
	}
}

func TestDeleteJobTaskByRunTimeReturnsPersistentLock(t *testing.T) {
	testDB := openRetentionTestDB(t, "WAL")
	blocker, err := testDB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback()
	if _, err := blocker.Exec("UPDATE job_task SET status=status WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	if err := DeleteJobTaskByRunTime(50); err == nil {
		t.Fatal("persistent lock must fail after bounded retries")
	}
	if err := blocker.Rollback(); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"job_task", "job_task_item"} {
		var count int
		if err := testDB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 7 {
			t.Fatalf("%s rows = %d, err = %v; want all 7 rows after failure", table, count, err)
		}
	}
}

func TestDeleteJobTaskByRunTimeRollsBackItemsOnTaskDeleteError(t *testing.T) {
	testDB := openRetentionTestDB(t, "WAL")
	if _, err := testDB.Exec("CREATE TRIGGER reject_task_delete BEFORE DELETE ON job_task BEGIN SELECT RAISE(ABORT, 'cannot delete task'); END"); err != nil {
		t.Fatal(err)
	}
	if err := DeleteJobTaskByRunTime(50); err == nil {
		t.Fatal("task delete failure must be reported")
	}
	for _, table := range []string{"job_task", "job_task_item"} {
		var count int
		if err := testDB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 7 {
			t.Fatalf("%s rows = %d, err = %v; failed transaction must keep all 7 rows", table, count, err)
		}
	}
}

func TestDeleteJobTaskByTaskIDAvoidsBusySnapshotUnderWALContention(t *testing.T) {
	testDB := openRetentionTestDB(t, "WAL")

	// Hold an active WAL writer that commits after 150ms. A DEFERRED SELECT-then-DELETE
	// transaction would hit SQLITE_BUSY_SNAPSHOT; BEGIN IMMEDIATE + retry must succeed.
	writer, err := testDB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Rollback()
	if _, err := writer.Exec("UPDATE job_task SET status=status WHERE id=4"); err != nil {
		t.Fatal(err)
	}
	committed := make(chan error, 1)
	go func() {
		time.Sleep(150 * time.Millisecond)
		committed <- writer.Commit()
	}()

	err = DeleteJobTaskByTaskID(1)
	if commitErr := <-committed; commitErr != nil {
		t.Fatal(commitErr)
	}
	if err != nil {
		t.Fatalf("DeleteJobTaskByTaskID(1) under WAL contention failed: %v", err)
	}

	var taskCount, itemCount int
	if err := testDB.QueryRow("SELECT COUNT(*) FROM job_task WHERE id=1").Scan(&taskCount); err != nil || taskCount != 0 {
		t.Fatalf("task 1 count = %d, err = %v; want 0", taskCount, err)
	}
	if err := testDB.QueryRow("SELECT COUNT(*) FROM job_task_item WHERE taskId=1").Scan(&itemCount); err != nil || itemCount != 0 {
		t.Fatalf("task 1 item count = %d, err = %v; want 0", itemCount, err)
	}
}

func TestDeleteJobTaskByTaskIDRejectsRunningAndMissingTasks(t *testing.T) {
	testDB := openRetentionTestDB(t, "WAL")
	_ = testDB

	if err := DeleteJobTaskByTaskID(2); err == nil || err.Error() != msg.JobRunningCannotDelete {
		t.Fatalf("DeleteJobTaskByTaskID(waiting) error = %v, want %q", err, msg.JobRunningCannotDelete)
	}
	if err := DeleteJobTaskByTaskID(3); err == nil || err.Error() != msg.JobRunningCannotDelete {
		t.Fatalf("DeleteJobTaskByTaskID(running) error = %v, want %q", err, msg.JobRunningCannotDelete)
	}
	if err := DeleteJobTaskByTaskID(999); err == nil || err.Error() != msg.TaskNotFound {
		t.Fatalf("DeleteJobTaskByTaskID(missing) error = %v, want %q", err, msg.TaskNotFound)
	}
}

func TestDeleteJobTaskChunksLargeItemDeletionAndProtectsActiveTasks(t *testing.T) {
	oldChunk := taskItemDeleteChunkSize
	taskItemDeleteChunkSize = 3
	t.Cleanup(func() { taskItemDeleteChunkSize = oldChunk })

	testDB, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "chunked.db")+"?_pragma=auto_vacuum(INCREMENTAL)&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = testDB.Close() })
	t.Cleanup(SetDBForTest(testDB))

	for _, stmt := range []string{
		"CREATE TABLE job_task (id INTEGER PRIMARY KEY, runTime INTEGER, createTime INTEGER, status INTEGER)",
		"CREATE TABLE job_task_item (id INTEGER PRIMARY KEY AUTOINCREMENT, taskId INTEGER)",
		// Expired finished tasks (1, 2), active task (3), unexpired finished task (4).
		"INSERT INTO job_task VALUES (1, 10, 10, 2), (2, 20, 20, 7), (3, 10, 10, 1), (4, 100, 100, 2)",
	} {
		if _, err := testDB.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 10; i++ {
		if _, err := testDB.Exec("INSERT INTO job_task_item(taskId) VALUES (1)"); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 8; i++ {
		if _, err := testDB.Exec("INSERT INTO job_task_item(taskId) VALUES (2)"); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 4; i++ {
		if _, err := testDB.Exec("INSERT INTO job_task_item(taskId) VALUES (3)"); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 7; i++ {
		if _, err := testDB.Exec("INSERT INTO job_task_item(taskId) VALUES (4)"); err != nil {
			t.Fatal(err)
		}
	}

	// Verify deleteJobTasksByIDs refuses to delete active task 3 even if passed directly.
	deleted, err := deleteJobTasksByIDs([]int64{3})
	if err != nil {
		t.Fatalf("deleteJobTasksByIDs(active) error: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("deleteJobTasksByIDs(active) deleted = %d, want 0", deleted)
	}

	// Retention cleanup must remove expired tasks 1 and 2 and all 18 of their items in chunks.
	if err := DeleteJobTaskByRunTime(50); err != nil {
		t.Fatalf("DeleteJobTaskByRunTime(50) error: %v", err)
	}
	var remainingExpiredItems int
	if err := testDB.QueryRow("SELECT COUNT(*) FROM job_task_item WHERE taskId IN (1, 2)").Scan(&remainingExpiredItems); err != nil || remainingExpiredItems != 0 {
		t.Fatalf("expired items remaining = %d, err = %v; want 0", remainingExpiredItems, err)
	}
	var activeItems int
	if err := testDB.QueryRow("SELECT COUNT(*) FROM job_task_item WHERE taskId=3").Scan(&activeItems); err != nil || activeItems != 4 {
		t.Fatalf("active task items = %d, err = %v; want 4", activeItems, err)
	}

	// DeleteJobTaskByTaskID must also chunk-delete all 7 items of task 4.
	if err := DeleteJobTaskByTaskID(4); err != nil {
		t.Fatalf("DeleteJobTaskByTaskID(4) error: %v", err)
	}
	var task4Items int
	if err := testDB.QueryRow("SELECT COUNT(*) FROM job_task_item WHERE taskId=4").Scan(&task4Items); err != nil || task4Items != 0 {
		t.Fatalf("task 4 items remaining = %d, err = %v; want 0", task4Items, err)
	}
}
