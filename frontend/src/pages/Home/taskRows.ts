import type { TaskItem, TaskRecord } from '../../types';

type CurrentTaskIdentity = {
  createTime?: number | null;
} | null | undefined;

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
  return next.map((task) => {
    const existing = previousByID.get(task.id);
    if (!existing) return task;
    return sameTaskRecord(existing, task) ? existing : task;
  });
}

export function getTaskItemKey(task: TaskItem, fallback = 0): string {
  return String(
    task.id ??
      task.alistTaskId ??
      `${task.fileName || ''}|${task.srcPath || ''}|${task.dstPath || ''}|${fallback}`,
  );
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
  return next.map((task, index) => {
    const existing = previousByKey.get(getTaskItemKey(task, index));
    if (!existing) return task;
    return sameTaskItem(existing, task) ? existing : task;
  });
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
