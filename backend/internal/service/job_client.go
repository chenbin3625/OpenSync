package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"opensync/internal/config"
	"opensync/internal/i18n"
	"opensync/internal/mapper"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	ignore "github.com/sabhiram/go-gitignore"
)

const (
	maxScanConcurrency       = 8
	maxCopyConcurrency       = 20
	maxQueuedCopyItems       = 5000
	maxRealtimeFinishedItems = 2000
	maxPersistTaskItemBatch  = 500
)

var errScanAborted = errors.New("scan aborted")

var persistJobTaskItems = mapper.AddJobTaskItemMany

type copyQueue struct {
	mu       sync.Mutex
	items    []*CopyItem
	head     int
	closed   bool
	capacity int
	notify   chan struct{}
	space    chan struct{}
}

func newCopyQueue() *copyQueue {
	return newCopyQueueWithCapacity(maxQueuedCopyItems)
}

func newCopyQueueWithCapacity(capacity int) *copyQueue {
	return &copyQueue{
		items:    make([]*CopyItem, 0),
		capacity: capacity,
		notify:   make(chan struct{}, 1),
		space:    make(chan struct{}, 1),
	}
}

func (q *copyQueue) push(item *CopyItem) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed || !q.hasCapacityLocked() {
		return false
	}
	q.items = append(q.items, item)
	q.signal()
	return true
}

func (q *copyQueue) pushWait(ctx context.Context, item *CopyItem) bool {
	for {
		q.mu.Lock()
		if q.closed {
			q.mu.Unlock()
			return false
		}
		if q.hasCapacityLocked() {
			q.items = append(q.items, item)
			q.signal()
			q.mu.Unlock()
			return true
		}
		space := q.space
		q.mu.Unlock()

		select {
		case <-ctx.Done():
			return false
		case <-space:
		}
	}
}

func (q *copyQueue) pop() (*CopyItem, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.head >= len(q.items) {
		q.compactLocked()
		return nil, false
	}

	item := q.items[q.head]
	q.items[q.head] = nil
	q.head++
	q.compactLocked()
	q.signalSpace()
	return item, true
}

func (q *copyQueue) len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items) - q.head
}

func (q *copyQueue) snapshot() []*CopyItem {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]*CopyItem(nil), q.items[q.head:]...)
}

func (q *copyQueue) closeAndDrain() []*CopyItem {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.closed = true
	items := append([]*CopyItem(nil), q.items[q.head:]...)
	for i := q.head; i < len(q.items); i++ {
		q.items[i] = nil
	}
	q.items = q.items[:0]
	q.head = 0
	q.signal()
	q.signalSpace()
	return items
}

func (q *copyQueue) waitCh() <-chan struct{} {
	return q.notify
}

func (q *copyQueue) compactLocked() {
	if q.head == 0 {
		return
	}
	if q.head == len(q.items) {
		q.items = q.items[:0]
		q.head = 0
		return
	}
	if q.head > len(q.items)/2 {
		q.items = append([]*CopyItem(nil), q.items[q.head:]...)
		q.head = 0
	}
}

func (q *copyQueue) hasCapacityLocked() bool {
	return q.capacity <= 0 || len(q.items)-q.head < q.capacity
}

func (q *copyQueue) signal() {
	select {
	case q.notify <- struct{}{}:
	default:
	}
}

func (q *copyQueue) signalSpace() {
	select {
	case q.space <- struct{}{}:
	default:
	}
}

// CopyItem represents a single file copy operation
type CopyItem struct {
	mu          sync.RWMutex
	SrcPath     string
	DstPath     string
	FileName    string
	FileSize    interface{}
	CopyType    int // 0=copy, 2=move
	AlistTaskID string
	Status      int
	Progress    float64
	ErrMsg      *string
	CreateTime  int64
	DoingKey    int64

	jobTask     *JobTask
	alistClient *AlistClient
}

func (ci *CopyItem) setStatus(status int) {
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
	ci.Status = 7
	ci.Progress = 0
	ci.ErrMsg = &errMsg
	ci.mu.Unlock()
}

func (ci *CopyItem) setProgress(status int, progress float64, errMsg *string) {
	ci.mu.Lock()
	ci.Status = status
	ci.Progress = progress
	ci.ErrMsg = errMsg
	ci.mu.Unlock()
}

func (ci *CopyItem) status() int {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	return ci.Status
}

func (ci *CopyItem) taskID() string {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	return ci.AlistTaskID
}

func (ci *CopyItem) snapshotMap() map[string]interface{} {
	ci.mu.RLock()
	defer ci.mu.RUnlock()

	return map[string]interface{}{
		"srcPath":    ci.SrcPath,
		"dstPath":    ci.DstPath,
		"isPath":     0,
		"fileName":   ci.FileName,
		"fileSize":   ci.FileSize,
		"status":     ci.Status,
		"type":       ci.CopyType,
		"progress":   ci.Progress,
		"errMsg":     ci.ErrMsg,
		"createTime": ci.CreateTime,
	}
}

