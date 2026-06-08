import assert from 'node:assert/strict';
import test from 'node:test';
import {
  filterCurrentTaskFromHistory,
  filterRunningTaskRows,
  getRealtimeTaskIdentity,
  getTaskItemKey,
  mergeTaskItems,
  mergeTaskRecords,
  normalizeTaskItemPage,
  shouldClearRealtimeRows,
  shouldResetRealtimeTotal,
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

test('shouldPollRealtime only enables polling for views that render realtime content', () => {
  assert.equal(shouldPollRealtime('all'), true);
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

test('shouldResetRealtimeTotal keeps pagination total when only page changes', () => {
  assert.equal(shouldResetRealtimeTotal(null, { status: 2, taskIdentity: '10:100', page: 1 }), true);
  assert.equal(
    shouldResetRealtimeTotal(
      { status: 2, taskIdentity: '10:100', page: 1 },
      { status: 2, taskIdentity: '10:100', page: 2 },
    ),
    false,
  );
  assert.equal(
    shouldResetRealtimeTotal(
      { status: 2, taskIdentity: '10:100', page: 1 },
      { status: 7, taskIdentity: '10:100', page: 1 },
    ),
    true,
  );
  assert.equal(
    shouldResetRealtimeTotal(
      { status: 2, taskIdentity: '10:100', page: 1 },
      { status: 2, taskIdentity: '11:200', page: 1 },
    ),
    true,
  );
});

test('shouldClearRealtimeRows keeps current rows visible when only page changes', () => {
  assert.equal(shouldClearRealtimeRows(null, { status: 2, taskIdentity: '10:100', page: 1 }), true);
  assert.equal(
    shouldClearRealtimeRows(
      { status: 2, taskIdentity: '10:100', page: 1 },
      { status: 2, taskIdentity: '10:100', page: 2 },
    ),
    false,
  );
  assert.equal(
    shouldClearRealtimeRows(
      { status: 2, taskIdentity: '10:100', page: 1 },
      { status: 7, taskIdentity: '10:100', page: 1 },
    ),
    true,
  );
  assert.equal(
    shouldClearRealtimeRows(
      { status: 2, taskIdentity: '10:100', page: 1 },
      { status: 2, taskIdentity: '11:200', page: 1 },
    ),
    true,
  );
});
