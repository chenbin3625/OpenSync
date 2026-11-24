import assert from 'node:assert/strict';
import { after, before, test } from 'node:test';
import { chromium, expect } from '@playwright/test';
import { createServer } from 'vite';

let server, browser, baseURL;
before(async () => {
  server = await createServer({ server: { host: '127.0.0.1', port: 0 } });
  await server.listen();
  baseURL = server.resolvedUrls.local[0];
  browser = await chromium.launch({ headless: true });
});
after(async () => { await browser?.close(); await server?.close(); });

const jobs = ['相册备份', '归档文档', '媒体同步'].map((remark, index) => ({
  id: index + 1, enable: 1, remark, srcPath: '/source', dstPath: '/backup', alistId: 1,
  useCacheT: false, useCacheS: false, method: 0, interval: 1440, isCron: 1,
}));
const engines = [1, 2].map((id) => ({ id, url: `http://engine-${id}.test`, userName: `engine-${id}` }));

function respond(route, data) {
  return route.fulfill({ json: { code: 200, msg: '', data } });
}

async function openPage(t, width) {
  const page = await browser.newPage({ viewport: { width, height: 900 } });
  t.after(() => page.close());
  const errors = [];
  page.on('pageerror', (error) => errors.push(error.message));
  t.after(() => assert.deepEqual(errors, []));
  await page.route('**/svr/user', (route) => respond(route, { id: 1, userName: '演示用户', createTime: 1 }));
  await page.route('**/svr/alist', (route) => respond(route, []));
  return page;
}

for (const width of [320, 1440]) test(`job list shows loading, retry and keyboard selection at ${width}px`, async (t) => {
  const page = await openPage(t, width);
  let releaseFirstResponse;
  const firstResponse = new Promise((resolve) => { releaseFirstResponse = resolve; });
  t.after(() => releaseFirstResponse());
  let retry = false;
  await page.route('**/svr/job?*', async (route) => {
    if (retry) return respond(route, { dataList: jobs.slice(0, 2), count: 2 });
    await firstResponse;
    return route.fulfill({ status: 503, json: { code: 503, msg: 'Unavailable', data: null } });
  });
  await page.goto(`${baseURL}home`);
  await expect(page.getByRole('status').filter({ hasText: '正在加载同步任务' })).toBeVisible();
  await expect(page.getByText('暂无同步任务', { exact: true })).toHaveCount(0);
  releaseFirstResponse();
  await expect(page.getByText('同步任务列表加载失败', { exact: true })).toBeVisible();
  await expect(page.getByText('暂无同步任务', { exact: true })).toHaveCount(0);
  retry = true;
  await page.getByRole('button', { name: '重新加载' }).click();
  await expect(page.getByRole('heading', { name: '相册备份', exact: true })).toBeVisible();

  const sidebar = page.getByRole('complementary', { name: '同步任务列表' });
  const albums = sidebar.getByRole('button', { name: /^相册备份/ });
  const documents = sidebar.getByRole('button', { name: /^归档文档/ });
  await documents.press('Enter');
  await expect(documents).toHaveAttribute('aria-current', 'true');
  await expect(page.getByRole('heading', { name: '归档文档', exact: true })).toBeVisible();
  await albums.press('Space');
  await expect(albums).toHaveAttribute('aria-current', 'true');
  await expect(page.getByRole('heading', { name: '相册备份', exact: true })).toBeVisible();
  const tabs = page.getByRole('tablist', { name: '同步任务视图' });
  assert.equal(await tabs.getByRole('tab').evaluateAll((items) =>
    items.every((item) => item.scrollWidth <= item.clientWidth + 1)), true);
  const scheduleLabel = page.getByText('调度:', { exact: true });
  assert.equal(await scheduleLabel.evaluate((label) =>
    label.clientHeight <= Number.parseFloat(getComputedStyle(label).lineHeight) + 1), true);
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
  if (process.env.OPENSYNC_SCREENSHOT_DIR) await page.screenshot({
    path: `${process.env.OPENSYNC_SCREENSHOT_DIR}/workspace-${width}.png`, fullPage: true,
  });
});

