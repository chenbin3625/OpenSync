package service

import (
	"context"
	"errors"
	"fmt"
	"opensync/internal/msg"
	"strings"
	"sync"
	"time"
)

type copyTaskWatch struct {
	jt            *JobTask
	ci            *CopyItem
	taskID        string
	copyType      taskItemType
	done          chan struct{}
	closeOnce     sync.Once
	transientErrs int
}

// maxUndonePickups bounds how many watched tasks are picked up from a single
// undone-list snapshot. The watch set is already bounded by the copy
// concurrency, so this is a safety valve against pathological task lists.
const maxUndonePickups = 256

func (watch *copyTaskWatch) closeDone() {
	watch.closeOnce.Do(func() {
		close(watch.done)
	})
}

type copyTaskMonitor struct {
	mu      sync.Mutex
	watches map[string]*copyTaskWatch
	stopCh  chan struct{}
	wg      sync.WaitGroup
	started bool
	// stopped is set under mu once the monitor loop has exited (task broken,
	// timed out, or shut down). A track() call that arrives after the loop has
	// exited must not enqueue a watch nobody will ever process, otherwise the
	// copy goroutine blocks forever on <-watch.done and the job stays "doing".
	stopped bool
	shared  bool
	alistID int64
	refs    int
}

var copyMonitors = struct {
	mu      sync.Mutex
	byAlist map[int64]*copyTaskMonitor
}{
	byAlist: make(map[int64]*copyTaskMonitor),
}

func newCopyTaskMonitor() *copyTaskMonitor {
	return &copyTaskMonitor{
		watches: make(map[string]*copyTaskWatch),
		stopCh:  make(chan struct{}),
	}
}

func (jt *JobTask) ensureCopyMonitor() *copyTaskMonitor {
	jt.runtimeMu.Lock()
	defer jt.runtimeMu.Unlock()
	jt.ensureRuntimeLocked()
	if jt.copyMonitor != nil {
		return jt.copyMonitor
	}
	jt.copyMonitor = acquireCopyMonitor(jt)
	return jt.copyMonitor
}

func acquireCopyMonitor(jt *JobTask) *copyTaskMonitor {
	if jt.copyMonitorClientOverride != nil || jt.AlistClient == nil || jt.AlistClient.AlistID <= 0 {
		return newCopyTaskMonitor()
	}

	alistID := jt.AlistClient.AlistID
	copyMonitors.mu.Lock()
	defer copyMonitors.mu.Unlock()
	monitor := copyMonitors.byAlist[alistID]
	if monitor == nil {
		monitor = newCopyTaskMonitor()
		monitor.shared = true
		monitor.alistID = alistID
		copyMonitors.byAlist[alistID] = monitor
	}
	monitor.refs++
	return monitor
}

func (jt *JobTask) waitForRemoteCopyCompletion(ci *CopyItem) {
	monitor := jt.ensureCopyMonitor()
	monitor.track(jt, ci)
}

func (jt *JobTask) stopCopyMonitor() {
	jt.runtimeMu.Lock()
	monitor := jt.copyMonitor
	jt.copyMonitor = nil
	jt.runtimeMu.Unlock()
	if monitor == nil {
		return
	}
	monitor.release(jt)
}

func watchKey(jt *JobTask, taskID string, copyType taskItemType) string {
	var taskPK int64
	if jt != nil {
		taskPK = jt.TaskID
	}
	return fmt.Sprintf("%d|%s|%d", taskPK, taskID, copyType.Int())
}

func (m *copyTaskMonitor) track(jt *JobTask, ci *CopyItem) {
	taskID := ci.taskID()
	if taskID == "" {
		return
	}

	watch := &copyTaskWatch{
		jt:       jt,
		ci:       ci,
		taskID:   taskID,
		copyType: ci.CopyType,
		done:     make(chan struct{}),
	}

	abortSelf := false
	startLoop := false
	m.mu.Lock()
	if m.stopped {
		// The loop has already exited; enqueuing this watch would block the
		// copy goroutine forever. Abort the remote task ourselves instead.
		abortSelf = true
	} else {
		m.watches[watchKey(jt, taskID, ci.CopyType)] = watch
		if !m.started {
			// Add before releasing m.mu. stop() takes the same lock before
			// waiting, so it cannot observe a zero WaitGroup and return while
			// the loop is still about to start.
			m.started = true
			m.wg.Add(1)
			startLoop = true
		}
	}
	m.mu.Unlock()
	if abortSelf {
		watch.ci.stopRemoteTask(jt.copyMonitorClient(), jt.context().Err())
		return
	}

	if startLoop {
		go m.loop()
	}

	select {
	case <-watch.done:
	case <-jt.context().Done():
		// Task cancelled while waiting for the monitor to process the watch.
		// Self-abort so the copy goroutine can proceed without waiting for the
		// loop's next iteration. abortWatch is a no-op if the loop already
		// took the watch, and closeDone is idempotent.
		m.abortWatch(watch, jt.context().Err())
	}
}

