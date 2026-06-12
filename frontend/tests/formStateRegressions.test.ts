import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const engineSource = readFileSync(new URL('../src/pages/Engine/index.tsx', import.meta.url), 'utf8');
const notifySource = readFileSync(new URL('../src/pages/Notify/index.tsx', import.meta.url), 'utf8');
const jobFormDrawerSource = readFileSync(new URL('../src/pages/Home/JobFormDrawer.tsx', import.meta.url), 'utf8');
const taskListSource = readFileSync(new URL('../src/pages/Home/TaskList.tsx', import.meta.url), 'utf8');
const loginSource = readFileSync(new URL('../src/pages/Login/index.tsx', import.meta.url), 'utf8');
const userApiSource = readFileSync(new URL('../src/api/user.ts', import.meta.url), 'utf8');
const settingSource = readFileSync(new URL('../src/pages/Setting/index.tsx', import.meta.url), 'utf8');

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
  assert.match(jobFormDrawerSource, /treeLoadRequestRef/);
  assert.match(jobFormDrawerSource, /if \(requestID !== treeLoadRequestRef\.current\) return;/);
});

test('forms inside overlays are force rendered before form APIs run', () => {
  assert.match(jobFormDrawerSource, /<Drawer[\s\S]*className="sync-job-drawer"[\s\S]*forceRender[\s\S]*>/);
  assert.match(engineSource, /<Modal[\s\S]*title=\{editingItem \? '编辑引擎' : '新增引擎'\}[\s\S]*forceRender[\s\S]*>/);
  assert.match(notifySource, /<Modal[\s\S]*title=\{editingItem \? '编辑通知' : '新增通知'\}[\s\S]*forceRender[\s\S]*>/);
  assert.match(loginSource, /<Modal[\s\S]*title="重置密码"[\s\S]*forceRender[\s\S]*>/);
  assert.match(settingSource, /<Modal[\s\S]*title="修改密码"[\s\S]*forceRender[\s\S]*>/);
});

test('system setting unit inputs bind the input control inside compact groups', () => {
  for (const field of ['expires', 'taskTimeout', 'taskSave']) {
    assert.doesNotMatch(
      settingSource,
      new RegExp(`<Form\\.Item[\\s\\S]{0,300}name="${field}"[\\s\\S]{0,300}>\\s*<Space\\.Compact`)
    );
    assert.match(
      settingSource,
      new RegExp(`<Form\\.Item[\\s\\S]{0,120}name="${field}"[\\s\\S]{0,120}noStyle[\\s\\S]{0,180}>\\s*<InputNumber`)
    );
  }
});

test('login reset success uses context-aware modal feedback', () => {
  assert.match(loginSource, /const \{ message, modal \} = App\.useApp\(\);/);
  assert.match(loginSource, /modal\.info\(/);
  assert.doesNotMatch(loginSource, /Modal\.info\(/);
});

test('login password reset uses recovery key instead of secret key', () => {
  assert.match(userApiSource, /recoveryKey/);
  assert.match(loginSource, /name="recoveryKey"/);
  assert.match(loginSource, /placeholder="恢复密钥"/);
  assert.doesNotMatch(loginSource, /secret\.key/);
  assert.doesNotMatch(loginSource, /加密秘钥/);
});

test('login page supports first-run web initialization', () => {
  assert.match(userApiSource, /getInitStatus/);
  assert.match(userApiSource, /initializeUser/);
  assert.match(loginSource, /getInitStatus\(\)/);
  assert.match(loginSource, /initializeUser\(\{ userName: values\.userName, passwd: values\.passwd \}\)/);
  assert.match(loginSource, /confirmPasswd/);
  assert.match(loginSource, /创建管理员账号/);
  assert.match(loginSource, /recoveryKey/);
  assert.match(loginSource, /请立即保存恢复密钥/);
});
