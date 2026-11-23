package service

import (
	"context"
	"errors"
	"math"
	"net/http"
	"opensync/internal/msg"
	"opensync/pkg/util"
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
	GetFileInfoContext(context.Context, string, string) (*FileListEntry, bool, error)
}

// maxTransientPollErrors is the number of consecutive transient TaskInfo polling
// errors (network blips, 5xx, connection limits) tolerated before a copy item is
// declared failed. Each retry rides the loop's existing poll-interval backoff.
const maxTransientPollErrors = 3

type CopyItem struct {
	mu               sync.RWMutex
	SrcPath          string
	DstPath          string
	FileName         string
	FileSize         interface{}
	CopyType         taskItemType
	AlistTaskID      string
	Status           taskStatus
	Progress         float64
	ErrMsg           *string
	CreateTime       int64
	DoingKey         int64
	copySlotHeld     bool
	retryBlocked     bool
	isRelocation     bool
	expectedMD5      string
	preCopyDst       *FileMetadata
	attemptStartTime int64

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

func clampProgress(progress float64) float64 {
	if math.IsNaN(progress) || math.IsInf(progress, 0) || progress < 0 {
		return 0
	}
	if progress > 100 {
		return 100
	}
	return progress
}

func isValidMD5Digest(value string) bool {
	if len(value) != 32 {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
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

func (ci *CopyItem) setRetryBlocked(blocked bool) {
	ci.mu.Lock()
	ci.retryBlocked = blocked
	ci.mu.Unlock()
}

func (ci *CopyItem) isRetryBlocked() bool {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	return ci.retryBlocked
}

func (ci *CopyItem) setAttemptStartTime(ts int64) {
	ci.mu.Lock()
	ci.attemptStartTime = ts
	ci.mu.Unlock()
}

func (ci *CopyItem) setExpectedVerification(md5 string, preCopyDst *FileMetadata) {
	ci.mu.Lock()
	ci.expectedMD5 = strings.TrimSpace(md5)
	if preCopyDst != nil {
		copied := *preCopyDst
		ci.preCopyDst = &copied
	} else {
		ci.preCopyDst = nil
	}
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
	ci.retryBlocked = false
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
	clamped := clampProgress(progress)
	ci.mu.Lock()
	ci.Status = status
	ci.Progress = clamped
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

func (ci *CopyItem) toStreamItem() streamDoingItem {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	return streamDoingItem{
		AlistTaskID: ci.AlistTaskID,
		FileName:    ci.FileName,
		SrcPath:     ci.SrcPath,
		DstPath:     ci.DstPath,
		FileSize:    util.ToInt64(ci.FileSize),
		Type:        ci.CopyType.Int(),
		Status:      ci.Status.Int(),
		Progress:    clampProgress(ci.Progress),
		CreateTime:  ci.CreateTime,
	}
}

func (ci *CopyItem) ToMap(taskID int64) map[string]interface{} {
	ci.mu.RLock()
	defer ci.mu.RUnlock()

	itemMap := NewCopyJobTaskItem(taskID, ci.SrcPath, ci.DstPath, ci.FileName, ci.FileSize,
		ci.AlistTaskID, ci.Status, ci.ErrMsg, taskItemFile, ci.CopyType, ci.CreateTime).ToMap()
	itemMap["progress"] = clampProgress(ci.Progress)
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
		ci.setAttemptStartTime(time.Now().Unix())
		taskID, err := ci.startTransfer(runtime.context(), client)
		if err != nil {
			if runtime.isBreak() {
				ci.setStatus(taskStatusStopped)
				break
			}
			// A transport error after a mutating AList request was sent has an
			// unknown outcome. Do not blindly replay copy/move, which can create
			// duplicate remote work or report a completed move as failed. HTTP/API
			// 5xx responses are explicit server failures and remain retryable.
			if !isRetryableTransferError(err) {
				ci.setFailure(err)
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
		if taskID == "" && !ci.confirmSynchronousCopy(runtime, client) {
			if runtime.isBreak() {
				ci.setStatus(taskStatusStopped)
				break
			}
			// AList accepted the request but returned no task id, and the file
			// could not be verified at the destination. Treat the attempt as failed
			// so the bounded retry loop can resubmit it.
			emptyTaskErr := errors.New(msg.AlistNoCopyTask)
			if attempt < maxRetries {
				ci.setRetrying(emptyTaskErr)
				if completed := runtime.waitForBreak(copyRetryDelay(attempt)); !completed {
					ci.setStatus(taskStatusStopped)
					break
				}
				continue
			}
			ci.setFailure(emptyTaskErr)
			break
		}
		if taskID == "" {
			ci.setProgress(taskStatusSuccess, 100, nil)
		} else if ci.status() != taskStatusStopped {
			runtime.waitForRemoteCopyCompletion(ci)
		}
		if ci.status() == taskStatusFailed && !ci.isRetryBlocked() && attempt < maxRetries {
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

func isRetryableTransferError(err error) bool {
	var statusErr *alistStatusError
	if !errors.As(err, &statusErr) {
		return false
	}
	return statusErr.httpStatus >= http.StatusInternalServerError || statusErr.alistCode >= http.StatusInternalServerError
}

func (ci *CopyItem) startTransfer(ctx context.Context, client copyItemClient) (string, error) {
	if ci.CopyType == taskItemTypeMove {
		return client.MoveFileContext(ctx, ci.SrcPath, ci.DstPath, ci.FileName)
	}
	return client.CopyFileContext(ctx, ci.SrcPath, ci.DstPath, ci.FileName)
}

// confirmSynchronousCopy handles a copy/move response with no task id: some
// backends complete the operation synchronously. The destination is verified so
// "no task" is only treated as success when the file actually arrived with
// matching metadata. Persistent verification errors fail the attempt instead of
// falsely assuming success.
func (ci *CopyItem) confirmSynchronousCopy(runtime copyItemRuntime, client copyItemClient) bool {
	for attempt := 0; attempt < maxTransientPollErrors; attempt++ {
		exists, err := ci.verifyDstExists(runtime, client)
		if err == nil {
			return exists
		}
		if runtime.isBreak() {
			return false
		}
		if attempt < maxTransientPollErrors-1 && !runtime.waitForBreak(copyRetryDelay(attempt)) {
			return false
		}
	}
	return false
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

// verifyDstExists checks whether the destination file is present and matches
// expected size/MD5/freshness metadata. Used when the AList task record has
// vanished (404) or returned synchronously with no task id. Runs on an
// independent cleanup context so it is not cut short by a cancelling task context.
func (ci *CopyItem) verifyDstExists(runtime copyItemRuntime, client copyItemClient) (bool, error) {
	ctx, cancel := runtime.cleanupContext()
	defer cancel()
	entry, exists, err := client.GetFileInfoContext(ctx, ci.DstPath, ci.FileName)
	if err != nil {
		return false, err
	}
	if !exists || entry == nil || entry.IsDir {
		return false, nil
	}

	meta := entry.Metadata()
	expectedSize := util.ToInt64(ci.FileSize)
	if expectedSize > 0 && meta.Size != expectedSize {
		return false, nil
	}

	ci.mu.RLock()
	expectedMD5 := ci.expectedMD5
	var preCopy *FileMetadata
	if ci.preCopyDst != nil {
		copied := *ci.preCopyDst
		preCopy = &copied
	}
	attemptStart := ci.attemptStartTime
	ci.mu.RUnlock()

	md5Verified := false
	if isValidMD5Digest(expectedMD5) && isValidMD5Digest(meta.MD5) {
		if !strings.EqualFold(expectedMD5, meta.MD5) {
			return false, nil
		}
		md5Verified = true
	}

	if preCopy != nil && !md5Verified {
		if preCopy.Modified > 0 && meta.Modified > 0 && meta.Modified < preCopy.Modified {
			return false, nil
		}
		if preCopy.Size == expectedSize {
			// Overwrite with unchanged size and no MD5 proof: require a newer mtime
			// so a pre-existing stale destination file is not mistaken for success.
			if preCopy.Modified > 0 {
				if meta.Modified <= preCopy.Modified {
					return false, nil
				}
			} else if attemptStart > 0 && meta.Modified > 0 && meta.Modified < attemptStart {
				return false, nil
			}
		}
	}

	return true, nil
}

func (ci *CopyItem) stopRemoteTask(client copyItemClient, cause error) {
	ci.setStatus(taskStatusStopped)
	if cause != nil {
		errMsg := cause.Error()
		ci.setProgress(taskStatusStopped, ci.progress(), &errMsg)
	}
	if taskID := ci.taskID(); taskID != "" {
		// Cancel and delete each get their own cleanup context so a slow cancel
		// cannot exhaust the budget and silently skip the delete.
		cancelCtx, cancelCancel := ci.copyRuntime().cleanupContext()
		if err := client.TaskCancelContext(cancelCtx, taskID, ci.CopyType); err != nil {
			errMsg := err.Error()
			ci.setProgress(taskStatusStopped, ci.progress(), &errMsg)
		}
		cancelCancel()
		deleteCtx, cancelDelete := ci.copyRuntime().cleanupContext()
		_ = client.TaskDeleteContext(deleteCtx, taskID, ci.CopyType)
		cancelDelete()
	}
}

func (ci *CopyItem) endIt() {
	runtime := ci.copyRuntime()
	runtime.finishCopyItem(ci)
}
