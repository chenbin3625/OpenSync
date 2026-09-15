import assert from 'node:assert/strict';
import { existsSync, readFileSync, readdirSync } from 'node:fs';
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
const layoutSource = readFileSync(new URL('../src/components/Layout/index.tsx', import.meta.url), 'utf8');
const cssSource = readFileSync(new URL('../src/index.css', import.meta.url), 'utf8');
const stylesSource = readFileSync(new URL('../src/lib/styles.ts', import.meta.url), 'utf8');
const badgeSource = readFileSync(new URL('../src/components/ui/badge.tsx', import.meta.url), 'utf8');
const realtimeHeroSource = readFileSync(
  new URL('../src/pages/Home/components/TaskRealtimeHero.tsx', import.meta.url),
  'utf8'
);
const realtimeRowsSource = readFileSync(
  new URL('../src/pages/Home/components/TaskRealtimeRows.tsx', import.meta.url),
  'utf8'
);
const taskDetailSource = readFileSync(new URL('../src/pages/Home/TaskDetail.tsx', import.meta.url), 'utf8');
const pathTreeSelectSource = readFileSync(
  new URL('../src/components/ui/path-tree-select.tsx', import.meta.url),
  'utf8'
);
const selectSource = readFileSync(new URL('../src/components/ui/select.tsx', import.meta.url), 'utf8');
const tooltipSource = readFileSync(new URL('../src/components/ui/tooltip.tsx', import.meta.url), 'utf8');
const sheetSource = readFileSync(new URL('../src/components/ui/sheet.tsx', import.meta.url), 'utf8');
const dialogSource = readFileSync(new URL('../src/components/ui/dialog.tsx', import.meta.url), 'utf8');
const switchSource = readFileSync(new URL('../src/components/ui/switch.tsx', import.meta.url), 'utf8');
const statusBadgeSource = readFileSync(
  new URL('../src/components/common/StatusBadge.tsx', import.meta.url),
  'utf8'
);
const errorReasonSource = readFileSync(
  new URL('../src/components/common/ErrorReason.tsx', import.meta.url),
  'utf8'
);
const paginationSource = readFileSync(
  new URL('../src/components/common/Pagination.tsx', import.meta.url),
  'utf8'
);
const fieldSource = readFileSync(
  new URL('../src/components/common/Field.tsx', import.meta.url),
  'utf8'
);

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
  // 左右两栏均具备白色卡片底，保持视觉结构对称一致
  assert.match(homeSidebarSource, /surface\.card/);
  assert.match(homeSource, /<main className=\{cn\(surface\.card/);
});

test('app shell fills the available viewport without viewport-math overflow', () => {
  // 内容区宽度不再设固定上限，避免宽屏下左右大面积留白
  assert.doesNotMatch(layoutSource, /max-w-7xl/);
  // 桌面端外壳固定一屏高、滚动交给内容区，页面本身不再出现常驻滚动条
  assert.match(layoutSource, /md:h-dvh md:overflow-hidden/);
  assert.match(layoutSource, /flex-1 min-h-0 w-full p-4 sm:p-6 flex flex-col md:overflow-y-auto/);
  // 余高由 flex 向下传递，页面不再用 100vh 手算：
  // 手算值一旦大于实际可用空间（视口高 - 页头 - 内边距），就会常驻整页滚动条
  assert.doesNotMatch(homeSource, /100vh/);
  assert.doesNotMatch(homeSidebarSource, /100vh/);
  assert.match(homeSource, /md:grow/);
  assert.match(homeSidebarSource, /flex-1 min-h-\[240px\] md:min-h-0 overflow-y-auto/);
});

