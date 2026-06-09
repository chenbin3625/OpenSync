import { useCallback, useEffect, useRef, useState } from 'react';
import { jobGetTaskCurrent } from '../../api/job';
import type { CurrentTaskData, CurrentTaskView, PageData, TaskItem } from '../../types';
import { canPollCurrentDocument } from './pollingVisibility';

function isCurrentTaskData(data: CurrentTaskData | PageData<TaskItem> | TaskItem[] | null): data is CurrentTaskData {
  return !!data && !Array.isArray(data) && 'doingTask' in data && Array.isArray(data.doingTask);
}

function calcProgress(cur: CurrentTaskData, previous: CurrentTaskView | null) {
  const doingTask = cur.doingTask || [];
  const doingSize = doingTask.reduce((sum, item) => {
    const progress = Number(item.progress || 0);
    return sum + (item.fileSize || 0) * progress / 100.0;
  }, 0);
  const sizeMap = cur.size || {};
  const remainSize = (sizeMap.running || 0) - doingSize + (sizeMap.wait || 0);
  const doneSize = (sizeMap.success || 0) + doingSize;

  let speed = 0;
  if (previous && cur.duration !== previous.duration) {
    speed = (doneSize - (previous.doneSize || 0)) / (cur.duration - previous.duration);
  }

  let speedAvg = 0;
  if (cur.firstSync && cur.duration > 0) {
    const syncDuration = cur.duration - (cur.firstSync - cur.createTime);
    if (syncDuration > 0) speedAvg = doneSize / syncDuration;
  }

  let remainTime = 0;
  if (speedAvg > 0 && remainSize > 0) {
    remainTime = Math.ceil(remainSize / speedAvg);
  }

  return { remainSize, doneSize, speed, speedAvg, remainTime };
}

export function useRealtimeTask(jobId: string, enabled: boolean): {
  currentTask: CurrentTaskView | null;
  nowTick: number;
  refreshCurrentTask: () => Promise<void>;
} {
  const [currentTask, setCurrentTask] = useState<CurrentTaskView | null>(null);
  const [nowTick, setNowTick] = useState(() => Math.floor(Date.now() / 1000));
  const prevTaskRef = useRef<CurrentTaskView | null>(null);
  const requestRef = useRef(0);

  const refreshCurrentTask = useCallback(async () => {
    if (!jobId || !canPollCurrentDocument()) return;
    const requestID = ++requestRef.current;
    try {
      const res = await jobGetTaskCurrent({ id: jobId });
      if (requestID !== requestRef.current) return;
      const data = res.data || null;
      if (isCurrentTaskData(data)) {
        const progress = calcProgress(data, prevTaskRef.current);
        const nextTask = { ...data, ...progress };
        prevTaskRef.current = nextTask;
        setCurrentTask(nextTask);
      } else {
        prevTaskRef.current = null;
        setCurrentTask(null);
      }
    } catch {
      /* keep the last visible realtime snapshot on transient polling errors */
    }
  }, [jobId]);

  useEffect(() => {
    requestRef.current += 1;
    prevTaskRef.current = null;
    setCurrentTask(null);
  }, [jobId]);

  useEffect(() => {
    if (!enabled) return undefined;
    refreshCurrentTask();
    const pollID = setInterval(refreshCurrentTask, 3000);
    return () => { clearInterval(pollID); };
  }, [enabled, refreshCurrentTask]);

  useEffect(() => {
    if (!currentTask) return undefined;
    const tickID = setInterval(() => {
      if (canPollCurrentDocument()) {
        setNowTick(Math.floor(Date.now() / 1000));
      }
    }, 1000);
    return () => { clearInterval(tickID); };
  }, [currentTask]);

  return { currentTask, nowTick, refreshCurrentTask };
}
