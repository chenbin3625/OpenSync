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
	monitor := &copyTaskMonitor{
		jt:      jt,
		watches: make(map[string]*copyTaskWatch),
		stopCh:  make(chan struct{}),
	}
	watch := &copyTaskWatch{
		ci:       item,
		taskID:   "task-1",
		copyType: taskItemTypeCopy,
		done:     make(chan struct{}),
	}
	monitor.watches[monitor.watchKey(watch.taskID, watch.copyType)] = watch
	return monitor, watch
}

func TestCopyMonitorPollTaskInfo404MarksSuccessWhenDstExists(t *testing.T) {
	client := &copyItemTestClient{
		fileExists: true,
		taskInfoFn: func(int) (map[string]interface{}, error) {
			return nil, errors.New("404 not found")
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
			return nil, errors.New("404 not found")
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

func TestCopyMonitorPollTaskInfoTransientErrorsExhaustedMarksFailed(t *testing.T) {
	client := &copyItemTestClient{
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
		ci:       item,
		taskID:   "copy-task",
		copyType: taskItemTypeCopy,
		done:     make(chan struct{}),
	}
	monitor.watches[monitor.watchKey(watch.taskID, watch.copyType)] = watch

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
	monitor := &copyTaskMonitor{
		jt:      jt,
		watches: make(map[string]*copyTaskWatch),
		stopCh:  make(chan struct{}),
	}
	monitor.stop()

	item := newCopyItem(jt, client, "/src", "/dst", "file.txt", int64(1), taskItemTypeCopy)
	item.setTaskID("late-task")
	monitor.track(item)

	if client.cancelCalls != 1 {
		t.Fatalf("cancelCalls = %d, want 1", client.cancelCalls)
	}
	if status := item.status(); status != taskStatusStopped {
		t.Fatalf("status = %d, want stopped", status)
	}
}
