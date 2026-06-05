import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const layoutSource = readFileSync(new URL('../src/components/Layout/index.tsx', import.meta.url), 'utf8');

test('primary app menu is rendered as a top horizontal navigation', () => {
  assert.match(layoutSource, /className="app-top-nav"/);
  assert.match(layoutSource, /mode="horizontal"/);
  assert.doesNotMatch(layoutSource, /<Sider\b|<\/Sider>/);
});
