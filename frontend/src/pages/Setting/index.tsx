import { useCallback, useEffect, useRef, useState } from 'react';
import { Lock, Save, HelpCircle, RefreshCw, Sliders, Shield } from 'lucide-react';
import { editPwd } from '../../api/user';
import { getSystemConfig, updateSystemConfig } from '../../api/system';
import type { SystemSettings } from '../../types';
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
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '../../components/ui/tooltip';
import { toast } from '../../components/ui/toaster';

export default function Setting() {
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [configError, setConfigError] = useState(false);
  const [configValues, setConfigValues] = useState<SystemSettings | null>(null);
  void configValues;

  // 表单受控状态
  const [expires, setExpires] = useState(720);
  const [taskTimeout, setTaskTimeout] = useState(1440);
  const [taskSave, setTaskSave] = useState(500);
  const [copyConcurrency, setCopyConcurrency] = useState(4);
  const [scanConcurrency, setScanConcurrency] = useState(4);
  const [maxRetries, setMaxRetries] = useState(3);

  // 修改密码弹窗
  const [passwordVisible, setPasswordVisible] = useState(false);
  const [oldPasswd, setOldPasswd] = useState('');
  const [newPasswd, setNewPasswd] = useState('');
  const [confirmPasswd, setConfirmPasswd] = useState('');
  const [passwordSaving, setPasswordSaving] = useState(false);
  const [passwordError, setPasswordError] = useState('');

  const configRequestRef = useRef(0);
  const configAbortRef = useRef<AbortController | null>(null);

  const fetchConfig = useCallback(async () => {
    const requestID = ++configRequestRef.current;
    configAbortRef.current?.abort();
    const controller = new AbortController();
    configAbortRef.current = controller;
    setLoading(true);
    setConfigError(false);
    try {
      const res = await getSystemConfig({ silent: true, signal: controller.signal });
      if (requestID !== configRequestRef.current) return;
      if (res.data) {
        setConfigValues(res.data);
        setExpires(res.data.expires ?? 720);
        setTaskTimeout(res.data.taskTimeout ?? 1440);
        setTaskSave(res.data.taskSave ?? 500);
        setCopyConcurrency(res.data.copyConcurrency ?? 4);
        setScanConcurrency(res.data.scanConcurrency ?? 4);
        setMaxRetries(res.data.maxRetries ?? 3);
      }
    } catch (err) {
      if (controller.signal.aborted) return;
      if (requestID !== configRequestRef.current) return;
      setConfigError(true);
      console.error('system config fetch failed', err);
    } finally {
      if (requestID === configRequestRef.current) {
        setLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    fetchConfig();
    return () => { configAbortRef.current?.abort(); };
  }, [fetchConfig]);

  const handleSaveConfig = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaving(true);
    try {
      const payload: SystemSettings = {
        expires: Number(expires),
        taskTimeout: Number(taskTimeout),
        taskSave: Number(taskSave),
        copyConcurrency: Number(copyConcurrency),
        scanConcurrency: Number(scanConcurrency),
        maxRetries: Number(maxRetries),
      };
      const res = await updateSystemConfig(payload);
      if (res.data) {
        setConfigValues(res.data);
      }
      toast.success('系统配置已保存');
    } catch (err) {
      console.error('system config save failed', err);
    } finally {
      setSaving(false);
    }
  };

  const handleChangePassword = async (e: React.FormEvent) => {
    e.preventDefault();
    setPasswordError('');

    if (!oldPasswd) {
      setPasswordError('请输入当前旧密码');
      return;
    }
    if (!newPasswd) {
      setPasswordError('请输入新密码');
      return;
    }
    if (newPasswd !== confirmPasswd) {
      setPasswordError('两次输入的新密码不一致');
      return;
    }

    setPasswordSaving(true);
    try {
      await editPwd({ oldPasswd, passwd: newPasswd });
      toast.success('密码修改成功');
      setOldPasswd('');
      setNewPasswd('');
      setConfirmPasswd('');
      setPasswordVisible(false);
    } catch (err: unknown) {
      console.error('password change failed', err);
    } finally {
      setPasswordSaving(false);
    }
  };

  return (
    <TooltipProvider>
      <div className="grid content-start gap-4 min-w-0 min-h-[calc(100vh-90px)]">
        {/* 标题栏 */}
        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 pb-2 border-b border-slate-200/80">
          <div className="space-y-1 min-w-0">
            <h1 className="text-xl font-bold tracking-tight text-slate-900">系统设置</h1>
            <p className="text-sm text-slate-500">
              调整运行参数并维护管理员密码
            </p>
          </div>
          <div className="flex shrink-0 items-center justify-end gap-2">
            <Button variant="outline" size="sm" onClick={() => setPasswordVisible(true)}>
              <Lock className="h-4 w-4 mr-1.5" />
              修改密码
            </Button>
          </div>
        </div>

        {/* 主配置表单 */}
        <div className="min-w-0 max-w-3xl">
          {configError ? (
            <div className="py-12 text-center space-y-3 bg-white rounded-xl border border-slate-200">
              <p className="text-sm text-slate-500">系统配置加载失败</p>
              <Button variant="outline" size="sm" onClick={fetchConfig} loading={loading}>
                <RefreshCw className="h-3.5 w-3.5 mr-1.5" />
                重试
              </Button>
            </div>
          ) : (
            <form onSubmit={handleSaveConfig} className="bg-white rounded-xl border border-slate-200/80 shadow-xs p-6 space-y-6">
              <div className="flex items-center gap-2 pb-3 border-b border-slate-100 text-slate-900 font-semibold text-sm">
                <Sliders className="h-4 w-4 text-teal-700" />
                <span>运行参数设置</span>
              </div>

              <div className="grid grid-cols-1 sm:grid-cols-2 gap-5">
                {/* expires */}
                <div className="space-y-1.5">
                  <div className="flex items-center gap-1.5">
                    <label className="text-xs font-medium text-slate-700">登录会话过期时间</label>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <HelpCircle className="h-3.5 w-3.5 text-slate-400 cursor-help" />
                      </TooltipTrigger>
                      <TooltipContent>登录 Token 保持有效的时间，单位：小时</TooltipContent>
                    </Tooltip>
                  </div>
                  <div className="flex items-center gap-2">
                    <Input
                      type="number"
                      name="expires"
                      min={1}
                      value={expires}
                      onChange={(e) => setExpires(Number(e.target.value))}
                      required
                    />
                    <span className="text-xs text-slate-500 shrink-0">小时</span>
                  </div>
                </div>

                {/* taskTimeout */}
                <div className="space-y-1.5">
                  <div className="flex items-center gap-1.5">
                    <label className="text-xs font-medium text-slate-700">单次任务超时时间</label>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <HelpCircle className="h-3.5 w-3.5 text-slate-400 cursor-help" />
                      </TooltipTrigger>
                      <TooltipContent>同步任务最长持续运行限制，超过该时间将被强制中止，单位：分钟</TooltipContent>
                    </Tooltip>
                  </div>
                  <div className="flex items-center gap-2">
                    <Input
                      type="number"
                      name="taskTimeout"
                      min={1}
                      value={taskTimeout}
                      onChange={(e) => setTaskTimeout(Number(e.target.value))}
                      required
                    />
                    <span className="text-xs text-slate-500 shrink-0">分钟</span>
                  </div>
                </div>

                {/* taskSave */}
                <div className="space-y-1.5">
                  <div className="flex items-center gap-1.5">
                    <label className="text-xs font-medium text-slate-700">历史任务日志保留条数</label>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <HelpCircle className="h-3.5 w-3.5 text-slate-400 cursor-help" />
                      </TooltipTrigger>
                      <TooltipContent>单个任务保留的历史记录上限，超出将自动清理旧日志</TooltipContent>
                    </Tooltip>
                  </div>
                  <div className="flex items-center gap-2">
                    <Input
                      type="number"
                      name="taskSave"
                      min={10}
                      value={taskSave}
                      onChange={(e) => setTaskSave(Number(e.target.value))}
                      required
                    />
                    <span className="text-xs text-slate-500 shrink-0">条</span>
                  </div>
                </div>

                {/* copyConcurrency */}
                <div className="space-y-1.5">
                  <div className="flex items-center gap-1.5">
                    <label className="text-xs font-medium text-slate-700">文件复制传输并发</label>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <HelpCircle className="h-3.5 w-3.5 text-slate-400 cursor-help" />
                      </TooltipTrigger>
                      <TooltipContent>同时执行文件拷贝与下载的最大并发协程数</TooltipContent>
                    </Tooltip>
                  </div>
                  <div className="flex items-center gap-2">
                    <Input
                      type="number"
                      name="copyConcurrency"
                      min={1}
                      max={64}
                      value={copyConcurrency}
                      onChange={(e) => setCopyConcurrency(Number(e.target.value))}
                      required
                    />
                    <span className="text-xs text-slate-500 shrink-0">并发</span>
                  </div>
                </div>

                {/* scanConcurrency */}
                <div className="space-y-1.5">
                  <div className="flex items-center gap-1.5">
                    <label className="text-xs font-medium text-slate-700">目录扫描并发</label>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <HelpCircle className="h-3.5 w-3.5 text-slate-400 cursor-help" />
                      </TooltipTrigger>
                      <TooltipContent>扫描比对源端和目标端目录层级的并发度</TooltipContent>
                    </Tooltip>
                  </div>
                  <div className="flex items-center gap-2">
                    <Input
                      type="number"
                      name="scanConcurrency"
                      min={1}
                      max={32}
                      value={scanConcurrency}
                      onChange={(e) => setScanConcurrency(Number(e.target.value))}
                      required
                    />
                    <span className="text-xs text-slate-500 shrink-0">并发</span>
                  </div>
                </div>

                {/* maxRetries */}
                <div className="space-y-1.5">
                  <div className="flex items-center gap-1.5">
                    <label className="text-xs font-medium text-slate-700">失败重试上限</label>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <HelpCircle className="h-3.5 w-3.5 text-slate-400 cursor-help" />
                      </TooltipTrigger>
                      <TooltipContent>网络抖动或临时错误时的单文件自动重试次数</TooltipContent>
                    </Tooltip>
                  </div>
                  <div className="flex items-center gap-2">
                    <Input
                      type="number"
                      name="maxRetries"
                      min={0}
                      max={10}
                      value={maxRetries}
                      onChange={(e) => setMaxRetries(Number(e.target.value))}
                      required
                    />
                    <span className="text-xs text-slate-500 shrink-0">次</span>
                  </div>
                </div>
              </div>

              <div className="pt-4 border-t border-slate-100 flex justify-end">
                <Button type="submit" loading={saving}>
                  <Save className="h-4 w-4 mr-1.5" />
                  保存系统配置
                </Button>
              </div>
            </form>
          )}
        </div>

        {/* 修改密码 Dialog */}
        <Dialog open={passwordVisible} onOpenChange={setPasswordVisible}>
          <DialogContent className="max-w-md" title="修改密码">
            <DialogHeader>
              <div className="flex items-center gap-2 text-slate-900 mb-1">
                <Shield className="h-5 w-5 text-teal-700" />
                <DialogTitle>修改密码</DialogTitle>
              </div>
              <DialogDescription>
                输入当前旧密码以完成身份验证，并设定新的登录密码。
              </DialogDescription>
            </DialogHeader>

            {passwordError && (
              <div className="p-2.5 rounded bg-rose-50 border border-rose-200 text-rose-700 text-xs font-medium">
                {passwordError}
              </div>
            )}

            <form onSubmit={handleChangePassword} className="space-y-3.5 my-2">
              <div className="space-y-1">
                <label className="text-xs font-medium text-slate-700">当前密码</label>
                <Input
                  type="password"
                  placeholder="请输入旧密码"
                  value={oldPasswd}
                  onChange={(e) => setOldPasswd(e.target.value)}
                  prefixIcon={<Lock className="h-4 w-4" />}
                  required
                />
              </div>

              <div className="space-y-1">
                <label className="text-xs font-medium text-slate-700">新密码</label>
                <Input
                  type="password"
                  placeholder="请输入新密码"
                  value={newPasswd}
                  onChange={(e) => setNewPasswd(e.target.value)}
                  prefixIcon={<Lock className="h-4 w-4" />}
                  required
                />
              </div>

              <div className="space-y-1">
                <label className="text-xs font-medium text-slate-700">确认新密码</label>
                <Input
                  type="password"
                  placeholder="请再次输入新密码"
                  value={confirmPasswd}
                  onChange={(e) => setConfirmPasswd(e.target.value)}
                  prefixIcon={<Lock className="h-4 w-4" />}
                  required
                />
              </div>

              <DialogFooter className="pt-2">
                <Button type="button" variant="outline" onClick={() => setPasswordVisible(false)}>
                  取消
                </Button>
                <Button type="submit" loading={passwordSaving}>
                  确认修改
                </Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      </div>
    </TooltipProvider>
  );
}
