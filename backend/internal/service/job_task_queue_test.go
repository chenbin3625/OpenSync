package service

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"opensync/internal/config"
	"opensync/internal/mapper"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestCopyQueueKeepsFIFOAndCompacts(t *testing.T) {
	queue := newCopyQueue()

	for i := 0; i < 128; i++ {
		queue.push(&CopyItem{FileName: string(rune('a' + i%26))})
	}

	for i := 0; i < 96; i++ {
		item, ok := queue.pop()
		if !ok {
			t.Fatalf("pop(%d) returned empty queue", i)
		}
		if item.FileName != string(rune('a'+i%26)) {
			t.Fatalf("pop(%d) = %q, want %q", i, item.FileName, string(rune('a'+i%26)))
		}
	}

	if queue.len() != 32 {
		t.Fatalf("queue.len() = %d, want 32", queue.len())
	}
	if len(queue.items) > 64 {
		t.Fatalf("queue retained %d backing items after many pops, want compacted queue", len(queue.items))
	}

	for i := 128; i < 160; i++ {
		queue.push(&CopyItem{FileName: string(rune('a' + i%26))})
	}

	for i := 96; i < 160; i++ {
		item, ok := queue.pop()
		if !ok {
			t.Fatalf("pop(%d) returned empty queue", i)
		}
		if item.FileName != string(rune('a'+i%26)) {
			t.Fatalf("pop(%d) = %q, want %q", i, item.FileName, string(rune('a'+i%26)))
		}
	}
	if _, ok := queue.pop(); ok {
		t.Fatalf("pop() on empty queue returned an item")
	}
}

func TestCopyQueueRejectsPushWhenCapacityIsFull(t *testing.T) {
	queue := newCopyQueueWithCapacity(1)

	if ok := queue.push(&CopyItem{FileName: "one.txt"}); !ok {
		t.Fatalf("first push returned false, want true")
	}
	if ok := queue.push(&CopyItem{FileName: "two.txt"}); ok {
		t.Fatalf("second push returned true for full bounded queue, want false")
	}

	item, ok := queue.pop()
	if !ok {
		t.Fatalf("pop() returned empty queue")
	}
	if item.FileName != "one.txt" {
		t.Fatalf("pop() = %q, want one.txt", item.FileName)
	}
	if ok := queue.push(&CopyItem{FileName: "two.txt"}); !ok {
		t.Fatalf("push after pop returned false, want true")
	}
}

func TestCopyItemUsesRuntimeAndClientInterfaces(t *testing.T) {
	var _ copyItemRuntime = (*JobTask)(nil)
	var _ copyItemClient = (*AlistClient)(nil)

	jt := &JobTask{
		TaskID:      42,
		Job:         map[string]interface{}{},
		AlistClient: &AlistClient{},
	}
	jt.initRuntime()

	item := newCopyItem(jt, jt.AlistClient, "/src/", "/dst/", "file.txt", int64(10), taskItemTypeMove)

	if item.runtime != jt {
		t.Fatalf("item runtime = %#v, want JobTask runtime", item.runtime)
	}
	if item.client != jt.AlistClient {
		t.Fatalf("item client = %#v, want JobTask AlistClient", item.client)
	}
	if item.Status != taskStatusWaiting || item.Progress != 0 {
		t.Fatalf("new item status/progress = %d/%f, want 0/0", item.Status, item.Progress)
	}
}

func TestRuntimeTaskLimitsUseConfiguredValues(t *testing.T) {
	limits := taskRuntimeLimitsFromServer(config.ServerConfig{
		Timeout:               72,
		CopyConcurrency:       7,
		ScanConcurrency:       20,
		RealtimeFinishedItems: 120,
		MaxRetries:            4,
	})

	if limits.CopyConcurrency != 7 {
		t.Fatalf("CopyConcurrency = %d, want 7", limits.CopyConcurrency)
	}
	if limits.ScanConcurrency != 20 {
		t.Fatalf("ScanConcurrency = %d, want 20", limits.ScanConcurrency)
	}
	if limits.RealtimeFinishedItems != 120 {
		t.Fatalf("RealtimeFinishedItems = %d, want 120", limits.RealtimeFinishedItems)
	}
	if limits.MaxRetries != 4 {
		t.Fatalf("MaxRetries = %d, want 4", limits.MaxRetries)
	}
}

