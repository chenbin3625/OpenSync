import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { jobGetTaskCurrent } from '../../api/job';
import type { CurrentTaskView, TaskItem } from '../../types';
import { canPollCurrentDocument } from './pollingVisibility';
import {
  getRealtimeTaskIdentity,
  mergeTaskItems,
  normalizeTaskItemPage,
  pageTaskItems,
  shouldReplaceRealtimeRows,
  shouldResetRealtimeSnapshot,
  type RealtimeTaskLoadKey,
} from './taskRows';

// Non-running tabs (success/fail/other/...) fetch from the DB-backed server
// page; polling them on a slower independent interval keeps them fresh while
// avoiding hammering sqlite aggregation queries on every SSE tick.
const NON_RUNNING_POLL_INTERVAL_MS = 15000;
const emptyTaskItems: TaskItem[] = [];

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
  tabError: boolean;
  retryTabTasks: () => void;
} {
  const [activeTab, setActiveTabValue] = useState(1);
  const [serverRows, setServerRows] = useState<TaskItem[]>(emptyTaskItems);
  const [serverTotal, setServerTotal] = useState(0);
  const [tabTaskPage, setTabTaskPageValue] = useState(1);
  const [serverLoading, setServerLoading] = useState(false);
  const [serverError, setServerError] = useState(false);
  const [retryNonce, setRetryNonce] = useState(0);
  const requestRef = useRef(0);
  const lastLoadedRef = useRef<RealtimeTaskLoadKey | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const tabFetchingRef = useRef(false);

  const taskIdentity = currentTask ? getRealtimeTaskIdentity(currentTask) : '';

  const setActiveTab = useCallback((status: number) => {
    setActiveTabValue(status);
    setTabTaskPageValue(1);
  }, []);

  const setTabTaskPage = useCallback((page: number) => {
    setTabTaskPageValue(page);
  }, []);

  const retryTabTasks = useCallback(() => {
    setRetryNonce((nonce) => nonce + 1);
  }, []);

  useEffect(() => {
    if (!enabled || !jobId || !taskIdentity) {
      requestRef.current += 1;
      lastLoadedRef.current = null;
      abortRef.current?.abort();
      abortRef.current = null;
      tabFetchingRef.current = false;
      setServerRows(emptyTaskItems);
      setServerTotal(0);
      setServerError(false);
      setServerLoading(false);
      return undefined;
    }

    const lastLoaded = lastLoadedRef.current;
    const mustResetPage = !lastLoaded ||
      lastLoaded.status !== activeTab ||
      lastLoaded.taskIdentity !== taskIdentity;

    if (mustResetPage && tabTaskPage !== 1) {
      requestRef.current += 1;
      abortRef.current?.abort();
      abortRef.current = null;
      tabFetchingRef.current = false;
      setServerRows(emptyTaskItems);
      setServerTotal(0);
      setServerError(false);
      setServerLoading(activeTab !== 1);
      setTabTaskPageValue(1);
      return undefined;
    }

    const loadKey = { status: activeTab, taskIdentity, page: tabTaskPage };
    const replaceRows = shouldReplaceRealtimeRows(lastLoaded, loadKey);
    const resetSnapshot = shouldResetRealtimeSnapshot(lastLoaded, loadKey);

    if (activeTab === 1) {
      requestRef.current += 1;
      abortRef.current?.abort();
      abortRef.current = null;
      tabFetchingRef.current = false;
      setServerError(false);
      setServerLoading(false);
      lastLoadedRef.current = loadKey;
      return undefined;
    }

    lastLoadedRef.current = loadKey;
    if (resetSnapshot) {
      setServerRows(emptyTaskItems);
      setServerTotal(0);
    }

    const loadTabTasks = async (options: { replace: boolean; showLoading: boolean }) => {
      if (!options.showLoading && tabFetchingRef.current) return;
      if (options.showLoading && tabFetchingRef.current) {
        abortRef.current?.abort();
      }

      const requestID = ++requestRef.current;
      const controller = new AbortController();
      abortRef.current = controller;
      tabFetchingRef.current = true;

      if (options.showLoading) {
        setServerError(false);
        setServerLoading(true);
      }

      try {
        const res = await jobGetTaskCurrent({
          id: jobId,
          status: activeTab,
          pageSize,
          pageNum: tabTaskPage,
        }, { silent: true, signal: controller.signal });
        if (controller.signal.aborted || requestID !== requestRef.current) return;
        const { rows, total } = normalizeTaskItemPage(res.data);
        setServerRows((previous) => options.replace ? rows : mergeTaskItems(previous, rows));
        setServerTotal(total);
        setServerError(false);
      } catch {
        if (controller.signal.aborted || requestID !== requestRef.current) return;
        setServerError(true);
      } finally {
        if (abortRef.current === controller) {
          abortRef.current = null;
          tabFetchingRef.current = false;
        }
        if (requestID === requestRef.current) {
          setServerLoading(false);
        }
      }
    };

    void loadTabTasks({ replace: replaceRows, showLoading: true });

    const pollID = setInterval(() => {
      if (canPollCurrentDocument()) {
        void loadTabTasks({ replace: false, showLoading: false });
      }
    }, NON_RUNNING_POLL_INTERVAL_MS);

    return () => {
      clearInterval(pollID);
      requestRef.current += 1;
      abortRef.current?.abort();
      abortRef.current = null;
      tabFetchingRef.current = false;
    };
  }, [activeTab, enabled, jobId, pageSize, retryNonce, tabTaskPage, taskIdentity]);

  // Abort any in-flight tab fetch when the component unmounts.
  useEffect(() => () => {
    requestRef.current += 1;
    abortRef.current?.abort();
    abortRef.current = null;
    tabFetchingRef.current = false;
  }, []);

  const tabTaskList = activeTab === 1 ? currentTask?.doingTask ?? emptyTaskItems : serverRows;
  const tabTaskTotal = activeTab === 1 ? tabTaskList.length : serverTotal;
  const pagedTabTaskList = useMemo(() => {
    return pageTaskItems(tabTaskList, activeTab, tabTaskPage, pageSize);
  }, [activeTab, pageSize, tabTaskList, tabTaskPage]);

  useEffect(() => {
    const maxPage = Math.max(1, Math.ceil(tabTaskTotal / pageSize));
    if (tabTaskPage > maxPage) {
      setTabTaskPageValue(maxPage);
    }
  }, [pageSize, tabTaskPage, tabTaskTotal]);

  return {
    activeTab,
    setActiveTab,
    tabTaskList,
    pagedTabTaskList,
    tabTaskTotal,
    tabTaskPage,
    setTabTaskPage,
    tabLoading: activeTab !== 1 && serverLoading,
    tabError: activeTab !== 1 && serverError,
    retryTabTasks,
  };
}
