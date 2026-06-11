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

func TestMigrateDBTxCreatesJobTaskItemFTS(t *testing.T) {
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
	if _, err := testDB.Exec("INSERT INTO user_list(userName, passwd, sqlVersion) VALUES ('admin', 'x', 260606)"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := testDB.Exec(`CREATE TABLE job_task_item(
		id integer primary key autoincrement,
		taskId integer,
		srcPath text,
		dstPath text,
		isPath integer DEFAULT 0,
		fileName text,
		fileSize integer,
		type integer,
		alistTaskId text,
		status integer DEFAULT 0,
		progress real,
		errMsg text,
		createTime integer DEFAULT (strftime('%s', 'now'))
	)`); err != nil {
		t.Fatalf("create job_task_item: %v", err)
	}
	if _, err := testDB.Exec(`INSERT INTO job_task_item(id, taskId, srcPath, dstPath, fileName, alistTaskId, errMsg)
		VALUES (1, 10, '/src/photos/cat.jpg', '/backup/photos/cat.jpg', 'cat.jpg', 'copy_task_002', 'copy failed')`); err != nil {
		t.Fatalf("insert existing task item: %v", err)
	}

	if err := migrateDBTx(testDB, 260606); err != nil {
		t.Fatalf("migrateDBTx() error: %v", err)
	}

	if !tableExists(testDB, "job_task_item_fts") {
		t.Fatalf("expected job_task_item_fts to exist")
	}
	var rowID int64
	if err := testDB.QueryRow("SELECT rowid FROM job_task_item_fts WHERE job_task_item_fts MATCH ?", fts5Phrase("copy failed")).Scan(&rowID); err != nil {
		t.Fatalf("query rebuilt fts row: %v", err)
	}
	if rowID != 1 {
		t.Fatalf("rebuilt fts rowid = %d, want 1", rowID)
	}

	if _, err := testDB.Exec(`INSERT INTO job_task_item(id, taskId, fileName, alistTaskId)
		VALUES (2, 10, 'movie.mkv', 'move_task_004')`); err != nil {
		t.Fatalf("insert triggered task item: %v", err)
	}
	if err := testDB.QueryRow("SELECT rowid FROM job_task_item_fts WHERE job_task_item_fts MATCH ?", fts5Phrase("move_task")).Scan(&rowID); err != nil {
		t.Fatalf("query triggered fts row: %v", err)
	}
	if rowID != 2 {
		t.Fatalf("triggered fts rowid = %d, want 2", rowID)
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

func tableExists(db *sql.DB, tableName string) bool {
	var name string
	err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", tableName).Scan(&name)
	return err == nil && name == tableName
}
