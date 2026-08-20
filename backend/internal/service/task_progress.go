package service

import (
	"log"
	"math"
	"opensync/internal/mapper"
	"opensync/pkg/util"
	"sort"
	"time"
)

func (jt *JobTask) scanProgress() scanProgressPayload {
	total := jt.ScanTotalDirs.Load()
	scanned := jt.ScanDoneDirs.Load()
	remaining := total - scanned
	if remaining < 0 {
		remaining = 0
	}
	return scanProgressPayload{
		ScannedDirs:   scanned,
		RemainingDirs: remaining,
		TotalDirs:     total,
	}
}

func (jt *JobTask) touchWatching() {
	jt.LastWatching.Store(time.Now().Unix())
}

// TouchJobWatching refreshes the "viewer present" timestamp used by copy pollers.
func TouchJobWatching(jobID int64) {
	jobClientListMu.RLock()
	client := jobClientList[jobID]
	jobClientListMu.RUnlock()
	if client == nil {
		return
	}
	if task := client.currentTask(); task != nil {
		task.touchWatching()
	}
}

type scanProgressPayload struct {
	ScannedDirs   int64 `json:"scannedDirs"`
	RemainingDirs int64 `json:"remainingDirs"`
	TotalDirs     int64 `json:"totalDirs"`
}

type taskNumStats struct {
	Wait    int `json:"wait"`
	Running int `json:"running"`
	Success int `json:"success"`
	Fail    int `json:"fail"`
	Other   int `json:"other"`
}

type taskSizeStats struct {
	Wait    int64 `json:"wait"`
	Running int64 `json:"running"`
	Success int64 `json:"success"`
	Fail    int64 `json:"fail"`
	Other   int64 `json:"other"`
}

type streamDoingItem struct {
	ID          int64   `json:"id,omitempty"`
	AlistTaskID string  `json:"alistTaskId,omitempty"`
	FileName    string  `json:"fileName"`
	SrcPath     string  `json:"srcPath"`
	DstPath     string  `json:"dstPath"`
	FileSize    int64   `json:"fileSize"`
	Type        int     `json:"type"`
	Status      int     `json:"status"`
	Progress    float64 `json:"progress"`
	CreateTime  int64   `json:"createTime"`
}

type streamDoingPatch struct {
	ID          int64   `json:"id,omitempty"`
	AlistTaskID string  `json:"alistTaskId,omitempty"`
	Status      int     `json:"status"`
	Progress    float64 `json:"progress"`
	FileName    string  `json:"fileName,omitempty"`
	SrcPath     string  `json:"srcPath,omitempty"`
	DstPath     string  `json:"dstPath,omitempty"`
}

// jobCurrentPayload is the live-progress JSON object shared by GET ?current=1
// and the SSE stream. Typed fields avoid boxing every doing file into
// map[string]interface{} on the 610ms poller / 400ms SSE debounce path.
type jobCurrentPayload struct {
	TaskID     int64                `json:"taskId"`
	ScanFinish bool                 `json:"scanFinish"`
	Scan       *scanProgressPayload `json:"scan,omitempty"`
	DoingTask  []streamDoingItem    `json:"doingTask,omitempty"`
	DoingPatch []streamDoingPatch   `json:"doingPatch,omitempty"`
	CreateTime int                  `json:"createTime"`
	Duration   int                  `json:"duration"`
	FirstSync  *int                 `json:"firstSync"`
	Num        taskNumStats         `json:"num"`
	Size       taskSizeStats        `json:"size"`
	DoneSize   int64                `json:"doneSize"`
	RemainSize int64                `json:"remainSize"`
	Speed      float64              `json:"speed,omitempty"`
	SpeedAvg   float64              `json:"speedAvg,omitempty"`
	RemainTime int                  `json:"remainTime,omitempty"`
}

type transferMeter struct {
	duration int
	doneSize int64
}

