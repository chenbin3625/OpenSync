package mapper

import (
	"database/sql"
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
