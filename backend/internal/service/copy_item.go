package service

import (
	"context"
	"errors"
	"opensync/pkg/util"
	"sync"
	"time"
)

// CopyItem represents a single file copy operation
type copyItemRuntime interface {
	context() context.Context
	cleanupContext() (context.Context, context.CancelFunc)
	isBreak() bool
	waitForBreak(time.Duration) bool
	jobConfig() map[string]interface{}
	lastWatchingUnix() int64
	finishCopyItem(*CopyItem)
	waitForRemoteCopyCompletion(*CopyItem)
	notifyProgressChange()
}

type copyItemClient interface {
	CopyFileContext(context.Context, string, string, string) (string, error)
	MoveFileContext(context.Context, string, string, string) (string, error)
	TaskCancelContext(context.Context, string, taskItemType) error
	TaskDeleteContext(context.Context, string, taskItemType) error
	TaskInfoContext(context.Context, string, taskItemType) (map[string]interface{}, error)
	TaskUndoneListContext(context.Context, taskItemType) ([]map[string]interface{}, error)
	DeleteFileContext(context.Context, string, []string, int) error
	FileExistsContext(context.Context, string, string) (bool, error)
}

// maxTransientPollErrors is the number of consecutive transient TaskInfo polling
// errors (network blips, 5xx, connection limits) tolerated before a copy item is
// declared failed. Each retry rides the loop's existing poll-interval backoff.
const maxTransientPollErrors = 3

type CopyItem struct {
	mu          sync.RWMutex
	SrcPath     string
	DstPath     string
	FileName    string
	FileSize    interface{}
	CopyType    taskItemType
	AlistTaskID string
	Status      taskStatus
	Progress    float64
	ErrMsg      *string
	CreateTime  int64
	DoingKey    int64

	runtime copyItemRuntime
	client  copyItemClient
}

func newCopyItem(runtime copyItemRuntime, client copyItemClient, srcPath, dstPath, fileName string, fileSize interface{}, copyType taskItemType) *CopyItem {
	return &CopyItem{
		SrcPath:    srcPath,
		DstPath:    dstPath,
		FileName:   fileName,
		FileSize:   fileSize,
		CopyType:   copyType,
		Status:     taskStatusWaiting,
		Progress:   0,
		CreateTime: time.Now().Unix(),
		runtime:    runtime,
		client:     client,
	}
}

func (ci *CopyItem) copyRuntime() copyItemRuntime {
	return ci.runtime
}

func (ci *CopyItem) copyClient() copyItemClient {
	return ci.client
}

func (ci *CopyItem) setStatus(status taskStatus) {
	ci.mu.Lock()
	ci.Status = status
	ci.mu.Unlock()
}

func (ci *CopyItem) setTaskID(taskID string) {
	ci.mu.Lock()
	ci.AlistTaskID = taskID
	ci.mu.Unlock()
}

func (ci *CopyItem) setFailure(err error) {
	errMsg := err.Error()
	ci.mu.Lock()
	ci.Status = taskStatusFailed
	ci.Progress = 0
	ci.ErrMsg = &errMsg
	ci.mu.Unlock()
}

func (ci *CopyItem) setRunning() {
	ci.mu.Lock()
	ci.Status = taskStatusRunning
	ci.Progress = 0
	ci.ErrMsg = nil
	ci.AlistTaskID = ""
	ci.mu.Unlock()
}

func (ci *CopyItem) setRetrying(err error) {
	errMsg := err.Error()
	ci.mu.Lock()
	ci.Status = taskStatusRetrying
	ci.Progress = 0
	ci.ErrMsg = &errMsg
	ci.AlistTaskID = ""
	ci.mu.Unlock()
}

func (ci *CopyItem) setProgress(status taskStatus, progress float64, errMsg *string) {
	ci.mu.Lock()
	ci.Status = status
	ci.Progress = progress
	ci.ErrMsg = errMsg
	ci.mu.Unlock()
	if runtime := ci.copyRuntime(); runtime != nil {
		runtime.notifyProgressChange()
	}
}

func (ci *CopyItem) status() taskStatus {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	return ci.Status
}

func (ci *CopyItem) taskID() string {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	return ci.AlistTaskID
}

func (ci *CopyItem) progress() float64 {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	return ci.Progress
}

