package service

import (
	"context"
	"errors"
	"testing"
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
	monitor := newCopyTaskMonitor()
	watch := &copyTaskWatch{
		jt:       jt,
		ci:       item,
		taskID:   "task-1",
		copyType: taskItemTypeCopy,
		done:     make(chan struct{}),
	}
	monitor.watches[watchKey(jt, watch.taskID, watch.copyType)] = watch
	return monitor, watch
}

func TestCopyMonitorPollTaskInfo404MarksSuccessWhenDstExists(t *testing.T) {
	client := &copyItemTestClient{
		fileExists: true,
		taskInfoFn: func(int) (alistRemoteTask, error) {
			return alistRemoteTask{}, errors.New("404 not found")
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
		taskInfoFn: func(int) (alistRemoteTask, error) {
			return alistRemoteTask{}, errors.New("404 not found")
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
		taskInfoFn: func(call int) (alistRemoteTask, error) {
			if call < maxTransientPollErrors {
				return alistRemoteTask{}, errors.New("connection refused")
			}
			return alistRemoteTask{State: taskStatusSuccess.Int(), Progress: 100}, nil
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

func TestCopyMonitorPollTaskInfoTransientErrorsExhaustedMarksFailed(t *testing.T) {
	client := &copyItemTestClient{
		taskInfoFn: func(int) (alistRemoteTask, error) {
			return alistRemoteTask{}, errors.New("connection refused")
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
	monitor := newCopyTaskMonitor()
	watch := &copyTaskWatch{
		jt:       jt,
		ci:       item,
		taskID:   "copy-task",
		copyType: taskItemTypeCopy,
		done:     make(chan struct{}),
	}
	monitor.watches[watchKey(jt, watch.taskID, watch.copyType)] = watch

	cancel()
	monitor.abortAll(ctx.Err())

	if status := item.status(); status != taskStatusStopped {
		t.Fatalf("status = %d, want stopped", status)
	}
	if client.cancelCalls != 1 || client.deleteCalls != 1 {
		t.Fatalf("cancel/delete calls = %d/%d, want 1/1", client.cancelCalls, client.deleteCalls)
	}
}

func TestCopyMonitorTrackAfterStopAbortsWithoutStartingLoop(t *testing.T) {
	client := &copyItemTestClient{}
	jt := newCopyMonitorTestJobTask(client)
	monitor := newCopyTaskMonitor()
	monitor.stop()

	item := newCopyItem(jt, client, "/src", "/dst", "file.txt", int64(1), taskItemTypeCopy)
	item.setTaskID("late-task")
	monitor.track(jt, item)

	if client.cancelCalls != 1 {
		t.Fatalf("cancelCalls = %d, want 1", client.cancelCalls)
	}
	if status := item.status(); status != taskStatusStopped {
		t.Fatalf("status = %d, want stopped", status)
	}
}

func resetCopyMonitorsForTest(t *testing.T) {
	t.Helper()
	copyMonitors.mu.Lock()
	copyMonitors.byAlist = make(map[int64]*copyTaskMonitor)
	copyMonitors.mu.Unlock()
	t.Cleanup(func() {
		copyMonitors.mu.Lock()
		copyMonitors.byAlist = make(map[int64]*copyTaskMonitor)
		copyMonitors.mu.Unlock()
	})
}

func TestEnsureCopyMonitorSharesByAlistID(t *testing.T) {
	resetCopyMonitorsForTest(t)
	shared := &AlistClient{AlistID: 9}
	a := &JobTask{TaskID: 1, AlistClient: shared}
	b := &JobTask{TaskID: 2, AlistClient: shared}
	first := a.ensureCopyMonitor()
	second := b.ensureCopyMonitor()
	if first != second {
		t.Fatal("jobs on the same alist must share a copy monitor")
	}
	if !first.shared || first.refs != 2 {
		t.Fatalf("shared=%v refs=%d, want shared refs=2", first.shared, first.refs)
	}
	other := &JobTask{TaskID: 3, AlistClient: &AlistClient{AlistID: 10}}
	if other.ensureCopyMonitor() == first {
		t.Fatal("jobs on different alists must not share a copy monitor")
	}
}

func TestEnsureCopyMonitorOverrideStaysPrivate(t *testing.T) {
	resetCopyMonitorsForTest(t)
	client := &copyItemTestClient{}
	a := &JobTask{TaskID: 1, copyMonitorClientOverride: client, AlistClient: &AlistClient{AlistID: 9}}
	b := &JobTask{TaskID: 2, copyMonitorClientOverride: client, AlistClient: &AlistClient{AlistID: 9}}
	if a.ensureCopyMonitor() == b.ensureCopyMonitor() {
		t.Fatal("override clients must keep per-task monitors")
	}
}

func TestSharedCopyMonitorReleaseStopsOnlyWhenLastJobLeaves(t *testing.T) {
	resetCopyMonitorsForTest(t)
	shared := &AlistClient{AlistID: 11}
	a := &JobTask{TaskID: 1, AlistClient: shared}
	b := &JobTask{TaskID: 2, AlistClient: shared}
	monitor := a.ensureCopyMonitor()
	_ = b.ensureCopyMonitor()
	a.stopCopyMonitor()
	copyMonitors.mu.Lock()
	_, still := copyMonitors.byAlist[11]
	refs := monitor.refs
	copyMonitors.mu.Unlock()
	if !still || refs != 1 {
		t.Fatalf("after first release registry=%v refs=%d, want still registered refs=1", still, refs)
	}
	b.stopCopyMonitor()
	copyMonitors.mu.Lock()
	_, still = copyMonitors.byAlist[11]
	copyMonitors.mu.Unlock()
	if still {
		t.Fatal("last release must drop the shared monitor")
	}
}

func TestFetchUndoneByTypeIssuesOneListForManyWatches(t *testing.T) {
	client := &copyItemTestClient{
		undoneTasks: []alistRemoteTask{
			{ID: "task-1", State: 1, Progress: 40},
			{ID: "task-2", State: 1, Progress: 70},
		},
	}
	jobA := newCopyMonitorTestJobTask(client)
	jobB := newCopyMonitorTestJobTask(client)
	jobB.TaskID = 2
	monitor := newCopyTaskMonitor()
	itemA := newCopyItem(jobA, client, "/src", "/dst", "a.txt", int64(1), taskItemTypeCopy)
	itemA.setTaskID("task-1")
	itemB := newCopyItem(jobB, client, "/src", "/dst", "b.txt", int64(1), taskItemTypeCopy)
	itemB.setTaskID("task-2")
	result := monitor.fetchUndoneByType([]*copyTaskWatch{
		{jt: jobA, ci: itemA, taskID: "task-1", copyType: taskItemTypeCopy, done: make(chan struct{})},
		{jt: jobB, ci: itemB, taskID: "task-2", copyType: taskItemTypeCopy, done: make(chan struct{})},
	})
	if client.undoneCalls != 1 {
		t.Fatalf("undoneCalls = %d, want 1 for two watches of the same type", client.undoneCalls)
	}
	if result[taskItemTypeCopy]["task-1"].idString() == "" || result[taskItemTypeCopy]["task-2"].idString() == "" {
		t.Fatalf("undone snapshot = %#v", result)
	}
}
