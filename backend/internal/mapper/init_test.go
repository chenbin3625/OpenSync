package mapper

import (
	"context"
	"database/sql"
	"opensync/internal/config"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestInitSQLCreatesSchemaWithoutInitialAdminUser(t *testing.T) {
	resetGlobalDBForTest(t, &config.Config{
		DB:     config.DBConfig{DBName: filepath.Join(t.TempDir(), "opensync.db")},
		Server: config.ServerConfig{PasswdStr: "test-secret"},
	})

	InitSQL()

	var count int
	if err := GetDB().QueryRow("SELECT COUNT(*) FROM user_list").Scan(&count); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 0 {
		t.Fatalf("initial users = %d, want 0", count)
	}
	if !tableHasColumn(GetDB(), "user_list", "recoveryKey") {
		t.Fatalf("user_list missing recoveryKey column")
	}
	var userVer int
	if err := GetDB().QueryRow("PRAGMA user_version").Scan(&userVer); err != nil {
		t.Fatalf("PRAGMA user_version error: %v", err)
	}
	if userVer != currentVersion {
		t.Fatalf("PRAGMA user_version = %d, want %d", userVer, currentVersion)
	}
}

func TestInitSQLCanRestartBeforeInitialAdminUserExists(t *testing.T) {
	resetGlobalDBForTest(t, &config.Config{
		DB:     config.DBConfig{DBName: filepath.Join(t.TempDir(), "opensync.db")},
		Server: config.ServerConfig{PasswdStr: "test-secret"},
	})

	InitSQL()
	if err := CloseDB(); err != nil {
		t.Fatalf("CloseDB() error: %v", err)
	}

	InitSQL()

	var count int
	if err := GetDB().QueryRow("SELECT COUNT(*) FROM user_list").Scan(&count); err != nil {
		t.Fatalf("count users after restart: %v", err)
	}
	if count != 0 {
		t.Fatalf("initial users after restart = %d, want 0", count)
	}
	var version int64
	if err := GetDB().QueryRow("SELECT sqlVersion FROM user_list LIMIT 1").Scan(&version); err != sql.ErrNoRows {
		t.Fatalf("read empty sqlVersion error = %v, want sql.ErrNoRows", err)
	}
	ver, err := schemaVersion(GetDB())
	if err != nil {
		t.Fatalf("schemaVersion() error: %v", err)
	}
	if ver != currentVersion {
		t.Fatalf("schemaVersion() = %d, want %d", ver, currentVersion)
	}
}

func TestInitSchemaRollsBackCompletelyOnPartialDDLFailure(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "partial-init.db")
	testDB, err := openConfiguredDB(dbPath)
	if err != nil {
		t.Fatalf("openConfiguredDB() error: %v", err)
	}
	defer testDB.Close()

	brokenStmts := append([]string{}, firstRunSchemaStatements()[:2]...)
	brokenStmts = append(brokenStmts, "CREATE TABLE invalid_syntax ((")

	if err := initSchemaWithStmts(testDB, brokenStmts); err == nil {
		t.Fatalf("initSchemaWithStmts(broken) error = nil, want failure")
	}

	if tableExists(testDB, "user_list") {
		t.Fatalf("partial first-run init left user_list behind instead of rolling back")
	}
	if tableExists(testDB, "alist_list") {
		t.Fatalf("partial first-run init left alist_list behind instead of rolling back")
	}
	var userVer int
	if err := testDB.QueryRow("PRAGMA user_version").Scan(&userVer); err != nil {
		t.Fatalf("PRAGMA user_version error: %v", err)
	}
	if userVer != 0 {
		t.Fatalf("PRAGMA user_version after rollback = %d, want 0", userVer)
	}

	// Retry with valid schema must succeed cleanly.
	if err := initSchema(testDB); err != nil {
		t.Fatalf("retry initSchema() after rollback error: %v", err)
	}
	for _, tbl := range []string{"user_list", "alist_list", "job", "job_task", "job_task_item", "job_task_item_fts", "notify"} {
		if !tableExists(testDB, tbl) {
			t.Fatalf("expected table %q after clean retry", tbl)
		}
	}
}

