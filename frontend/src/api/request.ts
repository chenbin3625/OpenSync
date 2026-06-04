import axios from 'axios';
import { message } from 'antd';
import { useStore } from '../stores/useStore';

const service = axios.create({
  baseURL: '/svr',
  timeout: 90000,
  headers: {
    'Content-Type': 'application/json;charset=utf-8',
  },
});

// Request interceptor
service.interceptors.request.use(
  (config) => {
    // For GET requests, serialize params into URL
    if (config.method === 'get' && config.params) {
      const searchParams = new URLSearchParams();
      Object.entries(config.params).forEach(([key, val]) => {
        if (val !== undefined && val !== null && val !== '') {
          searchParams.append(key, String(val));
        }
      });
      const qs = searchParams.toString();
      if (qs) {
        config.url = config.url + '?' + qs;
        config.params = undefined;
      }
    }
    return config;
  },
  (error) => Promise.reject(error)
);

// Response interceptor
service.interceptors.response.use(
  (res) => {
    const code = res.data.code || 200;
    const msg = res.data.msg || 'Error';

    if (code === 401) {
      // Clear auth state and redirect to login
      useStore.getState().setUserInfo(null);
      useStore.getState().setAuthChecked(true);
      window.location.hash = '#/login';
      return Promise.reject(new Error(msg));
    } else if (code === 500) {
      message.error(msg);
      return Promise.reject(new Error(msg));
    } else if (code !== 200) {
      message.error(msg);
      return Promise.reject(new Error(msg));
    }
    return res.data;
  },
  (error) => {
    let msg = error.message;
    if (msg === 'Network Error') {
      msg = 'Connection error';
    } else if (msg.includes('timeout')) {
      msg = 'Request timeout';
    }
    if (error.response?.status === 401) {
      useStore.getState().setUserInfo(null);
      useStore.getState().setAuthChecked(true);
      window.location.hash = '#/login';
    }
    message.error(msg);
    return Promise.reject(error);
  }
);

export default service;