test('badges share one recipe and never wrap onto a second line', () => {
  // 形状、尺寸与色系收敛到 lib/styles 的 badge 配方
  assert.match(stylesSource, /export const badge = \{/);
  assert.match(stylesSource, /base: '[^']*whitespace-nowrap shrink-0[^']*'/);
  assert.match(stylesSource, /base: '[^']*rounded-full border px-2 py-0\.5 text-2xs[^']*'/);
  // 每个语义只有一种写法：浅底 + 同色描边 + 深色文字
  for (const tone of [
    'neutral: \'bg-slate-100 text-slate-700 border-slate-200\'',
    'brand: \'bg-teal-100 text-teal-800 border-teal-200\'',
    'success: \'bg-emerald-100 text-emerald-800 border-emerald-200\'',
    'warning: \'bg-amber-100 text-amber-800 border-amber-200\'',
    'danger: \'bg-rose-100 text-rose-800 border-rose-200\'',
    'info: \'bg-blue-100 text-blue-800 border-blue-200\'',
  ]) {
    assert.ok(stylesSource.includes(tone), `missing badge tone: ${tone}`);
  }
  // 不再混用实心风格（bg-*-600/700 + text-white）与 border-transparent 的互相覆盖
  assert.doesNotMatch(stylesSource, /bg-(teal|slate|rose|emerald|amber|blue)-[67]00 text-white/);

  // Badge 复用配方，且用 span 承载（徽标常嵌在段落 / StatusBadge 的 span 里）
  assert.match(badgeSource, /cva\(badge\.base/);
  assert.match(badgeSource, /return <span /);
  assert.match(badgeSource, /HTMLAttributes<HTMLSpanElement>/);
  assert.doesNotMatch(badgeSource, /border-transparent/);
  assert.doesNotMatch(badgeSource, /hover:bg-/);
  // 删除类型标签走统一的危险色，不再保留 destructive 这一别名
  assert.match(realtimeRowsSource, /task\.type === 1 \? 'error' : task\.type === 2 \? 'warning' : 'default'/);
  assert.match(taskDetailSource, /itemType === 1 \? 'error' : itemType === 2 \? 'warning' : 'default'/);

  // 调用点不再各自覆盖标签尺寸
  assert.match(
    homeSidebarSource,
    /<Badge variant=\{isEnabled \? 'success' : 'secondary'\}>\n\s+\{isEnabled \? '已启用' : '已暂停'\}\n\s+<\/Badge>/
  );
  assert.match(
    homeOverviewSource,
    /<Badge variant=\{isEnabled \? 'success' : 'secondary'\}>\n\s+\{isEnabled \? '已启用' : '已暂停'\}\n\s+<\/Badge>/
  );
  assert.match(realtimeRowsSource, /<Badge variant=\{typeVariant\}>/);
  assert.doesNotMatch(realtimeRowsSource, /text-2xs px-1\.5 py-0/);
  assert.doesNotMatch(homeOverviewSource, /variant="outline" className="bg-slate-50"/);

  // 手写胶囊改用共享配方
  assert.match(taskListSource, /badge\.base/);
  assert.match(taskListSource, /badge\.tone\.brand/);
  assert.doesNotMatch(taskListSource, /px-1\.5 py-0\.5 rounded-full text-2xs/);
  assert.match(pathTreeSelectSource, /cn\(badge\.base, badge\.tone\.neutral/);
  assert.doesNotMatch(pathTreeSelectSource, /rounded bg-slate-100 border border-slate-200 px-2/);

  // 标题行固定单行：标签不再被 flex-wrap 挤到第二行
  assert.doesNotMatch(homeOverviewSource, /flex items-center gap-2 flex-wrap/);
  assert.match(homeOverviewSource, /flex items-center gap-2 min-w-0/);
  assert.match(homeOverviewSource, /truncate min-w-0/);
  assert.doesNotMatch(realtimeHeroSource, /<h2 className=\{text\.sectionTitle\}>/);
  assert.match(realtimeHeroSource, /truncate min-w-0/);
});

test('compact selects keep their value on one line', () => {
  // 下拉控件的当前值必须单行：固定宽度下的中文选项会被折成「全部对 / 象」
  assert.match(selectSource, /text-slate-800 whitespace-nowrap/);
  // 每页条数与「全部对象」的固定宽度放不下 4 个中文字符加箭头，放宽一档
  assert.match(paginationSource, /SelectTrigger className=\{cn\(control\.dense, 'w-28'\)\}/);
  for (const source of [paginationSource, taskDetailSource, taskListSource]) {
    assert.doesNotMatch(source, /SelectTrigger className=\{cn\(control\.\w+, 'w-24'\)\}/);
  }
});

test('error reason hover cards stay readable and share one entry point', () => {
  // 卡片限高并可滚动：失败原因可能是服务端返回的十几行重试记录，
  // 不限高会顶出视口上方，overflow-hidden 则让人滚不到被切掉的开头
  assert.match(tooltipSource, /max-h-\[min\(60vh,20rem\)\] overflow-y-auto overscroll-contain/);
  assert.doesNotMatch(tooltipSource, /overflow-hidden/);
  // 贴边时留出余量，避免卡片被视口裁掉半个字
  assert.match(tooltipSource, /collisionPadding=\{8\}/);
  // 长文本的换行处理收敛到卡片层，调用点不再各自拼装
  assert.match(tooltipSource, /max-w-xs whitespace-pre-wrap break-words/);

  // 失败原因入口收敛为一个组件：必须是真按钮（可聚焦、可 Tab），
  // 不能用 span 冒充交互元素——那样只能鼠标悬浮，键盘用户永远看不到原因
  assert.match(errorReasonSource, /export function ErrorReason/);
  assert.match(errorReasonSource, /<button\b/);
  assert.match(errorReasonSource, /type="button"/);
  assert.doesNotMatch(errorReasonSource, /<span\s+aria-label/);
  assert.match(errorReasonSource, /aria-label="查看失败原因"/);
  assert.match(errorReasonSource, /badge\.base, badge\.tone\.danger/);
  // 原因以中文为主，等宽字体在中文下会退化成字宽不一的回退字体
  assert.doesNotMatch(errorReasonSource, /font-mono/);
  assert.doesNotMatch(realtimeRowsSource, /font-mono whitespace-pre-wrap/);

  // 任务列表 / 详情表格与实时明细共用同一个人口，不再各写一套 Tooltip
  assert.match(statusBadgeSource, /<ErrorReason errMsg=\{errMsg\} \/>/);
  assert.doesNotMatch(statusBadgeSource, /Tooltip title=\{errMsg\}/);
  assert.match(realtimeRowsSource, /<ErrorReason errMsg=\{task\.errMsg\} label="失败原因" \/>/);
  assert.doesNotMatch(realtimeRowsSource, /text-rose-600 cursor-pointer font-medium/);
  // 实时明细只在没有失败原因时才回落到时间戳。
  // 这里必须把整个三元表达式连同两个分支一起钉住：只断言
  // `activeTab === 7 && task.errMsg ? (` 是空转的——把三元拆成两个并列的条件渲染
  // （失败原因和时间戳同时出现）照样能匹配上。
  assert.match(
    realtimeRowsSource.replace(/\s+/g, ' '),
    /\{activeTab === 7 && task\.errMsg\?\.trim\(\) \? \( <ErrorReason[^)]*?\/> \) : \( activeTab !== 1 && \( <span className="text-slate-400 font-mono">/
  );
  // 判空要和 ErrorReason 内部一致（它把纯空白也当作没有原因），
  // 否则纯空白时走入口分支、组件返回 null，这一格既没有入口也没有时间戳
  assert.match(errorReasonSource, /const message = typeof errMsg === 'string' \? errMsg\.trim\(\) : ''/);
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
  assert.match(stylesSource, /pageHeader:[\s\S]*?pb-2 border-b border-line/);
  for (const source of [engineSource, notifySource, settingSource]) {
    assert.match(source, /<PageHeader\b/);
    assert.match(source, /className="min-w-0/);
  }
  assert.match(settingSource, /max-w-3xl/);
  // 表单字段后缀固定宽度且输入控件 flex-1，保证系统设置等网格表单输入条严格等宽对齐
  assert.match(fieldSource, /flex-1 min-w-0/);
  assert.match(fieldSource, /w-8 shrink-0 text-left whitespace-nowrap/);
});

test('structural borders, dividers and tints are derived from the theme color', () => {
  // 品牌色阶覆盖 Tailwind 默认荧光青绿，采用低饱和淡绿对齐原始设计
  assert.match(cssSource, /--color-teal-50:\s*#eef2f1/);
  assert.match(cssSource, /--color-teal-100:\s*#d9efeb/);
  assert.match(cssSource, /--color-teal-200:\s*#cfdeda/);
  // 骨架色（描边 / 分隔线 / 浅底）由主题色推导，换主色时只改这四行
  assert.match(cssSource, /--color-line: var\(--color-teal-200\)/);
  assert.match(cssSource, /--color-line-strong: var\(--color-teal-300\)/);
  assert.match(cssSource, /--color-line-soft: var\(--color-teal-100\)/);
  assert.match(cssSource, /--color-tint: var\(--color-teal-50\)/);
  // 页面底色、滚动条同样走主题色，不再是与主色无关的灰
  assert.match(cssSource, /background: var\(--color-tint\)/);
  assert.match(cssSource, /scrollbar-color: var\(--color-line-soft\) transparent/);

  // 共享配方不再写死灰色骨架
  assert.match(stylesSource, /card: 'bg-white rounded-xl border border-line/);
  assert.match(stylesSource, /inset: 'bg-tint rounded-lg border border-line-soft/);
  assert.match(stylesSource, /wrapper: 'overflow-x-auto border border-line rounded-lg'/);
  assert.match(stylesSource, /head: 'bg-tint text-slate-600/);
  assert.match(stylesSource, /row: 'transition-colors hover:bg-tint'/);

  // 页面与组件层不再绕过 token 手写灰色骨架（含共享配方自身）
  const chromePattern = /(border|divide|bg|ring|from|to)-slate-(?:50|100|200|300)\b/g;
  /**
   * 灰色豁免名单。这里保留灰不是「漏了主题化」，而是语义本身要求无色：
   * - neutral 徽标：同一行同时出现下面三个 class 即视为豁免，顺序无关
   *   （写死正序字符串匹配，一次重排就能绕过去）
   * - Switch 关闭态轨道：轨道一旦染上主题色，就会被读成「已开启」
   */
  const NEUTRAL_TONE = ['bg-slate-100', 'text-slate-700', 'border-slate-200'];
  const ALLOWED_GRAY_BY_FILE: Record<string, string[]> = {
    'components/ui/switch.tsx': ['bg-slate-200'],
  };
  const offenders: string[] = [];
  const walk = (dir: URL) => {
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      const child = new URL(`${entry.name}${entry.isDirectory() ? '/' : ''}`, dir);
      if (entry.isDirectory()) walk(child);
      else if (/\.tsx?$/.test(entry.name)) {
        const relativePath = child.pathname.split('/src/')[1];
        const allowed = ALLOWED_GRAY_BY_FILE[relativePath] ?? [];
        readFileSync(child, 'utf8')
          // 注释里举例的灰色 class 不是违规代码。块注释逐字符换成空格而不是删掉，
          // 换行数保持不变，下面的行号才是文件里真实的行号
          .replace(/\/\*[\s\S]*?\*\//g, (comment) => comment.replace(/[^\n]/g, ' '))
          .replace(/\/\/[^\n]*/g, '')
          .split('\n')
          .forEach((line, i) => {
            // text-slate-* 是文字色，深灰在浅底上比深青更易读，不参与主题化
            const isNeutralTone = NEUTRAL_TONE.every((tone) => line.includes(tone));
            if (isNeutralTone) return;
            const offending = (line.match(chromePattern) ?? []).filter(
              (cls) => !allowed.includes(cls)
            );
            if (offending.length > 0) {
              offenders.push(`${relativePath}:${i + 1} ${offending.join(' ')}`);
            }
          });
      }
    }
  };
  walk(new URL('../src/', import.meta.url));
  assert.deepEqual(
    offenders,
    [],
    `以下位置仍手写灰色骨架，请改用 line / line-strong / line-soft / tint：\n${offenders.join('\n')}`
  );

  // 例外名单必须对应真实存在的写法，否则名单迟早变成「谁都能往里塞」的后门。
  // 1. neutral 徽标的语义就是「无色」，染成主题色后会与 brand 只剩文字深浅之差
  assert.ok(
    NEUTRAL_TONE.every((tone) => stylesSource.includes(tone)),
    'neutral 徽标的豁免名单必须对应 lib/styles.ts 里真实存在的写法'
  );
  // 2. Switch 关闭态的轨道：轨道染上主题色会被读成「已开启」（开启才用品牌色）
  assert.match(switchSource, /data-\[state=unchecked\]:bg-slate-200/);
  assert.match(switchSource, /data-\[state=checked\]:bg-teal-700/);
});

test('dialog scrolls its body so the close button stays pinned', () => {
  // 关闭按钮是 absolute 定位在弹窗上的。弹窗一旦自己成为滚动容器
  // （调用点写 max-h-* + overflow-y-auto），X 会跟着长内容滚出可视区，
  // 用户只剩 Esc 一条退路——重置密码弹窗展开「找不到恢复密钥？」后就是这样
  assert.match(dialogSource, /flex max-h-\[85vh\] w-full max-w-lg/);
  // min-h-0 不能省：flex 子项的 min-height 默认是 auto，不归零会重新撑破外层限高
  assert.match(dialogSource, /flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto/);
  const scrollBodyIndex = dialogSource.indexOf('min-h-0 flex-1 flex-col gap-4 overflow-y-auto');
  const closeIndex = dialogSource.indexOf('<DialogPrimitive.Close');
  assert.ok(
    scrollBodyIndex > 0 && closeIndex > scrollBodyIndex,
    '关闭按钮必须在滚动区之外（作为滚动区之后的兄弟节点），否则会随内容滚走'
  );

  for (const source of [loginSource, notifySource, engineSource, settingSource]) {
    // 调用点不再把弹窗本身设成滚动容器
    assert.doesNotMatch(source, /<DialogContent[^>]*overflow-y-auto/);
    // DialogContent 不支持 title，写了会透传到 DOM，整块弹窗挂上一个原生气泡，
    // 和内部已有的 DialogTitle 重复（悬浮在弹窗任意位置都会弹出来）
    assert.doesNotMatch(source, /<DialogContent[^>]*\stitle=/);
  }
});

test('drawer edges are painted with the theme colour, not currentColor', () => {
  // border-l / border-b 这类只设宽度不带颜色，不显式给色时会退回 currentColor，
  // 在抽屉贴合页面的那条边上画出一条跟正文同色的深线
  assert.match(sheetSource, /border-line transition ease-in-out/);
  // 四个方向都靠这一条基类供色，不能只覆盖其中一边
  for (const side of ['border-t', 'border-b', 'border-l', 'border-r']) {
    assert.ok(sheetSource.includes(side), `sheet 缺少方向类 ${side}`);
  }
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