func (m *copyTaskMonitor) stop() {
	m.mu.Lock()
	if !m.stopped {
		m.stopped = true
		close(m.stopCh)
	}
	m.mu.Unlock()
	m.wg.Wait()
}

func (m *copyTaskMonitor) release(jt *JobTask) {
	m.abortJob(jt)
	if !m.shared {
		m.stop()
		return
	}

	copyMonitors.mu.Lock()
	m.refs--
	stopNow := m.refs <= 0
	if stopNow && copyMonitors.byAlist[m.alistID] == m {
		delete(copyMonitors.byAlist, m.alistID)
	}
	copyMonitors.mu.Unlock()
	if stopNow {
		m.stop()
	}
}

func (m *copyTaskMonitor) loop() {
	defer m.wg.Done()
	for {
		select {
		case <-m.stopCh:
			m.abortAll(context.Canceled)
			return
		default:
		}

		active := m.snapshotWatches()
		if len(active) == 0 {
			timer := time.NewTimer(200 * time.Millisecond)
			select {
			case <-m.stopCh:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				m.abortAll(context.Canceled)
				return
			case <-timer.C:
				continue
			}
		}

		if !m.waitForPollInterval() {
			m.abortAll(context.Canceled)
			return
		}

		// Prefer the bulk "undone" snapshot only when it earns its cost: with a
		// single watched task a direct per-task info call is cheaper, and even
		// with several tasks a busy AList host can return a large unrelated
		// list that we then filter down. fetchUndoneByType already keeps only
		// the task IDs this job cares about. Sharing the monitor across jobs
		// on the same AList means N concurrent syncs issue one undone list
		// instead of N.
		var undoneByType map[taskItemType]map[string]alistRemoteTask
		if len(active) > 1 {
			undoneByType = m.fetchUndoneByType(active)
		}
		for _, watch := range active {
			if watch.jt.isBreak() || watch.jt.context().Err() != nil {
				m.abortWatch(watch, watch.jt.context().Err())
				continue
			}
			if undoneByType != nil {
				if taskInfo, ok := undoneByType[watch.copyType][watch.taskID]; ok {
					m.applyTaskInfo(watch, taskInfo)
					continue
				}
			}
			m.pollTaskInfo(watch)
		}

		select {
		case <-m.stopCh:
			m.abortAll(context.Canceled)
			return
		default:
		}
	}
}

func (m *copyTaskMonitor) snapshotWatches() []*copyTaskWatch {
	m.mu.Lock()
	defer m.mu.Unlock()
	active := make([]*copyTaskWatch, 0, len(m.watches))
	for _, watch := range m.watches {
		active = append(active, watch)
	}
	return active
}

func (m *copyTaskMonitor) anyViewerPresent() bool {
	now := time.Now().Unix()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, watch := range m.watches {
		if watch.jt != nil && now-watch.jt.lastWatchingUnix() < 3 {
			return true
		}
	}
	return false
}

func (m *copyTaskMonitor) waitForPollInterval() bool {
	sleepFor := 2930 * time.Millisecond
	if m.anyViewerPresent() {
		sleepFor = 610 * time.Millisecond
	}
	timer := time.NewTimer(sleepFor)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-m.stopCh:
		return false
	}
}

