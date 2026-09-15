import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const layoutSource = readFileSync(new URL('../src/components/Layout/index.tsx', import.meta.url), 'utf8');

test('primary app menu is rendered as a top horizontal navigation', () => {
  // 顶部横向导航：<nav> + 横向 flex，窄屏折叠为汉堡菜单，且不存在侧边栏
  assert.match(layoutSource, /<nav aria-label="主导航">/);
  assert.match(layoutSource, /hidden md:flex items-center/);
  assert.match(layoutSource, /md:hidden/);
  assert.doesNotMatch(layoutSource, /<Sider\b|<\/Sider>/);
});
test('user menu combines username and logout into a dropdown', () => {
  assert.match(layoutSource, /<DropdownMenuTrigger asChild>/);
  assert.match(layoutSource, /<Dropdown\b/);
  assert.match(layoutSource, /退出登录/);
  assert.match(layoutSource, /userName \|\| '用户'/);
  assert.doesNotMatch(layoutSource, /<User\b/);
  assert.doesNotMatch(layoutSource, /BulbFilled|BulbOutlined/);
});

test('top bar shows project links and version before the user menu', () => {
  assert.match(layoutSource, /const projectLinks = \[/);
  assert.match(layoutSource, /https:\/\/github\.com\/chenbin3625\/OpenSync/);
  assert.match(layoutSource, /https:\/\/opensync\.u1n1\.com\//);
  assert.match(layoutSource, /const appVersion = import\.meta\.env\.PACKAGE_VERSION/);
  assert.match(layoutSource, /v\{appVersion\}/);
  assert.match(layoutSource, /<div className="hidden lg:flex items-center gap-1">\s*\{projectLinks\.map/);
});