func (item streamDoingItem) countableSize() int64 {
	if taskItemTypeFromValue(item.Type) == taskItemTypeDelete {
		return 0
	}
	return item.FileSize
}

func (item streamDoingItem) transferredSize() int64 {
	return int64(float64(item.countableSize()) * item.Progress / 100.0)
}

// GetCurrent returns real-time task progress
func (jt *JobTask) GetCurrent() jobCurrentPayload {
	jt.initRuntime()
	jt.touchWatching()
	now := time.Now().Unix()

	doing := jt.doingStreamItems()
	waitCount, waitSize := jt.Waiting.stats()
	successCount, failCount, otherCount, successSize, failSize, otherSize := jt.finishedAggregates()

	runningSize := int64(0)
	doingSize := int64(0)
	for _, item := range doing {
		runningSize += item.countableSize()
		doingSize += item.transferredSize()
	}

	duration := int(float64(now) - jt.CreateTime)
	remainSize := runningSize - doingSize + waitSize
	if remainSize < 0 {
		remainSize = 0
	}
	doneSize := successSize + doingSize

	payload := jobCurrentPayload{
		TaskID:     jt.TaskID,
		ScanFinish: jt.ScanFinish.Load(),
		DoingTask:  doing,
		CreateTime: int(jt.CreateTime),
		Duration:   duration,
		Num: taskNumStats{
			Wait:    waitCount,
			Running: len(doing),
			Success: successCount,
			Fail:    failCount,
			Other:   otherCount,
		},
		Size: taskSizeStats{
			Wait:    waitSize,
			Running: runningSize,
			Success: successSize,
			Fail:    failSize,
			Other:   otherSize,
		},
		DoneSize:   doneSize,
		RemainSize: remainSize,
	}
	if !payload.ScanFinish {
		scan := jt.scanProgress()
		payload.Scan = &scan
	}
	if firstSync := jt.FirstSync.Load(); firstSync > 0 {
		value := int(firstSync)
		payload.FirstSync = &value
		syncDuration := duration - (value - int(jt.CreateTime))
		if syncDuration > 0 {
			payload.SpeedAvg = float64(doneSize) / float64(syncDuration)
		}
	}

	jt.CurrentMu.Lock()
	if len(jt.CurrentTasks) > 0 {
		clear(jt.CurrentTasks)
	}
	prev := jt.lastMeter
	if prev.duration > 0 && duration != prev.duration {
		payload.Speed = float64(doneSize-prev.doneSize) / float64(duration-prev.duration)
	}
	jt.lastMeter = transferMeter{duration: duration, doneSize: doneSize}
	jt.CurrentMu.Unlock()

	if payload.SpeedAvg > 0 && remainSize > 0 {
		payload.RemainTime = int(math.Ceil(float64(remainSize) / payload.SpeedAvg))
	}
	return payload
}

// GetCurrentByStatus returns tasks filtered by status
func (jt *JobTask) GetCurrentByStatus(status int) []map[string]interface{} {
	jt.initRuntime()
	statusValue := taskStatusFromValue(status)
	if statusValue == taskStatusWaiting || statusValue == taskStatusRunning {
		return jt.currentTasksForStatus(status)
	}
	page := jt.finishedTaskPageFromDB(status, 0, 0)
	tasks, _ := page["dataList"].([]map[string]interface{})
	return tasks
}

