import { useEffect } from 'react';
import AppRouter from './router';
import { getUser } from './api/user';
import { useStore } from './stores/useStore';
import { Toaster } from './components/ui/toaster';
import './index.css';

// OpenSync 翡翠青核心品牌色与设计规范
export const themeConfig = {
  colorPrimary: '#0f766e',
  borderRadius: 8,
};

function App() {
  const setUserInfo = useStore((s) => s.setUserInfo);
  const setAuthChecked = useStore((s) => s.setAuthChecked);

  useEffect(() => {
    let alive = true;
    getUser()
      .then((res) => {
        if (alive) setUserInfo(res.data);
      })
      .catch(() => {
        if (alive) setUserInfo(null);
      })
      .finally(() => {
        if (alive) setAuthChecked(true);
      });
    return () => {
      alive = false;
    };
  }, [setAuthChecked, setUserInfo]);

  return (
    <>
      <AppRouter />
      <Toaster />
    </>
  );
}

export default App;

