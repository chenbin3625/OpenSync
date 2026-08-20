package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"opensync/internal/mapper"
	"opensync/internal/msg"
	"opensync/pkg/util"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	jobClientList    = make(map[int64]*JobClient)
	jobClientListMu  sync.RWMutex
	getEnableJobList = mapper.GetEnableJobList
)

var (
	taskNumUpdateSlots      = make(chan struct{}, 1)
	pendingTaskNumUpdatesMu sync.Mutex
	pendingTaskNumUpdates   = make(map[int64]map[string]interface{})
)

// InitJobs loads and starts all enabled jobs on startup
func InitJobs() {
	logger := log.Default()
	if err := mapper.UpdateJobTaskStatusByStatus(); err != nil {
		logger.Printf("Failed to mark unfinished task history as aborted: %v", err)
	}
	RunTaskRetentionCleanup()
	jobList, err := mapper.GetJobListAll()
	if err != nil {
		logger.Printf("Failed to get job list: %v", err)
		return
	}
	for _, item := range jobList {
		logger.Printf("Adding jobId %v", item["id"])
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Printf("Error adding job: %v", r)
				}
			}()
			AddJobClient(item, true)
		}()
	}
}

func ShutdownJobs(ctx context.Context) {
	jobClientListMu.RLock()
	clients := make([]*JobClient, 0, len(jobClientList))
	for _, client := range jobClientList {
		clients = append(clients, client)
	}
	jobClientListMu.RUnlock()

	for _, client := range clients {
		client.StopJob(true)
	}

	var wg sync.WaitGroup
	for _, client := range clients {
		wg.Add(1)
		go func(client *JobClient) {
			defer wg.Done()
			waitJobClientIdleContext(ctx, client)
		}(client)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
	}
}

func waitJobClientIdleContext(ctx context.Context, client *JobClient) {
	client.waitUntilIdleContext(ctx, 0)
}

func CleanupExpiredTasks(logger *log.Logger, taskSaveDays int, now time.Time) {
	cutoff, ok := taskRetentionCutoff(now, taskSaveDays)
	if !ok {
		return
	}
	if err := mapper.DeleteJobTaskByRunTime(cutoff); err != nil {
		logger.Printf("Failed to delete expired task history: %v", err)
	}
}

func taskRetentionCutoff(now time.Time, taskSaveDays int) (int64, bool) {
	if taskSaveDays <= 0 {
		return 0, false
	}
	return now.Add(-time.Duration(taskSaveDays) * 24 * time.Hour).Unix(), true
}

// GetJobClientByID gets or creates a job client
func GetJobClientByID(jobID int64) *JobClient {
	jobClientListMu.RLock()
	client, ok := jobClientList[jobID]
	jobClientListMu.RUnlock()
	if ok {
		return client
	}

	jobClientListMu.Lock()
	defer jobClientListMu.Unlock()

	if client, ok := jobClientList[jobID]; ok {
		return client
	}

	job, err := mapper.GetJobByID(jobID)
	if err != nil {
		// Surface "job not found" as a meaningful public message instead of the
		// generic "internal server error" that a plain panic produces. Real DB
		// errors keep the raw panic (masked as 500).
		if err.Error() == msg.JobNotFound {
			panicPublic(msg.JobNotFound)
		}
		panic(err.Error())
	}
	client = NewJobClient(job, false)
	jobClientList[jobID] = client
	return client
}

// CleanJobInput sanitizes job input data
func CleanJobInput(job map[string]interface{}) {
	if util.ToInt(job["isCron"]) == 2 && util.ToInt(job["enable"]) != 1 {
		job["enable"] = 1
	}
	for key, value := range job {
		if s, ok := value.(string); ok {
			trimmed := strings.TrimSpace(s)
			if trimmed == "" {
				job[key] = nil
			} else {
				job[key] = trimmed
			}
		}
	}
	if job["exclude"] != nil {
		excludeStr := fmt.Sprintf("%v", job["exclude"])
		job["exclude"] = normalizeExclude(excludeStr)
	}
	if job["srcPath"] != nil {
		job["srcPath"] = normalizePathListForStorage(job["srcPath"])
	}
	if job["dstPath"] != nil {
		job["dstPath"] = normalizePathListForStorage(job["dstPath"])
	}
	normalizeJobFileSizeRange(job)
}