func TestRuntimeTaskLimitsClampInvalidConfiguredValues(t *testing.T) {
	limits := taskRuntimeLimitsFromServer(config.ServerConfig{
		CopyConcurrency:       0,
		ScanConcurrency:       99,
		RealtimeFinishedItems: 0,
		MaxRetries:            99,
	})

	if limits.CopyConcurrency != 5 {
		t.Fatalf("CopyConcurrency = %d, want default 5", limits.CopyConcurrency)
	}
	if limits.ScanConcurrency != 20 {
		t.Fatalf("ScanConcurrency = %d, want max 20", limits.ScanConcurrency)
	}
	if limits.RealtimeFinishedItems != 1000 {
		t.Fatalf("RealtimeFinishedItems = %d, want default 1000", limits.RealtimeFinishedItems)
	}
	if limits.MaxRetries != maxCopyRetries {
		t.Fatalf("MaxRetries = %d, want max %d", limits.MaxRetries, maxCopyRetries)
	}
}

func TestCopyItemRetriesFailedCopyBeforeSuccess(t *testing.T) {
	oldConfig := config.GetConfig()
	oldDelay := copyRetryDelay
	defer func() {
		config.SetConfigForTest(oldConfig)
		copyRetryDelay = oldDelay
	}()

	copyRetryDelay = func(int) time.Duration { return 0 }
	config.SetConfigForTest(&config.Config{
		Server: config.ServerConfig{
			Timeout:               0,
			CopyConcurrency:       1,
			ScanConcurrency:       1,
			RealtimeFinishedItems: 100,
			MaxRetries:            2,
		},
	})

	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/fs/copy" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if attempts.Add(1) <= 2 {
			_, _ = w.Write([]byte(`{"code":500,"message":"boom","data":{}}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"tasks":[]}}`))
	}))
	defer server.Close()

	jt := &JobTask{
		TaskID:  42,
		Job:     map[string]interface{}{},
		Finish:  make([]JobTaskItem, 0),
		Waiting: newCopyQueue(),
		AlistClient: &AlistClient{
			URL:    server.URL,
			client: server.Client(),
		},
	}
	jt.initRuntime()

	item := newCopyItem(jt, jt.AlistClient, "/src", "/dst", "file.txt", int64(11), taskItemTypeCopy)
	item.CreateTime = 100

	item.DoIt()

	if got := attempts.Load(); got != 3 {
		t.Fatalf("copy attempts = %d, want 3", got)
	}
	if status := item.status(); status != taskStatusSuccess {
		t.Fatalf("item status = %d, want success status 2", status)
	}
	if item.ErrMsg != nil {
		t.Fatalf("item ErrMsg = %q, want nil after successful retry", *item.ErrMsg)
	}

	jt.FinishMu.Lock()
	defer jt.FinishMu.Unlock()
	if len(jt.Finish) != 1 {
		t.Fatalf("finish len = %d, want 1", len(jt.Finish))
	}
	if jt.Finish[0].Status != taskStatusSuccess {
		t.Fatalf("finish status = %v, want 2", jt.Finish[0].Status)
	}
	if jt.Finish[0].ErrMsg != nil {
		t.Fatalf("finish errMsg = %q, want nil", *jt.Finish[0].ErrMsg)
	}
}

