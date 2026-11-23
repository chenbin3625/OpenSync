import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const taskDetailSource = readFileSync(new URL('../src/pages/Home/TaskDetail.tsx', import.meta.url), 'utf8');
const homeUtilsSource = readFileSync(new URL('../src/pages/Home/homeUtils.ts', import.meta.url), 'utf8');

test('task detail table hides backend-only identifier columns', () => {
  assert.doesNotMatch(taskDetailSource, /title:\s*'AList任务ID'/);
  assert.doesNotMatch(taskDetailSource, /title:\s*'明细ID'/);
});

const statusBadgeSource = readFileSync(
  new URL('../src/components/common/StatusBadge.tsx', import.meta.url),
  'utf8'
);

test('status badge renders the error reason beside the status as a shared component', () => {
  // 徽标 + 失败原因的组合收敛到 StatusBadge，列表页与详情页复用；
  // 悬浮入口本身再由 ErrorReason 统一实现，避免各处各写一套 Tooltip
  assert.match(statusBadgeSource, /className=\{cn\('inline-flex items-center gap-1 max-w-full'/);
  assert.match(statusBadgeSource, /<ErrorReason errMsg=\{errMsg\} \/>/);

  const errorReasonSource = readFileSync(
    new URL('../src/components/common/ErrorReason.tsx', import.meta.url),
    'utf8'
  );
  assert.match(errorReasonSource, /aria-label="查看失败原因"/);
});

test('task detail shows error reasons beside status instead of a separate error column', () => {
  assert.doesNotMatch(taskDetailSource, /title:\s*'错误信息'/);
  assert.match(taskDetailSource, /<StatusBadge\b/);
  assert.match(taskDetailSource, /errMsg=\{/);
  assert.match(taskDetailSource, /taskItemStatusColors\[status\] === 'error'/);
});

test('task history list shows task-level error reasons beside status like file-level failures', () => {
  const taskListSource = readFileSync(new URL('../src/pages/Home/TaskList.tsx', import.meta.url), 'utf8');
  assert.match(taskListSource, /<StatusBadge\b/);
  assert.match(taskListSource, /errMsg=\{/);
  assert.match(taskListSource, /taskStatusColors\[s\] === 'error'/);
});

test('task detail uses shared explicit task item status metadata', () => {
  assert.doesNotMatch(taskDetailSource, /const taskItemStatusList/);
  assert.match(taskDetailSource, /taskItemStatusNames/);
  assert.match(taskDetailSource, /taskItemStatusOptions/);
  assert.match(homeUtilsSource, /export const taskItemStatusNames/);
  assert.match(homeUtilsSource, /9:\s*'等待重试前'/);
});

test('task detail polling follows document visibility', () => {
  assert.match(taskDetailSource, /canPollCurrentDocument/);
  assert.match(taskDetailSource, /if \(!cancelled && canPollCurrentDocument\(\)\)/);
  assert.match(taskDetailSource, /await fetchData\(\{ silent: true \}\)/);
});

test('task detail silent polling clears its in-flight guard after each request', () => {
  assert.match(taskDetailSource, /if \(options\?\.silent && abortRef\.current\) return;/);
  assert.match(taskDetailSource, /abortRef\.current = controller/);
  assert.match(taskDetailSource, /if \(abortRef\.current === controller\) abortRef\.current = null;/);
});

test('task detail keeps its current rows during an unchanged refresh', () => {
  assert.match(taskDetailSource, /const loading = Boolean\(taskId && !currentPage\);/);
  assert.match(taskDetailSource, /mergeTaskItems\(previous\.items, items\)/);
  assert.match(taskDetailSource, /if \(merged === previous\.items && count === previous\.count\) return previous;/);
});
