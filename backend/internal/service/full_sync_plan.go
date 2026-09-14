package service

import (
	"fmt"
	"opensync/pkg/util"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	ignore "github.com/sabhiram/go-gitignore"
)

type fullSyncSnapshot struct {
	root string
	dirs map[string]FileListResult
	mu   sync.Mutex
}

type fullSyncFile struct {
	srcDir   string
	dstDir   string
	name     string
	metadata FileMetadata
	target   string
}

type fullSyncExtraFile struct {
	dir        string
	name       string
	metadata   FileMetadata
	object     string
	deleteRoot string
}

type fullSyncDir struct {
	srcDir string
	dstDir string
}

type fullSyncDelete struct {
	dir        string
	name       string
	metadata   FileMetadata
	object     string
	blocksPath string
}

type fullSyncRelocation struct {
	source *fullSyncExtraFile
	target *fullSyncFile
	item   *CopyItem
}

type fullSyncPlan struct {
	src *fullSyncSnapshot
	dst *fullSyncSnapshot

	newDirs      map[string]fullSyncDir
	newFiles     map[string]*fullSyncFile
	changed      []*fullSyncFile
	extraFiles   map[string]*fullSyncExtraFile
	extraDeletes map[string]fullSyncDelete
	blockers     map[string]fullSyncDelete
}

func (jt *JobTask) syncFull(work scanWork, spec *ignore.GitIgnore) {
	// Full sync needs both complete trees before it mutates the destination;
	// otherwise an old path may be deleted before it can satisfy a moved file.
	var srcSnapshot, dstSnapshot *fullSyncSnapshot
	var srcErr, dstErr error

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		defer jt.recoverWorkerPanic("full-sync source scan", &srcErr)
		srcSnapshot, srcErr = jt.scanFullSyncTree(work.SrcPath, work.FirstDst, spec, true)
	}()
	go func() {
		defer wg.Done()
		defer jt.recoverWorkerPanic("full-sync destination scan", &dstErr)
		dstSnapshot, dstErr = jt.scanFullSyncTree(work.DstPath, work.FirstDst, spec, false)
	}()
	wg.Wait()

	if srcErr != nil || jt.isBreak() {
		return
	}
	if dstErr != nil {
		// The destination root may simply not exist yet on a first run. Create it
		// and rescan once; anything else is a real failure.
		if !jt.ensureDstDirAfterListError(work.DstPath, dstErr) {
			return
		}
		dstSnapshot, dstErr = jt.scanFullSyncTree(work.DstPath, work.FirstDst, spec, false)
		if dstErr != nil || jt.isBreak() {
			return
		}
	}

	plan := newFullSyncPlan(srcSnapshot, dstSnapshot)
	plan.build(jt.Job)
	jt.executeFullSyncPlan(plan)
}

func (jt *JobTask) scanFullSyncTree(root string, firstDst bool, spec *ignore.GitIgnore, isSrc bool) (*fullSyncSnapshot, error) {
	root = normalizeDirPath(root)
	snapshot := &fullSyncSnapshot{
		root: root,
		dirs: make(map[string]FileListResult),
	}

	var firstErr error
	var errMu sync.Mutex
	var scanDir func(string)
	scanDir = func(relDir string) {
		jt.ScanTotalDirs.Add(1)
		defer jt.finishScanWork()
		if jt.isBreak() {
			return
		}

		items, err := jt.listDir(root+relDir, firstDst, spec, root, isSrc)
		if err != nil {
			errMu.Lock()
			if firstErr == nil {
				firstErr = err
			}
			errMu.Unlock()
			return
		}
		snapshot.mu.Lock()
		snapshot.dirs[relDir] = items
		snapshot.mu.Unlock()

		children := make([]string, 0)
		for name := range items {
			if strings.HasSuffix(name, "/") {
				children = append(children, relDir+name)
			}
		}
		sort.Strings(children)

		var wg sync.WaitGroup
		for _, child := range children {
			child := child
			if jt.tryAcquireScanBranchSlot() {
				wg.Add(1)
				go func() {
					var childErr error
					defer wg.Done()
					defer jt.releaseScanBranchSlot()
					defer func() {
						if childErr == nil {
							return
						}
						errMu.Lock()
						if firstErr == nil {
							firstErr = childErr
						}
						errMu.Unlock()
					}()
					defer jt.recoverWorkerPanic("full-sync child scan", &childErr)
					scanDir(child)
				}()
				continue
			}
			scanDir(child)
		}
		wg.Wait()
	}

	scanDir("")
	return snapshot, firstErr
}

