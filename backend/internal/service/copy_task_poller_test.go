package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newCopyMonitorTestJobTask(client copyItemClient) *JobTask {
	ctx, cancel := context.WithCancel(context.Background())
	return &JobTask{
		TaskID:                    1,
		copyMonitorClientOverride: client,
		ctx:                       ctx,
		cancel:                    cancel,
	}
}

func newCopyMonitorWatch(client copyItemClient, opts ...func(*CopyItem)) (*copyTaskMonitor, *copyTaskWatch) {
	jt := newCopyMonitorTestJobTask(client)
	item := newCopyItem(jt, client, "/src", "/dst", "file.txt", int64(1), taskItemTypeCopy)
	item.setTaskID("task-1")
	for _, opt := range opts {
		opt(item)
	}
	monitor := &copyTaskMonitor{
		jt:      jt,
		watches: make(map[string]*copyTaskWatch),
		stopCh:  make(chan struct{}),
	}
	watch := &copyTaskWatch{
		key:      "task-1|0|1",
		ci:       item,
		taskID:   "task-1",
		copyType: taskItemTypeCopy,
		done:     make(chan struct{}),
	}
	monitor.watches[watch.key] = watch
	return monitor, watch
}

func TestCopyMonitorPollTaskInfo404MarksSuccessWhenDstExists(t *testing.T) {
	client := &copyItemTestClient{
		fileExists: true,
		taskInfoFn: func(int) (map[string]interface{}, error) {
			return nil, &alistStatusError{httpStatus: 404, alistCode: 404}
		},
	}
	monitor, watch := newCopyMonitorWatch(client)

	if !monitor.pollTaskInfo(watch) {
		t.Fatal("pollTaskInfo() = false, want true")
	}
	if status := watch.ci.status(); status != taskStatusSuccess {
		t.Fatalf("status = %d, want success after 404 with dst present", status)
	}
	if client.existsCalls != 1 {
		t.Fatalf("existsCalls = %d, want 1", client.existsCalls)
	}
}

func TestCopyMonitorPollTaskInfo404MarksFailedWhenDstMissing(t *testing.T) {
	client := &copyItemTestClient{
		fileExists: false,
		taskInfoFn: func(int) (map[string]interface{}, error) {
			return nil, &alistStatusError{httpStatus: 404, alistCode: 404}
		},
	}
	monitor, watch := newCopyMonitorWatch(client)

	if !monitor.pollTaskInfo(watch) {
		t.Fatal("pollTaskInfo() = false, want true")
	}
	if status := watch.ci.status(); status != taskStatusFailed {
		t.Fatalf("status = %d, want failed after 404 with dst missing", status)
	}
}

func TestCopyMonitorPollTaskInfoTransientErrorsRetryThenSucceed(t *testing.T) {
	client := &copyItemTestClient{
		taskInfoFn: func(call int) (map[string]interface{}, error) {
			if call < maxTransientPollErrors {
				return nil, errors.New("connection refused")
			}
			return map[string]interface{}{"state": taskStatusSuccess.Int(), "progress": 100}, nil
		},
	}
	monitor, watch := newCopyMonitorWatch(client)

	for call := 1; call < maxTransientPollErrors; call++ {
		if monitor.pollTaskInfo(watch) {
			t.Fatalf("pollTaskInfo call %d finished early", call)
		}
	}
	if !monitor.pollTaskInfo(watch) {
		t.Fatal("pollTaskInfo() = false, want true after transient errors recovered")
	}
	if status := watch.ci.status(); status != taskStatusSuccess {
		t.Fatalf("status = %d, want success after transient blips recovered", status)
	}
	if client.taskInfoCalls != maxTransientPollErrors {
		t.Fatalf("taskInfoCalls = %d, want %d", client.taskInfoCalls, maxTransientPollErrors)
	}
}

func TestCopyMonitorPollTaskInfoTransientErrorsExhaustedCancelsAndBlocksRetryOnCancelFail(t *testing.T) {
	client := &copyItemTestClient{
		cancelErr: errors.New("cancel failed"),
		taskInfoFn: func(int) (map[string]interface{}, error) {
			return nil, errors.New("connection refused")
		},
	}
	monitor, watch := newCopyMonitorWatch(client)

	for call := 1; call < maxTransientPollErrors; call++ {
		if monitor.pollTaskInfo(watch) {
			t.Fatalf("pollTaskInfo call %d finished early", call)
		}
	}
	if !monitor.pollTaskInfo(watch) {
		t.Fatal("pollTaskInfo() = false, want true after transient errors exhausted")
	}
	if status := watch.ci.status(); status != taskStatusFailed {
		t.Fatalf("status = %d, want failed after transient errors exhausted", status)
	}
	if client.taskInfoCalls != maxTransientPollErrors {
		t.Fatalf("taskInfoCalls = %d, want %d", client.taskInfoCalls, maxTransientPollErrors)
	}
	if client.cancelCalls != 1 {
		t.Fatalf("cancelCalls = %d, want 1 remote cancel attempt", client.cancelCalls)
	}
	if !watch.ci.isRetryBlocked() {
		t.Fatal("retryBlocked = false, want true when remote cancel failed")
	}
}

