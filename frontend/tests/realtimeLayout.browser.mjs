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
  fixtureHTML = await server.transformIndexHtml('/__realtime_fixture__',
    readFileSync(new URL('../index.html', import.meta.url), 'utf8').replace('/src/main.tsx', '/tests/realtimeLayout.fixture.tsx'));
  browser = await chromium.launch({ headless: true });
});
after(async () => { await browser?.close(); await server?.close(); });

for (const theme of ['light', 'dark']) for (const width of [320, 375, 640, 767, 768, 1024, 1440]) {
  test(`realtime values fit at ${width}px in ${theme} mode`, async (t) => {
    const page = await browser.newPage({ viewport: { width, height: 900 } });
    t.after(() => page.close());
    const errors = [];
    page.on('pageerror', (error) => errors.push(error.message));
    await page.addInitScript((value) => localStorage.setItem('opensync_theme', value), theme);
    await page.route('**/__realtime_fixture__', (route) => route.fulfill({ contentType: 'text/html', body: fixtureHTML }));
    await page.goto(`${baseURL}__realtime_fixture__`);
    const remaining = page.getByText('4天 18小时 40分 59秒', { exact: true });
    await expect(remaining).toBeVisible();
    if (process.env.OPENSYNC_SCREENSHOT_DIR) await page.screenshot({
      path: `${process.env.OPENSYNC_SCREENSHOT_DIR}/realtime-${theme}-${width}.png`, fullPage: true,
    });
    const fits = (el) => el.scrollWidth <= el.clientWidth + 1 && el.scrollHeight <= el.clientHeight + 1;
    assert.equal(await remaining.evaluate(fits), true, 'remaining time must be fully visible');
    for (const name of ['Family.Holiday.2026.2160p.UHD.BluRay.REMUX.HEVC.DTS-HD.MA.7.1.mkv', 'L'.repeat(160) + '.mkv']) {
      const filename = page.getByText(name, { exact: true });
      await expect(filename).toBeVisible();
      if (width < 640) {
        assert.equal(await filename.evaluate(fits), true, 'mobile filename must be fully visible');
        const bounds = await filename.boundingBox();
        const rowWidth = await filename.evaluate((el) => el.parentElement.parentElement.parentElement.getBoundingClientRect().width);
        assert.ok(bounds.width >= rowWidth / 2, 'filename should use most of the row width');
        for (const path of [`源: /source/${name}`, `目标: /backup/${name}`]) {
          const pathText = page.getByText(path, { exact: true });
          await expect(pathText).toBeVisible();
          assert.equal(await pathText.evaluate(fits), true, 'mobile source and destination paths must be fully visible');
        }
      }
    }
    await expect(page.getByText('39%', { exact: true })).toBeVisible();
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
    assert.deepEqual(errors, []);
  });

  test(`overview task names fit at ${width}px in ${theme} mode`, async (t) => {
    const page = await browser.newPage({ viewport: { width, height: 900 } });
    t.after(() => page.close());
    await page.addInitScript((value) => localStorage.setItem('opensync_theme', value), theme);
    await page.route('**/__realtime_fixture__', (route) => route.fulfill({ contentType: 'text/html', body: fixtureHTML }));
    await page.goto(`${baseURL}__realtime_fixture__`);
    for (const name of ['家庭影音资料与旅行照片每日自动备份到云盘的同步任务', 'BackupJob'.repeat(20)]) {
      const heading = page.getByRole('heading', { name, exact: true });
      await expect(heading).toBeVisible();
      const layout = await heading.evaluate((el) => {
        const bounds = el.getBoundingClientRect();
        return {
          fits: el.scrollWidth <= el.clientWidth + 1 && el.scrollHeight <= el.clientHeight + 1,
          contained: bounds.left >= 0 && bounds.right <= innerWidth,
          badgesClear: [...el.parentElement.querySelectorAll('span')].every((badge) => {
            const box = badge.getBoundingClientRect();
            return box.top >= bounds.bottom || box.left >= bounds.right || box.right <= bounds.left;
          }),
        };
      });
      assert.equal(layout.fits, true, 'overview task name must be fully visible');
      assert.equal(layout.contained, true, 'overview title must stay within the viewport');
      assert.equal(layout.badgesClear, true, 'status badges must not overlap the task name');
    }
    await expect(page.getByRole('button', { name: '手动执行', exact: true })).toHaveCount(2);
    for (const button of await page.getByRole('button', { name: /^(手动执行|编辑|删除)$/ }).all()) {
      const bounds = await button.boundingBox();
      assert.ok(bounds && bounds.x >= 0 && bounds.x + bounds.width <= width, 'overview action buttons must stay within the viewport');
    }
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
  });
}
