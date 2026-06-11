import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const taskDetailSource = readFileSync(new URL('../src/pages/Home/TaskDetail.tsx', import.meta.url), 'utf8');

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