func TestJobTaskItemMapPreservesCopyAndDeletePayloadContract(t *testing.T) {
	copyErr := "copy failed"
	copyItem := JobTaskItem{
		TaskID:      42,
		SrcPath:     "/src/",
		DstPath:     "/dst/",
		IsPath:      taskItemFile,
		FileName:    "file.txt",
		FileSize:    int64(1024),
		Type:        taskItemTypeMove,
		AlistTaskID: "copy-1",
		Status:      taskStatusFailed,
		ErrMsg:      &copyErr,
		CreateTime:  100,
	}

	copyMap := copyItem.ToMap()

	if copyMap["taskId"] != int64(42) || copyMap["srcPath"] != "/src/" || copyMap["dstPath"] != "/dst/" {
		t.Fatalf("copy map paths = %#v", copyMap)
	}
	if copyMap["isPath"] != taskItemFile.Int() || copyMap["fileName"] != "file.txt" || copyMap["fileSize"] != int64(1024) {
		t.Fatalf("copy map file fields = %#v", copyMap)
	}
	if copyMap["type"] != taskItemTypeMove.Int() || copyMap["alistTaskId"] != "copy-1" {
		t.Fatalf("copy map type/task id fields = %#v", copyMap)
	}
	if copyMap["status"] != taskStatusFailed.Int() || copyMap["errMsg"] != &copyErr || copyMap["createTime"] != int64(100) {
		t.Fatalf("copy map status fields = %#v", copyMap)
	}

	deleteItem := NewDeleteJobTaskItem(42, "/dst/", "old.txt", int64(2048), taskStatusSuccess, nil, taskItemFile, 101)
	deleteMap := deleteItem.ToMap()

	if deleteMap["taskId"] != int64(42) || deleteMap["srcPath"] != nil || deleteMap["dstPath"] != "/dst/" {
		t.Fatalf("delete map paths = %#v", deleteMap)
	}
	if deleteMap["type"] != taskItemTypeDelete.Int() || deleteMap["alistTaskId"] != nil {
		t.Fatalf("delete map type/task id fields = %#v", deleteMap)
	}
	if deleteMap["status"] != taskStatusSuccess.Int() || deleteMap["createTime"] != int64(101) {
		t.Fatalf("delete map status fields = %#v", deleteMap)
	}
}

func TestCopyItemMapUsesTaskItemContractAndKeepsRealtimeProgress(t *testing.T) {
	errMsg := "copy failed"
	item := &CopyItem{
		SrcPath:     "/src/",
		DstPath:     "/dst/",
		FileName:    "file.txt",
		FileSize:    int64(1024),
		CopyType:    taskItemTypeCopy,
		AlistTaskID: "copy-1",
		Status:      taskStatusRunning,
		Progress:    35,
		ErrMsg:      &errMsg,
		CreateTime:  100,
	}

	itemMap := item.ToMap(42)

	if itemMap["taskId"] != int64(42) || itemMap["srcPath"] != "/src/" || itemMap["dstPath"] != "/dst/" {
		t.Fatalf("copy item map paths = %#v", itemMap)
	}
	if itemMap["isPath"] != taskItemFile.Int() || itemMap["fileName"] != "file.txt" || itemMap["fileSize"] != int64(1024) {
		t.Fatalf("copy item map file fields = %#v", itemMap)
	}
	if itemMap["type"] != taskItemTypeCopy.Int() || itemMap["alistTaskId"] != "copy-1" {
		t.Fatalf("copy item map type/task id fields = %#v", itemMap)
	}
	if itemMap["status"] != taskStatusRunning.Int() || itemMap["errMsg"] != &errMsg || itemMap["createTime"] != int64(100) {
		t.Fatalf("copy item map status fields = %#v", itemMap)
	}
	if itemMap["progress"] != float64(35) {
		t.Fatalf("copy item map progress = %#v, want 35", itemMap["progress"])
	}
}

func TestWaitForBreakReturnsWhenBreakRequested(t *testing.T) {
	jt := &JobTask{}
	jt.initRuntime()

	go func() {
		time.Sleep(20 * time.Millisecond)
		jt.requestBreak()
	}()

	start := time.Now()
	if completed := jt.waitForBreak(5 * time.Second); completed {
		t.Fatalf("waitForBreak returned completed=true after break")
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("waitForBreak took %s after break, want under 500ms", elapsed)
	}
}

