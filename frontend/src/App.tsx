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
        token: {
          colorPrimary: '#0f766e',
          colorSuccess: '#16a34a',
          colorWarning: '#d97706',
          colorError: '#dc2626',
          colorInfo: '#2563eb',
          borderRadius: 8,
          fontSize: 14,
          wireframe: false,
        },
        components: {
          Layout: {
            bodyBg: themeMode === 'dark' ? '#0f1417' : '#eef3f2',
            headerBg: themeMode === 'dark' ? '#11181c' : '#fbfdfc',
          },
          Card: {
            borderRadiusLG: 8,
            headerHeight: 44,
          },
          Button: {
            borderRadius: 6,
            controlHeight: 34,
          },
          Table: {
            headerBg: themeMode === 'dark' ? '#172126' : '#f4f8f7',
            rowHoverBg: themeMode === 'dark' ? '#172126' : '#f7fbfa',
          },
          Tabs: {
            horizontalItemGutter: 26,
          },
          Drawer: {
            footerPaddingBlock: 12,
            footerPaddingInline: 16,
          },
        },
      }}
    >
      <AntApp>
        <AppRouter />
      </AntApp>
    </ConfigProvider>
  );
}

export default App;
