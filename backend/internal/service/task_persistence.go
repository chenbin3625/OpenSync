package service

import "log"

func (jt *JobTask) persistRemainingTaskItems() error {
	var persistErr error
	if err := jt.flushPendingTaskItems(); err != nil {
		log.Printf("Failed to save pending task items for task %d: %v", jt.TaskID, err)
		persistErr = err
	}

	if err := jt.flushFinishedTaskItems(); err != nil {
		log.Printf("Failed to save task items for task %d: %v", jt.TaskID, err)
		if persistErr == nil {
			persistErr = err
		}
	}
	return persistErr
}

func (jt *JobTask) appendFinish(item JobTaskItem) {
	jt.initRuntime()
	var flush []JobTaskItem
	jt.FinishMu.Lock()
	status := item.Status
	jt.FinishedCounts[status]++
	if size := item.CountableFileSize(); size > 0 {
		jt.FinishedSizes[status] += size
	}
	jt.Finish = append(jt.Finish, item)
	if overflow := len(jt.Finish) - runtimeTaskLimits().RealtimeFinishedItems; overflow > 0 {
		jt.pendingPersist = append(jt.pendingPersist, jt.Finish[:overflow]...)
		jt.Finish = append([]JobTaskItem(nil), jt.Finish[overflow:]...)
	}
	if len(jt.pendingPersist) >= maxPersistTaskItemBatch {
		flush = append([]JobTaskItem(nil), jt.pendingPersist...)
		jt.pendingPersist = jt.pendingPersist[:0]
	}
	jt.FinishMu.Unlock()

	if len(flush) > 0 {
		if err := persistJobTaskItems(jobTaskItemsToMaps(flush)); err != nil {
			log.Printf("Failed to flush task items for task %d: %v", jt.TaskID, err)
		}
	}
}

func (jt *JobTask) flushPendingTaskItems() error {
	jt.initRuntime()
	jt.FinishMu.Lock()
	pending := append([]JobTaskItem(nil), jt.pendingPersist...)
	jt.pendingPersist = jt.pendingPersist[:0]
	jt.FinishMu.Unlock()
	if len(pending) == 0 {
		return nil
	}
	return persistJobTaskItems(jobTaskItemsToMaps(pending))
}

func (jt *JobTask) flushFinishedTaskItems() error {
	finish := jt.drainFinishedTaskItems()
	if len(finish) == 0 {
		return nil
	}
	return persistJobTaskItems(jobTaskItemsToMaps(finish))
}

func (jt *JobTask) drainFinishedTaskItems() []JobTaskItem {
	jt.FinishMu.Lock()
	defer jt.FinishMu.Unlock()

	finish := append([]JobTaskItem(nil), jt.Finish...)
	jt.Finish = jt.Finish[:0]
	return finish
}
