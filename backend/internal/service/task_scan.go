package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"opensync/internal/msg"
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

type retryPathPair struct {
	srcRoot string
	dstRoot string
}

type retryJobScope struct {
	configured bool
	method     int
	pairs      []retryPathPair
	spec       *ignore.GitIgnore
}

func (jt *JobTask) acquireScanSlot() bool {
	return jt.acquireScanSlotContext(jt.context())
}

func (jt *JobTask) acquireScanSlotContext(ctx context.Context) bool {
	if jt.isBreak() || ctx.Err() != nil {
		return false
	}
	jt.initRuntime()
	// Block on the semaphore instead of polling every 50ms. The context is
	// cancelled on break/timeout (requestBreak / task timeout) or sibling scan
	// error, so the wait propagates cancellation promptly.
	select {
	case jt.scanSem <- struct{}{}:
		if jt.isBreak() || ctx.Err() != nil {
			<-jt.scanSem
			return false
		}
		return true
	case <-ctx.Done():
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

func compileJobExcludeSpec(job map[string]interface{}) *ignore.GitIgnore {
	if job == nil || job["exclude"] == nil {
		return nil
	}
	excludeStr := fmt.Sprintf("%v", job["exclude"])
	if excludeStr == "" {
		return nil
	}
	patterns := parseExcludePatterns(excludeStr)
	if len(patterns) == 0 {
		return nil
	}
	return ignore.CompileIgnoreLines(patterns...)
}

func (jt *JobTask) buildRetryJobScope() retryJobScope {
	srcPaths := parsePathList(jt.Job["srcPath"])
	dstPaths := parsePathList(jt.Job["dstPath"])
	scope := retryJobScope{
		configured: len(srcPaths) > 0 && len(dstPaths) > 0,
		method:     util.ToInt(jt.Job["method"]),
		spec:       compileJobExcludeSpec(jt.Job),
	}
	if !scope.configured {
		return scope
	}
	for _, srcItem := range srcPaths {
		srcRoot := normalizeDirPath(srcItem)
		for _, dstItem := range dstPaths {
			dstRoot := dstPathForSrcSelection(normalizeDirPath(dstItem), srcRoot, srcPaths)
			scope.pairs = append(scope.pairs, retryPathPair{
				srcRoot: srcRoot,
				dstRoot: dstRoot,
			})
		}
	}
	return scope
}

func isDirUnderRoot(dirPath, rootDir string) bool {
	normDir := normalizeDirPath(dirPath)
	normRoot := normalizeDirPath(rootDir)
	if normDir == "" || normRoot == "" {
		return false
	}
	if normRoot == "/" {
		return strings.HasPrefix(normDir, "/")
	}
	return normDir == normRoot || strings.HasPrefix(normDir, normRoot)
}

func (jt *JobTask) sync() {
	if jt.hasRetrySource() {
		jt.syncRetryItems()
		return
	}

	srcPaths := parsePathList(jt.Job["srcPath"])
	spec := compileJobExcludeSpec(jt.Job)

	dstPaths := parsePathList(jt.Job["dstPath"])
	fullSync := util.ToInt(jt.Job["method"]) == 1
	for _, srcItem := range srcPaths {
		srcItem = normalizeDirPath(srcItem)
		for i, dstItem := range dstPaths {
			dstItem = normalizeDirPath(dstItem)
			resolvedDstPath := dstPathForSrcSelection(dstItem, srcItem, srcPaths)
			work := scanWork{
				SrcPath:     srcItem,
				DstPath:     resolvedDstPath,
				SrcRootPath: srcItem,
				DstRootPath: resolvedDstPath,
				FirstDst:    i == 0,
				Mode:        scanWorkCompare,
			}
			if fullSync {
				jt.syncFull(work, spec)
			} else {
				jt.runScanWork(work, spec)
			}
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
	scope := jt.buildRetryJobScope()
	err := forEachJobTaskItemsByStatuses(jt.RetrySourceTaskID, taskStatusValues(jt.RetryStatuses...), retryTaskItemBatchSize, func(items []map[string]interface{}) error {
		for _, item := range items {
			if jt.isBreak() {
				return errScanAborted
			}
			jt.retryTaskItemWithScope(item, scope)
		}
		return nil
	})
	if err != nil && !errors.Is(err, errScanAborted) {
		if jt.isBreak() {
			jt.markScanFinished()
			return
		}
		errMsg := err.Error()
		jt.CopyHook("", "", "", nil, "", taskStatusFailed, &errMsg, taskItemPath, taskItemTypeCopy, time.Now().Unix())
	}
	jt.markScanFinished()
}

func (jt *JobTask) retryTaskItem(item map[string]interface{}) {
	jt.retryTaskItemWithScope(item, jt.buildRetryJobScope())
}

func (jt *JobTask) retryTaskItemWithScope(item map[string]interface{}, scope retryJobScope) {
	if jt.isBreak() {
		return
	}
	copyType := taskItemTypeFromValue(item["type"])
	isPath := taskItemObjectFromValue(item["isPath"]) == taskItemPath
	srcPath := util.StringValue(item["srcPath"])
	dstPath := util.StringValue(item["dstPath"])
	fileName := util.StringValue(item["fileName"])
	fileSize := item["fileSize"]

	switch copyType {
	case taskItemTypeDelete:
		if scope.configured {
			// Only Full Sync (mirror mode) permits deleting destination entries.
			if scope.method != 1 {
				return
			}
			matchedPair, ok := scope.matchDstPair(dstPath)
			if !ok {
				return
			}
			if scope.spec != nil && fileName != "" {
				checkPath := excludeMatchPath(matchedPair.dstRoot, normalizeDirPath(dstPath), fileName)
				if scope.spec.MatchesPath(checkPath) {
					return
				}
			}
			// Verify the source object is still absent before deleting from
			// destination so a restored source file/dir is never wiped by a stale
			// historical delete retry.
			if jt.AlistClient != nil && fileName != "" {
				relDir := strings.TrimPrefix(normalizeDirPath(dstPath), matchedPair.dstRoot)
				srcDir := matchedPair.srcRoot + relDir
				probeName := strings.TrimSuffix(fileName, "/")
				entry, exists, err := jt.AlistClient.GetFileInfoContext(jt.context(), srcDir, probeName)
				if err != nil {
					if jt.isBreak() {
						return
					}
					errMsg := fmt.Sprintf("failed to verify source before retry delete: %v", err)
					var delSize interface{}
					if !strings.HasSuffix(fileName, "/") {
						delSize = fileSize
					}
					jt.DelHook(dstPath, fileName, delSize, taskStatusFailed, &errMsg, boolToTaskItemObject(strings.HasSuffix(fileName, "/")), time.Now().Unix())
					return
				}
				if exists && entry != nil {
					return
				}
			}
		}
		jt.delFile(dstPath, fileName, fileSize)
	case taskItemTypeCopy, taskItemTypeMove:
		if isPath {
			// Scan failures only retain the failing side, not a resumable source/destination pair.
			if srcPath == "" || dstPath == "" {
				if jt.isBreak() {
					return
				}
				errMsg := msg.ScanRetryRequiresRun
				if originalError := util.StringValue(item["errMsg"]); originalError != "" {
					errMsg += ": " + originalError
				}
				jt.CopyHook(srcPath, dstPath, fileName, fileSize, "", taskStatusFailed, &errMsg, taskItemPath, copyType, time.Now().Unix())
				return
			}
			if scope.configured {
				pair, ok := scope.matchSrcDstPair(srcPath, dstPath, copyType)
				if !ok {
					return
				}
				if scope.spec != nil && scope.isDirExcluded(pair.dstRoot, dstPath) {
					return
				}
			}
			jt.retryMkdir(srcPath, dstPath, copyType)
			return
		}
		if scope.configured {
			pair, isRelocation, ok := scope.matchFilePair(srcPath, dstPath, copyType)
			if !ok {
				return
			}
			if scope.spec != nil && fileName != "" {
				root := pair.srcRoot
				if isRelocation {
					root = pair.dstRoot
				}
				checkPath := excludeMatchPath(root, normalizeDirPath(srcPath), fileName)
				if scope.spec.MatchesPath(checkPath) {
					return
				}
			}
		}
		jt.queueCopyFile(srcPath, dstPath, fileName, fileSize, copyType)
	default:
		if jt.isBreak() {
			return
		}
		errMsg := fmt.Sprintf("unsupported retry task type: %d", copyType)
		jt.CopyHook(srcPath, dstPath, fileName, fileSize, "", taskStatusFailed, &errMsg, boolToTaskItemObject(isPath), copyType, time.Now().Unix())
	}
}

func (s retryJobScope) matchDstPair(dstPath string) (retryPathPair, bool) {
	for _, pair := range s.pairs {
		if isDirUnderRoot(dstPath, pair.dstRoot) {
			return pair, true
		}
	}
	return retryPathPair{}, false
}

func (s retryJobScope) matchSrcDstPair(srcPath, dstPath string, copyType taskItemType) (retryPathPair, bool) {
	for _, pair := range s.pairs {
		if isDirUnderRoot(srcPath, pair.srcRoot) && isDirUnderRoot(dstPath, pair.dstRoot) {
			return pair, true
		}
		if copyType == taskItemTypeMove && s.method == 1 && isDirUnderRoot(srcPath, pair.dstRoot) && isDirUnderRoot(dstPath, pair.dstRoot) {
			return pair, true
		}
	}
	return retryPathPair{}, false
}

func (s retryJobScope) matchFilePair(srcPath, dstPath string, copyType taskItemType) (retryPathPair, bool, bool) {
	for _, pair := range s.pairs {
		if copyType == taskItemTypeMove {
			if s.method == 2 && isDirUnderRoot(srcPath, pair.srcRoot) && isDirUnderRoot(dstPath, pair.dstRoot) {
				return pair, false, true
			}
			if s.method == 1 && isDirUnderRoot(srcPath, pair.dstRoot) && isDirUnderRoot(dstPath, pair.dstRoot) {
				return pair, true, true
			}
			continue
		}
		if isDirUnderRoot(srcPath, pair.srcRoot) && isDirUnderRoot(dstPath, pair.dstRoot) {
			return pair, false, true
		}
	}
	return retryPathPair{}, false, false
}

func (s retryJobScope) isDirExcluded(rootDir, dirPath string) bool {
	rel := strings.TrimPrefix(normalizeDirPath(dirPath), normalizeDirPath(rootDir))
	rel = strings.Trim(rel, "/")
	if rel == "" {
		return false
	}
	return s.spec.MatchesPath(rel + "/")
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
		if jt.isBreak() {
			return
		}
		status = taskStatusFailed
		e := err.Error()
		errMsg = &e
	}
	jt.CopyHook(srcPath, dstPath, "", nil, "", status, errMsg, taskItemPath, copyType, time.Now().Unix())
}

func dstPathForSrcSelection(dstPath, srcPath string, srcPaths []string) string {
	dstPath = normalizeDirPath(dstPath)
	if len(srcPaths) <= 1 {
		return dstPath
	}

	suffix := srcSelectionSuffix(srcPath, srcPaths)
	if suffix == "." || suffix == "/" || suffix == "" {
		return dstPath
	}
	return normalizeDirPath(dstPath + suffix)
}

func srcSelectionSuffix(srcPath string, srcPaths []string) string {
	cleanSrc := canonicalJobPath(srcPath)
	// A partially checked tree branch arrives as sibling paths rather than its
	// parent, so the shared parent is the highest segment the selection already
	// accounts for: everything above it is stripped, everything below it is kept
	// so nested branches still reach the destination with their hierarchy.
	commonParent := commonSrcSelectionParent(srcPaths)
	if commonParent == "" || commonParent == "." || commonParent == "/" {
		// Sources with no shared parent keep the historical base-name layout,
		// unless any colliding base names or mixed base-name/full-path suffixes
		// would nest one destination under another (e.g. ["/a/docs", "/b/docs",
		// "/x/a"] -> "a/docs", "b/docs", "a"). If any pairwise prefix overlap
		// exists, fall back to full normalized relative paths for all selections.
		if srcSelectionRootSuffixesOverlap(srcPaths) {
			return strings.TrimPrefix(cleanSrc, "/")
		}
		if srcSelectionBaseNameIsAmbiguous(cleanSrc, srcPaths) {
			return strings.TrimPrefix(cleanSrc, "/")
		}
		return path.Base(cleanSrc)
	}

	// A selected path that *is* the shared parent has no suffix of its own.
	// Without this the TrimPrefix below misses (the prefix carries a trailing
	// slash the path lacks) and the leftover leading slash produces "dst//src".
	if cleanSrc == commonParent {
		return ""
	}

	return strings.TrimPrefix(cleanSrc, normalizeDirPath(commonParent))
}

func srcSelectionRootSuffixesOverlap(srcPaths []string) bool {
	seenSrc := make(map[string]struct{}, len(srcPaths))
	candidates := make([]string, 0, len(srcPaths))
	for _, raw := range srcPaths {
		clean := canonicalJobPath(raw)
		if clean == "" {
			continue
		}
		if _, dup := seenSrc[clean]; dup {
			continue
		}
		seenSrc[clean] = struct{}{}
		var suffix string
		if srcSelectionBaseNameIsAmbiguous(clean, srcPaths) {
			suffix = strings.TrimPrefix(clean, "/")
		} else {
			suffix = path.Base(clean)
		}
		candidates = append(candidates, canonicalJobPath(suffix))
	}
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if pathsOverlap(candidates[i], candidates[j]) {
				return true
			}
		}
	}
	return false
}

// srcSelectionBaseNameIsAmbiguous reports whether another selected source path
// resolves to the same base name as cleanSrc.
func srcSelectionBaseNameIsAmbiguous(cleanSrc string, srcPaths []string) bool {
	base := path.Base(cleanSrc)
	for _, other := range srcPaths {
		cleanOther := canonicalJobPath(other)
		if cleanOther == "" || cleanOther == cleanSrc {
			continue
		}
		if path.Base(cleanOther) == base {
			return true
		}
	}
	return false
}

// commonSrcSelectionParent returns the deepest directory that contains every
// selected source path. Callers subtract exactly this prefix, so the value has
// to be the common parent of the paths themselves — not of their parent
// directories, which would leave one extra level in the destination path.
func commonSrcSelectionParent(srcPaths []string) string {
	if len(srcPaths) < 2 {
		return ""
	}

	common := canonicalJobPath(srcPaths[0])
	for _, srcPath := range srcPaths[1:] {
		selected := canonicalJobPath(srcPath)
		for common != "." && common != "/" && selected != common && !strings.HasPrefix(selected, normalizeDirPath(common)) {
			common = path.Dir(common)
		}
	}
	return common
}

func normalizeDirPath(p string) string {
	p = sanitizePathEntry(p)
	if p == "" {
		return ""
	}
	if !strings.HasSuffix(p, "/") {
		p += "/"
	}
	return p
}

func (jt *JobTask) copyFile(srcPath, dstPath, fileName string, fileSize interface{}) {
	jt.copyFileWithMeta(srcPath, dstPath, fileName, fileSize, "", nil)
}

func (jt *JobTask) copyFileWithMeta(srcPath, dstPath, fileName string, fileSize interface{}, expectedMD5 string, preCopyDst *FileMetadata) {
	if jt.isBreak() {
		return
	}
	method := util.ToInt(jt.Job["method"])
	copyType := taskItemTypeCopy
	if method >= 2 {
		copyType = taskItemTypeMove
	}
	jt.queueCopyFileWithMeta(srcPath, dstPath, fileName, fileSize, copyType, expectedMD5, preCopyDst)
}

func (jt *JobTask) queueCopyFile(srcPath, dstPath, fileName string, fileSize interface{}, copyType taskItemType) {
	jt.queueCopyFileWithMeta(srcPath, dstPath, fileName, fileSize, copyType, "", nil)
}

func (jt *JobTask) queueCopyFileWithMeta(srcPath, dstPath, fileName string, fileSize interface{}, copyType taskItemType, expectedMD5 string, preCopyDst *FileMetadata) {
	if jt.isBreak() {
		return
	}
	ci := newCopyItem(jt, jt.AlistClient, srcPath, dstPath, fileName, fileSize, copyType)
	if expectedMD5 != "" || preCopyDst != nil {
		ci.setExpectedVerification(expectedMD5, preCopyDst)
	}
	if !jt.Waiting.pushWait(jt.context(), ci) {
		ci.setStatus(taskStatusStopped)
		jt.CopyHook(ci.SrcPath, ci.DstPath, ci.FileName, ci.FileSize, ci.AlistTaskID,
			ci.Status, ci.ErrMsg, taskItemFile, ci.CopyType, ci.CreateTime)
	}
}

func (jt *JobTask) delFile(path, fileName string, size interface{}) taskStatus {
	if jt.isBreak() {
		return taskStatusStopped
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
		if jt.isBreak() {
			status = taskStatusStopped
		} else {
			status = taskStatusFailed
			e := err.Error()
			errMsg = &e
		}
	}

	var delSize interface{}
	if !isPath {
		delSize = size
	}
	jt.DelHook(path, fileName, delSize, status, errMsg, boolToTaskItemObject(isPath), createTime)
	return status
}

func (jt *JobTask) listDir(path string, firstDst bool, spec *ignore.GitIgnore, rootPath string, isSrc bool) (FileListResult, error) {
	result, _, err := jt.listDirContext(jt.context(), path, firstDst, spec, rootPath, isSrc, false)
	return result, err
}

func (jt *JobTask) listDirContext(ctx context.Context, path string, firstDst bool, spec *ignore.GitIgnore, rootPath string, isSrc bool, allowMissingRoot bool) (FileListResult, bool, error) {
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

	if !jt.acquireScanSlotContext(ctx) {
		return nil, false, errScanAborted
	}
	defer jt.releaseScanSlot()

	var result FileListResult
	var err error
	for attempt := 0; attempt <= maxScanListRetries; attempt++ {
		result, err = jt.AlistClient.FileListApiContext(ctx, path, useCache, scanInterval)
		if err == nil {
			break
		}
		if allowMissingRoot && !isSrc && isAlistObjectNotFound(err) {
			// A missing destination root will be auto-created by the caller;
			// do not retry 3x or emit a failed job_task_item.
			return nil, false, err
		}
		if !shouldRetryScanList(ctx, err) || attempt == maxScanListRetries {
			break
		}
		log.Printf("Directory scan failed for %q; retrying (%d/%d): %v", path, attempt+1, maxScanListRetries, err)
		if !jt.waitForBreakContext(ctx, scanListRetryDelay(attempt)) {
			return nil, false, context.Canceled
		}
	}
	if err != nil {
		if jt.isBreak() || ctx.Err() != nil {
			return nil, false, err
		}
		srcOrDst := msg.Src
		if !isSrc {
			srcOrDst = msg.Dst
		}
		errMsg := msg.ScanError(srcOrDst, err.Error())
		log.Printf("%s", errMsg)

		jt.CopyHook(pathIfTrue(isSrc, path), pathIfTrue(!isSrc, path), "", nil, "", taskStatusFailed, &errMsg, taskItemPath, taskItemTypeCopy, time.Now().Unix())
		return nil, false, err
	}

	// Apply exclude rules
	if spec != nil && len(result) > 0 {
		filtered := make(FileListResult, len(result))
		hasExcluded := false
		for key, val := range result {
			checkPath := excludeMatchPath(rootPath, path, key)
			if !spec.MatchesPath(checkPath) {
				filtered[key] = val
			} else {
				hasExcluded = true
			}
		}
		return filtered, hasExcluded, nil
	}

	return result, false, nil
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

func (jt *JobTask) recordDstScanError(dstPath string, err error) {
	if err == nil || jt.isBreak() {
		return
	}
	errMsg := msg.ScanError(msg.Dst, err.Error())
	log.Printf("%s", errMsg)
	jt.CopyHook("", dstPath, "", nil, "", taskStatusFailed, &errMsg, taskItemPath, taskItemTypeCopy, time.Now().Unix())
}

func (jt *JobTask) listSrcAndDst(srcPath, dstPath string, spec *ignore.GitIgnore, srcRootPath, dstRootPath string, firstDst bool) (FileListResult, FileListResult, error) {
	var srcFiles, dstFiles FileListResult
	var srcErr, dstErr error

	pairCtx, cancelPair := context.WithCancel(jt.context())
	defer cancelPair()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		defer jt.recoverWorkerPanic("source scan", &srcErr)
		srcFiles, _, srcErr = jt.listDirContext(pairCtx, srcPath, firstDst, spec, srcRootPath, true, false)
		if srcErr != nil {
			cancelPair()
		}
	}()
	go func() {
		defer wg.Done()
		defer jt.recoverWorkerPanic("destination scan", &dstErr)
		dstFiles, _, dstErr = jt.listDirContext(pairCtx, dstPath, firstDst, spec, dstRootPath, false, true)
		if dstErr != nil && !isAlistObjectNotFound(dstErr) {
			cancelPair()
		}
	}()
	wg.Wait()

	if srcErr != nil {
		return nil, nil, srcErr
	}
	if dstErr != nil {
		// A destination directory that does not exist yet is not a failure: the
		// job's whole purpose is to populate it. Create it and retry once so the
		// run proceeds instead of reporting "destination scan failed".
		created, mkdirErr := jt.ensureDstDirAfterListError(dstPath, dstErr)
		if !created {
			if isAlistObjectNotFound(dstErr) {
				if mkdirErr != nil {
					jt.recordDstScanError(dstPath, mkdirErr)
				} else {
					jt.recordDstScanError(dstPath, dstErr)
				}
			}
			return nil, nil, dstErr
		}
		dstFiles, _, dstErr = jt.listDirContext(jt.context(), dstPath, firstDst, spec, dstRootPath, false, false)
		if dstErr != nil {
			return nil, nil, dstErr
		}
	}
	if srcFiles == nil {
		srcFiles = make(FileListResult)
	}
	if dstFiles == nil {
		dstFiles = make(FileListResult)
	}
	return srcFiles, dstFiles, nil
}

// ensureDstDirAfterListError creates dstPath when listErr says it does not
// exist. It reports whether the caller should retry the listing, plus any
// Mkdir error. Any non-not-found error (auth, network, permissions) is left
// alone so real failures still surface.
func (jt *JobTask) ensureDstDirAfterListError(dstPath string, listErr error) (bool, error) {
	if listErr == nil || !isAlistObjectNotFound(listErr) || jt.isBreak() {
		return false, nil
	}
	scanIntervalT := util.ToInt(jt.Job["scanIntervalT"])
	if err := jt.AlistClient.MkdirContext(jt.context(), dstPath, scanIntervalT); err != nil {
		log.Printf("Failed to create missing destination directory %q: %v", dstPath, err)
		return false, err
	}
	return true, nil
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
			_, dstVal, exists := dstIndex.find(key, srcIndex)
			if !exists {
				jt.copyFileWithMeta(work.SrcPath, work.DstPath, key, srcSize, srcVal.MD5, nil)
			} else if fileChanged(srcVal, dstVal) {
				preCopy := dstVal
				jt.copyFileWithMeta(work.SrcPath, work.DstPath, key, srcSize, srcVal.MD5, &preCopy)
			}
		} else {
			// Directory
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
		if jt.isBreak() {
			jt.finishScanWork()
			return
		}
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
			jt.copyFileWithMeta(work.SrcPath, work.DstPath, key, srcSize, srcVal.MD5, nil)
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
	// Size is checked first and unconditionally: differing sizes always mean the
	// file changed, whatever the hashes claim. Only then is a hash comparison
	// allowed to declare same-size files identical, and only for well-formed
	// digests -- some drivers return a constant placeholder for every file, which
	// would otherwise make every changed file look unchanged.
	if src.Size != dst.Size {
		return true
	}
	if isMD5Digest(src.MD5) && isMD5Digest(dst.MD5) {
		return src.MD5 != dst.MD5
	}
	return false
}

// isMD5Digest reports whether value looks like a real MD5 hex digest. AList
// normalizes case but does not validate the shape.
func isMD5Digest(value string) bool {
	if len(value) != 32 {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
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
	case map[string]interface{}:
		return FileMetadata{Size: util.ToInt64(v["size"]), MD5: normalizeMD5(util.StringValue(v["md5"]))}
	default:
		return FileMetadata{Size: util.ToInt64(val)}
	}
}
