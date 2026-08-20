package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"opensync/internal/msg"
	"opensync/pkg/util"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	ignore "github.com/sabhiram/go-gitignore"
)

type scanWorkMode int

const (
	scanWorkCompare scanWorkMode = iota
	scanWorkMissingDst
)

type scanWork struct {
	SrcPath     string
	DstPath     string
	SrcRootPath string
	DstRootPath string
	FirstDst    bool
	Mode        scanWorkMode
	Counted     bool
}

func (jt *JobTask) acquireScanSlot() bool {
	if jt.isBreak() {
		return false
	}
	// Block on the semaphore instead of polling every 50ms. The context is
	// cancelled on break/timeout (requestBreak / task timeout), so the wait
	// still propagates cancellation promptly.
	select {
	case jt.scanSem <- struct{}{}:
		if jt.isBreak() {
			<-jt.scanSem
			return false
		}
		return true
	case <-jt.context().Done():
		return false
	}
}

func (jt *JobTask) releaseScanSlot() {
	<-jt.scanSem
}

func (jt *JobTask) tryAcquireScanBranchSlot() bool {
	jt.initRuntime()
	if jt.isBreak() {
		return false
	}
	select {
	case jt.scanBranchSem <- struct{}{}:
		return true
	default:
		return false
	}
}

func (jt *JobTask) releaseScanBranchSlot() {
	<-jt.scanBranchSem
}

func (jt *JobTask) beginScanWork(work scanWork) {
	if !work.Counted {
		jt.ScanTotalDirs.Add(1)
	}
}

func (jt *JobTask) addChildScanWork(children *[]scanWork, work scanWork) {
	work.Counted = true
	jt.ScanTotalDirs.Add(1)
	*children = append(*children, work)
}

func (jt *JobTask) finishScanWork() {
	jt.ScanDoneDirs.Add(1)
	jt.notifyProgressChange()
}

func (jt *JobTask) sync() {
	if jt.hasRetrySource() {
		jt.syncRetryItems()
		return
	}

	srcPaths := parsePathList(jt.Job["srcPath"])
	jobExclude := jt.Job["exclude"]

	var spec *ignore.GitIgnore
	if jobExclude != nil {
		excludeStr := fmt.Sprintf("%v", jobExclude)
		if excludeStr != "" {
			patterns := parseExcludePatterns(excludeStr)
			spec = ignore.CompileIgnoreLines(patterns...)
		}
	}

	dstPaths := parsePathList(jt.Job["dstPath"])
	hasMultipleSrc := len(srcPaths) > 1
	for _, srcItem := range srcPaths {
		srcItem = normalizeDirPath(srcItem)
		for i, dstItem := range dstPaths {
			dstItem = normalizeDirPath(dstItem)
			resolvedDstPath := dstPathForSrcSelection(dstItem, srcItem, hasMultipleSrc)
			jt.runScanWork(scanWork{
				SrcPath:     srcItem,
				DstPath:     resolvedDstPath,
				SrcRootPath: srcItem,
				DstRootPath: resolvedDstPath,
				FirstDst:    i == 0,
				Mode:        scanWorkCompare,
			}, spec)
		}
	}
	jt.markScanFinished()
}

func (jt *JobTask) hasRetrySource() bool {
	return jt.RetrySourceTaskID > 0
}

func (jt *JobTask) markScanFinished() {
	jt.ScanFinish.Store(true)
	jt.notifyProgressChange()
}

func (jt *JobTask) syncRetryItems() {
	err := forEachJobTaskItemsByStatuses(jt.RetrySourceTaskID, taskStatusValues(jt.RetryStatuses...), retryTaskItemBatchSize, func(items []map[string]interface{}) error {
		for _, item := range items {
			if jt.isBreak() {
				return errScanAborted
			}
			jt.retryTaskItem(item)
		}
		return nil
	})
	if err != nil && !errors.Is(err, errScanAborted) {
		errMsg := err.Error()
		jt.CopyHook("", "", "", nil, "", taskStatusFailed, &errMsg, taskItemPath, taskItemTypeCopy, time.Now().Unix())
	}
	jt.markScanFinished()
}

