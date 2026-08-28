import { create } from 'zustand';
import type { UserInfo } from '../types';

interface AppState {
  userInfo: UserInfo | null;
  authChecked: boolean;
  theme: 'dark' | 'light';
  setUserInfo: (user: UserInfo | null) => void;
  setAuthChecked: (checked: boolean) => void;
  setTheme: (theme: 'dark' | 'light') => void;
}

const getInitialTheme = (): 'dark' | 'light' => {
  try {
    const directTheme = localStorage.getItem('opensync_theme');
    if (directTheme === 'light' || directTheme === 'dark') {
      return directTheme;
    }
    const data = JSON.parse(localStorage.getItem('lifeData') || '{}');
    return data.vuex_theme === 'light' || data.vuex_theme === 'dark' ? data.vuex_theme : 'dark';
  } catch (err) {
    console.error('failed to read theme from localStorage', err);
    return 'dark';
  }
};

const isUserInfo = (value: unknown): value is UserInfo => {
  if (!value || typeof value !== 'object') return false;
  const user = value as Partial<UserInfo>;
  return typeof user.id === 'number' &&
    typeof user.userName === 'string' &&
    typeof user.createTime === 'number';
};

const getInitialUser = (): UserInfo | null => {
  try {
    const directUser = JSON.parse(localStorage.getItem('opensync_userInfo') || 'null');
    if (isUserInfo(directUser)) {
      return directUser;
    }
    const data = JSON.parse(localStorage.getItem('lifeData') || '{}');
    return isUserInfo(data.vuex_userInfo) ? data.vuex_userInfo : null;
  } catch (err) {
    console.error('failed to read user from localStorage', err);
    return null;
  }
};

// persistState writes the canonical key only. The legacy vuex `lifeData` blob
// is read for backward compatibility above but is no longer written: it used
// to keep stale user info around after logout.
const persistState = (key: string, value: unknown) => {
  try {
    if (value === null || value === undefined) {
      localStorage.removeItem(key);
    } else if (typeof value === 'string') {
      localStorage.setItem(key, value);
    } else {
      localStorage.setItem(key, JSON.stringify(value));
    }
  } catch (err) {
    console.error('failed to persist local state', err);
  }
};

export const useStore = create<AppState>((set) => ({
  userInfo: getInitialUser(),
  authChecked: false,
  theme: getInitialTheme(),
  setUserInfo: (user) => {
    // Reject malformed payloads so a broken response can never be persisted as
    // a "logged in" session.
    const valid = user === null || isUserInfo(user) ? user : null;
    persistState('opensync_userInfo', valid);
    set({ userInfo: valid });
  },
  setAuthChecked: (checked) => set({ authChecked: checked }),
  setTheme: (theme) => {
    persistState('opensync_theme', theme);
    set({ theme });
  },
}));
