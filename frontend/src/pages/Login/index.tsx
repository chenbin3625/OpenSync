import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { User, Lock, Key, ShieldCheck, HelpCircle, ChevronDown } from 'lucide-react';
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
import { Field } from '../../components/common/Field';
import { Alert } from '../../components/common/Alert';
import { cn } from '../../lib/utils';
import { icon, surface, text } from '../../lib/styles';

/** 兜底重置命令：等宽整行可选中，便于原样复制到服务器执行 */
function CliCommand({ command, children }: { command: string; children: React.ReactNode }) {
  return (
    <div className="space-y-1">
      <p className={text.muted}>{children}</p>
      <code className="block select-all whitespace-pre-wrap break-words rounded-md border border-line bg-white px-2 py-1.5 font-mono text-2xs text-slate-700">
        {command}
      </code>
    </div>
  );
}

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
  // 恢复密钥丢失时的兜底说明（默认收起，避免干扰正常重置流程）
  const [showRecoveryHelp, setShowRecoveryHelp] = useState(false);

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

  // CLI 兜底命令回填用户已输入的用户名；含非法字符则回落到默认示例
  const safeResetUserName = /^[A-Za-z0-9_.-]+$/.test(resetUserName.trim())
    ? resetUserName.trim()
    : 'admin';

  // 启动态与登录态共用同一背景，避免初始化检查结束时出现背景闪烁
  const pageShell =
    'min-h-screen flex items-center justify-center bg-gradient-to-br from-tint to-line-soft px-4 py-12';

  if (checkingInit) {
    return (
      <div className={pageShell}>
        <div className="animate-spin h-6 w-6 border-2 border-teal-600 border-t-transparent rounded-full" />
      </div>
    );
  }

  return (
    <div className={pageShell}>
      <div className={cn(surface.card, 'w-full max-w-md shadow-lg p-8 space-y-6 overflow-hidden')}>
        {/* Logo 与标题 */}
        <div className="text-center space-y-2">
          <div className={cn(surface.iconTile, 'inline-flex h-16 w-16 mx-auto')}>
            <img src="/favicon.svg" alt="OpenSync" className="h-10 w-10" />
          </div>
          <h1 className={text.pageTitle}>OpenSync</h1>
          <p className={text.pageSubtitle}>
            {initialized ? 'AList / OpenList 自动化同步调度系统' : '创建管理员账号'}
          </p>
        </div>

        {formError && <Alert className="animate-in fade-in-0">{formError}</Alert>}

        <form onSubmit={handleLoginSubmit} className="space-y-4">
          <Field label="用户名" required>
            <Input
              type="text"
              name="userName"
              placeholder="请输入用户名"
              value={userName}
              onChange={(e) => setUserName(e.target.value)}
              prefixIcon={<User className={icon.md} />}
              autoComplete="username"
              required
            />
          </Field>

          <Field label="密码" required>
            <Input
              type="password"
              name="passwd"
              placeholder="请输入密码"
              value={passwd}
              onChange={(e) => setPasswd(e.target.value)}
              prefixIcon={<Lock className={icon.md} />}
              autoComplete={initialized ? 'current-password' : 'new-password'}
              required
            />
          </Field>

          {!initialized && (
            <Field label="确认密码" required>
              <Input
                type="password"
                name="confirmPasswd"
                placeholder="请再次输入密码"
                value={confirmPasswd}
                onChange={(e) => setConfirmPasswd(e.target.value)}
                prefixIcon={<Lock className={icon.md} />}
                autoComplete="new-password"
                required
              />
            </Field>
          )}

          <Button type="submit" className="w-full" loading={loading}>
            {initialized ? '登 录' : '创建管理员并初始化'}
          </Button>

          {initialized && (
            <div className="text-center pt-2">
              <Button
                type="button"
                variant="link"
                size="sm"
                onClick={() => {
                  setResetError('');
                  setResetModalOpen(true);
                }}
              >
                忘记密码？使用恢复密钥找回
              </Button>
            </div>
          )}
        </form>
      </div>

      {/* 恢复密钥展示弹窗（仅展示一次，强调必须保存） */}
      <Dialog open={recoveryModalOpen} onOpenChange={setRecoveryModalOpen}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <div className="flex items-center gap-2 text-slate-900">
              <ShieldCheck className={cn(icon.lg, 'text-teal-700')} />
              <DialogTitle>请立即保存恢复密钥</DialogTitle>
            </div>
            <DialogDescription>
              该 24 位恢复密钥仅展示一次。如果忘记管理员密码，必须凭此密钥才能重置，请妥善保存在密码管理器或离线安全处。
              服务端只保存它的哈希值，之后无法再次查看；一旦连同密码一起丢失，只能登录服务器用 CLI 兜底重置。
            </DialogDescription>
          </DialogHeader>

          <div
            className={cn(
              surface.inset,
              'my-2 text-center font-mono text-base font-bold text-slate-900 tracking-wider select-all break-all'
            )}
          >
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
        {/* 限高与滚动由 DialogContent 的内容区负责：弹窗自己不滚动，
            关闭按钮才不会跟着「找不到恢复密钥？」展开后的长文一起滚出视口 */}
        <DialogContent className="max-w-md">
          <DialogHeader>
            <div className="flex items-center gap-2 text-slate-900">
              <Key className={cn(icon.lg, 'text-teal-700')} />
              <DialogTitle>重置密码</DialogTitle>
            </div>
            <DialogDescription>
              输入管理员用户名和此前保存的 24 位恢复密钥，以设定新的密码。
              重置成功后旧密钥即刻失效，会生成一份新的恢复密钥。
            </DialogDescription>
          </DialogHeader>

          {resetError && <Alert>{resetError}</Alert>}

          <form onSubmit={handleResetSubmit} className="space-y-3.5 my-2">
            <Field label="用户名" required>
              <Input
                placeholder="管理员用户名"
                value={resetUserName}
                onChange={(e) => setResetUserName(e.target.value)}
                prefixIcon={<User className={icon.md} />}
                required
              />
            </Field>

            <Field
              label="恢复密钥"
              required
              hint="24 位密钥，创建管理员或上次重置成功时一次性展示过"
            >
              <Input
                name="recoveryKey"
                placeholder="恢复密钥"
                value={resetRecoveryKey}
                onChange={(e) => setResetRecoveryKey(e.target.value)}
                prefixIcon={<Key className={icon.md} />}
                required
              />
            </Field>

            {/* 恢复密钥的获取方式：正常来源 + 丢失后的服务器兜底 */}
            <div className="rounded-lg border border-line bg-tint overflow-hidden">
              <button
                type="button"
                onClick={() => setShowRecoveryHelp((prev) => !prev)}
                aria-expanded={showRecoveryHelp}
                className="flex w-full items-center gap-1.5 px-3 py-2 text-xs font-medium text-slate-600 hover:text-slate-900"
              >
                <HelpCircle className={cn(icon.sm, 'shrink-0')} />
                <span>找不到恢复密钥？</span>
                <ChevronDown
                  className={cn(
                    icon.sm,
                    'ml-auto shrink-0 transition-transform',
                    showRecoveryHelp && 'rotate-180'
                  )}
                />
              </button>

              {showRecoveryHelp && (
                <div className={cn('space-y-2.5 border-t border-line px-3 py-2.5', text.muted)}>
                  <p>
                    恢复密钥只在创建管理员或上次重置成功时展示一次，服务端仅保存它的哈希值，
                    <span className="text-slate-600 font-medium">无法再次查看或找回</span>
                    ，只能按当初保存的记录（密码管理器、离线备份）逐位输入。
                  </p>
                  <p>
                    若确实已丢失，登录部署 OpenSync 的服务器执行兜底重置即可，
                    命令会直接输出新的密码与新的恢复密钥：
                  </p>
                  <div className="space-y-2">
                    <CliCommand
                      command={`docker compose exec --user "\${PUID:-1000}:\${PGID:-1000}" opensync ./opensync reset-password --user ${safeResetUserName}`}
                    >
                      Docker Compose 部署（以容器内应用用户执行，属主与 PUID/PGID 保持一致）
                    </CliCommand>
                    <CliCommand command={`./opensync reset-password --user ${safeResetUserName}`}>
                      二进制 / 裸机部署
                    </CliCommand>
                  </div>
                </div>
              )}
            </div>

            <Field label="新密码" required>
              <Input
                type="password"
                placeholder="设置新密码"
                value={resetNewPasswd}
                onChange={(e) => setResetNewPasswd(e.target.value)}
                prefixIcon={<Lock className={icon.md} />}
                required
              />
            </Field>

            <Field label="确认新密码" required>
              <Input
                type="password"
                name="confirmPasswd"
                placeholder="再次输入新密码"
                value={resetConfirmPasswd}
                onChange={(e) => setResetConfirmPasswd(e.target.value)}
                prefixIcon={<Lock className={icon.md} />}
                required
              />
            </Field>

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
