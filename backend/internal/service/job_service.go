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

var taskNumUpdateSlots = make(chan struct{}, 1)

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
	cutoff, ok = clampRetentionCutoffForActiveTasks(logger, cutoff)
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
	const secondsPerDay int64 = 86400
	days := int64(taskSaveDays)
	if days > math.MaxInt64/secondsPerDay {
		return 0, false
	}
	offset := days * secondsPerDay
	nowUnix := now.Unix()
	if offset >= nowUnix {
		return 0, false
	}
	return nowUnix - offset, true
}

func collectActiveProtectedTaskIDs() (activeTasks []*JobTask, retrySourceIDs []int64) {
	jobClientListMu.RLock()
	clients := make([]*JobClient, 0, len(jobClientList))
	for _, client := range jobClientList {
		clients = append(clients, client)
	}
	jobClientListMu.RUnlock()

	for _, client := range clients {
		task := client.currentTask()
		if task == nil {
			continue
		}
		activeTasks = append(activeTasks, task)
		if task.RetrySourceTaskID > 0 {
			retrySourceIDs = append(retrySourceIDs, task.RetrySourceTaskID)
		}
	}
	return activeTasks, retrySourceIDs
}

func isTaskIDProtected(taskID int64) bool {
	if taskID <= 0 {
		return false
	}
	activeTasks, retrySourceIDs := collectActiveProtectedTaskIDs()
	for _, task := range activeTasks {
		if task.TaskID == taskID {
			return true
		}
	}
	for _, id := range retrySourceIDs {
		if id == taskID {
			return true
		}
	}
	return false
}

// clampRetentionCutoffForActiveTasks ensures retention cleanup never deletes a
// currently running/stopping task or a historical task actively being replayed
// as RetrySourceTaskID.
func clampRetentionCutoffForActiveTasks(logger *log.Logger, cutoff int64) (int64, bool) {
	activeTasks, retrySourceIDs := collectActiveProtectedTaskIDs()
	lookupIDs := make([]int64, 0, len(activeTasks)+len(retrySourceIDs))
	for _, task := range activeTasks {
		if task.RunTime > 0 {
			if task.RunTime <= cutoff {
				cutoff = task.RunTime - 1
			}
		} else if task.TaskID > 0 {
			lookupIDs = append(lookupIDs, task.TaskID)
		}
	}
	lookupIDs = append(lookupIDs, retrySourceIDs...)

	for _, id := range lookupIDs {
		row, err := mapper.GetJobTaskByID(id)
		if err != nil {
			if err.Error() == msg.TaskNotFound {
				continue
			}
			logger.Printf("Skipping task retention cleanup because protected task %d could not be queried: %v", id, err)
			return 0, false
		}
		runTime := util.ToInt64(row["runTime"])
		if runTime > 0 && runTime <= cutoff {
			cutoff = runTime - 1
		}
	}
	if cutoff <= 0 {
		return 0, false
	}
	return cutoff, true
}

// getJobClientIfExists returns a cached job client without hitting the DB or panicking.
func getJobClientIfExists(jobID int64) (*JobClient, bool) {
	jobClientListMu.RLock()
	client, ok := jobClientList[jobID]
	jobClientListMu.RUnlock()
	if !ok || client == nil || client.isRemoved() {
		return nil, false
	}
	return client, true
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
		// Preserve leading/trailing spaces on srcPath/dstPath components; they are
		// sanitized separately by normalizePathListForStorage below.
		if key == "srcPath" || key == "dstPath" {
			continue
		}
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
	rawSrcs := parseRawPathList(job["srcPath"])
	rawDsts := parseRawPathList(job["dstPath"])
	if len(rawSrcs) == 0 ||
		len(rawDsts) == 0 ||
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
	if method == 2 && len(rawDsts) > 1 {
		panicPublic("移动模式不支持多个目标路径")
	}

	isCron := util.ToInt(job["isCron"])
	if isCron < 0 || isCron > 2 {
		panicPublic(msg.LostPart)
	}
	if isCron == 0 && util.ToInt(job["interval"]) <= 0 {
		panicPublic(msg.IntervalLost)
	}

	validateJobPaths(rawSrcs, rawDsts)
}

