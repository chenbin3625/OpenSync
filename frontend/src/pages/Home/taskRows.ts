import type { CurrentTaskData, PageData, TaskItem, TaskRecord } from '../../types';

export type TaskListView = 'realtime' | 'history';

type CurrentTaskIdentity = {
  createTime?: number | string | null;
  taskId?: number | string | null;
} | null | undefined;

export type RealtimeTaskLoadKey = {
  status: number;
  taskIdentity: string;
  page: number;
};

const runningHistoryStatuses = new Set([0, 1]);
const runningTaskItemStatus = 1;

export const IDLE_POLL_INTERVAL_MS = 15000;

export function historyHasActiveTask(rows: Array<{ status: number }>): boolean {
  return rows.some((row) => runningHistoryStatuses.has(row.status));
}

export function detailHasRunningItem(rows: Array<{ status: number }>): boolean {
  return rows.some((row) => row.status === runningTaskItemStatus);
}

export function pollIntervalForActiveWork(hasActive: boolean, activeMs: number, idleMs = IDLE_POLL_INTERVAL_MS): number {
  return hasActive ? activeMs : idleMs;
}

function mergeByKey<T>(
  previous: T[],
  next: T[],
  getKey: (item: T, index: number) => string,
  isSame: (a: T, b: T) => boolean,
): T[] {
  if (previous.length === 0) return next;

  const previousByKey = new Map(previous.map((item, index) => [getKey(item, index), item]));
  let changed = previous.length !== next.length;
  const merged = next.map((item, index) => {
    const existing = previousByKey.get(getKey(item, index));
    const row = existing && isSame(existing, item) ? existing : item;
    if (row !== previous[index]) changed = true;
    return row;
  });
  return changed ? merged : previous;
}

function sameTaskRecord(a: TaskRecord, b: TaskRecord): boolean {
  return a.id === b.id &&
    a.status === b.status &&
    a.errMsg === b.errMsg &&
    a.runTime === b.runTime &&
    a.successNum === b.successNum &&
    a.failNum === b.failNum &&
    a.allNum === b.allNum &&
    a.createTime === b.createTime;
}

export function mergeTaskRecords(previous: TaskRecord[], next: TaskRecord[]): TaskRecord[] {
  return mergeByKey(previous, next, (task) => String(task.id), sameTaskRecord);
}

export function getTaskItemKey(task: TaskItem, fallback = 0): string {
  return String(
    task.id ??
      task.alistTaskId ??
      `${task.fileName || ''}|${task.srcPath || ''}|${task.dstPath || ''}|${fallback}`,
  );
}

export function getRealtimeTaskIdentity(task: CurrentTaskIdentity): string {
  return `${Number(task?.taskId || 0)}:${Number(task?.createTime || 0)}`;
}

function sameTaskItem(a: TaskItem, b: TaskItem): boolean {
  return a.id === b.id &&
    a.taskId === b.taskId &&
    a.srcPath === b.srcPath &&
    a.dstPath === b.dstPath &&
    a.isPath === b.isPath &&
    a.fileName === b.fileName &&
    a.fileSize === b.fileSize &&
    a.type === b.type &&
    a.alistTaskId === b.alistTaskId &&
    a.status === b.status &&
    a.progress === b.progress &&
    a.errMsg === b.errMsg &&
    a.createTime === b.createTime;
}

export function mergeTaskItems(previous: TaskItem[], next: TaskItem[]): TaskItem[] {
  return mergeByKey(previous, next, getTaskItemKey, sameTaskItem);
}

export function applyDoingPatch(previous: TaskItem[], patch: TaskItem[]): TaskItem[] {
  if (previous.length === 0 || patch.length === 0) return previous;

  const patchByKey = new Map(patch.map((item, index) => [getTaskItemKey(item, index), item]));
  let changed = false;
  const next = previous.map((item, index) => {
    const delta = patchByKey.get(getTaskItemKey(item, index));
    if (!delta) return item;
    const merged = { ...item, ...delta };
    if (sameTaskItem(item, merged)) return item;
    changed = true;
    return merged;
  });
  return changed ? next : previous;
}

export function mergeCurrentTaskData(
  data: CurrentTaskData,
  previous: CurrentTaskData | null,
): CurrentTaskData {
  if (data.doingPatch && previous && getRealtimeTaskIdentity(data) === getRealtimeTaskIdentity(previous)) {
    return {
      ...data,
      doingTask: applyDoingPatch(previous.doingTask || [], data.doingPatch),
    };
  }
  return {
    ...data,
    doingTask: data.doingTask || previous?.doingTask || [],
  };
}

