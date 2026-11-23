package service

import (
	"context"
	"errors"
	"opensync/internal/config"
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
	fileEntry     *FileListEntry
	existsCalls   int
	fileExistsFn  func() (bool, error)
	getFileInfoFn func(context.Context, string, string) (*FileListEntry, bool, error)
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
	if c.fileExistsFn != nil {
		return c.fileExistsFn()
	}
	return c.fileExists, c.fileExistsErr
}

func (c *copyItemTestClient) GetFileInfoContext(ctx context.Context, dir, name string) (*FileListEntry, bool, error) {
	c.existsCalls++
	if c.getFileInfoFn != nil {
		return c.getFileInfoFn(ctx, dir, name)
	}
	if c.fileExistsFn != nil {
		exists, err := c.fileExistsFn()
		if err != nil || !exists {
			return nil, exists, err
		}
		if c.fileEntry != nil {
			copied := *c.fileEntry
			return &copied, true, nil
		}
		return &FileListEntry{Name: name, Size: 1}, true, nil
	}
	if c.fileExistsErr != nil {
		return nil, false, c.fileExistsErr
	}
	if !c.fileExists {
		return nil, false, nil
	}
	if c.fileEntry != nil {
		copied := *c.fileEntry
		return &copied, true, nil
	}
	return &FileListEntry{Name: name, Size: 1}, true, nil
}

