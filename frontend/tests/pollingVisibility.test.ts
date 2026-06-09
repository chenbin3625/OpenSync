import assert from 'node:assert/strict';
import test from 'node:test';
import { canPollVisibleDocument } from '../src/pages/Home/pollingVisibility.ts';

test('polling stays enabled outside a browser document', () => {
  assert.equal(canPollVisibleDocument(null), true);
  assert.equal(canPollVisibleDocument(undefined), true);
});

test('polling pauses when the browser document is hidden', () => {
  assert.equal(canPollVisibleDocument({ hidden: true }), false);
  assert.equal(canPollVisibleDocument({ hidden: false }), true);
});