// DoIt executes the copy operation in a goroutine
func (ci *CopyItem) DoIt() {
	if ci.jobTask.isBreak() {
		ci.setStatus(4)
	} else {
		ci.setStatus(1)
		taskID, err := ci.alistClient.CopyFileContext(ci.jobTask.context(), ci.SrcPath, ci.DstPath, ci.FileName)
		if err != nil {
			if errors.Is(err, context.Canceled) && ci.jobTask.isBreak() {
				ci.setStatus(4)
			} else {
				ci.setFailure(err)
			}
		} else {
			ci.setTaskID(taskID)
			if taskID == "" {
				ci.setStatus(2)
			} else if ci.status() != 4 {
				ci.checkAndGetStatus()
			}
		}
	}
	ci.endIt()
}

func (ci *CopyItem) checkAndGetStatus() {
	for {
		if ci.jobTask.isBreak() {
			ci.setStatus(4)
			if taskID := ci.taskID(); taskID != "" {
				ctx, cancel := ci.jobTask.cleanupContext()
				if err := ci.alistClient.CopyTaskCancelContext(ctx, taskID); err != nil {
					ci.setFailure(err)
				}
				_ = ci.alistClient.CopyTaskDeleteContext(ctx, taskID)
				cancel()
			}
			break
		}

		cuTime := time.Now().Unix()
		var sleepFor time.Duration
		if cuTime-ci.jobTask.LastWatching.Load() < 3 {
			sleepFor = 610 * time.Millisecond
		} else {
			sleepFor = 2930 * time.Millisecond
		}
		if completed := ci.jobTask.waitForBreak(sleepFor); !completed {
			continue
		}

		taskInfo, err := ci.alistClient.TaskInfoContext(ci.jobTask.context(), ci.taskID())
		if err != nil {
			if errors.Is(err, context.Canceled) && ci.jobTask.isBreak() {
				continue
			}
			eMsg := err.Error()
			if strings.Contains(eMsg, "404") {
				eMsg = i18n.G("task_may_delete")
			}
			ci.setProgress(7, 0, &eMsg)
			break
		}

		state := toInt(taskInfo["state"])
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

		if state == 2 || state == 4 || state == 7 {
			ctx, cancel := ci.jobTask.cleanupContext()
			_ = ci.alistClient.CopyTaskDeleteContext(ctx, ci.taskID())
			cancel()
			break
		}
	}
}

func (ci *CopyItem) endIt() {
	if ci.CopyType == 2 && ci.status() == 2 {
		scanIntervalS := toInt(ci.jobTask.Job["scanIntervalS"])
		ctx, cancel := ci.jobTask.cleanupContext()
		err := ci.alistClient.DeleteFileContext(ctx, ci.SrcPath, []string{ci.FileName}, scanIntervalS)
		cancel()
		if err != nil {
			errMsg := strings.Replace(i18n.G("copy_success_but_delete_fail"), "{}", err.Error(), 1)
			ci.setProgress(7, ci.snapshotMap()["progress"].(float64), &errMsg)
		}
	}
	ci.mu.RLock()
	ci.jobTask.CopyHook(ci.SrcPath, ci.DstPath, ci.FileName, ci.FileSize, ci.AlistTaskID,
		ci.Status, ci.ErrMsg, 0, ci.CopyType, ci.CreateTime)
	ci.mu.RUnlock()
	ci.jobTask.DoingMu.Lock()
	delete(ci.jobTask.Doing, ci.DoingKey)
	ci.jobTask.DoingMu.Unlock()
}

// JobTask represents a running task with sync engine
type JobTask struct {
	TaskID      int64
	JobClient   *JobClient
	Job         map[string]interface{}
	AlistClient *AlistClient
	CreateTime  float64

	Finish         []map[string]interface{}
	FinishMu       sync.Mutex
	pendingPersist []map[string]interface{}
	FinishedCounts map[int]int
	FinishedSizes  map[int]int64
	Doing          map[int64]*CopyItem
	DoingMu        sync.Mutex
	Waiting        *copyQueue

	LastWatching atomic.Int64
	QueueNum     int64
	ScanFinish   atomic.Bool
	FirstSync    atomic.Int64
	BreakFlag    atomic.Bool
	scanSem      chan struct{}
	ctx          context.Context
	cancel       context.CancelFunc
	copyWG       sync.WaitGroup

	CurrentTasks map[int][]map[string]interface{}
	CurrentMu    sync.RWMutex
}

// NewJobTask creates and starts a new task
func NewJobTask(taskID int64, jc *JobClient) *JobTask {
	jt := newJobTask(taskID, jc)
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
		Finish:         make([]map[string]interface{}, 0),
		pendingPersist: make([]map[string]interface{}, 0),
		FinishedCounts: make(map[int]int),
		FinishedSizes:  make(map[int]int64),
		Doing:          make(map[int64]*CopyItem),
		Waiting:        newCopyQueue(),
		QueueNum:       0,
		scanSem:        make(chan struct{}, scanConcurrencyLimit()),
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
	go jt.sync()
	go jt.taskSubmit()
}

