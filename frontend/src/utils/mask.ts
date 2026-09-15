/**
 * 敏感信息脱敏工具函数
 */

export function maskSecret(value?: string | null): string {
  if (!value) return '—';
  if (value.includes('****')) return value;
  if (value.length <= 4) return '****';
  return `****${value.slice(-4)}`;
}

export function maskWebhookUrl(value?: string | null): string {
  if (!value) return '—';
  if (value.includes('****')) return value;
  try {
    const url = new URL(value);
    let masked = false;
    url.searchParams.forEach((paramValue, key) => {
      const lowerKey = key.toLowerCase();
      if (lowerKey.includes('token') || lowerKey.includes('key') || lowerKey.includes('secret')) {
        url.searchParams.set(key, maskSecret(paramValue));
        masked = true;
      }
    });
    const parts = url.pathname.split('/').filter(Boolean);
    if (parts.length > 0 && parts[parts.length - 1].length >= 16) {
      parts[parts.length - 1] = maskSecret(parts[parts.length - 1]);
      url.pathname = `/${parts.join('/')}`;
      masked = true;
    }
    if (!masked && url.search) {
      url.search = '?****';
    }
    return url.toString().replace(/%2A/gi, '*');
  } catch {
    return maskSecret(value);
  }
}