test('pagination waits for the matching page before selecting a job', async (t) => {
  const page = await openPage(t, 1280);
  let releaseSecondPage;
  const secondPage = new Promise((resolve) => { releaseSecondPage = resolve; });
  t.after(() => releaseSecondPage());
  await page.route('**/svr/job?*', async (route) => {
    const number = new URL(route.request().url()).searchParams.get('pageNum');
    if (number === '2') await secondPage;
    return respond(route, { dataList: number === '2' ? jobs.slice(2) : jobs.slice(0, 2), count: 13 });
  });
  await page.goto(`${baseURL}home?jobId=2`);
  await expect(page.getByRole('heading', { name: '归档文档', exact: true })).toBeVisible();
  await page.getByRole('button', { name: '下一页' }).click();
  await expect(page.getByText('正在加载同步任务', { exact: true })).toBeVisible();
  await expect(page.getByRole('heading', { name: '相册备份', exact: true })).toHaveCount(0);
  assert.equal(new URL(page.url()).searchParams.has('jobId'), false);
  releaseSecondPage();
  await expect(page.getByRole('heading', { name: '媒体同步', exact: true })).toBeVisible();
  await expect.poll(() => new URL(page.url()).searchParams.get('jobId')).toBe('3');
});

test('overview defers task views and drawers until they are opened', async (t) => {
  const page = await openPage(t, 1280);
  const requested = new Set();
  page.on('request', (request) => requested.add(new URL(request.url()).pathname));
  await page.route('**/svr/job?*', (route) => {
    const params = new URL(route.request().url()).searchParams;
    if (params.has('taskId')) return respond(route, { count: 0, dataList: [] });
    if (params.has('id')) return respond(route, { count: 1, dataList: [{ id: 42, status: 2, allNum: 1, successNum: 1 }] });
    return respond(route, { count: 1, dataList: [jobs[0]] });
  });
  await page.goto(`${baseURL}home`);
  await expect(page.getByRole('heading', { name: '相册备份', exact: true })).toBeVisible();
  const taskList = '/src/pages/Home/TaskList.tsx';
  const taskDetail = '/src/pages/Home/TaskDetail.tsx';
  const jobForm = '/src/pages/Home/JobFormDrawer.tsx';
  assert.ok([taskList, taskDetail, jobForm].every((path) => !requested.has(path)));

  await page.getByRole('tab', { name: '历史任务' }).click();
  await page.getByRole('button', { name: '详情', exact: true }).click();
  await expect(page.getByRole('dialog', { name: /任务详情.*42/ })).toBeVisible();
  assert.equal(requested.has(taskList) && requested.has(taskDetail), true);
  assert.equal(requested.has(jobForm), false);
  await page.keyboard.press('Escape');
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await page.getByRole('button', { name: '新建任务', exact: true }).click();
  await expect(page.getByRole('dialog', { name: '新建同步任务' })).toBeVisible();
  assert.equal(requested.has(jobForm), true);
});

test('both directory pickers share one root request and save independent paths after refresh', async (t) => {
  const page = await openPage(t, 390);
  await page.route('**/svr/alist', (route) => respond(route, engines));
  await page.route('**/svr/job?*', (route) => respond(route, { dataList: [], count: 0 }));
  const saves = [];
  await page.route('**/svr/job', (route) => {
    if (route.request().method() === 'POST') saves.push(route.request().postDataJSON());
    return respond(route, null);
  });
  const rootReads = [];
  await page.route('**/svr/alist?*', (route) => {
    const params = new URL(route.request().url()).searchParams;
    if (params.get('path') === '/') rootReads.push(params.get('alistId'));
    return respond(route, [{ name: '共享素材' }, { name: '归档目录' }]);
  });
  await page.goto(`${baseURL}home`);
  await page.getByRole('button', { name: '新建任务', exact: true }).click();
  const drawer = page.getByRole('dialog', { name: '新建同步任务' });
  for (const [field, path] of [['源目录', '共享素材'], ['目标目录', '归档目录']]) {
    await drawer.getByRole('button', { name: field, exact: true }).click();
    await page.getByRole('button', { name: '展开 /', exact: true }).click();
    await page.getByRole('button', { name: path, exact: true }).click();
    await page.keyboard.press('Escape');
  }
  assert.deepEqual(rootReads, ['1']);

  await drawer.getByRole('button', { name: '源目录', exact: true }).click();
  const refresh = page.getByRole('button', { name: '刷新目录', exact: true });
  await refresh.click();
  await expect.poll(() => rootReads.length).toBe(2);
  await expect(refresh).toBeEnabled();
  assert.deepEqual(rootReads, ['1', '1']);
  await page.keyboard.press('Escape');

  await drawer.getByRole('button', { name: '保存任务配置', exact: true }).click();
  await expect.poll(() => saves.length).toBe(1);
  assert.deepEqual([saves[0].alistId, saves[0].srcPath, saves[0].dstPath], [1, ['/共享素材'], ['/归档目录']]);
});

