import { useEffect } from 'react';
import { ConfigProvider, App as AntApp } from 'antd';
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
        token: {
          colorPrimary: '#0f766e',
          colorSuccess: '#16a34a',
          colorWarning: '#d97706',
          colorError: '#dc2626',
          colorInfo: '#2563eb',
          colorBgBase: '#f6f8f7',
          colorTextBase: '#17201e',
          colorBgLayout: '#eef2f1',
          colorBgContainer: '#ffffff',
          colorBgElevated: '#ffffff',
          colorBorder: '#d1d9d6',
          colorBorderSecondary: '#e2e8e6',
          colorText: '#17201e',
          colorTextSecondary: '#52605c',
          colorTextTertiary: '#74817d',
          borderRadius: 8,
          borderRadiusSM: 6,
          borderRadiusLG: 10,
          fontSize: 14,
          fontFamily: `-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif`,
          wireframe: false,
        },
        components: {
          Layout: {
            bodyBg: '#eef2f1',
            headerBg: 'rgba(255, 255, 255, 0.85)',
          },
          Card: {
            borderRadiusLG: 10,
            headerHeight: 46,
            headerBg: '#fafcfb',
          },
          Button: {
            borderRadius: 6,
            controlHeight: 34,
            fontWeight: 500,
          },
          Table: {
            headerBg: '#f3f7f6',
            rowHoverBg: '#f5faf8',
            borderRadius: 8,
          },
          Menu: {
            itemBg: 'transparent',
            itemHoverBg: '#edf7f5',
            itemSelectedBg: '#d9efeb',
            itemSelectedColor: '#0f766e',
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
        <AppRouter />
      </AntApp>
    </ConfigProvider>
  );
}

export default App;
