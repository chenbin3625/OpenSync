package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"opensync/internal/config"
	"opensync/pkg/util"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// JobTask represents a running task with sync engine
type JobTask struct {
	TaskID      int64
	JobClient   *JobClient
	Job         map[string]interface{}
	AlistClient *AlistClient
	CreateTime  float64
	RunTime     int64

	FinishMu       sync.Mutex
	FinishedCounts map[taskStatus]int
	FinishedSizes  map[taskStatus]int64
	Doing          map[int64]*CopyItem
	DoingMu        sync.Mutex
	Waiting        *copyQueue

	LastWatching atomic.Int64
	// QueueNum is incremented by both the submit executor and the full-sync
	// relocation path, so it has to be atomic: a plain ++ let two items share a
	// DoingKey and silently overwrite each other in Doing.
	QueueNum           atomic.Int64
	ScanFinish         atomic.Bool
	FirstSync          atomic.Int64
	BreakFlag          atomic.Bool
	submitActive       atomic.Bool
	finished           atomic.Bool
	queuedCopyFailures atomic.Int64
	scanSem            chan struct{}
	scanBranchSem      chan struct{}
	copySem            chan struct{}
	scanDone           chan struct{}
	lastMeter          transferMeter
	ctx                context.Context
	cancel             context.CancelFunc
	runtimeMu          sync.Mutex
	copyWG             sync.WaitGroup
	ScanTotalDirs      atomic.Int64
	ScanDoneDirs       atomic.Int64

	deferredDeletesMu sync.Mutex
	deferredDeletes   []fullSyncDelete

	// CurrentMu guards the transfer meter below (speed bookkeeping).
	CurrentMu sync.RWMutex

	RetrySourceTaskID         int64
	RetrySourceAlistID        int64
	RetryStatuses             []taskStatus
	FatalMu                   sync.Mutex
	FatalErr                  *string
	PersistMu                 sync.Mutex
	PersistErr                error
	persistBufMu              sync.Mutex
	persistBuffer             []JobTaskItem
	persistWriteMu            sync.Mutex
	persistFlushMu            sync.Mutex
	persistFlushScheduled     bool
	persistFlushTimer         *time.Timer
	copyMonitor               *copyTaskMonitor
	copyMonitorClientOverride copyItemClient
}

// NewJobTask creates and starts a new task
func NewJobTask(taskID int64, jc *JobClient) *JobTask {
	jt := newJobTask(taskID, jc)
	jt.Start()
	return jt
}

func newJobTask(taskID int64, jc *JobClient) *JobTask {
	return newJobTaskWithConfig(taskID, jc, jc.jobSnapshot())
}

func newJobTaskWithConfig(taskID int64, jc *JobClient, job map[string]interface{}) *JobTask {
	now := time.Now().Unix()
	jt := &JobTask{
		TaskID:         taskID,
		JobClient:      jc,
		Job:            job,
		CreateTime:     float64(now),
		RunTime:        now,
		FinishedCounts: make(map[taskStatus]int),
		FinishedSizes:  make(map[taskStatus]int64),
		Doing:          make(map[int64]*CopyItem),
		Waiting:        newCopyQueue(),
		scanSem:        make(chan struct{}, scanConcurrencyLimit()),
		scanBranchSem:  make(chan struct{}, scanConcurrencyLimit()),
		copySem:        make(chan struct{}, runtimeTaskLimits().CopyConcurrency),
	}
	jt.ctx, jt.cancel = newTaskContext(config.GetConfig().Server.Timeout)
	ok := false
	defer func() {
		if !ok {
			jt.cancelTaskContext()
		}
	}()
	jt.AlistClient = GetClientByIDContext(jt.ctx, util.ToInt64(job["alistId"]))
	ok = true
	return jt
}

func newTaskContext(timeoutHours int) (context.Context, context.CancelFunc) {
	if timeoutHours <= 0 {
		return context.WithCancel(context.Background())
	}
	return context.WithTimeout(context.Background(), time.Duration(timeoutHours)*time.Hour)
}

func (jt *JobTask) Start() {
	jt.runtimeMu.Lock()
	jt.ensureRuntimeLocked()
	scanDone := make(chan struct{})
	jt.scanDone = scanDone
	jt.runtimeMu.Unlock()

	jt.submitActive.Store(true)
	jt.startWorker("scan", jt.sync, func() {
		close(scanDone)
	})
	jt.startWorker("submit", jt.taskSubmit, nil)
}

func (jt *JobTask) startWorker(name string, fn func(), onExit func()) {
	go func() {
		if onExit != nil {
			defer onExit()
		}
		defer func() {
			if r := recover(); r != nil {
				jt.handleWorkerPanic(name, r)
			}
		}()
		fn()
	}()
}

func (jt *JobTask) waitScanDone() {
	jt.runtimeMu.Lock()
	scanDone := jt.scanDone
	jt.runtimeMu.Unlock()
	if scanDone != nil {
		<-scanDone
	}
}

func (jt *JobTask) handleWorkerPanic(name string, recovered interface{}) {
	errMsg := workerPanicMessage(name, recovered)
	log.Printf("Task %d %s", jt.TaskID, errMsg)
	jt.setFatalError(errMsg)
	jt.ScanFinish.Store(true)
	jt.requestBreak()
	if name == "submit" {
		jt.cleanupAfterSubmitPanic(errMsg)
	}
}

func workerPanicMessage(name string, recovered interface{}) string {
	return fmt.Sprintf("%s worker panic: %v", name, recovered)
}

