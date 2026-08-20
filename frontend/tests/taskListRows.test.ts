import assert from 'node:assert/strict';
import test from 'node:test';
import {
  calcRealtimeProgress,
  detailHasRunningItem,
  filterCurrentTaskFromHistory,
  filterRunningTaskRows,
  getRealtimeTaskIdentity,
  getTaskItemKey,
  historyHasActiveTask,
  applyDoingPatch,
  mergeCurrentTaskData,
  mergeTaskItems,
  mergeTaskRecords,
  normalizeTaskItemPage,
  pollIntervalForActiveWork,
  shouldResetRealtimeSnapshot,
  shouldReplaceRealtimeRows,
  shouldPollRealtime,
  sortTaskItemsByCreateTimeDesc,
} from '../src/pages/Home/taskRows.ts';
import type { TaskItem, TaskRecord } from '../src/types.ts';

test('mergeTaskRecords keeps unchanged task row references during refresh', () => {
  const existing: TaskRecord[] = [
    { id: 1, status: 1, successNum: 2, failNum: 0, allNum: 10, runTime: 100 },
    { id: 2, status: 2, successNum: 10, failNum: 0, allNum: 10, runTime: 80 },
  ];
  const refreshed: TaskRecord[] = [
    { id: 1, status: 1, successNum: 3, failNum: 0, allNum: 10, runTime: 100 },
    { id: 2, status: 2, successNum: 10, failNum: 0, allNum: 10, runTime: 80 },
  ];

  const merged = mergeTaskRecords(existing, refreshed);

  assert.notEqual(merged[0], existing[0]);
  assert.deepEqual(merged[0], refreshed[0]);
  assert.equal(merged[1], existing[1]);
});

test('mergeTaskRecords replaces rows when task-level errMsg changes', () => {
  const existing: TaskRecord[] = [
    { id: 1, status: 6, errMsg: 'old failure', successNum: 0, failNum: 0, allNum: 0, runTime: 100 },
  ];
  const refreshed: TaskRecord[] = [
    { id: 1, status: 6, errMsg: 'new failure', successNum: 0, failNum: 0, allNum: 0, runTime: 100 },
  ];

  const merged = mergeTaskRecords(existing, refreshed);

  assert.notEqual(merged[0], existing[0]);
  assert.equal(merged[0].errMsg, 'new failure');
});

test('mergeTaskRecords reuses the previous array when refreshed rows are unchanged', () => {
  const existing: TaskRecord[] = [
    { id: 1, status: 2, successNum: 10, failNum: 0, allNum: 10, runTime: 100 },
    { id: 2, status: 3, successNum: 9, failNum: 1, allNum: 10, runTime: 80 },
  ];
  const refreshed: TaskRecord[] = [
    { id: 1, status: 2, successNum: 10, failNum: 0, allNum: 10, runTime: 100 },
    { id: 2, status: 3, successNum: 9, failNum: 1, allNum: 10, runTime: 80 },
  ];

  const merged = mergeTaskRecords(existing, refreshed);

  assert.equal(merged, existing);
});

test('mergeTaskItems keeps unchanged realtime file row references during refresh', () => {
  const existing: TaskItem[] = [
    { id: 10, status: 1, progress: 20, fileName: 'movie.mkv', fileSize: 1024 },
    { alistTaskId: 'copy-20', status: 0, progress: 0, fileName: 'photo.jpg', srcPath: '/a', dstPath: '/b' },
  ];
  const refreshed: TaskItem[] = [
    { id: 10, status: 1, progress: 35, fileName: 'movie.mkv', fileSize: 1024 },
    { alistTaskId: 'copy-20', status: 0, progress: 0, fileName: 'photo.jpg', srcPath: '/a', dstPath: '/b' },
  ];

  const merged = mergeTaskItems(existing, refreshed);

  assert.notEqual(merged[0], existing[0]);
  assert.deepEqual(merged[0], refreshed[0]);
  assert.equal(merged[1], existing[1]);
  assert.equal(getTaskItemKey(refreshed[1], 1), 'copy-20');
});

test('mergeTaskItems reuses the previous array when refreshed rows are unchanged', () => {
  const existing: TaskItem[] = [
    { id: 10, status: 1, progress: 35, fileName: 'movie.mkv', fileSize: 1024 },
    { alistTaskId: 'copy-20', status: 0, progress: 0, fileName: 'photo.jpg', srcPath: '/a', dstPath: '/b' },
  ];
  const refreshed: TaskItem[] = [
    { id: 10, status: 1, progress: 35, fileName: 'movie.mkv', fileSize: 1024 },
    { alistTaskId: 'copy-20', status: 0, progress: 0, fileName: 'photo.jpg', srcPath: '/a', dstPath: '/b' },
  ];

  const merged = mergeTaskItems(existing, refreshed);

  assert.equal(merged, existing);
});

