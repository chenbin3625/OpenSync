import axios from 'axios';
import type { AxiosError, AxiosRequestConfig } from 'axios';
import { useStore } from '../stores/useStore';
import { getMessageInstance } from './messageHolder';

declare module 'axios' {
  interface AxiosRequestConfig {
    silent?: boolean;
  }
}

type ApiEnvelope = {
  code: number;
  msg: string;
  data?: unknown;
};

const serializeParams = (params: Record<string, unknown>) => {
  const searchParams = new URLSearchParams();
  Object.entries(params).forEach(([key, val]) => {
    if (val === undefined || val === null || val === '') return;
    if (Array.isArray(val)) {
      val.forEach((item) => {
        if (item !== undefined && item !== null && item !== '') {
          searchParams.append(key, String(item));
        }
      });
      return;
    }
    searchParams.append(key, String(val));
  });
  return searchParams.toString();
};

const isApiEnvelope = (data: unknown): data is ApiEnvelope => {
  if (!data || typeof data !== 'object' || Array.isArray(data)) return false;
  const envelope = data as Partial<ApiEnvelope>;
  return typeof envelope.code === 'number' && typeof envelope.msg === 'string';
};

let unauthorizedHandler: (() => void) | null = null;
let redirectingToLogin = false;

export const setUnauthorizedHandler = (handler: (() => void) | null) => {
  unauthorizedHandler = handler;
  if (!handler) {
    redirectingToLogin = false;
  }
};

const redirectToLogin = () => {
  const state = useStore.getState();
  const wasChecked = state.authChecked;
  state.setUserInfo(null);
  state.setAuthChecked(true);

  if (window.location.pathname === '/login') {
    redirectingToLogin = false;
    return;
  }
  if (redirectingToLogin) {
    return;
  }
  redirectingToLogin = true;

  if (unauthorizedHandler) {
    unauthorizedHandler();
    queueMicrotask(() => {
      redirectingToLogin = false;
    });
    return;
  }

  // Initial auth bootstrap: AuthGuard handles the redirect via React Router
  // once authChecked becomes true, avoiding a duplicate hard reload.
  if (!wasChecked) {
    queueMicrotask(() => {
      redirectingToLogin = false;
    });
    return;
  }

  window.location.replace('/login');
};

const rejectApiEnvelope = (data: ApiEnvelope, fallbackStatus?: number, silent = false) => {
  const code = data.code ?? fallbackStatus ?? 200;
  const msg = data.msg || 'Error';

  if (code === 401 || fallbackStatus === 401) {
    redirectToLogin();
    return Promise.reject(new Error(msg));
  }
  if (code !== 200) {
    if (!silent) getMessageInstance()?.error(msg);
    return Promise.reject(new Error(msg));
  }
  return null;
};

const service = axios.create({
  baseURL: '/svr',
  timeout: 90000,
  headers: {
    'Content-Type': 'application/json;charset=utf-8',
  },
  paramsSerializer: {
    serialize: serializeParams,
  },
});

// Response interceptor
service.interceptors.response.use(
  (res) => {
    if (!isApiEnvelope(res.data)) {
      const msg = 'Invalid server response';
      if (!res.config.silent) {
        getMessageInstance()?.error(msg);
      }
      return Promise.reject(new Error(msg));
    }
    const rejection = rejectApiEnvelope(res.data, res.status, res.config.silent);
    if (rejection) return rejection;
    return res.data as unknown as typeof res;
  },
  (error: AxiosError) => {
    if (axios.isCancel(error) || error?.code === 'ERR_CANCELED' || error?.name === 'CanceledError') {
      return Promise.reject(error);
    }

    const config = error.config as AxiosRequestConfig | undefined;
    const silent = !!config?.silent;
    const status = error.response?.status;
    const data = error.response?.data;
    if (isApiEnvelope(data)) {
      const rejection = rejectApiEnvelope(data, status, silent);
      if (rejection) return rejection;
    }

    let msg = typeof error.message === 'string' ? error.message : 'Request failed';
    if (msg === 'Network Error') {
      msg = 'Connection error';
    } else if (msg.includes('timeout')) {
      msg = 'Request timeout';
    }
    if (status === 401) {
      redirectToLogin();
    } else if (!silent) {
      getMessageInstance()?.error(msg);
    }
    return Promise.reject(error);
  }
);

export default service;

export type RequestOptions = {
  signal?: AbortSignal;
  silent?: boolean;
};

export const withRequestOptions = (params: Record<string, unknown>, options?: RequestOptions) => ({
  params,
  ...(options?.signal ? { signal: options.signal } : {}),
  ...(options?.silent ? { silent: true } : {}),
});

export const POLL_INTERVAL_MS = 3000;
