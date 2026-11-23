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
const layoutSource = readFileSync(new URL('../src/components/Layout/index.tsx', import.meta.url), 'utf8');
const requestSource = readFileSync(new URL('../src/api/request.ts', import.meta.url), 'utf8');

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

test('custom webhook preserves masked body, httpMethod, and hidden webhook params across edits', () => {
  assert.match(notifySource, /isMaskedSecretPlaceholder\(savedBody\)/);
  assert.match(notifySource, /nextHints\.body = true;/);
  assert.match(
    notifySource,
    /if \(typeof normalized\.body === 'string' && isMaskedSecretPlaceholder\(normalized\.body\.trim\(\)\)\) \{\s+delete normalized\.body;/s,
  );
  assert.match(notifySource, /initialValues\.httpMethod = rawHttpMethod\.toUpperCase\(\);/);
  assert.match(notifySource, /normalized\.method = normalized\.method \|\| normalized\.httpMethod \|\| 'POST';/);
  assert.match(notifySource, /delete normalized\.httpMethod;/);
  assert.match(notifySource, /\.\.\.\(method === 0 \? preservedWebhookParams : \{\}\)/);
  assert.match(notifySource, /initialValues\.version = paramString\(params, 'version'\) \|\| 'v1';/);
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

test('task history pagination steps back when current page exceeds maxPage or becomes empty', () => {
  assert.match(taskListSource, /const maxPage = Math\.max\(1, Math\.ceil\(nextTotal \/ pageSize\)\);/);
  assert.match(
    taskListSource,
    /if \(page > 1 && \(nextList\.length === 0 \|\| page > maxPage\)\) \{\s+setPage\(Math\.min\(page - 1, maxPage\)\);\s+return;\s+\}/s,
  );
  assert.match(
    taskListSource,
    /if \(page > 1 && historyList\.length <= 1\) \{\s+setPage\(\(current\) => Math\.max\(1, current - 1\)\);/s,
  );
  assert.match(taskListSource, /if \(!jobId \|\| dateRangeInvalid\) return;/);
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
  assert.match(
    jobFormDrawerSource,
    /<SwitchRow\b[^>]*label="任务启用状态"[^>]*disabled=\{isCron === 2\}[^>]*checked=\{isCron === 2 \? true : enable\}/s,
  );
});

test('job drawer rejects whitespace inside cron fields and trims cron values on submit', () => {
  assert.match(
    jobFormDrawerSource,
    /if \(\/\\s\/\.test\(normalized\)\) \{\s+return Promise\.reject\(new Error\('不能包含空格'\)\);\s+\}/s,
  );
  assert.match(jobFormDrawerSource, /const normalizedCronState = \{/);
  assert.match(jobFormDrawerSource, /day_of_week: cronState\.day_of_week\.trim\(\)/);
});

test('job drawer preserves cleared exclude rules when editing an existing job', () => {
  assert.match(jobFormDrawerSource, /setExclude\(editingJob\.exclude \?\? ''\);/);
  assert.doesNotMatch(jobFormDrawerSource, /setExclude\(editingJob\.exclude \|\| defaultExclude\)/);
  assert.match(jobFormDrawerSource, /exclude: exclude\.trim\(\) \|\| null/);
});

test('directory tree loading ignores stale engine responses', () => {
  assert.match(jobFormDrawerSource, /treeLoadRequestRef/);
  assert.match(usePathTreeSource, /if \(requestID !== treeLoadRequestRef\.current \|\| generation !== generationRef\.current\) return;/);
});

test('job edit drawer seeds selected directory nodes before async tree data arrives', () => {
  assert.match(usePathTreeSource, /buildPathTreeData\(parseJobPathList\(paths\)\)/);
  assert.match(usePathTreeSource, /setTreeData\(pathTree\)/);
  assert.match(usePathTreeSource, /mergeTreeData\(root, pathTree\)/);
  assert.match(jobFormDrawerSource, /loadSrcRoot\(directorySelection\.sourcePaths\)/);
  assert.match(jobFormDrawerSource, /loadDstRoot\(directorySelection\.destinationPaths\)/);
});

test('job drawer aborts in-flight submit when closed', () => {
  assert.match(jobFormDrawerSource, /submitAbortRef/);
  assert.match(jobFormDrawerSource, /submitAbortRef\.current\?\.abort\(\)/);
  assert.match(jobFormDrawerSource, /jobPost\(jobData, \{ signal: controller\.signal \}\)/);
});

test('forms inside overlays use controlled dialog and sheet overlays', () => {
  assert.match(jobFormDrawerSource, /<Sheet\b/);
  assert.match(jobFormDrawerSource, /<SheetContent side="right"[^>]*overflow-y-auto/);
  assert.match(engineSource, /<Dialog\b[^>]*open=\{modalVisible\}/);
  assert.match(notifySource, /<Dialog\b[^>]*open=\{modalVisible\}/);
  assert.match(loginSource, /<Dialog\b[^>]*open=\{resetModalOpen\}/);
  assert.match(settingSource, /<Dialog\b[^>]*open=\{passwordVisible\}/);
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
  assert.match(settingSource, /if \(!Number\.isInteger\(num\)\)/);
  assert.match(settingSource, /configAbortRef\.current\?\.abort\(\)/);
});

test('system settings validate password UTF-8 byte limit and verify session after password rotation', () => {
  assert.match(settingSource, /const MAX_PASSWORD_BYTES = 72;/);
  assert.match(settingSource, /new TextEncoder\(\)\.encode\(value\)\.length > MAX_PASSWORD_BYTES/);
  assert.match(settingSource, /await editPwd\(\{ oldPasswd, passwd: newPasswd \}\);/);
  assert.match(settingSource, /const res = await getUser\(\);/);
});

test('login validates password UTF-8 byte limit, guards init status checks, and clears reset modal fields', () => {
  assert.match(loginSource, /const MAX_PASSWORD_BYTES = 72;/);
  assert.match(loginSource, /new TextEncoder\(\)\.encode\(value\)\.length > MAX_PASSWORD_BYTES/);
  assert.match(loginSource, /const initCheckReqRef = useRef\(0\);/);
  assert.match(loginSource, /if \(reqID !== initCheckReqRef\.current\) return;/);
  assert.match(loginSource, /setInitCheckError\(true\)/);
  assert.match(
    loginSource,
    /const handleResetModalOpenChange = \(open: boolean\) => \{\s+setResetModalOpen\(open\);\s+if \(!open\) \{\s+resetPasswordModalFields\(\);/s,
  );
});

test('layout logout keeps dialog open on failure and blocks dismissal while logout is in flight', () => {
  assert.match(layoutSource, /if \(!open && logoutLoading\) return;/);
  assert.match(layoutSource, /setLogoutError\('退出请求失败，请检查网络后重试'\);\s+return;/s);
  assert.match(layoutSource, /setUserInfo\(null\);\s+setLogoutDialogOpen\(false\);\s+navigate\('\/login'\);/s);
});

test('request interceptor ignores canceled axios errors, validates API envelopes, and deduplicates 401 redirects', () => {
  assert.match(
    requestSource,
    /if \(axios\.isCancel\(error\) \|\| error\?\.code === 'ERR_CANCELED' \|\| error\?\.name === 'CanceledError'\) \{\s+return Promise\.reject\(error\);\s+\}/s,
  );
  assert.match(requestSource, /if \(!isApiEnvelope\(res\.data\)\)/);
  assert.match(requestSource, /if \(redirectingToLogin\) \{\s+return;\s+\}/s);
  assert.match(requestSource, /if \(!wasChecked\) \{/);
  assert.match(requestSource, /paramsSerializer:\s*\{\s*serialize: serializeParams,/s);
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

test('login reset dialog documents where to obtain the recovery key', () => {
  // 恢复密钥只在展示时可见、服务端只存哈希，因此页面必须说明来源与丢失后的兜底获取方式
  assert.match(loginSource, /找不到恢复密钥？/);
  assert.match(loginSource, /无法再次查看或找回/);
  assert.match(loginSource, /reset-password --user/);
  // Docker 兜底命令需以容器内应用用户执行，避免 data 目录属主被改写
  assert.match(loginSource, /docker compose exec --user/);
  assert.match(loginSource, /opensync \.\/opensync reset-password --user/);
  // 兜底命令回填已输入的用户名，非法字符回落到示例用户名
  assert.match(loginSource, /safeResetUserName/);
  assert.match(loginSource, /reset-password --user \$\{safeResetUserName\}/);
  assert.doesNotMatch(loginSource, /secret\.key/);
  // 说明是可折叠的：按钮要带上 aria-expanded，点击要真的翻转状态，
  // 只写 onClick={() => {}} 或漏掉 aria-expanded，折叠区就是摆设（永远展开或永远不展开）
  assert.match(loginSource, /onClick=\{\(\) => setShowRecoveryHelp\(\(prev\) => !prev\)\}/);
  assert.match(loginSource, /aria-expanded=\{showRecoveryHelp\}/);
  assert.match(loginSource, /\{showRecoveryHelp && \(/);
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
