package mapper

import (
	"context"
	"database/sql"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"opensync/internal/config"
)

func resetGlobalDBForTest(t *testing.T, cfg *config.Config) {
	t.Helper()
	dbMu.Lock()
	oldDB := db
	db = nil
	resetFTSAvailability()
	dbMu.Unlock()
	oldConfig := config.GetConfig()
	t.Cleanup(func() {
		dbMu.Lock()
		cur := db
		db = oldDB
		resetFTSAvailability()
		dbMu.Unlock()
		if cur != nil && cur != oldDB {
			_ = cur.Close()
		}
		config.SetConfigForTest(oldConfig)
	})

	config.SetConfigForTest(cfg)
}

func TestParsePageParamsRejectsInvalidValues(t *testing.T) {
	cases := []map[string]interface{}{
		{"pageSize": "0", "pageNum": "1"},
		{"pageSize": "-1", "pageNum": "1"},
		{"pageSize": "20", "pageNum": "0"},
		{"pageSize": "abc", "pageNum": "1"},
		{"pageSize": "20", "pageNum": "abc"},
	}

	for _, params := range cases {
		if _, _, _, err := parsePageParams(params); err == nil {
			t.Fatalf("parsePageParams(%v) returned nil error, want error", params)
		}
	}
}

func TestPositiveIntRejectsFloat64OverflowAndSpecialValues(t *testing.T) {
	bad := []interface{}{
		math.NaN(),
		math.Inf(1),
		math.Inf(-1),
		float64(0),
		float64(-1),
		1.5,
		float64(math.MaxInt64), // rounds up to 2^63, saturates to MaxInt64 on ARM64
		math.Ldexp(1, 63),
		1e20,
		1e300,
	}
	for _, val := range bad {
		if got, err := positiveInt(val); err == nil {
			t.Fatalf("positiveInt(%v) = %d, want error", val, got)
		}
	}

	// Largest float64 strictly below 2^63 that fits in int64 (on 64-bit int).
	if math.MaxInt == math.MaxInt64 {
		largest := math.Nextafter(math.Ldexp(1, 63), 0)
		got, err := positiveInt(largest)
		if err != nil {
			t.Fatalf("positiveInt(%v) unexpected error: %v", largest, err)
		}
		if got != int(largest) {
			t.Fatalf("positiveInt(%v) = %d, want %d", largest, got, int(largest))
		}
	}
}

func TestParsePageParamsCapsLargePageSize(t *testing.T) {
	pageSize, pageNum, ok, err := parsePageParams(map[string]interface{}{
		"pageSize": "9999",
		"pageNum":  "2",
	})
	if err != nil {
		t.Fatalf("parsePageParams() error: %v", err)
	}
	if !ok {
		t.Fatalf("parsePageParams() ok = false, want true")
	}
	if pageSize != maxPageSize {
		t.Fatalf("pageSize = %d, want capped maxPageSize %d", pageSize, maxPageSize)
	}
	if pageNum != 2 {
		t.Fatalf("pageNum = %d, want 2", pageNum)
	}
}

func TestParsePageParamsAllowsUnpaginatedRequests(t *testing.T) {
	_, _, ok, err := parsePageParams(map[string]interface{}{})
	if err != nil {
		t.Fatalf("parsePageParams(empty) error: %v", err)
	}
	if ok {
		t.Fatalf("parsePageParams(empty) ok = true, want false")
	}
}

func TestCheckAndAddSQLRejectsUnsafeColumnNames(t *testing.T) {
	_, _, err := CheckAndAddSQL("UPDATE job SET", []string{"remark; DROP TABLE job;--"}, map[string]interface{}{
		"id":                        1,
		"remark; DROP TABLE job;--": "bad",
	})
	if err == nil {
		t.Fatalf("CheckAndAddSQL() error = nil, want unsafe column rejection")
	}
}

func TestInitDBAllowsConcurrentReadConnections(t *testing.T) {
	resetGlobalDBForTest(t, &config.Config{
		DB: config.DBConfig{DBName: filepath.Join(t.TempDir(), "opensync.db")},
	})

	testDB := InitDB()
	if maxOpen := testDB.Stats().MaxOpenConnections; maxOpen <= 1 {
		t.Fatalf("MaxOpenConnections = %d, want more than one read-capable connection", maxOpen)
	}
}

func TestInitDBCapsIsolatedMemoryDBToSingleConnection(t *testing.T) {
	for _, dsn := range []string{
		":memory:",
		"file::memory:",
		"file:isolated?mode=memory",
	} {
		t.Run(dsn, func(t *testing.T) {
			resetGlobalDBForTest(t, &config.Config{
				DB: config.DBConfig{DBName: dsn},
			})
			handle := InitDB()
			if maxOpen := handle.Stats().MaxOpenConnections; maxOpen != 1 {
				t.Fatalf("MaxOpenConnections(%q) = %d, want 1", dsn, maxOpen)
			}
			// Verify table created on connection is visible across subsequent queries.
			if _, err := handle.Exec("CREATE TABLE mem_check(id integer primary key)"); err != nil {
				t.Fatalf("create table: %v", err)
			}
			if _, err := handle.Exec("INSERT INTO mem_check(id) VALUES (1)"); err != nil {
				t.Fatalf("insert: %v", err)
			}
			var count int
			if err := handle.QueryRow("SELECT COUNT(*) FROM mem_check").Scan(&count); err != nil || count != 1 {
				t.Fatalf("count = %d, err = %v; want 1", count, err)
			}
		})
	}
}

