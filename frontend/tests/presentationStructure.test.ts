import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import test from 'node:test';

const appSource = readFileSync(new URL('../src/App.tsx', import.meta.url), 'utf8');
const routerSource = readFileSync(new URL('../src/router/index.tsx', import.meta.url), 'utf8');
const homeSource = readFileSync(new URL('../src/pages/Home/index.tsx', import.meta.url), 'utf8');
const taskListSource = readFileSync(new URL('../src/pages/Home/TaskList.tsx', import.meta.url), 'utf8');
const engineSource = readFileSync(new URL('../src/pages/Engine/index.tsx', import.meta.url), 'utf8');
const notifySource = readFileSync(new URL('../src/pages/Notify/index.tsx', import.meta.url), 'utf8');
const settingSource = readFileSync(new URL('../src/pages/Setting/index.tsx', import.meta.url), 'utf8');
const cssSource = readFileSync(new URL('../src/index.css', import.meta.url), 'utf8');

test('application defines shared presentation theme tokens', () => {
  assert.match(appSource, /colorPrimary:\s*'#0f766e'/);
  assert.match(cssSource, /\.ops-page-surface/);
  assert.match(cssSource, /\.ops-section-title/);
});

test('home dashboard exposes scannable task workspace sections', () => {
  assert.match(homeSource, /sync-sidebar-job/);
  assert.match(homeSource, /sync-overview-hero/);
  assert.match(homeSource, /sync-info-grid/);
});

test('task execution views use compact operational surfaces', () => {
  assert.match(taskListSource, /task-progress-hero/);
  assert.match(taskListSource, /task-history-panel/);
});

test('realtime task refresh logic is split into local hooks', () => {
  const realtimeTaskHook = new URL('../src/pages/Home/useRealtimeTask.ts', import.meta.url);
  const realtimeTaskItemsHook = new URL('../src/pages/Home/useRealtimeTaskItems.ts', import.meta.url);

  assert.equal(existsSync(realtimeTaskHook), true);
  assert.equal(existsSync(realtimeTaskItemsHook), true);

  const realtimeTaskHookSource = readFileSync(realtimeTaskHook, 'utf8');
  const realtimeTaskItemsHookSource = readFileSync(realtimeTaskItemsHook, 'utf8');

  assert.match(realtimeTaskHookSource, /export function useRealtimeTask/);
  assert.match(realtimeTaskHookSource, /jobGetTaskCurrent/);
  assert.match(realtimeTaskItemsHookSource, /export function useRealtimeTaskItems/);
  assert.match(realtimeTaskItemsHookSource, /normalizeTaskItemPage/);
  assert.match(taskListSource, /useRealtimeTask/);
  assert.match(taskListSource, /useRealtimeTaskItems/);
});

test('configuration pages share the same resource page shell', () => {
  assert.match(engineSource, /ops-resource-page/);
  assert.match(notifySource, /ops-resource-page/);
  assert.match(settingSource, /ops-resource-page/);
});

test('authenticated application shell is lazy loaded outside the login route', () => {
  assert.match(routerSource, /const Layout = lazy\(\(\) => import\('\.\.\/components\/Layout'\)\)/);
  assert.doesNotMatch(routerSource, /import Layout from '\.\.\/components\/Layout'/);
});

test('resource page header and body use separated layout primitives', () => {
  const headerRule = cssSource.match(/\.ops-page-header\s*{[^}]+}/)?.[0] || '';
  assert.doesNotMatch(headerRule, /margin-bottom/);
  assert.match(cssSource, /\.ops-page-main/);
  assert.match(cssSource, /\.ops-page-panel/);
  assert.match(engineSource, /ops-page-main/);
  assert.match(notifySource, /ops-page-main/);
  assert.match(settingSource, /ops-page-main/);
  assert.match(settingSource, /ops-settings-panel/);
});
