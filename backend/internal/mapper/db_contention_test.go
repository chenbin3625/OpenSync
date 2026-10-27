package mapper

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestExecuteManyRetriesSQLiteWriteContention(t *testing.T) {
	testDB, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "busy.db")+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer testDB.Close()
	defer SetDBForTest(testDB)()
	if _, err := testDB.Exec("CREATE TABLE items (name TEXT UNIQUE)"); err != nil {
		t.Fatal(err)
	}
	writer, err := testDB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Exec("INSERT INTO items VALUES ('held')"); err != nil {
		_ = writer.Rollback()
		t.Fatal(err)
	}
	released := make(chan error, 1)
	go func() {
		time.Sleep(150 * time.Millisecond)
		released <- writer.Commit()
	}()
	err = ExecuteMany("INSERT INTO items VALUES (?)", [][]interface{}{{"one"}, {"two"}})
	if releaseErr := <-released; releaseErr != nil {
		t.Fatal(releaseErr)
	}
	if err != nil {
		t.Fatalf("transient write lock failed the batch: %v", err)
	}
	var count int
	if err := testDB.QueryRow("SELECT COUNT(*) FROM items").Scan(&count); err != nil || count != 3 {
		t.Fatalf("rows = %d, err = %v; want 3 rows exactly once", count, err)
	}
}

func TestExecuteManyConcurrentBatchesKeepAllRows(t *testing.T) {
	testDB, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "concurrent.db")+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer testDB.Close()
	defer SetDBForTest(testDB)()
	if _, err := testDB.Exec("CREATE TABLE items (name TEXT UNIQUE)"); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	errors := make(chan error, 3)
	for task := 0; task < 3; task++ {
		workers.Go(func() {
			for batch := 0; batch < 2; batch++ {
				args := make([][]interface{}, 100)
				for row := range args {
					args[row] = []interface{}{fmt.Sprintf("%d/%d/%d", task, batch, row)}
				}
				if err := ExecuteMany("INSERT INTO items VALUES (?)", args); err != nil {
					errors <- err
					return
				}
			}
		})
	}
	workers.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	var count int
	if err := testDB.QueryRow("SELECT COUNT(*) FROM items").Scan(&count); err != nil || count != 600 {
		t.Fatalf("rows = %d, err = %v; want all 600 task items", count, err)
	}
}

func TestExecuteManyRetriesBusyCommitWithoutDuplicateRows(t *testing.T) {
	testDB, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "commit.db")+"?_pragma=journal_mode(DELETE)&_pragma=busy_timeout(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer testDB.Close()
	defer SetDBForTest(testDB)()
	if _, err := testDB.Exec("CREATE TABLE items (name TEXT UNIQUE)"); err != nil {
		t.Fatal(err)
	}
	reader, err := testDB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := reader.QueryRow("SELECT COUNT(*) FROM items").Scan(&count); err != nil {
		_ = reader.Rollback()
		t.Fatal(err)
	}
	released := make(chan error, 1)
	go func() {
		time.Sleep(150 * time.Millisecond)
		released <- reader.Rollback()
	}()
	err = ExecuteMany("INSERT INTO items VALUES (?)", [][]interface{}{{"one"}, {"two"}})
	if releaseErr := <-released; releaseErr != nil {
		t.Fatal(releaseErr)
	}
	if err != nil {
		t.Fatalf("transient reader lock failed the commit: %v", err)
	}
	if err := testDB.QueryRow("SELECT COUNT(*) FROM items").Scan(&count); err != nil || count != 2 {
		t.Fatalf("rows = %d, err = %v; want both rows exactly once", count, err)
	}
}

func TestExecuteManyReturnsPersistentLockError(t *testing.T) {
	testDB, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "locked.db")+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer testDB.Close()
	defer SetDBForTest(testDB)()
	if _, err := testDB.Exec("CREATE TABLE items (name TEXT)"); err != nil {
		t.Fatal(err)
	}
	writer, err := testDB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Rollback()
	if _, err := writer.Exec("INSERT INTO items VALUES ('held')"); err != nil {
		t.Fatal(err)
	}
	if err := ExecuteMany("INSERT INTO items VALUES (?)", [][]interface{}{{"one"}}); err == nil {
		t.Fatal("persistent lock must fail after bounded retries")
	}
}

func TestExecuteManyRollsBackBatchOnConstraintError(t *testing.T) {
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer testDB.Close()
	defer SetDBForTest(testDB)()
	if _, err := testDB.Exec("CREATE TABLE items (name TEXT UNIQUE)"); err != nil {
		t.Fatal(err)
	}
	if err := ExecuteMany("INSERT INTO items VALUES (?)", [][]interface{}{{"same"}, {"same"}}); err == nil {
		t.Fatal("duplicate names must fail the batch")
	}
	var count int
	if err := testDB.QueryRow("SELECT COUNT(*) FROM items").Scan(&count); err != nil || count != 0 {
		t.Fatalf("rows = %d, err = %v; failed batch must leave no partial rows", count, err)
	}
}