func TestInitDBConcurrentGetDBAndCloseDBNoRace(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "concurrent-init.db")
	resetGlobalDBForTest(t, &config.Config{
		DB: config.DBConfig{DBName: dbPath},
	})

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			handle := GetDB()
			if handle == nil {
				t.Errorf("GetDB() returned nil")
				return
			}
			// Every caller must only observe a fully configured pool.
			if maxOpen := handle.Stats().MaxOpenConnections; maxOpen != sqliteMaxOpenConns {
				t.Errorf("MaxOpenConnections = %d, want %d", maxOpen, sqliteMaxOpenConns)
			}
		}()
	}
	wg.Wait()
}

func TestInitDBCreatesDatabaseAndSidecarFilesWithOwnerOnlyPermissions(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "opensync.db")
	resetGlobalDBForTest(t, &config.Config{
		DB: config.DBConfig{DBName: dbPath},
	})

	InitSQL()

	info, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("Stat(db) error: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("db permissions = %v, want 0600", got)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		sidecar, err := os.Stat(dbPath + suffix)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("Stat(%s) error: %v", suffix, err)
		}
		if got := sidecar.Mode().Perm(); got != 0600 {
			t.Fatalf("%s permissions = %v, want 0600", suffix, got)
		}
	}
}

func TestInitDBEnablesIncrementalAutoVacuumOnNewDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "auto-vacuum.db")
	resetGlobalDBForTest(t, &config.Config{
		DB: config.DBConfig{DBName: dbPath},
	})

	InitSQL()

	var autoVacuum int
	if err := GetDB().QueryRow("PRAGMA auto_vacuum").Scan(&autoVacuum); err != nil {
		t.Fatalf("PRAGMA auto_vacuum error: %v", err)
	}
	if autoVacuum != 2 {
		t.Fatalf("PRAGMA auto_vacuum = %d, want 2 (INCREMENTAL)", autoVacuum)
	}
}

func TestCloseDBClosesGlobalHandleAndAllowsReinit(t *testing.T) {
	resetGlobalDBForTest(t, &config.Config{
		DB: config.DBConfig{DBName: filepath.Join(t.TempDir(), "opensync.db")},
	})

	first := InitDB()
	if err := CloseDB(); err != nil {
		t.Fatalf("CloseDB() error: %v", err)
	}
	if db != nil {
		t.Fatalf("CloseDB() left global db set")
	}
	if err := first.Ping(); err == nil {
		t.Fatalf("old DB Ping() error = nil, want closed database error")
	}

	second := InitDB()
	if second == nil {
		t.Fatalf("InitDB() after CloseDB() returned nil")
	}
	if err := second.Ping(); err != nil {
		t.Fatalf("new DB Ping() error: %v", err)
	}
}

func TestSQLiteDSNAppliesTuningPragmasToEveryConnection(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pragma-connections.db")
	testDB, err := sql.Open("sqlite", sqliteDSN(dbPath))
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	defer testDB.Close()
	testDB.SetMaxOpenConns(2)
	testDB.SetMaxIdleConns(2)

	ctx := context.Background()
	conns := make([]*sql.Conn, 0, 2)
	for i := 0; i < 2; i++ {
		conn, err := testDB.Conn(ctx)
		if err != nil {
			t.Fatalf("Conn() error: %v", err)
		}
		defer conn.Close()
		conns = append(conns, conn)
	}

	want := map[string]int64{
		"auto_vacuum":  2, // INCREMENTAL
		"synchronous":  1, // NORMAL
		"cache_size":   -16384,
		"temp_store":   2, // MEMORY
		"mmap_size":    67108864,
		"foreign_keys": 1,
	}
	for i, conn := range conns {
		for pragma, expected := range want {
			var got int64
			if err := conn.QueryRowContext(ctx, "PRAGMA "+pragma).Scan(&got); err != nil {
				t.Fatalf("connection %d: PRAGMA %s error: %v", i, pragma, err)
			}
			if got != expected {
				t.Errorf("connection %d: PRAGMA %s = %d, want %d (DSN tuning must apply to every pooled connection)", i, pragma, got, expected)
			}
		}
	}
}

const pageTotalColumnConstForTest = "__opensync_page_total"

