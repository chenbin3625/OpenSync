import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { after, before, test } from 'node:test';
import { chromium, expect } from '@playwright/test';
import { createServer } from 'vite';

let server, browser, baseURL, fixtureHTML;
before(async () => {
  server = await createServer({ server: { host: '127.0.0.1', port: 0 } });
  await server.listen();
  baseURL = server.resolvedUrls.local[0];
  fixtureHTML = await server.transformIndexHtml('/__task_settings_fixture__',
    readFileSync(new URL('../index.html', import.meta.url), 'utf8')
      .replace('/src/main.tsx', '/tests/taskSettings.fixture.tsx'));
  browser = await chromium.launch({ headless: true });
});
after(async () => { await browser?.close(); await server?.close(); });

async function openPage(t, width = 1280) {
  const page = await browser.newPage({ viewport: { width, height: 900 } });
  t.after(() => page.close());
  const errors = [];
  page.on('pageerror', (error) => errors.push(error.message));
  t.after(() => assert.deepEqual(errors, []));
  await page.route('**/__task_settings_fixture__*', (route) =>
    route.fulfill({ contentType: 'text/html', body: fixtureHTML }));
  return page;
}

for (const width of [320, 1280]) test(`task timeout displays and saves hours at ${width}px`, async (t) => {
  const page = await openPage(t, width);
  const settings = { expires: 720, taskTimeout: 1, taskSave: 500, copyConcurrency: 4, scanConcurrency: 4, maxRetries: 3 };
  const submitted = [];
  await page.route('**/svr/system/config', (route) => {
    if (route.request().method() === 'PUT') submitted.push(route.request().postDataJSON());
    return route.fulfill({ json: { code: 200, data: settings } });
  });
  await page.goto(`${baseURL}__task_settings_fixture__?settings`);
  const timeout = page.getByRole('spinbutton', { name: '单次任务超时时间', exact: true });
  await expect(timeout).toHaveValue('1');
  const field = timeout.locator('xpath=ancestor::div[div/label][1]');
  await expect(field.getByText('小时', { exact: true })).toBeVisible();
  await expect(timeout).toHaveAttribute('min', '0');
  await expect(timeout).toHaveAttribute('max', '8760');
  await page.getByRole('button', { name: '单次任务超时时间说明' }).hover();
  await expect(page.getByRole('tooltip')).toContainText('0 表示不限制');
  await page.getByRole('button', { name: '保存系统配置' }).click();
  await expect.poll(() => submitted.length).toBe(1);
  assert.equal(submitted[0].taskTimeout, 1);
  await timeout.fill('0');
  await page.getByRole('button', { name: '保存系统配置' }).click();
  await expect.poll(() => submitted.length).toBe(2);
  assert.equal(submitted[1].taskTimeout, 0);
  await expect(page.getByRole('button', { name: '保存系统配置' })).toBeEnabled();
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
  if (process.env.OPENSYNC_SCREENSHOT_DIR) await page.screenshot({
    path: `${process.env.OPENSYNC_SCREENSHOT_DIR}/timeout-hours-${width}.png`, fullPage: true,
  });
});

test('missing task timeout uses the backend default of 48 hours', async (t) => {
  const page = await openPage(t);
  await page.route('**/svr/system/config', (route) => route.fulfill({ json: { code: 200, data: {} } }));
  await page.goto(`${baseURL}__task_settings_fixture__?settings`);
  await expect(page.getByRole('spinbutton', { name: '单次任务超时时间', exact: true })).toHaveValue('48');
});

for (const width of [320, 1280]) test(`task tab counts stay in their own status at ${width}px`, async (t) => {
  const page = await openPage(t, width);
  const doingTask = [1, 2].map((id) => ({ id, taskId: 42, fileName: `running-${id}.mkv`,
    srcPath: '/source', dstPath: '/backup', status: 1, type: 0, progress: 25, createTime: id }));
  const currentTask = { taskId: 42, scanFinish: true, createTime: Math.floor(Date.now() / 1000) - 20, duration: 20,
    num: { wait: 37, running: 2, success: 10, fail: 1, other: 0 }, doingTask };
  let releaseWaiting;
  const waitingGate = new Promise((resolve) => { releaseWaiting = resolve; });
  t.after(() => releaseWaiting());
  let delayWaiting = false;
  let waitingRequests = 0;
  await page.addInitScript((data) => {
    const originalFetch = window.fetch;
    window.fetch = (input, init) => {
      if (!String(input).startsWith('/svr/job/stream?')) return originalFetch(input, init);
      const stream = new ReadableStream({
        start(controller) {
          controller.enqueue(new TextEncoder().encode(`data: ${JSON.stringify({ code: 200, data })}\n\n`));
          init.signal.addEventListener('abort', () => controller.close(), { once: true });
        },
      });
      return Promise.resolve(new Response(stream, { headers: { 'Content-Type': 'text/event-stream' } }));
    };
  }, currentTask);
  await page.route('**/svr/job?*', async (route) => {
    const status = new URL(route.request().url()).searchParams.get('status');
    if (status === '0') {
      waitingRequests += 1;
      if (delayWaiting) await waitingGate;
    }
    return route.fulfill({ json: { code: 200, data: { count: 37,
      dataList: [{ id: 3, fileName: 'waiting.mkv', status: 0, type: 0, createTime: 3 }] } } }).catch(() => {});
  });
  await page.goto(`${baseURL}__task_settings_fixture__`);
  const running = page.getByRole('button', { name: /^运行中/ });
  const waiting = page.getByRole('button', { name: /^等待/ });
  await expect(running).toHaveText('运行中2');
  for (let i = 0; i < 3; i += 1) {
    await waiting.click();
    await expect(page.getByText('waiting.mkv', { exact: true })).toBeVisible();
    await running.click();
    await expect(page.getByText('running-1.mkv', { exact: true })).toBeVisible();
  }
  delayWaiting = true;
  const previousRequests = waitingRequests;
  await waiting.click();
  await expect.poll(() => waitingRequests).toBeGreaterThan(previousRequests);
  await running.click();
  releaseWaiting();
  await expect(running).toHaveText('运行中2');
  await expect(waiting).toHaveText('等待37');
  await expect(page.getByText('running-1.mkv', { exact: true })).toBeVisible();
  await expect(page.getByText('waiting.mkv', { exact: true })).toHaveCount(0);
  const snapshots = await page.evaluate(() => window.taskTabSnapshots);
  assert.ok(snapshots.some((counts) => counts.length === 2));
  for (const counts of snapshots.filter((counts) => counts.length === 2)) {
    assert.deepEqual(counts, ['等待37', '运行中2'], 'every committed view must use the current status counts');
  }
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
  if (process.env.OPENSYNC_SCREENSHOT_DIR) await page.screenshot({
    path: `${process.env.OPENSYNC_SCREENSHOT_DIR}/task-tab-counts-${width}.png`, fullPage: true,
  });
});
