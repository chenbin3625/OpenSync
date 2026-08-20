package service

import "time"

func (jt *JobTask) taskSubmit() {
	jt.runCopyExecutor()
	jt.stopCopyMonitor()
	persistErr := jt.persistRemainingTaskItems()
	jt.finishSubmittedTask(persistErr)
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

	if jt.isBreak() {
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
	for jt.doingLen() < jt.copyConcurrencyLimit() {
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
		jt.runCopyItem(item)
	}()
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
