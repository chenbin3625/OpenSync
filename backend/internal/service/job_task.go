package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"opensync/internal/config"
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

	Finish         []JobTaskItem
	FinishMu       sync.Mutex
	pendingPersist []JobTaskItem
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
	ctx           context.Context
	cancel        context.CancelFunc
	copyWG        sync.WaitGroup
	ScanTotalDirs atomic.Int64
	ScanDoneDirs  atomic.Int64

	CurrentTasks map[int][]map[string]interface{}
	CurrentMu    sync.RWMutex
	RetryItems   []map[string]interface{}

	RetrySourceTaskID int64
	RetryStatuses     []taskStatus
	FatalMu           sync.Mutex
	FatalErr          *string
}

// NewJobTask creates and starts a new task
func NewJobTask(taskID int64, jc *JobClient) *JobTask {
	jt := newJobTask(taskID, jc)
	jt.Start()
	return jt
}

func NewRetryJobTask(taskID int64, jc *JobClient, retryItems []map[string]interface{}) *JobTask {
	jt := newJobTask(taskID, jc)
	jt.RetryItems = cloneTaskRows(retryItems)
	jt.Start()
	return jt
}

func newJobTask(taskID int64, jc *JobClient) *JobTask {
	jt := &JobTask{
		TaskID:         taskID,
		JobClient:      jc,
		Job:            jc.Job,
		AlistClient:    GetClientByID(toInt64(jc.Job["alistId"])),
		CreateTime:     float64(time.Now().Unix()),
		Finish:         make([]JobTaskItem, 0),
		pendingPersist: make([]JobTaskItem, 0),
		FinishedCounts: make(map[taskStatus]int),
		FinishedSizes:  make(map[taskStatus]int64),
		Doing:          make(map[int64]*CopyItem),
		Waiting:        newCopyQueue(),
		QueueNum:       0,
		scanSem:        make(chan struct{}, scanConcurrencyLimit()),
		scanBranchSem:  make(chan struct{}, scanConcurrencyLimit()),
		CurrentTasks:   make(map[int][]map[string]interface{}),
	}
	jt.ctx, jt.cancel = newTaskContext(config.GetConfig().Server.Timeout)
	return jt
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

func scanConcurrencyLimit() int {
	limit := runtimeTaskLimits().ScanConcurrency
	if limit <= 0 {
		limit = runtime.NumCPU()
	}
	return intInRangeOrDefault(limit, 1, maxScanConcurrency, defaultScanConcurrency)
}

func (jt *JobTask) initRuntime() {
	if jt.Waiting == nil {
		jt.Waiting = newCopyQueue()
	}
	if jt.Doing == nil {
		jt.Doing = make(map[int64]*CopyItem)
	}
	if jt.Finish == nil {
		jt.Finish = make([]JobTaskItem, 0)
	}
	if jt.pendingPersist == nil {
		jt.pendingPersist = make([]JobTaskItem, 0)
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
		jt.ctx, jt.cancel = context.WithCancel(context.Background())
	}
}

func (jt *JobTask) context() context.Context {
	jt.initRuntime()
	return jt.ctx
}

func (jt *JobTask) cleanupContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

func (jt *JobTask) isBreak() bool {
	return jt.BreakFlag.Load()
}

func (jt *JobTask) requestBreak() {
	jt.initRuntime()
	if !jt.BreakFlag.Swap(true) && jt.cancel != nil {
		jt.cancel()
	}
}

func (jt *JobTask) waitForBreak(d time.Duration) bool {
	jt.initRuntime()
	if d <= 0 {
		select {
		case <-jt.ctx.Done():
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
	case <-jt.ctx.Done():
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
	jt.CopyHook(item.SrcPath, item.DstPath, item.FileName, item.FileSize, item.AlistTaskID,
		item.Status, item.ErrMsg, taskItemFile, item.CopyType, item.CreateTime)
	item.mu.RUnlock()

	jt.DoingMu.Lock()
	delete(jt.Doing, item.DoingKey)
	jt.DoingMu.Unlock()
}