func (jt *JobTask) retryTaskItem(item map[string]interface{}) {
	copyType := taskItemTypeFromValue(item["type"])
	isPath := taskItemObjectFromValue(item["isPath"]) == taskItemPath
	srcPath := util.StringValue(item["srcPath"])
	dstPath := util.StringValue(item["dstPath"])
	fileName := util.StringValue(item["fileName"])
	fileSize := item["fileSize"]

	switch copyType {
	case taskItemTypeDelete:
		jt.delFile(dstPath, fileName, fileSize)
	case taskItemTypeCopy, taskItemTypeMove:
		if isPath {
			jt.retryMkdir(srcPath, dstPath, copyType)
			return
		}
		jt.queueCopyFile(srcPath, dstPath, fileName, fileSize, copyType)
	default:
		errMsg := fmt.Sprintf("unsupported retry task type: %d", copyType)
		jt.CopyHook(srcPath, dstPath, fileName, fileSize, "", taskStatusFailed, &errMsg, boolToTaskItemObject(isPath), copyType, time.Now().Unix())
	}
}

func (jt *JobTask) retryMkdir(srcPath, dstPath string, copyType taskItemType) {
	if dstPath == "" {
		errMsg := "missing destination path for directory retry"
		jt.CopyHook(srcPath, dstPath, "", nil, "", taskStatusFailed, &errMsg, taskItemPath, copyType, time.Now().Unix())
		return
	}

	status := taskStatusSuccess
	var errMsg *string
	scanIntervalT := util.ToInt(jt.Job["scanIntervalT"])
	if err := jt.AlistClient.MkdirContext(jt.context(), dstPath, scanIntervalT); err != nil {
		status = taskStatusFailed
		e := err.Error()
		errMsg = &e
	}
	jt.CopyHook(srcPath, dstPath, "", nil, "", status, errMsg, taskItemPath, copyType, time.Now().Unix())
}

func dstPathForSrcSelection(dstPath, srcPath string, hasMultipleSrc bool) string {
	dstPath = normalizeDirPath(dstPath)
	if !hasMultipleSrc {
		return dstPath
	}

	base := path.Base(strings.TrimSuffix(srcPath, "/"))
	if base == "." || base == "/" || base == "" {
		return dstPath
	}
	return normalizeDirPath(dstPath + base)
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
	method := util.ToInt(jt.Job["method"])
	copyType := taskItemTypeCopy
	if method >= 2 {
		copyType = taskItemTypeMove
	}
	jt.queueCopyFile(srcPath, dstPath, fileName, fileSize, copyType)
}

func (jt *JobTask) queueCopyFile(srcPath, dstPath, fileName string, fileSize interface{}, copyType taskItemType) {
	if jt.isBreak() {
		return
	}
	ci := newCopyItem(jt, jt.AlistClient, srcPath, dstPath, fileName, fileSize, copyType)
	if !jt.Waiting.pushWait(jt.context(), ci) {
		ci.setStatus(taskStatusStopped)
		jt.CopyHook(ci.SrcPath, ci.DstPath, ci.FileName, ci.FileSize, ci.AlistTaskID,
			ci.Status, ci.ErrMsg, taskItemFile, ci.CopyType, ci.CreateTime)
	}
}

func (jt *JobTask) delFile(path, fileName string, size interface{}) taskStatus {
	statuses := jt.delFiles(path, []string{fileName}, []interface{}{size})
	if len(statuses) == 0 {
		return taskStatusStopped
	}
	return statuses[0]
}

