package mapper

import (
	"database/sql"
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

	oldDB := db
	db = testDB
	defer func() {
		db = oldDB
	}()

	count, err := CountJobsByAlistID(7)
	if err != nil {
		t.Fatalf("CountJobsByAlistID() error: %v", err)
	}
	if count != 2 {
		t.Fatalf("CountJobsByAlistID(7) = %d, want 2", count)
	}
}
