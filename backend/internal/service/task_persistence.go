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
		jobID := int64(0)
		if jt.JobClient != nil {
			jobID = jt.JobClient.JobID
		}
		log.Printf(
			"Failed to save task item for task %d (job %d) file=%q src=%q status=%d: %v",
			jt.TaskID, jobID, item.FileName, item.SrcPath, item.Status, err,
		)
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