func TestFetchAllToPageUsesSeparateCountAndKeepsPageRowsClean(t *testing.T) {
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	defer testDB.Close()
	restore := SetDBForTest(testDB)
	defer restore()

	if _, err := testDB.Exec(`CREATE TABLE page_item(id integer primary key, name text, createTime integer)`); err != nil {
		t.Fatalf("create page_item: %v", err)
	}
	tx, err := testDB.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	for i := 1; i <= 605; i++ {
		if _, err := tx.Exec(`INSERT INTO page_item(id, name, createTime) VALUES (?, ?, ?)`, i, "item", i); err != nil {
			t.Fatalf("insert page_item %d: %v", i, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	baseSQL := "SELECT id, name, createTime FROM page_item ORDER BY createTime DESC, id DESC"

	first, err := FetchAllToPage(baseSQL, map[string]interface{}{"pageSize": 10, "pageNum": 1})
	if err != nil {
		t.Fatalf("FetchAllToPage(page 1) error: %v", err)
	}
	firstRows := first["dataList"].([]map[string]interface{})
	if len(firstRows) != 10 {
		t.Fatalf("page 1 row count = %d, want 10", len(firstRows))
	}
	if first["count"] != int64(605) {
		t.Fatalf("page 1 count = %v, want 605", first["count"])
	}
	if _, leaked := firstRows[0][pageTotalColumnConstForTest]; leaked {
		t.Fatalf("page row leaked the legacy window count column")
	}
	if _, truncated := first["truncated"]; truncated {
		t.Fatalf("paginated response must not set truncated")
	}

	lastPage, err := FetchAllToPage(baseSQL, map[string]interface{}{"pageSize": 10, "pageNum": 61})
	if err != nil {
		t.Fatalf("FetchAllToPage(page 61) error: %v", err)
	}
	if rows := lastPage["dataList"].([]map[string]interface{}); len(rows) != 5 {
		t.Fatalf("page 61 row count = %d, want 5", len(rows))
	}
	if lastPage["count"] != int64(605) {
		t.Fatalf("page 61 count = %v, want 605", lastPage["count"])
	}

	beyond, err := FetchAllToPage(baseSQL, map[string]interface{}{"pageSize": 10, "pageNum": 62})
	if err != nil {
		t.Fatalf("FetchAllToPage(page 62) error: %v", err)
	}
	if rows := beyond["dataList"].([]map[string]interface{}); len(rows) != 0 {
		t.Fatalf("page 62 row count = %d, want 0", len(rows))
	}
	if beyond["count"] != int64(605) {
		t.Fatalf("page 62 count = %v, want 605", beyond["count"])
	}

	unpaged, err := FetchAllToPage(baseSQL, map[string]interface{}{})
	if err != nil {
		t.Fatalf("FetchAllToPage(unpaged) error: %v", err)
	}
	if rows := unpaged["dataList"].([]map[string]interface{}); len(rows) != defaultUnpagedLimit {
		t.Fatalf("unpaged row count = %d, want %d", len(rows), defaultUnpagedLimit)
	}
	if unpaged["count"] != int64(605) {
		t.Fatalf("unpaged count = %v, want 605", unpaged["count"])
	}
	if truncated, _ := unpaged["truncated"].(bool); !truncated {
		t.Fatalf("unpaged response should set truncated when rows exceed the limit")
	}
}

func TestInitDBSupportsHashAndSpecialCharactersInPath(t *testing.T) {
	for _, dirName := range []string{"backup#1", "dir?v=1", "pct%20dir"} {
		t.Run(dirName, func(t *testing.T) {
			root := t.TempDir()
			dataDir := filepath.Join(root, dirName)
			if err := os.MkdirAll(dataDir, 0755); err != nil {
				t.Fatalf("MkdirAll(%s) error: %v", dataDir, err)
			}
			dbPath := filepath.Join(dataDir, "openSync.db")
			resetGlobalDBForTest(t, &config.Config{
				DB:     config.DBConfig{DBName: dbPath},
				Server: config.ServerConfig{PasswdStr: "test-secret"},
			})

			InitSQL()
			if _, err := os.Stat(dbPath); err != nil {
				t.Fatalf("expected DB file at %q: %v", dbPath, err)
			}
			if dirName == "backup#1" {
				sibling := filepath.Join(root, "backup")
				if _, err := os.Stat(sibling); !os.IsNotExist(err) {
					t.Fatalf("unexpected truncated sibling DB %q created instead of %q", sibling, dbPath)
				}
			}
			var journalMode string
			if err := GetDB().QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil || journalMode != "wal" {
				t.Fatalf("PRAGMA journal_mode = %q, err = %v; want wal", journalMode, err)
			}
			var fk, autoVac int
			if err := GetDB().QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
				t.Fatalf("PRAGMA foreign_keys = %d, err = %v; want 1", fk, err)
			}
			if err := GetDB().QueryRow("PRAGMA auto_vacuum").Scan(&autoVac); err != nil || autoVac != 2 {
				t.Fatalf("PRAGMA auto_vacuum = %d, err = %v; want 2", autoVac, err)
			}
		})
	}
}
