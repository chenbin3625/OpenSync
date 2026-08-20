package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"opensync/internal/config"
	"opensync/pkg/util"
	"runtime"
	"runtime/debug"
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

	FinishMu       sync.Mutex
	FinishedCounts map[taskStatus]int
	FinishedSizes  map[taskStatus]int64
	Doing          map[int64]*CopyItem
	DoingMu        sync.Mutex
	Waiting        *copyQueue

	LastWatching  atomic.Int64
	QueueNum      int64
	ScanFinish    atomic.Bool
	FirstSync     atomic.Int64
	BreakFlag     atomic.Bool
	scanSem       chan struct{}
	scanBranchSem chan struct{}
	taskLimits    taskRuntimeLimits
	limitsSet     bool
	ctx           context.Context
	cancel        context.CancelFunc
	runtimeMu     sync.Mutex
	copyWG        sync.WaitGroup
	ScanTotalDirs atomic.Int64
	ScanDoneDirs  atomic.Int64

	CurrentTasks map[int][]map[string]interface{}
	CurrentMu    sync.RWMutex

	RetrySourceTaskID         int64
	RetryStatuses             []taskStatus
	FatalMu                   sync.Mutex
	FatalErr                  *string
	PersistMu                 sync.Mutex
	PersistErr                error
	persistBufMu              sync.Mutex
	persistBuffer             []JobTaskItem
	persistFlushMu            sync.Mutex
	persistFlushScheduled     bool
	persistFlushWG            sync.WaitGroup
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
	job := jc.jobSnapshot()
	limits := runtimeTaskLimits()
	jt := &JobTask{
		TaskID:         taskID,
		JobClient:      jc,
		Job:            job,
		CreateTime:     float64(time.Now().Unix()),
		FinishedCounts: make(map[taskStatus]int),
		FinishedSizes:  make(map[taskStatus]int64),
		Doing:          make(map[int64]*CopyItem),
		Waiting:        newCopyQueue(),
		QueueNum:       0,
		scanSem:        make(chan struct{}, limits.ScanConcurrency),
		scanBranchSem:  make(chan struct{}, limits.ScanConcurrency),
		taskLimits:     limits,
		limitsSet:      true,
		CurrentTasks:   make(map[int][]map[string]interface{}),
	}
	jt.ctx, jt.cancel = newTaskContext(config.GetConfig().Server.Timeout)
	jt.AlistClient = GetClientByIDContext(jt.ctx, util.ToInt64(job["alistId"]))
	return jt
}

func (jt *JobTask) maxRetries() int {
	if jt.limitsSet {
		return jt.taskLimits.MaxRetries
	}
	return runtimeTaskLimits().MaxRetries
}

func (jt *JobTask) copyConcurrencyLimit() int {
	if jt.limitsSet {
		return jt.taskLimits.CopyConcurrency
	}
	return runtimeTaskLimits().CopyConcurrency
}

func newTaskContext(timeoutHours int) (context.Context, context.CancelFunc) {
	if timeoutHours <= 0 {
		return context.WithCancel(context.Background())
	}
	return context.WithTimeout(context.Background(), time.Duration(timeoutHours)*time.Hour)
}

func (jt *JobTask) Start() {
	jt.startWorker("scan", jt.sync)
	jt.startWorker("submit", jt.taskSubmit)
}

func (jt *JobTask) startWorker(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				jt.handleWorkerPanic(name, r)
			}
		}()
		fn()
	}()
}

func (jt *JobTask) handleWorkerPanic(name string, recovered interface{}) {
	errMsg := workerPanicMessage(name, recovered)
	log.Printf("Task %d %s", jt.TaskID, errMsg)
	jt.setFatalError(errMsg)
	jt.ScanFinish.Store(true)
	jt.requestBreak()
	if name == "submit" {
		jt.finishFailedTask(errMsg)
	}
}

func workerPanicMessage(name string, recovered interface{}) string {
	const maxStackCapture = 4096
	stack := debug.Stack()
	if len(stack) > maxStackCapture {
		stack = stack[:maxStackCapture]
	}
	return fmt.Sprintf("%s worker panic: %v\n%s", name, recovered, stack)
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
	if jt.CurrentTasks == nil {
		jt.CurrentTasks = make(map[int][]map[string]interface{})
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

func (jt *JobTask) cleanupContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

func (jt *JobTask) isBreak() bool {
	return jt.BreakFlag.Load()
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
	ctx := jt.context()
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
	item.mu.RUnlock()

	jt.CopyHook(srcPath, dstPath, fileName, fileSize, alistTaskID,
		status, errMsg, taskItemFile, copyType, createTime)

	jt.DoingMu.Lock()
	delete(jt.Doing, item.DoingKey)
	jt.DoingMu.Unlock()
}