func (jt *JobTask) delFiles(path string, fileNames []string, sizes []interface{}) []taskStatus {
	statuses := make([]taskStatus, len(fileNames))
	if len(fileNames) == 0 {
		return statuses
	}
	if jt.isBreak() {
		for i := range statuses {
			statuses[i] = taskStatusStopped
		}
		return statuses
	}

	apiNames := make([]string, len(fileNames))
	isPath := make([]bool, len(fileNames))
	for i, fileName := range fileNames {
		dir := strings.HasSuffix(fileName, "/")
		isPath[i] = dir
		if dir {
			apiNames[i] = fileName[:len(fileName)-1]
		} else {
			apiNames[i] = fileName
		}
	}

	scanIntervalT := util.ToInt(jt.Job["scanIntervalT"])
	err := jt.AlistClient.DeleteFileContext(jt.context(), path, apiNames, scanIntervalT)
	if err != nil && len(fileNames) > 1 {
		// AList rejected the batch; fall back to per-file deletes so a single
		// bad name does not skip the rest of a mirror pass.
		for i, fileName := range fileNames {
			var size interface{}
			if i < len(sizes) {
				size = sizes[i]
			}
			statuses[i] = jt.delFile(path, fileName, size)
		}
		return statuses
	}

	createTime := time.Now().Unix()
	for i, fileName := range fileNames {
		status := taskStatusSuccess
		var errMsg *string
		if err != nil {
			status = taskStatusFailed
			e := err.Error()
			errMsg = &e
		}
		var delSize interface{}
		if !isPath[i] && i < len(sizes) {
			delSize = sizes[i]
		}
		jt.DelHook(path, fileName, delSize, status, errMsg, boolToTaskItemObject(isPath[i]), createTime)
		statuses[i] = status
	}
	return statuses
}

func (jt *JobTask) listDir(path string, firstDst bool, spec *ignore.GitIgnore, rootPath string, isSrc bool) (FileListResult, error) {
	var useCache int
	if isSrc && !firstDst {
		useCache = 1
	} else {
		if isSrc {
			useCache = util.ToInt(jt.Job["useCacheS"])
		} else {
			useCache = util.ToInt(jt.Job["useCacheT"])
		}
	}

	var scanInterval int
	if isSrc {
		scanInterval = util.ToInt(jt.Job["scanIntervalS"])
	} else {
		scanInterval = util.ToInt(jt.Job["scanIntervalT"])
	}

	if !jt.acquireScanSlot() {
		return nil, errScanAborted
	}
	defer jt.releaseScanSlot()

	var result FileListResult
	var err error
	for attempt := 0; attempt <= maxScanListRetries; attempt++ {
		result, err = jt.AlistClient.FileListApiContext(jt.context(), path, useCache, scanInterval)
		if err == nil || !shouldRetryScanList(jt.context(), err) || attempt == maxScanListRetries {
			break
		}
		log.Printf("Directory scan failed for %q; retrying (%d/%d): %v", path, attempt+1, maxScanListRetries, err)
		if !jt.waitForBreak(scanListRetryDelay(attempt)) {
			return nil, context.Canceled
		}
	}
	if err != nil {
		if jt.isBreak() && errors.Is(err, context.Canceled) {
			return nil, err
		}
		srcOrDst := msg.Src
		if !isSrc {
			srcOrDst = msg.Dst
		}
		errMsg := msg.ScanError(srcOrDst, err.Error())
		log.Printf("%s", errMsg)

		jt.CopyHook(pathIfTrue(isSrc, path), pathIfTrue(!isSrc, path), "", nil, "", taskStatusFailed, &errMsg, taskItemPath, taskItemTypeCopy, time.Now().Unix())
		return nil, err
	}

	// Apply exclude rules
	if spec != nil && len(result) > 0 {
		filtered := make(FileListResult, len(result))
		for key, val := range result {
			checkPath := excludeMatchPath(rootPath, path, key)
			if !spec.MatchesPath(checkPath) {
				filtered[key] = val
			}
		}
		return filtered, nil
	}

	return result, nil
}

func shouldRetryScanList(ctx context.Context, err error) bool {
	if err == nil || ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return false
	}
	errMsg := err.Error()
	return errMsg != msg.AlistUnAuth && errMsg != msg.AddressIncorrect
}

func defaultScanListRetryDelay(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	return time.Duration(1<<attempt) * time.Second
}

func excludeMatchPath(rootPath, currentPath, name string) string {
	relDir := strings.TrimPrefix(currentPath, rootPath)
	relDir = strings.Trim(relDir, "/")
	if relDir == "" {
		return name
	}
	return relDir + "/" + name
}

