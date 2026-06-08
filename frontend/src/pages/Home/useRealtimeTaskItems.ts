import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { jobGetTaskCurrent } from '../../api/job';
import type { CurrentTaskView, TaskItem } from '../../types';
import {
  getRealtimeTaskIdentity,
  mergeTaskItems,
  normalizeTaskItemPage,
  shouldClearRealtimeRows,
  shouldReplaceRealtimeRows,
  shouldResetRealtimeTotal,
  sortTaskItemsByCreateTimeDesc,
  type RealtimeTaskLoadKey,
} from './taskRows';

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
      setTabTaskList([]);
      setTabTaskTotal(0);
      setTabLoading(true);
      setTabTaskPageValue(1);
      return;
    }

    const loadKey = { status: activeTab, taskIdentity, page: tabTaskPage };
    const replaceRows = shouldReplaceRealtimeRows(lastLoaded, loadKey);
    const resetTotal = shouldResetRealtimeTotal(lastLoaded, loadKey);
    const clearRows = shouldClearRealtimeRows(lastLoaded, loadKey);
    const requestID = ++requestRef.current;
    lastLoadedRef.current = loadKey;

    if (replaceRows) {
      if (clearRows) setTabTaskList([]);
      if (resetTotal) setTabTaskTotal(0);
      setTabLoading(true);
    }

    if (activeTab === 1) {
      const doingTask = currentTask.doingTask || [];
      setTabTaskList((previous) => replaceRows ? doingTask : mergeTaskItems(previous, doingTask));
      setTabTaskTotal(doingTask.length);
      setTabLoading(false);
      return;
    }

    async function loadTabTasks() {
      try {
        const res = await jobGetTaskCurrent({
          id: jobId,
          status: activeTab,
          pageSize,
          pageNum: tabTaskPage,
        });
        if (requestID !== requestRef.current) return;
        const { rows, total } = normalizeTaskItemPage(res.data);
        setTabTaskList((previous) => replaceRows ? rows : mergeTaskItems(previous, rows));
        setTabTaskTotal(total);
      } catch {
        if (requestID === requestRef.current && replaceRows) {
          if (clearRows) setTabTaskList([]);
          if (resetTotal) setTabTaskTotal(0);
        }
      } finally {
        if (requestID === requestRef.current) {
          setTabLoading(false);
        }
      }
    }

    loadTabTasks();
  }, [activeTab, currentTask, enabled, jobId, pageSize, tabTaskPage]);

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
