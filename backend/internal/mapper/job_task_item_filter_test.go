package mapper

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func setupTaskItemFilterDB(t testing.TB) *sql.DB {
	t.Helper()

	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}

	if _, err := testDB.Exec(`CREATE TABLE job_task_item(
		id integer primary key autoincrement,
		taskId integer,
		srcPath text,
		dstPath text,
		isPath integer,
		fileName text,
		fileSize integer,
		type integer,
		alistTaskId text,
		status integer,
		progress real,
		errMsg text,
		createTime integer
	)`); err != nil {
		testDB.Close()
		t.Fatalf("create job_task_item: %v", err)
	}

	rows := []struct {
		id       int
		srcPath  string
		dstPath  string
		isPath   int
		fileName string
		alistID  string
		typ      int
		status   int
		errMsg   *string
	}{
		{1, "/src/photos", "/backup/photos", 1, "photos", "", 0, 2, nil},
		{2, "/src/photos/cat.jpg", "/backup/photos/cat.jpg", 0, "cat.jpg", "copy_task_002", 0, 7, strPtr("copy failed")},
		{3, "", "/backup/old/movie.mkv", 0, "movie.mkv", "delete_task_003", 1, 2, nil},
		{4, "/src/docs/report.pdf", "/backup/docs/report.pdf", 0, "report.pdf", "move_task_004", 2, 1, nil},
	}

	for _, row := range rows {
		if _, err := testDB.Exec(
			`INSERT INTO job_task_item(id, taskId, srcPath, dstPath, isPath, fileName, alistTaskId, type, status, errMsg, createTime)
			 VALUES (?, 10, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			row.id, row.srcPath, row.dstPath, row.isPath, row.fileName, row.alistID, row.typ, row.status, row.errMsg, row.id,
		); err != nil {
			testDB.Close()
			t.Fatalf("insert job_task_item %d: %v", row.id, err)
		}
	}

	restore := SetDBForTest(testDB)
	t.Cleanup(func() {
		restore()
		testDB.Close()
	})

	return testDB
}

func strPtr(s string) *string {
	return &s
}

func TestGetJobTaskItemListFiltersByKeywordAcrossNamePathAndError(t *testing.T) {
	setupTaskItemFilterDB(t)

	result, err := GetJobTaskItemList(map[string]interface{}{
		"taskId":  10,
		"keyword": "failed",
	})
	if err != nil {
		t.Fatalf("GetJobTaskItemList() error: %v", err)
	}

	items := result["dataList"].([]map[string]interface{})
	if len(items) != 1 {
		t.Fatalf("item count = %d, want 1: %#v", len(items), items)
	}
	if items[0]["fileName"] != "cat.jpg" {
		t.Fatalf("fileName = %v, want cat.jpg", items[0]["fileName"])
	}
}

func TestGetJobTaskItemListFiltersByKeywordAcrossAlistTaskID(t *testing.T) {
	setupTaskItemFilterDB(t)

	result, err := GetJobTaskItemList(map[string]interface{}{
		"taskId":  10,
		"keyword": "delete_task",
	})
	if err != nil {
		t.Fatalf("GetJobTaskItemList() error: %v", err)
	}

	items := result["dataList"].([]map[string]interface{})
	if len(items) != 1 {
		t.Fatalf("item count = %d, want 1: %#v", len(items), items)
	}
	if items[0]["fileName"] != "movie.mkv" {
		t.Fatalf("fileName = %v, want movie.mkv", items[0]["fileName"])
	}
}

func TestGetJobTaskItemListUsesFTSWhenAvailable(t *testing.T) {
	testDB := setupTaskItemFilterDB(t)
	if err := installJobTaskItemFTS(testDB, true); err != nil {
		t.Fatalf("installJobTaskItemFTS() error: %v", err)
	}

	// Verify 5-column FTS5 filter avoids OR LIKE full-scan fallback.
	filterSQL, _, pureFTS := taskItemKeywordFilter(context.Background(), "lete_ta")
	if !pureFTS || strings.Contains(filterSQL, " OR ") {
		t.Fatalf("taskItemKeywordFilter() = (%q, pureFTS=%v), want pure FTS5 MATCH without OR fallback", filterSQL, pureFTS)
	}

	result, err := GetJobTaskItemList(map[string]interface{}{
		"taskId":  10,
		"keyword": "lete_ta",
	})
	if err != nil {
		t.Fatalf("GetJobTaskItemList() error: %v", err)
	}

	items := result["dataList"].([]map[string]interface{})
	if len(items) != 1 {
		t.Fatalf("item count = %d, want 1: %#v", len(items), items)
	}
	if items[0]["fileName"] != "movie.mkv" {
		t.Fatalf("fileName = %v, want movie.mkv", items[0]["fileName"])
	}
}

func TestGetJobTaskItemListFTS5SanitizesNULControlCharsAndNilKeyword(t *testing.T) {
	testDB := setupTaskItemFilterDB(t)
	if err := installJobTaskItemFTS(testDB, true); err != nil {
		t.Fatalf("installJobTaskItemFTS() error: %v", err)
	}

	if got := fts5Phrase("foo\x00\x01\x1f\x7fbar\"baz"); got != `"foobar""baz"` {
		t.Fatalf("fts5Phrase() = %q, want %q", got, `"foobar""baz"`)
	}

	for _, kw := range []interface{}{
		nil,
		"\x00\x00\x00",
		"cat\x00.jpg",
		"cat\x01\x02\x03",
		`"""`,
		"OR AND NOT NEAR",
	} {
		result, err := GetJobTaskItemList(map[string]interface{}{
			"taskId":  10,
			"keyword": kw,
		})
		if err != nil {
			t.Fatalf("GetJobTaskItemList(keyword=%#v) error: %v", kw, err)
		}
		if kw == nil {
			// nil keyword must not turn into "<nil>"; all 4 rows should be returned.
			if got := result["count"]; got != int64(4) {
				t.Fatalf("GetJobTaskItemList(keyword=nil) count = %v, want 4", got)
			}
		}
	}
}

func TestFTSAvailabilityResetsWhenFTSInstalledOnSameDB(t *testing.T) {
	testDB := setupTaskItemFilterDB(t)

	// Query before FTS exists (caches ok=false).
	if taskItemFTSAvailable(context.Background()) {
		t.Fatalf("expected taskItemFTSAvailable = false before install")
	}

	// Installing FTS must invalidate the stale negative cache entry.
	if err := installJobTaskItemFTS(testDB, true); err != nil {
		t.Fatalf("installJobTaskItemFTS() error: %v", err)
	}
	ok, coversAll := taskItemFTSCapabilities(context.Background())
	if !ok || !coversAll {
		t.Fatalf("taskItemFTSCapabilities() = (%v, %v), want (true, true)", ok, coversAll)
	}
}

func TestGetJobTaskItemListFiltersByDirectoryAndErrorPresence(t *testing.T) {
	setupTaskItemFilterDB(t)

	result, err := GetJobTaskItemList(map[string]interface{}{
		"taskId":   10,
		"isPath":   0,
		"hasError": 1,
	})
	if err != nil {
		t.Fatalf("GetJobTaskItemList() error: %v", err)
	}

	items := result["dataList"].([]map[string]interface{})
	if len(items) != 1 {
		t.Fatalf("item count = %d, want 1: %#v", len(items), items)
	}
	if items[0]["id"] != int64(2) {
		t.Fatalf("id = %v, want 2", items[0]["id"])
	}
}

func TestGetJobTaskItemListContextCancelsWhileWaitingForConnection(t *testing.T) {
	testDB := setupTaskItemFilterDB(t)
	testDB.SetMaxOpenConns(1)
	conn, err := testDB.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = GetJobTaskItemListContext(ctx, map[string]interface{}{
		"taskId": 10, "keyword": "report", "pageNum": 1, "pageSize": 10,
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("GetJobTaskItemListContext() error = %v, want context deadline exceeded", err)
	}
}

func BenchmarkGetJobTaskItemList(b *testing.B) {
	testDB := setupTaskItemFilterDB(b)
	if _, err := testDB.Exec(`CREATE INDEX idx_job_task_item_task_time ON job_task_item(taskId, createTime DESC, id DESC)`); err != nil {
		b.Fatal(err)
	}
	if _, err := testDB.Exec(`WITH RECURSIVE generated(id) AS (
		VALUES (1) UNION ALL SELECT id + 1 FROM generated WHERE id < 50000
	)
	INSERT INTO job_task_item(taskId, fileName, status, errMsg, createTime)
	SELECT 42, 'file-' || id, 2, CASE WHEN id % 12000 = 0 THEN 'copy stalled' END, id FROM generated`); err != nil {
		b.Fatal(err)
	}
	if err := installJobTaskItemFTS(testDB, true); err != nil {
		b.Fatal(err)
	}
	for _, sample := range []struct {
		name, keyword string
		count         int64
	}{
		{name: "selective_keyword", keyword: "stalled", count: 4},
		{name: "full_page", count: 50000},
	} {
		b.Run(sample.name, func(b *testing.B) {
			params := map[string]interface{}{"taskId": 42, "pageNum": 1, "pageSize": 10, "keyword": sample.keyword}
			for b.Loop() {
				result, err := GetJobTaskItemList(params)
				if err != nil || result["count"] != sample.count {
					b.Fatalf("GetJobTaskItemList() = %v, %v; want count %d", result, err, sample.count)
				}
			}
		})
	}
}