func ValidateJobInput(job map[string]interface{}) {
	if len(parsePathList(job["srcPath"])) == 0 ||
		len(parsePathList(job["dstPath"])) == 0 ||
		util.ToInt64(job["alistId"]) <= 0 {
		panicPublic(msg.LostPart)
	}

	if enable, ok := job["enable"]; ok {
		enableInt := util.ToInt(enable)
		if enableInt != 0 && enableInt != 1 {
			panicPublic(msg.LostPart)
		}
	}

	method := util.ToInt(job["method"])
	if method < 0 || method > 2 {
		panicPublic(msg.LostPart)
	}

	isCron := util.ToInt(job["isCron"])
	if isCron < 0 || isCron > 2 {
		panicPublic(msg.LostPart)
	}
	if isCron == 0 && util.ToInt(job["interval"]) <= 0 {
		panicPublic(msg.IntervalLost)
	}
}

func normalizeJobFileSizeRange(job map[string]interface{}) {
	minSize, err := nonNegativeFileSize(job["minFileSize"])
	if err != nil {
		panicPublic(msg.MinFileSizeInvalid)
	}
	maxSize, err := nonNegativeFileSize(job["maxFileSize"])
	if err != nil {
		panicPublic(msg.MaxFileSizeInvalid)
	}
	if maxSize > 0 && minSize > maxSize {
		panicPublic(msg.MinFileSizeGtMax)
	}
	job["minFileSize"] = minSize
	job["maxFileSize"] = maxSize
}

func nonNegativeFileSize(value interface{}) (int64, error) {
	if value == nil {
		return 0, nil
	}
	switch v := value.(type) {
	case int:
		if v < 0 {
			return 0, fmt.Errorf("negative file size")
		}
		return int64(v), nil
	case int64:
		if v < 0 {
			return 0, fmt.Errorf("negative file size")
		}
		return v, nil
	case float64:
		if v < 0 || math.Trunc(v) != v {
			return 0, fmt.Errorf("invalid file size")
		}
		parsed, err := strconv.ParseInt(strconv.FormatFloat(v, 'f', 0, 64), 10, 64)
		if err != nil || parsed < 0 {
			return 0, fmt.Errorf("invalid file size")
		}
		return parsed, nil
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0, nil
		}
		parsed, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil || parsed < 0 {
			return 0, fmt.Errorf("invalid file size")
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("invalid file size")
	}
}

// AddJobClient creates a new job client
func AddJobClient(job map[string]interface{}, isInit bool) {
	CleanJobInput(job)
	ValidateJobInput(job)
	client := NewJobClient(job, isInit)
	jobClientListMu.Lock()
	jobClientList[client.JobID] = client
	jobClientListMu.Unlock()
}

// EditJobClient updates an existing job client
func EditJobClient(job map[string]interface{}) {
	jobID := util.ToInt64(job["id"])
	CleanJobInput(job)
	ValidateJobInput(job)
	client := GetJobClientByID(jobID)
	oldJob := client.jobSnapshot()
	oldScheduler := client.schedulerSnapshot()
	if oldScheduler != nil {
		oldScheduler.Stop()
	}
	nextScheduler := NewScheduler()
	dbUpdated := false
	defer func() {
		if r := recover(); r != nil {
			nextScheduler.Stop()
			if dbUpdated && oldJob != nil {
				if err := mapper.UpdateJob(oldJob); err != nil {
					log.Printf("failed to roll back job %d after edit panic: %v", jobID, err)
				}
			}
			panic(r)
		}
	}()
	if err := nextScheduler.AddJob(util.ToInt(job["isCron"]), job, func() {
		client.DoScheduled()
	}); err != nil {
		nextScheduler.Stop()
		panic(err.Error())
	}
	if err := mapper.UpdateJob(job); err != nil {
		nextScheduler.Stop()
		panic(err.Error())
	}
	dbUpdated = true
	client.replaceJobConfig(job, nextScheduler)
}

// DoAllJobManual executes all enabled jobs manually
func DoAllJobManual() {
	jobList, err := getEnableJobList()
	if err != nil {
		panic(err.Error())
	}
	if len(jobList) == 0 {
		panicPublic(msg.NoJobForRun)
	}
	for _, jobItem := range jobList {
		client := GetJobClientByID(util.ToInt64(jobItem["id"]))
		if client.enabled() {
			client.DoManual()
		}
	}
}

// DoJobManual executes a specific job manually
func DoJobManual(jobID int64) {
	client := GetJobClientByID(jobID)
	if !client.enabled() {
		panicPublic(msg.DisabledJobCannotRun)
	}
	client.DoManual()
}

