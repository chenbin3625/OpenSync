import { useCallback, useEffect, useRef, useState } from 'react';
import { Lock, Save, Sliders, Shield } from 'lucide-react';
import { editPwd, getUser } from '../../api/user';
import { getSystemConfig, updateSystemConfig } from '../../api/system';
import { useStore } from '../../stores/useStore';
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
import { TooltipProvider } from '../../components/ui/tooltip';
import { toast } from '../../components/ui/toaster';
import { PageHeader, SectionHeading } from '../../components/common/PageHeader';
import { Field } from '../../components/common/Field';
import { Alert } from '../../components/common/Alert';
import { ErrorState, PlaceholderCard } from '../../components/common/StatePlaceholder';
import { cn } from '../../lib/utils';
import { icon, layout, surface } from '../../lib/styles';

const MIN_PASSWORD_LENGTH = 8;
const MAX_PASSWORD_BYTES = 72;

const validateNewPassword = (value: string): string | null => {
  if (value.trim().length < MIN_PASSWORD_LENGTH) {
    return `新密码长度不能少于 ${MIN_PASSWORD_LENGTH} 位`;
  }
  if (new TextEncoder().encode(value).length > MAX_PASSWORD_BYTES) {
    return `新密码长度不能超过 ${MAX_PASSWORD_BYTES} 字节`;
  }
  return null;
};