func newFullSyncPlan(src, dst *fullSyncSnapshot) *fullSyncPlan {
	return &fullSyncPlan{
		src:          src,
		dst:          dst,
		newDirs:      make(map[string]fullSyncDir),
		newFiles:     make(map[string]*fullSyncFile),
		extraFiles:   make(map[string]*fullSyncExtraFile),
		extraDeletes: make(map[string]fullSyncDelete),
		blockers:     make(map[string]fullSyncDelete),
	}
}

func (plan *fullSyncPlan) build(job map[string]interface{}) {
	plan.compareDir(job, "", "")
}

func (plan *fullSyncPlan) compareDir(job map[string]interface{}, srcRelDir, dstRelDir string) {
	srcItems := plan.src.dirs[srcRelDir]
	dstItems := plan.dst.dirs[dstRelDir]
	dstIndex := newDstNameMatchIndex(dstItems)
	srcIndex := newSrcNameMatchIndex(srcItems)
	matchedDst := make(map[string]struct{})
	srcDir := plan.src.root + srcRelDir
	dstDir := plan.dst.root + dstRelDir

	for _, name := range sortedFileListKeys(srcItems) {
		metadata := srcItems[name]
		if !strings.HasSuffix(name, "/") {
			// The size filter decides what gets copied, never what gets deleted.
			// Claim the matching destination entry before skipping so a file that
			// still exists at the source is not treated as an extra file and
			// deleted from the destination.
			allowed := jobAllowsFileSize(job, metadata.Size)
			if !allowed {
				if dstName, _, exists := dstIndex.find(name, srcIndex); exists {
					matchedDst[dstName] = struct{}{}
				}
				continue
			}

			if conflictName, conflictMeta, ok := dstIndex.find(name+"/", srcIndex); ok {
				matchedDst[conflictName] = struct{}{}
				plan.addBlocker(dstDir, conflictName, conflictMeta, fullSyncObjectPath(dstDir, name))
			}
			dstName, dstMetadata, exists := dstIndex.find(name, srcIndex)
			if exists {
				matchedDst[dstName] = struct{}{}
				if fileChanged(metadata, dstMetadata) {
					plan.changed = append(plan.changed, newFullSyncFile(srcDir, dstDir, name, metadata))
				}
				continue
			}
			plan.addNewFile(newFullSyncFile(srcDir, dstDir, name, metadata))
			continue
		}

		fileName := strings.TrimSuffix(name, "/")
		if conflictName, conflictMeta, ok := dstIndex.find(fileName, srcIndex); ok {
			matchedDst[conflictName] = struct{}{}
			plan.addBlocker(dstDir, conflictName, conflictMeta, fullSyncObjectPath(dstDir, fileName)+"/")
		}
		dstName, _, exists := dstIndex.find(name, srcIndex)
		if exists {
			matchedDst[dstName] = struct{}{}
			plan.compareDir(job, srcRelDir+name, dstRelDir+dstName)
			continue
		}
		plan.collectSourceOnly(job, srcRelDir+name, dstRelDir+name)
	}

	for _, name := range sortedFileListKeys(dstItems) {
		if _, matched := matchedDst[name]; matched {
			continue
		}
		metadata := dstItems[name]
		deleteRoot := plan.addExtraDelete(dstDir, name, metadata)
		if strings.HasSuffix(name, "/") {
			plan.collectDestinationFiles(dstRelDir+name, deleteRoot)
			continue
		}
		plan.addExtraFile(dstDir, name, metadata, deleteRoot)
	}
}

func (plan *fullSyncPlan) collectSourceOnly(job map[string]interface{}, srcRelDir, dstRelDir string) {
	srcDir := plan.src.root + srcRelDir
	dstDir := plan.dst.root + dstRelDir
	plan.newDirs[normalizeDirPath(dstDir)] = fullSyncDir{srcDir: srcDir, dstDir: dstDir}

	for _, name := range sortedFileListKeys(plan.src.dirs[srcRelDir]) {
		metadata := plan.src.dirs[srcRelDir][name]
		if strings.HasSuffix(name, "/") {
			plan.collectSourceOnly(job, srcRelDir+name, dstRelDir+name)
			continue
		}
		if jobAllowsFileSize(job, metadata.Size) {
			plan.addNewFile(newFullSyncFile(srcDir, dstDir, name, metadata))
		}
	}
}

func (plan *fullSyncPlan) collectDestinationFiles(dstRelDir, deleteRoot string) {
	dstDir := plan.dst.root + dstRelDir
	for _, name := range sortedFileListKeys(plan.dst.dirs[dstRelDir]) {
		metadata := plan.dst.dirs[dstRelDir][name]
		if strings.HasSuffix(name, "/") {
			plan.collectDestinationFiles(dstRelDir+name, deleteRoot)
			continue
		}
		plan.addExtraFile(dstDir, name, metadata, deleteRoot)
	}
}

func (plan *fullSyncPlan) addNewFile(file *fullSyncFile) {
	plan.newFiles[file.target] = file
}