func validateJobPaths(rawSrcs, rawDsts []string) {
	canonSrcs := make([]string, len(rawSrcs))
	for i, src := range rawSrcs {
		canonSrcs[i] = canonicalJobPath(src)
	}
	for i := 0; i < len(canonSrcs); i++ {
		for j := i + 1; j < len(canonSrcs); j++ {
			if canonSrcs[i] == canonSrcs[j] {
				panicPublic("源路径不能重复")
			}
			if pathsOverlap(canonSrcs[i], canonSrcs[j]) {
				panicPublic("源路径不能包含父子目录重叠")
			}
		}
	}

	canonDsts := make([]string, len(rawDsts))
	for i, dst := range rawDsts {
		canonDsts[i] = canonicalJobPath(dst)
	}
	for i := 0; i < len(canonDsts); i++ {
		for j := i + 1; j < len(canonDsts); j++ {
			if canonDsts[i] == canonDsts[j] {
				panicPublic("目标路径不能重复")
			}
			if pathsOverlap(canonDsts[i], canonDsts[j]) {
				panicPublic("目标路径不能包含父子目录重叠")
			}
		}
	}

	for _, src := range canonSrcs {
		for _, dst := range canonDsts {
			if pathsOverlap(src, dst) {
				panicPublic("源路径与目标路径不能相同或互相包含")
			}
			mappedDst := canonicalJobPath(dstPathForSrcSelection(dst, src, canonSrcs))
			for _, otherSrc := range canonSrcs {
				if pathsOverlap(otherSrc, mappedDst) {
					panicPublic("源路径与目标路径不能相同或互相包含")
				}
			}
		}
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
	if !isInit {
		ValidateJobInput(job)
		// Interactive add: validate that the engine exists and hold the
		// reference lock until the job row is inserted, so a concurrent
		// RemoveClient cannot delete the engine in between and leave a job
		// pointing at nothing.
		alistRefMu.Lock()
		defer alistRefMu.Unlock()
		if _, err := getAlistByID(util.ToInt64(job["alistId"])); err != nil {
			panicPublicIf(err, msg.AlistNotFound)
		}
	}
	client := NewJobClient(job, isInit)
	jobClientListMu.Lock()
	jobClientList[client.idSnapshot()] = client
	jobClientListMu.Unlock()
}

// EditJobClient updates an existing job client
func EditJobClient(job map[string]interface{}) {
	jobID := util.ToInt64(job["id"])
	CleanJobInput(job)
	ValidateJobInput(job)
	if err := validateJobSchedule(util.ToInt(job["isCron"]), job); err != nil {
		panicPublic(err.Error())
	}

	client := GetJobClientByID(jobID)
	var oldScheduler *Scheduler
	func() {
		client.opMu.Lock()
		defer client.opMu.Unlock()

		if client.isRemoved() {
			panicPublic(msg.JobNotFound)
		}
		oldJob := client.jobSnapshot()

		alistRefMu.Lock()
		if _, err := getAlistByID(util.ToInt64(job["alistId"])); err != nil {
			alistRefMu.Unlock()
			panicPublicIf(err, msg.AlistNotFound)
		}
		if oldJob != nil {
			oldAlistID := util.ToInt64(oldJob["alistId"])
			newAlistID := util.ToInt64(job["alistId"])
			if oldAlistID > 0 && oldAlistID != newAlistID {
				// Tag any pre-migration historical tasks with the previous engine
				// before switching job.alistId so old failed deletes cannot be
				// replayed against the new engine.
				if err := mapper.BackfillJobTaskAlistID(jobID, oldAlistID); err != nil {
					alistRefMu.Unlock()
					panic(err.Error())
				}
			}
		}
		if err := mapper.UpdateJob(job); err != nil {
			alistRefMu.Unlock()
			panicPublicIf(err, msg.JobNotFound)
		}
		alistRefMu.Unlock()

		nextScheduler := NewScheduler()
		swapped := false
		defer func() {
			if r := recover(); r != nil {
				if !swapped {
					nextScheduler.Stop()
					if oldJob != nil {
						if err := mapper.UpdateJob(oldJob); err != nil {
							log.Printf("failed to roll back job %d after edit panic: %v", jobID, err)
						}
					}
				}
				panic(r)
			}
		}()

		if err := nextScheduler.AddJob(util.ToInt(job["isCron"]), job, func() {
			client.DoScheduled()
		}); err != nil {
			nextScheduler.Stop()
			if oldJob != nil {
				if rbErr := mapper.UpdateJob(oldJob); rbErr != nil {
					log.Printf("failed to roll back job %d after schedule error: %v", jobID, rbErr)
				}
			}
			panicPublic(err.Error())
		}

		oldScheduler = client.replaceJobConfig(job, nextScheduler)
		swapped = true
	}()

	if oldScheduler != nil {
		oldScheduler.Stop()
	}
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
	started := 0
	busy := 0
	for _, jobItem := range jobList {
		jobID := util.ToInt64(jobItem["id"])
		var client *JobClient
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("Skipping job %d in DoAllJobManual: %v", jobID, r)
				}
			}()
			client = GetJobClientByID(jobID)
		}()
		if client == nil {
			continue
		}
		if err := client.tryStartManualRun(); err != nil {
			if err.Error() == msg.JobRunning {
				busy++
			}
			continue
		}
		started++
		go client.runMarkedJob()
	}
	if started == 0 {
		if busy > 0 {
			panicPublic(msg.JobRunning)
		}
		panicPublic(msg.NoJobForRun)
	}
}

