import assert from 'node:assert/strict';
import test from 'node:test';
import { fileSizeToBytes, splitBytesToFileSize } from '../src/pages/Home/fileSizeUnits.ts';

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
