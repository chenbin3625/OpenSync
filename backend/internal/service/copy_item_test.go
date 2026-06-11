package service

import (
	"context"
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

type copyItemTestClient struct {
	copyCalls int
	moveCalls int
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
	return nil
}

func (c *copyItemTestClient) TaskDeleteContext(context.Context, string, taskItemType) error {
	return nil
}

func (c *copyItemTestClient) TaskInfoContext(context.Context, string, taskItemType) (map[string]interface{}, error) {
	return map[string]interface{}{"state": taskStatusSuccess.Int(), "progress": 100}, nil
}

func (c *copyItemTestClient) DeleteFileContext(context.Context, string, []string, int) error {
	return nil
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
}
