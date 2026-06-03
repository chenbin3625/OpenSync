package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"taosync/internal/i18n"
	"taosync/internal/mapper"
	"time"

	ignore "github.com/sabhiram/go-gitignore"
)

const maxScanConcurrency = 8

var errScanAborted = errors.New("scan aborted")

// CopyItem represents a single file copy operation
type CopyItem struct {
	SrcPath     string
	DstPath     string
	FileName    string
	FileSize    interface{}
	CopyType    int // 0=copy, 2=move
	AlistTaskID string
	Status      int
	Progress    float64
	ErrMsg      *string
	CreateTime  int64
	DoingKey    int64

	jobTask     *JobTask
	alistClient *AlistClient
}

// DoIt executes the copy operation in a goroutine
func (ci *CopyItem) DoIt() {
	if ci.jobTask.isBreak() {
		ci.Status = 4
	} else {
		taskID, err := ci.alistClient.CopyFile(ci.SrcPath, ci.DstPath, ci.FileName)
		if err != nil {
			errMsg := err.Error()
			ci.ErrMsg = &errMsg
			ci.Status = 7
		} else {
			ci.AlistTaskID = taskID
			if taskID == "" {
				ci.Status = 2
			} else if ci.Status != 4 {
				ci.checkAndGetStatus()
			}
		}
	}
	ci.endIt()
}

func (ci *CopyItem) checkAndGetStatus() {
	for {
		if ci.jobTask.isBreak() {
			ci.Status = 4
			if ci.AlistTaskID != "" {
				if err := ci.alistClient.CopyTaskCancel(ci.AlistTaskID); err != nil {
					errMsg := err.Error()
					ci.Status = 7
					ci.ErrMsg = &errMsg
				}
				ci.alistClient.CopyTaskDelete(ci.AlistTaskID)
			}
			break
		}

		cuTime := time.Now().Unix()
		if cuTime-ci.jobTask.LastWatching.Load() < 3 {
			time.Sleep(610 * time.Millisecond)
		} else {
			time.Sleep(2930 * time.Millisecond)
		}

		taskInfo, err := ci.alistClient.TaskInfo(ci.AlistTaskID)
		if err != nil {
			eMsg := err.Error()
			if strings.Contains(eMsg, "404") {
				eMsg = i18n.G("task_may_delete")
			}
			ci.Status = 7
			ci.Progress = 0
			ci.ErrMsg = &eMsg
			break
		}

		state := toInt(taskInfo["state"])
		progress := toFloat64(taskInfo["progress"])
		errStr := ""
		if e, ok := taskInfo["error"]; ok && e != nil {
			errStr = fmt.Sprintf("%v", e)
		}

		if state == ci.Status && progress == ci.Progress {
			continue
		}
		ci.Status = state
		ci.Progress = progress
		if errStr != "" {
			ci.ErrMsg = &errStr
		} else {
			ci.ErrMsg = nil
		}

		if state == 2 || state == 4 || state == 7 {
			ci.alistClient.CopyTaskDelete(ci.AlistTaskID)
			break
		}
	}
}

func (ci *CopyItem) endIt() {
	if ci.CopyType == 2 && ci.Status == 2 {
		scanIntervalS := toInt(ci.jobTask.Job["scanIntervalS"])
		err := ci.alistClient.DeleteFile(ci.SrcPath, []string{ci.FileName}, scanIntervalS)
		if err != nil {
			ci.Status = 7
			errMsg := strings.Replace(i18n.G("copy_success_but_delete_fail"), "{}", err.Error(), 1)
			ci.ErrMsg = &errMsg
		}
	}
	ci.jobTask.CopyHook(ci.SrcPath, ci.DstPath, ci.FileName, ci.FileSize, ci.AlistTaskID,
		ci.Status, ci.ErrMsg, 0, ci.CopyType, ci.CreateTime)
	ci.jobTask.DoingMu.Lock()
	delete(ci.jobTask.Doing, ci.DoingKey)
	ci.jobTask.DoingMu.Unlock()
}