// RemoveJobClient deletes a job
func RemoveJobClient(jobID int64) {
	client := GetJobClientByID(jobID)
	if client.isBusy() {
		panicPublic(msg.JobRunningCannotDelete)
	}
	client.StopJob(true)
	if !client.waitUntilIdle(2 * time.Minute) {
		panicPublic(msg.JobDeleteWaitTimeout)
	}
	if err := mapper.DeleteJob(jobID); err != nil {
		panic(err.Error())
	}
	jobClientListMu.Lock()
	delete(jobClientList, jobID)
	jobClientListMu.Unlock()
}

// ContinueJob enables a job
func ContinueJob(jobID int64) {
	client := GetJobClientByID(jobID)
	client.ResumeJob()
}

// PauseJob disables a job
func PauseJob(jobID int64) {
	client := GetJobClientByID(jobID)
	if util.ToInt(client.jobSnapshot()["isCron"]) == 2 {
		panicPublic(msg.CannotDisableManualJob)
	}
	client.StopJob(false)
}

// AbortJob aborts a running job
func AbortJob(jobID int64) {
	client := GetJobClientByID(jobID)
	client.AbortJob()
}

// StopTask stops a currently running task without changing the job schedule.
func StopTask(taskID int64) {
	job, err := mapper.GetJobByTaskID(taskID)
	if err != nil {
		panic(err.Error())
	}
	client := GetJobClientByID(util.ToInt64(job["id"]))
	task := client.currentTask()
	if task == nil || task.TaskID != taskID {
		panicPublic(msg.TaskNotRunningStop)
	}
	task.requestBreak()
}

// RetryFailedTask replays the non-success items of a historical task.
func RetryFailedTask(taskID int64) {
	job, err := mapper.GetJobByTaskID(taskID)
	if err != nil {
		panic(err.Error())
	}
	client := GetJobClientByID(util.ToInt64(job["id"]))
	if !client.enabled() {
		panicPublic(msg.DisabledJobCannotRun)
	}
	if client.isBusy() {
		panicPublic(msg.JobRunning)
	}
	count, err := countJobTaskItemsByStatuses(taskID, retryableStatusValues())
	if err != nil {
		panic(err.Error())
	}
	if count == 0 {
		panicPublic(msg.NoFailedTaskItems)
	}
	client.DoRetryFailedTaskItems(taskID)
}

// GetJobList returns paginated job list
func GetJobList(params map[string]interface{}) map[string]interface{} {
	result, err := mapper.GetJobList(params)
	if err != nil {
		panic(err.Error())
	}
	return result
}

// GetJobCurrent returns real-time task progress
func GetJobCurrent(jobID int64, params map[string]interface{}) interface{} {
	client := GetJobClientByID(jobID)
	taskClient := client.currentTask()
	if taskClient != nil {
		status, hasStatus := params["status"]
		if !hasStatus || fmt.Sprintf("%v", status) == "" {
			return taskClient.GetCurrent()
		}
		statusInt := util.ToInt(status)
		pageSize := util.ToInt(params["pageSize"])
		pageNum := util.ToInt(params["pageNum"])
		if pageSize > 0 && pageNum > 0 {
			return taskClient.GetCurrentByStatusPage(statusInt, pageSize, pageNum)
		}
		return taskClient.GetCurrentByStatusPage(statusInt, 500, 1)
	}
	return nil
}

