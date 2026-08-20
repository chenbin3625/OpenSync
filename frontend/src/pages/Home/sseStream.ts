export function parseSSEChunk(buffer: string): { events: string[]; rest: string } {
  let rest = buffer.replace(/\r\n/g, '\n').replace(/\r/g, '\n');
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
  return { events, rest };
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
  if (!response.ok || !response.body) {
    throw new Error(`sse ${response.status}`);
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    const parsed = parseSSEChunk(buffer);
    buffer = parsed.rest;
    for (const data of parsed.events) {
      onMessage(data);
    }
  }
}
