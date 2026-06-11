import type { JobItem } from '../../types';

export type ScheduleValues = {
  isCron?: number;
  interval?: number;
  second?: string | null;
  minute?: string | null;
  hour?: string | null;
  day?: string | null;
  month?: string | null;
  day_of_week?: string | null;
};

// ---- Constants ----

export const statusColors: Record<number, string> = {
  0: 'default', 1: 'processing', 2: 'success', 3: 'warning',
  4: 'default', 5: 'warning', 6: 'error', 7: 'error',
};
export const statusLabels: Record<number, string> = {
  0: '禁用', 1: '启用',
};
export const methodOptions = [
  {
    name: '仅新增',
    description: '复制源目录中目标端不存在或内容变化的文件，不删除目标端多余文件，适合增量备份。',
  },
  {
    name: '全同步',
    description: '目标目录尽量与源目录保持一致，会复制新增/变更文件，并删除目标端源目录已不存在的文件。',
  },
  {
    name: '移动模式',
    description: '按移动任务处理新增/变更文件，适合把文件从源端迁移到目标端，用于归档或腾挪空间。',
  },
];
export const methodNames = methodOptions.map((method) => method.name);
export const cronTypeNames = ['间隔(分钟)', 'Cron', '仅手动'];
export const cronFields = [
  { name: 'second', label: '秒', placeholder: '0' },
  { name: 'minute', label: '分', placeholder: '*' },
  { name: 'hour', label: '时', placeholder: '*' },
  { name: 'day', label: '日', placeholder: '*' },
  { name: 'month', label: '月', placeholder: '*' },
  { name: 'day_of_week', label: '周', placeholder: '*' },
];
export const defaultCronFields = {
  second: '0',
  minute: '0',
  hour: '2',
  day: '*',
  month: '*',
  day_of_week: '*',
};

export const compactItemStyle = { marginBottom: 12 };
export const compactDividerStyle = { margin: '8px 0 12px' };

export const defaultExclude = `# macOS
.DS_Store
._*
.AppleDouble/
.LSOverride
.Spotlight-V100/
.Trashes/
.TemporaryItems/
.fseventsd/
.DocumentRevisions-V100/

# Windows
Thumbs.db
ehthumbs.db
ehthumbs_vista.db
Desktop.ini
$RECYCLE.BIN/
RECYCLER/
System Volume Information/

# Linux / NAS
lost+found/
@eaDir/
#recycle/
@Recycle/
.Recycle/
.Trash-*/
.Trash/

# 临时文件 / 下载未完成文件
*.tmp
*.temp
*.log
*.bak
*.old
*.orig
*.part
*.crdownload
*.download
*.swp
*.swo
*.swn
*~
~*
~$*
*.lock
.~lock.*#

# 缓存目录
.cache/
cache/
tmp/
temp/
logs/
log/

# 开发相关缓存 / 依赖
node_modules/
.npm/
.yarn/
.pnpm-store/
__pycache__/
*.pyc
*.pyo
.pytest_cache/
.mypy_cache/
.ruff_cache/
.tox/
.venv/
venv/
env/
.idea/
.vscode/
*.iml

# 版本控制目录
.git/
.svn/
.hg/

# 构建产物
.sass-cache/
.gradle/
build/
dist/
target/
coverage/
.next/
.nuxt/
.turbo/`;

// ---- Utility Functions ----

export const cronValue = (value?: string | null, fallback = '*') => {
  const normalized = String(value ?? '').trim();
  return normalized || fallback;
};

export const formatTime = (hour?: string | null, minute?: string | null, second?: string | null) => {
  const h = cronValue(hour);
  const m = cronValue(minute);
  const s = cronValue(second, '0');
  if (/^\d+$/.test(h) && /^\d+$/.test(m) && /^\d+$/.test(s)) {
    return `${h.padStart(2, '0')}:${m.padStart(2, '0')}:${s.padStart(2, '0')}`;
  }
  return `${h}:${m}:${s}`;
};

