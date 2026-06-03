import type React from 'react';
import { HashRouter, Routes, Route, Navigate } from 'react-router-dom';
import { useStore } from '../stores/useStore';
import Layout from '../components/Layout';
import Login from '../pages/Login';
import Home from '../pages/Home';
import TaskDetail from '../pages/Home/TaskDetail';
import Engine from '../pages/Engine';
import Notify from '../pages/Notify';
import Setting from '../pages/Setting';

function AuthGuard({ children }: { children: React.ReactNode }) {
  const userInfo = useStore((s) => s.userInfo);
  if (!userInfo) {
    return <Navigate to="/login" replace />;
  }
  return <>{children}</>;
}

export default function AppRouter() {
  return (
    <HashRouter>
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
    </HashRouter>
  );
}