func (plan *fullSyncPlan) addExtraFile(dir, name string, metadata FileMetadata, deleteRoot string) {
	object := fullSyncObjectPath(dir, name)
	plan.extraFiles[object] = &fullSyncExtraFile{
		dir:        dir,
		name:       name,
		metadata:   metadata,
		object:     object,
		deleteRoot: deleteRoot,
	}
}

func (plan *fullSyncPlan) addExtraDelete(dir, name string, metadata FileMetadata) string {
	object := fullSyncObjectPath(dir, name)
	plan.extraDeletes[object] = fullSyncDelete{
		dir:      dir,
		name:     name,
		metadata: metadata,
		object:   object,
	}
	return object
}

func (plan *fullSyncPlan) addBlocker(dir, name string, metadata FileMetadata, blocksPath string) {
	object := fullSyncObjectPath(dir, name)
	plan.blockers[object] = fullSyncDelete{
		dir:        dir,
		name:       name,
		metadata:   metadata,
		object:     object,
		blocksPath: blocksPath,
	}
}

func newFullSyncFile(srcDir, dstDir, name string, metadata FileMetadata) *fullSyncFile {
	return &fullSyncFile{
		srcDir:   srcDir,
		dstDir:   dstDir,
		name:     name,
		metadata: metadata,
		target:   fullSyncObjectPath(dstDir, name),
	}
}

func (jt *JobTask) executeFullSyncPlan(plan *fullSyncPlan) {
	blocked := make(map[string]struct{})
	// Remove type conflicts first so missing destination directories can be
	// created before target-side relocations begin.
	for _, key := range sortedDeleteKeys(plan.blockers, false) {
		item := plan.blockers[key]
		if jt.delFile(item.dir, item.name, item.metadata.Size) != taskStatusSuccess {
			blocked[normalizeBlockedPath(item.blocksPath)] = struct{}{}
		}
	}

	for _, key := range sortedDirKeys(plan.newDirs) {
		item := plan.newDirs[key]
		if fullSyncPathBlocked(item.dstDir, blocked) {
			continue
		}
		if jt.createFullSyncDir(item) != taskStatusSuccess {
			blocked[normalizeBlockedPath(item.dstDir)] = struct{}{}
		}
	}

	relocations := plan.relocations()
	active := make([]fullSyncRelocation, 0, len(relocations))
	protectedDeletes := make(map[string]struct{})
	for _, relocation := range relocations {
		if fullSyncPathBlocked(relocation.target.target, blocked) {
			protectedDeletes[relocation.source.deleteRoot] = struct{}{}
			continue
		}
		relocation.item = newCopyItem(
			jt,
			jt.AlistClient,
			relocation.source.dir,
			relocation.target.dstDir,
			relocation.source.name,
			relocation.source.metadata.Size,
			taskItemTypeMove,
		)
		active = append(active, relocation)
	}
	jt.runFullSyncRelocations(active)

	movedTargets := make(map[string]struct{})
	consumedFileDeletes := make(map[string]struct{})
	for _, relocation := range active {
		if relocation.item.status() == taskStatusSuccess {
			movedTargets[relocation.target.target] = struct{}{}
			if relocation.source.deleteRoot == relocation.source.object {
				consumedFileDeletes[relocation.source.deleteRoot] = struct{}{}
			}
			continue
		}
		protectedDeletes[relocation.source.deleteRoot] = struct{}{}
	}

	for _, file := range sortedFullSyncFiles(plan.changed) {
		if !fullSyncPathBlocked(file.target, blocked) {
			jt.queueCopyFile(file.srcDir, file.dstDir, file.name, file.metadata.Size, taskItemTypeCopy)
		}
	}
	for _, target := range sortedNewFileTargets(plan.newFiles) {
		file := plan.newFiles[target]
		if _, moved := movedTargets[target]; moved || fullSyncPathBlocked(file.target, blocked) {
			continue
		}
		jt.queueCopyFile(file.srcDir, file.dstDir, file.name, file.metadata.Size, taskItemTypeCopy)
	}

	for _, key := range sortedDeleteKeys(plan.extraDeletes, true) {
		if _, protected := protectedDeletes[key]; protected {
			continue
		}
		if _, consumed := consumedFileDeletes[key]; consumed {
			continue
		}
		item := plan.extraDeletes[key]
		jt.delFile(item.dir, item.name, item.metadata.Size)
	}
}

