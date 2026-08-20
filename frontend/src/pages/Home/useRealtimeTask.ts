import { useCallback, useEffect, useRef, useState } from 'react';
import { jobGetTaskCurrent } from '../../api/job';
import { POLL_INTERVAL_MS } from '../../api/request';
import type { ApiResponse, CurrentTaskData, CurrentTaskView, PageData, TaskItem } from '../../types';
import { calcRealtimeProgress, mergeCurrentTaskData } from './taskRows';
import { canPollCurrentDocument } from './pollingVisibility';
import { canUseFetchSSE, readSSEStream } from './sseStream';

const SSE_MAX_RETRIES = 3;
const SSE_RETRY_BASE_MS = 1000;

function isCurrentTaskData(data: CurrentTaskData | PageData<TaskItem> | TaskItem[] | null): data is CurrentTaskData {
  return !!data && !Array.isArray(data) && 'taskId' in data && 'num' in data;
}

function applyCurrentPayload(
  data: CurrentTaskData,
  prevTaskRef: { current: CurrentTaskView | null },
  setCurrentTask: (task: CurrentTaskView | null) => void,
) {
  const merged = mergeCurrentTaskData(data, prevTaskRef.current);
  const progress = calcRealtimeProgress(merged, prevTaskRef.current);
  const nextTask = { ...merged, ...progress };
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
  const inFlightRef = useRef(false);

  const refreshCurrentTask = useCallback(async () => {
    if (!jobId || !canPollCurrentDocument()) return;
    if (inFlightRef.current) return;
    const requestID = ++requestRef.current;
    inFlightRef.current = true;
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
    } finally {
      inFlightRef.current = false;
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
    let abortStream: AbortController | null = null;
    let sseRetries = 0;
    let raf = 0;
    let pendingPayload: CurrentTaskData | null | undefined;

    const flushFrame = () => {
      raf = 0;
      const data = pendingPayload;
      pendingPayload = undefined;
      if (closed || data === undefined) return;
      if (data === null) {
        prevTaskRef.current = null;
        setCurrentTask(null);
        return;
      }
      applyCurrentPayload(data, prevTaskRef, setCurrentTask);
    };

    const schedulePayload = (data: CurrentTaskData | null) => {
      pendingPayload = data;
      if (raf) return;
      raf = requestAnimationFrame(flushFrame);
    };

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

    const applyStreamJSON = (raw: string) => {
      if (closed || !canPollCurrentDocument()) return;
      sseRetries = 0;
      try {
        const payload = JSON.parse(raw) as ApiResponse<CurrentTaskData | null>;
        if (payload.code !== 200) {
          return;
        }
        if (!payload.data) {
          schedulePayload(null);
          return;
        }
        if (!isCurrentTaskData(payload.data)) {
          return;
        }
        schedulePayload(payload.data);
      } catch {
        /* ignore malformed stream payloads */
      }
    };

    const cleanupStream = () => {
      if (retryTimer) {
        clearTimeout(retryTimer);
        retryTimer = null;
      }
      if (abortStream) {
        abortStream.abort();
        abortStream = null;
      }
      if (eventSource) {
        eventSource.close();
        eventSource = null;
      }
      if (raf) {
        cancelAnimationFrame(raf);
        raf = 0;
      }
    };

    const retryOrPoll = () => {
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

    const connectSSE = () => {
      if (closed || !canPollCurrentDocument()) {
        startPolling();
        return;
      }

      cleanupStream();
      stopPolling();

      const streamURL = `/svr/job/stream?id=${encodeURIComponent(jobId)}`;
      if (canUseFetchSSE()) {
        const controller = new AbortController();
        abortStream = controller;
        readSSEStream(streamURL, controller.signal, applyStreamJSON).then(() => {
          if (closed || controller.signal.aborted) return;
          retryOrPoll();
        }).catch(() => {
          if (closed || controller.signal.aborted) return;
          retryOrPoll();
        });
        return;
      }

      if (typeof EventSource === 'undefined') {
        startPolling();
        return;
      }
      eventSource = new EventSource(streamURL);
      eventSource.onmessage = (event) => applyStreamJSON(event.data);
      eventSource.onerror = () => retryOrPoll();
    };

    if ((canUseFetchSSE() || typeof EventSource !== 'undefined') && canPollCurrentDocument()) {
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

  useEffect(() => {
    const onVisibilityChange = () => {
      if (!document.hidden) refreshCurrentTask();
    };
    document.addEventListener('visibilitychange', onVisibilityChange);
    return () => document.removeEventListener('visibilitychange', onVisibilityChange);
  }, [refreshCurrentTask]);

  return { currentTask, refreshCurrentTask };
}
