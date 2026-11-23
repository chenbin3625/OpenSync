import { lazy, Suspense, useEffect, type ReactNode } from 'react';
import { BrowserRouter, Routes, Route, Navigate, Outlet, useNavigate } from 'react-router-dom';
import { LoadingState } from '../components/common/StatePlaceholder';
import { setUnauthorizedHandler } from '../api/request';
import { useStore } from '../stores/useStore';

const Layout = lazy(() => import('../components/Layout'));
const Login = lazy(() => import('../pages/Login'));
const Home = lazy(() => import('../pages/Home'));
const TaskDetail = lazy(() => import('../pages/Home/TaskDetail'));
const Engine = lazy(() => import('../pages/Engine'));
const Notify = lazy(() => import('../pages/Notify'));
const Setting = lazy(() => import('../pages/Setting'));
const appLoading = <LoadingState title="正在载入 OpenSync" className="min-h-dvh" />;

function UnauthorizedBridge() {
  const navigate = useNavigate();
  useEffect(() => {
    setUnauthorizedHandler(() => {
      navigate('/login', { replace: true });
    });
    return () => {
      setUnauthorizedHandler(null);
    };
  }, [navigate]);
  return null;
}

function AuthGuard({ children }: { children: ReactNode }) {
  const userInfo = useStore((s) => s.userInfo);
  const authChecked = useStore((s) => s.authChecked);
  if (!authChecked) {
    return appLoading;
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
    return appLoading;
  }
  if (userInfo) {
    return <Navigate to="/home" replace />;
  }
  return <>{children}</>;
}

export default function AppRouter() {
  return (
    <BrowserRouter>
      <UnauthorizedBridge />
      <Suspense fallback={appLoading}>
        <Routes>
          <Route path="/login" element={<ReverseAuthGuard><Login /></ReverseAuthGuard>} />
          <Route path="/" element={<Navigate to="/home" replace />} />
          <Route element={
            <AuthGuard>
              <Layout>
                <Suspense fallback={<LoadingState title="正在加载页面" />}>
                  <Outlet />
                </Suspense>
              </Layout>
            </AuthGuard>
          }>
            <Route path="/home" element={<Home />} />
            <Route path="/home/task/detail" element={<TaskDetail />} />
            <Route path="/engine" element={<Engine />} />
            <Route path="/notify" element={<Notify />} />
            <Route path="/setting" element={<Setting />} />
          </Route>
          <Route path="*" element={<Navigate to="/home" replace />} />
        </Routes>
      </Suspense>
    </BrowserRouter>
  );
}