func TestMarkWaitingAsAbortedMovesQueuedItemsToFinish(t *testing.T) {
	jt := &JobTask{
		TaskID:  42,
		Finish:  make([]JobTaskItem, 0),
		Waiting: newCopyQueue(),
	}
	jt.initRuntime()

	jt.Waiting.push(&CopyItem{
		SrcPath:    "/src/",
		DstPath:    "/dst/",
		FileName:   "one.txt",
		FileSize:   int64(10),
		CopyType:   0,
		Status:     0,
		CreateTime: 100,
	})
	jt.Waiting.push(&CopyItem{
		SrcPath:    "/src/",
		DstPath:    "/dst/",
		FileName:   "two.txt",
		FileSize:   int64(20),
		CopyType:   2,
		Status:     0,
		CreateTime: 101,
	})

	jt.markWaitingAsAborted()

	if jt.Waiting.len() != 0 {
		t.Fatalf("waiting queue len = %d, want 0", jt.Waiting.len())
	}
	if len(jt.Finish) != 2 {
		t.Fatalf("finish len = %d, want 2", len(jt.Finish))
	}
	for i, item := range jt.Finish {
		if item.Status != 4 {
			t.Fatalf("finish[%d].status = %v, want 4", i, item.Status)
		}
		if item.TaskID != int64(42) {
			t.Fatalf("finish[%d].taskId = %v, want 42", i, item.TaskID)
		}
	}
}

func TestCopyHookBuffersOldFinishedItemsBeforePersistThreshold(t *testing.T) {
	oldPersist := persistJobTaskItems
	defer func() {
		persistJobTaskItems = oldPersist
	}()

	var persisted []map[string]interface{}
	var calls int
	var batchSizes []int
	persistJobTaskItems = func(items []map[string]interface{}) error {
		calls++
		batchSizes = append(batchSizes, len(items))
		persisted = append(persisted, items...)
		return nil
	}

	jt := &JobTask{
		TaskID: 42,
		Finish: make([]JobTaskItem, 0),
	}
	jt.initRuntime()

	for i := 0; i < defaultRealtimeFinishedItems+3; i++ {
		jt.CopyHook("/src/", "/dst/", "file.txt", int64(10), "", taskStatusSuccess, nil, taskItemFile, taskItemTypeCopy, int64(100+i))
	}

	if len(jt.Finish) != defaultRealtimeFinishedItems {
		t.Fatalf("finish len = %d, want capped len %d", len(jt.Finish), defaultRealtimeFinishedItems)
	}
	if len(persisted) != 0 {
		t.Fatalf("persisted len = %d, want 0 before persist batch threshold", len(persisted))
	}
	if calls != 0 {
		t.Fatalf("persist calls = %d with batch sizes %v, want 0 before persist batch threshold", calls, batchSizes)
	}
	if len(jt.pendingPersist) != 3 {
		t.Fatalf("pendingPersist len = %d, want 3 buffered items", len(jt.pendingPersist))
	}
	if jt.pendingPersist[0].CreateTime != int64(100) {
		t.Fatalf("first pending createTime = %v, want 100", jt.pendingPersist[0].CreateTime)
	}
}

func TestFlushPendingTaskItemsPersistsOverflowInOneBatch(t *testing.T) {
	oldPersist := persistJobTaskItems
	defer func() {
		persistJobTaskItems = oldPersist
	}()

	var calls int
	var batchSizes []int
	var persisted []map[string]interface{}
	persistJobTaskItems = func(items []map[string]interface{}) error {
		calls++
		batchSizes = append(batchSizes, len(items))
		persisted = append(persisted, items...)
		return nil
	}

	jt := &JobTask{
		TaskID: 42,
		Finish: make([]JobTaskItem, 0),
	}
	jt.initRuntime()

	for i := 0; i < defaultRealtimeFinishedItems+3; i++ {
		jt.CopyHook("/src/", "/dst/", "file.txt", int64(10), "", taskStatusSuccess, nil, taskItemFile, taskItemTypeCopy, int64(100+i))
	}

	if err := jt.flushPendingTaskItems(); err != nil {
		t.Fatalf("flushPendingTaskItems() error: %v", err)
	}

	if calls != 1 || len(batchSizes) != 1 || batchSizes[0] != 3 {
		t.Fatalf("persist calls = %d with batch sizes %v, want one batch of 3", calls, batchSizes)
	}
	if len(persisted) != 3 {
		t.Fatalf("persisted len = %d, want 3 flushed items", len(persisted))
	}
	if len(jt.pendingPersist) != 0 {
		t.Fatalf("pendingPersist len = %d, want 0 after flush", len(jt.pendingPersist))
	}
}

