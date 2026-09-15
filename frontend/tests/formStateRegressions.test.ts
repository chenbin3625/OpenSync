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
  assert.match(engineSource, /const handleEdit = \(item: AlistItem\) => \{\s+setEditingItem\(item\);/s);
  assert.match(engineSource, /setToken\(''\)/);
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
  assert.match(notifySource, /setEditingItem\(item\)/);
  assert.match(notifySource, /setPendingNotifyValues/);
  assert.match(notifySource, /handleMethodChange/);
});

test('notification edit values are applied after method-specific fields are mounted', () => {
  assert.match(notifySource, /const \[pendingNotifyValues, setPendingNotifyValues\]/);
  assert.match(notifySource, /\[form, method, modalVisible, pendingNotifyValues\]/);
});

test('history task queries request completed statuses from the server', () => {
  assert.match(taskListSource, /params\.statusIn = historyCompletedStatuses/);
});

test('job edit drawer binds file size inputs with unit conversion', () => {
  for (const field of ['minFileSize', 'maxFileSize']) {
    assert.match(jobFormDrawerSource, new RegExp(`name="${field}"`));
  }
  assert.match(jobFormDrawerSource, /fileSizeToBytes/);
  assert.match(jobFormDrawerSource, /splitBytesToFileSize/);
});

test('manual-only jobs keep enable true in the drawer and submit payload', () => {
  assert.match(jobFormDrawerSource, /enable: isCron === 2 \? 1 : \(enable \? 1 : 0\)/);
  assert.match(jobFormDrawerSource, /<Switch[\s\S]*disabled=\{isCron === 2\}/);
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

test('forms inside overlays use controlled dialog and sheet overlays', () => {
  assert.match(jobFormDrawerSource, /<Sheet/);
  assert.match(jobFormDrawerSource, /<SheetContent side="right"[^>]*overflow-y-auto/);
  assert.match(engineSource, /<Dialog[\s\S]*open=\{modalVisible\}/);
  assert.match(notifySource, /<Dialog[\s\S]*open=\{modalVisible\}/);
  assert.match(loginSource, /<Dialog[\s\S]*open=\{resetModalOpen\}/);
  assert.match(settingSource, /<Dialog[\s\S]*open=\{passwordVisible\}/);
});

test('system setting unit inputs bind the input control with number validation', () => {
  // 六个字段改为数据驱动渲染，name 来自 configFields 表
  for (const field of ['expires', 'taskTimeout', 'taskSave']) {
    assert.match(settingSource, new RegExp(`name: '${field}'`));
  }
  assert.match(settingSource, /name=\{field\.name\}/);
  assert.match(settingSource, /type="number"/);
  assert.match(settingSource, /min=\{field\.min\}/);
});

test('system settings keep fetched config in state before syncing into the mounted form', () => {
  assert.match(settingSource, /const \[configValues, setConfigValues\] = useState<SystemSettings \| null>\(null\)/);
  assert.match(settingSource, /setConfigValues\(res\.data\)/);
});

test('login reset success uses toast feedback', () => {
  assert.match(loginSource, /toast\.success\('密码重置成功'\)/);
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
  assert.match(loginSource, /initializeUser\(\{ userName: userName, passwd: passwd \}\)/);
  assert.match(loginSource, /confirmPasswd/);
  assert.match(loginSource, /创建管理员账号/);
  assert.match(loginSource, /recoveryKey/);
  assert.match(loginSource, /请立即保存恢复密钥/);
});