func (jt *JobTask) recoverWorkerPanic(name string, errTarget *error) {
	if r := recover(); r != nil {
		errMsg := workerPanicMessage(name, r)
		if errTarget != nil {
			*errTarget = errors.New(errMsg)
		}
		jt.handleWorkerPanic(name, r)
	}
}

func (jt *JobTask) setFatalError(errMsg string) {
	jt.FatalMu.Lock()
	defer jt.FatalMu.Unlock()
	if jt.FatalErr != nil {
		return
	}
	msg := errMsg
	jt.FatalErr = &msg
}

func (jt *JobTask) fatalError() *string {
	jt.FatalMu.Lock()
	defer jt.FatalMu.Unlock()
	if jt.FatalErr == nil {
		return nil
	}
	msg := *jt.FatalErr
	return &msg
}

func (jt *JobTask) copyMonitorClient() copyItemClient {
	if jt.copyMonitorClientOverride != nil {
		return jt.copyMonitorClientOverride
	}
	return jt.AlistClient
}

func scanConcurrencyLimit() int {
	limit := runtimeTaskLimits().ScanConcurrency
	if limit <= 0 {
		limit = runtime.NumCPU()
	}
	return intInRangeOrDefault(limit, config.MinScanConcurrency, config.MaxScanConcurrency, config.DefaultScanConcurrency)
}

func (jt *JobTask) initRuntime() {
	jt.runtimeMu.Lock()
	defer jt.runtimeMu.Unlock()
	jt.ensureRuntimeLocked()
}

func (jt *JobTask) ensureRuntimeLocked() {
	if jt.Waiting == nil {
		jt.Waiting = newCopyQueue()
	}
	if jt.Doing == nil {
		jt.Doing = make(map[int64]*CopyItem)
	}
	if jt.FinishedCounts == nil {
		jt.FinishedCounts = make(map[taskStatus]int)
	}
	if jt.FinishedSizes == nil {
		jt.FinishedSizes = make(map[taskStatus]int64)
	}
	if jt.scanSem == nil {
		jt.scanSem = make(chan struct{}, scanConcurrencyLimit())
	}
	if jt.scanBranchSem == nil {
		jt.scanBranchSem = make(chan struct{}, scanConcurrencyLimit())
	}
	if jt.copySem == nil {
		limit := runtimeTaskLimits().CopyConcurrency
		if limit < 1 {
			limit = 1
		}
		jt.copySem = make(chan struct{}, limit)
	}
	if jt.ctx == nil || jt.cancel == nil {
		jt.ctx, jt.cancel = newTaskContext(config.GetConfig().Server.Timeout)
	}
	if jt.BreakFlag.Load() && jt.cancel != nil {
		jt.cancel()
	}
}

func (jt *JobTask) context() context.Context {
	jt.runtimeMu.Lock()
	defer jt.runtimeMu.Unlock()
	jt.ensureRuntimeLocked()
	return jt.ctx
}

func (jt *JobTask) cancelTaskContext() {
	jt.runtimeMu.Lock()
	cancel := jt.cancel
	jt.runtimeMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (jt *JobTask) cleanupContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

func (jt *JobTask) isBreak() bool {
	return jt.BreakFlag.Load() || jt.context().Err() != nil
}

func (jt *JobTask) requestBreak() {
	if !jt.BreakFlag.Swap(true) {
		jt.runtimeMu.Lock()
		jt.ensureRuntimeLocked()
		cancel := jt.cancel
		jt.runtimeMu.Unlock()
		if cancel != nil {
			cancel()
		}
	}
}

func (jt *JobTask) waitForBreak(d time.Duration) bool {
	return jt.waitForBreakContext(jt.context(), d)
}

func (jt *JobTask) waitForBreakContext(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		select {
		case <-ctx.Done():
			return false
		default:
			return true
		}
	}

	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func (jt *JobTask) jobConfig() map[string]interface{} {
	return jt.Job
}

func (jt *JobTask) lastWatchingUnix() int64 {
	return jt.LastWatching.Load()
}

func (jt *JobTask) finishCopyItem(item *CopyItem) {
	item.mu.RLock()
	srcPath := item.SrcPath
	dstPath := item.DstPath
	fileName := item.FileName
	fileSize := item.FileSize
	alistTaskID := item.AlistTaskID
	status := item.Status
	errMsg := item.ErrMsg
	copyType := item.CopyType
	createTime := item.CreateTime
	doingKey := item.DoingKey
	isRelocation := item.isRelocation
	item.mu.RUnlock()

	// Any non-success terminal status (failed, remotely canceled/stopped, etc.)
	// means the replacement file did not land and deferred mirror deletes must not run.
	if status != taskStatusSuccess && !isRelocation {
		jt.queuedCopyFailures.Add(1)
	}

	taskItem := NewCopyJobTaskItem(jt.TaskID, srcPath, dstPath, fileName, fileSize,
		alistTaskID, status, errMsg, taskItemFile, copyType, createTime)

	// Atomically move the item from Doing into Finished counters/buffer so
	// progress snapshots never double-count or drop it, while releasing the copy
	// concurrency slot before performing any synchronous DB flush.
	jt.DoingMu.Lock()
	delete(jt.Doing, doingKey)
	needFlush := jt.recordFinishNoFlush([]JobTaskItem{taskItem})
	jt.DoingMu.Unlock()

	item.mu.Lock()
	if item.copySlotHeld {
		item.copySlotHeld = false
		<-jt.copySem
	}
	item.mu.Unlock()

	jt.postRecordFinish(needFlush)
}
