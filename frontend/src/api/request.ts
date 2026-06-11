import axios from 'axios';
import { useStore } from '../stores/useStore';
import { getMessageInstance } from './messageHolder';

type ApiEnvelope = {
  code?: number;
  msg?: string;
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

const isApiEnvelope = (data: unknown): data is ApiEnvelope => (
  !!data && typeof data === 'object' && ('code' in data || 'msg' in data || 'data' in data)
);

const redirectToLogin = () => {
  useStore.getState().setUserInfo(null);
  useStore.getState().setAuthChecked(true);
  window.location.hash = '#/login';
};

const rejectApiEnvelope = (data: ApiEnvelope, fallbackStatus?: number) => {
  const code = data.code ?? fallbackStatus ?? 200;
  const msg = data.msg || 'Error';

  if (code === 401 || fallbackStatus === 401) {
    redirectToLogin();
    return Promise.reject(new Error(msg));
  }
  if (code !== 200) {
    getMessageInstance()?.error(msg);
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
      const rejection = rejectApiEnvelope(res.data, res.status);
      if (rejection) return rejection;
    }
    return res.data;
  },
  (error) => {
    const status = error.response?.status;
    const data = error.response?.data;
    if (isApiEnvelope(data)) {
      const rejection = rejectApiEnvelope(data, status);
      if (rejection) return rejection;
    }

    let msg = error.message;
    if (msg === 'Network Error') {
      msg = 'Connection error';
    } else if (msg.includes('timeout')) {
      msg = 'Request timeout';
    }
    if (status === 401) {
      redirectToLogin();
    } else {
      getMessageInstance()?.error(msg);
    }
    return Promise.reject(error);
  }
);

export default service;
