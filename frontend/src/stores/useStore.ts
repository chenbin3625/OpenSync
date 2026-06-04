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
    return data.vuex_theme || 'dark';
  } catch {
    return 'dark';
  }
};

const getInitialUser = (): UserInfo | null => {
  try {
    const data = JSON.parse(localStorage.getItem('lifeData') || '{}');
    return data.vuex_userInfo || null;
  } catch {
    return null;
  }
};

const saveLifeData = (key: string, value: unknown) => {
  try {
    const data = JSON.parse(localStorage.getItem('lifeData') || '{}');
    data[key] = value;
    localStorage.setItem('lifeData', JSON.stringify(data));
  } catch { /* ignore */ }
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
