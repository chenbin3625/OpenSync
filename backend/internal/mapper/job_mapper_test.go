package mapper

import (
	"database/sql"
	"math"
	"opensync/internal/msg"
	"reflect"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestDeleteJobRollsBackWhenChildDeleteFails(t *testing.T) {
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	defer testDB.Close()

	if _, err := testDB.Exec(`CREATE TABLE job(
		id integer primary key autoincrement,
		remark text
	)`); err != nil {
		t.Fatalf("create job: %v", err)
	}
	if _, err := testDB.Exec(`CREATE TABLE job_task(
		id integer primary key autoincrement,
		jobId integer
	)`); err != nil {
		t.Fatalf("create job_task: %v", err)
	}
	if _, err := testDB.Exec("INSERT INTO job(id, remark) VALUES (1, 'job')"); err != nil {
		t.Fatalf("insert job: %v", err)
	}
	if _, err := testDB.Exec("INSERT INTO job_task(id, jobId) VALUES (10, 1)"); err != nil {
		t.Fatalf("insert job_task: %v", err)
	}

	defer SetDBForTest(testDB)()

	if err := DeleteJob(1); err == nil {
		t.Fatalf("DeleteJob() returned nil error, want failure")
	}

	var jobCount int
	if err := testDB.QueryRow("SELECT COUNT(*) FROM job WHERE id=1").Scan(&jobCount); err != nil {
		t.Fatalf("count job: %v", err)
	}
	if jobCount != 1 {
		t.Fatalf("job count = %d, want rollback to keep 1", jobCount)
	}

	var taskCount int
	if err := testDB.QueryRow("SELECT COUNT(*) FROM job_task WHERE id=10").Scan(&taskCount); err != nil {
		t.Fatalf("count job_task: %v", err)
	}
	if taskCount != 1 {
		t.Fatalf("job_task count = %d, want rollback to keep 1", taskCount)
	}
}

func TestUpdateJobReturnsNotFoundWhenRowMissing(t *testing.T) {
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	defer testDB.Close()

	if _, err := testDB.Exec(`CREATE TABLE job(
		id integer primary key autoincrement,
		enable integer, remark text, srcPath text, dstPath text, alistId integer,
		useCacheT integer, scanIntervalT integer, useCacheS integer, scanIntervalS integer,
		method integer, interval integer, isCron integer,
		month text, day text, day_of_week text, hour text, minute text, second text,
		exclude text, minFileSize integer, maxFileSize integer
	)`); err != nil {
		t.Fatalf("create job: %v", err)
	}
	if _, err := testDB.Exec("INSERT INTO job(id, enable, remark) VALUES (1, 1, 'old')"); err != nil {
		t.Fatalf("insert job: %v", err)
	}

	defer SetDBForTest(testDB)()

	if err := UpdateJob(map[string]interface{}{"id": int64(1), "enable": 1, "remark": "new"}); err != nil {
		t.Fatalf("UpdateJob(existing) error: %v", err)
	}
	if err := UpdateJob(map[string]interface{}{"id": int64(999), "enable": 1, "remark": "missing"}); err == nil || err.Error() != msg.JobNotFound {
		t.Fatalf("UpdateJob(missing) error = %v, want %q", err, msg.JobNotFound)
	}
}

func TestGetJobTaskListAppliesHistoryFilters(t *testing.T) {
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

	rows := []struct {
		id         int
		jobID      int
		status     int
		runTime    int
		createTime int
	}{
		{101, 1, 2, 100, 90},
		{202, 1, 6, 200, 190},
		{302, 1, 2, 300, 290},
		{402, 2, 2, 300, 290},
	}
	for _, row := range rows {
		if _, err := testDB.Exec(
			"INSERT INTO job_task(id, jobId, status, runTime, createTime) VALUES (?, ?, ?, ?, ?)",
			row.id, row.jobID, row.status, row.runTime, row.createTime,
		); err != nil {
			t.Fatalf("insert job_task %d: %v", row.id, err)
		}
	}

	defer SetDBForTest(testDB)()

	result, err := GetJobTaskList(map[string]interface{}{
		"id":        int64(1),
		"status":    "2",
		"startTime": "150",
		"endTime":   "350",
		"keyword":   "02",
		"pageSize":  "10",
		"pageNum":   "1",
	})
	if err != nil {
		t.Fatalf("GetJobTaskList() error: %v", err)
	}

	dataList := result["dataList"].([]map[string]interface{})
	if len(dataList) != 1 {
		t.Fatalf("dataList len = %d, want 1: %#v", len(dataList), dataList)
	}
	if gotID := dataList[0]["id"]; gotID != int64(302) {
		t.Fatalf("matched task id = %v, want 302", gotID)
	}
	if gotCount := result["count"]; gotCount != int64(1) {
		t.Fatalf("count = %v, want 1", gotCount)
	}
}

func TestGetJobTaskListHandlesNilKeywordLargeTimestampsAndDeterministicTieBreak(t *testing.T) {
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

	// Millisecond timestamps exceeding 32-bit int max, with identical createTime to verify id DESC tie-breaker.
	const ts1 = int64(1700000000000)
	const ts2 = int64(1750000000000)
	for _, stmt := range []struct {
		id, runTime, createTime int64
	}{
		{10, ts1, ts1},
		{20, ts1, ts1},
		{30, ts1, ts1},
		{40, ts2, ts2},
	} {
		if _, err := testDB.Exec(
			"INSERT INTO job_task(id, jobId, status, runTime, createTime) VALUES (?, 1, 2, ?, ?)",
			stmt.id, stmt.runTime, stmt.createTime,
		); err != nil {
			t.Fatalf("insert job_task %d: %v", stmt.id, err)
		}
	}

	defer SetDBForTest(testDB)()

	result, err := GetJobTaskList(map[string]interface{}{
		"id":        int64(1),
		"keyword":   nil, // must not become "<nil>"
		"startTime": int64(1690000000000),
		"endTime":   int64(1710000000000),
		"pageSize":  10,
		"pageNum":   1,
	})
	if err != nil {
		t.Fatalf("GetJobTaskList() error: %v", err)
	}
	dataList := result["dataList"].([]map[string]interface{})
	if len(dataList) != 3 {
		t.Fatalf("dataList len = %d, want 3", len(dataList))
	}
	gotIDs := []int64{dataList[0]["id"].(int64), dataList[1]["id"].(int64), dataList[2]["id"].(int64)}
	wantIDs := []int64{30, 20, 10}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("tie-break task ids = %v, want %v", gotIDs, wantIDs)
	}
}

