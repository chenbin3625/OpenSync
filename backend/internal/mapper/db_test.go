package mapper

import (
	"path/filepath"
	"sync"
	"testing"

	"opensync/internal/config"
)

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
	once = sync.Once{}
	config.SetConfigForTest(&config.Config{
		DB: config.DBConfig{DBName: filepath.Join(t.TempDir(), "opensync.db")},
	})

	testDB := InitDB()
	if maxOpen := testDB.Stats().MaxOpenConnections; maxOpen <= 1 {
		t.Fatalf("MaxOpenConnections = %d, want more than one read-capable connection", maxOpen)
	}
}

func TestCloseDBClosesGlobalHandleAndAllowsReinit(t *testing.T) {
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
	once = sync.Once{}
	config.SetConfigForTest(&config.Config{
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
