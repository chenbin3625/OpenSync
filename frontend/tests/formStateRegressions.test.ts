import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const engineSource = readFileSync(new URL('../src/pages/Engine/index.tsx', import.meta.url), 'utf8');
const notifySource = readFileSync(new URL('../src/pages/Notify/index.tsx', import.meta.url), 'utf8');
const jobFormDrawerSource = readFileSync(new URL('../src/pages/Home/JobFormDrawer.tsx', import.meta.url), 'utf8');
const usePathTreeSource = readFileSync(new URL('../src/pages/Home/usePathTree.ts', import.meta.url), 'utf8');
const taskListSource = readFileSync(new URL('../src/pages/Home/TaskList.tsx', import.meta.url), 'utf8');
const loginSource = readFileSync(new URL('../src/pages/Login/index.tsx', import.meta.url), 'utf8');
const userApiSource = readFileSync(new URL('../src/api/user.ts', import.meta.url), 'utf8');
const settingSource = readFileSync(new URL('../src/pages/Setting/index.tsx', import.meta.url), 'utf8');

test('editing an engine clears stale modal token state before applying current values', () => {
  assert.match(engineSource, /const handleEdit = \(item: AlistItem\) => \{\s+setEditingItem\(item\);\s+form\.resetFields\(\);/s);
  assert.match(engineSource, /form\.setFieldsValue\(\{ url: item\.url, remark: item\.remark \|\| '', token: undefined \}\)/);
});

test('engine list ignores stale responses after overlapping refreshes', () => {
  assert.match(engineSource, /const listReqRef = useRef\(0\)/);
  assert.match(engineSource, /const reqID = \+\+listReqRef\.current/);
  assert.match(engineSource, /if \(reqID !== listReqRef\.current\) return;/);
});

test('engine connection test uses silent request handling', () => {
  assert.match(engineSource, /alistGetPath\(item\.id, '\/', \{ silent: true \}\)/);
  assert.doesNotMatch(engineSource, /res\.code === 200/);
});

test('engine URL submission accepts http and https addresses', () => {
  assert.match(engineSource, /validateAlistURL/);
  assert.match(engineSource, /new URL\(value\)/);
  assert.match(engineSource, /url\.protocol === 'http:' \|\| url\.protocol === 'https:'/);
  assert.doesNotMatch(engineSource, /非本机地址请使用 HTTPS/);
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
  assert.match(notifySource, /form\.resetFields\(\);[\s\S]+setPendingNotifyValues\(\{ \.\.\.params, method: item\.method, enable: item\.enable === 1 \}/);
  assert.match(notifySource, /if \(!modalVisible \|\| !pendingNotifyValues \|\| pendingNotifyValues\.method !== method\) return;/);
  assert.match(notifySource, /handleMethodChange/);
});

test('notification edit values are applied after method-specific fields are mounted', () => {
  assert.match(notifySource, /const \[pendingNotifyValues, setPendingNotifyValues\]/);
  assert.match(notifySource, /form\.setFieldsValue\(pendingNotifyValues\)/);
  assert.match(notifySource, /\[form, method, modalVisible, pendingNotifyValues\]/);
});

test('history task queries request completed statuses from the server', () => {
  assert.match(taskListSource, /params\.statusIn = historyCompletedStatuses/);
});

test('job edit drawer binds file size inputs to InputNumber via inner noStyle items', () => {
  for (const field of ['minFileSize', 'maxFileSize']) {
    const wrapPattern = new RegExp(`<Form\\.Item[^>]*name="${field}"[^>]*>\\s*<Space\\.Compact`);
    assert.doesNotMatch(jobFormDrawerSource, wrapPattern);
    const innerPattern = new RegExp(`<Form\\.Item\\b[^>]*name="${field}"[\\s\\S]{0,1500}?noStyle[\\s\\S]{0,1500}?>\\s*<InputNumber`);
    assert.match(jobFormDrawerSource, innerPattern);
  }
});

test('manual-only jobs keep enable true in the drawer and submit payload', () => {
  assert.match(jobFormDrawerSource, /if \(isCronValue === 2 && form\.getFieldValue\('enable'\) !== true\)/);
  assert.match(jobFormDrawerSource, /enable: values\.isCron === 2 \? 1 : \(values\.enable \? 1 : 0\)/);
  assert.match(jobFormDrawerSource, /<Switch disabled=\{isCronValue === 2\} \/>/);
});

test('directory tree loading ignores stale engine responses', () => {
  assert.match(jobFormDrawerSource, /treeLoadRequestRef/);
  assert.match(usePathTreeSource, /if \(requestID !== treeLoadRequestRef\.current\) return;/);
});

test('job edit drawer seeds selected directory nodes before async tree data arrives', () => {
  assert.match(usePathTreeSource, /buildPathTreeData\(parseJobPathList\(paths\)\)/);
  assert.match(usePathTreeSource, /setTreeData\(pathTree\)/);
  assert.match(usePathTreeSource, /mergeTreeData\(root, pathTree\)/);
  assert.match(jobFormDrawerSource, /loadSrcRoot\(editingJobSrcPath\)/);
  assert.match(jobFormDrawerSource, /loadDstRoot\(editingJobDstPath\)/);
});

test('job drawer aborts in-flight submit when closed', () => {
  assert.match(jobFormDrawerSource, /submitAbortRef/);
  assert.match(jobFormDrawerSource, /submitAbortRef\.current\?\.abort\(\)/);
  assert.match(jobFormDrawerSource, /jobPost\(jobData, \{ signal: controller\.signal \}\)/);
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

test('system settings keep fetched config in state before syncing into the mounted form', () => {
  assert.match(settingSource, /const \[configValues, setConfigValues\] = useState<SystemSettings \| null>\(null\)/);
  assert.match(settingSource, /setConfigValues\(res\.data\)/);
  assert.match(settingSource, /if \(!loading && configValues\) \{\s+configForm\.setFieldsValue\(configValues\);/s);
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
  assert.match(loginSource, /initializeUser\(\{[\s\S]*setupToken/);
  assert.match(loginSource, /confirmPasswd/);
  assert.match(loginSource, /创建管理员账号/);
  assert.match(loginSource, /recoveryKey/);
  assert.match(loginSource, /请立即保存恢复密钥/);
});
