import { lazy, Suspense, type ReactNode } from 'react';
import { HashRouter, Routes, Route, Navigate } from 'react-router-dom';
import { useStore } from '../stores/useStore';
import Layout from '../components/Layout';

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
    return null;
  }
  if (!userInfo) {
    return <Navigate to="/login" replace />;
  }
  return <>{children}</>;
}

export default function AppRouter() {
  return (
    <HashRouter>
      <Suspense fallback={null}>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route path="/" element={<Navigate to="/home" replace />} />
          <Route path="/home" element={<AuthGuard><Layout><Home /></Layout></AuthGuard>} />
          <Route path="/home/task/detail" element={<AuthGuard><Layout><TaskDetail /></Layout></AuthGuard>} />
          <Route path="/engine" element={<AuthGuard><Layout><Engine /></Layout></AuthGuard>} />
          <Route path="/notify" element={<AuthGuard><Layout><Notify /></Layout></AuthGuard>} />
          <Route path="/setting" element={<AuthGuard><Layout><Setting /></Layout></AuthGuard>} />
          <Route path="*" element={<Navigate to="/home" replace />} />
        </Routes>
      </Suspense>
    </HashRouter>
  );
}
