import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const scriptDir = dirname(fileURLToPath(import.meta.url));
const frontendRoot = join(scriptDir, '..');
const homeSource = readFileSync(join(frontendRoot, 'src/pages/Home/index.tsx'), 'utf8');
const cssSource = readFileSync(join(frontendRoot, 'src/index.css'), 'utf8');

const checks = [
  {
    ok: homeSource.includes('function JobExcludeText'),
    message: '任务卡片排除项需要专用渲染组件，避免多行文本直接交给 Typography ellipsis tooltip。',
  },
  {
    ok: !homeSource.includes('ellipsis={{ tooltip: job.exclude }}'),
    message: '排除项不能继续使用原始多行文本作为 ellipsis tooltip。',
  },
  {
    ok: homeSource.includes('<JobExcludeText value={selectedJob.exclude} />'),
    message: '任务概览排除项需要使用 JobExcludeText。',
  },
  {
    ok: homeSource.includes('sync-job-exclude-tooltip'),
    message: '排除项 Tooltip 需要稳定的浮层 className。',
  },
  {
    ok: cssSource.includes('.sync-job-exclude-preview'),
    message: '排除项预览需要单行省略样式。',
  },
  {
    ok: cssSource.includes('.ant-tooltip.sync-job-exclude-tooltip'),
    message: '排除项 Tooltip 需要覆盖 AntD 默认浮层宽度。',
  },
  {
    ok: cssSource.includes('white-space: pre-wrap'),
    message: '排除项 Tooltip 需要保留换行并允许内容换行。',
  },
  {
    ok: cssSource.includes('max-height: 320px'),
    message: '排除项 Tooltip 需要最大高度，避免 hover 时撑满页面。',
  },
];

const failures = checks.filter((check) => !check.ok);

if (failures.length > 0) {
  console.error(failures.map((failure) => `- ${failure.message}`).join('\n'));
  process.exit(1);
}

console.log('Task card exclude hover styles are guarded.');
