package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

type copyItemTestRuntime struct{}

func (copyItemTestRuntime) context() context.Context {
	return context.Background()
}

func (copyItemTestRuntime) cleanupContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}

func (copyItemTestRuntime) isBreak() bool {
	return false
}

func (copyItemTestRuntime) waitForBreak(time.Duration) bool {
	return true
}

func (copyItemTestRuntime) jobConfig() map[string]interface{} {
	return map[string]interface{}{}
}

func (copyItemTestRuntime) lastWatchingUnix() int64 {
	return 0
}

func (copyItemTestRuntime) finishCopyItem(*CopyItem) {}

func (copyItemTestRuntime) waitForRemoteCopyCompletion(*CopyItem) {}

func (copyItemTestRuntime) notifyProgressChange() {}

type copyItemTestClient struct {
	copyCalls     int
	moveCalls     int
	deleteCalls   int
	cancelCalls   int
	cancelErr     error
	fileExists    bool
	fileExistsErr error
	existsCalls   int
	taskInfoCalls int
	taskInfoFn    func(call int) (map[string]interface{}, error)
}

func (c *copyItemTestClient) CopyFileContext(context.Context, string, string, string) (string, error) {
	c.copyCalls++
	return "", nil
}

func (c *copyItemTestClient) MoveFileContext(context.Context, string, string, string) (string, error) {
	c.moveCalls++
	return "", nil
}

func (c *copyItemTestClient) TaskCancelContext(context.Context, string, taskItemType) error {
	c.cancelCalls++
	return c.cancelErr
}

func (c *copyItemTestClient) TaskDeleteContext(context.Context, string, taskItemType) error {
	c.deleteCalls++
	return nil
}

func (c *copyItemTestClient) TaskInfoContext(context.Context, string, taskItemType) (map[string]interface{}, error) {
	c.taskInfoCalls++
	if c.taskInfoFn != nil {
		return c.taskInfoFn(c.taskInfoCalls)
	}
	return map[string]interface{}{"state": taskStatusSuccess.Int(), "progress": 100}, nil
}

func (c *copyItemTestClient) TaskUndoneListContext(context.Context, taskItemType) ([]map[string]interface{}, error) {
	return []map[string]interface{}{}, nil
}

func (c *copyItemTestClient) DeleteFileContext(context.Context, string, []string, int) error {
	c.deleteCalls++
	return nil
}

func (c *copyItemTestClient) FileExistsContext(context.Context, string, string) (bool, error) {
	c.existsCalls++
	return c.fileExists, c.fileExistsErr
}

func TestCopyItemUsesMoveAPIForMoveItems(t *testing.T) {
	client := &copyItemTestClient{}
	item := newCopyItem(copyItemTestRuntime{}, client, "/src", "/dst", "file.txt", int64(1), taskItemTypeMove)

	item.DoIt()

	if client.moveCalls != 1 {
		t.Fatalf("moveCalls = %d, want 1", client.moveCalls)
	}
	if client.copyCalls != 0 {
		t.Fatalf("copyCalls = %d, want 0 for move item", client.copyCalls)
	}
	if client.deleteCalls != 0 {
		t.Fatalf("deleteCalls = %d, want 0 because AList move already removes source", client.deleteCalls)
	}
}

type breakingCopyItemTestRuntime struct {
	copyItemTestRuntime
}

func (breakingCopyItemTestRuntime) isBreak() bool {
	return true
}

func TestCopyItemKeepsStoppedStatusWhenCancelFails(t *testing.T) {
	client := &copyItemTestClient{cancelErr: errors.New("cancel failed")}
	item := newCopyItem(breakingCopyItemTestRuntime{}, client, "/src", "/dst", "file.txt", int64(1), taskItemTypeCopy)
	item.setTaskID("copy-task")

	item.stopRemoteTask(client, nil)

	if status := item.status(); status != taskStatusStopped {
		t.Fatalf("status = %d, want stopped", status)
	}
	if item.ErrMsg == nil || *item.ErrMsg != "cancel failed" {
		t.Fatalf("ErrMsg = %#v, want cancel failure recorded", item.ErrMsg)
	}
}