func TestSchemaVersionPropagatesDBErrors(t *testing.T) {
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	_ = testDB.Close()

	if _, err := schemaVersion(testDB); err == nil {
		t.Fatalf("schemaVersion(closedDB) error = nil, want error")
	}
}

func TestSchemaVersionNeverRegressesOnStaleColumnDefault(t *testing.T) {
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	defer testDB.Close()
	defer SetDBForTest(testDB)()

	// Simulate a DB originally created by version 260612 with a stale column DEFAULT.
	if _, err := testDB.Exec(`CREATE TABLE user_list(
		id integer primary key autoincrement,
		userName text,
		passwd text,
		recoveryKey text,
		sqlVersion integer DEFAULT 260612
	)`); err != nil {
		t.Fatalf("create user_list: %v", err)
	}
	if _, err := testDB.Exec("INSERT INTO user_list(userName, passwd, sqlVersion) VALUES ('admin', 'x', 260612)"); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := migrateDBTx(testDB, 260612); err != nil {
		t.Fatalf("migrateDBTx() error: %v", err)
	}

	// Wipe users and re-create via CreateInitialUser; sqlVersion must be currentVersion.
	if _, err := testDB.Exec("DELETE FROM user_list"); err != nil {
		t.Fatalf("delete users: %v", err)
	}
	if _, err := CreateInitialUser("admin2", "hash", "rec"); err != nil {
		t.Fatalf("CreateInitialUser() error: %v", err)
	}
	var rowVer int
	if err := testDB.QueryRow("SELECT sqlVersion FROM user_list WHERE userName='admin2'").Scan(&rowVer); err != nil {
		t.Fatalf("read sqlVersion: %v", err)
	}
	if rowVer != currentVersion {
		t.Fatalf("CreateInitialUser sqlVersion = %d, want %d", rowVer, currentVersion)
	}

	// Even if a row is inserted via raw SQL triggering the stale 260612 DEFAULT,
	// schemaVersion() must still return currentVersion via PRAGMA user_version.
	if _, err := testDB.Exec("DELETE FROM user_list"); err != nil {
		t.Fatalf("delete users: %v", err)
	}
	if _, err := testDB.Exec("INSERT INTO user_list(userName, passwd) VALUES ('legacy-insert', 'hash')"); err != nil {
		t.Fatalf("raw insert user: %v", err)
	}
	ver, err := schemaVersion(testDB)
	if err != nil {
		t.Fatalf("schemaVersion() error: %v", err)
	}
	if ver != currentVersion {
		t.Fatalf("schemaVersion() = %d, want %d (must not regress to stale column DEFAULT)", ver, currentVersion)
	}
}

