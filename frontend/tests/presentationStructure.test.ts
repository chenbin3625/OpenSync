import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import test from 'node:test';

const appSource = readFileSync(new URL('../src/App.tsx', import.meta.url), 'utf8');
const routerSource = readFileSync(new URL('../src/router/index.tsx', import.meta.url), 'utf8');
const requestSource = readFileSync(new URL('../src/api/request.ts', import.meta.url), 'utf8');
const homeSidebarSource = readFileSync(new URL('../src/pages/Home/HomeSidebar.tsx', import.meta.url), 'utf8');
const homeOverviewSource = readFileSync(new URL('../src/pages/Home/HomeOverview.tsx', import.meta.url), 'utf8');
const taskListSource = readFileSync(new URL('../src/pages/Home/TaskList.tsx', import.meta.url), 'utf8');
const engineSource = readFileSync(new URL('../src/pages/Engine/index.tsx', import.meta.url), 'utf8');
const notifySource = readFileSync(new URL('../src/pages/Notify/index.tsx', import.meta.url), 'utf8');
const settingSource = readFileSync(new URL('../src/pages/Setting/index.tsx', import.meta.url), 'utf8');
const loginSource = readFileSync(new URL('../src/pages/Login/index.tsx', import.meta.url), 'utf8');
const cssSource = readFileSync(new URL('../src/index.css', import.meta.url), 'utf8');
const homeCssSource = readFileSync(new URL('../src/pages/Home/Home.css', import.meta.url), 'utf8');
const loginCssSource = readFileSync(new URL('../src/pages/Login/Login.css', import.meta.url), 'utf8');

test('application defines shared presentation theme tokens', () => {
  assert.match(appSource, /colorPrimary:\s*isDark\s*\?\s*'#2dd4bf'\s*:\s*'#0f766e'/);
  assert.match(cssSource, /\.ops-page-surface/);
  assert.match(cssSource, /\.ops-section-title/);
});

test('login page inherits the active theme and uses shared color tokens', () => {
  assert.doesNotMatch(loginSource, /theme\.defaultAlgorithm|ConfigProvider/);
  assert.match(loginCssSource, /background:\s*var\(--ant-color-bg-layout\)/);
  assert.match(loginCssSource, /background:\s*var\(--ant-color-bg-container\)/);
  assert.match(loginCssSource, /border:\s*1px solid var\(--ant-color-border-secondary\)/);
});

test('home dashboard exposes scannable task workspace sections', () => {
  assert.match(homeSidebarSource, /sync-sidebar-job/);
  assert.match(homeOverviewSource, /sync-overview-hero/);
  assert.match(homeOverviewSource, /sync-info-grid/);
  assert.match(homeCssSource, /grid-template-rows:\s*minmax\(calc\(100vh - 90px\), auto\)/);
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
  assert.match(cssSource, /\.ops-resource-grid\s*>\s*\.ops-resource-card:only-child\s*{[^}]*max-width:\s*420px/s);
});

test('authenticated application shell is lazy loaded outside the login route', () => {
  assert.match(routerSource, /const Layout = lazy\(\(\) => import\('\.\.\/components\/Layout'\)\)/);
  assert.doesNotMatch(routerSource, /import Layout from '\.\.\/components\/Layout'/);
});

test('login route redirects already authenticated users', () => {
  assert.match(routerSource, /function ReverseAuthGuard/);
  assert.match(routerSource, /if \(userInfo\) \{\s+return <Navigate to="\/home" replace \/>;\s+\}/s);
  assert.match(routerSource, /path="\/login" element=\{<ReverseAuthGuard><Login \/><\/ReverseAuthGuard>\}/);
});

test('application routes use browser history without URL hashes', () => {
  assert.match(routerSource, /BrowserRouter/);
  assert.doesNotMatch(routerSource, /HashRouter/);
  assert.match(requestSource, /window\.location\.replace\('\/login'\)/);
  assert.doesNotMatch(requestSource, /window\.location\.hash|#\/login/);
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

test('home dashboard lazy-loads heavy task panes and skips sticky-header blur', () => {
  const homeSource = readFileSync(new URL('../src/pages/Home/index.tsx', import.meta.url), 'utf8');
  const viteSource = readFileSync(new URL('../vite.config.ts', import.meta.url), 'utf8');
  const ellipsisSource = readFileSync(new URL('../src/pages/Home/components/EllipsisText.tsx', import.meta.url), 'utf8');
  const historyPanelSource = readFileSync(new URL('../src/pages/Home/TaskHistoryPanel.tsx', import.meta.url), 'utf8');
  const realtimeTaskHookSource = readFileSync(new URL('../src/pages/Home/useRealtimeTask.ts', import.meta.url), 'utf8');
  assert.match(homeSource, /const TaskList = lazy\(/);
  assert.match(homeSource, /const JobFormDrawer = lazy\(/);
  assert.match(homeSource, /destroyInactiveTabPane/);
  assert.doesNotMatch(cssSource, /backdrop-filter/);
  assert.doesNotMatch(appSource, /antd\/locale\/zh_CN/);
  assert.match(appSource, /from '\.\/locales\/zhCN'/);
  assert.match(viteSource, /return 'antd-vendor'/);
  assert.match(viteSource, /function isAntdShell/);
  assert.match(viteSource, /antdShellModules/);
  assert.match(viteSource, /@rc-component\/table/);
  assert.match(viteSource, /@rc-component\/picker/);
  assert.match(viteSource, /@ant-design\/icons/);
  assert.doesNotMatch(viteSource, /return 'antd-picker'|return 'antd-table'|return 'antd-icons'/);
  assert.doesNotMatch(taskListSource, /DatePicker|antd\/es\/date-picker/);
  assert.doesNotMatch(taskListSource, /from ['"]antd['"]/);
  assert.doesNotMatch(engineSource, /from ['"]antd['"]/);
  assert.doesNotMatch(notifySource, /from ['"]antd['"]/);
  assert.doesNotMatch(settingSource, /from ['"]antd['"]/);
  assert.match(taskListSource, /const TaskHistoryPanel = lazy\(\(\) => import\('\.\/TaskHistoryPanel'\)\)/);
  assert.match(historyPanelSource, /antd\/es\/date-picker\/locale\/zh_CN/);
  assert.match(historyPanelSource, /DatePicker\.RangePicker/);
  assert.match(taskListSource, /task-progress-meter/);
  assert.match(taskListSource, /pollIntervalForActiveWork\(historyHasActiveTask\(list\)/);
  assert.match(taskListSource, /function LiveDuration/);
  assert.match(realtimeTaskHookSource, /mergeCurrentTaskData/);
  assert.match(realtimeTaskHookSource, /readSSEStream/);
  assert.match(realtimeTaskHookSource, /canUseFetchSSE/);
  assert.match(homeSource, /updateHomeRouteState\(\{ tab: 'realtime', jobId: id \}\);\s+try \{\s+await jobPut/s);
  assert.match(homeCssSource, /content-visibility:\s*auto/);
  assert.match(ellipsisSource, /ResizeObserver/);
  assert.match(homeSidebarSource, /const menuItems = useMemo/);
});