export default function Setting() {
  const setUserInfo = useStore((s) => s.setUserInfo);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [configError, setConfigError] = useState(false);
  const [configFormError, setConfigFormError] = useState('');
  const [configValues, setConfigValues] = useState<SystemSettings | null>(null);

  // 表单受控状态（使用字符串允许输入过程中临时清空，失焦或提交时校验解析）
  const [expires, setExpires] = useState('7');
  const [taskTimeout, setTaskTimeout] = useState('48');
  const [taskSave, setTaskSave] = useState('30');
  const [copyConcurrency, setCopyConcurrency] = useState('4');
  const [scanConcurrency, setScanConcurrency] = useState('4');
  const [maxRetries, setMaxRetries] = useState('3');

  // 修改密码弹窗
  const [passwordVisible, setPasswordVisible] = useState(false);
  const [oldPasswd, setOldPasswd] = useState('');
  const [newPasswd, setNewPasswd] = useState('');
  const [confirmPasswd, setConfirmPasswd] = useState('');
  const [passwordSaving, setPasswordSaving] = useState(false);
  const [passwordError, setPasswordError] = useState('');

  const configRequestRef = useRef(0);
  const configAbortRef = useRef<AbortController | null>(null);

  const syncFormFromConfig = useCallback((data: SystemSettings) => {
    setExpires(String(data.expires ?? 7));
    setTaskTimeout(String(data.taskTimeout ?? 48));
    setTaskSave(String(data.taskSave ?? 30));
    setCopyConcurrency(String(data.copyConcurrency ?? 4));
    setScanConcurrency(String(data.scanConcurrency ?? 4));
    setMaxRetries(String(data.maxRetries ?? 3));
  }, []);

  const fetchConfig = useCallback(async () => {
    const requestID = ++configRequestRef.current;
    configAbortRef.current?.abort();
    const controller = new AbortController();
    configAbortRef.current = controller;
    setLoading(true);
    setConfigError(false);
    setConfigFormError('');
    try {
      const res = await getSystemConfig({ silent: true, signal: controller.signal });
      if (requestID !== configRequestRef.current) return;
      if (res.data) {
        setConfigValues(res.data);
        syncFormFromConfig(res.data);
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
  }, [syncFormFromConfig]);

  useEffect(() => {
    fetchConfig();
    return () => { configAbortRef.current?.abort(); };
  }, [fetchConfig]);

  /** 运行参数字段表。六个字段结构一致，用数据驱动避免重复标记 */
  const configFields = [
    {
      name: 'expires',
      label: '登录会话过期时间',
      tooltip: '登录 Token 保持有效的时间，单位：天',
      unit: '天',
      min: 1,
      max: 365,
      value: expires,
      onChange: setExpires,
    },
    {
      name: 'taskTimeout',
      label: '单次任务超时时间',
      tooltip: '同步任务最长运行时间，单位：小时，0 表示不限制；仅对新启动任务生效，超时后停止扫描与传输并请求取消远端任务',
      unit: '小时',
      min: 0,
      max: 8760,
      value: taskTimeout,
      onChange: setTaskTimeout,
    },
    {
      name: 'taskSave',
      label: '历史任务保留时间',
      tooltip: '历史任务保留时间，单位：天，0 表示不自动清理',
      unit: '天',
      min: 0,
      value: taskSave,
      onChange: setTaskSave,
    },
    {
      name: 'copyConcurrency',
      label: '文件复制传输并发',
      tooltip: '同时执行文件拷贝与下载的最大并发协程数',
      unit: '并发',
      min: 1,
      max: 64,
      value: copyConcurrency,
      onChange: setCopyConcurrency,
    },
    {
      name: 'scanConcurrency',
      label: '目录扫描并发',
      tooltip: '扫描比对源端和目标端目录层级的并发度',
      unit: '并发',
      min: 1,
      max: 32,
      value: scanConcurrency,
      onChange: setScanConcurrency,
    },
    {
      name: 'maxRetries',
      label: '失败重试上限',
      tooltip: '网络抖动或临时错误时的单文件自动重试次数',
      unit: '次',
      min: 0,
      max: 10,
      value: maxRetries,
      onChange: setMaxRetries,
    },
  ] satisfies Array<{
    name: keyof SystemSettings;
    label: string;
    tooltip: string;
    unit: string;
    min: number;
    max?: number;
    value: string;
    onChange: (value: string) => void;
  }>;

  const handleFieldBlur = (field: (typeof configFields)[number]) => {
    const raw = field.value.trim();
    if (!raw) return;
    const num = Number(raw);
    if (Number.isInteger(num)) {
      field.onChange(String(num));
    }
  };

  const handleSaveConfig = async (e: React.FormEvent) => {
    e.preventDefault();
    setConfigFormError('');

    const parsed = {} as Record<keyof SystemSettings, number>;
    for (const field of configFields) {
      const raw = field.value.trim();
      if (raw === '') {
        setConfigFormError(`请输入${field.label}`);
        return;
      }
      const num = Number(raw);
      if (!Number.isInteger(num)) {
        setConfigFormError(`${field.label}必须为整数`);
        return;
      }
      if (num < field.min) {
        setConfigFormError(`${field.label}不能小于 ${field.min}`);
        return;
      }
      if (field.max !== undefined && num > field.max) {
        setConfigFormError(`${field.label}不能大于 ${field.max}`);
        return;
      }
      parsed[field.name] = num;
    }

    setSaving(true);
    try {
      const payload: SystemSettings = {
        expires: parsed.expires,
        taskTimeout: parsed.taskTimeout,
        taskSave: parsed.taskSave,
        copyConcurrency: parsed.copyConcurrency,
        scanConcurrency: parsed.scanConcurrency,
        maxRetries: parsed.maxRetries,
      };
      const res = await updateSystemConfig(payload);
      if (res.data) {
        setConfigValues(res.data);
        syncFormFromConfig(res.data);
      }
      toast.success('系统配置已保存');
    } catch (err) {
      console.error('system config save failed', err);
    } finally {
      setSaving(false);
    }
  };

  const resetPasswordForm = () => {
    setOldPasswd('');
    setNewPasswd('');
    setConfirmPasswd('');
    setPasswordError('');
  };

  const handlePasswordDialogOpenChange = (open: boolean) => {
    setPasswordVisible(open);
    if (!open) {
      resetPasswordForm();
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
    const passwordErr = validateNewPassword(newPasswd);
    if (passwordErr) {
      setPasswordError(passwordErr);
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
      resetPasswordForm();
      setPasswordVisible(false);
      // Verify session state after password rotation: if the backend re-issued a
      // cookie this refreshes userInfo seamlessly; if the session was revoked,
      // the 401 response interceptor clears auth state and redirects to /login.
      try {
        const res = await getUser();
        if (res.data) {
          setUserInfo(res.data);
        }
      } catch {
        // Handled by the global request interceptor when 401 is returned.
      }
    } catch (err: unknown) {
      console.error('password change failed', err);
    } finally {
      setPasswordSaving(false);
    }
  };

  return (
    <TooltipProvider>
      <div className={layout.page}>
        <PageHeader
          title="系统设置"
          subtitle="调整运行参数并维护管理员密码"
          actions={
            <Button variant="outline" size="sm" onClick={() => setPasswordVisible(true)}>
              <Lock className={cn(icon.md, 'mr-1.5')} />
              修改密码
            </Button>
          }
        />

        {/* 主配置表单 */}
        <div className="min-w-0 max-w-3xl">
          {configError ? (
            <PlaceholderCard>
              <ErrorState
                icon={Sliders}
                title="系统配置加载失败"
                onRetry={fetchConfig}
                loading={loading}
                retryLabel="重试"
              />
            </PlaceholderCard>
          ) : (
            <form
              onSubmit={handleSaveConfig}
              className={cn(surface.card, 'p-6 space-y-6')}
            >
              <SectionHeading title="运行参数设置" icon={Sliders} bordered />

              {configFormError && <Alert>{configFormError}</Alert>}

              <div className={layout.formGrid}>
                {configFields.map((field) => (
                  <Field
                    key={field.name}
                    label={field.label}
                    tooltip={field.tooltip}
                    suffix={field.unit}
                    required
                  >
                    <Input
                      type="number"
                      name={field.name}
                      min={field.min}
                      max={field.max}
                      step={1}
                      value={field.value}
                      onChange={(e) => {
                        setConfigFormError('');
                        field.onChange(e.target.value);
                      }}
                      onBlur={() => handleFieldBlur(field)}
                      required
                    />
                  </Field>
                ))}
              </div>

              <div className={cn('pt-4 border-t flex justify-end', surface.divider)}>
                <Button type="submit" loading={saving} disabled={saving || !configValues}>
                  <Save className={cn(icon.md, 'mr-1.5')} />
                  保存系统配置
                </Button>
              </div>
            </form>
          )}
        </div>

        {/* 修改密码 Dialog */}
        <Dialog open={passwordVisible} onOpenChange={handlePasswordDialogOpenChange}>
          <DialogContent className="max-w-md">
            <DialogHeader>
              <div className="flex items-center gap-2 text-slate-900">
                <Shield className={cn(icon.lg, 'text-teal-700')} />
                <DialogTitle>修改密码</DialogTitle>
              </div>
              <DialogDescription>
                输入当前旧密码以完成身份验证，并设定新的登录密码。
              </DialogDescription>
            </DialogHeader>

            {passwordError && <Alert>{passwordError}</Alert>}

            <form onSubmit={handleChangePassword} className="space-y-3.5 my-2">
              <Field label="当前密码" required>
                <Input
                  type="password"
                  placeholder="请输入旧密码"
                  value={oldPasswd}
                  onChange={(e) => setOldPasswd(e.target.value)}
                  prefixIcon={<Lock className={icon.md} />}
                  required
                />
              </Field>

              <Field label="新密码" required hint="密码长度为 8-72 字符">
                <Input
                  type="password"
                  placeholder="请输入 8-72 位新密码"
                  value={newPasswd}
                  onChange={(e) => setNewPasswd(e.target.value)}
                  prefixIcon={<Lock className={icon.md} />}
                  required
                />
              </Field>

              <Field label="确认新密码" required>
                <Input
                  type="password"
                  placeholder="请再次输入新密码"
                  value={confirmPasswd}
                  onChange={(e) => setConfirmPasswd(e.target.value)}
                  prefixIcon={<Lock className={icon.md} />}
                  required
                />
              </Field>

              <DialogFooter className="pt-2">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => handlePasswordDialogOpenChange(false)}
                >
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
