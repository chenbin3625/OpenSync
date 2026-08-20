package mapper

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"opensync/internal/config"
	"opensync/pkg/util"
)

func resetGlobalDBForTest(t *testing.T, cfg *config.Config) {
	t.Helper()
	oldDB := db
	oldOnce := once
	oldConfig := config.GetConfig()
	t.Cleanup(func() {
		if db != nil && db != oldDB {
			_ = db.Close()
		}
		db = oldDB
		once = oldOnce
		config.SetConfigForTest(oldConfig)
	})

	db = nil
	once = &sync.Once{}
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

func TestPageOffsetRejectsIntegerOverflow(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	if _, err := pageOffset(maxPageSize, maxInt); err == nil {
		t.Fatal("pageOffset() error = nil, want overflow rejection")
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

func TestInitDBCreatesDatabaseFileWithOwnerOnlyPermissions(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "opensync.db")
	resetGlobalDBForTest(t, &config.Config{
		DB: config.DBConfig{DBName: dbPath},
	})

	if InitDB() == nil {
		t.Fatalf("InitDB() returned nil")
	}
	info, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("Stat(db) error: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("db permissions = %v, want 0600", got)
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

func TestCountQueryFromSelectUsesCoveringCount(t *testing.T) {
	got := countQueryFromSelect("SELECT * FROM job_task WHERE jobId=? ORDER BY createTime DESC")
	want := "SELECT COUNT(*) FROM job_task WHERE jobId=?"
	if got != want {
		t.Fatalf("countQueryFromSelect() = %q, want %q", got, want)
	}

	got = countQueryFromSelect("SELECT a, b FROM t GROUP BY a")
	if !strings.HasPrefix(got, "SELECT COUNT(*) FROM (") {
		t.Fatalf("GROUP BY should keep a subquery, got %q", got)
	}
}

func TestWithPageTotalRewritesSelect(t *testing.T) {
	got := withPageTotal("SELECT * FROM job ORDER BY createTime DESC")
	want := "SELECT COUNT(*) OVER() AS " + pageTotalColumn + ", * FROM job ORDER BY createTime DESC"
	if got != want {
		t.Fatalf("withPageTotal() = %q, want %q", got, want)
	}

	query, args, useWindow := pageQuery("SELECT a FROM t UNION SELECT b FROM u", 10, 20, true, []interface{}{"x"})
	if useWindow {
		t.Fatal("UNION queries must not use COUNT(*) OVER()")
	}
	if !strings.HasSuffix(query, " LIMIT ? OFFSET ?") {
		t.Fatalf("UNION page query = %q, want bound LIMIT/OFFSET", query)
	}
	if len(args) != 3 || args[1] != 10 || args[2] != int64(20) {
		t.Fatalf("UNION page args = %#v, want [x 10 20]", args)
	}
}

func TestFetchAllToPageUsesWindowCountInOneQuery(t *testing.T) {
	resetGlobalDBForTest(t, &config.Config{
		DB: config.DBConfig{DBName: filepath.Join(t.TempDir(), "opensync.db")},
	})
	if InitDB() == nil {
		t.Fatal("InitDB() returned nil")
	}
	if _, err := GetDB().Exec(`CREATE TABLE page_probe (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	for i := 1; i <= 5; i++ {
		if _, err := GetDB().Exec(`INSERT INTO page_probe(name) VALUES (?)`, "r"+string(rune('0'+i))); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	page, err := FetchAllToPage("SELECT id, name FROM page_probe ORDER BY id", map[string]interface{}{
		"pageSize": 2,
		"pageNum":  2,
	})
	if err != nil {
		t.Fatalf("FetchAllToPage() error: %v", err)
	}
	if util.ToInt64(page["count"]) != 5 {
		t.Fatalf("count = %v, want 5 from COUNT(*) OVER()", page["count"])
	}
	rows, _ := page["dataList"].([]map[string]interface{})
	if len(rows) != 2 {
		t.Fatalf("len(dataList) = %d, want 2", len(rows))
	}
	if util.ToInt64(rows[0]["id"]) != 3 {
		t.Fatalf("first id = %v, want 3", rows[0]["id"])
	}
	for _, row := range rows {
		if _, exists := row[pageTotalColumn]; exists {
			t.Fatalf("page total column leaked into API row: %#v", row)
		}
	}

	empty, err := FetchAllToPage("SELECT id, name FROM page_probe ORDER BY id", map[string]interface{}{
		"pageSize": 2,
		"pageNum":  10,
	})
	if err != nil {
		t.Fatalf("empty page FetchAllToPage() error: %v", err)
	}
	if util.ToInt64(empty["count"]) != 5 {
		t.Fatalf("empty page count = %v, want 5 from COUNT fallback", empty["count"])
	}
}

func TestSqliteDSNIncludesSynchronousPragma(t *testing.T) {
	tests := []struct {
		mode string
		want string // "" means the pragma is omitted entirely
	}{
		{"normal", "synchronous(normal)"},
		{"full", "synchronous(full)"},
		{"off", "synchronous(off)"},
		{"", ""},
		{"BOGUS", ""},
		{"Normal", "synchronous(normal)"},
	}
	for _, tt := range tests {
		dsn := sqliteDSN(config.DBConfig{DBName: "/tmp/x.db", SqliteSync: tt.mode})
		decoded, err := url.QueryUnescape(dsn)
		if err != nil {
			t.Fatalf("QueryUnescape(%q) error: %v", dsn, err)
		}
		if tt.want == "" && strings.Contains(decoded, "synchronous(") {
			t.Errorf("sqliteDSN(mode=%q) contains synchronous pragma, want none", tt.mode)
		}
		if tt.want != "" && !strings.Contains(decoded, tt.want) {
			t.Errorf("sqliteDSN(mode=%q) = %q, want to contain %q", tt.mode, decoded, tt.want)
		}
	}
}

func TestInitDBAppliesConfiguredSynchronousMode(t *testing.T) {
	resetGlobalDBForTest(t, &config.Config{
		DB: config.DBConfig{DBName: filepath.Join(t.TempDir(), "opensync.db"), SqliteSync: "normal"},
	})
	testDB := InitDB()
	if testDB == nil {
		t.Fatalf("InitDB() returned nil")
	}
	var mode string
	if err := testDB.QueryRow("PRAGMA synchronous").Scan(&mode); err != nil {
		t.Fatalf("read PRAGMA synchronous: %v", err)
	}
	// modernc/sqlite reports numeric synchronous levels; "normal" = 1.
	if mode != "1" && mode != "normal" {
		t.Fatalf("PRAGMA synchronous = %q, want normal (1)", mode)
	}
}
