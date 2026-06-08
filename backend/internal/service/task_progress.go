package service

import (
	"sort"
	"time"
)

func (jt *JobTask) scanProgress() map[string]int64 {
	total := jt.ScanTotalDirs.Load()
	scanned := jt.ScanDoneDirs.Load()
	remaining := total - scanned
	if remaining < 0 {
		remaining = 0
	}
	return map[string]int64{
		"scannedDirs":   scanned,
		"remainingDirs": remaining,
		"totalDirs":     total,
	}
}

// GetCurrent returns real-time task progress
func (jt *JobTask) GetCurrent() map[string]interface{} {
	jt.initRuntime()
	now := time.Now().Unix()
	jt.LastWatching.Store(now)

	waits := jt.waitingTaskMaps()
	dos := jt.doingTaskMaps()

	jt.CurrentMu.Lock()
	jt.CurrentTasks = map[int][]map[string]interface{}{
		taskStatusWaiting.Int(): waits,
		taskStatusRunning.Int(): dos,
	}
	jt.CurrentMu.Unlock()

	result := map[string]interface{}{
		"taskId":     jt.TaskID,
		"scanFinish": jt.ScanFinish.Load(),
		"scan":       jt.scanProgress(),
		"doingTask":  dos,
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
	numMap["wait"] = len(waits)
	sizeMap["wait"] = taskListSize(waits)
	numMap["running"] = len(dos)
	sizeMap["running"] = taskListSize(dos)

	for _, item := range []struct {
		key    string
		status taskStatus
	}{
		{"success", taskStatusSuccess},
		{"fail", taskStatusFailed},
		{"other", taskStatusOther},
	} {
		count, size := jt.finishedAggregateForStatus(item.status)
		numMap[item.key] = count
		sizeMap[item.key] = size
	}
	return result
}

// GetCurrentByStatus returns tasks filtered by status
func (jt *JobTask) GetCurrentByStatus(status int) []map[string]interface{} {
	jt.initRuntime()
	return jt.currentTasksForStatus(status)
}

func (jt *JobTask) GetCurrentByStatusPage(status, pageSize, pageNum int) map[string]interface{} {
	jt.initRuntime()
	tasks := jt.currentTasksForStatus(status)
	count := len(tasks)
	if pageSize > 0 && pageNum > 0 {
		start := (pageNum - 1) * pageSize
		if start >= count {
			tasks = []map[string]interface{}{}
		} else {
			end := start + pageSize
			if end > count {
				end = count
			}
			tasks = tasks[start:end]
		}
	}
	return map[string]interface{}{
		"dataList": tasks,
		"count":    count,
	}
}

func (jt *JobTask) finishedAggregateForStatus(status taskStatus) (int, int64) {
	jt.FinishMu.Lock()
	defer jt.FinishMu.Unlock()
	if status != taskStatusOther {
		return jt.FinishedCounts[status], jt.FinishedSizes[status]
	}
	count := 0
	size := int64(0)
	for s, c := range jt.FinishedCounts {
		if isOtherTaskStatus(s) {
			count += c
			size += jt.FinishedSizes[s]
		}
	}
	return count, size
}

func (jt *JobTask) waitingTaskMaps() []map[string]interface{} {
	waitingItems := jt.Waiting.snapshot()
	waits := make([]map[string]interface{}, len(waitingItems))
	for i, w := range waitingItems {
		waits[i] = jt.copyItemToMap(w)
	}
	return waits
}

func (jt *JobTask) doingTaskMaps() []map[string]interface{} {
	jt.DoingMu.Lock()
	defer jt.DoingMu.Unlock()

	dos := make([]map[string]interface{}, 0, len(jt.Doing))
	for _, d := range jt.Doing {
		dos = append(dos, jt.copyItemToMap(d))
	}
	return dos
}

func (jt *JobTask) currentTasksForStatus(statusValue int) []map[string]interface{} {
	status := taskStatusFromValue(statusValue)
	var tasks []map[string]interface{}
	switch status {
	case taskStatusWaiting:
		tasks = jt.waitingTaskMaps()
	case taskStatusRunning:
		tasks = jt.doingTaskMaps()
	case taskStatusOther:
		tasks = jt.finishedTaskMaps(func(status taskStatus) bool {
			return isOtherTaskStatus(status)
		})
	default:
		tasks = jt.finishedTaskMaps(func(itemStatus taskStatus) bool {
			return itemStatus == status
		})
	}
	sortTaskMapsByCreateTimeDesc(tasks)
	return tasks
}

func (jt *JobTask) finishedTaskMaps(match func(status taskStatus) bool) []map[string]interface{} {
	jt.FinishMu.Lock()
	defer jt.FinishMu.Unlock()

	tasks := make([]map[string]interface{}, 0)
	for _, item := range jt.Finish {
		if match(item.Status) {
			tasks = append(tasks, item.ToMap())
		}
	}
	return tasks
}

func sortTaskMapsByCreateTimeDesc(tasks []map[string]interface{}) {
	sort.SliceStable(tasks, func(i, j int) bool {
		left := toInt64Val(tasks[i]["createTime"])
		right := toInt64Val(tasks[j]["createTime"])
		if left == right {
			return toInt64Val(tasks[i]["id"]) > toInt64Val(tasks[j]["id"])
		}
		return left > right
	})
}

func taskListSize(tasks []map[string]interface{}) int64 {
	var totalSize int64
	for _, task := range tasks {
		if task["fileSize"] != nil && taskItemTypeFromValue(task["type"]) != taskItemTypeDelete {
			totalSize += toInt64Val(task["fileSize"])
		}
	}
	return totalSize
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
	return item.ToMap(jt.TaskID)
}

// CopyHook is called when a copy operation completes
func (jt *JobTask) CopyHook(srcPath, dstPath, fileName string, fileSize interface{},
	alistTaskID string, status taskStatus, errMsg *string, isPath taskItemObject, copyType taskItemType, createTime int64) {
	jt.appendFinish(NewCopyJobTaskItem(jt.TaskID, srcPath, dstPath, fileName, fileSize,
		alistTaskID, status, errMsg, isPath, copyType, createTime))
}

// DelHook is called when a delete operation completes
func (jt *JobTask) DelHook(dstPath, fileName string, fileSize interface{}, status taskStatus, errMsg *string, isPath taskItemObject, createTime int64) {
	jt.appendFinish(NewDeleteJobTaskItem(jt.TaskID, dstPath, fileName, fileSize,
		status, errMsg, isPath, createTime))
}