func (plan *fullSyncPlan) relocations() []fullSyncRelocation {
	targetGroups := make(map[string][]*fullSyncFile)
	for _, file := range plan.newFiles {
		key := fullSyncRelocationKey(file.name, file.metadata)
		if key != "" {
			targetGroups[key] = append(targetGroups[key], file)
		}
	}
	sourceGroups := make(map[string][]*fullSyncExtraFile)
	for _, file := range plan.extraFiles {
		key := fullSyncRelocationKey(file.name, file.metadata)
		if key != "" {
			sourceGroups[key] = append(sourceGroups[key], file)
		}
	}

	keys := make([]string, 0, len(targetGroups))
	for key := range targetGroups {
		if len(sourceGroups[key]) > 0 {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	var result []fullSyncRelocation
	for _, key := range keys {
		targets := targetGroups[key]
		sources := sourceGroups[key]
		sort.Slice(targets, func(i, j int) bool { return targets[i].target < targets[j].target })
		sort.Slice(sources, func(i, j int) bool { return sources[i].object < sources[j].object })
		count := len(targets)
		if len(sources) < count {
			count = len(sources)
		}
		for i := 0; i < count; i++ {
			result = append(result, fullSyncRelocation{source: sources[i], target: targets[i]})
		}
	}
	return result
}

// runFullSyncRelocations starts the destination-side moves and waits for them.
// The caller inspects each item's terminal status to decide which deletes are
// safe, so this has to be synchronous. Admission goes through the shared
// in-flight budget: this path runs on the scan goroutine while the submit
// executor is also starting copies, and both honouring the limit independently
// would put twice the configured concurrency on the AList server. Waiting is
// scoped to this batch so unrelated copies do not serialize it.
func (jt *JobTask) runFullSyncRelocations(relocations []fullSyncRelocation) {
	if len(relocations) == 0 {
		return
	}
	limit := runtimeTaskLimits().CopyConcurrency
	if limit < 1 {
		limit = 1
	}

	var batch sync.WaitGroup
	for i := range relocations {
		if jt.isBreak() {
			break
		}
		if !jt.waitForCopySlot(limit) {
			break
		}
		jt.startCopyItemTracked(relocations[i].item, &batch)
	}
	batch.Wait()
}

func (jt *JobTask) createFullSyncDir(item fullSyncDir) taskStatus {
	status := taskStatusSuccess
	var errMsg *string
	err := jt.AlistClient.MkdirContext(jt.context(), item.dstDir, util.ToInt(jt.Job["scanIntervalT"]))
	if err != nil {
		status = taskStatusFailed
		message := err.Error()
		errMsg = &message
	}
	jt.CopyHook(item.srcDir, item.dstDir, "", nil, "", status, errMsg, taskItemPath, taskItemTypeCopy, time.Now().Unix())
	return status
}

func fullSyncRelocationKey(name string, metadata FileMetadata) string {
	// Size-only matches are deliberately excluded: moving the wrong target file
	// is more damaging than falling back to the existing copy-and-delete path.
	// The digest also has to be well-formed, so a driver that returns a constant
	// placeholder for every file cannot make unrelated files look interchangeable.
	md5 := normalizeMD5(metadata.MD5)
	if !isMD5Digest(md5) || strings.Contains(name, "/") {
		return ""
	}
	return fmt.Sprintf("%s\x00%d\x00%s", name, metadata.Size, md5)
}

func fullSyncObjectPath(dir, name string) string {
	return path.Join(strings.TrimSpace(dir), strings.TrimSuffix(name, "/"))
}

func normalizeBlockedPath(value string) string {
	return path.Clean(strings.TrimSpace(value))
}

func fullSyncPathBlocked(value string, blocked map[string]struct{}) bool {
	value = normalizeBlockedPath(value)
	for prefix := range blocked {
		if value == prefix || strings.HasPrefix(value, prefix+"/") {
			return true
		}
	}
	return false
}

func sortedFileListKeys(items FileListResult) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedDirKeys(items map[string]fullSyncDir) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		leftDepth := strings.Count(strings.Trim(keys[i], "/"), "/")
		rightDepth := strings.Count(strings.Trim(keys[j], "/"), "/")
		if leftDepth == rightDepth {
			return keys[i] < keys[j]
		}
		return leftDepth < rightDepth
	})
	return keys
}

func sortedDeleteKeys(items map[string]fullSyncDelete, deepestFirst bool) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		leftDepth := strings.Count(strings.Trim(keys[i], "/"), "/")
		rightDepth := strings.Count(strings.Trim(keys[j], "/"), "/")
		if leftDepth == rightDepth {
			return keys[i] < keys[j]
		}
		if deepestFirst {
			return leftDepth > rightDepth
		}
		return leftDepth < rightDepth
	})
	return keys
}

func sortedNewFileTargets(items map[string]*fullSyncFile) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedFullSyncFiles(items []*fullSyncFile) []*fullSyncFile {
	result := append([]*fullSyncFile(nil), items...)
	sort.Slice(result, func(i, j int) bool { return result[i].target < result[j].target })
	return result
}