// JobTask represents a running task with sync engine
type JobTask struct {
	TaskID      int64
	JobClient   *JobClient
	Job         map[string]interface{}
	AlistClient *AlistClient
	CreateTime  float64

	Finish    []map[string]interface{}
	FinishMu  sync.Mutex
	Doing     map[int64]*CopyItem
	DoingMu   sync.Mutex
	Waiting   []*CopyItem
	WaitingMu sync.Mutex

	LastWatching atomic.Int64
	QueueNum     int64
	ScanFinish   atomic.Bool
	FirstSync    atomic.Int64
	BreakFlag    atomic.Bool
	scanSem      chan struct{}

	CurrentTasks map[int][]map[string]interface{}
	CurrentMu    sync.RWMutex
}

// NewJobTask creates and starts a new task
func NewJobTask(taskID int64, jc *JobClient) *JobTask {
	jt := &JobTask{
		TaskID:       taskID,
		JobClient:    jc,
		Job:          jc.Job,
		AlistClient:  GetClientByID(toInt64(jc.Job["alistId"])),
		CreateTime:   float64(time.Now().Unix()),
		Finish:       make([]map[string]interface{}, 0),
		Doing:        make(map[int64]*CopyItem),
		Waiting:      make([]*CopyItem, 0),
		QueueNum:     0,
		scanSem:      make(chan struct{}, scanConcurrencyLimit()),
		CurrentTasks: make(map[int][]map[string]interface{}),
	}

	go jt.sync()
	go jt.taskSubmit()

	return jt
}

func scanConcurrencyLimit() int {
	limit := runtime.NumCPU()
	if limit < 2 {
		return 2
	}
	if limit > maxScanConcurrency {
		return maxScanConcurrency
	}
	return limit
}

func (jt *JobTask) isBreak() bool {
	return jt.BreakFlag.Load()
}

func (jt *JobTask) requestBreak() {
	jt.BreakFlag.Store(true)
}

