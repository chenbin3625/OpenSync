package service

import (
	"log"
	"time"
)

func (jt *JobTask) persistRemainingTaskItems() error {
	jt.cancelPersistFlushTimer()
	if err := jt.flushPersistBuffer(); err != nil {
		return err
	}
	return jt.taskPersistenceError()
}

func (jt *JobTask) appendFinish(item JobTaskItem) {
	jt.appendFinishMany([]JobTaskItem{item})
}

func (jt *JobTask) recordFinishNoFlush(items []JobTaskItem) bool {
	if len(items) == 0 {
		return false
	}
	jt.initRuntime()

	jt.FinishMu.Lock()
	for _, item := range items {
		status := item.Status
		jt.FinishedCounts[status]++
		if size := item.CountableFileSize(); size > 0 {
			jt.FinishedSizes[status] += size
		}
	}
	jt.FinishMu.Unlock()

	jt.persistBufMu.Lock()
	jt.persistBuffer = append(jt.persistBuffer, items...)
	needFlush := len(jt.persistBuffer) >= persistBatchSize
	jt.persistBufMu.Unlock()
	return needFlush
}

func (jt *JobTask) postRecordFinish(needFlush bool) {
	if needFlush {
		if err := jt.flushPersistBuffer(); err != nil {
			jt.recordTaskPersistenceError(err)
			jt.requestBreak()
		}
	} else {
		jt.schedulePersistFlush()
	}
	jt.notifyProgressChange()
}

func (jt *JobTask) appendFinishMany(items []JobTaskItem) {
	if len(items) == 0 {
		return
	}
	needFlush := jt.recordFinishNoFlush(items)
	jt.postRecordFinish(needFlush)
}

func (jt *JobTask) schedulePersistFlush() {
	jt.persistFlushMu.Lock()
	defer jt.persistFlushMu.Unlock()
	if jt.persistFlushScheduled {
		return
	}
	jt.persistFlushScheduled = true
	jt.persistFlushTimer = time.AfterFunc(persistFlushInterval, func() {
		jt.persistFlushMu.Lock()
		jt.persistFlushScheduled = false
		jt.persistFlushTimer = nil
		jt.persistFlushMu.Unlock()
		if err := jt.flushPersistBuffer(); err != nil {
			jt.recordTaskPersistenceError(err)
			jt.requestBreak()
		}
	})
}

func (jt *JobTask) flushPersistBuffer() error {
	jt.cancelPersistFlushTimer()
	// Finalization must wait for batches already removed from the buffer.
	jt.persistWriteMu.Lock()
	defer jt.persistWriteMu.Unlock()
	jt.persistBufMu.Lock()
	if len(jt.persistBuffer) == 0 {
		jt.persistBufMu.Unlock()
		return jt.taskPersistenceError()
	}
	items := append([]JobTaskItem(nil), jt.persistBuffer...)
	jt.persistBuffer = jt.persistBuffer[:0]
	jt.persistBufMu.Unlock()

	if err := callPersistJobTaskItems(jobTaskItemsToMaps(items)); err != nil {
		jt.persistBufMu.Lock()
		jt.persistBuffer = append(items, jt.persistBuffer...)
		jt.persistBufMu.Unlock()

		jobID := int64(0)
		if jt.JobClient != nil {
			jobID = jt.JobClient.idSnapshot()
		}
		first := items[0]
		log.Printf(
			"Failed to save %d task items for task %d (job %d) first file=%q: %v",
			len(items), jt.TaskID, jobID, first.FileName, err,
		)
		jt.recordTaskPersistenceError(err)
		jt.requestBreak()
		return err
	}
	return jt.taskPersistenceError()
}

func (jt *JobTask) cancelPersistFlushTimer() {
	jt.persistFlushMu.Lock()
	if jt.persistFlushTimer != nil {
		jt.persistFlushTimer.Stop()
		jt.persistFlushTimer = nil
	}
	jt.persistFlushScheduled = false
	jt.persistFlushMu.Unlock()
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