export function normalizeTaskItemPage(
  data: CurrentTaskData | PageData<TaskItem> | TaskItem[] | null | undefined,
): { rows: TaskItem[]; total: number } {
  if (!data) return { rows: [], total: 0 };
  if (Array.isArray(data)) return { rows: data, total: data.length };
  if ('dataList' in data && Array.isArray(data.dataList)) {
    return { rows: data.dataList, total: Number(data.count || 0) };
  }
  return { rows: [], total: 0 };
}

export function sortTaskItemsByCreateTimeDesc(rows: TaskItem[]): TaskItem[] {
  return [...rows].sort((a, b) => {
    const left = Number(a.createTime || 0);
    const right = Number(b.createTime || 0);
    if (left === right) return Number(b.id || 0) - Number(a.id || 0);
    return right - left;
  });
}

export function shouldReplaceRealtimeRows(
  previous: RealtimeTaskLoadKey | null,
  next: RealtimeTaskLoadKey,
): boolean {
  return !previous ||
    previous.status !== next.status ||
    previous.taskIdentity !== next.taskIdentity ||
    previous.page !== next.page;
}

export function shouldResetRealtimeSnapshot(
  previous: RealtimeTaskLoadKey | null,
  next: RealtimeTaskLoadKey,
): boolean {
  return !previous ||
    previous.status !== next.status ||
    previous.taskIdentity !== next.taskIdentity;
}

export function filterCurrentTaskFromHistory(
  history: TaskRecord[],
  currentTask: CurrentTaskIdentity,
): TaskRecord[] {
  const currentCreateTime = Number(currentTask?.createTime || 0);
  if (!Number.isFinite(currentCreateTime) || currentCreateTime <= 0) return history;

  return history.filter((task) => {
    const taskCreateTime = Number(task.createTime || task.runTime || 0);
    return !(taskCreateTime === currentCreateTime && runningHistoryStatuses.has(task.status));
  });
}

export function filterRunningTaskRows(history: TaskRecord[]): TaskRecord[] {
  return history.filter((task) => !runningHistoryStatuses.has(task.status));
}

export function shouldPollRealtime(view: TaskListView): boolean {
  return view === 'realtime';
}

type ProgressSnapshot = {
  duration?: number;
  doneSize?: number;
} | null;

export function calcRealtimeProgress(cur: CurrentTaskData, previous: ProgressSnapshot): {
  remainSize: number;
  doneSize: number;
  speed: number;
  speedAvg: number;
  remainTime: number;
} {
  const sizeMap = cur.size || {};
  const serverDone = Number(cur.doneSize);
  const serverRemain = Number(cur.remainSize);
  let doneSize: number;
  let remainSize: number;
  if (Number.isFinite(serverDone) && Number.isFinite(serverRemain)) {
    doneSize = serverDone;
    remainSize = Math.max(0, serverRemain);
  } else {
    const doingSize = (cur.doingTask || []).reduce((sum, item) => {
      const progress = Number(item.progress || 0);
      return sum + (item.fileSize || 0) * progress / 100.0;
    }, 0);
    remainSize = Math.max(0, (sizeMap.running || 0) - doingSize + (sizeMap.wait || 0));
    doneSize = (sizeMap.success || 0) + doingSize;
  }

  let speed = 0;
  if (typeof cur.speed === 'number' && Number.isFinite(cur.speed)) {
    speed = cur.speed;
  } else if (previous && typeof previous.duration === 'number' && cur.duration !== previous.duration) {
    speed = (doneSize - (previous.doneSize || 0)) / (cur.duration - previous.duration);
  }

  let speedAvg = 0;
  if (typeof cur.speedAvg === 'number' && Number.isFinite(cur.speedAvg)) {
    speedAvg = cur.speedAvg;
  } else if (cur.firstSync && cur.duration > 0) {
    const syncDuration = cur.duration - (cur.firstSync - cur.createTime);
    if (syncDuration > 0) speedAvg = doneSize / syncDuration;
  }

  let remainTime = 0;
  if (typeof cur.remainTime === 'number' && Number.isFinite(cur.remainTime)) {
    remainTime = cur.remainTime;
  } else if (speedAvg > 0 && remainSize > 0) {
    remainTime = Math.ceil(remainSize / speedAvg);
  }

  return { remainSize, doneSize, speed, speedAvg, remainTime };
}