func TestInitSchemaRebuildsPre260614FTSWithEmptyUserList(t *testing.T) {
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	defer testDB.Close()
	defer SetDBForTest(testDB)()

	// Simulate a pre-260614 DB with empty user_list (PRAGMA user_version == 0)
	// and an outdated FTS5 table without trigram or alistTaskId/errMsg columns.
	for _, stmt := range []string{
		`CREATE TABLE user_list(
			id integer primary key autoincrement,
			userName text,
			passwd text,
			recoveryKey text,
			sqlVersion integer DEFAULT 260612,
			createTime integer DEFAULT (strftime('%s', 'now'))
		)`,
		`CREATE TABLE job(
			id integer primary key autoincrement,
			enable integer DEFAULT 1,
			remark text,
			srcPath text,
			dstPath text,
			alistId integer,
			useCacheT integer DEFAULT 0,
			scanIntervalT integer DEFAULT 0,
			useCacheS integer DEFAULT 0,
			scanIntervalS integer DEFAULT 0,
			method integer,
			interval integer,
			isCron integer DEFAULT 0,
			month text DEFAULT NULL,
			day text DEFAULT NULL,
			day_of_week text DEFAULT NULL,
			hour text DEFAULT NULL,
			minute text DEFAULT NULL,
			second text DEFAULT NULL,
			exclude text DEFAULT NULL,
			minFileSize integer DEFAULT 0,
			maxFileSize integer DEFAULT 0,
			createTime integer DEFAULT (strftime('%s', 'now'))
		)`,
		`CREATE TABLE job_task(
			id integer primary key autoincrement,
			jobId integer,
			status integer DEFAULT 1,
			errMsg text,
			runTime integer,
			taskNum text,
			createTime integer DEFAULT (strftime('%s', 'now'))
		)`,
		`CREATE TABLE job_task_item(
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
		)`,
		`CREATE VIRTUAL TABLE job_task_item_fts USING fts5(
			fileName,
			content='job_task_item',
			content_rowid='id'
		)`,
		`INSERT INTO job_task_item(id, taskId, srcPath, dstPath, fileName, alistTaskId, errMsg)
		 VALUES (1, 10, '/src/alpha.mkv', '/dst/alpha.mkv', 'alpha.mkv', 'copy_xyz_999', 'network stalled')`,
	} {
		if _, err := testDB.Exec(stmt); err != nil {
			t.Fatalf("setup SQL %q failed: %v", stmt, err)
		}
	}

	if err := initSchema(testDB); err != nil {
		t.Fatalf("initSchema() error: %v", err)
	}

	// Substring trigram searches across fileName, alistTaskId, and errMsg must match.
	for _, needle := range []string{"lpha.m", "xyz_9", "stalled"} {
		var rowID int64
		if err := testDB.QueryRow("SELECT rowid FROM job_task_item_fts WHERE job_task_item_fts MATCH ?", fts5Phrase(needle)).Scan(&rowID); err != nil {
			t.Fatalf("rebuilt FTS5 failed to match %q: %v", needle, err)
		}
		if rowID != 1 {
			t.Fatalf("rebuilt FTS5 rowid for %q = %d, want 1", needle, rowID)
		}
	}
}

func TestMigrationHelpersPropagateErrors(t *testing.T) {
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	_ = testDB.Close()

	ctx := context.Background()
	if _, err := tableExistsContext(ctx, testDB, "user_list"); err == nil {
		t.Fatalf("tableExistsContext(closedDB) error = nil, want error")
	}
	if _, err := tableHasColumnDB(testDB, "user_list", "sqlVersion"); err == nil {
		t.Fatalf("tableHasColumnDB(closedDB) error = nil, want error")
	}
	if _, err := shouldSkipMigrationStatement(ctx, testDB, "ALTER TABLE job DROP COLUMN cron"); err == nil {
		t.Fatalf("shouldSkipMigrationStatement(closedDB) error = nil, want error")
	}
}

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

