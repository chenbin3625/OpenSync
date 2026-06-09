package mapper

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestMigrateDBTxSkipsLegacyRenameWhenSpeedColumnMissing(t *testing.T) {
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

	if _, err := testDB.Exec(`CREATE TABLE job(
		id integer primary key autoincrement,
		useCacheT integer DEFAULT 0
	)`); err != nil {
		t.Fatalf("create job: %v", err)
	}

	if err := migrateDBTx(testDB, 250520); err != nil {
		t.Fatalf("migrateDBTx() error: %v", err)
	}

	var version int64
	if err := testDB.QueryRow("SELECT sqlVersion FROM user_list LIMIT 1").Scan(&version); err != nil {
		t.Fatalf("read sqlVersion: %v", err)
	}
	if version != currentVersion {
		t.Fatalf("sqlVersion = %d, want currentVersion %d", version, currentVersion)
	}

	for _, column := range []string{"scanIntervalT", "useCacheS", "scanIntervalS"} {
		if !tableHasColumn(testDB, "job", column) {
			t.Fatalf("job table missing migrated column %q", column)
		}
	}
}

func TestMigrateDBTxAddsJobFileSizeRangeColumns(t *testing.T) {
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
	if _, err := testDB.Exec("INSERT INTO user_list(userName, passwd, sqlVersion) VALUES ('admin', 'x', 250608)"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := testDB.Exec(`CREATE TABLE job(
		id integer primary key autoincrement,
		exclude text DEFAULT NULL
	)`); err != nil {
		t.Fatalf("create job: %v", err)
	}

	if err := migrateDBTx(testDB, 250608); err != nil {
		t.Fatalf("migrateDBTx() error: %v", err)
	}

	for _, column := range []string{"minFileSize", "maxFileSize"} {
		if !tableHasColumn(testDB, "job", column) {
			t.Fatalf("job table missing migrated column %q", column)
		}
	}
}

func TestMigrateDBTxDropsUnusedCronColumns(t *testing.T) {
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
	if _, err := testDB.Exec("INSERT INTO user_list(userName, passwd, sqlVersion) VALUES ('admin', 'x', 260605)"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := testDB.Exec(`CREATE TABLE job(
		id integer primary key autoincrement,
		year text DEFAULT NULL,
		month text DEFAULT NULL,
		day text DEFAULT NULL,
		week text DEFAULT NULL,
		day_of_week text DEFAULT NULL,
		hour text DEFAULT NULL,
		minute text DEFAULT NULL,
		second text DEFAULT NULL,
		start_date text DEFAULT NULL,
		end_date text DEFAULT NULL
	)`); err != nil {
		t.Fatalf("create job: %v", err)
	}

	if err := migrateDBTx(testDB, 260605); err != nil {
		t.Fatalf("migrateDBTx() error: %v", err)
	}

	for _, column := range []string{"year", "week", "start_date", "end_date"} {
		if tableHasColumn(testDB, "job", column) {
			t.Fatalf("job table still has unused cron column %q", column)
		}
	}
	for _, column := range []string{"month", "day", "day_of_week", "hour", "minute", "second"} {
		if !tableHasColumn(testDB, "job", column) {
			t.Fatalf("job table missing active cron column %q", column)
		}
	}
}

func TestEnsureIndexesCreatesTaskStatusTimeIndex(t *testing.T) {
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	defer testDB.Close()

	if _, err := testDB.Exec(`CREATE TABLE job_task(
		id integer primary key autoincrement,
		jobId integer,
		status integer,
		createTime integer
	)`); err != nil {
		t.Fatalf("create job_task: %v", err)
	}
	if _, err := testDB.Exec(`CREATE TABLE job_task_item(
		id integer primary key autoincrement,
		taskId integer,
		status integer,
		type integer,
		createTime integer
	)`); err != nil {
		t.Fatalf("create job_task_item: %v", err)
	}

	ensureIndexes(testDB)

	if !indexExists(testDB, "idx_job_task_item_task_status_time") {
		t.Fatalf("expected idx_job_task_item_task_status_time to exist")
	}
}

func tableHasColumn(db *sql.DB, tableName, columnName string) bool {
	rows, err := db.Query("PRAGMA table_info(" + tableName + ")")
	if err != nil {
		return false
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue interface{}
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return false
		}
		if name == columnName {
			return true
		}
	}
	return false
}

func indexExists(db *sql.DB, indexName string) bool {
	var name string
	err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='index' AND name=?", indexName).Scan(&name)
	return err == nil && name == indexName
}