func scanConcurrencyLimit() int {
	limit := runtime.NumCPU()
	if limit < 2 {
		return 2
	}
	if limit > maxScanConcurrency {
		return maxScanConcurrency
	}
	return limit
}

func (jt *JobTask) initRuntime() {
	if jt.Waiting == nil {
		jt.Waiting = newCopyQueue()
	}
	if jt.Doing == nil {
		jt.Doing = make(map[int64]*CopyItem)
	}
	if jt.Finish == nil {
		jt.Finish = make([]map[string]interface{}, 0)
	}
	if jt.pendingPersist == nil {
		jt.pendingPersist = make([]map[string]interface{}, 0)
	}
	if jt.FinishedCounts == nil {
		jt.FinishedCounts = make(map[int]int)
	}
	if jt.FinishedSizes == nil {
		jt.FinishedSizes = make(map[int]int64)
	}
	if jt.scanSem == nil {
		jt.scanSem = make(chan struct{}, scanConcurrencyLimit())
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

func (jt *JobTask) acquireScanSlot() bool {
	for {
		if jt.isBreak() {
			return false
		}
		select {
		case jt.scanSem <- struct{}{}:
			if jt.isBreak() {
				<-jt.scanSem
				return false
			}
			return true
		default:
			if completed := jt.waitForBreak(50 * time.Millisecond); !completed {
				return false
			}
		}
	}
}

func (jt *JobTask) releaseScanSlot() {
	<-jt.scanSem
}

// GetCurrent returns real-time task progress
func (jt *JobTask) GetCurrent() map[string]interface{} {
	now := time.Now().Unix()
	jt.LastWatching.Store(now)

	waitingItems := jt.Waiting.snapshot()
	waits := make([]map[string]interface{}, len(waitingItems))
	for i, w := range waitingItems {
		waits[i] = jt.copyItemToMap(w)
	}

	jt.DoingMu.Lock()
	dos := make([]map[string]interface{}, 0, len(jt.Doing))
	for _, d := range jt.Doing {
		dos = append(dos, jt.copyItemToMap(d))
	}
	jt.DoingMu.Unlock()

	allTask := make([]map[string]interface{}, 0)
	allTask = append(allTask, waits...)
	allTask = append(allTask, dos...)
	jt.FinishMu.Lock()
	allTask = append(allTask, jt.Finish...)
	jt.FinishMu.Unlock()

	keyValSpace := map[string]int{
		"wait":    0,
		"running": 1,
		"success": 2,
		"fail":    7,
		"other":   -1,
	}

	currentTasks := make(map[int][]map[string]interface{})
	for _, v := range keyValSpace {
		currentTasks[v] = make([]map[string]interface{}, 0)
	}

	otkStatus := map[int]bool{3: true, 4: true, 5: true, 6: true, 8: true, 9: true}
	otk := make([]map[string]interface{}, 0)
	grouped := make(map[int][]map[string]interface{})
	for _, task := range allTask {
		s := task["status"].(int)
		grouped[s] = append(grouped[s], task)
	}
	for status, tasks := range grouped {
		if otkStatus[status] {
			otk = append(otk, tasks...)
		} else {
			currentTasks[status] = tasks
		}
	}
	currentTasks[-1] = otk
	jt.CurrentMu.Lock()
	jt.CurrentTasks = currentTasks
	jt.CurrentMu.Unlock()

	result := map[string]interface{}{
		"scanFinish": jt.ScanFinish.Load(),
		"doingTask":  currentTasks[1],
		"createTime": int(jt.CreateTime),
		"duration":   int(float64(now) - jt.CreateTime),
		"firstSync":  nil,
		"num":        map[string]int{},
		"size":       map[string]int64{},
	}
	if firstSync := jt.FirstSync.Load(); firstSync > 0 {
		result["firstSync"] = int(firstSync)
	}

	numMap := result["num"].(map[string]int)
	sizeMap := result["size"].(map[string]int64)
	for key, val := range keyValSpace {
		tasks := currentTasks[val]
		if val == 0 || val == 1 {
			numMap[key] = len(tasks)
			var totalSize int64
			for _, t := range tasks {
				if t["fileSize"] != nil && t["type"].(int) != 1 {
					totalSize += toInt64Val(t["fileSize"])
				}
			}
			sizeMap[key] = totalSize
			continue
		}
		count, size := jt.finishedAggregateForStatus(val)
		numMap[key] = count
		sizeMap[key] = size
	}
	return result
}

// GetCurrentByStatus returns tasks filtered by status
func (jt *JobTask) GetCurrentByStatus(status int) []map[string]interface{} {
	jt.CurrentMu.RLock()
	defer jt.CurrentMu.RUnlock()
	if tasks, ok := jt.CurrentTasks[status]; ok {
		return append([]map[string]interface{}(nil), tasks...)
	}
	return []map[string]interface{}{}
}

func (jt *JobTask) currentTasksSnapshot() map[int][]map[string]interface{} {
	jt.CurrentMu.RLock()
	defer jt.CurrentMu.RUnlock()

	snapshot := make(map[int][]map[string]interface{}, len(jt.CurrentTasks))
	for status, tasks := range jt.CurrentTasks {
		snapshot[status] = append([]map[string]interface{}(nil), tasks...)
	}
	return snapshot
}

func (jt *JobTask) copyItemToMap(item *CopyItem) map[string]interface{} {
	return item.snapshotMap()
}

func (jt *JobTask) taskSubmit() {
	jt.initRuntime()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		if jt.isBreak() {
			jt.markWaitingAsAborted()
			break
		}

		started := false
		for jt.doingLen() < maxCopyConcurrency {
			if jt.isBreak() {
				jt.markWaitingAsAborted()
				break
			}

			item, ok := jt.Waiting.pop()
			if !ok {
				break
			}
			jt.startCopyItem(item)
			started = true
		}

		if jt.ScanFinish.Load() && jt.doingLen() == 0 && jt.Waiting.len() == 0 {
			break
		}

		if started {
			continue
		}

		select {
		case <-jt.context().Done():
			jt.markWaitingAsAborted()
		case <-jt.Waiting.waitCh():
		case <-ticker.C:
		}
	}

	if jt.isBreak() {
		jt.markWaitingAsAborted()
	}
	jt.copyWG.Wait()

	if err := jt.flushPendingTaskItems(); err != nil {
		log.Printf("Failed to save pending task items for task %d: %v", jt.TaskID, err)
	}

	// Batch save finish items
	jt.FinishMu.Lock()
	finish := append([]map[string]interface{}(nil), jt.Finish...)
	jt.Finish = jt.Finish[:0]
	jt.FinishMu.Unlock()
	if len(finish) > 0 {
		if err := persistJobTaskItems(finish); err != nil {
			log.Printf("Failed to save task items for task %d: %v", jt.TaskID, err)
		}
	}

	jt.updateTaskStatus()
	jt.JobClient.markDone()
	jt.JobClient.clearCurrentTask(jt)
}

func (jt *JobTask) doingLen() int {
	jt.DoingMu.Lock()
	defer jt.DoingMu.Unlock()
	return len(jt.Doing)
}

func (jt *JobTask) startCopyItem(item *CopyItem) {
	if jt.FirstSync.Load() == 0 {
		jt.FirstSync.CompareAndSwap(0, time.Now().Unix())
	}
	jt.QueueNum++
	item.DoingKey = jt.QueueNum

	jt.DoingMu.Lock()
	jt.Doing[jt.QueueNum] = item
	jt.DoingMu.Unlock()

	jt.copyWG.Add(1)
	go func() {
		defer jt.copyWG.Done()
		item.DoIt()
	}()
}

func (jt *JobTask) markWaitingAsAborted() {
	for _, item := range jt.Waiting.closeAndDrain() {
		item.setStatus(4)
		item.mu.RLock()
		jt.CopyHook(item.SrcPath, item.DstPath, item.FileName, item.FileSize, item.AlistTaskID,
			4, item.ErrMsg, 0, item.CopyType, item.CreateTime)
		item.mu.RUnlock()
	}
}

// CopyHook is called when a copy operation completes
func (jt *JobTask) CopyHook(srcPath, dstPath, fileName string, fileSize interface{},
	alistTaskID string, status int, errMsg *string, isPath, copyType int, createTime int64) {
	jt.appendFinish(map[string]interface{}{
		"taskId":      jt.TaskID,
		"srcPath":     srcPath,
		"dstPath":     dstPath,
		"isPath":      isPath,
		"fileName":    fileName,
		"fileSize":    fileSize,
		"type":        copyType,
		"alistTaskId": alistTaskID,
		"status":      status,
		"errMsg":      errMsg,
		"createTime":  createTime,
	})
}

// DelHook is called when a delete operation completes
func (jt *JobTask) DelHook(dstPath, fileName string, fileSize interface{}, status int, errMsg *string, isPath int, createTime int64) {
	jt.appendFinish(map[string]interface{}{
		"taskId":      jt.TaskID,
		"srcPath":     nil,
		"dstPath":     dstPath,
		"isPath":      isPath,
		"fileName":    fileName,
		"fileSize":    fileSize,
		"type":        1,
		"alistTaskId": nil,
		"status":      status,
		"errMsg":      errMsg,
		"createTime":  createTime,
	})
}

func (jt *JobTask) appendFinish(item map[string]interface{}) {
	jt.initRuntime()
	var flush []map[string]interface{}
	jt.FinishMu.Lock()
	status := toInt(item["status"])
	jt.FinishedCounts[status]++
	if item["fileSize"] != nil && toInt(item["type"]) != 1 {
		jt.FinishedSizes[status] += toInt64Val(item["fileSize"])
	}
	jt.Finish = append(jt.Finish, item)
	if overflow := len(jt.Finish) - maxRealtimeFinishedItems; overflow > 0 {
		jt.pendingPersist = append(jt.pendingPersist, jt.Finish[:overflow]...)
		jt.Finish = append([]map[string]interface{}(nil), jt.Finish[overflow:]...)
	}
	if len(jt.pendingPersist) >= maxPersistTaskItemBatch {
		flush = append([]map[string]interface{}(nil), jt.pendingPersist...)
		jt.pendingPersist = jt.pendingPersist[:0]
	}
	jt.FinishMu.Unlock()

	if len(flush) > 0 {
		if err := persistJobTaskItems(flush); err != nil {
			log.Printf("Failed to flush task items for task %d: %v", jt.TaskID, err)
		}
	}
}

func (jt *JobTask) flushPendingTaskItems() error {
	jt.initRuntime()
	jt.FinishMu.Lock()
	pending := append([]map[string]interface{}(nil), jt.pendingPersist...)
	jt.pendingPersist = jt.pendingPersist[:0]
	jt.FinishMu.Unlock()
	if len(pending) == 0 {
		return nil
	}
	return persistJobTaskItems(pending)
}

func (jt *JobTask) finishedAggregateForStatus(status int) (int, int64) {
	jt.FinishMu.Lock()
	defer jt.FinishMu.Unlock()
	if status != -1 {
		return jt.FinishedCounts[status], jt.FinishedSizes[status]
	}
	count := 0
	size := int64(0)
	for s, c := range jt.FinishedCounts {
		if s != 0 && s != 1 && s != 2 && s != 7 {
			count += c
			size += jt.FinishedSizes[s]
		}
	}
	return count, size
}

func (jt *JobTask) sync() {
	srcPath := normalizeDirPath(fmt.Sprintf("%v", jt.Job["srcPath"]))
	jobExclude := jt.Job["exclude"]

	var spec *ignore.GitIgnore
	if jobExclude != nil {
		excludeStr := fmt.Sprintf("%v", jobExclude)
		if excludeStr != "" {
			patterns := strings.Split(excludeStr, ":")
			spec = ignore.CompileIgnoreLines(patterns...)
		}
	}

	dstPaths := strings.Split(fmt.Sprintf("%v", jt.Job["dstPath"]), ":")
	for i, dstItem := range dstPaths {
		dstItem = normalizeDirPath(dstItem)
		jt.syncWithHave(srcPath, dstItem, spec, srcPath, dstItem, i == 0)
	}
	jt.ScanFinish.Store(true)
}

func normalizeDirPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	return path
}

