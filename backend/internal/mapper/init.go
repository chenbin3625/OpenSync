package mapper

import (
	"database/sql"
	"fmt"
	"log"
	"opensync/internal/config"
	"opensync/internal/i18n"
	"opensync/pkg/crypto"
	"time"
)

const currentVersion = 250608

// InitSQL initializes the database schema and runs migrations
// Returns the initial admin password if first run, empty string otherwise
func InitSQL() string {
	db := GetDB()

	// Check if user_list table exists
	var name string
	err := db.QueryRow("SELECT name FROM sqlite_master WHERE name='user_list'").Scan(&name)

	var passwd string
	if err != nil {
		// First run - create all tables
		passwd = crypto.GeneratePassword(8)
		cfg := config.GetConfig()
		encPasswd := crypto.PasswordToMD5(passwd, cfg.Server.PasswdStr)

		stmts := []string{
			fmt.Sprintf(`CREATE TABLE user_list(
				id integer primary key autoincrement,
				userName text,
				passwd text,
				sqlVersion integer DEFAULT %d,
				createTime integer DEFAULT (strftime('%%s', 'now'))
			)`, currentVersion),

			fmt.Sprintf(`INSERT INTO user_list(userName, passwd) VALUES ('admin', '%s')`, encPasswd),

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
				year text DEFAULT NULL,
				month text DEFAULT NULL,
				day text DEFAULT NULL,
				week text DEFAULT NULL,
				day_of_week text DEFAULT NULL,
				hour text DEFAULT NULL,
				minute text DEFAULT NULL,
				second text DEFAULT NULL,
				start_date text DEFAULT NULL,
				end_date text DEFAULT NULL,
				exclude text DEFAULT NULL,
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

			`CREATE TABLE notify(
				id integer primary key autoincrement,
				enable integer DEFAULT 1,
				method integer,
				params text,
				createTime integer DEFAULT (strftime('%s', 'now'))
			)`,
		}

		for _, stmt := range stmts {
			if _, err := db.Exec(stmt); err != nil {
				log.Fatalf("Failed to initialize database: %v\nSQL: %s", err, stmt)
			}
		}
		ensureIndexes(db)

		log.Printf("Database initialized with admin password: %s", passwd)
		return passwd
	}

	// Existing database - check version and migrate if needed
	var sqlVersion int64
	err = db.QueryRow("SELECT sqlVersion FROM user_list LIMIT 1").Scan(&sqlVersion)
	if err != nil {
		sqlVersion = 0
	}

	if sqlVersion < int64(currentVersion) {
		if err := migrateDB(sqlVersion); err != nil {
			log.Fatalf("Failed to migrate database from version %d to %d: %v", sqlVersion, currentVersion, err)
		}
	}
	ensureIndexes(db)

	return passwd
}

func ensureIndexes(db *sql.DB) {
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_job_task_job_time ON job_task(jobId, createTime DESC)",
		"CREATE INDEX IF NOT EXISTS idx_job_task_status_job ON job_task(status, jobId)",
		"CREATE INDEX IF NOT EXISTS idx_job_task_item_task_time ON job_task_item(taskId, createTime DESC)",
		"CREATE INDEX IF NOT EXISTS idx_job_task_item_task_status ON job_task_item(taskId, status)",
		"CREATE INDEX IF NOT EXISTS idx_job_task_item_task_type ON job_task_item(taskId, type)",
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
		// SQLite doesn't support DROP COLUMN before 3.35.0, use recreate approach
		// For simplicity, just add new columns (old 'cron' column will remain unused)
		stmts = append(stmts,
			"ALTER TABLE job ADD COLUMN isCron integer DEFAULT 0",
			"ALTER TABLE job ADD COLUMN year text DEFAULT NULL",
			"ALTER TABLE job ADD COLUMN month text DEFAULT NULL",
			"ALTER TABLE job ADD COLUMN day text DEFAULT NULL",
			"ALTER TABLE job ADD COLUMN week text DEFAULT NULL",
			"ALTER TABLE job ADD COLUMN day_of_week text DEFAULT NULL",
			"ALTER TABLE job ADD COLUMN hour text DEFAULT NULL",
			"ALTER TABLE job ADD COLUMN minute text DEFAULT NULL",
			"ALTER TABLE job ADD COLUMN second text DEFAULT NULL",
			"ALTER TABLE job ADD COLUMN start_date text DEFAULT NULL",
			"ALTER TABLE job ADD COLUMN end_date text DEFAULT NULL",
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
	stmts = append(stmts, fmt.Sprintf("UPDATE user_list SET sqlVersion=%d", currentVersion))
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

// InitLogger sets up log file rotation (called at startup and midnight)
func InitLogger() {
	_ = time.Now() // Will be used by log service
	_ = i18n.G("log_rename_start")
}