func (jt *JobTask) acquireScanSlot() bool {
	for {
		if jt.isBreak() {
			return false
		}
		select {
		case jt.scanSem <- struct{}{}:
			if jt.isBreak() {
				<-jt.scanSem
				return false
			}
			return true
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func (jt *JobTask) releaseScanSlot() {
	<-jt.scanSem
}

// GetCurrent returns real-time task progress
func (jt *JobTask) GetCurrent() map[string]interface{} {
	now := time.Now().Unix()
	jt.LastWatching.Store(now)

	jt.WaitingMu.Lock()
	waits := make([]map[string]interface{}, len(jt.Waiting))
	for i, w := range jt.Waiting {
		waits[i] = jt.copyItemToMap(w)
	}
	jt.WaitingMu.Unlock()

	jt.DoingMu.Lock()
	dos := make([]map[string]interface{}, 0, len(jt.Doing))
	for _, d := range jt.Doing {
		dos = append(dos, jt.copyItemToMap(d))
	}
	jt.DoingMu.Unlock()

	allTask := make([]map[string]interface{}, 0)
	allTask = append(allTask, waits...)
	allTask = append(allTask, dos...)
	jt.FinishMu.Lock()
	allTask = append(allTask, jt.Finish...)
	jt.FinishMu.Unlock()

	keyValSpace := map[string]int{
		"wait":    0,
		"running": 1,
		"success": 2,
		"fail":    7,
		"other":   -1,
	}

	currentTasks := make(map[int][]map[string]interface{})
	for _, v := range keyValSpace {
		currentTasks[v] = make([]map[string]interface{}, 0)
	}

	otkStatus := map[int]bool{3: true, 4: true, 5: true, 6: true, 8: true, 9: true}
	otk := make([]map[string]interface{}, 0)
	grouped := make(map[int][]map[string]interface{})
	for _, task := range allTask {
		s := task["status"].(int)
		grouped[s] = append(grouped[s], task)
	}
	for status, tasks := range grouped {
		if otkStatus[status] {
			otk = append(otk, tasks...)
		} else {
			currentTasks[status] = tasks
		}
	}
	currentTasks[-1] = otk
	jt.CurrentMu.Lock()
	jt.CurrentTasks = currentTasks
	jt.CurrentMu.Unlock()

	result := map[string]interface{}{
		"scanFinish": jt.ScanFinish.Load(),
		"doingTask":  currentTasks[1],
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
	for key, val := range keyValSpace {
		tasks := currentTasks[val]
		numMap[key] = len(tasks)
		var totalSize int64
		for _, t := range tasks {
			if t["fileSize"] != nil && t["type"].(int) != 1 {
				totalSize += toInt64Val(t["fileSize"])
			}
		}
		sizeMap[key] = totalSize
	}
	return result
}

// GetCurrentByStatus returns tasks filtered by status
func (jt *JobTask) GetCurrentByStatus(status int) []map[string]interface{} {
	jt.CurrentMu.RLock()
	defer jt.CurrentMu.RUnlock()
	if tasks, ok := jt.CurrentTasks[status]; ok {
		return append([]map[string]interface{}(nil), tasks...)
	}
	return []map[string]interface{}{}
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
	return map[string]interface{}{
		"srcPath":    item.SrcPath,
		"dstPath":    item.DstPath,
		"isPath":     0,
		"fileName":   item.FileName,
		"fileSize":   item.FileSize,
		"status":     item.Status,
		"type":       item.CopyType,
		"progress":   item.Progress,
		"errMsg":     item.ErrMsg,
		"createTime": item.CreateTime,
	}
}

func (jt *JobTask) taskSubmit() {
	for {
		if jt.isBreak() {
			break
		}
		time.Sleep(500 * time.Millisecond)

		jt.DoingMu.Lock()
		doingNums := len(jt.Doing)
		jt.DoingMu.Unlock()

		jt.WaitingMu.Lock()
		waitingNums := len(jt.Waiting)
		jt.WaitingMu.Unlock()

		if !jt.ScanFinish.Load() || doingNums != 0 || waitingNums != 0 {
			for doingNums < 20 {
				if jt.isBreak() {
					break
				}
				jt.WaitingMu.Lock()
				if len(jt.Waiting) == 0 {
					jt.WaitingMu.Unlock()
					break
				}
				if jt.FirstSync.Load() == 0 {
					jt.FirstSync.CompareAndSwap(0, time.Now().Unix())
				}
				jt.QueueNum++
				item := jt.Waiting[0]
				jt.Waiting[0] = nil
				jt.Waiting = jt.Waiting[1:]
				jt.WaitingMu.Unlock()

				item.DoingKey = jt.QueueNum
				jt.DoingMu.Lock()
				jt.Doing[jt.QueueNum] = item
				jt.DoingMu.Unlock()

				go item.DoIt()

				jt.DoingMu.Lock()
				doingNums = len(jt.Doing)
				jt.DoingMu.Unlock()
			}
		} else {
			break
		}
	}

	// Wait for remaining doing tasks
	tryTime := 0
	for {
		jt.DoingMu.Lock()
		dLen := len(jt.Doing)
		jt.DoingMu.Unlock()
		if dLen == 0 {
			break
		}
		tryTime++
		time.Sleep(500 * time.Millisecond)
		if tryTime > 3 {
			break
		}
	}

	// Batch save finish items
	jt.FinishMu.Lock()
	finish := append([]map[string]interface{}(nil), jt.Finish...)
	jt.FinishMu.Unlock()
	if len(finish) > 0 {
		mapper.AddJobTaskItemMany(finish)
	}

	jt.updateTaskStatus()
	jt.JobClient.JobDoing = false
	jt.JobClient.CurrentJobTask = nil
}

// CopyHook is called when a copy operation completes
func (jt *JobTask) CopyHook(srcPath, dstPath, fileName string, fileSize interface{},
	alistTaskID string, status int, errMsg *string, isPath, copyType int, createTime int64) {
	jt.FinishMu.Lock()
	defer jt.FinishMu.Unlock()
	jt.Finish = append(jt.Finish, map[string]interface{}{
		"taskId":      jt.TaskID,
		"srcPath":     srcPath,
		"dstPath":     dstPath,
		"isPath":      isPath,
		"fileName":    fileName,
		"fileSize":    fileSize,
		"type":        copyType,
		"alistTaskId": alistTaskID,
		"status":      status,
		"errMsg":      errMsg,
		"createTime":  createTime,
	})
}

// DelHook is called when a delete operation completes
func (jt *JobTask) DelHook(dstPath, fileName string, fileSize interface{}, status int, errMsg *string, isPath int, createTime int64) {
	jt.FinishMu.Lock()
	defer jt.FinishMu.Unlock()
	jt.Finish = append(jt.Finish, map[string]interface{}{
		"taskId":      jt.TaskID,
		"srcPath":     nil,
		"dstPath":     dstPath,
		"isPath":      isPath,
		"fileName":    fileName,
		"fileSize":    fileSize,
		"type":        1,
		"alistTaskId": nil,
		"status":      status,
		"errMsg":      errMsg,
		"createTime":  createTime,
	})
}

func (jt *JobTask) sync() {
	srcPath := normalizeDirPath(fmt.Sprintf("%v", jt.Job["srcPath"]))
	jobExclude := jt.Job["exclude"]

	var spec *ignore.GitIgnore
	if jobExclude != nil {
		excludeStr := fmt.Sprintf("%v", jobExclude)
		if excludeStr != "" {
			patterns := strings.Split(excludeStr, ":")
			spec = ignore.CompileIgnoreLines(patterns...)
		}
	}

	dstPaths := strings.Split(fmt.Sprintf("%v", jt.Job["dstPath"]), ":")
	for i, dstItem := range dstPaths {
		dstItem = normalizeDirPath(dstItem)
		jt.syncWithHave(srcPath, dstItem, spec, srcPath, dstItem, i == 0)
	}
	jt.ScanFinish.Store(true)
}

func normalizeDirPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	return path
}

func (jt *JobTask) copyFile(srcPath, dstPath, fileName string, fileSize interface{}) {
	if jt.isBreak() {
		return
	}
	method := toInt(jt.Job["method"])
	copyType := 0
	if method >= 2 {
		copyType = 2
	}
	ci := &CopyItem{
		SrcPath:     srcPath,
		DstPath:     dstPath,
		FileName:    fileName,
		FileSize:    fileSize,
		CopyType:    copyType,
		Status:      0,
		Progress:    0,
		CreateTime:  time.Now().Unix(),
		jobTask:     jt,
		alistClient: jt.AlistClient,
	}
	jt.WaitingMu.Lock()
	jt.Waiting = append(jt.Waiting, ci)
	jt.WaitingMu.Unlock()
}

func (jt *JobTask) delFile(path, fileName string, size interface{}) {
	if jt.isBreak() {
		return
	}
	isPath := strings.HasSuffix(fileName, "/")
	status := 2
	var errMsg *string
	createTime := time.Now().Unix()

	name := fileName
	if isPath {
		name = fileName[:len(fileName)-1]
	}
	scanIntervalT := toInt(jt.Job["scanIntervalT"])
	err := jt.AlistClient.DeleteFile(path, []string{name}, scanIntervalT)
	if err != nil {
		status = 7
		e := err.Error()
		errMsg = &e
	}

	var delSize interface{}
	if !isPath {
		delSize = size
	}
	jt.DelHook(path, fileName, delSize, status, errMsg, boolToInt(isPath), createTime)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (jt *JobTask) listDir(path string, firstDst bool, spec *ignore.GitIgnore, rootPath string, isSrc bool) (map[string]interface{}, error) {
	var useCache int
	if isSrc && !firstDst {
		useCache = 1
	} else {
		if isSrc {
			useCache = toInt(jt.Job["useCacheS"])
		} else {
			useCache = toInt(jt.Job["useCacheT"])
		}
	}

	var scanInterval int
	if isSrc {
		scanInterval = toInt(jt.Job["scanIntervalS"])
	} else {
		scanInterval = toInt(jt.Job["scanIntervalT"])
	}

	if !jt.acquireScanSlot() {
		return nil, errScanAborted
	}
	result, err := jt.AlistClient.FileListApi(path, useCache, scanInterval)
	jt.releaseScanSlot()
	if err != nil {
		srcOrDst := i18n.G("src")
		if !isSrc {
			srcOrDst = i18n.G("dst")
		}
		errMsg := strings.Replace(i18n.G("scan_error"), "{}", srcOrDst, 1)
		errMsg = strings.Replace(errMsg, "{}", err.Error(), 1)
		log.Printf("%s", errMsg)

		jt.CopyHook(pathIfTrue(isSrc, path), pathIfTrue(!isSrc, path), "", nil, "", 7, &errMsg, 1, 0, time.Now().Unix())
		return nil, err
	}

	// Apply exclude rules
	if spec != nil && len(result) > 0 {
		filtered := make(map[string]interface{})
		for key, val := range result {
			checkPath := path[len(rootPath):] + "/" + key
			if !spec.MatchesPath(checkPath) {
				filtered[key] = val
			}
		}
		return filtered, nil
	}

	return result, nil
}

func pathIfTrue(cond bool, path string) string {
	if cond {
		return path
	}
	return ""
}

func (jt *JobTask) listSrcAndDst(srcPath, dstPath string, spec *ignore.GitIgnore, srcRootPath, dstRootPath string, firstDst bool) (map[string]interface{}, map[string]interface{}, error) {
	var srcFiles, dstFiles map[string]interface{}
	var srcErr, dstErr error

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		srcFiles, srcErr = jt.listDir(srcPath, firstDst, spec, srcRootPath, true)
	}()
	go func() {
		defer wg.Done()
		dstFiles, dstErr = jt.listDir(dstPath, firstDst, spec, dstRootPath, false)
	}()
	wg.Wait()

	if srcErr != nil {
		return nil, nil, srcErr
	}
	if dstErr != nil {
		return nil, nil, dstErr
	}
	if srcFiles == nil {
		srcFiles = make(map[string]interface{})
	}
	if dstFiles == nil {
		dstFiles = make(map[string]interface{})
	}
	return srcFiles, dstFiles, nil
}

func (jt *JobTask) syncWithHave(srcPath, dstPath string, spec *ignore.GitIgnore, srcRootPath, dstRootPath string, firstDst bool) {
	if jt.isBreak() {
		return
	}

	srcFiles, dstFiles, err := jt.listSrcAndDst(srcPath, dstPath, spec, srcRootPath, dstRootPath, firstDst)
	if err != nil {
		return
	}

	var wg sync.WaitGroup
	for key, srcVal := range srcFiles {
		if !strings.HasSuffix(key, "/") {
			// File
			dstVal, exists := dstFiles[key]
			if !exists || dstVal != srcVal {
				jt.copyFile(srcPath, dstPath, key, srcVal)
			}
		} else {
			// Directory
			if _, exists := dstFiles[key]; !exists {
				wg.Add(1)
				go func(key string) {
					defer wg.Done()
					jt.syncWithoutHave(srcPath+key, dstPath+key, spec, srcRootPath, dstRootPath, firstDst)
				}(key)
			} else {
				wg.Add(1)
				go func(key string) {
					defer wg.Done()
					jt.syncWithHave(srcPath+key, dstPath+key, spec, srcRootPath, dstRootPath, firstDst)
				}(key)
			}
		}
	}
	wg.Wait()

	if toInt(jt.Job["method"]) == 1 {
		for dstKey, dstVal := range dstFiles {
			if _, exists := srcFiles[dstKey]; !exists {
				jt.delFile(dstPath, dstKey, dstVal)
			}
		}
	}
}

func (jt *JobTask) syncWithoutHave(srcPath, dstPath string, spec *ignore.GitIgnore, srcRootPath, dstRootPath string, firstDst bool) {
	if jt.isBreak() {
		return
	}

	status := 2
	var errMsg *string
	scanIntervalT := toInt(jt.Job["scanIntervalT"])
	err := jt.AlistClient.Mkdir(dstPath, scanIntervalT)
	if err != nil {
		status = 7
		e := err.Error()
		errMsg = &e
	}

	jt.CopyHook(srcPath, dstPath, "", nil, "", status, errMsg, 1, 0, time.Now().Unix())
	if status != 2 {
		return
	}

	srcFiles, err := jt.listDir(srcPath, firstDst, spec, srcRootPath, true)
	if err != nil {
		return
	}

	var wg sync.WaitGroup
	for key := range srcFiles {
		if jt.isBreak() {
			break
		}
		if strings.HasSuffix(key, "/") {
			wg.Add(1)
			go func(key string) {
				defer wg.Done()
				jt.syncWithoutHave(srcPath+key, dstPath+key, spec, srcRootPath, dstRootPath, firstDst)
			}(key)
		} else {
			jt.copyFile(srcPath, dstPath, key, srcFiles[key])
		}
	}
	wg.Wait()
}

func (jt *JobTask) updateTaskStatus() {
	jt.GetCurrent()
	currentTasks := jt.currentTasksSnapshot()
	failOrOtherNum := len(currentTasks[7]) + len(currentTasks[-1])
	status := 2
	if jt.isBreak() {
		status = 7
	} else if failOrOtherNum > 0 {
		status = 3
	}

	UpdateJobTaskStatusFinal(jt.TaskID, status, currentTasks, jt.CreateTime)
}

// JobClient manages a single job's lifecycle
type JobClient struct {
	JobID          int64
	Job            map[string]interface{}
	Scheduler      *Scheduler
	JobDoing       bool
	CurrentJobTask *JobTask
	mu             sync.Mutex
}

// NewJobClient creates a new job client
func NewJobClient(job map[string]interface{}, isInit bool) *JobClient {
	jc := &JobClient{
		Job: job,
	}

	addJobID := int64(0)
	if _, ok := job["enable"]; !ok {
		job["enable"] = 1
	}
	if _, ok := job["method"]; !ok {
		job["method"] = 0
	}

	if _, ok := job["id"]; !ok {
		id, err := mapper.AddJob(job)
		if err != nil {
			panic(err.Error())
		}
		addJobID = id
		var err2 error
		job, err2 = mapper.GetJobByID(id)
		if err2 != nil {
			panic(err2.Error())
		}
	}

	jc.JobID = toInt64(job["id"])
	jc.Job = job

	sched := NewScheduler()
	jc.Scheduler = sched

	err := sched.AddJob(toInt(job["isCron"]), job, jc.DoJob)
	if err != nil {
		if isInit || addJobID != 0 {
			log.Printf("Error during job setup, deleting job: %v", job)
			mapper.DeleteJob(jc.JobID)
		}
		panic(err.Error())
	}

	return jc
}

// DoJob executes the job
func (jc *JobClient) DoJob() {
	jc.mu.Lock()
	if jc.JobDoing {
		jc.mu.Unlock()
		for jc.JobDoing {
			if toInt(jc.Job["enable"]) == 0 {
				return
			}
			time.Sleep(10 * time.Second)
		}
		jc.mu.Lock()
	}
	jc.JobDoing = true
	jc.mu.Unlock()

	taskID := int64(0)
	defer func() {
		if r := recover(); r != nil {
			jc.JobDoing = false
			errMsg := fmt.Sprintf("%v", r)
			log.Printf("Job execution error: %s", errMsg)
			if taskID > 0 {
				UpdateJobTaskStatusSimple(taskID, 6, &errMsg)
			}
		}
	}()

	var err error
	taskID, err = mapper.AddJobTask(jc.JobID, time.Now().Unix())
	if err != nil {
		panic(err.Error())
	}
	if toInt(jc.Job["enable"]) == 0 {
		panic("abort")
	}
	jc.CurrentJobTask = NewJobTask(taskID, jc)
}

// DoManual triggers manual execution
func (jc *JobClient) DoManual() {
	if jc.JobDoing {
		panic(i18n.G("job_running"))
	}
	go jc.DoJob()
}

// ResumeJob enables and resumes the job
func (jc *JobClient) ResumeJob() {
	if toInt(jc.Job["isCron"]) == 2 {
		// Manual only, just enable
		mapper.UpdateJobEnable(jc.JobID, 1)
		jc.Job["enable"] = 1
		return
	}

	err := jc.Scheduler.Resume(toInt(jc.Job["isCron"]), jc.Job, jc.DoJob)
	if err != nil {
		panic(err.Error())
	}
	mapper.UpdateJobEnable(jc.JobID, 1)
	jc.Job["enable"] = 1
}

// AbortJob aborts the current running task
func (jc *JobClient) AbortJob() {
	if jc.CurrentJobTask != nil {
		jc.CurrentJobTask.requestBreak()
	}
}

// StopJob stops the job (for disable or delete)
func (jc *JobClient) StopJob(remove bool) {
	jc.Job["enable"] = 0
	if jc.CurrentJobTask != nil {
		jc.CurrentJobTask.requestBreak()
	}
	if remove {
		jc.Scheduler.Stop()
	} else {
		jc.Scheduler.Pause()
		mapper.UpdateJobEnable(jc.JobID, 0)
		mapper.UpdateJobTaskStatusByStatusAndJobID(jc.JobID)
	}
	jc.JobDoing = false
}

// UpdateJobTaskStatusFinal updates task status after completion with notification
func UpdateJobTaskStatusFinal(taskID int64, status int, currentTasks map[int][]map[string]interface{}, createTime float64) {
	var duration int
	if createTime > 0 {
		duration = int(float64(time.Now().Unix()) - createTime)
	}

	var taskNum map[string]interface{}
	var errMsg *string

	if currentTasks != nil {
		successNum := len(currentTasks[2])
		failNum := len(currentTasks[7])
		otherNum := len(currentTasks[-1])
		allNum := successNum + failNum + otherNum

		var sumSize int64
		if tasks, ok := currentTasks[2]; ok {
			for _, t := range tasks {
				if t["fileSize"] != nil && t["type"].(int) != 1 {
					sumSize += toInt64Val(t["fileSize"])
				}
			}
		}

		taskNum = map[string]interface{}{
			"waitNum":    0,
			"runningNum": 0,
			"successNum": successNum,
			"failNum":    failNum,
			"otherNum":   otherNum,
			"allNum":     allNum,
			"duration":   duration,
			"sumSize":    sumSize,
		}
	} else {
		taskNum = GetCuTaskNum(taskID)
	}

	mapper.UpdateJobTaskStatus(taskID, status, errMsg)
	taskNumJSON, _ := json.Marshal(taskNum)
	mapper.UpdateJobTaskNumMany([]map[string]interface{}{
		{"taskId": taskID, "taskNum": string(taskNumJSON)},
	})

	// Send notifications
	SendTaskNotification(taskID, status, taskNum, duration, createTime)
}

// UpdateJobTaskStatusSimple updates task status with error message
func UpdateJobTaskStatusSimple(taskID int64, status int, errMsg *string) {
	mapper.UpdateJobTaskStatus(taskID, status, errMsg)
	taskNum := GetCuTaskNum(taskID)
	taskNumJSON, _ := json.Marshal(taskNum)
	mapper.UpdateJobTaskNumMany([]map[string]interface{}{
		{"taskId": taskID, "taskNum": string(taskNumJSON)},
	})
}

// GetCuTaskNum gets current task counts from DB
func GetCuTaskNum(taskID int64) map[string]interface{} {
	return mapper.GetJobTaskCounts(taskID)
}

func toInt64Val(v interface{}) int64 {
	switch val := v.(type) {
	case int64:
		return val
	case int:
		return int64(val)
	case float64:
		return int64(val)
	default:
		return 0
	}
}

func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	default:
		return 0
	}
}
