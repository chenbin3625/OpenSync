import assert from 'node:assert/strict';
import test from 'node:test';
import {
  calcRealtimeProgress,
  filterCurrentTaskFromHistory,
  filterRunningTaskRows,
  getRealtimeTaskIdentity,
  getTaskItemKey,
  mergeCurrentTaskData,
  mergeTaskItems,
  mergeTaskRecords,
  normalizeTaskItemPage,
  pageTaskItems,
  shouldResetRealtimeSnapshot,
  shouldReplaceRealtimeRows,
  shouldPollRealtime,
  sortTaskItemsByCreateTimeDesc,
} from '../src/pages/Home/taskRows.ts';
import type { CurrentTaskData, TaskItem, TaskRecord } from '../src/types.ts';

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

test('getTaskItemKey avoids collisions when alistTaskId is empty or whitespace', () => {
  const first: TaskItem = { alistTaskId: '', status: 1, fileName: 'a.mkv', srcPath: '/s/a.mkv', dstPath: '/d/a.mkv' };
  const second: TaskItem = { alistTaskId: '   ', status: 1, fileName: 'b.mkv', srcPath: '/s/b.mkv', dstPath: '/d/b.mkv' };
  const duplicatePath: TaskItem = { alistTaskId: '', status: 1, fileName: 'a.mkv', srcPath: '/s/a.mkv', dstPath: '/d/a.mkv' };

  assert.equal(getTaskItemKey(first, 0), 'a.mkv|/s/a.mkv|/d/a.mkv|0');
  assert.equal(getTaskItemKey(second, 1), 'b.mkv|/s/b.mkv|/d/b.mkv|1');
  assert.notEqual(getTaskItemKey(first, 0), getTaskItemKey(second, 1));
  assert.notEqual(getTaskItemKey(first, 0), getTaskItemKey(duplicatePath, 2));
  assert.equal(getTaskItemKey({ id: 0, status: 1 }), '0');

  const merged = mergeTaskItems(
    [first, second],
    [{ ...first, progress: 50 }, second],
  );
  assert.notEqual(merged[0], first);
  assert.equal(merged[0].progress, 50);
  assert.equal(merged[1], second);
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

test('mergeCurrentTaskData clears rows on an empty full snapshot', () => {
  const previous: CurrentTaskData = {
    taskId: 10,
    scanFinish: false,
    createTime: 100,
    duration: 1,
    num: { wait: 0, running: 1, success: 0, fail: 0, other: 0 },
    size: { wait: 0, running: 1, success: 0, fail: 0, other: 0 },
    doingTask: [{ id: 1, status: 1, progress: 100 }],
  };
  const next: CurrentTaskData = {
    taskId: 10,
    scanFinish: false,
    createTime: 100,
    duration: 2,
    num: { wait: 0, running: 0, success: 0, fail: 0, other: 0 },
    size: { wait: 0, running: 0, success: 0, fail: 0, other: 0 },
    doingTask: [],
  };

  const merged = mergeCurrentTaskData(next, previous);

  assert.deepEqual(merged.doingTask, []);
});

test('stream patches preserve sorted rows and match items without IDs by path', () => {
  const older = { fileName: 'older.mkv', srcPath: '/source/older.mkv', dstPath: '/backup/older.mkv',
    status: 1, progress: 10, createTime: 10 };
  const newer = { id: 12, status: 1, progress: 20, createTime: 20 };
  const task: CurrentTaskData = {
    taskId: 10, createTime: 80, scanFinish: true, duration: 20,
    num: { wait: 0, running: 2, success: 0, fail: 0, other: 0 },
    size: { wait: 0, running: 2, success: 0, fail: 0, other: 0 },
    doingTask: [older, newer],
  };
  const snapshot = mergeCurrentTaskData(task, null);
  assert.deepEqual(snapshot.doingTask, [newer, older]);
  const { createTime: _createTime, ...patch } = { ...older, progress: 45 };
  const patched = mergeCurrentTaskData({ ...task, doingTask: null, doingPatch: [patch] }, snapshot);
  assert.equal(patched.doingTask?.[0], newer);
  assert.equal(patched.doingTask?.[1].progress, 45);
  assert.equal(mergeCurrentTaskData({ ...task, doingTask: null }, patched).doingTask, patched.doingTask);
  assert.deepEqual(mergeCurrentTaskData({ ...task, taskId: 11, doingTask: null }, patched).doingTask, []);
});

test('calcRealtimeProgress treats null/empty server metrics as absent instead of 0', () => {
  const cur = {
    taskId: 10,
    scanFinish: true,
    createTime: 1000,
    firstSync: 1010,
    duration: 20,
    num: { wait: 1, running: 1, success: 1, fail: 0, other: 0 },
    size: { wait: 500, running: 1000, success: 2000, fail: 0, other: 0 },
    doingTask: [
      { id: 1, status: 1, fileSize: 1000, progress: 50 },
    ],
    // Simulate JSON payload with explicit nulls that Number(null) would wrongly coerce to 0
    doneSize: null,
    remainSize: null,
    speed: null,
    speedAvg: null,
    remainTime: null,
  } as unknown as CurrentTaskData;

  const progress = calcRealtimeProgress(cur, { duration: 10, doneSize: 1500 });

  assert.equal(progress.doneSize, 2500);
  assert.equal(progress.remainSize, 1000);
  assert.equal(progress.speed, 100);
  assert.equal(progress.speedAvg, 250);
  assert.equal(progress.remainTime, 4);

  // Explicit server zero values are still honored
  const explicitZeros = calcRealtimeProgress({
    ...cur,
    doneSize: 0,
    remainSize: 0,
    speed: 0,
    speedAvg: 0,
    remainTime: 0,
  }, { duration: 10, doneSize: 1500 });
  assert.deepEqual(explicitZeros, {
    doneSize: 0,
    remainSize: 0,
    speed: 0,
    speedAvg: 0,
    remainTime: 0,
  });
});

test('calcRealtimeProgress clamps negative server and client-derived speeds and sizes to 0', () => {
  const negativeServer: CurrentTaskData = {
    taskId: 10,
    scanFinish: true,
    createTime: 1000,
    duration: 20,
    num: { wait: 0, running: 1, success: 0, fail: 0, other: 0 },
    size: { wait: 0, running: 0, success: 0, fail: 0, other: 0 },
    doneSize: -100,
    remainSize: -200,
    speed: -50,
    speedAvg: -25,
    remainTime: -10,
  };
  assert.deepEqual(calcRealtimeProgress(negativeServer, null), {
    doneSize: 0,
    remainSize: 0,
    speed: 0,
    speedAvg: 0,
    remainTime: 0,
  });

  // Client-side fallback: doneSize decreased vs previous snapshot and doingSize > size.running
  const clientFallback: CurrentTaskData = {
    taskId: 10,
    scanFinish: true,
    createTime: 1000,
    duration: 20,
    num: { wait: 0, running: 2, success: 0, fail: 0, other: 0 },
    size: { wait: 0, running: 100, success: 0, fail: 0, other: 0 },
    doingTask: [
      { id: 1, status: 1, fileSize: 200, progress: 150 },
      { id: 2, status: 1, fileSize: -50, progress: -20 },
    ],
  };
  const fallbackResult = calcRealtimeProgress(clientFallback, { duration: 10, doneSize: 1000 });
  assert.equal(fallbackResult.doneSize, 200);
  assert.equal(fallbackResult.remainSize, 0);
  assert.equal(fallbackResult.speed, 0);
  assert.equal(fallbackResult.speedAvg, 0);
  assert.equal(fallbackResult.remainTime, 0);
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

test('pageTaskItems paginates running snapshots and preserves server-paged rows', () => {
  const rows: TaskItem[] = [
    { id: 1, status: 1, createTime: 100 },
    { id: 2, status: 1, createTime: 300 },
    { id: 3, status: 1, createTime: 200 },
  ];

  assert.deepEqual(pageTaskItems(rows, 1, 1, 3).map((row) => row.id), [1, 2, 3]);
  assert.deepEqual(pageTaskItems(rows, 1, 2, 2).map((row) => row.id), [3]);

  const serverPage = rows.map((row) => ({ ...row, status: 2 }));
  assert.deepEqual(pageTaskItems(serverPage, 2, 3, 20).map((row) => row.id), [1, 2, 3]);
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
