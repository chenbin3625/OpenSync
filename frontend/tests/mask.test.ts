import assert from 'node:assert/strict';
import test from 'node:test';
import { maskSecret, maskWebhookUrl } from '../src/utils/mask.ts';

test('maskSecret handles empty and short inputs', () => {
  assert.equal(maskSecret(''), '—');
  assert.equal(maskSecret(null), '—');
  assert.equal(maskSecret('abc'), '****');
  assert.equal(maskSecret('1234567890'), '****7890');
  assert.equal(maskSecret('already****masked'), 'already****masked');
});

test('maskWebhookUrl masks query token and path secret', () => {
  assert.equal(maskWebhookUrl(''), '—');
  const urlWithToken = 'https://api.telegram.org/bot1234567890abcdef123456/sendMessage?token=mysecrettoken123';
  const masked = maskWebhookUrl(urlWithToken);
  assert.match(masked, /\*\*\*\*/);
  assert.doesNotMatch(masked, /mysecrettoken123/);
});
