import { create } from 'zustand';
import type { UserInfo } from '../types';

interface AppState {
  userInfo: UserInfo | null;
  authChecked: boolean;
  theme: 'dark' | 'light';
  leftIndex: string;
  setUserInfo: (user: UserInfo | null) => void;
  setAuthChecked: (checked: boolean) => void;
  setTheme: (theme: 'dark' | 'light') => void;
  setLeftIndex: (index: string) => void;
}

const getInitialTheme = (): 'dark' | 'light' => {
  try {
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
    const data = JSON.parse(localStorage.getItem('lifeData') || '{}');
    return isUserInfo(data.vuex_userInfo) ? data.vuex_userInfo : null;
  } catch (err) {
    console.error('failed to read user from localStorage', err);
    return null;
  }
};

const saveLifeData = (key: string, value: unknown) => {
  try {
    const data = JSON.parse(localStorage.getItem('lifeData') || '{}');
    data[key] = value;
    localStorage.setItem('lifeData', JSON.stringify(data));
  } catch (err) {
    console.error('failed to persist local state', err);
  }
};

export const useStore = create<AppState>((set) => ({
  userInfo: getInitialUser(),
  authChecked: false,
  theme: getInitialTheme(),
  leftIndex: '/home',
  setUserInfo: (user) => {
    saveLifeData('vuex_userInfo', user);
    set({ userInfo: user });
  },
  setAuthChecked: (checked) => set({ authChecked: checked }),
  setTheme: (theme) => {
    saveLifeData('vuex_theme', theme);
    set({ theme });
  },
  setLeftIndex: (index) => set({ leftIndex: index }),
}));