func pathIfTrue(cond bool, path string) string {
	if cond {
		return path
	}
	return ""
}

func (jt *JobTask) listSrcAndDst(srcPath, dstPath string, spec *ignore.GitIgnore, srcRootPath, dstRootPath string, firstDst bool) (FileListResult, FileListResult, error) {
	var srcFiles, dstFiles FileListResult
	var srcErr, dstErr error

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		defer jt.recoverWorkerPanic("source scan", &srcErr)
		srcFiles, srcErr = jt.listDir(srcPath, firstDst, spec, srcRootPath, true)
	}()
	go func() {
		defer wg.Done()
		defer jt.recoverWorkerPanic("destination scan", &dstErr)
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
		srcFiles = make(FileListResult)
	}
	if dstFiles == nil {
		dstFiles = make(FileListResult)
	}
	return srcFiles, dstFiles, nil
}

func (jt *JobTask) runScanWork(work scanWork, spec *ignore.GitIgnore) {
	if work.Mode == scanWorkMissingDst {
		jt.syncWithoutHave(work, spec)
		return
	}
	jt.syncWithHave(work, spec)
}

func (jt *JobTask) runChildScanWorks(children []scanWork, spec *ignore.GitIgnore) {
	if len(children) == 0 {
		return
	}

	var wg sync.WaitGroup
	for _, child := range children {
		child := child
		if jt.tryAcquireScanBranchSlot() {
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer jt.releaseScanBranchSlot()
				defer jt.recoverWorkerPanic("child scan", nil)
				jt.runScanWork(child, spec)
			}()
			continue
		}
		jt.runScanWork(child, spec)
	}
	wg.Wait()
}

func (jt *JobTask) syncWithHave(work scanWork, spec *ignore.GitIgnore) {
	jt.beginScanWork(work)
	if jt.isBreak() {
		jt.finishScanWork()
		return
	}

	srcFiles, dstFiles, err := jt.listSrcAndDst(work.SrcPath, work.DstPath, spec, work.SrcRootPath, work.DstRootPath, work.FirstDst)
	if err != nil {
		jt.finishScanWork()
		return
	}

	children := make([]scanWork, 0)
	dstIndex := newDstNameMatchIndex(dstFiles)
	srcIndex := newSrcNameMatchIndex(srcFiles)
	matchedDstKeys := make(map[string]struct{})
	for _, key := range sortedFileKeys(srcFiles) {
		srcVal := srcFiles[key]
		if jt.isBreak() {
			break
		}
		if !strings.HasSuffix(key, "/") {
			// File
			srcSize := fileSize(srcVal)
			if !jobAllowsFileSize(jt.Job, srcSize) {
				continue
			}
			if util.ToInt(jt.Job["method"]) == 1 {
				if dstDirKey, dstDirVal, exists := dstIndex.find(key+"/", srcIndex); exists {
					if jt.delFile(work.DstPath, dstDirKey, fileSize(dstDirVal)) != taskStatusSuccess {
						continue
					}
					delete(dstFiles, dstDirKey)
				}
			}
			dstKey, dstVal, exists := dstIndex.find(key, srcIndex)
			if exists {
				matchedDstKeys[dstKey] = struct{}{}
			}
			if !exists || fileChanged(srcVal, dstVal) {
				jt.copyFile(work.SrcPath, work.DstPath, key, srcSize)
			}
		} else {
			// Directory
			if util.ToInt(jt.Job["method"]) == 1 {
				fileKey := strings.TrimSuffix(key, "/")
				if dstFileKey, dstFileVal, exists := dstIndex.find(fileKey, srcIndex); exists {
					if jt.delFile(work.DstPath, dstFileKey, fileSize(dstFileVal)) != taskStatusSuccess {
						continue
					}
					delete(dstFiles, dstFileKey)
				}
			}
			dstKey, _, exists := dstIndex.find(key, srcIndex)
			if !exists {
				jt.addChildScanWork(&children, scanWork{
					SrcPath:     work.SrcPath + key,
					DstPath:     work.DstPath + key,
					SrcRootPath: work.SrcRootPath,
					DstRootPath: work.DstRootPath,
					FirstDst:    work.FirstDst,
					Mode:        scanWorkMissingDst,
				})
			} else {
				matchedDstKeys[dstKey] = struct{}{}
				jt.addChildScanWork(&children, scanWork{
					SrcPath:     work.SrcPath + key,
					DstPath:     work.DstPath + dstKey,
					SrcRootPath: work.SrcRootPath,
					DstRootPath: work.DstRootPath,
					FirstDst:    work.FirstDst,
					Mode:        scanWorkCompare,
				})
			}
		}
	}

	if jt.isBreak() {
		jt.finishScanWork()
		jt.runChildScanWorks(children, spec)
		return
	}

	if util.ToInt(jt.Job["method"]) == 1 {
		var extraNames []string
		var extraSizes []interface{}
		for _, dstKey := range sortedFileKeys(dstFiles) {
			if _, matched := matchedDstKeys[dstKey]; matched {
				continue
			}
			extraNames = append(extraNames, dstKey)
			extraSizes = append(extraSizes, fileSize(dstFiles[dstKey]))
		}
		jt.delFiles(work.DstPath, extraNames, extraSizes)
	}
	jt.finishScanWork()
	jt.runChildScanWorks(children, spec)
}

