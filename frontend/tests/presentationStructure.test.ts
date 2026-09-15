import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import test from 'node:test';

const appSource = readFileSync(new URL('../src/App.tsx', import.meta.url), 'utf8');
const routerSource = readFileSync(new URL('../src/router/index.tsx', import.meta.url), 'utf8');
const requestSource = readFileSync(new URL('../src/api/request.ts', import.meta.url), 'utf8');
const homeSource = readFileSync(new URL('../src/pages/Home/index.tsx', import.meta.url), 'utf8');
const homeSidebarSource = readFileSync(new URL('../src/pages/Home/HomeSidebar.tsx', import.meta.url), 'utf8');
const homeOverviewSource = readFileSync(new URL('../src/pages/Home/HomeOverview.tsx', import.meta.url), 'utf8');
const taskListSource = readFileSync(new URL('../src/pages/Home/TaskList.tsx', import.meta.url), 'utf8');
const engineSource = readFileSync(new URL('../src/pages/Engine/index.tsx', import.meta.url), 'utf8');
const notifySource = readFileSync(new URL('../src/pages/Notify/index.tsx', import.meta.url), 'utf8');
const settingSource = readFileSync(new URL('../src/pages/Setting/index.tsx', import.meta.url), 'utf8');
const loginSource = readFileSync(new URL('../src/pages/Login/index.tsx', import.meta.url), 'utf8');
const cssSource = readFileSync(new URL('../src/index.css', import.meta.url), 'utf8');

test('application defines shared presentation theme tokens', () => {
  assert.match(appSource, /colorPrimary:\s*'#0f766e'/);
  assert.doesNotMatch(appSource, /isDark/);
  // 全局样式表只负责 Tailwind 入口与根级规则，组件样式一律走 utility class
  assert.match(cssSource, /@import "tailwindcss";/);
  assert.doesNotMatch(cssSource, /--ant-color-/);
  assert.doesNotMatch(cssSource, /\.ant-/);
});

test('presentation layer keeps no legacy component stylesheets', () => {
  // 遗留 CSS 未分层，会整体压过 Tailwind 的 @layer utilities，必须保持删除状态
  assert.equal(existsSync(new URL('../src/pages/Home/Home.css', import.meta.url)), false);
  assert.equal(existsSync(new URL('../src/pages/Login/Login.css', import.meta.url)), false);
  for (const source of [homeSidebarSource, homeOverviewSource, taskListSource, loginSource]) {
    assert.doesNotMatch(source, /\.css'/);
    assert.doesNotMatch(source, /className="[^"]*\b(sync|task|ops|app|login)-[a-z-]+/);
  }
});

test('login page inherits the active theme and uses shared color tokens', () => {
  assert.doesNotMatch(loginSource, /theme\.defaultAlgorithm|ConfigProvider/);
  assert.match(loginSource, /min-h-screen/);
  assert.match(loginSource, /bg-gradient-to-br/);
  assert.match(loginSource, /max-w-md/);
});

test('home dashboard exposes scannable task workspace sections', () => {
  assert.match(homeSidebarSource, /overflow-y-auto/);
  assert.match(homeOverviewSource, /md:flex-row/);
  assert.match(homeOverviewSource, /md:grid-cols-2/);
  // 侧边栏 + 内容区两栏栅格，窄屏塌陷为单栏
  assert.match(homeSource, /md:grid-cols-\[300px_minmax\(0,1fr\)\]/);
});

test('task execution views use compact operational surfaces', () => {
  assert.match(taskListSource, /divide-y|space-y-4/);
  assert.match(taskListSource, /rounded-xl/);
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
  // 三个配置页共用同一套栅格外壳（utility class 形式）
  const shell = /className="grid content-start gap-4 min-w-0 min-h-\[calc\(100vh-90px\)\]"/;
  assert.match(engineSource, shell);
  assert.match(notifySource, shell);
  assert.match(settingSource, shell);
  assert.match(engineSource, /md:grid-cols-2 lg:grid-cols-3/);
  assert.match(notifySource, /md:grid-cols-2 lg:grid-cols-3/);
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
  // 页头与主体各自独立：页头用 border-b + pb-* 收边，不依赖 margin 撑开间距
  for (const source of [engineSource, notifySource, settingSource]) {
    assert.match(source, /pb-2 border-b border-slate-200\/80/);
    assert.match(source, /className="min-w-0/);
  }
  assert.match(settingSource, /max-w-3xl/);
});
