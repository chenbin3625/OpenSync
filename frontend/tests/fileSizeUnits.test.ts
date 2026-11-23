import assert from 'node:assert/strict';
import test from 'node:test';
import {
  fileSizeToBytes,
  MAX_SAFE_FILE_SIZE_BYTES,
  normalizeFileSizeUnit,
  splitBytesToFileSize,
} from '../src/pages/Home/fileSizeUnits.ts';
import { buildPathTreeData, mergeTreeData, normalizeTreePath } from '../src/pages/Home/homeUtils.ts';

test('normalizeFileSizeUnit accepts valid units and falls back to MB', () => {
  assert.equal(normalizeFileSizeUnit('B'), 'B');
  assert.equal(normalizeFileSizeUnit('TB'), 'TB');
  assert.equal(normalizeFileSizeUnit('PB'), 'MB');
  assert.equal(normalizeFileSizeUnit(null), 'MB');
  assert.equal(normalizeFileSizeUnit(undefined), 'MB');
});

test('fileSizeToBytes converts selected units to bytes', () => {
  assert.equal(fileSizeToBytes(512, 'B'), 512);
  assert.equal(fileSizeToBytes(2, 'KB'), 2048);
  assert.equal(fileSizeToBytes(1.5, 'GB'), 1610612736);
  assert.equal(fileSizeToBytes(1, 'TB'), 1024 ** 4);
  assert.equal(fileSizeToBytes(0, 'MB'), 0);
  assert.equal(fileSizeToBytes(-10, 'MB'), 0);
  assert.equal(fileSizeToBytes(Number.NaN, 'MB'), 0);
  assert.equal(fileSizeToBytes(Number.POSITIVE_INFINITY, 'MB'), 0);
});

test('fileSizeToBytes clamps overflowing values to MAX_SAFE_FILE_SIZE_BYTES', () => {
  assert.equal(fileSizeToBytes(9000, 'TB'), MAX_SAFE_FILE_SIZE_BYTES);
  assert.equal(fileSizeToBytes(Number.MAX_SAFE_INTEGER, 'KB'), MAX_SAFE_FILE_SIZE_BYTES);
  assert.equal(fileSizeToBytes(1e308, 'TB'), MAX_SAFE_FILE_SIZE_BYTES);
});

test('splitBytesToFileSize chooses a readable unit for existing byte values', () => {
  assert.deepEqual(splitBytesToFileSize(0), { value: 0, unit: 'MB' });
  assert.deepEqual(splitBytesToFileSize(-1), { value: 0, unit: 'MB' });
  assert.deepEqual(splitBytesToFileSize(Number.NaN), { value: 0, unit: 'MB' });
  assert.deepEqual(splitBytesToFileSize(Number.POSITIVE_INFINITY), { value: 0, unit: 'MB' });
  assert.deepEqual(splitBytesToFileSize(2048), { value: 2, unit: 'KB' });
  assert.deepEqual(splitBytesToFileSize(1536), { value: 1.5, unit: 'KB' });
  assert.deepEqual(splitBytesToFileSize(3145728), { value: 3, unit: 'MB' });
  assert.deepEqual(splitBytesToFileSize(5 * 1024 * 1024 * 1024), { value: 5, unit: 'GB' });
  assert.deepEqual(splitBytesToFileSize(2 * 1024 ** 4), { value: 2, unit: 'TB' });
});

test('splitBytesToFileSize preserves exact round-trip to bytes and clamps above MAX_SAFE_FILE_SIZE_BYTES', () => {
  const samples = [
    1,
    512,
    1024,
    1025,
    1536,
    1048576,
    1048577,
    1572864,
    1234567,
    1.25 * 1024 ** 2,
    1.5 * 1024 ** 3,
    1024 ** 4,
    1024 ** 4 + 1,
    MAX_SAFE_FILE_SIZE_BYTES,
  ];

  for (const bytes of samples) {
    const split = splitBytesToFileSize(bytes);
    assert.equal(
      fileSizeToBytes(split.value, split.unit),
      bytes,
      `round-trip failed for ${bytes} bytes (got ${split.value} ${split.unit})`,
    );
  }

  // Non-divisible byte counts fall back to B so precision is never silently lost
  assert.deepEqual(splitBytesToFileSize(1025), { value: 1025, unit: 'B' });
  assert.deepEqual(splitBytesToFileSize(1048577), { value: 1048577, unit: 'B' });

  // Values above MAX_SAFE_FILE_SIZE_BYTES clamp safely
  assert.deepEqual(splitBytesToFileSize(MAX_SAFE_FILE_SIZE_BYTES * 2), {
    value: MAX_SAFE_FILE_SIZE_BYTES,
    unit: 'B',
  });
});

test('normalizeTreePath keeps backend paths stable for tree-select values', () => {
  assert.equal(normalizeTreePath('media/movies/'), '/media/movies');
  assert.equal(normalizeTreePath('//media///shows'), '/media/shows');
  assert.equal(normalizeTreePath('/'), '/');
});

test('buildPathTreeData creates selected path nodes before async tree loading finishes', () => {
  const tree = buildPathTreeData(['/media/movies/2026', '/media/music']);

  assert.equal(tree[0].value, '/');
  assert.deepEqual(
    tree[0].children?.map((node) => node.value),
    ['/media'],
  );
  assert.deepEqual(
    tree[0].children?.[0].children?.map((node) => node.value),
    ['/media/movies', '/media/music'],
  );
  assert.equal(tree[0].children?.[0].children?.[0].children?.[0].value, '/media/movies/2026');
});

test('mergeTreeData keeps loaded directory nodes and adds selected descendants', () => {
  const loadedTree = [{
    title: '/',
    value: '/',
    key: '/',
    children: [{ title: 'media', value: '/media', key: '/media', isLeaf: false }],
  }];
  const selectedTree = buildPathTreeData(['/media/movies']);

  const merged = mergeTreeData(loadedTree, selectedTree);

  assert.deepEqual(
    merged[0].children?.[0].children?.map((node) => node.value),
    ['/media/movies'],
  );
});
