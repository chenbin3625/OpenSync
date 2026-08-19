import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { jobGetTaskCurrent } from '../../api/job';
import { POLL_INTERVAL_MS } from '../../api/request';
import type { CurrentTaskView, TaskItem } from '../../types';
import {
  getRealtimeTaskIdentity,
  mergeTaskItems,
  normalizeTaskItemPage,
  shouldReplaceRealtimeRows,
  shouldResetRealtimeSnapshot,
  sortTaskItemsByCreateTimeDesc,
  type RealtimeTaskLoadKey,
} from './taskRows';

// Non-running tabs (success/fail/other/... ) fetch from the DB-backed server
// page; slowing them down avoids hammering the sqlite aggregation queries on
// very large task item sets while each tab is simply being watched.
const NON_RUNNING_POLL_INTERVAL_MS = 15000;

type RealtimeTaskItemsParams = {
  jobId: string;
  enabled: boolean;
  currentTask: CurrentTaskView | null;
  pageSize: number;
};

export function useRealtimeTaskItems({
  jobId,
  enabled,
  currentTask,
  pageSize,
}: RealtimeTaskItemsParams): {
  activeTab: number;
  setActiveTab: (status: number) => void;
  tabTaskList: TaskItem[];
  pagedTabTaskList: TaskItem[];
  tabTaskTotal: number;
  tabTaskPage: number;
  setTabTaskPage: (page: number) => void;
  tabLoading: boolean;
} {
  const [activeTab, setActiveTabValue] = useState(1);
  const [tabTaskList, setTabTaskList] = useState<TaskItem[]>([]);
  const [tabTaskTotal, setTabTaskTotal] = useState(0);
  const [tabTaskPage, setTabTaskPageValue] = useState(1);
  const [tabLoading, setTabLoading] = useState(false);
  const requestRef = useRef(0);
  const lastLoadedRef = useRef<RealtimeTaskLoadKey | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const tabFetchingRef = useRef(false);
  const lastFetchKeyRef = useRef<string | null>(null);
  const lastFetchAtRef = useRef<number | null>(null);

  const setActiveTab = useCallback((status: number) => {
    setActiveTabValue(status);
    setTabTaskPageValue(1);
  }, []);

  const setTabTaskPage = useCallback((page: number) => {
    setTabTaskPageValue(page);
  }, []);

  useEffect(() => {
    if (!enabled || !jobId || !currentTask) {
      requestRef.current += 1;
      lastLoadedRef.current = null;
      lastFetchKeyRef.current = null;
      lastFetchAtRef.current = null;
      abortRef.current?.abort();
      setTabTaskList([]);
      setTabTaskTotal(0);
      setTabLoading(false);
      return;
    }

    const taskIdentity = getRealtimeTaskIdentity(currentTask);
    const lastLoaded = lastLoadedRef.current;
    const mustResetPage = !lastLoaded ||
      lastLoaded.status !== activeTab ||
      lastLoaded.taskIdentity !== taskIdentity;

    if (mustResetPage && tabTaskPage !== 1) {
      requestRef.current += 1;
      lastFetchKeyRef.current = null;
      lastFetchAtRef.current = null;
      abortRef.current?.abort();
      setTabTaskList([]);
      setTabTaskTotal(0);
      setTabLoading(true);
      setTabTaskPageValue(1);
      return;
    }

    const loadKey = { status: activeTab, taskIdentity, page: tabTaskPage };
    const replaceRows = shouldReplaceRealtimeRows(lastLoaded, loadKey);
    const resetSnapshot = shouldResetRealtimeSnapshot(lastLoaded, loadKey);

    if (activeTab === 1) {
      // Invalidate any in-flight server fetch and reset the fetch throttle so
      // returning to a non-running tab always fetches fresh.
      requestRef.current += 1;
      lastFetchKeyRef.current = null;
      lastFetchAtRef.current = null;
      abortRef.current?.abort();
      lastLoadedRef.current = loadKey;
      const doingTask = currentTask.doingTask || [];
      setTabTaskList((previous) => replaceRows ? doingTask : mergeTaskItems(previous, doingTask));
      setTabTaskTotal(doingTask.length);
      setTabLoading(false);
      return;
    }

    // Non-running tabs fetch server-side rows. Throttle to at most one request
    // per poll interval per view: currentTask changes on every SSE push, and
    // refetching on each push would pile up requests against a slow backend.
    // A fresh view (new tab / task / page) always fetches immediately.
    const fetchKey = `${loadKey.status}:${loadKey.taskIdentity}:${loadKey.page}`;
    const now = Date.now();
    const changedView = lastFetchKeyRef.current !== fetchKey;
    const pollIntervalMs = activeTab === 1 ? POLL_INTERVAL_MS : NON_RUNNING_POLL_INTERVAL_MS;
    if (!changedView && lastFetchAtRef.current != null && now - lastFetchAtRef.current < pollIntervalMs) {
      return;
    }
    // Skip the whole run while a request is still in flight: the in-flight
    // request completes and updates state instead of being cancelled, and the
    // next push/tick (changedView stays true) fetches any new view. Guarding
    // here — before any state mutation — avoids clearing the view and leaving
    // a stuck loading spinner when a tab/page switch races a slow fetch.
    if (tabFetchingRef.current) return;
    lastFetchKeyRef.current = fetchKey;
    lastFetchAtRef.current = now;

    const requestID = ++requestRef.current;
    lastLoadedRef.current = loadKey;

    if (replaceRows) {
      if (resetSnapshot) setTabTaskList([]);
      if (resetSnapshot) setTabTaskTotal(0);
      setTabLoading(true);
    }

    const controller = new AbortController();
    abortRef.current = controller;

    async function loadTabTasks() {
      tabFetchingRef.current = true;
      try {
        const res = await jobGetTaskCurrent({
          id: jobId,
          status: activeTab,
          pageSize,
          pageNum: tabTaskPage,
        }, { silent: true, signal: controller.signal });
        if (controller.signal.aborted || requestID !== requestRef.current) return;
        const { rows, total } = normalizeTaskItemPage(res.data);
        setTabTaskList((previous) => replaceRows ? rows : mergeTaskItems(previous, rows));
        setTabTaskTotal(total);
      } catch {
        if (controller.signal.aborted) return;
        if (requestID === requestRef.current && replaceRows) {
          if (resetSnapshot) setTabTaskList([]);
          if (resetSnapshot) setTabTaskTotal(0);
        }
      } finally {
        tabFetchingRef.current = false;
        if (requestID === requestRef.current) {
          setTabLoading(false);
        }
      }
    }

    loadTabTasks();
  }, [activeTab, currentTask, enabled, jobId, pageSize, tabTaskPage]);

  // Abort any in-flight tab fetch when the component unmounts (job switch /
  // tab teardown remounts TaskList via `key`).
  useEffect(() => () => { abortRef.current?.abort(); }, []);

  const pagedTabTaskList = useMemo(() => {
    const sortedList = sortTaskItemsByCreateTimeDesc(tabTaskList);
    if (activeTab !== 1) return sortedList;
    const start = (tabTaskPage - 1) * pageSize;
    return sortedList.slice(start, start + pageSize);
  }, [activeTab, pageSize, tabTaskList, tabTaskPage]);

  useEffect(() => {
    const totalRows = activeTab === 1 ? tabTaskList.length : tabTaskTotal;
    const maxPage = Math.max(1, Math.ceil(totalRows / pageSize));
    if (tabTaskPage > maxPage) {
      setTabTaskPageValue(maxPage);
    }
  }, [activeTab, pageSize, tabTaskList.length, tabTaskPage, tabTaskTotal]);

  return {
    activeTab,
    setActiveTab,
    tabTaskList,
    pagedTabTaskList,
    tabTaskTotal,
    tabTaskPage,
    setTabTaskPage,
    tabLoading,
  };
}
