import assert from 'node:assert/strict';
import test from 'node:test';
import { buildHomeRouteSearch, readHomeRouteState } from '../src/pages/Home/routeState.ts';

test('readHomeRouteState falls back when task management route state is invalid', () => {
  const state = readHomeRouteState(new URLSearchParams('tab=missing&jobId=-1'));

  assert.equal(state.tab, 'overview');
  assert.equal(state.jobId, null);
});

test('buildHomeRouteSearch stores selected task management tab and job id', () => {
  const params = buildHomeRouteSearch(new URLSearchParams('drawer=open'), {
    tab: 'history',
    jobId: 42,
  });

  assert.equal(params.get('drawer'), 'open');
  assert.equal(params.get('tab'), 'history');
  assert.equal(params.get('jobId'), '42');
});

test('buildHomeRouteSearch removes job id when no task is selected', () => {
  const params = buildHomeRouteSearch(new URLSearchParams('tab=realtime&jobId=42'), {
    tab: 'overview',
    jobId: null,
  });

  assert.equal(params.get('tab'), 'overview');
  assert.equal(params.has('jobId'), false);
});
