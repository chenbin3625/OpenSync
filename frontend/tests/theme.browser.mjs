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
  fixtureHTML = await server.transformIndexHtml('/__theme_fixture__',
    readFileSync(new URL('../index.html', import.meta.url), 'utf8').replace('/src/main.tsx', '/tests/theme.fixture.tsx'));
  browser = await chromium.launch({ headless: true });
});
after(async () => { await browser?.close(); await server?.close(); });

async function openPage(t, { stored, blocked = false, mobile = false } = {}) {
  const page = await browser.newPage({ viewport: mobile ? { width: 375, height: 812 } : { width: 1280, height: 800 }, colorScheme: 'dark' });
  t.after(() => page.close());
  const errors = [];
  page.on('pageerror', (err) => errors.push(err.message));
  t.after(() => assert.deepEqual(errors, []));
  await page.addInitScript(({ stored, blocked }) => {
    if (stored !== undefined) localStorage.setItem('opensync_theme', stored);
    if (blocked) Object.defineProperty(window, 'localStorage', { get() { throw new DOMException('Storage denied', 'SecurityError'); } });
  }, { stored, blocked });
  await page.route('**/svr/**', (route) => {
    const path = new URL(route.request().url()).pathname;
    return route.fulfill({ json: path === '/svr/user'
      ? { code: 401, msg: 'not logged in' }
      : { code: 200, msg: 'ok', data: { initialized: true } } });
  });
  await page.route('**/__theme_fixture__', (route) => route.fulfill({ contentType: 'text/html', body: fixtureHTML }));
  await page.goto(`${baseURL}login`);
  await expect(page.getByRole('heading', { name: 'OpenSync' })).toBeVisible();
  return page;
}

async function assertReadable(locator) {
  await expect.poll(() => locator.evaluate((el) => {
    const rgb = (value) => value.match(/[\d.]+/g).slice(0, 3).map(Number);
    const luminance = (values) => values.map((v) => {
      const c = v / 255;
      return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
    }).reduce((sum, value, i) => sum + value * [0.2126, 0.7152, 0.0722][i], 0);
    let parent = el;
    while (parent.parentElement && getComputedStyle(parent).backgroundColor === 'rgba(0, 0, 0, 0)') parent = parent.parentElement;
    const fg = luminance(rgb(getComputedStyle(el).color));
    const bg = luminance(rgb(getComputedStyle(parent).backgroundColor));
    return (Math.max(fg, bg) + 0.05) / (Math.min(fg, bg) + 0.05);
  })).toBeGreaterThanOrEqual(4.5);
}

test('light stays the default and the login toggle persists both themes', async (t) => {
  const page = await openPage(t);
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
  const toggle = page.getByRole('button', { name: '深色模式', exact: true });
  await expect(toggle).toHaveAttribute('aria-pressed', 'false');
  await toggle.click();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  assert.equal(await page.evaluate(() => localStorage.getItem('opensync_theme')), 'dark');
  await page.reload();
  await expect(toggle).toHaveAttribute('aria-pressed', 'true');
  await toggle.click();
  await page.reload();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
});

test('saved dark theme applies before the application module loads', async (t) => {
  const page = await openPage(t, { stored: 'dark' });
  let release;
  const pending = new Promise((resolve) => { release = resolve; });
  t.after(() => release());
  await page.route('**/src/main.tsx', async (route) => { await pending; await route.continue(); });
  await page.goto(`${baseURL}login`, { waitUntil: 'commit' });
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  assert.equal(await page.locator('html').evaluate((el) => getComputedStyle(el).colorScheme), 'dark');
  release();
  await expect(page.getByRole('button', { name: '深色模式', exact: true })).toHaveAttribute('aria-pressed', 'true');
});

test('invalid preferences fall back to light and blocked storage still allows switching', async (t) => {
  const invalid = await openPage(t, { stored: 'invalid' });
  await expect(invalid.locator('html')).toHaveAttribute('data-theme', 'light');
  const blocked = await openPage(t, { blocked: true });
  await blocked.getByRole('button', { name: '深色模式', exact: true }).click();
  await expect(blocked.locator('html')).toHaveAttribute('data-theme', 'dark');
});