func (m *copyTaskMonitor) fetchUndoneByType(active []*copyTaskWatch) map[taskItemType]map[string]alistRemoteTask {
	needed := make(map[taskItemType]map[string]struct{})
	for _, watch := range active {
		if needed[watch.copyType] == nil {
			needed[watch.copyType] = make(map[string]struct{})
		}
		needed[watch.copyType][watch.taskID] = struct{}{}
	}

	result := make(map[taskItemType]map[string]alistRemoteTask, len(needed))
	if len(active) == 0 {
		return result
	}
	client := active[0].jt.copyMonitorClient()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if len(needed) == 1 {
		for copyType, wantedIDs := range needed {
			if byID := m.loadUndone(ctx, client, copyType, wantedIDs); byID != nil {
				result[copyType] = byID
			}
		}
		return result
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	for copyType, wantedIDs := range needed {
		wg.Add(1)
		go func(copyType taskItemType, wantedIDs map[string]struct{}) {
			defer wg.Done()
			byID := m.loadUndone(ctx, client, copyType, wantedIDs)
			if byID == nil {
				return
			}
			mu.Lock()
			result[copyType] = byID
			mu.Unlock()
		}(copyType, wantedIDs)
	}
	wg.Wait()
	return result
}

func (m *copyTaskMonitor) loadUndone(ctx context.Context, client copyItemClient, copyType taskItemType, wantedIDs map[string]struct{}) map[string]alistRemoteTask {
	if picker, ok := client.(interface {
		TaskUndoneByIDsContext(context.Context, taskItemType, map[string]struct{}, int) (map[string]alistRemoteTask, error)
	}); ok {
		byID, err := picker.TaskUndoneByIDsContext(ctx, copyType, wantedIDs, maxUndonePickups)
		if err != nil {
			return nil
		}
		return byID
	}
	tasks, err := client.TaskUndoneListContext(ctx, copyType)
	if err != nil {
		return nil
	}
	byID := make(map[string]alistRemoteTask, len(wantedIDs))
	picked := 0
	for _, task := range tasks {
		id := task.idString()
		if id == "" {
			continue
		}
		if _, wanted := wantedIDs[id]; !wanted {
			continue
		}
		byID[id] = task
		picked++
		if picked >= maxUndonePickups {
			break
		}
	}
	return byID
}

func (m *copyTaskMonitor) applyTaskInfo(watch *copyTaskWatch, taskInfo alistRemoteTask) bool {
	state := taskStatus(taskInfo.State)
	progress := taskInfo.Progress
	errStr := taskInfo.Error

	watch.ci.mu.RLock()
	unchanged := state == watch.ci.Status && progress == watch.ci.Progress
	watch.ci.mu.RUnlock()
	if unchanged {
		return false
	}
	if errStr != "" {
		watch.ci.setProgress(state, progress, &errStr)
	} else {
		watch.ci.setProgress(state, progress, nil)
	}

	if state == taskStatusSuccess || state == taskStatusStopped || state == taskStatusFailed {
		m.finishWatch(watch)
		return true
	}
	return false
}

func (m *copyTaskMonitor) pollTaskInfo(watch *copyTaskWatch) bool {
	client := watch.jt.copyMonitorClient()
	taskInfo, err := client.TaskInfoContext(watch.jt.context(), watch.taskID, watch.copyType)
	if err != nil {
		if errors.Is(err, context.Canceled) && watch.jt.isBreak() {
			return false
		}
		eMsg := err.Error()
		if strings.Contains(eMsg, "404") {
			exists, verr := watch.ci.verifyDstExists(watch.jt, client)
			if verr == nil && exists {
				watch.ci.setProgress(taskStatusSuccess, 100, nil)
				m.finishWatch(watch)
				return true
			}
			if verr != nil {
				watch.transientErrs++
				if watch.transientErrs < maxTransientPollErrors {
					return false
				}
			}
			eMsg = msg.TaskMayDelete
			watch.ci.setProgress(taskStatusFailed, 0, &eMsg)
			m.finishWatch(watch)
			return true
		}
		watch.transientErrs++
		if watch.transientErrs < maxTransientPollErrors {
			return false
		}
		watch.ci.setProgress(taskStatusFailed, 0, &eMsg)
		m.finishWatch(watch)
		return true
	}
	watch.transientErrs = 0
	return m.applyTaskInfo(watch, taskInfo)
}

func (m *copyTaskMonitor) takeWatch(watch *copyTaskWatch) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := watchKey(watch.jt, watch.taskID, watch.copyType)
	current, ok := m.watches[key]
	if !ok || current != watch {
		return false
	}
	delete(m.watches, key)
	return true
}

func (m *copyTaskMonitor) finishWatch(watch *copyTaskWatch) {
	if !m.takeWatch(watch) {
		return
	}
	ctx, cancel := watch.jt.cleanupContext()
	_ = watch.jt.copyMonitorClient().TaskDeleteContext(ctx, watch.taskID, watch.copyType)
	cancel()
	watch.closeDone()
}

func (m *copyTaskMonitor) abortWatch(watch *copyTaskWatch, cause error) {
	if !m.takeWatch(watch) {
		return
	}
	watch.ci.stopRemoteTask(watch.jt.copyMonitorClient(), cause)
	watch.closeDone()
}

func (m *copyTaskMonitor) abortJob(jt *JobTask) {
	m.mu.Lock()
	watches := make([]*copyTaskWatch, 0)
	for key, watch := range m.watches {
		if watch.jt == jt {
			delete(m.watches, key)
			watches = append(watches, watch)
		}
	}
	m.mu.Unlock()
	m.stopWatches(watches, jt.context().Err())
}

func (m *copyTaskMonitor) abortAll(cause error) {
	m.mu.Lock()
	m.stopped = true
	watches := make([]*copyTaskWatch, 0, len(m.watches))
	for _, watch := range m.watches {
		watches = append(watches, watch)
	}
	m.watches = make(map[string]*copyTaskWatch)
	m.mu.Unlock()
	m.stopWatches(watches, cause)
}

func (m *copyTaskMonitor) stopWatches(watches []*copyTaskWatch, cause error) {
	// Cancel/delete remote tasks in parallel so shutdown is gated by the
	// slowest single cleanup (not the sum). closeDone stays after stopRemoteTask
	// per watch to preserve the existing status-before-unblock ordering.
	var wg sync.WaitGroup
	for _, watch := range watches {
		wg.Add(1)
		go func(w *copyTaskWatch) {
			defer wg.Done()
			w.ci.stopRemoteTask(w.jt.copyMonitorClient(), cause)
			w.closeDone()
		}(watch)
	}
	wg.Wait()
}
