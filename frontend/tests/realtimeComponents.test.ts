import assert from 'node:assert/strict';
import test from 'node:test';
import { formatDuration, formatSchedulePlan, getTaskDisplayName } from '../src/pages/Home/homeUtils.ts';
import type { TaskItem } from '../src/types.ts';

test('formatDuration handles 0 and negative seconds', () => {
  assert.equal(formatDuration(0), '0秒');
  assert.equal(formatDuration(-10), '0秒');
});

test('formatDuration formats seconds, minutes, hours, days', () => {
  assert.equal(formatDuration(45), '45秒');
  assert.equal(formatDuration(125), '2分 5秒');
  assert.equal(formatDuration(3665), '1小时 1分 5秒');
  assert.equal(formatDuration(90000), '1天 1小时');
});

test('formatSchedulePlan renders cron day 0 as Sunday instead of week 0', () => {
  assert.equal(
    formatSchedulePlan({
      isCron: 1,
      second: '0',
      minute: '30',
      hour: '8',
      day: '*',
      month: '*',
      day_of_week: '0',
    }),
    '每周 周日 的 08:30:00 执行'
  );
});

test('getTaskDisplayName prioritizes fileName', () => {
  const task: TaskItem = {
    status: 1,
    fileName: 'photo.jpg',
    srcPath: '/nas/photos/old.jpg',
    dstPath: '/cloud/photos/old.jpg',
  };
  assert.equal(getTaskDisplayName(task), 'photo.jpg');
});

test('getTaskDisplayName falls back to dstPath or srcPath tail', () => {
  const taskDstOnly: TaskItem = {
    status: 1,
    dstPath: '/cloud/backup/document.pdf/',
  };
  assert.equal(getTaskDisplayName(taskDstOnly), 'document.pdf');

  const taskSrcOnly: TaskItem = {
    status: 1,
    srcPath: '/local/media/movie.mkv',
  };
  assert.equal(getTaskDisplayName(taskSrcOnly), 'movie.mkv');

  const taskEmpty: TaskItem = {
    status: 1,
  };
  assert.equal(getTaskDisplayName(taskEmpty), '--');
});