func TestMigrateDBTxDropsLegacyCronColumn(t *testing.T) {
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
	if _, err := testDB.Exec("INSERT INTO user_list(userName, passwd, sqlVersion) VALUES ('admin', 'x', 260612)"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := testDB.Exec(`CREATE TABLE job(
		id integer primary key autoincrement,
		cron text DEFAULT NULL,
		isCron integer DEFAULT 0,
		month text DEFAULT NULL,
		day text DEFAULT NULL,
		day_of_week text DEFAULT NULL,
		hour text DEFAULT NULL,
		minute text DEFAULT NULL,
		second text DEFAULT NULL
	)`); err != nil {
		t.Fatalf("create job: %v", err)
	}

	if err := migrateDBTx(testDB, 260612); err != nil {
		t.Fatalf("migrateDBTx() error: %v", err)
	}

	if tableHasColumn(testDB, "job", "cron") {
		t.Fatalf("job table still has legacy cron column")
	}
	for _, column := range []string{"isCron", "month", "day", "day_of_week", "hour", "minute", "second"} {
		if !tableHasColumn(testDB, "job", column) {
			t.Fatalf("job table missing active scheduling column %q", column)
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
		runTime integer,
		createTime integer
	)`); err != nil {
		t.Fatalf("create job_task: %v", err)
	}
	if _, err := testDB.Exec(`CREATE TABLE job_task_item(
		id integer primary key autoincrement,
		taskId integer,
		status integer,
		type integer,
		isPath integer,
		createTime integer
	)`); err != nil {
		t.Fatalf("create job_task_item: %v", err)
	}
	if _, err := testDB.Exec(`CREATE INDEX idx_job_task_item_task_status ON job_task_item(taskId, status)`); err != nil {
		t.Fatalf("create legacy redundant index: %v", err)
	}

	if err := ensureIndexes(testDB); err != nil {
		t.Fatalf("ensureIndexes() error: %v", err)
	}

	if indexExists(testDB, "idx_job_task_item_task_status") {
		t.Fatalf("expected redundant idx_job_task_item_task_status to be dropped")
	}
	if !indexExists(testDB, "idx_job_task_item_task_status_time") {
		t.Fatalf("expected idx_job_task_item_task_status_time to exist")
	}
	if !indexExists(testDB, "idx_job_task_item_task_type_time") {
		t.Fatalf("expected idx_job_task_item_task_type_time to exist")
	}
	if !indexExists(testDB, "idx_job_task_item_task_is_path_time") {
		t.Fatalf("expected idx_job_task_item_task_is_path_time to exist")
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
	if err := testDB.QueryRow("SELECT rowid FROM job_task_item_fts WHERE job_task_item_fts MATCH ?", fts5Phrase("cat.jpg")).Scan(&rowID); err != nil {
		t.Fatalf("query rebuilt fts row: %v", err)
	}
	if rowID != 1 {
		t.Fatalf("rebuilt fts rowid = %d, want 1", rowID)
	}

	if _, err := testDB.Exec(`INSERT INTO job_task_item(id, taskId, fileName, alistTaskId)
		VALUES (2, 10, 'movie.mkv', 'move_task_004')`); err != nil {
		t.Fatalf("insert triggered task item: %v", err)
	}
	if err := testDB.QueryRow("SELECT rowid FROM job_task_item_fts WHERE job_task_item_fts MATCH ?", fts5Phrase("movie.mkv")).Scan(&rowID); err != nil {
		t.Fatalf("query triggered fts row: %v", err)
	}
	if rowID != 2 {
		t.Fatalf("triggered fts rowid = %d, want 2", rowID)
	}
}

func TestMigrateDBTxAddsRecoveryKeyColumn(t *testing.T) {
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
	if _, err := testDB.Exec("INSERT INTO user_list(userName, passwd, sqlVersion) VALUES ('admin', 'x', 260611)"); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := migrateDBTx(testDB, 260611); err != nil {
		t.Fatalf("migrateDBTx() error: %v", err)
	}

	if !tableHasColumn(testDB, "user_list", "recoveryKey") {
		t.Fatalf("user_list missing migrated recoveryKey column")
	}
	var version int64
	if err := testDB.QueryRow("SELECT sqlVersion FROM user_list LIMIT 1").Scan(&version); err != nil {
		t.Fatalf("read sqlVersion: %v", err)
	}
	if version != currentVersion {
		t.Fatalf("sqlVersion = %d, want currentVersion %d", version, currentVersion)
	}
}

func TestMigrateDBTxAddsJobTaskAlistIDColumn(t *testing.T) {
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	defer testDB.Close()

	for _, stmt := range []string{
		`CREATE TABLE user_list(id integer primary key autoincrement, userName text, passwd text, sqlVersion integer)`,
		`INSERT INTO user_list(userName, passwd, sqlVersion) VALUES ('admin', 'x', 260616)`,
		`CREATE TABLE job_task(id integer primary key autoincrement, jobId integer, status integer, errMsg text, runTime integer, taskNum text, createTime integer)`,
	} {
		if _, err := testDB.Exec(stmt); err != nil {
			t.Fatalf("setup SQL %q: %v", stmt, err)
		}
	}

	if err := migrateDBTx(testDB, 260616); err != nil {
		t.Fatalf("migrateDBTx(260616) error: %v", err)
	}
	if !tableHasColumn(testDB, "job_task", "alistId") {
		t.Fatalf("job_task missing migrated alistId column")
	}
	var version int64
	if err := testDB.QueryRow("SELECT sqlVersion FROM user_list LIMIT 1").Scan(&version); err != nil {
		t.Fatalf("read sqlVersion: %v", err)
	}
	if version != currentVersion {
		t.Fatalf("sqlVersion = %d, want %d", version, currentVersion)
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
