import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const engineSource = readFileSync(new URL('../src/pages/Engine/index.tsx', import.meta.url), 'utf8');
const notifySource = readFileSync(new URL('../src/pages/Notify/index.tsx', import.meta.url), 'utf8');
const homeSource = readFileSync(new URL('../src/pages/Home/index.tsx', import.meta.url), 'utf8');
const taskListSource = readFileSync(new URL('../src/pages/Home/TaskList.tsx', import.meta.url), 'utf8');

test('editing an engine clears stale modal token state before applying current values', () => {
  assert.match(engineSource, /const handleEdit = \(item: AlistItem\) => \{\s+setEditingItem\(item\);\s+form\.resetFields\(\);/s);
  assert.match(engineSource, /form\.setFieldsValue\(\{ url: item\.url, remark: item\.remark \|\| '', token: undefined \}\)/);
});

test('notification params are built from method-specific allowlists instead of the whole form', () => {
  assert.match(notifySource, /getNotifyParamsFromValues/);
  assert.doesNotMatch(notifySource, /const params: NotifyParams = \{ \.\.\.values \}/);
});

test('custom webhook exposes advanced body and headers fields', () => {
  assert.match(notifySource, /0: \[[^\]]*'body'[^\]]*'headers'[^\]]*\]/s);
  assert.match(notifySource, /name="body"/);
  assert.match(notifySource, /name="headers"/);
});

test('editing or switching notification methods clears unrelated preserved fields', () => {
  assert.match(notifySource, /form\.resetFields\(\);[\s\S]+form\.setFieldsValue\(\{ \.\.\.params, method: item\.method, enable: item\.enable === 1 \}\)/);
  assert.match(notifySource, /handleMethodChange/);
});

test('history task queries request completed statuses from the server', () => {
  assert.match(taskListSource, /params\.statusIn = historyCompletedStatuses/);
});

test('directory tree loading ignores stale engine responses', () => {
  assert.match(homeSource, /treeLoadRequestRef/);
  assert.match(homeSource, /if \(requestID !== treeLoadRequestRef\.current\) return;/);
});