test('switching the engine and closing the drawer abort obsolete directory reads', async (t) => {
  const page = await openPage(t, 1280);
  await page.route('**/svr/alist', (route) => respond(route, engines));
  await page.route('**/svr/job?*', (route) => respond(route, { dataList: [jobs[0]], count: 1 }));

  let releaseDelayedReads;
  const delayedReads = new Promise((resolve) => { releaseDelayedReads = resolve; });
  t.after(() => releaseDelayedReads());
  const initialRoot = (request) => {
    const url = new URL(request.url());
    return url.pathname === '/svr/alist' && url.searchParams.get('alistId') === '1' && url.searchParams.get('path') === '/';
  };
  const expansion = (request) => new URL(request.url()).searchParams.get('path') === '/新引擎目录';
  await page.route('**/svr/alist?*', async (route) => {
    if (initialRoot(route.request()) || expansion(route.request())) {
      await delayedReads;
      await respond(route, [{ name: '过期目录' }]).catch(() => {});
      return;
    }
    return respond(route, [{ name: '新引擎目录' }]);
  });

  await page.goto(`${baseURL}home`);
  const firstStarted = page.waitForRequest(initialRoot);
  await page.getByRole('button', { name: '编辑', exact: true }).click();
  await firstStarted;
  const drawer = page.getByRole('dialog', { name: '编辑同步任务' });
  await expect(drawer.getByRole('button', { name: '移除 /source', exact: true })).toBeVisible();
  const firstCancelled = page.waitForEvent('requestfailed', { predicate: initialRoot, timeout: 5000 });
  await drawer.getByRole('combobox', { name: '存储引擎' }).selectOption('2');
  assert.equal((await firstCancelled).failure()?.errorText, 'net::ERR_ABORTED');

  await expect(drawer.getByRole('button', { name: '移除 /source', exact: true })).toHaveCount(0);
  await drawer.getByRole('button', { name: '源目录', exact: true }).click();
  await page.getByRole('button', { name: '展开 /', exact: true }).click();
  await expect(page.getByRole('button', { name: '新引擎目录', exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'source', exact: true })).toHaveCount(0);
  const childStarted = page.waitForRequest(expansion);
  await page.getByRole('button', { name: '展开 新引擎目录', exact: true }).click();
  await childStarted;
  const childCancelled = page.waitForEvent('requestfailed', { predicate: expansion, timeout: 5000 });
  await page.keyboard.press('Escape');
  await page.keyboard.press('Escape');
  assert.equal((await childCancelled).failure()?.errorText, 'net::ERR_ABORTED');
  await expect(drawer).toHaveCount(0);
  releaseDelayedReads();
});

for (const width of [320, 768, 1440]) test(`file detail keeps paths and status usable at ${width}px`, async (t) => {
  const page = await openPage(t, width);
  if (width === 320) await page.addInitScript(() => localStorage.setItem('opensync_theme', 'dark'));
  const filename = '家庭相册-' + 'OriginalArchive'.repeat(8) + '.mkv';
  const source = `/source/${'photos/'.repeat(15)}${filename}`;
  const target = `/target/${'archives/'.repeat(15)}${filename}`;
  await page.route('**/svr/job?*', (route) => respond(route, { count: 2, dataList: [
    { id: 41, fileName: filename, srcPath: source, dstPath: target, status: 7, errMsg: '目标存储已满', fileSize: 1024, type: 0, createTime: 1700000000 },
    { id: 42, fileName: '进度中的备份.mkv', srcPath: '/source/live.mkv', dstPath: '/target/live.mkv', status: 1, progress: 41, type: 2, createTime: 1700000000 },
  ] }));
  await page.goto(`${baseURL}home/task/detail?taskId=42`);
  const table = page.getByRole('table', { name: '文件明细' });
  const file = table.getByText(filename, { exact: true });
  const failedRow = table.getByRole('row').filter({ has: page.getByText(filename, { exact: true }) });
  await expect(failedRow.getByText('已失败', { exact: true })).toBeVisible();
  await expect(table.getByText('41%', { exact: true })).toBeVisible();
  await expect(table.getByRole('columnheader', { name: '来源目录', exact: true })).toHaveCount(1);

  if (width < 1280) {
    for (const path of [filename, source, target]) {
      const label = failedRow.getByText(path, { exact: true });
      assert.equal(await label.evaluate((element) => {
        const bounds = element.getBoundingClientRect();
        return bounds.left >= 0 && bounds.right <= innerWidth && element.scrollWidth <= element.clientWidth + 1;
      }), true);
    }
    assert.equal(await table.evaluate((element) => element.parentElement.scrollWidth <= element.parentElement.clientWidth + 1), true);
  } else {
    assert.equal(await failedRow.evaluate((element) => getComputedStyle(element).display), 'table-row');
    assert.equal(await file.evaluate((element) => getComputedStyle(element).textOverflow), 'ellipsis');
  }
  await failedRow.getByRole('button', { name: '查看失败原因' }).focus();
  await expect(page.getByRole('tooltip')).toHaveText('目标存储已满');
  if (process.env.OPENSYNC_SCREENSHOT_DIR) await page.screenshot({
    path: `${process.env.OPENSYNC_SCREENSHOT_DIR}/file-detail-${width}.png`, fullPage: true,
  });
});