func TestParseStatusListValidatesAndDeduplicates(t *testing.T) {
	if got := parseStatusList(nil); got != nil {
		t.Fatalf("parseStatusList(nil) = %v, want nil", got)
	}
	if got := parseStatusList("2, abc, , 2, 7, 7, 3"); !reflect.DeepEqual(got, []int{2, 7, 3}) {
		t.Fatalf("parseStatusList(string) = %v, want [2 7 3]", got)
	}
	if got := parseStatusList([]interface{}{2, "bad", nil, 1.5, math.NaN(), math.Inf(1), 2, 3, float64(7)}); !reflect.DeepEqual(got, []int{2, 3, 7}) {
		t.Fatalf("parseStatusList([]interface{}) = %v, want [2 3 7]", got)
	}
	if got := parseStatusList([]int{2, 2, 3, 2}); !reflect.DeepEqual(got, []int{2, 3}) {
		t.Fatalf("parseStatusList([]int) = %v, want [2 3]", got)
	}
	if got := parseStatusList("not-a-number"); len(got) != 0 {
		t.Fatalf("parseStatusList(invalid) = %v, want empty", got)
	}
}

func TestGetJobTaskListFiltersByStatusSet(t *testing.T) {
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

	rows := []struct {
		id     int
		jobID  int
		status int
	}{
		{101, 1, 0},
		{102, 1, 1},
		{103, 1, 2},
		{104, 1, 7},
		{105, 1, 8},
		{201, 2, 2},
	}
	for _, row := range rows {
		if _, err := testDB.Exec(
			"INSERT INTO job_task(id, jobId, status, runTime, createTime) VALUES (?, ?, ?, ?, ?)",
			row.id, row.jobID, row.status, row.id, row.id,
		); err != nil {
			t.Fatalf("insert job_task %d: %v", row.id, err)
		}
	}

	defer SetDBForTest(testDB)()

	result, err := GetJobTaskList(map[string]interface{}{
		"id":       int64(1),
		"statusIn": []int{2, 3, 4, 5, 6, 7, 8},
		"pageSize": "10",
		"pageNum":  "1",
	})
	if err != nil {
		t.Fatalf("GetJobTaskList() error: %v", err)
	}

	dataList := result["dataList"].([]map[string]interface{})
	if gotCount := result["count"]; gotCount != int64(3) {
		t.Fatalf("count = %v, want 3", gotCount)
	}
	if len(dataList) != 3 {
		t.Fatalf("dataList len = %d, want 3: %#v", len(dataList), dataList)
	}
	gotIDs := []int64{dataList[0]["id"].(int64), dataList[1]["id"].(int64), dataList[2]["id"].(int64)}
	wantIDs := []int64{105, 104, 103}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("matched task ids = %v, want %v", gotIDs, wantIDs)
	}
}

