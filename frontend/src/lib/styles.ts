/**
 * 共享样式配方（design tokens 的 class 层）。
 *
 * 页面里凡是「同一语义角色」的排版、卡片、表格、控件尺寸，都从这里取值，
 * 避免同一角色在不同文件里出现 text-xs / text-[11px] / text-sm 这类漂移。
 *
 * 设计约定：
 * - 字号只用 2xs / xs / sm / base / lg / xl，不再出现 text-[10px]、text-[11px]
 * - 卡片圆角 rounded-xl，内嵌面板 rounded-lg，控件 rounded-md
 * - 描边、分隔线、浅底、悬浮态一律取主题色推导的 line / line-strong / line-soft / tint
 *   （见 index.css 的 @theme），不直接写 slate-100/200/300，也不再使用 /60 /80 /90 透明度变体
 * - tint 与 line-soft 分工固定：tint 是页面底色，只能用于画布和不参与交互的内嵌面板；
 *   悬浮 / 选中 / 聚焦填充一律 line-soft，否则在画布上前后景同色（对比度 1.000）
 * - 选中态必须比悬浮态更强，否则「悬浮到别的项」比「当前项」还像被选中
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
  card: 'bg-white rounded-xl border border-line shadow-xs',
  /** 卡片内边距（响应式，窄屏收紧） */
  cardPadding: 'p-4 sm:p-5',
  /** 可点击卡片的悬浮反馈 */
  cardHover: 'transition-shadow hover:shadow-md',
  /** 内嵌面板（卡片内部的次级容器） */
  inset: 'bg-tint rounded-lg border border-line-soft p-3',
  /** 空状态 / 错误态占位容器 */
  placeholder: 'bg-white rounded-xl border border-dashed border-line py-16 text-center',
  /** 品牌色图标底座 */
  iconTile: 'rounded-lg bg-teal-50 text-teal-700 flex items-center justify-center shrink-0',
  /** 区块分隔线 */
  divider: 'border-line-soft',
} as const;

/**
 * 标签 / 徽标：状态、操作类型、计数等一切「小胶囊」共用同一形状与色系。
 *
 * 约定：
 * - 只有一种尺寸（text-2xs + px-2 py-0.5），调用点不再各自覆盖字号与内距
 * - 只有一种写法：浅底 + 同色描边 + 深色文字；不再混用实心（bg-*-600 text-white）
 *   与描边两种风格，也不再出现 border-transparent 与 border-*-200 互相覆盖
 * - whitespace-nowrap + shrink-0：中文标签的最小宽度只有一个字，
 *   在 table / flex 行里会被压成两行，必须禁止换行并禁止被压缩
 */
export const badge = {
  /** 形状与排版：圆角胶囊、单行、不收缩 */
  base: 'inline-flex items-center gap-1 whitespace-nowrap shrink-0 rounded-full border px-2 py-0.5 text-2xs font-medium transition-colors',
  /** 语义色调：每个语义只有一种颜色，色板与设计令牌一致 */
  tone: {
    /** 中性：次要状态、被动信息。唯一保留灰色的色调——它的语义就是「无色」，
     *  一旦染成主题色就会和 brand 只剩文字深浅之差 */
    neutral: 'bg-slate-100 text-slate-700 border-slate-200',
    /** 品牌：默认标签、进行中的主流程 */
    brand: 'bg-teal-100 text-teal-800 border-teal-200',
    /** 成功 */
    success: 'bg-emerald-100 text-emerald-800 border-emerald-200',
    /** 警告 */
    warning: 'bg-amber-100 text-amber-800 border-amber-200',
    /** 危险 / 失败 */
    danger: 'bg-rose-100 text-rose-800 border-rose-200',
    /** 处理中（扫描、排队等中性进行态） */
    info: 'bg-blue-100 text-blue-800 border-blue-200',
    /** 描边：白底，用于需要与中性标签区分开的次要标记 */
    outline: 'bg-white text-slate-700 border-line',
  },
} as const;

/** 表格：列表页与详情页共用同一套外壳 */
export const table = {
  wrapper: 'overflow-x-auto border border-line rounded-lg',
  root: 'w-full text-left text-xs divide-y divide-line',
  head: 'bg-tint text-slate-600 uppercase font-medium tracking-wider',
  headCell: 'px-3 py-2.5',
  body: 'divide-y divide-line-soft bg-white',
  /** 单元格纵向内距统一为 py-2.5 */
  cell: 'px-3 py-2.5',
  row: 'transition-colors hover:bg-tint',
} as const;

/** 布局：页面外壳与常用间距 */
export const layout = {
  /** 页面根容器（宽度与 padding 由 Layout 的 main 提供，页面自身不限制宽度） */
  page: 'grid content-start gap-4 min-w-0',
  /** 页头：标题区 + 操作区，窄屏堆叠 */
  pageHeader:
    'flex flex-col gap-4 pb-2 border-b border-line sm:flex-row sm:items-center sm:justify-between',
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
  placeholder: 'h-10 w-10 text-line-strong',
} as const;
