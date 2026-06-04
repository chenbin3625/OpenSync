package mapper

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestDeleteJobRollsBackWhenChildDeleteFails(t *testing.T) {
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	defer testDB.Close()

	if _, err := testDB.Exec(`CREATE TABLE job(
		id integer primary key autoincrement,
		remark text
	)`); err != nil {
		t.Fatalf("create job: %v", err)
	}
	if _, err := testDB.Exec(`CREATE TABLE job_task(
		id integer primary key autoincrement,
		jobId integer
	)`); err != nil {
		t.Fatalf("create job_task: %v", err)
	}
	if _, err := testDB.Exec("INSERT INTO job(id, remark) VALUES (1, 'job')"); err != nil {
		t.Fatalf("insert job: %v", err)
	}
	if _, err := testDB.Exec("INSERT INTO job_task(id, jobId) VALUES (10, 1)"); err != nil {
		t.Fatalf("insert job_task: %v", err)
	}

	oldDB := db
	db = testDB
	defer func() {
		db = oldDB
	}()

	if err := DeleteJob(1); err == nil {
		t.Fatalf("DeleteJob() returned nil error, want failure")
	}

	var jobCount int
	if err := testDB.QueryRow("SELECT COUNT(*) FROM job WHERE id=1").Scan(&jobCount); err != nil {
		t.Fatalf("count job: %v", err)
	}
	if jobCount != 1 {
		t.Fatalf("job count = %d, want rollback to keep 1", jobCount)
	}

	var taskCount int
	if err := testDB.QueryRow("SELECT COUNT(*) FROM job_task WHERE id=10").Scan(&taskCount); err != nil {
		t.Fatalf("count job_task: %v", err)
	}
	if taskCount != 1 {
		t.Fatalf("job_task count = %d, want rollback to keep 1", taskCount)
	}
}
