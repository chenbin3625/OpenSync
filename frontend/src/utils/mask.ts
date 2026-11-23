/**
 * 敏感信息脱敏工具函数
 */

const SENSITIVE_QUERY_KEYWORDS = ['token', 'key', 'secret', 'pass', 'pwd', 'auth', 'sign', 'sig'];
const PATH_SECRET_MIN_LENGTH = 12;

export function maskSecret(value?: string | null): string {
  if (!value) return '—';
  if (value.includes('****')) return value;
  if (value.length <= 4) return '****';
  return `****${value.slice(-4)}`;
}

const isSensitiveQueryKey = (key: string): boolean => {
  const lowerKey = key.toLowerCase();
  return SENSITIVE_QUERY_KEYWORDS.some((keyword) => lowerKey.includes(keyword));
};

export function maskWebhookUrl(value?: string | null): string {
  if (!value) return '—';
  try {
    const url = new URL(value);
    let masked = false;

    if (url.username || url.password) {
      url.username = '****';
      url.password = '';
      masked = true;
    }

    const entries = Array.from(url.searchParams.entries());
    if (entries.length > 0) {
      const nextParams = new URLSearchParams();
      let queryMasked = false;
      for (const [key, paramValue] of entries) {
        if (isSensitiveQueryKey(key)) {
          nextParams.append(key, maskSecret(paramValue));
          queryMasked = true;
          masked = true;
        } else {
          nextParams.append(key, paramValue);
        }
      }
      if (queryMasked) {
        url.search = nextParams.toString();
      }
    }

    const parts = url.pathname.split('/').filter(Boolean);
    if (parts.length > 0) {
      const last = parts[parts.length - 1];
      if (last.includes('****')) {
        masked = true;
      } else if (last.length >= PATH_SECRET_MIN_LENGTH) {
        parts[parts.length - 1] = maskSecret(last);
        url.pathname = `/${parts.join('/')}`;
        masked = true;
      }
    }

    if (!masked && url.search) {
      url.search = '?****';
    }

    return url.toString().replace(/%2A/gi, '*');
  } catch {
    return maskSecret(value);
  }
}
