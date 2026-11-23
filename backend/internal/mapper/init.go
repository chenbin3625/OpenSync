package mapper

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"opensync/internal/config"
	"strings"
)

const currentVersion = 260616

const (
	ftsCreateTableSQL = `CREATE VIRTUAL TABLE IF NOT EXISTS job_task_item_fts USING fts5(
			fileName,
			srcPath,
			dstPath,
			alistTaskId,
			errMsg,
			content='job_task_item',
			content_rowid='id',
			tokenize='trigram'
		)`
	ftsRebuildSQL       = "INSERT INTO job_task_item_fts(job_task_item_fts) VALUES('rebuild')"
	ftsTriggerInsertSQL = `CREATE TRIGGER IF NOT EXISTS job_task_item_ai AFTER INSERT ON job_task_item BEGIN
			INSERT INTO job_task_item_fts(rowid, fileName, srcPath, dstPath, alistTaskId, errMsg)
			VALUES (new.id, new.fileName, new.srcPath, new.dstPath, new.alistTaskId, new.errMsg);
		END`
	ftsTriggerDeleteSQL = `CREATE TRIGGER IF NOT EXISTS job_task_item_ad AFTER DELETE ON job_task_item BEGIN
			INSERT INTO job_task_item_fts(job_task_item_fts, rowid, fileName, srcPath, dstPath, alistTaskId, errMsg)
			VALUES ('delete', old.id, old.fileName, old.srcPath, old.dstPath, old.alistTaskId, old.errMsg);
		END`
	ftsTriggerUpdateSQL = `CREATE TRIGGER IF NOT EXISTS job_task_item_au AFTER UPDATE ON job_task_item BEGIN
			INSERT INTO job_task_item_fts(job_task_item_fts, rowid, fileName, srcPath, dstPath, alistTaskId, errMsg)
			VALUES ('delete', old.id, old.fileName, old.srcPath, old.dstPath, old.alistTaskId, old.errMsg);
			INSERT INTO job_task_item_fts(rowid, fileName, srcPath, dstPath, alistTaskId, errMsg)
			VALUES (new.id, new.fileName, new.srcPath, new.dstPath, new.alistTaskId, new.errMsg);
		END`
)

type sqlExecer interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}

type sqlContextExecer interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

type sqlContextQueryer interface {
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

// InitSQL initializes the database schema and runs migrations
func InitSQL() {
	handle := GetDB()
	if err := initSchema(handle); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	tightenSQLiteFilePermissions(config.GetConfig().DB.DBName)
}

func initSchema(db *sql.DB) error {
	return initSchemaWithStmts(db, firstRunSchemaStatements())
}

func initSchemaWithStmts(db *sql.DB, createStmts []string) error {
	ctx := context.Background()

	exists, err := tableExistsContext(ctx, db, "user_list")
	if err != nil {
		// A transient failure (locked DB, I/O error) must not be mistaken
		// for an empty database, or the "first run" branch would try to
		// re-create the schema on top of existing data.
		return fmt.Errorf("inspect database schema: %w", err)
	}

	if !exists {
		initializedFresh := false
		if err := runImmediateTx(ctx, db, func(ctx context.Context, conn *sql.Conn) error {
			// Re-check inside the write transaction in case another process
			// initialized the schema concurrently.
			nowExists, err := tableExistsContext(ctx, conn, "user_list")
			if err != nil {
				return fmt.Errorf("re-check user_list table: %w", err)
			}
			if nowExists {
				return nil
			}
			for _, stmt := range createStmts {
				if _, err := conn.ExecContext(ctx, stmt); err != nil {
					return fmt.Errorf("first-run DDL failed: %w\nSQL: %s", err, stmt)
				}
			}
			if err := ensureIndexesExec(ctx, conn); err != nil {
				return fmt.Errorf("first-run index creation failed: %w", err)
			}
			if _, err := conn.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version=%d", currentVersion)); err != nil {
				return fmt.Errorf("set initial PRAGMA user_version: %w", err)
			}
			initializedFresh = true
			return nil
		}); err != nil {
			return err
		}
		if initializedFresh {
			resetFTSAvailability()
			log.Printf("Database initialized; waiting for web account setup")
			return nil
		}
	}

	// Existing database - check version and migrate if needed.
	ver, err := schemaVersion(db)
	if err != nil {
		return fmt.Errorf("read database schema version: %w", err)
	}

	if ver < currentVersion {
		if err := migrateDBTx(db, int64(ver)); err != nil {
			return fmt.Errorf("migrate database from version %d to %d: %w", ver, currentVersion, err)
		}
		log.Printf("Database migrated from version %d to %d", ver, currentVersion)
	} else {
		// Ensure PRAGMA user_version is synced even on DBs that already had
		// user_list.sqlVersion == currentVersion from an older binary.
		var userVer int
		if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&userVer); err != nil {
			return fmt.Errorf("read PRAGMA user_version: %w", err)
		}
		if userVer < currentVersion {
			if _, err := db.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version=%d", currentVersion)); err != nil {
				return fmt.Errorf("sync PRAGMA user_version: %w", err)
			}
		}
	}

	if err := ensureIndexes(db); err != nil {
		// Non-fatal on existing databases (the schema works without indexes),
		// but loud: silent degradation would show up only as unexplained slowness.
		log.Printf("WARNING: index creation failed, query performance may degrade: %v", err)
	}
	resetFTSAvailability()
	return nil
}