func (jt *JobTask) copyFile(srcPath, dstPath, fileName string, fileSize interface{}) {
	if jt.isBreak() {
		return
	}
	method := toInt(jt.Job["method"])
	copyType := 0
	if method >= 2 {
		copyType = 2
	}
	ci := &CopyItem{
		SrcPath:     srcPath,
		DstPath:     dstPath,
		FileName:    fileName,
		FileSize:    fileSize,
		CopyType:    copyType,
		Status:      0,
		Progress:    0,
		CreateTime:  time.Now().Unix(),
		jobTask:     jt,
		alistClient: jt.AlistClient,
	}
	if !jt.Waiting.pushWait(jt.context(), ci) {
		ci.setStatus(4)
		jt.CopyHook(ci.SrcPath, ci.DstPath, ci.FileName, ci.FileSize, ci.AlistTaskID,
			ci.Status, ci.ErrMsg, 0, ci.CopyType, ci.CreateTime)
	}
}

func (jt *JobTask) delFile(path, fileName string, size interface{}) {
	if jt.isBreak() {
		return
	}
	isPath := strings.HasSuffix(fileName, "/")
	status := 2
	var errMsg *string
	createTime := time.Now().Unix()

	name := fileName
	if isPath {
		name = fileName[:len(fileName)-1]
	}
	scanIntervalT := toInt(jt.Job["scanIntervalT"])
	err := jt.AlistClient.DeleteFileContext(jt.context(), path, []string{name}, scanIntervalT)
	if err != nil {
		status = 7
		e := err.Error()
		errMsg = &e
	}

	var delSize interface{}
	if !isPath {
		delSize = size
	}
	jt.DelHook(path, fileName, delSize, status, errMsg, boolToInt(isPath), createTime)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (jt *JobTask) listDir(path string, firstDst bool, spec *ignore.GitIgnore, rootPath string, isSrc bool) (map[string]interface{}, error) {
	var useCache int
	if isSrc && !firstDst {
		useCache = 1
	} else {
		if isSrc {
			useCache = toInt(jt.Job["useCacheS"])
		} else {
			useCache = toInt(jt.Job["useCacheT"])
		}
	}

	var scanInterval int
	if isSrc {
		scanInterval = toInt(jt.Job["scanIntervalS"])
	} else {
		scanInterval = toInt(jt.Job["scanIntervalT"])
	}

	if !jt.acquireScanSlot() {
		return nil, errScanAborted
	}
	defer jt.releaseScanSlot()

	result, err := jt.AlistClient.FileListApiContext(jt.context(), path, useCache, scanInterval)
	if err != nil {
		if jt.isBreak() && errors.Is(err, context.Canceled) {
			return nil, err
		}
		srcOrDst := i18n.G("src")
		if !isSrc {
			srcOrDst = i18n.G("dst")
		}
		errMsg := strings.Replace(i18n.G("scan_error"), "{}", srcOrDst, 1)
		errMsg = strings.Replace(errMsg, "{}", err.Error(), 1)
		log.Printf("%s", errMsg)

		jt.CopyHook(pathIfTrue(isSrc, path), pathIfTrue(!isSrc, path), "", nil, "", 7, &errMsg, 1, 0, time.Now().Unix())
		return nil, err
	}

	// Apply exclude rules
	if spec != nil && len(result) > 0 {
		filtered := make(map[string]interface{})
		for key, val := range result {
			checkPath := path[len(rootPath):] + "/" + key
			if !spec.MatchesPath(checkPath) {
				filtered[key] = val
			}
		}
		return filtered, nil
	}

	return result, nil
}

func pathIfTrue(cond bool, path string) string {
	if cond {
		return path
	}
	return ""
}

func (jt *JobTask) listSrcAndDst(srcPath, dstPath string, spec *ignore.GitIgnore, srcRootPath, dstRootPath string, firstDst bool) (map[string]interface{}, map[string]interface{}, error) {
	var srcFiles, dstFiles map[string]interface{}
	var srcErr, dstErr error

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		srcFiles, srcErr = jt.listDir(srcPath, firstDst, spec, srcRootPath, true)
	}()
	go func() {
		defer wg.Done()
		dstFiles, dstErr = jt.listDir(dstPath, firstDst, spec, dstRootPath, false)
	}()
	wg.Wait()

	if srcErr != nil {
		return nil, nil, srcErr
	}
	if dstErr != nil {
		return nil, nil, dstErr
	}
	if srcFiles == nil {
		srcFiles = make(map[string]interface{})
	}
	if dstFiles == nil {
		dstFiles = make(map[string]interface{})
	}
	return srcFiles, dstFiles, nil
}

