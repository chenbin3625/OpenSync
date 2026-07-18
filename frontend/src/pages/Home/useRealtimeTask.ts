import { useCallback, useEffect, useRef, useState } from 'react';
import { jobGetTaskCurrent } from '../../api/job';
import { POLL_INTERVAL_MS } from '../../api/request';
import type { ApiResponse, CurrentTaskData, CurrentTaskView, PageData, TaskItem } from '../../types';
import { canPollCurrentDocument } from './pollingVisibility';

const SSE_MAX_RETRIES = 3;
const SSE_RETRY_BASE_MS = 1000;

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

function applyCurrentPayload(
  data: CurrentTaskData,
  prevTaskRef: { current: CurrentTaskView | null },
  setCurrentTask: (task: CurrentTaskView | null) => void,
) {
  const progress = calcProgress(data, prevTaskRef.current);
  const nextTask = { ...data, ...progress };
  prevTaskRef.current = nextTask;
  setCurrentTask(nextTask);
}

export function useRealtimeTask(jobId: string, enabled: boolean): {
  currentTask: CurrentTaskView | null;
  refreshCurrentTask: () => Promise<void>;
} {
  const [currentTask, setCurrentTask] = useState<CurrentTaskView | null>(null);
  const prevTaskRef = useRef<CurrentTaskView | null>(null);
  const requestRef = useRef(0);

  const refreshCurrentTask = useCallback(async () => {
    if (!jobId || !canPollCurrentDocument()) return;
    const requestID = ++requestRef.current;
    try {
      const res = await jobGetTaskCurrent({ id: jobId }, { silent: true });
      if (requestID !== requestRef.current) return;
      const data = res.data || null;
      if (isCurrentTaskData(data)) {
        applyCurrentPayload(data, prevTaskRef, setCurrentTask);
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
    if (!enabled || !jobId) return undefined;

    let closed = false;
    let pollID: ReturnType<typeof setInterval> | null = null;
    let retryTimer: ReturnType<typeof setTimeout> | null = null;
    let eventSource: EventSource | null = null;
    let sseRetries = 0;

    const startPolling = () => {
      if (pollID || closed) return;
      refreshCurrentTask();
      pollID = setInterval(refreshCurrentTask, POLL_INTERVAL_MS);
    };

    const stopPolling = () => {
      if (!pollID) return;
      clearInterval(pollID);
      pollID = null;
    };

    const cleanupStream = () => {
      if (retryTimer) {
        clearTimeout(retryTimer);
        retryTimer = null;
      }
      if (eventSource) {
        eventSource.close();
        eventSource = null;
      }
    };

    const connectSSE = () => {
      if (closed || typeof EventSource === 'undefined' || !canPollCurrentDocument()) {
        startPolling();
        return;
      }

      cleanupStream();
      stopPolling();

      const streamURL = `/svr/job/stream?id=${encodeURIComponent(jobId)}`;
      eventSource = new EventSource(streamURL);

      eventSource.onmessage = (event) => {
        if (closed || !canPollCurrentDocument()) return;
        sseRetries = 0;
        try {
          const payload = JSON.parse(event.data) as ApiResponse<CurrentTaskData | null>;
          if (payload.code !== 200) {
            return;
          }
          if (!payload.data) {
            prevTaskRef.current = null;
            setCurrentTask(null);
            return;
          }
          if (!isCurrentTaskData(payload.data)) {
            return;
          }
          applyCurrentPayload(payload.data, prevTaskRef, setCurrentTask);
        } catch {
          /* ignore malformed stream payloads */
        }
      };

      eventSource.onerror = () => {
        if (closed) return;
        cleanupStream();
        if (sseRetries < SSE_MAX_RETRIES) {
          const delay = SSE_RETRY_BASE_MS * (2 ** sseRetries);
          sseRetries += 1;
          retryTimer = setTimeout(connectSSE, delay);
          return;
        }
        startPolling();
      };
    };

    if (typeof EventSource !== 'undefined' && canPollCurrentDocument()) {
      connectSSE();
      return () => {
        closed = true;
        cleanupStream();
        stopPolling();
      };
    }

    startPolling();
    return () => {
      closed = true;
      cleanupStream();
      stopPolling();
    };
  }, [enabled, jobId, refreshCurrentTask]);

  return { currentTask, refreshCurrentTask };
}