func TestUpdateJobTaskStatusAndNumWritesStatusAndTaskNumTogether(t *testing.T) {
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	defer testDB.Close()

	if _, err := testDB.Exec(`CREATE TABLE job_task(
		id integer primary key autoincrement,
		status integer,
		errMsg text,
		taskNum text
	)`); err != nil {
		t.Fatalf("create job_task: %v", err)
	}
	if _, err := testDB.Exec("INSERT INTO job_task(id, status) VALUES (10, 1)"); err != nil {
		t.Fatalf("insert job_task: %v", err)
	}

	defer SetDBForTest(testDB)()

	errMsg := "copy failed"
	if err := UpdateJobTaskStatusAndNum(10, 3, &errMsg, `{"failNum":1}`); err != nil {
		t.Fatalf("UpdateJobTaskStatusAndNum() error: %v", err)
	}

	var status int
	var gotErrMsg string
	var taskNum string
	if err := testDB.QueryRow("SELECT status, errMsg, taskNum FROM job_task WHERE id=10").Scan(&status, &gotErrMsg, &taskNum); err != nil {
		t.Fatalf("read job_task: %v", err)
	}
	if status != 3 || gotErrMsg != errMsg || taskNum != `{"failNum":1}` {
		t.Fatalf("row = status %d errMsg %q taskNum %q, want 3/%q/{failNum}", status, gotErrMsg, taskNum, errMsg)
	}
}

