import type { CurrentTaskData, PageData, TaskItem, TaskRecord } from '../../types';

export type TaskListView = 'all' | 'realtime' | 'history';

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

function sameTaskRecord(a: TaskRecord, b: TaskRecord): boolean {
  return a.id === b.id &&
    a.status === b.status &&
    a.runTime === b.runTime &&
    a.successNum === b.successNum &&
    a.failNum === b.failNum &&
    a.allNum === b.allNum &&
    a.createTime === b.createTime;
}

export function mergeTaskRecords(previous: TaskRecord[], next: TaskRecord[]): TaskRecord[] {
  if (previous.length === 0) return next;

  const previousByID = new Map(previous.map((task) => [task.id, task]));
  let changed = previous.length !== next.length;
  const merged = next.map((task, index) => {
    const existing = previousByID.get(task.id);
    const row = existing && sameTaskRecord(existing, task) ? existing : task;
    if (row !== previous[index]) changed = true;
    return row;
  });
  return changed ? merged : previous;
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
  if (previous.length === 0) return next;

  const previousByKey = new Map(previous.map((task, index) => [getTaskItemKey(task, index), task]));
  let changed = previous.length !== next.length;
  const merged = next.map((task, index) => {
    const existing = previousByKey.get(getTaskItemKey(task, index));
    const row = existing && sameTaskItem(existing, task) ? existing : task;
    if (row !== previous[index]) changed = true;
    return row;
  });
  return changed ? merged : previous;
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

export function shouldResetRealtimeTotal(
  previous: RealtimeTaskLoadKey | null,
  next: RealtimeTaskLoadKey,
): boolean {
  return !previous ||
    previous.status !== next.status ||
    previous.taskIdentity !== next.taskIdentity;
}

export function shouldClearRealtimeRows(
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
  return view === 'all' || view === 'realtime';
}
