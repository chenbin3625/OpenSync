import { useCallback, useEffect, useRef, useState } from 'react';
import { jobGetTaskCurrent } from '../../api/job';
import { POLL_INTERVAL_MS } from '../../api/request';
import { useStore } from '../../stores/useStore';
import type { ApiResponse, CurrentTaskData, CurrentTaskView, PageData, TaskItem } from '../../types';
import { calcRealtimeProgress, getRealtimeTaskIdentity, mergeCurrentTaskData } from './taskRows';
import { canPollCurrentDocument } from './pollingVisibility';
import { canUseFetchSSE, readSSEStream, SSEHttpError } from './sseStream';

const SSE_MAX_RETRIES = 3;
const SSE_RETRY_BASE_MS = 1000;

function redirectToLogin() {
  useStore.getState().setUserInfo(null);
  useStore.getState().setAuthChecked(true);
  if (typeof window !== 'undefined' && window.location.pathname !== '/login') {
    window.location.replace('/login');
  }
}

function isCurrentTaskData(data: CurrentTaskData | PageData<TaskItem> | TaskItem[] | null): data is CurrentTaskData {
  return !!data && !Array.isArray(data) && 'taskId' in data && 'num' in data;
}
export function useRealtimeTask(jobId: string, enabled: boolean): {
  currentTask: CurrentTaskView | null;
  refreshCurrentTask: () => Promise<void>;
} {
  const [currentTask, setCurrentTask] = useState<CurrentTaskView | null>(null);
  const prevTaskRef = useRef<CurrentTaskView | null>(null);
  const requestRef = useRef(0);
  const inFlightRef = useRef(false);
  const pollAbortRef = useRef<AbortController | null>(null);

  const publishCurrentTask = useCallback((data: CurrentTaskData | null) => {
    const previous = prevTaskRef.current;
    const sameTask = data && previous && getRealtimeTaskIdentity(data) === getRealtimeTaskIdentity(previous);
    const nextTask = data ? { ...data, ...calcRealtimeProgress(data, sameTask ? previous : null) } : null;
    prevTaskRef.current = nextTask;
    setCurrentTask(nextTask);
  }, []);

  const refreshCurrentTask = useCallback(async () => {
    if (!jobId || !canPollCurrentDocument()) return;
    if (inFlightRef.current) return;
    const requestID = ++requestRef.current;
    const controller = new AbortController();
    pollAbortRef.current?.abort();
    pollAbortRef.current = controller;
    inFlightRef.current = true;
    try {
      const res = await jobGetTaskCurrent({ id: jobId }, { silent: true, signal: controller.signal });
      if (controller.signal.aborted || requestID !== requestRef.current) return;
      const data = res.data || null;
      publishCurrentTask(isCurrentTaskData(data) ? mergeCurrentTaskData(data, prevTaskRef.current) : null);
    } catch {
      /* keep the last visible realtime snapshot on transient polling errors */
    } finally {
      if (pollAbortRef.current === controller) {
        pollAbortRef.current = null;
        inFlightRef.current = false;
      }
    }
  }, [jobId, publishCurrentTask]);

  useEffect(() => {
    requestRef.current += 1;
    pollAbortRef.current?.abort();
    pollAbortRef.current = null;
    inFlightRef.current = false;
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
    let pendingTask: CurrentTaskData | null | undefined;
    const supportsSSE = canUseFetchSSE() || typeof EventSource !== 'undefined';

    // A hidden tab consumes nothing: closing the stream also releases the
    // backend's "someone is watching" flag, which lets its AList copy poller
    // fall back from the fast interval to the idle one.
    const isVisible = () => !closed && canPollCurrentDocument();

    const flushFrame = () => {
      raf = 0;
      const data = pendingTask;
      pendingTask = undefined;
      if (data !== undefined && isVisible()) publishCurrentTask(data);
    };

    const schedulePayload = (data: CurrentTaskData | null) => {
      const previous = pendingTask === undefined ? prevTaskRef.current : pendingTask;
      pendingTask = data ? mergeCurrentTaskData(data, previous) : null;
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
      pendingTask = undefined;
    };

    const stopRealtime = () => {
      cleanupStream();
      stopPolling();
      // Drop any in-flight polling request so its completion callback cannot
      // touch state after teardown.
      pollAbortRef.current?.abort();
      pollAbortRef.current = null;
      inFlightRef.current = false;
    };

    const handleUnauthorized = () => {
      closed = true;
      stopRealtime();
      redirectToLogin();
    };

    const applyStreamJSON = (raw: string) => {
      if (closed || !canPollCurrentDocument()) return;
      try {
        const payload = JSON.parse(raw) as ApiResponse<CurrentTaskData | null>;
        if (!payload || typeof payload !== 'object' || typeof payload.code !== 'number') {
          return;
        }
        if (payload.code === 401) {
          handleUnauthorized();
          return;
        }
        if (payload.code !== 200) {
          return;
        }
        if (!payload.data) {
          sseRetries = 0;
          schedulePayload(null);
          return;
        }
        if (!isCurrentTaskData(payload.data)) {
          return;
        }
        sseRetries = 0;
        schedulePayload(payload.data);
      } catch {
        /* ignore malformed stream payloads */
      }
    };

    const retryOrPoll = () => {
      if (closed) return;
      if (raf) {
        cancelAnimationFrame(raf);
        flushFrame();
      }
      cleanupStream();
      // While hidden there is nobody to update; wait for the next
      // visibilitychange instead of reconnecting or polling in the background.
      if (!isVisible()) return;
      if (supportsSSE && sseRetries < SSE_MAX_RETRIES) {
        const delay = SSE_RETRY_BASE_MS * (2 ** sseRetries);
        sseRetries += 1;
        retryTimer = setTimeout(connectSSE, delay);
        return;
      }
      startPolling();
    };

    const connectSSE = () => {
      if (closed || !isVisible()) return;
      if (!supportsSSE) {
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
        }).catch((err) => {
          if (closed || controller.signal.aborted) return;
          if (err instanceof SSEHttpError && err.status === 401) {
            handleUnauthorized();
            return;
          }
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

    const startRealtime = () => {
      if (!isVisible()) return;
      if (!supportsSSE) {
        startPolling();
        return;
      }
      // A fresh connection replays the current snapshot and resets the retry
      // budget, so a tab that had degraded to polling gets SSE back.
      sseRetries = 0;
      connectSSE();
    };

    const onVisibilityChange = () => {
      if (closed) return;
      if (!isVisible()) {
        stopRealtime();
        return;
      }
      startRealtime();
    };

    document.addEventListener('visibilitychange', onVisibilityChange);
    startRealtime();

    return () => {
      closed = true;
      document.removeEventListener('visibilitychange', onVisibilityChange);
      stopRealtime();
    };
  }, [enabled, jobId, publishCurrentTask, refreshCurrentTask]);

  return { currentTask, refreshCurrentTask };
}
