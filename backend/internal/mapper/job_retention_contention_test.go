package mapper

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func retentionContentionDB(t *testing.T, journalMode string) *sql.DB {
	t.Helper()
	testDB, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "retention.db")+"?_pragma=journal_mode("+journalMode+")&_pragma=busy_timeout(1)&_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = testDB.Close() })
	t.Cleanup(SetDBForTest(testDB))
	for _, query := range []string{
		"CREATE TABLE job_task (id INTEGER PRIMARY KEY, status INTEGER, runTime INTEGER, createTime INTEGER)",
		"CREATE TABLE job_task_item (id INTEGER PRIMARY KEY, taskId INTEGER REFERENCES job_task(id))",
		"INSERT INTO job_task VALUES (1, 2, 100, 100), (2, 1, 100, 100), (3, 0, 100, 100), (4, 2, 300, 300)",
		"INSERT INTO job_task_item VALUES (1, 1), (2, 2), (3, 3), (4, 4)",
	} {
		if _, err := testDB.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	return testDB
}

func TestDeleteJobTaskByRunTimeRetriesSQLiteContention(t *testing.T) {
	for _, mode := range []string{"WAL", "DELETE"} {
		t.Run(mode, func(t *testing.T) {
			testDB := retentionContentionDB(t, mode)
			holder, err := testDB.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer holder.Rollback()
			if mode == "WAL" {
				if _, err := holder.Exec("UPDATE job_task SET createTime=101 WHERE id=1"); err != nil {
					t.Fatal(err)
				}
			} else {
				var count int
				if err := holder.QueryRow("SELECT COUNT(*) FROM job_task_item").Scan(&count); err != nil {
					t.Fatal(err)
				}
			}
			released := make(chan error, 1)
			go func() {
				time.Sleep(150 * time.Millisecond)
				released <- holder.Rollback()
			}()
			err = DeleteJobTaskByRunTime(200)
			if releaseErr := <-released; releaseErr != nil {
				t.Fatal(releaseErr)
			}
			if err != nil {
				t.Fatalf("transient %s lock stopped history cleanup: %v", mode, err)
			}
			for _, table := range []string{"job_task", "job_task_item"} {
				var count, remainingExpired int
				if err := testDB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 3 {
					t.Fatalf("%s rows = %d, err = %v; want active, queued and recent rows retained", table, count, err)
				}
				if err := testDB.QueryRow("SELECT COUNT(*) FROM " + table + " WHERE id=1").Scan(&remainingExpired); err != nil || remainingExpired != 0 {
					t.Fatalf("%s expired rows = %d, err = %v; want 0", table, remainingExpired, err)
				}
			}
		})
	}
}

func TestDeleteJobTaskByRunTimeReturnsPersistentLock(t *testing.T) {
	testDB := retentionContentionDB(t, "WAL")
	holder, err := testDB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Rollback()
	if _, err := holder.Exec("UPDATE job_task SET createTime=101 WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	if err := DeleteJobTaskByRunTime(200); err == nil {
		t.Fatal("persistent lock must be returned to the cleanup logger")
	}
	for _, table := range []string{"job_task", "job_task_item"} {
		var count int
		if err := testDB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 4 {
			t.Fatalf("%s rows = %d, err = %v; failed cleanup must retain all rows", table, count, err)
		}
	}
}

func TestDeleteJobTaskByRunTimeRollsBackChildDeletes(t *testing.T) {
	testDB := retentionContentionDB(t, "WAL")
	if _, err := testDB.Exec("CREATE TRIGGER reject_task_delete BEFORE DELETE ON job_task BEGIN SELECT RAISE(ABORT, 'retention blocked'); END"); err != nil {
		t.Fatal(err)
	}
	if err := DeleteJobTaskByRunTime(200); err == nil {
		t.Fatal("parent delete failure must be returned")
	}
	for _, table := range []string{"job_task", "job_task_item"} {
		var count int
		if err := testDB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 4 {
			t.Fatalf("%s rows = %d, err = %v; failed transaction must roll back child deletes", table, count, err)
		}
	}
}
