import { useEffect } from 'react';
import { ConfigProvider, App as AntApp, theme } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import AppRouter from './router';
import { getUser } from './api/user';
import { useStore } from './stores/useStore';
import { setMessageInstance } from './api/messageHolder';
import './index.css';

function MessageInitializer() {
  const { message } = AntApp.useApp();
  useEffect(() => {
    setMessageInstance(message);
  }, [message]);
  return null;
}

function App() {
  const themeMode = useStore((s) => s.theme);
  const isDark = themeMode === 'dark';
  const setUserInfo = useStore((s) => s.setUserInfo);
  const setAuthChecked = useStore((s) => s.setAuthChecked);

  // 让原生控件（滚动条、表单控件）跟随主题而非系统偏好。
  useEffect(() => {
    document.documentElement.style.colorScheme = isDark ? 'dark' : 'light';
  }, [isDark]);

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
        algorithm: isDark ? theme.darkAlgorithm : theme.defaultAlgorithm,
        token: {
          colorPrimary: isDark ? '#2dd4bf' : '#0f766e',
          colorSuccess: isDark ? '#4ade80' : '#16a34a',
          colorWarning: isDark ? '#fbbf24' : '#d97706',
          colorError: isDark ? '#fb7185' : '#dc2626',
          colorInfo: isDark ? '#60a5fa' : '#2563eb',
          colorBgBase: isDark ? '#17191c' : '#f6f8f7',
          colorTextBase: isDark ? '#f3f4f6' : '#17201e',
          colorBgLayout: isDark ? '#151817' : '#eef3f2',
          colorBgContainer: isDark ? '#202322' : '#ffffff',
          colorBgElevated: isDark ? '#292d2b' : '#ffffff',
          colorBorder: isDark ? '#48504d' : '#cfd8d5',
          colorBorderSecondary: isDark ? '#353b39' : '#dde4e2',
          colorText: isDark ? '#eef2f1' : '#17201e',
          colorTextSecondary: isDark ? '#bcc4c1' : '#52605c',
          colorTextTertiary: isDark ? '#8f9a96' : '#74817d',
          borderRadius: 8,
          fontSize: 14,
          wireframe: false,
        },
        components: {
          Layout: {
            bodyBg: isDark ? '#111315' : '#eef3f2',
            headerBg: isDark ? '#191c20' : '#ffffff',
          },
          Card: {
            borderRadiusLG: 8,
            headerHeight: 44,
            headerBg: isDark ? '#1d2024' : '#fbfcfc',
          },
          Button: {
            borderRadius: 6,
            controlHeight: 34,
          },
          Table: {
            headerBg: isDark ? '#23272c' : '#f2f6f5',
            rowHoverBg: isDark ? '#242a2f' : '#f5faf8',
          },
          Menu: {
            itemBg: 'transparent',
            itemHoverBg: isDark ? '#2a302e' : '#edf7f5',
            itemSelectedBg: isDark ? '#1c4e47' : '#d9efeb',
            itemSelectedColor: isDark ? '#5eead4' : '#0f766e',
            darkItemBg: '#191c20',
            darkItemColor: '#c8cdd2',
            darkItemHoverBg: '#242a2f',
            darkItemSelectedBg: '#174d48',
            darkItemSelectedColor: '#ccfbf1',
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
        <MessageInitializer />
        <AppRouter />
      </AntApp>
    </ConfigProvider>
  );
}

export default App;
