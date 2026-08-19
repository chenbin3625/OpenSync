package service

import (
	"context"
	"errors"
	"fmt"
	"opensync/internal/msg"
	"opensync/pkg/util"
	"strings"
	"sync"
	"time"
)

type copyTaskWatch struct {
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
	jt      *JobTask
	mu      sync.Mutex
	watches map[string]*copyTaskWatch
	stopCh  chan struct{}
	once    sync.Once
	wg      sync.WaitGroup
	// stopped is set under mu once the monitor loop has exited (task broken,
	// timed out, or shut down). A track() call that arrives after the loop has
	// exited must not enqueue a watch nobody will ever process, otherwise the
	// copy goroutine blocks forever on <-watch.done and the job stays "doing".
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
	m.mu.Lock()
	if m.stopped {
		// The loop has already exited; enqueuing this watch would block the
		// copy goroutine forever. Abort the remote task ourselves instead.
		abortSelf = true
	} else {
		m.watches[m.watchKey(taskID, ci.CopyType)] = watch
	}
	m.mu.Unlock()
	if abortSelf {
		watch.ci.stopRemoteTask(m.jt.copyMonitorClient(), m.jt.context().Err())
		return
	}

	m.once.Do(func() {
		m.wg.Add(1)
		go m.loop()
	})

	select {
	case <-watch.done:
	case <-m.jt.context().Done():
		// Task cancelled while waiting for the monitor to process the watch.
		// Self-abort so the copy goroutine can proceed without waiting for the
		// loop's next iteration. abortWatch is a no-op if the loop already
		// took the watch, and closeDone is idempotent.
		m.abortWatch(watch, m.jt.context().Err())
	}
}

func (m *copyTaskMonitor) stop() {
	select {
	case <-m.stopCh:
	default:
		close(m.stopCh)
	}
	m.mu.Lock()
	m.stopped = true
	m.mu.Unlock()
	m.wg.Wait()
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
				return
			case <-time.After(200 * time.Millisecond):
				continue
			}
		}

		if !m.waitForPollInterval() {
			m.abortAll(m.jt.context().Err())
			return
		}

		// Prefer the bulk "undone" snapshot only when it earns its cost: with a
		// single watched task a direct per-task info call is cheaper, and even
		// with several tasks a busy AList host can return a large unrelated
		// list that we then filter down. fetchUndoneByType already keeps only
		// the task IDs this job cares about.
		var undoneByType map[taskItemType]map[string]map[string]interface{}
		if len(active) > 1 {
			undoneByType = m.fetchUndoneByType(active)
		}
		for _, watch := range active {
			if m.jt.isBreak() {
				m.abortWatch(watch, nil)
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
	needed := make(map[taskItemType]map[string]struct{})
	for _, watch := range active {
		if needed[watch.copyType] == nil {
			needed[watch.copyType] = make(map[string]struct{})
		}
		needed[watch.copyType][watch.taskID] = struct{}{}
	}

	result := make(map[taskItemType]map[string]map[string]interface{}, len(needed))
	client := m.jt.copyMonitorClient()
	ctx := m.jt.context()
	for copyType, wantedIDs := range needed {
		tasks, err := client.TaskUndoneListContext(ctx, copyType)
		if err != nil {
			continue
		}
		byID := make(map[string]map[string]interface{}, len(tasks))
		picked := 0
		for _, task := range tasks {
			id := fmt.Sprintf("%v", task["id"])
			if id == "" {
				continue
			}
			// Keep only the tasks this job actually watches; everything else
			// (other jobs, other clients, or a stale task list) is dropped so
			// a busy AList host cannot make us buffer an unbounded list.
			if _, wanted := wantedIDs[id]; !wanted {
				continue
			}
			byID[id] = task
			picked++
			if picked >= maxUndonePickups {
				break
			}
		}
		result[copyType] = byID
	}
	return result
}

func (m *copyTaskMonitor) applyTaskInfo(watch *copyTaskWatch, taskInfo map[string]interface{}) bool {
	state := taskStatusFromValue(taskInfo["state"])
	progress := util.ToFloat64(taskInfo["progress"])
	errStr := ""
	if e, ok := taskInfo["error"]; ok && e != nil {
		errStr = fmt.Sprintf("%v", e)
	}

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
	client := m.jt.copyMonitorClient()
	taskInfo, err := client.TaskInfoContext(m.jt.context(), watch.taskID, watch.copyType)
	if err != nil {
		if errors.Is(err, context.Canceled) && m.jt.isBreak() {
			return false
		}
		eMsg := err.Error()
		if strings.Contains(eMsg, "404") {
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
	key := m.watchKey(watch.taskID, watch.copyType)
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
	ctx, cancel := m.jt.cleanupContext()
	_ = m.jt.copyMonitorClient().TaskDeleteContext(ctx, watch.taskID, watch.copyType)
	cancel()
	watch.closeDone()
}

func (m *copyTaskMonitor) abortWatch(watch *copyTaskWatch, cause error) {
	if !m.takeWatch(watch) {
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