func (jt *JobTask) syncWithoutHave(work scanWork, spec *ignore.GitIgnore) {
	jt.beginScanWork(work)
	if jt.isBreak() {
		jt.finishScanWork()
		return
	}

	status := taskStatusSuccess
	var errMsg *string
	scanIntervalT := util.ToInt(jt.Job["scanIntervalT"])
	err := jt.AlistClient.MkdirContext(jt.context(), work.DstPath, scanIntervalT)
	if err != nil {
		status = taskStatusFailed
		e := err.Error()
		errMsg = &e
	}

	jt.CopyHook(work.SrcPath, work.DstPath, "", nil, "", status, errMsg, taskItemPath, taskItemTypeCopy, time.Now().Unix())
	if status != taskStatusSuccess {
		jt.finishScanWork()
		return
	}

	srcFiles, err := jt.listDir(work.SrcPath, work.FirstDst, spec, work.SrcRootPath, true)
	if err != nil {
		jt.finishScanWork()
		return
	}

	children := make([]scanWork, 0)
	for _, key := range sortedFileKeys(srcFiles) {
		srcVal := srcFiles[key]
		if jt.isBreak() {
			break
		}
		if strings.HasSuffix(key, "/") {
			jt.addChildScanWork(&children, scanWork{
				SrcPath:     work.SrcPath + key,
				DstPath:     work.DstPath + key,
				SrcRootPath: work.SrcRootPath,
				DstRootPath: work.DstRootPath,
				FirstDst:    work.FirstDst,
				Mode:        scanWorkMissingDst,
			})
		} else {
			srcSize := fileSize(srcVal)
			if !jobAllowsFileSize(jt.Job, srcSize) {
				continue
			}
			jt.copyFile(work.SrcPath, work.DstPath, key, srcSize)
		}
	}
	jt.finishScanWork()
	jt.runChildScanWorks(children, spec)
}

func jobAllowsFileSize(job map[string]interface{}, size int64) bool {
	minSize := util.ToInt64(job["minFileSize"])
	maxSize := util.ToInt64(job["maxFileSize"])
	if minSize > 0 && size < minSize {
		return false
	}
	if maxSize > 0 && size > maxSize {
		return false
	}
	return true
}

func fileChanged(src, dst FileMetadata) bool {
	if src.MD5 != "" && dst.MD5 != "" {
		return src.MD5 != dst.MD5
	}
	if src.Size != dst.Size {
		return true
	}
	if src.Modified != 0 && dst.Modified != 0 {
		return src.Modified != dst.Modified
	}
	return false
}

func sortedFileKeys(files FileListResult) []string {
	keys := make([]string, 0, len(files))
	for key := range files {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func fileSize(meta FileMetadata) int64 {
	return meta.Size
}
