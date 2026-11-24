package service

import (
	"sync"
	"time"
)

func (jt *JobTask) taskSubmit() {
	jt.submitActive.Store(true)
	jt.runCopyExecutor()
	if !jt.isBreak() && jt.context().Err() == nil && jt.fatalError() == nil && jt.taskPersistenceError() == nil && jt.queuedCopyFailures.Load() == 0 {
		jt.runDeferredDeletes()
	}
	jt.stopCopyMonitor()
	persistErr := jt.persistRemainingTaskItems()
	jt.finishSubmittedTask(persistErr)
}

func (jt *JobTask) cleanupAfterSubmitPanic(errMsg string) {
	jt.cancelTaskContext()
	jt.markWaitingAsAborted()
	jt.waitScanDone()
	jt.markWaitingAsAborted()
	jt.copyWG.Wait()
	jt.stopCopyMonitor()
	_ = jt.persistRemainingTaskItems()
	jt.finishFailedTask(errMsg)
}

func (jt *JobTask) runCopyExecutor() {
	jt.initRuntime()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		if jt.stopCopyExecutorIfBroken() {
			break
		}
		// Context cancelled (e.g. task timeout) without an explicit break flag:
		// drain waiting items and stop the executor. Without this, the select in
		// waitForCopyExecutorSignal returns immediately on the always-ready
		// ctx.Done() channel and the loop spins at 100% CPU until scan finishes.
		if jt.context().Err() != nil {
			jt.markWaitingAsAborted()
			break
		}

		started := jt.startAvailableCopyItems()
		if jt.copyExecutorDrained() {
			break
		}
		if started {
			continue
		}

		jt.waitForCopyExecutorSignal(ticker.C)
	}

	// Ensure the scan goroutine has fully exited before waiting on copyWG,
	// stopping the monitor, persisting remaining items, or releasing JobClient.
	if jt.isBreak() || jt.context().Err() != nil {
		jt.cancelTaskContext()
		jt.markWaitingAsAborted()
	}
	jt.waitScanDone()
	if jt.isBreak() || jt.context().Err() != nil {
		jt.markWaitingAsAborted()
	}
	jt.copyWG.Wait()
}

func (jt *JobTask) stopCopyExecutorIfBroken() bool {
	if !jt.isBreak() {
		return false
	}
	jt.markWaitingAsAborted()
	return true
}

func (jt *JobTask) startAvailableCopyItems() bool {
	started := false
	for {
		if jt.isBreak() {
			jt.markWaitingAsAborted()
			break
		}
		if !jt.tryAcquireCopySlot() {
			break
		}

		item, ok := jt.Waiting.pop()
		if !ok {
			jt.releaseCopySlot()
			break
		}
		jt.startCopyItemTrackedWithSlot(item, nil)
		started = true
	}
	return started
}

func (jt *JobTask) copyExecutorDrained() bool {
	return jt.ScanFinish.Load() && jt.doingLen() == 0 && jt.Waiting.len() == 0
}

func (jt *JobTask) waitForCopyExecutorSignal(tick <-chan time.Time) {
	select {
	case <-jt.context().Done():
		jt.markWaitingAsAborted()
	case <-jt.Waiting.waitCh():
	case <-tick:
	}
}

func (jt *JobTask) doingLen() int {
	jt.DoingMu.Lock()
	defer jt.DoingMu.Unlock()
	return len(jt.Doing)
}

func (jt *JobTask) startCopyItem(item *CopyItem) {
	if jt.acquireCopySlot() {
		jt.startCopyItemTrackedWithSlot(item, nil)
	}
}

// startCopyItemTracked starts item and, when batch is non-nil, also reports
// completion to it. Callers outside the submit executor use the batch handle to
// wait for their own items instead of every copy in flight.
func (jt *JobTask) startCopyItemTracked(item *CopyItem, batch *sync.WaitGroup) {
	if !jt.acquireCopySlot() {
		return
	}
	jt.startCopyItemTrackedWithSlot(item, batch)
}

func (jt *JobTask) startCopyItemTrackedWithSlot(item *CopyItem, batch *sync.WaitGroup) {
	if jt.FirstSync.Load() == 0 {
		jt.FirstSync.CompareAndSwap(0, time.Now().Unix())
	}
	key := jt.QueueNum.Add(1)
	item.mu.Lock()
	item.DoingKey = key
	item.copySlotHeld = true
	item.mu.Unlock()

	jt.DoingMu.Lock()
	jt.Doing[key] = item
	jt.DoingMu.Unlock()

	jt.copyWG.Add(1)
	if batch != nil {
		batch.Add(1)
	}
	go func() {
		defer jt.copyWG.Done()
		if batch != nil {
			defer batch.Done()
		}
		jt.runCopyItem(item)
	}()
}

// waitForCopySlot blocks until the number of in-flight copies drops below limit.
// It reports false when the task was stopped or cancelled while waiting.
func (jt *JobTask) waitForCopySlot(limit int) bool {
	_ = limit
	return jt.acquireCopySlot()
}

func (jt *JobTask) tryAcquireCopySlot() bool {
	jt.initRuntime()
	select {
	case jt.copySem <- struct{}{}:
		return true
	default:
		return false
	}
}

func (jt *JobTask) acquireCopySlot() bool {
	jt.initRuntime()
	select {
	case jt.copySem <- struct{}{}:
		return true
	case <-jt.context().Done():
		return false
	}
}

func (jt *JobTask) releaseCopySlot() {
	jt.initRuntime()
	<-jt.copySem
}

func (jt *JobTask) runCopyItem(item *CopyItem) {
	defer func() {
		if r := recover(); r != nil {
			errMsg := workerPanicMessage("copy", r)
			jt.failCopyItemIfStillDoing(item, errMsg)
			jt.handleWorkerPanic("copy", r)
		}
	}()
	item.DoIt()
}

func (jt *JobTask) failCopyItemIfStillDoing(item *CopyItem, errMsg string) {
	if !jt.copyItemStillDoing(item) {
		return
	}
	item.setProgress(taskStatusFailed, item.progress(), &errMsg)
	jt.finishCopyItem(item)
}

func (jt *JobTask) copyItemStillDoing(item *CopyItem) bool {
	jt.DoingMu.Lock()
	defer jt.DoingMu.Unlock()
	return item != nil && jt.Doing[item.DoingKey] == item
}

func (jt *JobTask) markWaitingAsAborted() {
	items := jt.Waiting.closeAndDrain()
	if len(items) == 0 {
		return
	}
	jt.queuedCopyFailures.Add(int64(len(items)))
	taskItems := make([]JobTaskItem, 0, len(items))
	for _, item := range items {
		item.setStatus(taskStatusStopped)
		item.mu.RLock()
		taskItems = append(taskItems, NewCopyJobTaskItem(
			jt.TaskID, item.SrcPath, item.DstPath, item.FileName, item.FileSize,
			item.AlistTaskID, taskStatusStopped, item.ErrMsg, taskItemFile, item.CopyType, item.CreateTime,
		))
		item.mu.RUnlock()
	}
	jt.appendFinishMany(taskItems)
}
