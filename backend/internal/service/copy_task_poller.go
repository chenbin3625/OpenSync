package service

import (
	"fmt"
	"opensync/internal/msg"
	"opensync/pkg/util"
	"sync"
	"time"
)

type copyTaskWatch struct {
	key           string
	ci            *CopyItem
	taskID        string
	copyType      taskItemType
	done          chan struct{}
	closeOnce     sync.Once
	transientErrs int
}

func (watch *copyTaskWatch) closeDone() {
	watch.closeOnce.Do(func() {
		close(watch.done)
	})
}

type copyTaskMonitor struct {
	jt       *JobTask
	mu       sync.Mutex
	watches  map[string]*copyTaskWatch
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
	started  bool
	nextSeq  uint64
	// stopped is set under mu once the monitor is shutting down or has exited
	// (task broken, timed out, or shut down). A track() call that arrives after
	// stopped is set must not enqueue a watch or start loop(), otherwise the copy
	// goroutine blocks forever on <-watch.done or races with wg.Wait().
	stopped bool
}

func (jt *JobTask) ensureCopyMonitor() *copyTaskMonitor {
	jt.runtimeMu.Lock()
	defer jt.runtimeMu.Unlock()
	jt.ensureRuntimeLocked()
	if jt.copyMonitor != nil {
		return jt.copyMonitor
	}
	jt.copyMonitor = &copyTaskMonitor{
		jt:      jt,
		watches: make(map[string]*copyTaskWatch),
		stopCh:  make(chan struct{}),
	}
	return jt.copyMonitor
}

func (jt *JobTask) waitForRemoteCopyCompletion(ci *CopyItem) {
	monitor := jt.ensureCopyMonitor()
	monitor.track(ci)
}

func (jt *JobTask) stopCopyMonitor() {
	jt.runtimeMu.Lock()
	monitor := jt.copyMonitor
	jt.runtimeMu.Unlock()
	if monitor == nil {
		return
	}
	monitor.stop()
}

func (m *copyTaskMonitor) watchKey(taskID string, copyType taskItemType) string {
	return fmt.Sprintf("%s|%d", taskID, copyType.Int())
}

func (m *copyTaskMonitor) track(ci *CopyItem) {
	taskID := ci.taskID()
	if taskID == "" {
		return
	}

	watch := &copyTaskWatch{
		ci:       ci,
		taskID:   taskID,
		copyType: ci.CopyType,
		done:     make(chan struct{}),
	}

	abortSelf := false
	startLoop := false
	m.mu.Lock()
	if m.stopped || m.jt.isBreak() || m.jt.context().Err() != nil {
		// The monitor is stopping/stopped or the task is already broken; abort the
		// remote task directly instead of enqueuing a watch or racing with stop().
		abortSelf = true
	} else {
		m.nextSeq++
		watch.key = fmt.Sprintf("%s|%d|%d", taskID, ci.CopyType.Int(), m.nextSeq)
		m.watches[watch.key] = watch
		if !m.started {
			m.started = true
			m.wg.Add(1)
			startLoop = true
		}
	}
	m.mu.Unlock()

	if abortSelf {
		watch.ci.stopRemoteTask(m.jt.copyMonitorClient(), m.jt.context().Err())
		return
	}
	if startLoop {
		go m.loop()
	}

	select {
	case <-watch.done:
	case <-m.jt.context().Done():
		// Task cancelled while waiting for the monitor to process the watch.
		// Self-abort if still owned by m.watches; if loop() already claimed the
		// watch, abortWatch waits for <-watch.done so stopRemoteTask finishes
		// setting taskStatusStopped before track() returns.
		m.abortWatch(watch, m.jt.context().Err())
	}
}

func (m *copyTaskMonitor) stop() {
	m.mu.Lock()
	m.stopped = true
	m.stopOnce.Do(func() {
		close(m.stopCh)
	})
	m.mu.Unlock()

	m.wg.Wait()
	m.abortAll(m.jt.context().Err())
}

