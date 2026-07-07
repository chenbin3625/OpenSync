import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const taskDetailSource = readFileSync(new URL('../src/pages/Home/TaskDetail.tsx', import.meta.url), 'utf8');
const homeUtilsSource = readFileSync(new URL('../src/pages/Home/homeUtils.ts', import.meta.url), 'utf8');

test('task detail table hides backend-only identifier columns', () => {
  assert.doesNotMatch(taskDetailSource, /title:\s*'AList任务ID'/);
  assert.doesNotMatch(taskDetailSource, /title:\s*'明细ID'/);
});

test('task detail shows error reasons beside status instead of a separate error column', () => {
  assert.doesNotMatch(taskDetailSource, /title:\s*'错误信息'/);
  assert.match(taskDetailSource, /InfoCircleOutlined/);
  assert.match(taskDetailSource, /className="task-status-with-error"/);
  assert.match(taskDetailSource, /Tooltip\s+title=\{record\.errMsg\}/);
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
  assert.match(taskDetailSource, /if \(canPollCurrentDocument\(\)\) fetchData\(\{ silent: true \}\)/);
});

test('task detail silent polling does not toggle table loading state', () => {
  assert.match(taskDetailSource, /const showLoading = !options\?\.silent/);
  assert.match(taskDetailSource, /if \(showLoading\) \{\s+setLoading\(true\);\s+setError\(false\);\s+\}/s);
  assert.match(taskDetailSource, /showLoading && requestID === requestRef\.current/);
});
