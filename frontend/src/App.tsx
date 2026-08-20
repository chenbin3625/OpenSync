import { useEffect } from 'react';
import { ConfigProvider, App as AntApp, theme } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import AppRouter from './router';
import { getUser } from './api/user';
import { useStore } from './stores/useStore';
import { setMessageInstance } from './api/messageHolder';
import AppErrorBoundary from './components/AppErrorBoundary';
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
          colorBgLayout: isDark ? '#121416' : '#eef2f1',
          colorBgContainer: isDark ? '#1c1f22' : '#ffffff',
          colorBgElevated: isDark ? '#24282c' : '#ffffff',
          colorBorder: isDark ? '#3d4449' : '#d1d9d6',
          colorBorderSecondary: isDark ? '#2d3337' : '#e2e8e6',
          colorText: isDark ? '#f0f4f3' : '#17201e',
          colorTextSecondary: isDark ? '#bac3c0' : '#52605c',
          colorTextTertiary: isDark ? '#8a9692' : '#74817d',
          borderRadius: 8,
          borderRadiusSM: 6,
          borderRadiusLG: 10,
          fontSize: 14,
          fontFamily: `-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif`,
          wireframe: false,
        },
        components: {
          Layout: {
            bodyBg: isDark ? '#121416' : '#eef2f1',
            headerBg: isDark ? 'rgba(28, 31, 34, 0.85)' : 'rgba(255, 255, 255, 0.85)',
          },
          Card: {
            borderRadiusLG: 10,
            headerHeight: 46,
            headerBg: isDark ? '#202428' : '#fafcfb',
          },
          Button: {
            borderRadius: 6,
            controlHeight: 34,
            fontWeight: 500,
          },
          Table: {
            headerBg: isDark ? '#22262b' : '#f3f7f6',
            rowHoverBg: isDark ? '#252a30' : '#f5faf8',
            borderRadius: 8,
          },
          Menu: {
            itemBg: 'transparent',
            itemHoverBg: isDark ? '#272d2b' : '#edf7f5',
            itemSelectedBg: isDark ? '#173d38' : '#d9efeb',
            itemSelectedColor: isDark ? '#5eead4' : '#0f766e',
            darkItemBg: '#191c20',
            darkItemColor: '#c8cdd2',
            darkItemHoverBg: '#242a2f',
            darkItemSelectedBg: '#174d48',
            darkItemSelectedColor: '#ccfbf1',
            itemBorderRadius: 6,
          },
          Tabs: {
            horizontalItemGutter: 24,
            titleFontSize: 14,
          },
          Drawer: {
            footerPaddingBlock: 12,
            footerPaddingInline: 16,
          },
          Modal: {
            borderRadiusLG: 10,
          },
          Tooltip: {
            borderRadius: 6,
          },
          Tag: {
            borderRadiusSM: 4,
          },
        },
      }}
    >
      <AntApp>
        <MessageInitializer />
        <AppErrorBoundary>
          <AppRouter />
        </AppErrorBoundary>
      </AntApp>
    </ConfigProvider>
  );
}

export default App;