test('filterCurrentTaskFromHistory removes the active running task from history rows', () => {
  const rows: TaskRecord[] = [
    { id: 12, status: 1, createTime: 300, runTime: 300, successNum: 1, failNum: 0, allNum: 3 },
    { id: 11, status: 2, createTime: 200, runTime: 200, successNum: 3, failNum: 0, allNum: 3 },
    { id: 10, status: 2, createTime: 100, runTime: 100, successNum: 2, failNum: 0, allNum: 2 },
  ];

  const filtered = filterCurrentTaskFromHistory(rows, { createTime: 300 });

  assert.deepEqual(filtered.map((row) => row.id), [11, 10]);
});

test('filterRunningTaskRows removes active rows when history view does not poll realtime state', () => {
  const rows: TaskRecord[] = [
    { id: 12, status: 1, createTime: 300, runTime: 300, successNum: 1, failNum: 0, allNum: 3 },
    { id: 11, status: 0, createTime: 200, runTime: 200, successNum: 0, failNum: 0, allNum: 3 },
    { id: 10, status: 2, createTime: 100, runTime: 100, successNum: 2, failNum: 0, allNum: 2 },
  ];

  const filtered = filterRunningTaskRows(rows);

  assert.deepEqual(filtered.map((row) => row.id), [10]);
});

test('shouldPollRealtime only enables polling for realtime view', () => {
  assert.equal(shouldPollRealtime('realtime'), true);
  assert.equal(shouldPollRealtime('history'), false);
});

test('normalizeTaskItemPage accepts paged, array, and null realtime item payloads', () => {
  assert.deepEqual(
    normalizeTaskItemPage({ dataList: [{ id: 1, status: 2 }], count: 9 }),
    { rows: [{ id: 1, status: 2 }], total: 9 },
  );
  assert.deepEqual(
    normalizeTaskItemPage([{ id: 2, status: 7 }]),
    { rows: [{ id: 2, status: 7 }], total: 1 },
  );
  assert.deepEqual(normalizeTaskItemPage(null), { rows: [], total: 0 });
});

test('sortTaskItemsByCreateTimeDesc sorts by create time and id fallback', () => {
  const rows: TaskItem[] = [
    { id: 1, status: 2, createTime: 100 },
    { id: 2, status: 2, createTime: 300 },
    { id: 3, status: 2, createTime: 300 },
    { id: 4, status: 2, createTime: 200 },
  ];

  const sorted = sortTaskItemsByCreateTimeDesc(rows);

  assert.deepEqual(sorted.map((row) => row.id), [3, 2, 4, 1]);
  assert.notEqual(sorted, rows);
});

test('getRealtimeTaskIdentity combines task id and create time', () => {
  assert.equal(getRealtimeTaskIdentity({ taskId: 10, createTime: 100 }), '10:100');
  assert.equal(getRealtimeTaskIdentity(null), '0:0');
});

test('shouldReplaceRealtimeRows replaces rows only when task, tab, or page changes', () => {
  assert.equal(shouldReplaceRealtimeRows(null, { status: 2, taskIdentity: '10:100', page: 1 }), true);
  assert.equal(
    shouldReplaceRealtimeRows(
      { status: 2, taskIdentity: '10:100', page: 1 },
      { status: 2, taskIdentity: '10:100', page: 1 },
    ),
    false,
  );
  assert.equal(
    shouldReplaceRealtimeRows(
      { status: 2, taskIdentity: '10:100', page: 1 },
      { status: 2, taskIdentity: '10:100', page: 2 },
    ),
    true,
  );
  assert.equal(
    shouldReplaceRealtimeRows(
      { status: 2, taskIdentity: '10:100', page: 1 },
      { status: 7, taskIdentity: '10:100', page: 1 },
    ),
    true,
  );
  assert.equal(
    shouldReplaceRealtimeRows(
      { status: 2, taskIdentity: '10:100', page: 1 },
      { status: 2, taskIdentity: '11:200', page: 1 },
    ),
    true,
  );
});

test('shouldResetRealtimeSnapshot keeps pagination total when only page changes', () => {
  assert.equal(shouldResetRealtimeSnapshot(null, { status: 2, taskIdentity: '10:100', page: 1 }), true);
  assert.equal(
    shouldResetRealtimeSnapshot(
      { status: 2, taskIdentity: '10:100', page: 1 },
      { status: 2, taskIdentity: '10:100', page: 2 },
    ),
    false,
  );
  assert.equal(
    shouldResetRealtimeSnapshot(
      { status: 2, taskIdentity: '10:100', page: 1 },
      { status: 7, taskIdentity: '10:100', page: 1 },
    ),
    true,
  );
  assert.equal(
    shouldResetRealtimeSnapshot(
      { status: 2, taskIdentity: '10:100', page: 1 },
      { status: 2, taskIdentity: '11:200', page: 1 },
    ),
    true,
  );
});

