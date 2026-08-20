import assert from 'node:assert/strict';
import test from 'node:test';
import { parseSSEChunk } from '../src/pages/Home/sseStream.ts';

test('parseSSEChunk splits EventStream frames and ignores comments', () => {
  const parsed = parseSSEChunk('data: {"code":200}\n\n: heartbeat\n\ndata: {"code":201}\n\npartial');
  assert.deepEqual(parsed.events, ['{"code":200}', '{"code":201}']);
  assert.equal(parsed.rest, 'partial');
});

test('parseSSEChunk joins multi-line data and accepts CRLF', () => {
  const parsed = parseSSEChunk('data: hello\ndata: world\r\n\r\n');
  assert.deepEqual(parsed.events, ['hello\nworld']);
  assert.equal(parsed.rest, '');
});