test('file detail aborts replaced pages and distinguishes loading, error and empty filters', async (t) => {
  const page = await openPage(t, 1280);
  const releases = [];
  const delayedPage = new Promise((resolve) => releases.push(resolve));
  const delayedError = new Promise((resolve) => releases.push(resolve));
  t.after(() => releases.forEach((release) => release()));
  let retry = false;
  const requests = [];
  const isSecondPage = (request) => {
    const url = new URL(request.url());
    return url.pathname === '/svr/job' && url.searchParams.get('pageNum') === '2';
  };
  await page.route('**/svr/job?*', async (route) => {
    const params = new URL(route.request().url()).searchParams;
    requests.push(params);
    if (isSecondPage(route.request())) {
      await delayedPage;
      await respond(route, { count: 18, dataList: [{ id: 2, fileName: '过期结果.mkv', status: 2 }] }).catch(() => {});
      return;
    }
    if (params.get('keyword') === '新筛选') {
      if (!retry) {
        await delayedError;
        return route.fulfill({ status: 503, json: { code: 503, data: null, msg: 'Unavailable' } });
      }
      return respond(route, { count: 1, dataList: [{ id: 3, fileName: '新结果.mkv', status: 2 }] });
    }
    if (params.get('keyword')) return respond(route, { count: 0, dataList: [] });
    return respond(route, { count: 18, dataList: [{ id: 1, fileName: '旧结果.mkv', status: 2 }] });
  });

  await page.goto(`${baseURL}home/task/detail?taskId=42`);
  await expect(page.getByText('旧结果.mkv', { exact: true })).toBeVisible();
  const pageStarted = page.waitForRequest(isSecondPage);
  await page.getByRole('button', { name: '下一页' }).click();
  await pageStarted;
  const loading = page.getByRole('status').filter({ hasText: '正在加载文件明细' });
  await expect(loading).toBeVisible();
  await expect(page.getByText('旧结果.mkv', { exact: true })).toHaveCount(0);
  await expect(page.getByRole('button', { name: '上一页' })).toBeEnabled();

  const cancelled = page.waitForEvent('requestfailed', { predicate: isSecondPage });
  const search = page.getByRole('textbox', { name: '文件 / 路径 / 错误' });
  await search.fill('新筛选');
  await search.press('Enter');
  assert.equal((await cancelled).failure()?.errorText, 'net::ERR_ABORTED');
  await expect(loading).toBeVisible();
  await expect.poll(() => requests.at(-1).get('keyword')).toBe('新筛选');
  assert.equal(requests.at(-1).get('pageNum'), '1');
  releases.forEach((release) => release());
  await expect(page.getByText('文件详情加载失败', { exact: true })).toBeVisible();
  await expect(page.getByText('过期结果.mkv', { exact: true })).toHaveCount(0);

  retry = true;
  await page.getByRole('button', { name: '重试', exact: true }).click();
  await expect(page.getByText('新结果.mkv', { exact: true })).toBeVisible();
  await search.fill('无匹配');
  await search.press('Enter');
  await expect(page.getByText('没有符合筛选条件的文件明细', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: '重置', exact: true }).click();
  await expect(page.getByText('旧结果.mkv', { exact: true })).toBeVisible();
});