// DoJobManual executes a specific job manually
func DoJobManual(jobID int64) {
	client := GetJobClientByID(jobID)
	client.DoManual()
}

// RemoveJobClient deletes a job
func RemoveJobClient(jobID int64) {
	client := GetJobClientByID(jobID)
	var schedToStop *Scheduler
	func() {
		client.opMu.Lock()
		defer client.opMu.Unlock()

		sched, err := client.beginRemove()
		if err != nil {
			panicPublic(err.Error())
		}
		if sched != nil {
			sched.Pause()
		}
		resumeOnCancel := func() {
			client.cancelRemove()
			if sched != nil && client.enabled() {
				job := client.jobSnapshot()
				_ = sched.Resume(util.ToInt(job["isCron"]), job, func() {
					client.DoScheduled()
				})
			}
		}
		if !client.waitUntilIdle(2 * time.Minute) {
			resumeOnCancel()
			panicPublic(msg.JobDeleteWaitTimeout)
		}
		if err := mapper.DeleteJob(jobID); err != nil {
			resumeOnCancel()
			panicPublicIf(err, msg.JobNotFound)
		}
		client.finishRemove()
		jobClientListMu.Lock()
		if current, ok := jobClientList[jobID]; ok && current == client {
			delete(jobClientList, jobID)
		}
		jobClientListMu.Unlock()
		schedToStop = sched
	}()
	if schedToStop != nil {
		schedToStop.Stop()
	}
}

// ContinueJob enables a job
func ContinueJob(jobID int64) {
	client := GetJobClientByID(jobID)
	client.opMu.Lock()
	defer client.opMu.Unlock()
	if client.isRemoved() {
		panicPublic(msg.JobNotFound)
	}
	client.ResumeJob()
}