func TestAddJobTaskItemManyPersistsProvidedCreateTime(t *testing.T) {
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	defer testDB.Close()

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
		createTime integer DEFAULT 1
	)`); err != nil {
		t.Fatalf("create job_task_item: %v", err)
	}

	defer SetDBForTest(testDB)()

	if err := AddJobTaskItemMany([]map[string]interface{}{
		{
			"taskId":      int64(10),
			"srcPath":     "/src/",
			"dstPath":     "/dst/",
			"isPath":      0,
			"fileName":    "movie.mkv",
			"fileSize":    int64(1024),
			"type":        0,
			"alistTaskId": "copy-1",
			"status":      2,
			"errMsg":      nil,
			"createTime":  int64(123456),
		},
	}); err != nil {
		t.Fatalf("AddJobTaskItemMany() error: %v", err)
	}

	var createTime int64
	if err := testDB.QueryRow("SELECT createTime FROM job_task_item WHERE taskId=10").Scan(&createTime); err != nil {
		t.Fatalf("read createTime: %v", err)
	}
	if createTime != 123456 {
		t.Fatalf("createTime = %d, want provided value 123456", createTime)
	}
}

func TestForEachJobTaskItemsByStatusesReadsBatchesInCreateOrder(t *testing.T) {
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	defer testDB.Close()

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
		t.Fatalf("create job_task_item: %v", err)
	}

	rows := []struct {
		id         int
		status     int
		createTime int
		fileName   string
	}{
		{1, 7, 20, "failed-b.txt"},
		{2, 2, 10, "success.txt"},
		{3, 7, 10, "failed-a.txt"},
		{4, 4, 30, "aborted.txt"},
		{5, 7, 30, "failed-c.txt"},
	}
	for _, row := range rows {
		if _, err := testDB.Exec(
			`INSERT INTO job_task_item(id, taskId, fileName, status, createTime)
			 VALUES (?, 10, ?, ?, ?)`,
			row.id, row.fileName, row.status, row.createTime,
		); err != nil {
			t.Fatalf("insert job_task_item %d: %v", row.id, err)
		}
	}

	defer SetDBForTest(testDB)()

	var got []string
	var batchSizes []int
	err = ForEachJobTaskItemsByStatuses(10, []int{7}, 2, func(items []map[string]interface{}) error {
		batchSizes = append(batchSizes, len(items))
		for _, item := range items {
			got = append(got, item["fileName"].(string))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ForEachJobTaskItemsByStatuses() error: %v", err)
	}

	want := []string{"failed-a.txt", "failed-b.txt", "failed-c.txt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("items = %v, want %v", got, want)
	}
	if !reflect.DeepEqual(batchSizes, []int{2, 1}) {
		t.Fatalf("batch sizes = %v, want [2 1]", batchSizes)
	}
}

func TestForEachJobTaskItemsByStatusesIncludesNullCreateTimeAndAvoidsTempSort(t *testing.T) {
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	defer testDB.Close()

	if _, err := testDB.Exec(`CREATE TABLE job_task(
		id integer primary key autoincrement,
		jobId integer, status integer, runTime integer, createTime integer
	)`); err != nil {
		t.Fatalf("create job_task: %v", err)
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
		t.Fatalf("create job_task_item: %v", err)
	}
	if err := ensureIndexes(testDB); err != nil {
		t.Fatalf("ensureIndexes() error: %v", err)
	}

	// Mix NULL createTime rows (spanning multiple batches) with non-NULL rows.
	for _, stmt := range []string{
		`INSERT INTO job_task_item(id, taskId, fileName, status, createTime) VALUES (1, 10, 'null-1', 7, NULL)`,
		`INSERT INTO job_task_item(id, taskId, fileName, status, createTime) VALUES (2, 10, 'null-2', 7, NULL)`,
		`INSERT INTO job_task_item(id, taskId, fileName, status, createTime) VALUES (3, 10, 'null-3', 7, NULL)`,
		`INSERT INTO job_task_item(id, taskId, fileName, status, createTime) VALUES (4, 10, 'ts-20', 7, 20)`,
		`INSERT INTO job_task_item(id, taskId, fileName, status, createTime) VALUES (5, 10, 'ts-10', 7, 10)`,
		`INSERT INTO job_task_item(id, taskId, fileName, status, createTime) VALUES (6, 10, 'ts-10-b', 7, 10)`,
	} {
		if _, err := testDB.Exec(stmt); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	defer SetDBForTest(testDB)()

	var got []string
	err = ForEachJobTaskItemsByStatuses(10, []int{7, 7}, 2, func(items []map[string]interface{}) error {
		for _, item := range items {
			got = append(got, item["fileName"].(string))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ForEachJobTaskItemsByStatuses() error: %v", err)
	}
	want := []string{"null-1", "null-2", "null-3", "ts-10", "ts-10-b", "ts-20"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("items = %v, want %v", got, want)
	}

	// Verify EXPLAIN QUERY PLAN does not use a temp B-tree sort for the index-ordered query.
	for _, q := range []string{
		`EXPLAIN QUERY PLAN SELECT id FROM job_task_item WHERE taskId=10 AND +status IN (7) ORDER BY createTime ASC, id ASC LIMIT 2`,
		`EXPLAIN QUERY PLAN SELECT id FROM job_task_item WHERE taskId=10 AND +status IN (7) AND (createTime, id) > (10, 5) ORDER BY createTime ASC, id ASC LIMIT 2`,
	} {
		rows, err := testDB.Query(q)
		if err != nil {
			t.Fatalf("EXPLAIN QUERY PLAN error: %v", err)
		}
		for rows.Next() {
			var id, parent, notused int
			var detail string
			if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
				t.Fatalf("scan plan: %v", err)
			}
			if strings.Contains(strings.ToUpper(detail), "TEMP B-TREE") {
				t.Fatalf("unexpected temp B-tree sort in plan %q for %s", detail, q)
			}
		}
		_ = rows.Close()
	}
}

func TestGetJobTaskCountsByTaskIDsAggregatesManyTasks(t *testing.T) {
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	defer testDB.Close()

	if _, err := testDB.Exec(`CREATE TABLE job_task_item(
		id integer primary key autoincrement,
		taskId integer,
		fileSize integer,
		type integer,
		status integer
	)`); err != nil {
		t.Fatalf("create job_task_item: %v", err)
	}

	rows := []struct {
		taskID   int64
		status   int
		itemType int
		fileSize interface{}
	}{
		{10, 0, 0, int64(10)},
		{10, 1, 0, int64(20)},
		{10, 2, 0, int64(30)},
		{10, 2, 1, int64(40)},
		{10, 7, 0, nil},
		{10, 8, 0, int64(50)},
		{20, 2, 0, int64(70)},
		{20, 7, 0, int64(80)},
	}
	for _, row := range rows {
		if _, err := testDB.Exec(
			"INSERT INTO job_task_item(taskId, status, type, fileSize) VALUES (?, ?, ?, ?)",
			row.taskID, row.status, row.itemType, row.fileSize,
		); err != nil {
			t.Fatalf("insert job_task_item: %v", err)
		}
	}

	defer SetDBForTest(testDB)()

	counts := GetJobTaskCountsByTaskIDs([]int64{10, 20, 30, 10})
	task10 := counts[10]
	if task10["allNum"] != int64(6) || task10["waitNum"] != int64(1) || task10["runningNum"] != int64(1) {
		t.Fatalf("task 10 base counts = %#v, want all/wait/running 6/1/1", task10)
	}
	if task10["successNum"] != int64(2) || task10["failNum"] != int64(1) || task10["otherNum"] != int64(1) {
		t.Fatalf("task 10 status counts = %#v, want success/fail/other 2/1/1", task10)
	}
	if task10["sumSize"] != int64(30) {
		t.Fatalf("task 10 sumSize = %v, want 30", task10["sumSize"])
	}

	task20 := counts[20]
	if task20["allNum"] != int64(2) || task20["successNum"] != int64(1) || task20["failNum"] != int64(1) {
		t.Fatalf("task 20 counts = %#v, want all/success/fail 2/1/1", task20)
	}
	if task20["sumSize"] != int64(70) {
		t.Fatalf("task 20 sumSize = %v, want 70", task20["sumSize"])
	}

	task30 := counts[30]
	if task30["allNum"] != int64(0) || task30["sumSize"] != int64(0) {
		t.Fatalf("task 30 counts = %#v, want zeros", task30)
	}
}

func TestGetJobTaskCountsByTaskIDsWithErrorChunksLargeSlices(t *testing.T) {
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	defer testDB.Close()

	if _, err := testDB.Exec(`CREATE TABLE job_task_item(
		id integer primary key autoincrement,
		taskId integer,
		fileSize integer,
		type integer,
		status integer
	)`); err != nil {
		t.Fatalf("create job_task_item: %v", err)
	}

	tx, err := testDB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	const totalTasks = 1205
	ids := make([]int64, totalTasks)
	for i := 1; i <= totalTasks; i++ {
		ids[i-1] = int64(i)
		if i%2 == 0 {
			if _, err := tx.Exec("INSERT INTO job_task_item(taskId, status, type, fileSize) VALUES (?, 2, 0, ?)", i, i*10); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	defer SetDBForTest(testDB)()

	counts, err := GetJobTaskCountsByTaskIDsWithError(ids)
	if err != nil {
		t.Fatalf("GetJobTaskCountsByTaskIDsWithError(%d ids) error: %v", totalTasks, err)
	}
	if len(counts) != totalTasks {
		t.Fatalf("counts len = %d, want %d", len(counts), totalTasks)
	}
	if counts[1]["allNum"] != int64(0) {
		t.Fatalf("task 1 allNum = %v, want 0", counts[1]["allNum"])
	}
	if counts[1204]["allNum"] != int64(1) || counts[1204]["sumSize"] != int64(12040) {
		t.Fatalf("task 1204 counts = %#v, want allNum=1 sumSize=12040", counts[1204])
	}
}
