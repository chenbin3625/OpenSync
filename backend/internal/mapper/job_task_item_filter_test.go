package mapper

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func setupTaskItemFilterDB(t *testing.T) *sql.DB {
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

	oldDB := db
	db = testDB
	t.Cleanup(func() {
		db = oldDB
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
