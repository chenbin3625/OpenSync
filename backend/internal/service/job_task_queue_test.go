package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"opensync/internal/config"
	"sync/atomic"
	"testing"
	"time"
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
		Finish:  make([]map[string]interface{}, 0),
		Waiting: newCopyQueue(),
		AlistClient: &AlistClient{
			URL:    server.URL,
			client: server.Client(),
		},
	}
	jt.initRuntime()

	item := &CopyItem{
		SrcPath:     "/src",
		DstPath:     "/dst",
		FileName:    "file.txt",
		FileSize:    int64(11),
		CopyType:    0,
		Status:      0,
		CreateTime:  100,
		jobTask:     jt,
		alistClient: jt.AlistClient,
	}

	item.DoIt()

	if got := attempts.Load(); got != 3 {
		t.Fatalf("copy attempts = %d, want 3", got)
	}
	if status := item.status(); status != 2 {
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
	if jt.Finish[0]["status"] != 2 {
		t.Fatalf("finish status = %v, want 2", jt.Finish[0]["status"])
	}
	switch errMsg := jt.Finish[0]["errMsg"].(type) {
	case nil:
	case *string:
		if errMsg != nil {
			t.Fatalf("finish errMsg = %q, want nil", *errMsg)
		}
	default:
		t.Fatalf("finish errMsg = %#v, want nil", errMsg)
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
		Finish:  make([]map[string]interface{}, 0),
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
		if item["status"] != 4 {
			t.Fatalf("finish[%d].status = %v, want 4", i, item["status"])
		}
		if item["taskId"] != int64(42) {
			t.Fatalf("finish[%d].taskId = %v, want 42", i, item["taskId"])
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
		Finish: make([]map[string]interface{}, 0),
	}
	jt.initRuntime()

	for i := 0; i < defaultRealtimeFinishedItems+3; i++ {
		jt.CopyHook("/src/", "/dst/", "file.txt", int64(10), "", 2, nil, 0, 0, int64(100+i))
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
	if jt.pendingPersist[0]["createTime"] != int64(100) {
		t.Fatalf("first pending createTime = %v, want 100", jt.pendingPersist[0]["createTime"])
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
		Finish: make([]map[string]interface{}, 0),
	}
	jt.initRuntime()

	for i := 0; i < defaultRealtimeFinishedItems+3; i++ {
		jt.CopyHook("/src/", "/dst/", "file.txt", int64(10), "", 2, nil, 0, 0, int64(100+i))
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
	if status := finalTaskStatus(false, context.DeadlineExceeded, 0); status != 5 {
		t.Fatalf("finalTaskStatus() = %d, want timeout status 5", status)
	}
}

func TestFinalTaskStatusKeepsManualBreakAsStopped(t *testing.T) {
	if status := finalTaskStatus(true, context.DeadlineExceeded, 0); status != 7 {
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
		Finish:     make([]map[string]interface{}, 0),
		Waiting:    newCopyQueue(),
	}
	jt.initRuntime()

	current := jt.GetCurrent()

	if current["taskId"] != int64(123) {
		t.Fatalf("taskId = %v, want 123", current["taskId"])
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