func TestCopyMonitorAbortWatchKeepsStoppedStatusWhenCancelFails(t *testing.T) {
	client := &copyItemTestClient{cancelErr: errors.New("cancel failed")}
	monitor, watch := newCopyMonitorWatch(client)

	monitor.abortWatch(watch, nil)

	if status := watch.ci.status(); status != taskStatusStopped {
		t.Fatalf("status = %d, want stopped", status)
	}
	if watch.ci.ErrMsg == nil || *watch.ci.ErrMsg != "cancel failed" {
		t.Fatalf("ErrMsg = %#v, want cancel failure recorded", watch.ci.ErrMsg)
	}
}

func TestCopyMonitorAbortAllStopsWhenContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := &copyItemTestClient{}
	jt := &JobTask{
		TaskID:                    1,
		copyMonitorClientOverride: client,
		ctx:                       ctx,
		cancel:                    cancel,
	}
	item := newCopyItem(jt, client, "/src", "/dst", "file.txt", int64(1), taskItemTypeCopy)
	item.setTaskID("copy-task")
	monitor := &copyTaskMonitor{
		jt:      jt,
		watches: make(map[string]*copyTaskWatch),
		stopCh:  make(chan struct{}),
	}
	watch := &copyTaskWatch{
		key:      "copy-task|0|1",
		ci:       item,
		taskID:   "copy-task",
		copyType: taskItemTypeCopy,
		done:     make(chan struct{}),
	}
	monitor.watches[watch.key] = watch

	cancel()
	monitor.abortAll(ctx.Err())

	if status := item.status(); status != taskStatusStopped {
		t.Fatalf("status = %d, want stopped", status)
	}
	if client.cancelCalls != 1 || client.deleteCalls != 1 {
		t.Fatalf("cancel/delete calls = %d/%d, want 1/1", client.cancelCalls, client.deleteCalls)
	}
}

func TestCopyMonitorApplyTaskInfoFinishesTerminalWatchEvenWhenUnchanged(t *testing.T) {
	client := &copyItemTestClient{}
	monitor, watch := newCopyMonitorWatch(client, func(ci *CopyItem) {
		ci.setProgress(taskStatusSuccess, 100, nil)
	})

	if !monitor.applyTaskInfo(watch, map[string]interface{}{
		"state":    taskStatusSuccess.Int(),
		"progress": 100.0,
	}) {
		t.Fatal("applyTaskInfo() = false for terminal state, want true")
	}
	select {
	case <-watch.done:
	case <-time.After(time.Second):
		t.Fatal("watch.done was not closed for unchanged terminal state")
	}
	monitor.wg.Wait()
}

func TestCopyMonitorAbortWatchWaitsWhenAlreadyClaimed(t *testing.T) {
	client := &copyItemTestClient{}
	monitor, watch := newCopyMonitorWatch(client)

	// Simulate loop() claiming the watch by removing it from monitor.watches.
	monitor.mu.Lock()
	delete(monitor.watches, watch.key)
	monitor.mu.Unlock()

	returned := make(chan struct{})
	go func() {
		defer close(returned)
		monitor.abortWatch(watch, context.Canceled)
	}()

	select {
	case <-returned:
		t.Fatal("abortWatch returned before claimed watch closed done")
	case <-time.After(40 * time.Millisecond):
	}

	watch.ci.setStatus(taskStatusStopped)
	watch.closeDone()

	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("abortWatch did not return after claimed watch finished")
	}
}

type raceSafeCopyClient struct {
	cancelCalls atomic.Int32
}

func (c *raceSafeCopyClient) CopyFileContext(context.Context, string, string, string) (string, error) {
	return "", nil
}
func (c *raceSafeCopyClient) MoveFileContext(context.Context, string, string, string) (string, error) {
	return "", nil
}
func (c *raceSafeCopyClient) TaskCancelContext(context.Context, string, taskItemType) error {
	c.cancelCalls.Add(1)
	return nil
}
func (c *raceSafeCopyClient) TaskDeleteContext(context.Context, string, taskItemType) error {
	return nil
}
func (c *raceSafeCopyClient) TaskInfoContext(context.Context, string, taskItemType) (map[string]interface{}, error) {
	return map[string]interface{}{"state": taskStatusSuccess.Int(), "progress": 100}, nil
}
func (c *raceSafeCopyClient) TaskUndoneListContext(context.Context, taskItemType) ([]map[string]interface{}, error) {
	return []map[string]interface{}{}, nil
}
func (c *raceSafeCopyClient) DeleteFileContext(context.Context, string, []string, int) error {
	return nil
}
func (c *raceSafeCopyClient) FileExistsContext(context.Context, string, string) (bool, error) {
	return true, nil
}
func (c *raceSafeCopyClient) GetFileInfoContext(context.Context, string, string) (*FileListEntry, bool, error) {
	return &FileListEntry{Name: "file.txt", Size: 1}, true, nil
}

func TestCopyMonitorStopAndTrackConcurrentRaceAndDuplicateTaskIDs(t *testing.T) {
	for iter := 0; iter < 5; iter++ {
		client := &raceSafeCopyClient{}
		jt := newCopyMonitorTestJobTask(client)
		monitor := jt.ensureCopyMonitor()

		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				item := newCopyItem(jt, client, "/src", "/dst", "file.txt", int64(1), taskItemTypeCopy)
				// Intentionally reuse the exact same taskID across concurrent watches.
				item.setTaskID("dup-task-id")
				monitor.track(item)
			}()
		}

		time.Sleep(5 * time.Millisecond)
		jt.requestBreak()
		monitor.stop()

		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatalf("iter %d: track() deadlocked during concurrent stop()", iter)
		}
	}
}