func TestTaskSubmitMarksTaskFailedWhenFinishedItemPersistenceFails(t *testing.T) {
	oldPersist := persistJobTaskItems
	defer func() {
		persistJobTaskItems = oldPersist
	}()
	persistJobTaskItems = func([]map[string]interface{}) error {
		return errors.New("write failed")
	}

	testDB := newServiceTaskStatusTestDB(t)
	oldDB := mapperDBForServiceTest(testDB)
	defer oldDB()

	client := &JobClient{}
	client.tryMarkDoing()
	jt := &JobTask{
		TaskID:     10,
		JobClient:  client,
		CreateTime: float64(time.Now().Unix()),
		Finish: []JobTaskItem{
			NewCopyJobTaskItem(10, "/src/", "/dst/", "failed.txt", int64(1), "",
				taskStatusFailed, nil, taskItemFile, taskItemTypeCopy, 100),
		},
	}
	jt.initRuntime()
	jt.ScanFinish.Store(true)
	client.setCurrentTask(jt)

	jt.taskSubmit()

	status, errMsg := readServiceTaskStatus(t, testDB, 10)
	if status != taskStatusSystemFailed.Int() {
		t.Fatalf("task status = %d, want 6 after persistence failure", status)
	}
	if !strings.Contains(errMsg, "write failed") {
		t.Fatalf("errMsg = %q, want persistence error", errMsg)
	}
}

func TestJobTaskStartRecoversPanickingSyncWorker(t *testing.T) {
	testDB := newServiceTaskStatusTestDB(t)
	oldDB := mapperDBForServiceTest(testDB)
	defer oldDB()

	client := &JobClient{
		Job: map[string]interface{}{"enable": 1, "isCron": 2},
	}
	if !client.tryMarkDoing() {
		t.Fatalf("tryMarkDoing() = false, want true")
	}
	jt := &JobTask{
		TaskID:     10,
		JobClient:  client,
		Job:        map[string]interface{}{"srcPath": "/src", "dstPath": "/dst"},
		CreateTime: float64(time.Now().Unix()),
		Finish:     make([]JobTaskItem, 0),
		Waiting:    newCopyQueue(),
	}
	jt.initRuntime()
	client.setCurrentTask(jt)

	jt.Start()

	if !client.waitUntilIdle(time.Second) {
		t.Fatalf("job client stayed busy after worker panic")
	}
	status, errMsg := readServiceTaskStatus(t, testDB, 10)
	if status != taskStatusSystemFailed.Int() {
		t.Fatalf("task status = %d, want 6 after worker panic", status)
	}
	if !strings.Contains(errMsg, "panic") {
		t.Fatalf("errMsg = %q, want panic details", errMsg)
	}
}

func newServiceTaskStatusTestDB(t *testing.T) *sql.DB {
	t.Helper()
	testDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error: %v", err)
	}
	t.Cleanup(func() {
		_ = testDB.Close()
	})

	if _, err := testDB.Exec(`CREATE TABLE job_task(
		id integer primary key autoincrement,
		jobId integer,
		status integer DEFAULT 1,
		errMsg text,
		runTime integer,
		taskNum text,
		createTime integer DEFAULT 1
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
		createTime integer DEFAULT 1
	)`); err != nil {
		t.Fatalf("create job_task_item: %v", err)
	}
	if _, err := testDB.Exec("INSERT INTO job_task(id, jobId, status, runTime) VALUES (10, 1, 1, 100)"); err != nil {
		t.Fatalf("insert job_task: %v", err)
	}
	return testDB
}

func mapperDBForServiceTest(testDB *sql.DB) func() {
	return mapper.SetDBForTest(testDB)
}

func readServiceTaskStatus(t *testing.T, testDB *sql.DB, taskID int64) (int, string) {
	t.Helper()
	var status int
	var errMsg sql.NullString
	if err := testDB.QueryRow("SELECT status, errMsg FROM job_task WHERE id=?", taskID).Scan(&status, &errMsg); err != nil {
		t.Fatalf("read job_task: %v", err)
	}
	return status, errMsg.String
}

