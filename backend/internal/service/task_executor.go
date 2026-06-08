package service

import "time"

func (jt *JobTask) taskSubmit() {
	jt.runCopyExecutor()
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
	limits := runtimeTaskLimits()
	for jt.doingLen() < limits.CopyConcurrency {
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
		defer jt.recoverWorkerPanic("copy", nil)
		item.DoIt()
	}()
}

func (jt *JobTask) markWaitingAsAborted() {
	for _, item := range jt.Waiting.closeAndDrain() {
		item.setStatus(taskStatusStopped)
		item.mu.RLock()
		jt.CopyHook(item.SrcPath, item.DstPath, item.FileName, item.FileSize, item.AlistTaskID,
			taskStatusStopped, item.ErrMsg, taskItemFile, item.CopyType, item.CreateTime)
		item.mu.RUnlock()
	}
}
