package service

import (
	"context"
	"errors"
	"fmt"
	"opensync/internal/i18n"
	"strings"
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
}

type copyItemClient interface {
	CopyFileContext(context.Context, string, string, string) (string, error)
	CopyTaskCancelContext(context.Context, string) error
	CopyTaskDeleteContext(context.Context, string) error
	TaskInfoContext(context.Context, string) (map[string]interface{}, error)
	DeleteFileContext(context.Context, string, []string, int) error
}

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
		taskID, err := client.CopyFileContext(runtime.context(), ci.SrcPath, ci.DstPath, ci.FileName)
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
			ci.checkAndGetStatus()
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

func (ci *CopyItem) checkAndGetStatus() {
	runtime := ci.copyRuntime()
	client := ci.copyClient()
	for {
		if runtime.isBreak() {
			ci.setStatus(taskStatusStopped)
			if taskID := ci.taskID(); taskID != "" {
				ctx, cancel := runtime.cleanupContext()
				if err := client.CopyTaskCancelContext(ctx, taskID); err != nil {
					ci.setFailure(err)
				}
				_ = client.CopyTaskDeleteContext(ctx, taskID)
				cancel()
			}
			break
		}

		cuTime := time.Now().Unix()
		var sleepFor time.Duration
		if cuTime-runtime.lastWatchingUnix() < 3 {
			sleepFor = 610 * time.Millisecond
		} else {
			sleepFor = 2930 * time.Millisecond
		}
		if completed := runtime.waitForBreak(sleepFor); !completed {
			continue
		}

		taskInfo, err := client.TaskInfoContext(runtime.context(), ci.taskID())
		if err != nil {
			if errors.Is(err, context.Canceled) && runtime.isBreak() {
				continue
			}
			eMsg := err.Error()
			if strings.Contains(eMsg, "404") {
				eMsg = i18n.G("task_may_delete")
			}
			ci.setProgress(taskStatusFailed, 0, &eMsg)
			break
		}

		state := taskStatusFromValue(taskInfo["state"])
		progress := toFloat64(taskInfo["progress"])
		errStr := ""
		if e, ok := taskInfo["error"]; ok && e != nil {
			errStr = fmt.Sprintf("%v", e)
		}

		ci.mu.RLock()
		unchanged := state == ci.Status && progress == ci.Progress
		ci.mu.RUnlock()
		if unchanged {
			continue
		}
		if errStr != "" {
			ci.setProgress(state, progress, &errStr)
		} else {
			ci.setProgress(state, progress, nil)
		}

		if state == taskStatusSuccess || state == taskStatusStopped || state == taskStatusFailed {
			ctx, cancel := runtime.cleanupContext()
			_ = client.CopyTaskDeleteContext(ctx, ci.taskID())
			cancel()
			break
		}
	}
}

func (ci *CopyItem) endIt() {
	runtime := ci.copyRuntime()
	client := ci.copyClient()
	if ci.CopyType == taskItemTypeMove && ci.status() == taskStatusSuccess {
		scanIntervalS := toInt(runtime.jobConfig()["scanIntervalS"])
		ctx, cancel := runtime.cleanupContext()
		err := client.DeleteFileContext(ctx, ci.SrcPath, []string{ci.FileName}, scanIntervalS)
		cancel()
		if err != nil {
			errMsg := strings.Replace(i18n.G("copy_success_but_delete_fail"), "{}", err.Error(), 1)
			ci.setProgress(taskStatusFailed, ci.progress(), &errMsg)
		}
	}
	runtime.finishCopyItem(ci)
}
