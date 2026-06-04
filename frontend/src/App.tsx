import { useEffect } from 'react';
import { ConfigProvider, App as AntApp, theme } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import AppRouter from './router';
import { getUser } from './api/user';
import { useStore } from './stores/useStore';
import './index.css';

function App() {
  const themeMode = useStore((s) => s.theme);
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
    <ConfigProvider
      locale={zhCN}
      theme={{
        cssVar: { key: 'openlist-sync' },
        algorithm: themeMode === 'dark' ? theme.darkAlgorithm : theme.defaultAlgorithm,
      }}
    >
      <AntApp>
        <AppRouter />
      </AntApp>
    </ConfigProvider>
  );
}

export default App;