// PauseJob disables a job
func PauseJob(jobID int64) {
	client := GetJobClientByID(jobID)
	client.opMu.Lock()
	defer client.opMu.Unlock()
	if client.isRemoved() {
		panicPublic(msg.JobNotFound)
	}
	if util.ToInt(client.jobSnapshot()["isCron"]) == 2 {
		panicPublic(msg.CannotDisableManualJob)
	}
	client.StopJob(false)
}

// AbortJob aborts a running job
func AbortJob(jobID int64) {
	client := GetJobClientByID(jobID)
	client.opMu.Lock()
	defer client.opMu.Unlock()
	if client.isRemoved() {
		panicPublic(msg.JobNotFound)
	}
	client.AbortJob()
}

// StopTask stops a currently running task without changing the job schedule.
func StopTask(taskID int64) {
	job, err := mapper.GetJobByTaskID(taskID)
	if err != nil {
		panicPublicIf(err, msg.JobNotFound)
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
	sourceTask, err := mapper.GetJobTaskByID(taskID)
	if err != nil {
		panicPublicIf(err, msg.TaskNotFound)
	}
	client := GetJobClientByID(util.ToInt64(sourceTask["jobId"]))
	client.opMu.Lock()
	defer client.opMu.Unlock()

	if client.isRemoved() {
		panicPublic(msg.JobNotFound)
	}
	if !client.enabled() {
		panicPublic(msg.DisabledJobCannotRun)
	}
	if client.isBusy() {
		panicPublic(msg.JobRunning)
	}
	sourceAlistID := util.ToInt64(sourceTask["alistId"])
	currentAlistID := util.ToInt64(client.jobSnapshot()["alistId"])
	if sourceAlistID > 0 && sourceAlistID != currentAlistID {
		panicPublic(msg.RetryTaskEngineChanged)
	}
	count, err := countJobTaskItemsByStatuses(taskID, retryableStatusValues())
	if err != nil {
		panic(err.Error())
	}
	if count == 0 {
		panicPublic(msg.NoFailedTaskItems)
	}
	client.DoRetryFailedTaskItemsWithAlist(taskID, sourceAlistID, currentAlistID)
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
	if taskClient != nil && !taskClient.finished.Load() {
		status, hasStatus := params["status"]
		if !hasStatus || fmt.Sprintf("%v", status) == "" {
			return taskClient.getCurrentPayload()
		}
		statusInt := util.ToInt(status)
		pageSize := util.ToInt(params["pageSize"])
		pageNum := util.ToInt(params["pageNum"])
		if pageSize > 0 && pageNum > 0 {
			return taskClient.GetCurrentByStatusPage(statusInt, pageSize, pageNum)
		}
		return taskClient.GetCurrentByStatus(statusInt)
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
		taskNumByID, err := mapper.GetJobTaskCountsByTaskIDsWithError(missingTaskIDs)
		if err != nil {
			log.Printf("Failed to backfill task counts: %v", err)
			return jobTaskList
		}
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
	taskNums = cloneTaskRows(taskNums)
	select {
	case taskNumUpdateSlots <- struct{}{}:
		go func() {
			defer func() {
				<-taskNumUpdateSlots
			}()
			if err := mapper.UpdateJobTaskNumMany(taskNums); err != nil {
				log.Printf("Failed to update task counts: %v", err)
			}
		}()
	default:
		log.Printf("Skipping task count backfill because a previous update is still running")
	}
}

func GetTaskItemList(ctx context.Context, req map[string]interface{}) map[string]interface{} {
	result, err := mapper.GetJobTaskItemListContext(ctx, req)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		panic(err.Error())
	}
	return result
}

// RemoveTask deletes a task
func RemoveTask(taskID int64) {
	if isTaskIDProtected(taskID) {
		panicPublic(msg.JobRunningCannotDelete)
	}
	if err := mapper.DeleteJobTaskByTaskID(taskID); err != nil {
		panicPublicIf(err, msg.TaskNotFound, msg.JobRunningCannotDelete)
	}
}
