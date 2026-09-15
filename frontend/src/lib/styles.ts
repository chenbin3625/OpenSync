/**
 * 共享样式配方（design tokens 的 class 层）。
 *
 * 页面里凡是「同一语义角色」的排版、卡片、表格、控件尺寸，都从这里取值，
 * 避免同一角色在不同文件里出现 text-xs / text-[11px] / text-sm 这类漂移。
 *
 * 设计约定：
 * - 字号只用 2xs / xs / sm / base / lg / xl，不再出现 text-[10px]、text-[11px]
 * - 卡片圆角 rounded-xl，内嵌面板 rounded-lg，控件 rounded-md
 * - 边框统一 slate-200（内嵌面板 slate-100），不再使用 /60 /80 /90 透明度变体
 * - 控件高度三档：default h-9、compact h-8、dense h-7
 * - 危险色统一 rose，成功色 emerald，警告色 amber，品牌色 teal
 */

/** 排版：同一语义角色只有一种写法 */
export const text = {
  /** 页面主标题（每个页面/抽屉的唯一 h1） */
  pageTitle: 'text-xl font-bold tracking-tight text-slate-900',
  /** 页面副标题 / 描述 */
  pageSubtitle: 'text-sm text-slate-500',
  /** 卡片与区块标题 */
  sectionTitle: 'text-sm font-semibold text-slate-900',
  /** 表单分组小标题（全大写） */
  groupLabel: 'text-2xs font-semibold uppercase tracking-wider text-slate-400',
  /** 正文 */
  body: 'text-sm text-slate-700',
  /** 次要说明文字 */
  muted: 'text-xs text-slate-500',
  /** 更弱的辅助文字（键名、单位、时间戳） */
  subtle: 'text-xs text-slate-400',
  /** 表单标签 */
  label: 'text-xs font-medium text-slate-700',
  /** 表单字段下方的提示语 */
  hint: 'text-2xs text-slate-400',
  /** 表格列头（配合 tableHead 使用） */
  tableHead: 'text-xs font-medium uppercase tracking-wider text-slate-600',
  /** 表格主单元格 */
  cell: 'text-xs text-slate-700',
  /** 表格强调单元格（文件名等） */
  cellStrong: 'text-xs font-medium text-slate-800',
  /** 数值 / 路径等等宽文本 */
  mono: 'font-mono text-xs text-slate-700',
  /** 统计数字 */
  stat: 'text-sm font-bold font-mono tracking-tight text-slate-800',
} as const;

/** 表面：卡片、内嵌面板、提示条 */
export const surface = {
  /** 标准卡片 */
  card: 'bg-white rounded-xl border border-slate-200 shadow-xs',
  /** 卡片内边距（响应式，窄屏收紧） */
  cardPadding: 'p-4 sm:p-5',
  /** 可点击卡片的悬浮反馈 */
  cardHover: 'transition-shadow hover:shadow-md',
  /** 内嵌面板（卡片内部的次级容器） */
  inset: 'bg-slate-50 rounded-lg border border-slate-100 p-3',
  /** 空状态 / 错误态占位容器 */
  placeholder: 'bg-white rounded-xl border border-dashed border-slate-200 py-16 text-center',
  /** 品牌色图标底座 */
  iconTile: 'rounded-lg bg-teal-50 text-teal-700 flex items-center justify-center shrink-0',
  /** 区块分隔线 */
  divider: 'border-slate-100',
} as const;

/** 表格：列表页与详情页共用同一套外壳 */
export const table = {
  wrapper: 'overflow-x-auto border border-slate-200 rounded-lg',
  root: 'w-full text-left text-xs divide-y divide-slate-200',
  head: 'bg-slate-50 text-slate-600 uppercase font-medium tracking-wider',
  headCell: 'px-3 py-2.5',
  body: 'divide-y divide-slate-100 bg-white',
  /** 单元格纵向内距统一为 py-2.5 */
  cell: 'px-3 py-2.5',
  row: 'transition-colors hover:bg-slate-50/70',
} as const;

/** 布局：页面外壳与常用间距 */
export const layout = {
  /** 页面根容器（Layout 的 main 已提供 max-w 与 padding） */
  page: 'grid content-start gap-4 min-w-0',
  /** 页头：标题区 + 操作区，窄屏堆叠 */
  pageHeader:
    'flex flex-col gap-4 pb-2 border-b border-slate-200 sm:flex-row sm:items-center sm:justify-between',
  /** 页头操作区 */
  pageHeaderActions: 'flex shrink-0 items-center justify-end gap-2',
  /** 卡片栅格 */
  cardGrid: 'grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 items-stretch',
  /** 表单栅格 */
  formGrid: 'grid grid-cols-1 sm:grid-cols-2 gap-4',
  /** 表单字段行 */
  field: 'space-y-1.5',
  /** 筛选条 */
  filterBar: 'flex flex-wrap items-center gap-2',
} as const;

/** 控件尺寸：只有三档，避免 h-7/h-8/h-9 混用 */
export const control = {
  /** 默认表单控件（Input / Select 原生高度） */
  default: 'h-9',
  /** 紧凑控件：筛选条、次级操作 */
  compact: 'h-8 text-xs',
  /** 密集控件：表格行内操作、分页器 */
  dense: 'h-7 text-xs',
} as const;

/** 图标尺寸：只有三档 */
export const icon = {
  /** 行内 / 小按钮内的图标 */
  sm: 'h-3.5 w-3.5',
  /** 默认按钮、页头、卡片标题图标 */
  md: 'h-4 w-4',
  /** 弹窗标题图标 */
  lg: 'h-5 w-5',
  /** 空状态大图标 */
  placeholder: 'h-10 w-10 text-slate-300',
} as const;
