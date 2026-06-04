package service

import (
	"encoding/json"
	"fmt"
	"log"
	"opensync/internal/config"
	"opensync/internal/i18n"
	"opensync/internal/mapper"
	"strings"
	"sync"
	"time"
)

var (
	jobClientList   = make(map[int64]*JobClient)
	jobClientListMu sync.RWMutex
)

// InitJobs loads and starts all enabled jobs on startup
func InitJobs() {
	logger := log.Default()
	mapper.UpdateJobTaskStatusByStatus()
	CleanupExpiredTasks(logger, config.GetConfig().Server.TaskSave, time.Now())
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
		panic(err.Error())
	}
	client = NewJobClient(job, false)
	jobClientList[jobID] = client
	return client
}

// CleanJobInput sanitizes job input data
func CleanJobInput(job map[string]interface{}) {
	if toInt(job["isCron"]) == 2 && toInt(job["enable"]) != 1 {
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
		job["srcPath"] = normalizeSrcPathForStorage(job["srcPath"])
	}
	if job["dstPath"] != nil {
		job["dstPath"] = normalizeDstPathForStorage(job["dstPath"])
	}
}

// AddJobClient creates a new job client
func AddJobClient(job map[string]interface{}, isInit bool) {
	CleanJobInput(job)
	client := NewJobClient(job, isInit)
	jobClientListMu.Lock()
	jobClientList[client.JobID] = client
	jobClientListMu.Unlock()
}

// EditJobClient updates an existing job client
func EditJobClient(job map[string]interface{}) {
	jobID := toInt64(job["id"])
	CleanJobInput(job)
	client := GetJobClientByID(jobID)
	if client.enabled() && toInt(client.Job["isCron"]) != 2 {
		panic(i18n.G("disable_then_edit"))
	}
	newClient := NewJobClient(job, false)
	if err := mapper.UpdateJob(job); err != nil {
		newClient.StopJob(true)
		panic(err.Error())
	}
	client.StopJob(true)
	jobClientListMu.Lock()
	delete(jobClientList, jobID)
	jobClientListMu.Unlock()
	jobClientListMu.Lock()
	jobClientList[jobID] = newClient
	jobClientListMu.Unlock()
}

// DoAllJobManual executes all enabled jobs manually
func DoAllJobManual() {
	jobList, err := mapper.GetEnableJobList()
	if err != nil || len(jobList) == 0 {
		panic(i18n.G("no_job_for_run"))
	}
	for _, jobItem := range jobList {
		client := GetJobClientByID(toInt64(jobItem["id"]))
		if client.enabled() {
			client.DoManual()
		}
	}
}

// DoJobManual executes a specific job manually
func DoJobManual(jobID int64) {
	client := GetJobClientByID(jobID)
	if !client.enabled() {
		panic(i18n.G("disabled_job_cannot_run"))
	}
	client.DoManual()
}

// RemoveJobClient deletes a job
func RemoveJobClient(jobID int64) {
	client := GetJobClientByID(jobID)
	client.StopJob(true)
	if !client.waitUntilIdle(2 * time.Minute) {
		panic(i18n.G("job_delete_wait_timeout"))
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
	if toInt(client.Job["isCron"]) == 2 {
		panic(i18n.G("cannot_disable_manual_job"))
	}
	client.StopJob(false)
}

// AbortJob aborts a running job
func AbortJob(jobID int64) {
	client := GetJobClientByID(jobID)
	client.AbortJob()
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
func GetJobCurrent(jobID int64, status *string) interface{} {
	client := GetJobClientByID(jobID)
	taskClient := client.currentTask()
	if taskClient != nil {
		if status == nil || *status == "" {
			return taskClient.GetCurrent()
		}
		statusInt := toInt(*status)
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
	for _, item := range dataList {
		var taskNum map[string]interface{}
		taskNumStr, hasTaskNum := item["taskNum"]
		if hasTaskNum && taskNumStr != nil && fmt.Sprintf("%v", taskNumStr) != "" {
			json.Unmarshal([]byte(fmt.Sprintf("%v", taskNumStr)), &taskNum)
		} else {
			taskNum = GetCuTaskNum(toInt64(item["id"]))
			if toInt(item["status"]) > 1 {
				taskNumJSON, _ := json.Marshal(taskNum)
				needUpdateList = append(needUpdateList, map[string]interface{}{
					"taskId":  item["id"],
					"taskNum": string(taskNumJSON),
				})
			}
		}
		if taskNum != nil {
			for k, v := range taskNum {
				item[k] = v
			}
		}
	}

	if len(needUpdateList) > 0 {
		go mapper.UpdateJobTaskNumMany(needUpdateList)
	}

	return jobTaskList
}

// GetTaskItemList returns paginated task item list
func GetTaskItemList(req map[string]interface{}) map[string]interface{} {
	result, err := mapper.GetJobTaskItemList(req)
	if err != nil {
		panic(err.Error())
	}
	return result
}

// RemoveTask deletes a task
func RemoveTask(taskID int64) {
	if err := mapper.DeleteJobTaskByTaskID(taskID); err != nil {
		panic(err.Error())
	}
}
