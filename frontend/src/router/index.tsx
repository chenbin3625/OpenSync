import { lazy, Suspense, type ReactNode } from 'react';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { useStore } from '../stores/useStore';

const Layout = lazy(() => import('../components/Layout'));
const Login = lazy(() => import('../pages/Login'));
const Home = lazy(() => import('../pages/Home'));
const TaskDetail = lazy(() => import('../pages/Home/TaskDetail'));
const Engine = lazy(() => import('../pages/Engine'));
const Notify = lazy(() => import('../pages/Notify'));
const Setting = lazy(() => import('../pages/Setting'));

function AuthGuard({ children }: { children: ReactNode }) {
  const userInfo = useStore((s) => s.userInfo);
  const authChecked = useStore((s) => s.authChecked);
  if (!authChecked) {
    return <div style={{ padding: 32, textAlign: 'center' }}>正在检查登录状态…</div>;
  }
  if (!userInfo) {
    return <Navigate to="/login" replace />;
  }
  return <>{children}</>;
}

function ReverseAuthGuard({ children }: { children: ReactNode }) {
  const userInfo = useStore((s) => s.userInfo);
  const authChecked = useStore((s) => s.authChecked);
  if (!authChecked) {
    return <div style={{ padding: 32, textAlign: 'center' }}>正在检查登录状态…</div>;
  }
  if (userInfo) {
    return <Navigate to="/home" replace />;
  }
  return <>{children}</>;
}

export default function AppRouter() {
  return (
    <BrowserRouter>
      <Suspense fallback={<div style={{ padding: 32, textAlign: 'center' }}>页面加载中…</div>}>
        <Routes>
          <Route path="/login" element={<ReverseAuthGuard><Login /></ReverseAuthGuard>} />
          <Route path="/" element={<Navigate to="/home" replace />} />
          <Route path="/home" element={<AuthGuard><Layout><Home /></Layout></AuthGuard>} />
          <Route path="/home/task/detail" element={<AuthGuard><Layout><TaskDetail /></Layout></AuthGuard>} />
          <Route path="/engine" element={<AuthGuard><Layout><Engine /></Layout></AuthGuard>} />
          <Route path="/notify" element={<AuthGuard><Layout><Notify /></Layout></AuthGuard>} />
          <Route path="/setting" element={<AuthGuard><Layout><Setting /></Layout></AuthGuard>} />
          <Route path="*" element={<Navigate to="/home" replace />} />
        </Routes>
      </Suspense>
    </BrowserRouter>
  );
}
