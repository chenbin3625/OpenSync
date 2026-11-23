export class SSEHttpError extends Error {
  status: number;

  constructor(status: number, message?: string) {
    super(message || `sse ${status}`);
    this.name = 'SSEHttpError';
    this.status = status;
  }
}

export function parseSSEChunk(buffer: string, flush = false): { events: string[]; rest: string } {
  // Hold back a trailing '\r' across chunk boundaries so a split '\r\n' or
  // '\r\n\r\n' is never prematurely turned into a standalone '\n'.
  let trailingCR = '';
  let input = buffer;
  if (!flush && input.endsWith('\r')) {
    trailingCR = '\r';
    input = input.slice(0, -1);
  }

  let rest = input.replace(/\r\n/g, '\n').replace(/\r/g, '\n');
  const events: string[] = [];
  let sep = rest.indexOf('\n\n');
  while (sep >= 0) {
    const block = rest.slice(0, sep);
    rest = rest.slice(sep + 2);
    const dataLines: string[] = [];
    for (const line of block.split('\n')) {
      if (line.startsWith('data:')) {
        dataLines.push(line.slice(5).replace(/^ /, ''));
      }
    }
    if (dataLines.length > 0) {
      events.push(dataLines.join('\n'));
    }
    sep = rest.indexOf('\n\n');
  }
  return { events, rest: rest + trailingCR };
}

export function canUseFetchSSE(): boolean {
  return typeof fetch === 'function' && typeof ReadableStream !== 'undefined';
}

export async function readSSEStream(
  url: string,
  signal: AbortSignal,
  onMessage: (data: string) => void,
): Promise<void> {
  const response = await fetch(url, {
    method: 'GET',
    credentials: 'same-origin',
    headers: { Accept: 'text/event-stream' },
    signal,
    cache: 'no-store',
  });
  if (!response.ok) {
    await response.body?.cancel().catch(() => {});
    throw new SSEHttpError(response.status);
  }
  const contentType = response.headers.get('content-type') || '';
  if (!contentType.toLowerCase().includes('text/event-stream')) {
    await response.body?.cancel().catch(() => {});
    throw new Error(`unexpected sse content-type: ${contentType || 'none'}`);
  }
  if (!response.body) {
    throw new Error('sse empty body');
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) {
        buffer += decoder.decode();
        if (buffer) {
          const parsed = parseSSEChunk(buffer, true);
          for (const data of parsed.events) {
            onMessage(data);
          }
        }
        break;
      }
      buffer += decoder.decode(value, { stream: true });
      const parsed = parseSSEChunk(buffer);
      buffer = parsed.rest;
      for (const data of parsed.events) {
        onMessage(data);
      }
    }
  } finally {
    await reader.cancel().catch(() => {});
    try {
      reader.releaseLock();
    } catch {
      // ignore if lock was already released
    }
  }
}
