import assert from 'node:assert/strict';
import test from 'node:test';
import { filterCurrentTaskFromHistory, getTaskItemKey, mergeTaskItems, mergeTaskRecords } from '../src/pages/Home/taskRows.ts';
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