func TestNewTaskContextUsesConfiguredTimeoutHours(t *testing.T) {
	ctx, cancel := newTaskContext(2)
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatalf("newTaskContext(2) has no deadline, want deadline")
	}
	remaining := time.Until(deadline)
	if remaining < 119*time.Minute || remaining > 121*time.Minute {
		t.Fatalf("deadline remaining = %s, want about 2h", remaining)
	}
}

func TestNewTaskContextFallsBackToCancelableContextWhenTimeoutDisabled(t *testing.T) {
	ctx, cancel := newTaskContext(0)
	defer cancel()

	if _, ok := ctx.Deadline(); ok {
		t.Fatalf("newTaskContext(0) has deadline, want no deadline")
	}
	cancel()
	if err := ctx.Err(); err != context.Canceled {
		t.Fatalf("ctx.Err() = %v, want context.Canceled", err)
	}
}

func TestFinalTaskStatusUsesTimeoutWhenContextDeadlineExceeded(t *testing.T) {
	if status := finalTaskStatus(false, context.DeadlineExceeded, 0); status != taskStatusTimeout {
		t.Fatalf("finalTaskStatus() = %d, want timeout status 5", status)
	}
}

func TestFinalTaskStatusKeepsManualBreakAsStopped(t *testing.T) {
	if status := finalTaskStatus(true, context.DeadlineExceeded, 0); status != taskStatusFailed {
		t.Fatalf("finalTaskStatus() = %d, want stopped status 7", status)
	}
}

func TestScanProgressTracksDiscoveredAndFinishedDirectories(t *testing.T) {
	jt := &JobTask{}

	jt.beginScanWork(scanWork{})
	progress := jt.scanProgress()
	if progress["totalDirs"] != 1 || progress["scannedDirs"] != 0 || progress["remainingDirs"] != 1 {
		t.Fatalf("scanProgress after root start = %#v, want total=1 scanned=0 remaining=1", progress)
	}

	children := make([]scanWork, 0)
	jt.addChildScanWork(&children, scanWork{Mode: scanWorkCompare})
	progress = jt.scanProgress()
	if progress["totalDirs"] != 2 || progress["scannedDirs"] != 0 || progress["remainingDirs"] != 2 {
		t.Fatalf("scanProgress after child discovery = %#v, want total=2 scanned=0 remaining=2", progress)
	}

	jt.finishScanWork()
	progress = jt.scanProgress()
	if progress["totalDirs"] != 2 || progress["scannedDirs"] != 1 || progress["remainingDirs"] != 1 {
		t.Fatalf("scanProgress after root finish = %#v, want total=2 scanned=1 remaining=1", progress)
	}
}

func TestGetCurrentIncludesTaskIDForTaskActions(t *testing.T) {
	jt := &JobTask{
		TaskID:     123,
		CreateTime: float64(time.Now().Unix()),
		Finish:     make([]JobTaskItem, 0),
		Waiting:    newCopyQueue(),
	}
	jt.initRuntime()

	current := jt.GetCurrent()

	if current["taskId"] != int64(123) {
		t.Fatalf("taskId = %v, want 123", current["taskId"])
	}
}

func TestGetCurrentDoesNotCacheFinishedTaskLists(t *testing.T) {
	jt := &JobTask{
		TaskID:     123,
		CreateTime: float64(time.Now().Unix()),
		Finish:     make([]JobTaskItem, 0),
		Waiting:    newCopyQueue(),
	}
	jt.initRuntime()

	for i := 0; i < 3; i++ {
		jt.CopyHook("/src/", "/dst/", "done.txt", int64(10), "", taskStatusSuccess, nil, taskItemFile, taskItemTypeCopy, int64(100+i))
	}

	current := jt.GetCurrent()

	if current["doingTask"] == nil {
		t.Fatalf("doingTask missing from current payload")
	}
	if tasks := jt.CurrentTasks[taskStatusSuccess.Int()]; len(tasks) != 0 {
		t.Fatalf("cached success task list len = %d, want 0 so polling avoids finished-list snapshots", len(tasks))
	}
}