for (const width of [320, 768, 1440]) test(`navigation, keyboard toggle and logout preserve the theme at ${width}px`, async (t) => {
  const page = await openPage(t);
  await page.setViewportSize({ width, height: 900 });
  await page.getByRole('button', { name: '深色模式', exact: true }).focus();
  await page.keyboard.press('Space');
  let signedIn = true;
  await page.route('**/svr/**', (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === '/svr/noAuth/login' && route.request().method() === 'DELETE') signedIn = false;
    let data = [];
    if (path === '/svr/user') data = { id: 1, userName: 'theme-test-user', createTime: 100 };
    if (path === '/svr/job') data = { dataList: [], count: 0 };
    if (path === '/svr/system/config') data = {};
    if (path === '/svr/noAuth/init') data = { initialized: true };
    return route.fulfill({ json: path === '/svr/user' && !signedIn
      ? { code: 401, msg: 'not logged in' }
      : { code: 200, msg: 'ok', data } });
  });
  await page.goto(`${baseURL}home`);
  await expect(page.getByRole('button', { name: '用户菜单' })).toBeVisible();
  for (const label of ['引擎管理', '通知配置', '系统设置']) {
    if (width < 768) await page.getByRole('button', { name: '切换移动菜单' }).click();
    await page.getByRole('button', { name: label, exact: true }).click();
    await expect(page.getByRole('heading', { name: label, exact: true })).toBeVisible();
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
  }
  if (process.env.OPENSYNC_SCREENSHOT_DIR) await page.screenshot({ path: `${process.env.OPENSYNC_SCREENSHOT_DIR}/dark-settings-${width}.png`, fullPage: true });
  await page.getByRole('button', { name: '用户菜单' }).click();
  await assertReadable(page.getByRole('menuitem', { name: '退出登录' }));
  await page.getByRole('menuitem', { name: '退出登录' }).click();
  await assertReadable(page.getByRole('alertdialog').getByRole('heading'));
  await page.getByRole('alertdialog').getByRole('button', { name: '退出登录', exact: true }).click();
  await expect(page).toHaveURL(`${baseURL}login`);
  await expect(page.getByRole('button', { name: '深色模式', exact: true })).toHaveAttribute('aria-pressed', 'true');
  if (process.env.OPENSYNC_SCREENSHOT_DIR) await page.screenshot({ path: `${process.env.OPENSYNC_SCREENSHOT_DIR}/dark-login-${width}.png`, fullPage: true });
});

for (const mobile of [false, true]) test(`dark surfaces and portals remain readable on ${mobile ? 'mobile' : 'desktop'}`, async (t) => {
  const page = await openPage(t, { mobile });
  await page.getByRole('button', { name: '深色模式', exact: true }).click();
  await page.goto(`${baseURL}__theme_fixture__`);
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  await assertReadable(page.getByRole('textbox', { name: '任务名称' }));
  await assertReadable(page.getByRole('cell', { name: '/media/photos' }));
  const switchControl = page.getByRole('switch', { name: '启用任务' });
  assert.equal(await switchControl.locator('span').evaluate((el) => getComputedStyle(el).backgroundColor), 'rgb(255, 255, 255)');
  await switchControl.click();
  await expect(switchControl).toHaveAttribute('aria-checked', 'false');
  await switchControl.click();
  await expect(switchControl).toHaveAttribute('aria-checked', 'true');
  for (const label of ['成功', '等待', '失败', '扫描中']) await assertReadable(page.getByText(label, { exact: true }));
  for (const label of ['打开弹窗', '删除任务']) await assertReadable(page.getByRole('button', { name: label, exact: true }));
  assert.equal(await page.getByRole('textbox', { name: '任务名称' }).evaluate((el) => getComputedStyle(el).backgroundColor === 'rgb(255, 255, 255)'), false);
  await page.getByRole('combobox', { name: '任务状态' }).click();
  await assertReadable(page.getByRole('option', { name: '失败任务' }));
  await page.keyboard.press('Escape');
  await page.getByRole('button', { name: '选择源目录', exact: true }).click();
  await assertReadable(page.getByText('media', { exact: true }));
  await page.keyboard.press('Escape');
  await page.getByRole('button', { name: '打开弹窗', exact: true }).click();
  await assertReadable(page.getByRole('dialog').getByRole('heading'));
  await assertReadable(page.getByRole('textbox', { name: '目标目录' }));
  if (process.env.OPENSYNC_SCREENSHOT_DIR) await page.screenshot({ path: `${process.env.OPENSYNC_SCREENSHOT_DIR}/dark-dialog-${mobile ? 'mobile' : 'desktop'}.png`, fullPage: true });
  await page.keyboard.press('Escape');
  await page.getByRole('button', { name: '打开抽屉', exact: true }).click();
  await assertReadable(page.getByRole('textbox', { name: '任务备注' }));
  await page.keyboard.press('Escape');
  await page.getByRole('button', { name: '保存通知', exact: true }).click();
  await assertReadable(page.getByText('已保存', { exact: true }));
  await page.getByRole('button', { name: '路径提示', exact: true }).hover();
  await assertReadable(page.getByRole('tooltip'));
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
  if (process.env.OPENSYNC_SCREENSHOT_DIR) await page.screenshot({ path: `${process.env.OPENSYNC_SCREENSHOT_DIR}/dark-controls-${mobile ? 'mobile' : 'desktop'}.png`, fullPage: true });
  await page.getByRole('button', { name: '深色模式', exact: true }).click();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
  await assertReadable(page.getByRole('textbox', { name: '任务名称' }));
  await page.reload();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
});
