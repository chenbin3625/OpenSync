import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { User, Lock, Key, ShieldCheck, AlertCircle } from 'lucide-react';
import { getInitStatus, initializeUser, login, resetPwd } from '../../api/user';
import { useStore } from '../../stores/useStore';
import { Button } from '../../components/ui/button';
import { Input } from '../../components/ui/input';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '../../components/ui/dialog';
import { toast } from '../../components/ui/toaster';

export default function Login() {
  const navigate = useNavigate();
  const { setUserInfo, setAuthChecked } = useStore();
  const [loading, setLoading] = useState(false);
  const [checkingInit, setCheckingInit] = useState(true);
  const [initialized, setInitialized] = useState(true);

  // 表单状态
  const [userName, setUserName] = useState('');
  const [passwd, setPasswd] = useState('');
  const [confirmPasswd, setConfirmPasswd] = useState('');
  const [formError, setFormError] = useState('');

  // 恢复密钥弹窗展示
  const [recoveryModalOpen, setRecoveryModalOpen] = useState(false);
  const [newRecoveryKey, setNewRecoveryKey] = useState('');

  // 重置密码弹窗
  const [resetModalOpen, setResetModalOpen] = useState(false);
  const [resetUserName, setResetUserName] = useState('');
  const [resetRecoveryKey, setResetRecoveryKey] = useState('');
  const [resetNewPasswd, setResetNewPasswd] = useState('');
  const [resetConfirmPasswd, setResetConfirmPasswd] = useState('');
  const [resetLoading, setResetLoading] = useState(false);
  const [resetError, setResetError] = useState('');

  useEffect(() => {
    let active = true;
    getInitStatus()
      .then((res) => {
        if (!active) return;
        setInitialized(res.data.initialized);
      })
      .catch((err) => {
        console.error('getInitStatus error', err);
      })
      .finally(() => {
        if (active) setCheckingInit(false);
      });
    return () => {
      active = false;
    };
  }, []);

  const handleLoginSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setFormError('');

    if (!userName.trim()) {
      setFormError('请输入用户名');
      return;
    }
    if (!passwd) {
      setFormError('请输入密码');
      return;
    }

    if (!initialized) {
      if (passwd !== confirmPasswd) {
        setFormError('两次输入的密码不一致');
        return;
      }
    }

    setLoading(true);
    try {
      if (initialized) {
        const res = await login({ userName, passwd });
        setUserInfo(res.data);
        setAuthChecked(true);
        toast.success('登录成功');
        navigate('/home');
        return;
      }

      // 首次初始化
      const res = await initializeUser({ userName: userName, passwd: passwd });
      const { recoveryKey, ...info } = res.data;
      setUserInfo(info);
      setAuthChecked(true);
      setNewRecoveryKey(recoveryKey);
      setRecoveryModalOpen(true);
    } catch (err) {
      console.error('auth error', err);
    } finally {
      setLoading(false);
    }
  };

  const handleResetSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setResetError('');

    if (!resetUserName.trim()) {
      setResetError('请输入用户名');
      return;
    }
    if (!resetRecoveryKey.trim()) {
      setResetError('请输入恢复密钥');
      return;
    }
    if (!resetNewPasswd) {
      setResetError('请输入新密码');
      return;
    }
    if (resetNewPasswd !== resetConfirmPasswd) {
      setResetError('两次输入的密码不一致');
      return;
    }

    setResetLoading(true);
    try {
      const res = await resetPwd({
        userName: resetUserName,
        recoveryKey: resetRecoveryKey,
        passwd: resetNewPasswd,
      });
      setNewRecoveryKey(res.data);
      setResetModalOpen(false);
      setRecoveryModalOpen(true);
      toast.success('密码重置成功');
    } catch (err) {
      console.error('reset password error', err);
    } finally {
      setResetLoading(false);
    }
  };

  if (checkingInit) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-slate-50">
        <div className="animate-spin h-6 w-6 border-2 border-teal-600 border-t-transparent rounded-full" />
      </div>
    );
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-slate-50 to-slate-100 px-4 py-12">
      <div className="w-full max-w-md bg-white rounded-xl shadow-lg border border-slate-200/80 p-8 space-y-6 overflow-hidden">
        {/* Logo 与标题 */}
        <div className="text-center space-y-2">
          <div className="inline-flex p-3 rounded-2xl bg-teal-50 border border-teal-100 shadow-xs mb-1">
            <img src="/favicon.svg" alt="OpenSync" className="h-10 w-10" />
          </div>
          <h1 className="text-2xl font-bold tracking-tight text-slate-900">OpenSync</h1>
          <p className="text-sm text-slate-500">
            {initialized ? 'AList / OpenList 自动化同步调度系统' : '创建管理员账号'}
          </p>
        </div>

        {formError && (
          <div className="flex items-center gap-2 p-3 rounded-lg bg-rose-50 border border-rose-200 text-rose-700 text-xs font-medium animate-in fade-in-0">
            <AlertCircle className="h-4 w-4 shrink-0" />
            <span>{formError}</span>
          </div>
        )}

        <form onSubmit={handleLoginSubmit} className="space-y-4">
          <div className="space-y-1.5">
            <label className="text-xs font-medium text-slate-700 block">用户名</label>
            <Input
              type="text"
              name="userName"
              placeholder="请输入用户名"
              value={userName}
              onChange={(e) => setUserName(e.target.value)}
              prefixIcon={<User className="h-4 w-4" />}
              autoComplete="username"
              required
            />
          </div>

          <div className="space-y-1.5">
            <label className="text-xs font-medium text-slate-700 block">密码</label>
            <Input
              type="password"
              name="passwd"
              placeholder="请输入密码"
              value={passwd}
              onChange={(e) => setPasswd(e.target.value)}
              prefixIcon={<Lock className="h-4 w-4" />}
              autoComplete={initialized ? 'current-password' : 'new-password'}
              required
            />
          </div>

          {!initialized && (
            <div className="space-y-1.5">
              <label className="text-xs font-medium text-slate-700 block">确认密码</label>
              <Input
                type="password"
                name="confirmPasswd"
                placeholder="请再次输入密码"
                value={confirmPasswd}
                onChange={(e) => setConfirmPasswd(e.target.value)}
                prefixIcon={<Lock className="h-4 w-4" />}
                autoComplete="new-password"
                required
              />
            </div>
          )}

          <Button type="submit" className="w-full mt-2" loading={loading}>
            {initialized ? '登 录' : '创建管理员并初始化'}
          </Button>

          {initialized && (
            <div className="text-center pt-2">
              <button
                type="button"
                onClick={() => {
                  setResetError('');
                  setResetModalOpen(true);
                }}
                className="text-xs text-teal-700 hover:text-teal-800 hover:underline font-medium transition-colors"
              >
                忘记密码？使用恢复密钥找回
              </button>
            </div>
          )}
        </form>
      </div>

      {/* 恢复密钥展示弹窗（仅展示一次，强调必须保存） */}
      <Dialog open={recoveryModalOpen} onOpenChange={setRecoveryModalOpen}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <div className="flex items-center gap-2 text-teal-700 mb-1">
              <ShieldCheck className="h-5 w-5" />
              <DialogTitle>请立即保存恢复密钥</DialogTitle>
            </div>
            <DialogDescription>
              该 24 位恢复密钥仅展示一次。如果忘记管理员密码，必须凭此密钥才能重置，请妥善保存在密码管理器或离线安全处。
            </DialogDescription>
          </DialogHeader>

          <div className="my-2 p-3.5 rounded-lg bg-slate-100 border border-slate-200 text-center font-mono text-base font-bold text-slate-900 tracking-wider select-all break-all">
            {newRecoveryKey}
          </div>

          <DialogFooter>
            <Button
              className="w-full"
              onClick={() => {
                setRecoveryModalOpen(false);
                navigate('/home');
              }}
            >
              我已安全保存，进入系统
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 重置密码弹窗 */}
      <Dialog open={resetModalOpen} onOpenChange={setResetModalOpen}>
        <DialogContent className="max-w-md" title="重置密码">
          <DialogHeader>
            <div className="flex items-center gap-2 text-slate-900 mb-1">
              <Key className="h-5 w-5 text-teal-700" />
              <DialogTitle>重置密码</DialogTitle>
            </div>
            <DialogDescription>
              输入管理员用户名和此前保存的恢复密钥，以设定新的密码。
            </DialogDescription>
          </DialogHeader>

          {resetError && (
            <div className="p-2.5 rounded bg-rose-50 border border-rose-200 text-rose-700 text-xs font-medium">
              {resetError}
            </div>
          )}

          <form onSubmit={handleResetSubmit} className="space-y-3.5 my-2">
            <div className="space-y-1">
              <label className="text-xs font-medium text-slate-700">用户名</label>
              <Input
                placeholder="管理员用户名"
                value={resetUserName}
                onChange={(e) => setResetUserName(e.target.value)}
                prefixIcon={<User className="h-4 w-4" />}
                required
              />
            </div>

            <div className="space-y-1">
              <label className="text-xs font-medium text-slate-700">恢复密钥</label>
              <Input
                name="recoveryKey"
                placeholder="恢复密钥"
                value={resetRecoveryKey}
                onChange={(e) => setResetRecoveryKey(e.target.value)}
                prefixIcon={<Key className="h-4 w-4" />}
                required
              />
            </div>

            <div className="space-y-1">
              <label className="text-xs font-medium text-slate-700">新密码</label>
              <Input
                type="password"
                placeholder="设置新密码"
                value={resetNewPasswd}
                onChange={(e) => setResetNewPasswd(e.target.value)}
                prefixIcon={<Lock className="h-4 w-4" />}
                required
              />
            </div>

            <div className="space-y-1">
              <label className="text-xs font-medium text-slate-700">确认新密码</label>
              <Input
                type="password"
                name="confirmPasswd"
                placeholder="再次输入新密码"
                value={resetConfirmPasswd}
                onChange={(e) => setResetConfirmPasswd(e.target.value)}
                prefixIcon={<Lock className="h-4 w-4" />}
                required
              />
            </div>

            <DialogFooter className="pt-2">
              <Button type="button" variant="outline" onClick={() => setResetModalOpen(false)}>
                取消
              </Button>
              <Button type="submit" loading={resetLoading}>
                确认重置
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
