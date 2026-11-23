import assert from 'node:assert/strict';
import test from 'node:test';
import { maskSecret, maskWebhookUrl } from '../src/utils/mask.ts';

test('maskSecret handles empty and short inputs', () => {
  assert.equal(maskSecret(''), '—');
  assert.equal(maskSecret(null), '—');
  assert.equal(maskSecret(undefined), '—');
  assert.equal(maskSecret('abc'), '****');
  assert.equal(maskSecret('1234'), '****');
  assert.equal(maskSecret('1234567890'), '****7890');
  assert.equal(maskSecret('already****masked'), 'already****masked');
});

test('maskWebhookUrl masks query token and path secret', () => {
  assert.equal(maskWebhookUrl(''), '—');
  assert.equal(maskWebhookUrl(null), '—');
  const urlWithToken = 'https://api.telegram.org/bot1234567890abcdef123456/sendMessage?token=mysecrettoken123';
  const masked = maskWebhookUrl(urlWithToken);
  assert.match(masked, /\*\*\*\*/);
  assert.doesNotMatch(masked, /mysecrettoken123/);
});

test('maskWebhookUrl masks remaining secrets even when part of URL already contains ****', () => {
  const maskedPathUnmaskedQuery = maskWebhookUrl(
    'https://example.com/hook/****abcd?token=plainsecret9876&sign=anothersecret5432',
  );
  assert.equal(
    maskedPathUnmaskedQuery,
    'https://example.com/hook/****abcd?token=****9876&sign=****5432',
  );

  const maskedQueryUnmaskedPath = maskWebhookUrl(
    'https://example.com/hook/abcdefgh1234?token=****5678',
  );
  assert.equal(
    maskedQueryUnmaskedPath,
    'https://example.com/hook/****1234?token=****5678',
  );

  const maskedPathUnmaskedUserinfo = maskWebhookUrl(
    'https://admin:supersecret@example.com/hook/****abcd?channel=ops',
  );
  assert.equal(
    maskedPathUnmaskedUserinfo,
    'https://****@example.com/hook/****abcd?channel=ops',
  );
  assert.doesNotMatch(maskedPathUnmaskedUserinfo, /admin|supersecret/);
});

test('maskWebhookUrl masks URL user:pass@ userinfo', () => {
  const withUserPass = maskWebhookUrl('https://alice:topsecret@example.com/notify');
  assert.equal(withUserPass, 'https://****@example.com/notify');
  assert.doesNotMatch(withUserPass, /alice|topsecret/);

  const withUserOnly = maskWebhookUrl('https://alice@example.com/notify');
  assert.equal(withUserOnly, 'https://****@example.com/notify');
  assert.doesNotMatch(withUserOnly, /alice/);
});

test('maskWebhookUrl masks multiple sensitive query params while preserving non-sensitive params', () => {
  const masked = maskWebhookUrl(
    'https://oapi.dingtalk.com/robot/send?access_token=tok11112222&sign=sig33334444&apiKey=key55556666&pwd=ab12&channel=alerts',
  );
  assert.equal(
    masked,
    'https://oapi.dingtalk.com/robot/send?access_token=****2222&sign=****4444&apiKey=****6666&pwd=****&channel=alerts',
  );

  const duplicateSensitive = maskWebhookUrl(
    'https://example.com/hook?token=first1111&token=second2222&sig=third3333',
  );
  assert.equal(
    duplicateSensitive,
    'https://example.com/hook?token=****1111&token=****2222&sig=****3333',
  );

  // Fallback masks entire query string when no sensitive key, path secret, or userinfo matched
  assert.equal(
    maskWebhookUrl('https://example.com/hook?foo=bar&baz=1'),
    'https://example.com/hook?****',
  );
});

test('maskWebhookUrl enforces path segment length threshold of 12', () => {
  // 11 characters: below threshold, preserved
  assert.equal(
    maskWebhookUrl('https://example.com/hooks/12345678901'),
    'https://example.com/hooks/12345678901',
  );

  // 12 characters: at threshold, masked
  assert.equal(
    maskWebhookUrl('https://example.com/hooks/123456789012'),
    'https://example.com/hooks/****9012',
  );

  // Already-masked path segment counts as masked so non-sensitive query stays visible
  assert.equal(
    maskWebhookUrl('https://example.com/hooks/****9012?env=prod'),
    'https://example.com/hooks/****9012?env=prod',
  );

  // Invalid URL falls back to maskSecret
  assert.equal(maskWebhookUrl('not-a-valid-url-secret9999'), '****9999');
});
