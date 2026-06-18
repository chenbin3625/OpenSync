import assert from 'node:assert/strict';
import test from 'node:test';
import { fileSizeToBytes, splitBytesToFileSize } from '../src/pages/Home/fileSizeUnits.ts';
import { buildPathTreeData, mergeTreeData, normalizeTreePath } from '../src/pages/Home/homeUtils.ts';

test('fileSizeToBytes converts selected units to bytes', () => {
  assert.equal(fileSizeToBytes(512, 'B'), 512);
  assert.equal(fileSizeToBytes(2, 'KB'), 2048);
  assert.equal(fileSizeToBytes(1.5, 'GB'), 1610612736);
});

test('splitBytesToFileSize chooses a readable unit for existing byte values', () => {
  assert.deepEqual(splitBytesToFileSize(0), { value: 0, unit: 'MB' });
  assert.deepEqual(splitBytesToFileSize(2048), { value: 2, unit: 'KB' });
  assert.deepEqual(splitBytesToFileSize(3145728), { value: 3, unit: 'MB' });
  assert.deepEqual(splitBytesToFileSize(5 * 1024 * 1024 * 1024), { value: 5, unit: 'GB' });
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
