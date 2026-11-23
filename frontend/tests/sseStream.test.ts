import assert from 'node:assert/strict';
import test from 'node:test';
import {
  canUseFetchSSE,
  parseSSEChunk,
  readSSEStream,
  SSEHttpError,
} from '../src/pages/Home/sseStream.ts';

test('parseSSEChunk parses standard LF events, multi-line data, and ignores comments', () => {
  const input = [
    ': keepalive',
    'event: progress',
    'id: 1',
    'data: {"step":1}',
    '',
    'data: line1',
    'data:  line2',
    '',
    'data: partial',
  ].join('\n');

  const parsed = parseSSEChunk(input);

  assert.deepEqual(parsed.events, ['{"step":1}', 'line1\n line2']);
  assert.equal(parsed.rest, 'data: partial');
});

test('parseSSEChunk handles CRLF delimiters split across arbitrary chunk boundaries', () => {
  const payload = 'data: first\r\n\r\ndata: second\r\n\r\n';

  // Split into 2 chunks at every possible byte boundary (including inside \r\n\r\n)
  for (let splitAt = 1; splitAt < payload.length; splitAt += 1) {
    const first = parseSSEChunk(payload.slice(0, splitAt));
    const second = parseSSEChunk(first.rest + payload.slice(splitAt));
    assert.deepEqual(
      [...first.events, ...second.events],
      ['first', 'second'],
      `split at offset ${splitAt} should preserve all CRLF events`,
    );
    assert.equal(second.rest, '');
  }

  // Stream 1 character at a time
  let rest = '';
  const received: string[] = [];
  for (const ch of payload) {
    const parsed = parseSSEChunk(rest + ch);
    received.push(...parsed.events);
    rest = parsed.rest;
  }
  assert.deepEqual(received, ['first', 'second']);
  assert.equal(rest, '');
});

test('parseSSEChunk holds trailing CR until next chunk or EOF flush', () => {
  const held = parseSSEChunk('data: hello\r\n\r', false);
  assert.deepEqual(held.events, []);
  assert.equal(held.rest, 'data: hello\n\r');

  // Next chunk completes CRLF without creating a spurious blank event
  const completed = parseSSEChunk(`${held.rest}\ndata: next\r\n\r\n`, false);
  assert.deepEqual(completed.events, ['hello', 'next']);
  assert.equal(completed.rest, '');

  // Bare CR delimiter at EOF flushes cleanly when flush=true
  const bareCRHeld = parseSSEChunk('data: final\r\r', false);
  assert.deepEqual(bareCRHeld.events, []);
  assert.equal(bareCRHeld.rest, 'data: final\n\r');

  const flushed = parseSSEChunk(bareCRHeld.rest, true);
  assert.deepEqual(flushed.events, ['final']);
  assert.equal(flushed.rest, '');
});

test('parseSSEChunk does not emit unterminated trailing data on flush', () => {
  const parsed = parseSSEChunk('data: incomplete', true);
  assert.deepEqual(parsed.events, []);
  assert.equal(parsed.rest, 'data: incomplete');
});

test('readSSEStream handles chunk-split CRLF frames and EOF flush', async () => {
  assert.equal(canUseFetchSSE(), true);
  const encoder = new TextEncoder();
  const chunks = [
    'data: {"id":1}\r',
    '\n\r',
    '\ndata: {"id":2}\r\r',
  ];
  const originalFetch = globalThis.fetch;
  const received: string[] = [];

  globalThis.fetch = (async () => new Response(
    new ReadableStream<Uint8Array>({
      start(controller) {
        for (const chunk of chunks) {
          controller.enqueue(encoder.encode(chunk));
        }
        controller.close();
      },
    }),
    {
      status: 200,
      headers: { 'content-type': 'text/event-stream; charset=utf-8' },
    },
  )) as typeof fetch;

  try {
    const abortController = new AbortController();
    await readSSEStream('/svr/job/stream?id=1', abortController.signal, (data) => {
      received.push(data);
    });
  } finally {
    globalThis.fetch = originalFetch;
  }

  assert.deepEqual(received, ['{"id":1}', '{"id":2}']);
});

test('readSSEStream throws SSEHttpError on non-OK HTTP status and rejects non-SSE content-type', async () => {
  const originalFetch = globalThis.fetch;

  globalThis.fetch = (async () => new Response('unauthorized', {
    status: 401,
    headers: { 'content-type': 'text/plain' },
  })) as typeof fetch;

  try {
    await assert.rejects(
      () => readSSEStream('/svr/job/stream?id=1', new AbortController().signal, () => {}),
      (err: unknown) => err instanceof SSEHttpError && err.status === 401,
    );

    globalThis.fetch = (async () => new Response('{"code":200}', {
      status: 200,
      headers: { 'content-type': 'application/json' },
    })) as typeof fetch;

    await assert.rejects(
      () => readSSEStream('/svr/job/stream?id=1', new AbortController().signal, () => {}),
      /unexpected sse content-type/,
    );
  } finally {
    globalThis.fetch = originalFetch;
  }
});