func (jt *JobTask) syncWithHave(srcPath, dstPath string, spec *ignore.GitIgnore, srcRootPath, dstRootPath string, firstDst bool) {
	if jt.isBreak() {
		return
	}

	srcFiles, dstFiles, err := jt.listSrcAndDst(srcPath, dstPath, spec, srcRootPath, dstRootPath, firstDst)
	if err != nil {
		return
	}

	for key, srcVal := range srcFiles {
		if jt.isBreak() {
			return
		}
		if !strings.HasSuffix(key, "/") {
			// File
			dstVal, exists := dstFiles[key]
			if !exists || fileChanged(srcVal, dstVal) {
				jt.copyFile(srcPath, dstPath, key, fileSize(srcVal))
			}
		} else {
			// Directory
			if _, exists := dstFiles[key]; !exists {
				jt.syncWithoutHave(srcPath+key, dstPath+key, spec, srcRootPath, dstRootPath, firstDst)
			} else {
				jt.syncWithHave(srcPath+key, dstPath+key, spec, srcRootPath, dstRootPath, firstDst)
			}
		}
	}

	if toInt(jt.Job["method"]) == 1 {
		for dstKey, dstVal := range dstFiles {
			if _, exists := srcFiles[dstKey]; !exists {
				jt.delFile(dstPath, dstKey, fileSize(dstVal))
			}
		}
	}
}

