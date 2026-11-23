package mapper

import (
	"database/sql"
	"opensync/internal/msg"
	"testing"

	_ "modernc.org/sqlite"
)

func TestCountJobsByAlistID(t *testing.T) {
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	defer testDB.Close()

	if _, err := testDB.Exec(`CREATE TABLE job(
		id integer primary key autoincrement,
		alistId integer
	)`); err != nil {
		t.Fatalf("create job: %v", err)
	}
	if _, err := testDB.Exec("INSERT INTO job(alistId) VALUES (7), (7), (8)"); err != nil {
		t.Fatalf("insert jobs: %v", err)
	}

	defer SetDBForTest(testDB)()

	count, err := CountJobsByAlistID(7)
	if err != nil {
		t.Fatalf("CountJobsByAlistID() error: %v", err)
	}
	if count != 2 {
		t.Fatalf("CountJobsByAlistID(7) = %d, want 2", count)
	}
}

func TestUpdateAlistReturnsNotFoundWhenRowMissing(t *testing.T) {
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	defer testDB.Close()

	if _, err := testDB.Exec(`CREATE TABLE alist_list(
		id integer primary key autoincrement,
		remark text,
		url text,
		userName text,
		token text
	)`); err != nil {
		t.Fatalf("create alist_list: %v", err)
	}
	if _, err := testDB.Exec("INSERT INTO alist_list(id, remark, url, userName, token) VALUES (1, 'old', 'https://old.test', 'admin', 'tok')"); err != nil {
		t.Fatalf("insert alist: %v", err)
	}

	defer SetDBForTest(testDB)()

	if err := UpdateAlist(1, "new", "https://new.test", nil); err != nil {
		t.Fatalf("UpdateAlist(existing) error: %v", err)
	}
	if err := UpdateAlist(999, "missing", "https://missing.test", nil); err == nil || err.Error() != msg.AlistNotFound {
		t.Fatalf("UpdateAlist(missing) error = %v, want %q", err, msg.AlistNotFound)
	}
	tok := "new-token"
	if err := UpdateAlist(999, "missing", "https://missing.test", &tok); err == nil || err.Error() != msg.AlistNotFound {
		t.Fatalf("UpdateAlist(missing with token) error = %v, want %q", err, msg.AlistNotFound)
	}
}