export const describeCronPlan = (values: ScheduleValues) => {
  const second = cronValue(values.second, '0');
  const minute = cronValue(values.minute);
  const hour = cronValue(values.hour);
  const day = cronValue(values.day);
  const month = cronValue(values.month);
  const dayOfWeek = cronValue(values.day_of_week);
  const time = formatTime(hour, minute, second);

  if (day === '*' && month === '*' && dayOfWeek === '*' && hour !== '*' && minute !== '*') {
    return `每天 ${time} 执行`;
  }
  if (day !== '*' && month === '*' && dayOfWeek === '*' && hour !== '*' && minute !== '*') {
    return `每月 ${day} 日 ${time} 执行`;
  }
  if (day === '*' && month === '*' && dayOfWeek !== '*' && hour !== '*' && minute !== '*') {
    return `每周 ${dayOfWeek} 的 ${time} 执行`;
  }
  return `按 Cron 表达式 ${[second, minute, hour, day, month, dayOfWeek].join(' ')} 执行`;
};

export const formatSchedulePlan = (values: ScheduleValues) => {
  if (values.isCron === 0) return `每 ${values.interval || 0} 分钟执行一次`;
  if (values.isCron === 1) return describeCronPlan(values);
  return '不自动执行，只能手动触发';
};

export const parseJobPathList = (value: unknown): string[] => {
  if (Array.isArray(value)) {
    return value.map((item) => String(item).trim()).filter(Boolean);
  }
  const raw = String(value ?? '').trim();
  if (!raw) return [];
  try {
    const parsed = JSON.parse(raw);
    if (Array.isArray(parsed)) {
      return parsed.map((item) => String(item).trim()).filter(Boolean);
    }
  } catch { /* plain single path */ }
  return [raw];
};

export const normalizeFormPaths = (value: string | string[] | undefined): string[] => {
  const paths = Array.isArray(value) ? value : [value];
  return paths.map((item) => String(item ?? '').trim()).filter(Boolean);
};

export const parseJobSrcPaths = parseJobPathList;
export const parseJobDstPaths = parseJobPathList;

export const formatJobPaths = (value: unknown, separator = '、') => {
  const paths = parseJobPathList(value);
  return paths.length > 0 ? paths.join(separator) : '';
};

export const countJobPaths = (value: unknown) => parseJobPathList(value).length;

export const formatSize = (bytes: number) => {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let size = bytes;
  let unitIndex = 0;
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024;
    unitIndex++;
  }
  return `${size.toFixed(unitIndex === 0 ? 0 : 2)} ${units[unitIndex]}`;
};

export const formatFileSizeRange = (minSize?: number | null, maxSize?: number | null) => {
  const min = Number(minSize || 0);
  const max = Number(maxSize || 0);
  if (min <= 0 && max <= 0) return '';
  if (min > 0 && max > 0) return `${formatSize(min)} ~ ${formatSize(max)}`;
  if (min > 0) return `不小于 ${formatSize(min)}`;
  return `不大于 ${formatSize(max)}`;
};

export function formatExcludePreview(value?: string | null): string {
  const rules = String(value ?? '')
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter((line) => line && !line.startsWith('#'));

  if (rules.length === 0) return '仅注释';
  const preview = rules.slice(0, 3).join('、');
  return rules.length > 3 ? `${preview} 等 ${rules.length} 条` : preview;
}

export const getJobName = (job: JobItem) => job.remark || `同步任务 #${job.id}`;

export const formatSchedule = (job: JobItem) => {
  if (job.isCron === 0) return `每 ${job.interval} 分钟`;
  if (job.isCron === 1) {
    return describeCronPlan(job);
  }
  return '仅手动触发';
};

export const formatCache = (useS: number | boolean, useT: number | boolean) => {
  const s = useS ? '源端✓' : '源端✗';
  const t = useT ? '目标✓' : '目标✗';
  return `${s} / ${t}`;
};