func (jt *JobTask) syncWithoutHave(srcPath, dstPath string, spec *ignore.GitIgnore, srcRootPath, dstRootPath string, firstDst bool) {
	if jt.isBreak() {
		return
	}

	status := 2
	var errMsg *string
	scanIntervalT := toInt(jt.Job["scanIntervalT"])
	err := jt.AlistClient.MkdirContext(jt.context(), dstPath, scanIntervalT)
	if err != nil {
		status = 7
		e := err.Error()
		errMsg = &e
	}

	jt.CopyHook(srcPath, dstPath, "", nil, "", status, errMsg, 1, 0, time.Now().Unix())
	if status != 2 {
		return
	}

	srcFiles, err := jt.listDir(srcPath, firstDst, spec, srcRootPath, true)
	if err != nil {
		return
	}

	for key := range srcFiles {
		if jt.isBreak() {
			break
		}
		if strings.HasSuffix(key, "/") {
			jt.syncWithoutHave(srcPath+key, dstPath+key, spec, srcRootPath, dstRootPath, firstDst)
		} else {
			jt.copyFile(srcPath, dstPath, key, fileSize(srcFiles[key]))
		}
	}
}

func fileChanged(srcVal, dstVal interface{}) bool {
	src := toFileMetadata(srcVal)
	dst := toFileMetadata(dstVal)
	if src.MD5 != "" && dst.MD5 != "" {
		return src.MD5 != dst.MD5
	}
	return src.Size != dst.Size
}

