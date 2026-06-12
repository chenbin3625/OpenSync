package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"opensync/internal/i18n"
	"opensync/pkg/util"
	"path"
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
		default:
			if completed := jt.waitForBreak(50 * time.Millisecond); !completed {
				return false
			}
		}
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
	jt.ScanFinish.Store(true)
}

func (jt *JobTask) hasRetrySource() bool {
	return len(jt.RetryItems) > 0 || jt.RetrySourceTaskID > 0
}

func (jt *JobTask) syncRetryItems() {
	if jt.RetrySourceTaskID > 0 {
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
		jt.ScanFinish.Store(true)
		return
	}

	for _, item := range jt.RetryItems {
		if jt.isBreak() {
			break
		}
		jt.retryTaskItem(item)
	}
	jt.ScanFinish.Store(true)
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

func (jt *JobTask) delFile(path, fileName string, size interface{}) {
	if jt.isBreak() {
		return
	}
	isPath := strings.HasSuffix(fileName, "/")
	status := taskStatusSuccess
	var errMsg *string
	createTime := time.Now().Unix()

	name := fileName
	if isPath {
		name = fileName[:len(fileName)-1]
	}
	scanIntervalT := util.ToInt(jt.Job["scanIntervalT"])
	err := jt.AlistClient.DeleteFileContext(jt.context(), path, []string{name}, scanIntervalT)
	if err != nil {
		status = taskStatusFailed
		e := err.Error()
		errMsg = &e
	}

	var delSize interface{}
	if !isPath {
		delSize = size
	}
	jt.DelHook(path, fileName, delSize, status, errMsg, boolToTaskItemObject(isPath), createTime)
}

func (jt *JobTask) listDir(path string, firstDst bool, spec *ignore.GitIgnore, rootPath string, isSrc bool) (map[string]interface{}, error) {
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

	result, err := jt.AlistClient.FileListApiContext(jt.context(), path, useCache, scanInterval)
	if err != nil {
		if jt.isBreak() && errors.Is(err, context.Canceled) {
			return nil, err
		}
		srcOrDst := i18n.G("src")
		if !isSrc {
			srcOrDst = i18n.G("dst")
		}
		errMsg := strings.Replace(i18n.G("scan_error"), "{}", srcOrDst, 1)
		errMsg = strings.Replace(errMsg, "{}", err.Error(), 1)
		log.Printf("%s", errMsg)

		jt.CopyHook(pathIfTrue(isSrc, path), pathIfTrue(!isSrc, path), "", nil, "", taskStatusFailed, &errMsg, taskItemPath, taskItemTypeCopy, time.Now().Unix())
		return nil, err
	}

	// Apply exclude rules
	if spec != nil && len(result) > 0 {
		filtered := make(map[string]interface{})
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

func (jt *JobTask) listSrcAndDst(srcPath, dstPath string, spec *ignore.GitIgnore, srcRootPath, dstRootPath string, firstDst bool) (map[string]interface{}, map[string]interface{}, error) {
	var srcFiles, dstFiles map[string]interface{}
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
		srcFiles = make(map[string]interface{})
	}
	if dstFiles == nil {
		dstFiles = make(map[string]interface{})
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
	for key, srcVal := range srcFiles {
		if jt.isBreak() {
			break
		}
		if !strings.HasSuffix(key, "/") {
			// File
			srcSize := fileSize(srcVal)
			if !jobAllowsFileSize(jt.Job, srcSize) {
				continue
			}
			dstVal, exists := dstFiles[key]
			if !exists || fileChanged(srcVal, dstVal) {
				jt.copyFile(work.SrcPath, work.DstPath, key, srcSize)
			}
		} else {
			// Directory
			if _, exists := dstFiles[key]; !exists {
				jt.addChildScanWork(&children, scanWork{
					SrcPath:     work.SrcPath + key,
					DstPath:     work.DstPath + key,
					SrcRootPath: work.SrcRootPath,
					DstRootPath: work.DstRootPath,
					FirstDst:    work.FirstDst,
					Mode:        scanWorkMissingDst,
				})
			} else {
				jt.addChildScanWork(&children, scanWork{
					SrcPath:     work.SrcPath + key,
					DstPath:     work.DstPath + key,
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
		for dstKey, dstVal := range dstFiles {
			if _, exists := srcFiles[dstKey]; !exists {
				jt.delFile(work.DstPath, dstKey, fileSize(dstVal))
			}
		}
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
	for key, srcVal := range srcFiles {
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

func fileChanged(srcVal, dstVal interface{}) bool {
	src := toFileMetadata(srcVal)
	dst := toFileMetadata(dstVal)
	if src.MD5 != "" && dst.MD5 != "" {
		return src.MD5 != dst.MD5
	}
	return src.Size != dst.Size
}

func fileSize(val interface{}) int64 {
	return toFileMetadata(val).Size
}

func toFileMetadata(val interface{}) FileMetadata {
	switch v := val.(type) {
	case FileMetadata:
		v.MD5 = normalizeMD5(v.MD5)
		return v
	case *FileMetadata:
		if v == nil {
			return FileMetadata{}
		}
		metadata := *v
		metadata.MD5 = normalizeMD5(metadata.MD5)
		return metadata
	default:
		return FileMetadata{Size: util.ToInt64(val)}
	}
}