func (m *copyTaskMonitor) loop() {
	defer m.wg.Done()
	for {
		if m.jt.isBreak() || m.jt.context().Err() != nil {
			m.abortAll(m.jt.context().Err())
			return
		}

		active := m.snapshotWatches()
		if len(active) == 0 {
			select {
			case <-m.stopCh:
				m.abortAll(m.jt.context().Err())
				return
			case <-time.After(200 * time.Millisecond):
				continue
			}
		}

		if !m.waitForPollInterval() {
			m.abortAll(m.jt.context().Err())
			return
		}

		undoneByType := m.fetchUndoneByType(active)
		for _, watch := range active {
			if m.jt.isBreak() {
				m.abortWatch(watch, nil)
				continue
			}
			taskInfo, ok := undoneByType[watch.copyType][watch.taskID]
			if ok {
				m.applyTaskInfo(watch, taskInfo)
				continue
			}
			m.pollTaskInfo(watch)
		}

		select {
		case <-m.stopCh:
			m.abortAll(m.jt.context().Err())
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

func (m *copyTaskMonitor) waitForPollInterval() bool {
	cuTime := time.Now().Unix()
	var sleepFor time.Duration
	if cuTime-m.jt.lastWatchingUnix() < 3 {
		sleepFor = 610 * time.Millisecond
	} else {
		sleepFor = 2930 * time.Millisecond
	}
	return m.jt.waitForBreak(sleepFor)
}

func (m *copyTaskMonitor) fetchUndoneByType(active []*copyTaskWatch) map[taskItemType]map[string]map[string]interface{} {
	needed := make(map[taskItemType]struct{})
	for _, watch := range active {
		needed[watch.copyType] = struct{}{}
	}

	result := make(map[taskItemType]map[string]map[string]interface{}, len(needed))
	client := m.jt.copyMonitorClient()
	ctx := m.jt.context()
	for copyType := range needed {
		tasks, err := client.TaskUndoneListContext(ctx, copyType)
		if err != nil {
			continue
		}
		byID := make(map[string]map[string]interface{}, len(tasks))
		for _, task := range tasks {
			id := fmt.Sprintf("%v", task["id"])
			if id == "" {
				continue
			}
			byID[id] = task
		}
		result[copyType] = byID
	}
	return result
}

func (m *copyTaskMonitor) applyTaskInfo(watch *copyTaskWatch, taskInfo map[string]interface{}) bool {
	state := mapAlistTaskState(util.ToInt(taskInfo["state"]))
	progress := clampProgress(util.ToFloat64(taskInfo["progress"]))
	errStr := ""
	if e, ok := taskInfo["error"]; ok && e != nil {
		errStr = fmt.Sprintf("%v", e)
	}

	isTerminal := state == taskStatusSuccess || state == taskStatusStopped || state == taskStatusFailed

	watch.ci.mu.RLock()
	curErr := ""
	if watch.ci.ErrMsg != nil {
		curErr = *watch.ci.ErrMsg
	}
	unchanged := state == watch.ci.Status && progress == watch.ci.Progress && errStr == curErr
	watch.ci.mu.RUnlock()

	if !unchanged {
		if errStr != "" {
			watch.ci.setProgress(state, progress, &errStr)
		} else {
			watch.ci.setProgress(state, progress, nil)
		}
	}

	if isTerminal {
		m.finishWatch(watch)
		return true
	}
	return false
}

func (m *copyTaskMonitor) pollTaskInfo(watch *copyTaskWatch) bool {
	client := m.jt.copyMonitorClient()
	taskInfo, err := client.TaskInfoContext(m.jt.context(), watch.taskID, watch.copyType)
	if err != nil {
		if m.jt.isBreak() {
			return false
		}
		eMsg := err.Error()
		if isAlistObjectNotFound(err) {
			if exists, verr := watch.ci.verifyDstExists(m.jt, client); verr == nil && exists {
				watch.ci.setProgress(taskStatusSuccess, 100, nil)
				m.finishWatch(watch)
				return true
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
		// Polling failed repeatedly; cancel the remote task before failing so an
		// automatic retry cannot run concurrently with a still-active remote job.
		// If cancel fails, block automatic retry for this item.
		cancelCtx, cancel := m.jt.cleanupContext()
		cancelErr := client.TaskCancelContext(cancelCtx, watch.taskID, watch.copyType)
		cancel()
		if cancelErr != nil {
			watch.ci.setRetryBlocked(true)
		}
		watch.ci.setProgress(taskStatusFailed, 0, &eMsg)
		m.finishWatch(watch)
		return true
	}
	watch.transientErrs = 0
	return m.applyTaskInfo(watch, taskInfo)
}

// mapAlistTaskState converts an AList admin-task state (tache.State: 0 pending,
// 1 running, 2 succeeded, 4 canceled, 7 failed; 5 and other values are interim
// retry states) to the local task status. Non-terminal states map to Running so
// the watch keeps polling until a terminal state arrives.
func mapAlistTaskState(state int) taskStatus {
	switch state {
	case 0:
		return taskStatusWaiting
	case 2:
		return taskStatusSuccess
	case 4:
		return taskStatusStopped
	case 7:
		return taskStatusFailed
	default:
		return taskStatusRunning
	}
}

func (m *copyTaskMonitor) takeWatch(watch *copyTaskWatch) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := watch.key
	if key == "" {
		key = m.watchKey(watch.taskID, watch.copyType)
	}
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
	// The item already reached a terminal state, so delete the finished remote
	// task off the polling loop: a slow cleanup (up to the 30s cleanup timeout)
	// would otherwise delay status updates for the remaining watches.
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ctx, cancel := m.jt.cleanupContext()
		_ = m.jt.copyMonitorClient().TaskDeleteContext(ctx, watch.taskID, watch.copyType)
		cancel()
		watch.closeDone()
	}()
}

func (m *copyTaskMonitor) abortWatch(watch *copyTaskWatch, cause error) {
	if !m.takeWatch(watch) {
		// Another owner (loop's abortAll or finishWatch) already claimed this
		// watch; wait until it finishes updating watch.ci and closes watch.done.
		<-watch.done
		return
	}
	watch.ci.stopRemoteTask(m.jt.copyMonitorClient(), cause)
	watch.closeDone()
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

	if len(watches) == 0 {
		return
	}

	// Cancel/delete remote tasks in parallel so shutdown is gated by the
	// slowest single cleanup (not the sum). closeDone stays after stopRemoteTask
	// per watch to preserve the existing status-before-unblock ordering.
	var wg sync.WaitGroup
	for _, watch := range watches {
		wg.Add(1)
		go func(w *copyTaskWatch) {
			defer wg.Done()
			w.ci.stopRemoteTask(m.jt.copyMonitorClient(), cause)
			w.closeDone()
		}(watch)
	}
	wg.Wait()
}