func fileSize(val interface{}) int64 {
	return toFileMetadata(val).Size
}

func toFileMetadata(val interface{}) FileMetadata {
	switch v := val.(type) {
	case FileMetadata:
		v.MD5 = normalizeMD5(v.MD5)
		return v
	case *FileMetadata:
		if v == nil {
			return FileMetadata{}
		}
		metadata := *v
		metadata.MD5 = normalizeMD5(metadata.MD5)
		return metadata
	default:
		return FileMetadata{Size: toInt64Val(val)}
	}
}

func (jt *JobTask) updateTaskStatus() {
	jt.GetCurrent()
	taskNum := GetCuTaskNum(jt.TaskID)
	failOrOtherNum := toInt(taskNum["failNum"]) + toInt(taskNum["otherNum"])
	status := finalTaskStatus(jt.isBreak(), jt.context().Err(), failOrOtherNum)

	UpdateJobTaskStatusFinal(jt.TaskID, status, nil, jt.CreateTime)
}

func finalTaskStatus(isBreak bool, ctxErr error, failOrOtherNum int) int {
	if isBreak {
		return 7
	}
	if errors.Is(ctxErr, context.DeadlineExceeded) {
		return 5
	}
	if failOrOtherNum > 0 {
		return 3
	}
	return 2
}

// JobClient manages a single job's lifecycle
type JobClient struct {
	JobID          int64
	Job            map[string]interface{}
	Scheduler      *Scheduler
	JobDoing       bool
	CurrentJobTask *JobTask
	mu             sync.Mutex
}

func (jc *JobClient) tryMarkDoing() bool {
	jc.mu.Lock()
	defer jc.mu.Unlock()
	if jc.JobDoing {
		return false
	}
	jc.JobDoing = true
	return true
}

func (jc *JobClient) isDoing() bool {
	jc.mu.Lock()
	defer jc.mu.Unlock()
	return jc.JobDoing
}

func (jc *JobClient) markDone() {
	jc.mu.Lock()
	jc.JobDoing = false
	jc.mu.Unlock()
}

func (jc *JobClient) setCurrentTask(task *JobTask) {
	jc.mu.Lock()
	jc.CurrentJobTask = task
	jc.mu.Unlock()
}

func (jc *JobClient) currentTask() *JobTask {
	jc.mu.Lock()
	defer jc.mu.Unlock()
	return jc.CurrentJobTask
}

func (jc *JobClient) clearCurrentTask(task *JobTask) {
	jc.mu.Lock()
	if task == nil || jc.CurrentJobTask == task {
		jc.CurrentJobTask = nil
	}
	jc.mu.Unlock()
}

func (jc *JobClient) setEnable(enable int) {
	jc.mu.Lock()
	if jc.Job != nil {
		jc.Job["enable"] = enable
	}
	jc.mu.Unlock()
}

func (jc *JobClient) enabled() bool {
	jc.mu.Lock()
	defer jc.mu.Unlock()
	return jc.Job != nil && toInt(jc.Job["enable"]) == 1
}

// NewJobClient creates a new job client
func NewJobClient(job map[string]interface{}, isInit bool) *JobClient {
	jc := &JobClient{
		Job: job,
	}

	addJobID := int64(0)
	if _, ok := job["enable"]; !ok {
		job["enable"] = 1
	}
	if _, ok := job["method"]; !ok {
		job["method"] = 0
	}

	if _, ok := job["id"]; !ok {
		id, err := mapper.AddJob(job)
		if err != nil {
			panic(err.Error())
		}
		addJobID = id
		var err2 error
		job, err2 = mapper.GetJobByID(id)
		if err2 != nil {
			panic(err2.Error())
		}
	}

	jc.JobID = toInt64(job["id"])
	jc.Job = job

	sched := NewScheduler()
	jc.Scheduler = sched

	err := sched.AddJob(toInt(job["isCron"]), job, func() {
		jc.DoScheduled()
	})
	if err != nil {
		if isInit || addJobID != 0 {
			log.Printf("Error during job setup, deleting job: %v", job)
			mapper.DeleteJob(jc.JobID)
		}
		panic(err.Error())
	}

	return jc
}

func (jc *JobClient) runMarkedJob() {
	taskID := int64(0)
	defer func() {
		if r := recover(); r != nil {
			jc.markDone()
			jc.clearCurrentTask(nil)
			errMsg := fmt.Sprintf("%v", r)
			log.Printf("Job execution error: %s", errMsg)
			if taskID > 0 {
				UpdateJobTaskStatusSimple(taskID, 6, &errMsg)
			}
		}
	}()

	var err error
	taskID, err = mapper.AddJobTask(jc.JobID, time.Now().Unix())
	if err != nil {
		panic(err.Error())
	}
	if !jc.enabled() {
		panic("abort")
	}
	task := newJobTask(taskID, jc)
	jc.setCurrentTask(task)
	task.Start()
}

