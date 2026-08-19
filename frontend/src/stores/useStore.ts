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

const persistState = (newKey: string, oldKey: string, value: unknown) => {
  try {
    if (value === null || value === undefined) {
      localStorage.removeItem(newKey);
    } else if (typeof value === 'string') {
      localStorage.setItem(newKey, value);
    } else {
      localStorage.setItem(newKey, JSON.stringify(value));
    }
    const data = JSON.parse(localStorage.getItem('lifeData') || '{}');
    data[oldKey] = value;
    localStorage.setItem('lifeData', JSON.stringify(data));
  } catch (err) {
    console.error('failed to persist local state', err);
  }
};

export const useStore = create<AppState>((set) => ({
  userInfo: getInitialUser(),
  authChecked: false,
  theme: getInitialTheme(),
  setUserInfo: (user) => {
    persistState('opensync_userInfo', 'vuex_userInfo', user);
    set({ userInfo: user });
  },
  setAuthChecked: (checked) => set({ authChecked: checked }),
  setTheme: (theme) => {
    persistState('opensync_theme', 'vuex_theme', theme);
    set({ theme });
  },
}));
