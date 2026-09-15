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
const stylesSource = readFileSync(new URL('../src/lib/styles.ts', import.meta.url), 'utf8');

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
  assert.match(taskListSource, /space-y-4/);
  // 卡片外壳与表格样式取自共享配方，不再内联 rounded-xl / px-3 py-2.5
  assert.match(taskListSource, /surface\.card/);
  assert.match(taskListSource, /table\.wrapper/);
  assert.match(stylesSource, /card: 'bg-white rounded-xl/);
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
  // 三个配置页不再各自手写栅格外壳，统一引用 lib/styles 的 layout.page
  for (const source of [engineSource, notifySource, settingSource]) {
    assert.match(source, /className=\{layout\.page\}/);
    assert.doesNotMatch(source, /grid content-start gap-4/);
  }
  // 卡片栅格同样收敛到 layout.cardGrid
  assert.match(stylesSource, /cardGrid:\s*'grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3/);
  assert.match(engineSource, /className=\{layout\.cardGrid\}/);
  assert.match(notifySource, /className=\{layout\.cardGrid\}/);
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
  // 页头收敛为共享 PageHeader 组件：页头用 border-b + pb-* 收边，不依赖 margin 撑开间距
  assert.match(stylesSource, /pageHeader:[\s\S]*?pb-2 border-b border-slate-200/);
  for (const source of [engineSource, notifySource, settingSource]) {
    assert.match(source, /<PageHeader\b/);
    assert.match(source, /className="min-w-0/);
  }
  assert.match(settingSource, /max-w-3xl/);
});

test('shared design tokens replace ad-hoc typography and border values', () => {
  // 令牌层：字号与语义色集中在 index.css 的 @theme
  assert.match(cssSource, /@theme\s*\{/);
  assert.match(cssSource, /--text-2xs:/);
  assert.match(cssSource, /--color-brand:/);
  // 页面不再出现绕过字号阶梯的任意值，也不再出现 slate-200 的透明度变体
  const pages = [engineSource, notifySource, settingSource, loginSource];
  for (const source of pages) {
    assert.doesNotMatch(source, /text-\[\d+px\]/);
    assert.doesNotMatch(source, /border-slate-200\/(60|80|90)/);
  }
});