// DoJob executes the job, waiting until any current run has finished.
func (jc *JobClient) DoJob() {
	for !jc.tryMarkDoing() {
		if !jc.enabled() {
			return
		}
		time.Sleep(10 * time.Second)
	}
	jc.runMarkedJob()
}

// DoScheduled executes a scheduled job once, skipping if the previous run is still active.
func (jc *JobClient) DoScheduled() bool {
	if !jc.tryMarkDoing() {
		log.Printf("Skipping job %d because previous run is still active", jc.JobID)
		return false
	}
	jc.runMarkedJob()
	return true
}

// DoManual triggers manual execution
func (jc *JobClient) DoManual() {
	if !jc.tryMarkDoing() {
		panic(i18n.G("job_running"))
	}
	go jc.runMarkedJob()
}

// ResumeJob enables and resumes the job
func (jc *JobClient) ResumeJob() {
	if toInt(jc.Job["isCron"]) == 2 {
		// Manual only, just enable
		mapper.UpdateJobEnable(jc.JobID, 1)
		jc.setEnable(1)
		return
	}

	err := jc.Scheduler.Resume(toInt(jc.Job["isCron"]), jc.Job, func() {
		jc.DoScheduled()
	})
	if err != nil {
		panic(err.Error())
	}
	mapper.UpdateJobEnable(jc.JobID, 1)
	jc.setEnable(1)
}

// AbortJob aborts the current running task
func (jc *JobClient) AbortJob() {
	if task := jc.currentTask(); task != nil {
		task.requestBreak()
	}
}

// StopJob stops the job (for disable or delete)
func (jc *JobClient) StopJob(remove bool) {
	jc.setEnable(0)
	if task := jc.currentTask(); task != nil {
		task.requestBreak()
	}
	if remove {
		jc.Scheduler.Stop()
	} else {
		jc.Scheduler.Pause()
		mapper.UpdateJobEnable(jc.JobID, 0)
		mapper.UpdateJobTaskStatusByStatusAndJobID(jc.JobID)
	}
}

// UpdateJobTaskStatusFinal updates task status after completion with notification
func UpdateJobTaskStatusFinal(taskID int64, status int, currentTasks map[int][]map[string]interface{}, createTime float64) {
	var duration int
	if createTime > 0 {
		duration = int(float64(time.Now().Unix()) - createTime)
	}

	var taskNum map[string]interface{}
	var errMsg *string

	if currentTasks != nil {
		successNum := len(currentTasks[2])
		failNum := len(currentTasks[7])
		otherNum := len(currentTasks[-1])
		allNum := successNum + failNum + otherNum

		var sumSize int64
		if tasks, ok := currentTasks[2]; ok {
			for _, t := range tasks {
				if t["fileSize"] != nil && t["type"].(int) != 1 {
					sumSize += toInt64Val(t["fileSize"])
				}
			}
		}

		taskNum = map[string]interface{}{
			"waitNum":    0,
			"runningNum": 0,
			"successNum": successNum,
			"failNum":    failNum,
			"otherNum":   otherNum,
			"allNum":     allNum,
			"duration":   duration,
			"sumSize":    sumSize,
		}
	} else {
		taskNum = GetCuTaskNum(taskID)
		taskNum["duration"] = duration
	}

	mapper.UpdateJobTaskStatus(taskID, status, errMsg)
	taskNumJSON, _ := json.Marshal(taskNum)
	mapper.UpdateJobTaskNumMany([]map[string]interface{}{
		{"taskId": taskID, "taskNum": string(taskNumJSON)},
	})

	// Send notifications
	SendTaskNotification(taskID, status, taskNum, duration, createTime)
}

// UpdateJobTaskStatusSimple updates task status with error message
func UpdateJobTaskStatusSimple(taskID int64, status int, errMsg *string) {
	mapper.UpdateJobTaskStatus(taskID, status, errMsg)
	taskNum := GetCuTaskNum(taskID)
	taskNumJSON, _ := json.Marshal(taskNum)
	mapper.UpdateJobTaskNumMany([]map[string]interface{}{
		{"taskId": taskID, "taskNum": string(taskNumJSON)},
	})
}

// GetCuTaskNum gets current task counts from DB
func GetCuTaskNum(taskID int64) map[string]interface{} {
	return mapper.GetJobTaskCounts(taskID)
}

func toInt64Val(v interface{}) int64 {
	switch val := v.(type) {
	case int64:
		return val
	case int:
		return int64(val)
	case float64:
		return int64(val)
	default:
		return 0
	}
}

func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	default:
		return 0
	}
}