func firstRunSchemaStatements() []string {
	stmts := []string{
		fmt.Sprintf(`CREATE TABLE user_list(
				id integer primary key autoincrement,
				userName text,
				passwd text,
				recoveryKey text,
				sessionVersion integer NOT NULL DEFAULT 1,
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
	return stmts
}

func schemaVersion(db *sql.DB) (int, error) {
	ctx := context.Background()

	var userVer int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&userVer); err != nil {
		return 0, fmt.Errorf("read PRAGMA user_version: %w", err)
	}

	hasCol, err := tableHasColumnDB(db, "user_list", "sqlVersion")
	if err != nil {
		return 0, fmt.Errorf("check user_list.sqlVersion column: %w", err)
	}
	if !hasCol {
		return userVer, nil
	}

	var rowVer sql.NullInt64
	err = db.QueryRowContext(ctx, "SELECT sqlVersion FROM user_list LIMIT 1").Scan(&rowVer)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// user_list is empty: PRAGMA user_version tracks the schema version
			// on fresh/migrated databases, and is 0 on legacy pre-user_version
			// databases so idempotent migrations run.
			return userVer, nil
		}
		return 0, fmt.Errorf("read user_list.sqlVersion: %w", err)
	}

	if rowVer.Valid && int(rowVer.Int64) > userVer {
		return int(rowVer.Int64), nil
	}
	return userVer, nil
}

func indexStatements() []string {
	return []string{
		"CREATE INDEX IF NOT EXISTS idx_job_task_job_time ON job_task(jobId, createTime DESC, id DESC)",
		"CREATE INDEX IF NOT EXISTS idx_job_task_status_job ON job_task(status, jobId)",
		"CREATE INDEX IF NOT EXISTS idx_job_task_effective_time ON job_task(COALESCE(NULLIF(runTime, 0), createTime), id)",
		"CREATE INDEX IF NOT EXISTS idx_job_task_item_task_time ON job_task_item(taskId, createTime DESC, id DESC)",
		// idx_job_task_item_task_status_time fully covers (taskId, status) queries,
		// so drop the redundant prefix-only index.
		"DROP INDEX IF EXISTS idx_job_task_item_task_status",
		"CREATE INDEX IF NOT EXISTS idx_job_task_item_task_status_time ON job_task_item(taskId, status, createTime DESC, id DESC)",
		// The time+id suffix lets type/object-filtered detail pages stream rows in
		// deterministic order without a temp B-tree sort.
		"CREATE INDEX IF NOT EXISTS idx_job_task_item_task_type_time ON job_task_item(taskId, type, createTime DESC, id DESC)",
		"DROP INDEX IF EXISTS idx_job_task_item_task_type",
		"CREATE INDEX IF NOT EXISTS idx_job_task_item_task_is_path_time ON job_task_item(taskId, isPath, createTime DESC, id DESC)",
	}
}

func ensureIndexesExec(ctx context.Context, exec sqlContextExecer) error {
	for _, stmt := range indexStatements() {
		if _, err := exec.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("%w\nSQL: %s", err, stmt)
		}
	}
	return nil
}

func ensureIndexes(db *sql.DB) error {
	ctx := context.Background()
	var firstErr error
	for _, stmt := range indexStatements() {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			log.Printf("WARNING: index creation failed, query performance may degrade: %v\nSQL: %s", err, stmt)
			if firstErr == nil {
				firstErr = fmt.Errorf("%w\nSQL: %s", err, stmt)
			}
		}
	}
	return firstErr
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
	if fromVersion < 260612 {
		stmts = append(stmts, "ALTER TABLE user_list ADD COLUMN recoveryKey text")
	}
	if fromVersion < 260613 {
		// Drop the legacy single 'cron' column carried over from the earlier
		// Python implementation; scheduling now uses the dedicated isCron +
		// second/minute/hour/day/month/day_of_week fields.
		stmts = append(stmts, "ALTER TABLE job DROP COLUMN cron")
	}
	if fromVersion < 260615 {
		stmts = append(stmts,
			"ALTER TABLE user_list ADD COLUMN sessionVersion integer NOT NULL DEFAULT 1",
			"UPDATE user_list SET sessionVersion=1 WHERE sessionVersion IS NULL OR sessionVersion <= 0",
		)
	}
	if fromVersion < 260616 {
		// Rebuild FTS5 for all pre-260616 databases (including pre-260614 DBs
		// with empty user_list where fromVersion==0) so job_task_item_fts
		// indexes fileName, srcPath, dstPath, alistTaskId, and errMsg with
		// the trigram tokenizer.
		stmts = append(stmts, rebuildJobTaskItemFTSStatements()...)
		// Drop legacy indexes that lacked the trailing id DESC tie-breaker so
		// ensureIndexes recreates them with sort-free deterministic ordering.
		stmts = append(stmts,
			"DROP INDEX IF EXISTS idx_job_task_job_time",
			"DROP INDEX IF EXISTS idx_job_task_item_task_time",
			"DROP INDEX IF EXISTS idx_job_task_item_task_status",
			"DROP INDEX IF EXISTS idx_job_task_item_task_status_time",
			"DROP INDEX IF EXISTS idx_job_task_item_task_type_time",
			"DROP INDEX IF EXISTS idx_job_task_item_task_is_path_time",
		)
	}
	stmts = append(stmts,
		fmt.Sprintf("UPDATE user_list SET sqlVersion=%d", currentVersion),
		fmt.Sprintf("PRAGMA user_version=%d", currentVersion),
	)
	return stmts
}

func installJobTaskItemFTS(exec sqlExecer, rebuild bool) error {
	for _, stmt := range jobTaskItemFTSStatements(rebuild) {
		if _, err := exec.Exec(stmt); err != nil {
			return err
		}
	}
	resetFTSAvailability()
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
	stmts := []string{ftsCreateTableSQL}
	if rebuild {
		stmts = append(stmts, ftsRebuildSQL)
	}
	stmts = append(stmts, ftsTriggerInsertSQL, ftsTriggerDeleteSQL, ftsTriggerUpdateSQL)
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
	ctx := context.Background()
	err := runImmediateTx(ctx, db, func(ctx context.Context, conn *sql.Conn) error {
		for _, stmt := range migrationStatements(fromVersion) {
			skip, err := shouldSkipMigrationStatement(ctx, conn, stmt)
			if err != nil {
				return fmt.Errorf("migration metadata check failed: %w\nSQL: %s", err, stmt)
			}
			if skip {
				continue
			}
			if _, err := conn.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("migration SQL failed: %w\nSQL: %s", err, stmt)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	resetFTSAvailability()
	return nil
}

func shouldSkipMigrationStatement(ctx context.Context, q sqlContextQueryer, stmt string) (bool, error) {
	if tableName, columnName, ok := parseAlterTableAddColumn(stmt); ok {
		return tableHasColumnContext(ctx, q, tableName, columnName)
	}

	switch stmt {
	case "ALTER TABLE job RENAME COLUMN speed TO useCacheT":
		hasSpeed, err := tableHasColumnContext(ctx, q, "job", "speed")
		if err != nil {
			return false, err
		}
		hasUseCacheT, err := tableHasColumnContext(ctx, q, "job", "useCacheT")
		if err != nil {
			return false, err
		}
		return !hasSpeed && hasUseCacheT, nil
	case "ALTER TABLE job DROP COLUMN cron":
		hasCol, err := tableHasColumnContext(ctx, q, "job", "cron")
		return !hasCol, err
	case "ALTER TABLE job DROP COLUMN year":
		hasCol, err := tableHasColumnContext(ctx, q, "job", "year")
		return !hasCol, err
	case "ALTER TABLE job DROP COLUMN week":
		hasCol, err := tableHasColumnContext(ctx, q, "job", "week")
		return !hasCol, err
	case "ALTER TABLE job DROP COLUMN start_date":
		hasCol, err := tableHasColumnContext(ctx, q, "job", "start_date")
		return !hasCol, err
	case "ALTER TABLE job DROP COLUMN end_date":
		hasCol, err := tableHasColumnContext(ctx, q, "job", "end_date")
		return !hasCol, err
	case ftsCreateTableSQL:
		exists, err := tableExistsContext(ctx, q, "job_task_item")
		return !exists, err
	case ftsRebuildSQL:
		exists, err := tableExistsContext(ctx, q, "job_task_item_fts")
		return !exists, err
	case ftsTriggerInsertSQL, ftsTriggerDeleteSQL, ftsTriggerUpdateSQL:
		hasItems, err := tableExistsContext(ctx, q, "job_task_item")
		if err != nil {
			return false, err
		}
		hasFTS, err := tableExistsContext(ctx, q, "job_task_item_fts")
		if err != nil {
			return false, err
		}
		return !hasItems || !hasFTS, nil
	default:
		return false, nil
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

func tableExistsContext(ctx context.Context, q sqlContextQueryer, tableName string) (bool, error) {
	var name string
	err := q.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name=?", tableName).Scan(&name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return name == tableName, nil
}

func tableHasColumnContext(ctx context.Context, q sqlContextQueryer, tableName, columnName string) (bool, error) {
	if !isSafeSQLIdentifier(tableName) {
		return false, fmt.Errorf("unsafe SQL identifier: %q", tableName)
	}
	rows, err := q.QueryContext(ctx, "PRAGMA table_info("+tableName+")")
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue interface{}
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if name == columnName {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, nil
}

func tableHasColumnDB(db *sql.DB, tableName, columnName string) (bool, error) {
	return tableHasColumnContext(context.Background(), db, tableName, columnName)
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
