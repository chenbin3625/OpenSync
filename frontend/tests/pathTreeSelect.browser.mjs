import assert from 'node:assert/strict';
import { after, before, test } from 'node:test';
import { chromium } from '@playwright/test';
import { createServer } from 'vite';

let server;
let browser;
let baseURL;

before(async () => {
  server = await createServer({ server: { host: '127.0.0.1', port: 0 } });
  await server.listen();
  baseURL = server.resolvedUrls.local[0];
  browser = await chromium.launch({ headless: true });
});

after(async () => {
  await browser?.close();
  await server?.close();
});

async function openFixture(t, directoryResponse, viewport = { width: 1280, height: 800 }) {
  const page = await browser.newPage({ viewport });
  t.after(() => page.close());
  const requests = [];
  await page.route('**/svr/alist?**', async (route) => {
    const url = new URL(route.request().url());
    const path = url.searchParams.get('path');
    const engine = Number(url.searchParams.get('alistId'));
    requests.push({ engine, path });
    const response = await directoryResponse(path, engine);
    await route.fulfill({ json: Array.isArray(response)
      ? { code: 200, msg: 'ok', data: response }
      : response });
  });
  await page.route('**/__path_tree_test__', (route) => route.fulfill({
    contentType: 'text/html',
    body: '<html><meta name="viewport" content="width=device-width, initial-scale=1"><div id="root"></div><script type="module">import RefreshRuntime from "/@react-refresh"; RefreshRuntime.injectIntoGlobalHook(window); window.$RefreshReg$ = () => {}; window.$RefreshSig$ = () => (type) => type; window.__vite_plugin_react_preamble_installed__ = true;</script><script type="module" src="/tests/pathTreeSelect.fixture.tsx"></script></html>',
  }));
  await page.goto(`${baseURL}__path_tree_test__`);
  await page.getByRole('button').filter({ hasText: '/media/movies/2026' }).click();
  return { page, requests };
}

async function expand(page, title) {
  const row = page.getByText(title, { exact: true }).locator('..');
  await row.locator('button').first().click();
}

test('editing a selected path loads its siblings and keeps the selection', async (t) => {
  const { page, requests } = await openFixture(t, (path) => {
    if (path === '/') return [{ name: 'media' }];
    if (path === '/media') return [{ name: 'movies' }, { name: 'music' }];
    return [];
  });
  await expand(page, '/');
  await expand(page, 'media');
  await page.getByText('music', { exact: true }).waitFor({ timeout: 3000 });
  if (process.env.OPENSYNC_SCREENSHOT_DIR) {
    await page.screenshot({ path: `${process.env.OPENSYNC_SCREENSHOT_DIR}/path-tree-desktop.png`, fullPage: true });
  }
  assert.equal(requests.filter((r) => r.path === '/media').length, 1);
  assert.equal(await page.locator('output').textContent(), '["/media/movies/2026"]');
  await page.getByText('music', { exact: true }).click();
  assert.equal(await page.locator('output').textContent(), '["/media/movies/2026","/media/music"]');
});

test('a failed expansion can be retried', async (t) => {
  let attempts = 0;
  const { page } = await openFixture(t, (path) => {
    if (path === '/') return [{ name: 'media' }];
    if (path === '/media' && ++attempts === 1) return { code: 500, msg: 'temporary failure' };
    return [{ name: 'music' }];
  });
  await expand(page, '/');
  await expand(page, 'media');
  await expand(page, 'media');
  await expand(page, 'media');
  await page.getByText('music', { exact: true }).waitFor({ timeout: 3000 });
  assert.equal(attempts, 2);
});

test('switching engines does not reuse an in-flight directory request', async (t) => {
  let releaseFirst;
  const first = new Promise((resolve) => { releaseFirst = resolve; });
  t.after(() => releaseFirst([]));
  const { page, requests } = await openFixture(t, (path, engine) => {
    if (path === '/' && engine === 1) return first;
    return [{ name: 'engine-two' }];
  });
  const nextRoot = page.waitForResponse((response) => {
    const url = new URL(response.url());
    return url.searchParams.get('alistId') === '2';
  }, { timeout: 3000 });
  await page.getByRole('button', { name: 'Switch engine' }).click({ force: true });
  await nextRoot;
  releaseFirst([{ name: 'engine-one' }]);
  await page.getByRole('button').filter({ hasText: '/media/movies/2026' }).click();
  await expand(page, '/');
  await page.getByText('engine-two', { exact: true }).waitFor({ timeout: 3000 });
  assert.equal(await page.getByText('engine-one', { exact: true }).count(), 0);
  assert.equal(requests.filter((r) => r.engine === 2).length, 1);
});

test('refresh reloads directories without clearing selected paths on mobile', async (t) => {
  let refreshed = false;
  const longName = 'a-very-long-directory-name-that-must-stay-readable-at-deep-levels';
  const { page } = await openFixture(t, (path) => {
    if (path === '/') return [{ name: 'media' }];
    if (path === '/media') return [{ name: refreshed ? 'new-folder' : 'movies' }];
    return [{ name: longName }];
  }, { width: 375, height: 812 });
  await expand(page, '/');
  await expand(page, 'media');
  await expand(page, 'movies');
  const label = page.getByText(longName, { exact: true });
  await label.waitFor({ timeout: 3000 });
  assert.equal(await label.evaluate((el) => el.scrollWidth <= el.clientWidth), true);
  if (process.env.OPENSYNC_SCREENSHOT_DIR) {
    await page.screenshot({ path: `${process.env.OPENSYNC_SCREENSHOT_DIR}/path-tree-mobile.png`, fullPage: true });
  }
  refreshed = true;
  await page.getByRole('button', { name: '刷新目录', exact: true }).click();
  await expand(page, '/');
  await expand(page, 'media');
  await page.getByText('new-folder', { exact: true }).waitFor({ timeout: 3000 });
  assert.equal(await page.locator('output').textContent(), '["/media/movies/2026"]');
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
});