test('calcRealtimeProgress prefers server-computed transfer totals', () => {
  const progress = calcRealtimeProgress({
    taskId: 1,
    scanFinish: true,
    doingTask: [{ status: 1, fileSize: 1000, progress: 50 }],
    createTime: 100,
    duration: 10,
    num: { wait: 0, running: 1, success: 0, fail: 0, other: 0 },
    size: { wait: 0, running: 1000, success: 0, fail: 0, other: 0 },
    doneSize: 800,
    remainSize: 200,
  }, null);

  assert.equal(progress.doneSize, 800);
  assert.equal(progress.remainSize, 200);
});

test('calcRealtimeProgress prefers server-computed speed meters', () => {
  const progress = calcRealtimeProgress({
    taskId: 1,
    scanFinish: true,
    doingTask: [{ status: 1, fileSize: 1000, progress: 50 }],
    createTime: 100,
    duration: 10,
    firstSync: 100,
    num: { wait: 0, running: 1, success: 0, fail: 0, other: 0 },
    size: { wait: 0, running: 1000, success: 0, fail: 0, other: 0 },
    doneSize: 800,
    remainSize: 200,
    speed: 40,
    speedAvg: 80,
    remainTime: 3,
  }, { duration: 5, doneSize: 400 });

  assert.equal(progress.speed, 40);
  assert.equal(progress.speedAvg, 80);
  assert.equal(progress.remainTime, 3);
});

test('calcRealtimeProgress falls back to doingTask when server totals are absent', () => {
  const progress = calcRealtimeProgress({
    taskId: 1,
    scanFinish: true,
    doingTask: [{ status: 1, fileSize: 1000, progress: 50 }],
    createTime: 100,
    duration: 10,
    firstSync: 100,
    num: { wait: 0, running: 1, success: 0, fail: 0, other: 0 },
    size: { wait: 0, running: 1000, success: 400, fail: 0, other: 0 },
  }, { duration: 5, doneSize: 400 });

  assert.equal(progress.doneSize, 900);
  assert.equal(progress.remainSize, 500);
  assert.equal(progress.speed, 100);
});

test('applyDoingPatch keeps unchanged row references and merges progress', () => {
  const existing: TaskItem[] = [
    { alistTaskId: 'a', status: 1, fileName: 'a.bin', fileSize: 1000, progress: 10, srcPath: '/s', dstPath: '/d' },
    { alistTaskId: 'b', status: 1, fileName: 'b.bin', fileSize: 2000, progress: 20, srcPath: '/s', dstPath: '/d' },
  ];
  const patched = applyDoingPatch(existing, [{ alistTaskId: 'a', status: 1, progress: 40 }]);

  assert.notEqual(patched[0], existing[0]);
  assert.equal(patched[0].progress, 40);
  assert.equal(patched[0].fileName, 'a.bin');
  assert.equal(patched[1], existing[1]);
});

test('mergeCurrentTaskData applies doingPatch only for the same running task', () => {
  const previous = {
    taskId: 9,
    scanFinish: true,
    doingTask: [{ alistTaskId: 'a', status: 1, fileName: 'a.bin', progress: 10 }],
    createTime: 100,
    duration: 4,
    num: { wait: 0, running: 1, success: 0, fail: 0, other: 0 },
    size: { wait: 0, running: 1000, success: 0, fail: 0, other: 0 },
    doneSize: 100,
    remainSize: 900,
  };
  const patched = mergeCurrentTaskData({
    ...previous,
    duration: 5,
    doingTask: undefined,
    doingPatch: [{ alistTaskId: 'a', status: 1, progress: 55 }],
    doneSize: 550,
    remainSize: 450,
  }, previous);

  assert.equal(patched.doingTask?.[0].progress, 55);
  assert.equal(patched.doingTask?.[0].fileName, 'a.bin');
  assert.equal(patched.duration, 5);

  const replaced = mergeCurrentTaskData({
    ...previous,
    taskId: 10,
    createTime: 200,
    doingTask: [{ alistTaskId: 'z', status: 1, fileName: 'z.bin', progress: 1 }],
    doingPatch: [{ alistTaskId: 'a', status: 1, progress: 99 }],
  }, previous);
  assert.equal(replaced.doingTask?.[0].fileName, 'z.bin');
});

test('history and detail poll intervals slow down when no work is running', () => {
  assert.equal(historyHasActiveTask([{ status: 2 }, { status: 1 }]), true);
  assert.equal(historyHasActiveTask([{ status: 2 }, { status: 7 }]), false);
  assert.equal(detailHasRunningItem([{ status: 1 }, { status: 2 }]), true);
  assert.equal(detailHasRunningItem([{ status: 2 }, { status: 7 }]), false);
  assert.equal(pollIntervalForActiveWork(true, 3000), 3000);
  assert.equal(pollIntervalForActiveWork(false, 3000), 15000);
});