// GetTaskList returns paginated task list with task num info
func GetTaskList(req map[string]interface{}) map[string]interface{} {
	jobTaskList, err := mapper.GetJobTaskList(req)
	if err != nil {
		panic(err.Error())
	}

	dataList, ok := jobTaskList["dataList"].([]map[string]interface{})
	if !ok {
		return jobTaskList
	}

	var needUpdateList []map[string]interface{}
	missingTaskItems := make([]map[string]interface{}, 0)
	missingTaskIDs := make([]int64, 0)
	for _, item := range dataList {
		var taskNum map[string]interface{}
		taskNumStr, hasTaskNum := item["taskNum"]
		if hasTaskNum && taskNumStr != nil {
			taskNum = parseTaskNumJSON(taskNumStr)
			if taskNum == nil {
				taskID := util.ToInt64(item["id"])
				missingTaskIDs = append(missingTaskIDs, taskID)
				missingTaskItems = append(missingTaskItems, item)
			}
		} else {
			taskID := util.ToInt64(item["id"])
			missingTaskIDs = append(missingTaskIDs, taskID)
			missingTaskItems = append(missingTaskItems, item)
		}
		if taskNum != nil {
			for k, v := range taskNum {
				item[k] = v
			}
		}
	}

	if len(missingTaskItems) > 0 {
		taskNumByID := mapper.GetJobTaskCountsByTaskIDs(missingTaskIDs)
		for _, item := range missingTaskItems {
			taskID := util.ToInt64(item["id"])
			taskNum := taskNumByID[taskID]
			if taskNum == nil {
				taskNum = mapper.EmptyJobTaskCounts()
			}
			for k, v := range taskNum {
				item[k] = v
			}
			if util.ToInt(item["status"]) > 1 {
				taskNumJSON, _ := json.Marshal(taskNum)
				needUpdateList = append(needUpdateList, map[string]interface{}{
					"taskId":  item["id"],
					"taskNum": string(taskNumJSON),
				})
			}
		}
	}

	if len(needUpdateList) > 0 {
		scheduleTaskNumUpdate(needUpdateList)
	}

	return jobTaskList
}

func parseTaskNumJSON(value interface{}) map[string]interface{} {
	var raw []byte
	switch v := value.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		raw = []byte(v)
	case []byte:
		if len(v) == 0 {
			return nil
		}
		raw = v
	default:
		text := strings.TrimSpace(fmt.Sprintf("%v", value))
		if text == "" {
			return nil
		}
		raw = []byte(text)
	}
	var taskNum map[string]interface{}
	if err := json.Unmarshal(raw, &taskNum); err != nil {
		return nil
	}
	return taskNum
}

func scheduleTaskNumUpdate(taskNums []map[string]interface{}) {
	// Merge into a pending map instead of dropping when a previous worker is
	// still running: each task keeps only its latest value, and the running
	// worker drains the merged set on its next pass.
	pendingTaskNumUpdatesMu.Lock()
	for _, tn := range taskNums {
		if tn == nil {
			continue
		}
		taskID := util.ToInt64(tn["taskId"])
		if taskID <= 0 {
			continue
		}
		pendingTaskNumUpdates[taskID] = cloneTaskRows([]map[string]interface{}{tn})[0]
	}
	pendingTaskNumUpdatesMu.Unlock()

	select {
	case taskNumUpdateSlots <- struct{}{}:
		go func() {
			defer func() {
				<-taskNumUpdateSlots
			}()
			drainTaskNumUpdates()
		}()
	default:
		// A worker is already running and will pick up the merged pending
		// updates on its next drain, so nothing is lost here.
	}
}

func drainTaskNumUpdates() {
	for {
		pendingTaskNumUpdatesMu.Lock()
		if len(pendingTaskNumUpdates) == 0 {
			pendingTaskNumUpdatesMu.Unlock()
			pendingTaskNumUpdatesMu.Lock()
			stillEmpty := len(pendingTaskNumUpdates) == 0
			pendingTaskNumUpdatesMu.Unlock()
			if stillEmpty {
				return
			}
			continue
		}
		batch := make([]map[string]interface{}, 0, len(pendingTaskNumUpdates))
		for taskID, tn := range pendingTaskNumUpdates {
			item := cloneTaskRows([]map[string]interface{}{tn})[0]
			item["taskId"] = taskID
			batch = append(batch, item)
			delete(pendingTaskNumUpdates, taskID)
		}
		pendingTaskNumUpdatesMu.Unlock()

		if err := mapper.UpdateJobTaskNumMany(batch); err != nil {
			log.Printf("Failed to update task counts: %v", err)
		}
	}
}

func GetTaskItemList(req map[string]interface{}) map[string]interface{} {
	result, err := mapper.GetJobTaskItemList(req)
	if err != nil {
		panic(err.Error())
	}
	return result
}

// RemoveTask deletes a task
func RemoveTask(taskID int64) {
	if isTaskCurrentlyRunning(taskID) {
		panicPublic(msg.TaskRunningCannotDelete)
	}
	if err := mapper.DeleteJobTaskByTaskID(taskID); err != nil {
		panic(err.Error())
	}
}

func isTaskCurrentlyRunning(taskID int64) bool {
	jobClientListMu.RLock()
	defer jobClientListMu.RUnlock()
	for _, client := range jobClientList {
		if client.isCurrentTask(taskID) {
			return true
		}
	}
	return false
}
