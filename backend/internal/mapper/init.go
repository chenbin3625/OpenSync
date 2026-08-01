package mapper

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
)

const currentVersion = 260614

// InitSQL initializes the database schema and runs migrations
func InitSQL() {
	db := GetDB()

	// Check if user_list table exists
	var name string
	err := db.QueryRow("SELECT name FROM sqlite_master WHERE name='user_list'").Scan(&name)

	if err != nil {
		// First run - create all tables
		stmts := []string{
			fmt.Sprintf(`CREATE TABLE user_list(
				id integer primary key autoincrement,
				userName text,
				passwd text,
				recoveryKey text,
				sqlVersion integer DEFAULT %d,
				createTime integer DEFAULT (strftime('%%s', 'now')),
				UNIQUE (userName)
			)`, currentVersion),

			`CREATE TABLE alist_list(
				id integer primary key autoincrement,
				remark text,
				url text,
				userName text,
				token text,
				createTime integer DEFAULT (strftime('%s', 'now')),
				UNIQUE (url, userName)
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
				createTime integer DEFAULT (strftime('%s', 'now')),
				UNIQUE (srcPath, dstPath, alistId)
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
		}

		stmts = append(stmts, jobTaskItemFTSStatements(false)...)
		stmts = append(stmts,

			`CREATE TABLE notify(
				id integer primary key autoincrement,
				enable integer DEFAULT 1,
				method integer,
				params text,
				createTime integer DEFAULT (strftime('%s', 'now'))
			)`,
		)

		for _, stmt := range stmts {
			if _, err := db.Exec(stmt); err != nil {
				log.Fatalf("Failed to initialize database: %v\nSQL: %s", err, stmt)
			}
		}
		ensureIndexes(db)

		log.Printf("Database initialized; waiting for web account setup")
		return
	}

	// Existing database - check version and migrate if needed. A fresh schema
	// can exist without any user rows when the server was stopped before web
	// setup completed. schemaVersion() returns 0 in that case so the idempotent
	// migrations run (a no-op on an already-current schema), which is important
	// when the schema was created by an older binary missing newer columns.
	sqlVersion := schemaVersion(db)

	if sqlVersion < int64(currentVersion) {
		if err := migrateDB(sqlVersion); err != nil {
			log.Fatalf("Failed to migrate database from version %d to %d: %v", sqlVersion, currentVersion, err)
		}
	}
	ensureIndexes(db)
}

func schemaVersion(db *sql.DB) int64 {
	if !tableHasColumnDB(db, "user_list", "sqlVersion") {
		return 0
	}

	var sqlVersion int64
	err := db.QueryRow("SELECT sqlVersion FROM user_list LIMIT 1").Scan(&sqlVersion)
	if err == nil {
		return sqlVersion
	}
	if err == sql.ErrNoRows {
		// user_list exists with the sqlVersion column but has no rows. This can
		// happen when an older binary created the table (with sqlVersion but
		// missing newer columns) and web setup was never completed. Returning 0
		// runs the idempotent migrations to bring the schema up to date before
		// first-run setup; migrateDBTx(0) on an already-current schema is a
		// no-op. Returning currentVersion here would skip migrations and leave
		// the schema stale, breaking InitializeUser (e.g. missing recoveryKey).
		return 0
	}
	return 0
}

func ensureIndexes(db *sql.DB) {
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_job_task_job_time ON job_task(jobId, createTime DESC)",
		"CREATE INDEX IF NOT EXISTS idx_job_task_status_job ON job_task(status, jobId)",
		"CREATE INDEX IF NOT EXISTS idx_job_task_effective_time ON job_task(COALESCE(NULLIF(runTime, 0), createTime), id)",
		"CREATE INDEX IF NOT EXISTS idx_job_task_item_task_time ON job_task_item(taskId, createTime DESC)",
		"CREATE INDEX IF NOT EXISTS idx_job_task_item_task_status ON job_task_item(taskId, status)",
		"CREATE INDEX IF NOT EXISTS idx_job_task_item_task_status_time ON job_task_item(taskId, status, createTime DESC)",
		"CREATE INDEX IF NOT EXISTS idx_job_task_item_task_type ON job_task_item(taskId, type)",
		// Enforce userName uniqueness on existing databases so the first-run
		// InitializeUser endpoint cannot create a duplicate admin account under
		// a TOCTOU race. Fresh databases get it via the CREATE TABLE statement.
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_user_list_username ON user_list(userName)",
	}
	for _, stmt := range indexes {
		if _, err := db.Exec(stmt); err != nil {
			log.Printf("Index creation failed: %v\nSQL: %s", err, stmt)
		}
	}
}

func migrationStatements(fromVersion int64) []string {
	var stmts []string
	if fromVersion < 240731 {
		stmts = append(stmts,
			fmt.Sprintf("ALTER TABLE user_list ADD COLUMN sqlVersion integer DEFAULT %d", currentVersion),
			"ALTER TABLE job_task ADD COLUMN errMsg text",
		)
	}
	if fromVersion < 240813 {
		stmts = append(stmts,
			"ALTER TABLE job ADD COLUMN isCron integer DEFAULT 0",
			"ALTER TABLE job ADD COLUMN month text DEFAULT NULL",
			"ALTER TABLE job ADD COLUMN day text DEFAULT NULL",
			"ALTER TABLE job ADD COLUMN day_of_week text DEFAULT NULL",
			"ALTER TABLE job ADD COLUMN hour text DEFAULT NULL",
			"ALTER TABLE job ADD COLUMN minute text DEFAULT NULL",
			"ALTER TABLE job ADD COLUMN second text DEFAULT NULL",
		)
	}
	if fromVersion < 240905 {
		stmts = append(stmts, "ALTER TABLE job ADD COLUMN exclude text DEFAULT NULL")
	}
	if fromVersion < 241014 {
		stmts = append(stmts, `CREATE TABLE IF NOT EXISTS notify(
			id integer primary key autoincrement,
			enable integer DEFAULT 1,
			method integer,
			params text,
			createTime integer DEFAULT (strftime('%s', 'now'))
		)`)
	}
	if fromVersion < 250307 {
		stmts = append(stmts, "ALTER TABLE job_task ADD COLUMN taskNum text")
	}
	if fromVersion < 250416 {
		stmts = append(stmts, "ALTER TABLE job ADD COLUMN remark text")
	}
	if fromVersion < 250520 {
		stmts = append(stmts, "ALTER TABLE job_task_item ADD COLUMN isPath integer DEFAULT 0")
	}
	if fromVersion < 250608 {
		stmts = append(stmts,
			"ALTER TABLE job RENAME COLUMN speed TO useCacheT",
			"ALTER TABLE job ADD COLUMN scanIntervalT integer DEFAULT 0",
			"ALTER TABLE job ADD COLUMN useCacheS integer DEFAULT 0",
			"ALTER TABLE job ADD COLUMN scanIntervalS integer DEFAULT 0",
			"UPDATE job SET scanIntervalT = 10, useCacheT = 0 WHERE useCacheT = 2",
		)
	}
	if fromVersion < 260605 {
		stmts = append(stmts,
			"ALTER TABLE job ADD COLUMN minFileSize integer DEFAULT 0",
			"ALTER TABLE job ADD COLUMN maxFileSize integer DEFAULT 0",
		)
	}
	if fromVersion < 260606 {
		stmts = append(stmts,
			"ALTER TABLE job DROP COLUMN year",
			"ALTER TABLE job DROP COLUMN week",
			"ALTER TABLE job DROP COLUMN start_date",
			"ALTER TABLE job DROP COLUMN end_date",
		)
	}
	if fromVersion < 260611 {
		stmts = append(stmts, jobTaskItemFTSStatements(true)...)
	}
	if fromVersion < 260612 {
		stmts = append(stmts, "ALTER TABLE user_list ADD COLUMN recoveryKey text")
	}
	if fromVersion < 260613 {
		// Drop the legacy single 'cron' column carried over from the earlier
		// Python implementation; scheduling now uses the dedicated isCron +
		// second/minute/hour/day/month/day_of_week fields.
		stmts = append(stmts, "ALTER TABLE job DROP COLUMN cron")
	}
	if fromVersion >= 260611 && fromVersion < 260614 {
		stmts = append(stmts, rebuildJobTaskItemFTSStatements()...)
	}
	stmts = append(stmts, fmt.Sprintf("UPDATE user_list SET sqlVersion=%d", currentVersion))
	return stmts
}

type sqlExecer interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}

func installJobTaskItemFTS(exec sqlExecer, rebuild bool) error {
	for _, stmt := range jobTaskItemFTSStatements(rebuild) {
		if _, err := exec.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func rebuildJobTaskItemFTSStatements() []string {
	stmts := []string{
		"DROP TRIGGER IF EXISTS job_task_item_ai",
		"DROP TRIGGER IF EXISTS job_task_item_ad",
		"DROP TRIGGER IF EXISTS job_task_item_au",
		"DROP TABLE IF EXISTS job_task_item_fts",
	}
	return append(stmts, jobTaskItemFTSStatements(true)...)
}

func jobTaskItemFTSStatements(rebuild bool) []string {
	stmts := []string{
		`CREATE VIRTUAL TABLE IF NOT EXISTS job_task_item_fts USING fts5(
			fileName,
			srcPath,
			dstPath,
			content='job_task_item',
			content_rowid='id',
			tokenize='trigram'
		)`,
	}
	if rebuild {
		stmts = append(stmts, "INSERT INTO job_task_item_fts(job_task_item_fts) VALUES('rebuild')")
	}
	stmts = append(stmts,
		`CREATE TRIGGER IF NOT EXISTS job_task_item_ai AFTER INSERT ON job_task_item BEGIN
			INSERT INTO job_task_item_fts(rowid, fileName, srcPath, dstPath)
			VALUES (new.id, new.fileName, new.srcPath, new.dstPath);
		END`,
		`CREATE TRIGGER IF NOT EXISTS job_task_item_ad AFTER DELETE ON job_task_item BEGIN
			INSERT INTO job_task_item_fts(job_task_item_fts, rowid, fileName, srcPath, dstPath)
			VALUES ('delete', old.id, old.fileName, old.srcPath, old.dstPath);
		END`,
		`CREATE TRIGGER IF NOT EXISTS job_task_item_au AFTER UPDATE ON job_task_item BEGIN
			INSERT INTO job_task_item_fts(job_task_item_fts, rowid, fileName, srcPath, dstPath)
			VALUES ('delete', old.id, old.fileName, old.srcPath, old.dstPath);
			INSERT INTO job_task_item_fts(rowid, fileName, srcPath, dstPath)
			VALUES (new.id, new.fileName, new.srcPath, new.dstPath);
		END`,
	)
	return stmts
}

// migrateDB runs database migrations
func migrateDB(fromVersion int64) error {
	if err := migrateDBTx(GetDB(), fromVersion); err != nil {
		return err
	}
	log.Printf("Database migrated from version %d to %d", fromVersion, currentVersion)
	return nil
}

func migrateDBTx(db *sql.DB, fromVersion int64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	for _, stmt := range migrationStatements(fromVersion) {
		if shouldSkipMigrationStatement(tx, stmt) {
			continue
		}
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("migration SQL failed: %w\nSQL: %s", err, stmt)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

func shouldSkipMigrationStatement(tx *sql.Tx, stmt string) bool {
	if tableName, columnName, ok := parseAlterTableAddColumn(stmt); ok {
		return txTableHasColumn(tx, tableName, columnName)
	}

	switch stmt {
	case "ALTER TABLE job RENAME COLUMN speed TO useCacheT":
		return !txTableHasColumn(tx, "job", "speed") && txTableHasColumn(tx, "job", "useCacheT")
	case "ALTER TABLE job DROP COLUMN cron":
		return !txTableHasColumn(tx, "job", "cron")
	case "ALTER TABLE job DROP COLUMN year":
		return !txTableHasColumn(tx, "job", "year")
	case "ALTER TABLE job DROP COLUMN week":
		return !txTableHasColumn(tx, "job", "week")
	case "ALTER TABLE job DROP COLUMN start_date":
		return !txTableHasColumn(tx, "job", "start_date")
	case "ALTER TABLE job DROP COLUMN end_date":
		return !txTableHasColumn(tx, "job", "end_date")
	case `CREATE VIRTUAL TABLE IF NOT EXISTS job_task_item_fts USING fts5(
			fileName,
			srcPath,
			dstPath,
			content='job_task_item',
			content_rowid='id',
			tokenize='trigram'
		)`:
		return !txTableExists(tx, "job_task_item")
	case "INSERT INTO job_task_item_fts(job_task_item_fts) VALUES('rebuild')":
		return !txTableExists(tx, "job_task_item_fts")
	case `CREATE TRIGGER IF NOT EXISTS job_task_item_ai AFTER INSERT ON job_task_item BEGIN
			INSERT INTO job_task_item_fts(rowid, fileName, srcPath, dstPath)
			VALUES (new.id, new.fileName, new.srcPath, new.dstPath);
		END`:
		return !txTableExists(tx, "job_task_item") || !txTableExists(tx, "job_task_item_fts")
	case `CREATE TRIGGER IF NOT EXISTS job_task_item_ad AFTER DELETE ON job_task_item BEGIN
			INSERT INTO job_task_item_fts(job_task_item_fts, rowid, fileName, srcPath, dstPath)
			VALUES ('delete', old.id, old.fileName, old.srcPath, old.dstPath);
		END`:
		return !txTableExists(tx, "job_task_item") || !txTableExists(tx, "job_task_item_fts")
	case `CREATE TRIGGER IF NOT EXISTS job_task_item_au AFTER UPDATE ON job_task_item BEGIN
			INSERT INTO job_task_item_fts(job_task_item_fts, rowid, fileName, srcPath, dstPath)
			VALUES ('delete', old.id, old.fileName, old.srcPath, old.dstPath);
			INSERT INTO job_task_item_fts(rowid, fileName, srcPath, dstPath)
			VALUES (new.id, new.fileName, new.srcPath, new.dstPath);
		END`:
		return !txTableExists(tx, "job_task_item") || !txTableExists(tx, "job_task_item_fts")
	default:
		return false
	}
}

func parseAlterTableAddColumn(stmt string) (string, string, bool) {
	fields := strings.Fields(stmt)
	if len(fields) < 6 {
		return "", "", false
	}
	if !strings.EqualFold(fields[0], "ALTER") ||
		!strings.EqualFold(fields[1], "TABLE") ||
		!strings.EqualFold(fields[3], "ADD") ||
		!strings.EqualFold(fields[4], "COLUMN") {
		return "", "", false
	}
	tableName := strings.Trim(fields[2], "`\"[]")
	columnName := strings.Trim(fields[5], "`\"[]")
	if !isSafeSQLIdentifier(tableName) || !isSafeSQLIdentifier(columnName) {
		return "", "", false
	}
	return tableName, columnName, true
}

func txTableExists(tx *sql.Tx, tableName string) bool {
	var name string
	err := tx.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", tableName).Scan(&name)
	return err == nil && name == tableName
}

func txTableHasColumn(tx *sql.Tx, tableName, columnName string) bool {
	if !isSafeSQLIdentifier(tableName) {
		return false
	}
	rows, err := tx.Query("PRAGMA table_info(" + tableName + ")")
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

func tableHasColumnDB(db *sql.DB, tableName, columnName string) bool {
	if !isSafeSQLIdentifier(tableName) {
		return false
	}
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

// UpdateAbnormalTasks updates incomplete tasks to aborted status on startup
func UpdateAbnormalTasks() {
	if err := UpdateJobTaskStatusByStatus(); err != nil {
		log.Printf("Failed to update abnormal tasks: %v", err)
	}
}

// GetEnabledJobs returns all enabled jobs for scheduler startup
func GetEnabledJobs() []map[string]interface{} {
	jobs, err := GetEnableJobList()
	if err != nil {
		log.Printf("Failed to get enabled jobs: %v", err)
		return nil
	}
	return jobs
}
