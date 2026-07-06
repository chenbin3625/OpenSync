package service

import "log"

func (jt *JobTask) persistRemainingTaskItems() error {
	return jt.taskPersistenceError()
}

func (jt *JobTask) appendFinish(item JobTaskItem) {
	jt.initRuntime()
	jt.FinishMu.Lock()
	status := item.Status
	jt.FinishedCounts[status]++
	if size := item.CountableFileSize(); size > 0 {
		jt.FinishedSizes[status] += size
	}
	jt.FinishMu.Unlock()

	if err := persistJobTaskItems(jobTaskItemsToMaps([]JobTaskItem{item})); err != nil {
		log.Printf("Failed to save task item for task %d: %v", jt.TaskID, err)
		jt.recordTaskPersistenceError(err)
		jt.requestBreak()
	}
}

func (jt *JobTask) recordTaskPersistenceError(err error) {
	if err == nil {
		return
	}
	jt.PersistMu.Lock()
	defer jt.PersistMu.Unlock()
	if jt.PersistErr == nil {
		jt.PersistErr = err
	}
}

func (jt *JobTask) taskPersistenceError() error {
	jt.PersistMu.Lock()
	defer jt.PersistMu.Unlock()
	return jt.PersistErr
}
