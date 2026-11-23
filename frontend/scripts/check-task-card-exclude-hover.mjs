import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const scriptDir = dirname(fileURLToPath(import.meta.url));
const frontendRoot = join(scriptDir, '..');
const overviewSource = readFileSync(join(frontendRoot, 'src/pages/Home/HomeOverview.tsx'), 'utf8');
const homeUtilsSource = readFileSync(join(frontendRoot, 'src/pages/Home/homeUtils.ts'), 'utf8');
const tooltipSource = readFileSync(join(frontendRoot, 'src/components/ui/tooltip.tsx'), 'utf8');

const checks = [
  {
    ok: overviewSource.includes('formatExcludePreview(selectedJob.exclude)'),
    message: '任务概览排除项需要使用 formatExcludePreview 渲染紧凑预览，避免直接铺开多行文本。',
  },
  {
    ok: !overviewSource.includes('ellipsis={{ tooltip:'),
    message: '排除项不能使用已移除的 AntD ellipsis tooltip 写法。',
  },
  {
    ok: /<Tooltip>[\s\S]*?<TooltipTrigger asChild>[\s\S]*?formatExcludePreview\(selectedJob\.exclude\)[\s\S]*?<TooltipContent className="font-mono">[\s\S]*?\{selectedJob\.exclude\}/.test(overviewSource),
    message: '任务概览排除项需要通过 TooltipTrigger + font-mono TooltipContent 悬浮展示完整规则。',
  },
  {
    ok: homeUtilsSource.includes('export function formatExcludePreview'),
    message: 'homeUtils 需要导出 formatExcludePreview 以过滤注释并规范化预览文案。',
  },
  {
    ok: tooltipSource.includes('max-h-[min(60vh,20rem)]') && tooltipSource.includes('overflow-y-auto'),
    message: 'TooltipContent 需要限高并支持内部滚动，避免长排除规则 hover 时撑满页面。',
  },
  {
    ok: tooltipSource.includes('whitespace-pre-wrap') && tooltipSource.includes('break-words'),
    message: 'TooltipContent 需要保留换行并允许超长规则折行。',
  },
  {
    ok: tooltipSource.includes('max-w-xs'),
    message: 'TooltipContent 需要最大宽度限制，避免超长单行规则横向溢出。',
  },
];

const failures = checks.filter((check) => !check.ok);

if (failures.length > 0) {
  console.error(failures.map((failure) => `- ${failure.message}`).join('\n'));
  process.exit(1);
}

console.log('Task card exclude hover styles are guarded.');