test('file detail keeps its rows visible while a silent refresh is pending', async (t) => {
  const page = await openPage(t, 1024);
  await page.clock.install();
  let releaseRefresh;
  const delayedRefresh = new Promise((resolve) => { releaseRefresh = resolve; });
  t.after(() => releaseRefresh());
  let delay = false;
  let refreshStarted = false;
  const result = { count: 1, dataList: [{ id: 1, fileName: '已完成的文件.mkv', status: 2 }] };
  await page.route('**/svr/job?*', async (route) => {
    if (delay) {
      refreshStarted = true;
      await delayedRefresh;
    }
    return respond(route, result);
  });
  await page.goto(`${baseURL}home/task/detail?taskId=42`);
  const file = page.getByRole('cell', { name: '已完成的文件.mkv', exact: true });
  await expect(file).toBeVisible();
  delay = true;
  await page.clock.fastForward(16000);
  await expect.poll(() => refreshStarted).toBe(true);
  await expect(file).toBeVisible();
  await expect(page.getByText('正在加载文件明细', { exact: true })).toHaveCount(0);
  const refreshed = page.waitForResponse((response) => new URL(response.url()).searchParams.get('taskId') === '42');
  releaseRefresh();
  await refreshed;
  await expect(file).toBeVisible();
});

test('editing only remark preserves minute-only hourly cron schedule', async (t) => {
  const page = await openPage(t, 1280);
  await page.route('**/svr/alist', (route) => respond(route, engines));
  await page.route('**/svr/alist?*', (route) => respond(route, []));
  const hourlyJob = {
    id: 1, enable: 1, remark: '每小时备份', srcPath: '/source', dstPath: '/backup', alistId: 1,
    useCacheT: false, useCacheS: false, method: 0, interval: 1440, isCron: 1,
    second: '', minute: '30', hour: '', day: '', month: '', day_of_week: '',
  };
  await page.route('**/svr/job?*', (route) => respond(route, { dataList: [hourlyJob], count: 1 }));
  const saves = [];
  await page.route('**/svr/job', (route) => {
    if (route.request().method() === 'POST') saves.push(route.request().postDataJSON());
    return respond(route, null);
  });

  await page.goto(`${baseURL}home`);
  await expect(page.getByRole('heading', { name: '每小时备份', exact: true })).toBeVisible();
  await page.getByRole('button', { name: '编辑', exact: true }).click();
  const drawer = page.getByRole('dialog', { name: '编辑同步任务' });
  await drawer.getByRole('textbox', { name: '任务备注' }).fill('每小时备份-已改备注');
  await drawer.getByRole('button', { name: '保存任务配置', exact: true }).click();
  await expect.poll(() => saves.length).toBe(1);
  assert.equal(saves[0].remark, '每小时备份-已改备注');
  assert.equal(saves[0].minute, '30');
  assert.equal(saves[0].hour, '*');
  assert.equal(saves[0].second, '0');
});

test('saved jobId link locates job on later page and never substitutes another job when missing', async (t) => {
  const page = await openPage(t, 1280);
  await page.route('**/svr/job?*', (route) => {
    const params = new URL(route.request().url()).searchParams;
    const locateJobId = params.get('locateJobId');
    if (locateJobId === '3') {
      return respond(route, { dataList: [jobs[2]], count: 13, pageNum: 2, locatedJobFound: true });
    }
    if (locateJobId === '999') {
      return respond(route, { dataList: jobs.slice(0, 2), count: 13, pageNum: 1, locatedJobFound: false });
    }
    const pageNum = params.get('pageNum');
    return respond(route, { dataList: pageNum === '2' ? [jobs[2]] : jobs.slice(0, 2), count: 13 });
  });

  // 1. Job 3 lives on page 2: opening /home?jobId=3 must move to page 2 and select job 3.
  await page.goto(`${baseURL}home?jobId=3`);
  await expect(page.getByRole('heading', { name: '媒体同步', exact: true })).toBeVisible();
  await expect(page.getByRole('heading', { name: '相册备份', exact: true })).toHaveCount(0);
  await expect.poll(() => new URL(page.url()).searchParams.get('jobId')).toBe('3');
  await expect.poll(() => new URL(page.url()).searchParams.get('page')).toBe('2');

  // 2. Non-existent jobId=999 must show missing state without selecting job 1 or rewriting jobId.
  await page.goto(`${baseURL}home?jobId=999`);
  await expect(page.getByText('未找到同步任务 #999', { exact: true })).toBeVisible();
  await expect(page.getByRole('heading', { name: '相册备份', exact: true })).toHaveCount(0);
  assert.equal(new URL(page.url()).searchParams.get('jobId'), '999');
});
