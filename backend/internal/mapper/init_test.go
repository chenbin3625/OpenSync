package mapper

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestMigrateDBTxRollsBackWhenMigrationFails(t *testing.T) {
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	defer testDB.Close()

	if _, err := testDB.Exec(`CREATE TABLE user_list(
		id integer primary key autoincrement,
		userName text,
		passwd text,
		sqlVersion integer
	)`); err != nil {
		t.Fatalf("create user_list: %v", err)
	}
	if _, err := testDB.Exec("INSERT INTO user_list(userName, passwd, sqlVersion) VALUES ('admin', 'x', 250520)"); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := migrateDBTx(testDB, 250520); err == nil {
		t.Fatalf("migrateDBTx() returned nil error, want failure")
	}

	var version int64
	if err := testDB.QueryRow("SELECT sqlVersion FROM user_list LIMIT 1").Scan(&version); err != nil {
		t.Fatalf("read sqlVersion: %v", err)
	}
	if version != 250520 {
		t.Fatalf("sqlVersion = %d, want rollback to keep 250520", version)
	}
}
