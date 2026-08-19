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
  if (!data || typeof data !== 'object') return false;
  const envelope = data as Partial<ApiEnvelope>;
  return typeof envelope.code === 'number' && typeof envelope.msg === 'string';
};

const redirectToLogin = () => {
  useStore.getState().setUserInfo(null);
  useStore.getState().setAuthChecked(true);
  if (window.location.pathname !== '/login') {
    window.location.replace('/login');
  }
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
    if (isApiEnvelope(res.data)) {
      const rejection = rejectApiEnvelope(res.data, res.status, res.config.silent);
      if (rejection) return rejection;
    }
    return res.data;
  },
  (error: AxiosError) => {
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
