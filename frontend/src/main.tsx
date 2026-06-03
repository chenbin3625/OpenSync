import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { ConfigProvider, App, theme } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import AppRouter from './router';
import { useStore } from './stores/useStore';
import './index.css';

function ThemedApp() {
  const themeMode = useStore((s) => s.theme);
  return (
    <ConfigProvider
      locale={zhCN}
      theme={{
        cssVar: { key: 'openlist-sync' },
        algorithm: themeMode === 'dark' ? theme.darkAlgorithm : theme.defaultAlgorithm,
      }}
    >
      <App>
        <AppRouter />
      </App>
    </ConfigProvider>
  );
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ThemedApp />
  </StrictMode>,
);