func (ci *CopyItem) countableWaitSize() int64 {
	if ci.CopyType == taskItemTypeDelete || ci.FileSize == nil {
		return 0
	}
	return util.ToInt64(ci.FileSize)
}

func (ci *CopyItem) ToMap(taskID int64) map[string]interface{} {
	ci.mu.RLock()
	defer ci.mu.RUnlock()

	itemMap := NewCopyJobTaskItem(taskID, ci.SrcPath, ci.DstPath, ci.FileName, ci.FileSize,
		ci.AlistTaskID, ci.Status, ci.ErrMsg, taskItemFile, ci.CopyType, ci.CreateTime).ToMap()
	itemMap["progress"] = ci.Progress
	return itemMap
}

// DoIt executes the copy operation in a goroutine.
func (ci *CopyItem) DoIt() {
	runtime := ci.copyRuntime()
	client := ci.copyClient()
	maxRetries := runtimeTaskLimits().MaxRetries
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if runtime.isBreak() {
			ci.setStatus(taskStatusStopped)
			break
		}

		ci.setRunning()
		taskID, err := ci.startTransfer(runtime.context(), client)
		if err != nil {
			if errors.Is(err, context.Canceled) && runtime.isBreak() {
				ci.setStatus(taskStatusStopped)
				break
			}
			if attempt < maxRetries {
				ci.setRetrying(err)
				if completed := runtime.waitForBreak(copyRetryDelay(attempt)); !completed {
					ci.setStatus(taskStatusStopped)
					break
				}
				continue
			}
			ci.setFailure(err)
			break
		}

		ci.setTaskID(taskID)
		if taskID == "" {
			ci.setProgress(taskStatusSuccess, 100, nil)
		} else if ci.status() != taskStatusStopped {
			runtime.waitForRemoteCopyCompletion(ci)
		}
		if ci.status() == taskStatusFailed && attempt < maxRetries {
			ci.setRetrying(errors.New(ci.errorMessage()))
			if completed := runtime.waitForBreak(copyRetryDelay(attempt)); !completed {
				ci.setStatus(taskStatusStopped)
				break
			}
			continue
		}
		break
	}
	ci.endIt()
}

func (ci *CopyItem) startTransfer(ctx context.Context, client copyItemClient) (string, error) {
	if ci.CopyType == taskItemTypeMove {
		return client.MoveFileContext(ctx, ci.SrcPath, ci.DstPath, ci.FileName)
	}
	return client.CopyFileContext(ctx, ci.SrcPath, ci.DstPath, ci.FileName)
}

func defaultCopyRetryDelay(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	delay := time.Duration(attempt+1) * time.Second
	if delay > 5*time.Second {
		return 5 * time.Second
	}
	return delay
}

func (ci *CopyItem) errorMessage() string {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	if ci.ErrMsg == nil {
		return "copy failed"
	}
	return *ci.ErrMsg
}

// verifyDstExists checks whether the destination file is present. Used when the
// AList task record has vanished (404) to decide whether a move/copy actually
// completed. Runs on an independent cleanup context so it is not cut short by a
// cancelling task context.
func (ci *CopyItem) verifyDstExists(runtime copyItemRuntime, client copyItemClient) (bool, error) {
	ctx, cancel := runtime.cleanupContext()
	defer cancel()
	return client.FileExistsContext(ctx, ci.DstPath, ci.FileName)
}

func (ci *CopyItem) stopRemoteTask(client copyItemClient, cause error) {
	ci.setStatus(taskStatusStopped)
	if cause != nil {
		errMsg := cause.Error()
		ci.setProgress(taskStatusStopped, ci.progress(), &errMsg)
	}
	if taskID := ci.taskID(); taskID != "" {
		ctx, cancel := ci.copyRuntime().cleanupContext()
		if err := client.TaskCancelContext(ctx, taskID, ci.CopyType); err != nil {
			errMsg := err.Error()
			ci.setProgress(taskStatusStopped, ci.progress(), &errMsg)
		}
		_ = client.TaskDeleteContext(ctx, taskID, ci.CopyType)
		cancel()
	}
}

func (ci *CopyItem) endIt() {
	runtime := ci.copyRuntime()
	runtime.finishCopyItem(ci)
}