func TestCopyItemUsesMoveAPIForMoveItems(t *testing.T) {
	// The stub returns no remote task id, so DoIt verifies the destination;
	// fileExists=true represents the synchronous-completion path.
	client := &copyItemTestClient{fileExists: true}
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

type deadlineCopyItemTestRuntime struct {
	copyItemTestRuntime
	stopped bool
}

func (runtime *deadlineCopyItemTestRuntime) isBreak() bool {
	return runtime.stopped
}

func TestCopyItemStopsWhenSynchronousVerificationHitsDeadline(t *testing.T) {
	oldConfig := config.GetConfig()
	defer config.SetConfigForTest(oldConfig)
	config.SetConfigForTest(&config.Config{Server: config.ServerConfig{MaxRetries: 0}})

	runtime := &deadlineCopyItemTestRuntime{}
	client := &copyItemTestClient{
		fileExistsFn: func() (bool, error) {
			runtime.stopped = true
			return false, errors.New("verification interrupted")
		},
	}
	item := newCopyItem(runtime, client, "/src", "/dst", "file.txt", int64(1), taskItemTypeCopy)

	item.DoIt()

	if status := item.status(); status != taskStatusStopped {
		t.Fatalf("item status = %d, want stopped status %d", status, taskStatusStopped)
	}
	if item.ErrMsg != nil {
		t.Fatalf("item ErrMsg = %v, want nil", item.ErrMsg)
	}
	if client.copyCalls != 1 || client.existsCalls != 1 {
		t.Fatalf("copy/existence calls = %d/%d, want 1/1", client.copyCalls, client.existsCalls)
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

func TestVerifyDstExistsChecksSizeMD5AndOverwriteMtime(t *testing.T) {
	const goodMD5 = "0123456789abcdef0123456789abcdef"
	const wrongMD5 = "fedcba9876543210fedcba9876543210"
	const oldTime int64 = 1_700_000_000
	const newTime int64 = 1_700_000_500

	t.Run("rejects size mismatch", func(t *testing.T) {
		client := &copyItemTestClient{
			fileExists: true,
			fileEntry:  &FileListEntry{Name: "a.bin", Size: 50, Modified: newTime},
		}
		item := newCopyItem(copyItemTestRuntime{}, client, "/src", "/dst", "a.bin", int64(100), taskItemTypeCopy)
		ok, err := item.verifyDstExists(copyItemTestRuntime{}, client)
		if err != nil || ok {
			t.Fatalf("verifyDstExists(size mismatch) = (%v, %v), want (false, nil)", ok, err)
		}
	})

	t.Run("rejects MD5 mismatch", func(t *testing.T) {
		client := &copyItemTestClient{
			fileExists: true,
			fileEntry:  &FileListEntry{Name: "a.bin", Size: 100, Modified: newTime, HashInfo: map[string]interface{}{"md5": wrongMD5}},
		}
		item := newCopyItem(copyItemTestRuntime{}, client, "/src", "/dst", "a.bin", int64(100), taskItemTypeCopy)
		item.expectedMD5 = goodMD5
		ok, err := item.verifyDstExists(copyItemTestRuntime{}, client)
		if err != nil || ok {
			t.Fatalf("verifyDstExists(MD5 mismatch) = (%v, %v), want (false, nil)", ok, err)
		}
	})

	t.Run("rejects stale same-size overwrite without newer mtime", func(t *testing.T) {
		client := &copyItemTestClient{
			fileExists: true,
			fileEntry:  &FileListEntry{Name: "a.bin", Size: 100, Modified: oldTime},
		}
		item := newCopyItem(copyItemTestRuntime{}, client, "/src", "/dst", "a.bin", int64(100), taskItemTypeCopy)
		item.preCopyDst = &FileMetadata{Size: 100, Modified: oldTime}
		item.setAttemptStartTime(1_700_000_100)
		ok, err := item.verifyDstExists(copyItemTestRuntime{}, client)
		if err != nil || ok {
			t.Fatalf("verifyDstExists(stale overwrite) = (%v, %v), want (false, nil)", ok, err)
		}
	})

	t.Run("accepts updated overwrite with newer mtime and matching MD5", func(t *testing.T) {
		client := &copyItemTestClient{
			fileExists: true,
			fileEntry:  &FileListEntry{Name: "a.bin", Size: 100, Modified: newTime, HashInfo: map[string]interface{}{"md5": goodMD5}},
		}
		item := newCopyItem(copyItemTestRuntime{}, client, "/src", "/dst", "a.bin", int64(100), taskItemTypeCopy)
		item.expectedMD5 = goodMD5
		item.preCopyDst = &FileMetadata{Size: 100, Modified: oldTime}
		item.setAttemptStartTime(1_700_000_100)
		ok, err := item.verifyDstExists(copyItemTestRuntime{}, client)
		if err != nil || !ok {
			t.Fatalf("verifyDstExists(valid overwrite) = (%v, %v), want (true, nil)", ok, err)
		}
	})
}

func TestConfirmSynchronousCopyReturnsFalseWhenProbesExhaustErrors(t *testing.T) {
	oldDelay := copyRetryDelay
	copyRetryDelay = func(int) time.Duration { return 0 }
	defer func() { copyRetryDelay = oldDelay }()

	client := &copyItemTestClient{
		fileExistsErr: errors.New("probe error"),
	}
	item := newCopyItem(copyItemTestRuntime{}, client, "/src", "/dst", "a.bin", int64(10), taskItemTypeCopy)
	if ok := item.confirmSynchronousCopy(copyItemTestRuntime{}, client); ok {
		t.Fatal("confirmSynchronousCopy() = true after exhausted probe errors, want false")
	}
	if client.existsCalls != maxTransientPollErrors {
		t.Fatalf("existsCalls = %d, want %d", client.existsCalls, maxTransientPollErrors)
	}
}

type retryBlockedTestRuntime struct {
	copyItemTestRuntime
}

func (retryBlockedTestRuntime) waitForRemoteCopyCompletion(ci *CopyItem) {
	ci.setRetryBlocked(true)
	ci.setFailure(errors.New("remote still running"))
}

type taskIDCopyClient struct {
	copyItemTestClient
}

func (c *taskIDCopyClient) CopyFileContext(context.Context, string, string, string) (string, error) {
	c.copyCalls++
	return "remote-task-1", nil
}

func TestCopyItemDoesNotRetryWhenRetryBlocked(t *testing.T) {
	oldConfig := config.GetConfig()
	oldDelay := copyRetryDelay
	defer func() {
		config.SetConfigForTest(oldConfig)
		copyRetryDelay = oldDelay
	}()
	copyRetryDelay = func(int) time.Duration { return 0 }
	config.SetConfigForTest(&config.Config{Server: config.ServerConfig{MaxRetries: 3}})

	client := &taskIDCopyClient{}
	item := newCopyItem(retryBlockedTestRuntime{}, client, "/src", "/dst", "a.bin", int64(10), taskItemTypeCopy)
	item.DoIt()

	if client.copyCalls != 1 {
		t.Fatalf("copyCalls = %d, want 1 when retryBlocked=true", client.copyCalls)
	}
	if item.status() != taskStatusFailed {
		t.Fatalf("status = %d, want failed", item.status())
	}
}