func TestGetCurrentByStatusPagePaginatesRecentFinishedItems(t *testing.T) {
	jt := &JobTask{
		TaskID:     123,
		CreateTime: float64(time.Now().Unix()),
		Finish:     make([]JobTaskItem, 0),
		Waiting:    newCopyQueue(),
	}
	jt.initRuntime()

	for i := 0; i < 5; i++ {
		jt.CopyHook("/src/", "/dst/", "done.txt", int64(10), "", taskStatusSuccess, nil, taskItemFile, taskItemTypeCopy, int64(100+i))
	}

	page := jt.GetCurrentByStatusPage(taskStatusSuccess.Int(), 2, 2)
	items := page["dataList"].([]map[string]interface{})

	if page["count"] != 5 {
		t.Fatalf("count = %v, want 5", page["count"])
	}
	if len(items) != 2 {
		t.Fatalf("page item len = %d, want 2", len(items))
	}
	if items[0]["createTime"] != int64(102) || items[1]["createTime"] != int64(101) {
		t.Fatalf("page createTimes = %v, %v; want 102, 101", items[0]["createTime"], items[1]["createTime"])
	}
}

func TestSyncRetryItemsReadsRetrySourceInBatches(t *testing.T) {
	oldForEach := forEachJobTaskItemsByStatuses
	defer func() {
		forEachJobTaskItemsByStatuses = oldForEach
	}()

	var batchSizes []int
	forEachJobTaskItemsByStatuses = func(taskID int64, statuses []int, batchSize int, fn func([]map[string]interface{}) error) error {
		if taskID != 55 {
			t.Fatalf("taskID = %d, want 55", taskID)
		}
		if len(statuses) != 1 || statuses[0] != taskStatusFailed.Int() {
			t.Fatalf("statuses = %v, want [7]", statuses)
		}
		if batchSize <= 0 {
			t.Fatalf("batchSize = %d, want positive", batchSize)
		}
		batches := [][]map[string]interface{}{
			{
				{"type": 0, "srcPath": "/src/", "dstPath": "/dst/", "fileName": "one.txt", "fileSize": int64(1)},
				{"type": 0, "srcPath": "/src/", "dstPath": "/dst/", "fileName": "two.txt", "fileSize": int64(2)},
			},
			{
				{"type": 0, "srcPath": "/src/", "dstPath": "/dst/", "fileName": "three.txt", "fileSize": int64(3)},
			},
		}
		for _, batch := range batches {
			batchSizes = append(batchSizes, len(batch))
			if err := fn(batch); err != nil {
				return err
			}
		}
		return nil
	}

	jt := &JobTask{
		TaskID:            123,
		Job:               map[string]interface{}{},
		RetrySourceTaskID: 55,
		RetryStatuses:     []taskStatus{taskStatusFailed},
		Waiting:           newCopyQueue(),
	}
	jt.initRuntime()

	jt.syncRetryItems()

	items := jt.Waiting.snapshot()
	if len(items) != 3 {
		t.Fatalf("queued retry items = %d, want 3", len(items))
	}
	if items[0].FileName != "one.txt" || items[1].FileName != "two.txt" || items[2].FileName != "three.txt" {
		t.Fatalf("queued file names = %q, %q, %q; want one/two/three", items[0].FileName, items[1].FileName, items[2].FileName)
	}
	if got := batchSizes; len(got) != 2 || got[0] != 2 || got[1] != 1 {
		t.Fatalf("batch sizes = %v, want [2 1]", got)
	}
	if !jt.ScanFinish.Load() {
		t.Fatalf("ScanFinish = false, want true after retry source is exhausted")
	}
}

func TestRunChildScanWorksFinishesCountedChildrenAfterBreak(t *testing.T) {
	jt := &JobTask{}
	jt.initRuntime()

	children := make([]scanWork, 0)
	jt.addChildScanWork(&children, scanWork{Mode: scanWorkCompare})
	jt.requestBreak()

	jt.runChildScanWorks(children, nil)

	progress := jt.scanProgress()
	if progress["totalDirs"] != 1 || progress["scannedDirs"] != 1 || progress["remainingDirs"] != 0 {
		t.Fatalf("scanProgress after break child finish = %#v, want total=1 scanned=1 remaining=0", progress)
	}
}