func (jt *JobTask) GetCurrentByStatusPage(status, pageSize, pageNum int) map[string]interface{} {
	jt.initRuntime()
	statusValue := taskStatusFromValue(status)
	if statusValue == taskStatusWaiting {
		return jt.waitingTaskPage(pageSize, pageNum)
	}
	if statusValue != taskStatusWaiting && statusValue != taskStatusRunning {
		return jt.finishedTaskPageFromDB(status, pageSize, pageNum)
	}
	tasks := jt.currentTasksForStatus(status)
	count := len(tasks)
	if pageSize > 0 && pageNum > 0 {
		pageIndex := int64(pageNum) - 1
		size := int64(pageSize)
		maxInt := int64(^uint(0) >> 1)
		if pageIndex > maxInt/size {
			tasks = []map[string]interface{}{}
		} else if start64 := pageIndex * size; start64 >= int64(count) {
			tasks = []map[string]interface{}{}
		} else {
			start := int(start64)
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

func (jt *JobTask) waitingTaskPage(pageSize, pageNum int) map[string]interface{} {
	items, count := jt.Waiting.snapshotPage(pageSize, pageNum)
	tasks := make([]map[string]interface{}, len(items))
	for i, item := range items {
		tasks[i] = jt.copyItemToMap(item)
	}
	return map[string]interface{}{
		"dataList": tasks,
		"count":    count,
	}
}

func (jt *JobTask) finishedTaskPageFromDB(status, pageSize, pageNum int) map[string]interface{} {
	params := map[string]interface{}{
		"taskId": jt.TaskID,
		"status": status,
	}
	if pageSize > 0 && pageNum > 0 {
		params["pageSize"] = pageSize
		params["pageNum"] = pageNum
	}
	result, err := mapper.GetJobTaskItemList(params)
	if err != nil {
		log.Printf("Failed to load task items for task %d status %d: %v", jt.TaskID, status, err)
		return map[string]interface{}{
			"dataList": []map[string]interface{}{},
			"count":    int64(0),
		}
	}
	return result
}

func (jt *JobTask) finishedAggregates() (successCount, failCount, otherCount int, successSize, failSize, otherSize int64) {
	jt.FinishMu.Lock()
	defer jt.FinishMu.Unlock()
	successCount = jt.FinishedCounts[taskStatusSuccess]
	successSize = jt.FinishedSizes[taskStatusSuccess]
	failCount = jt.FinishedCounts[taskStatusFailed]
	failSize = jt.FinishedSizes[taskStatusFailed]
	for s, c := range jt.FinishedCounts {
		if isOtherTaskStatus(s) {
			otherCount += c
			otherSize += jt.FinishedSizes[s]
		}
	}
	return
}

func (jt *JobTask) finishedAggregateForStatus(status taskStatus) (int, int64) {
	successCount, failCount, otherCount, successSize, failSize, otherSize := jt.finishedAggregates()
	switch status {
	case taskStatusSuccess:
		return successCount, successSize
	case taskStatusFailed:
		return failCount, failSize
	default:
		return otherCount, otherSize
	}
}

func (jt *JobTask) waitingTaskMaps() []map[string]interface{} {
	waitingItems := jt.Waiting.snapshot()
	waits := make([]map[string]interface{}, len(waitingItems))
	for i, w := range waitingItems {
		waits[i] = jt.copyItemToMap(w)
	}
	return waits
}

func (jt *JobTask) doingStreamItems() []streamDoingItem {
	jt.DoingMu.Lock()
	defer jt.DoingMu.Unlock()

	doing := make([]streamDoingItem, 0, len(jt.Doing))
	for _, item := range jt.Doing {
		doing = append(doing, item.toStreamItem())
	}
	return doing
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
	default:
		tasks = []map[string]interface{}{}
	}
	sortTaskMapsByCreateTimeDesc(tasks)
	return tasks
}

func sortTaskMapsByCreateTimeDesc(tasks []map[string]interface{}) {
	sort.Slice(tasks, func(i, j int) bool {
		left := util.ToInt64(tasks[i]["createTime"])
		right := util.ToInt64(tasks[j]["createTime"])
		if left == right {
			return util.ToInt64(tasks[i]["id"]) > util.ToInt64(tasks[j]["id"])
		}
		return left > right
	})
}

func taskListSize(tasks []map[string]interface{}) int64 {
	var totalSize int64
	for _, task := range tasks {
		if task["fileSize"] != nil && taskItemTypeFromValue(task["type"]) != taskItemTypeDelete {
			totalSize += util.ToInt64(task["fileSize"])
		}
	}
	return totalSize
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
