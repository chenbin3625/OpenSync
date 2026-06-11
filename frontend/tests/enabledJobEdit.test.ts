import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const homeSource = readFileSync(new URL('../src/pages/Home/index.tsx', import.meta.url), 'utf8');
const homeOverviewSource = readFileSync(new URL('../src/pages/Home/HomeOverview.tsx', import.meta.url), 'utf8');

test('enabled jobs can still open the edit drawer', () => {
  assert.doesNotMatch(homeOverviewSource, /disabled=\{selectedJob\.enable === 1\}/);
  assert.match(homeOverviewSource, /onClick=\{\(\) => onEdit\(selectedJob\)\}/);
});

test('editing a job tells the user changes apply to the next run', () => {
  assert.match(homeSource, /编辑成功，下次任务生效/);
});
