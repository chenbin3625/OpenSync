import { useState, useEffect, useCallback, useMemo, useRef } from 'react';
import {
  Plus, Edit, Trash2, Send, Bell,
} from 'lucide-react';
import { notifyGet, notifyPost, notifyPostTest, notifyPut, notifyDelete } from '../../api/notify';
import dayjs from 'dayjs';
import type { NotifyFormValues, NotifyItem } from '../../types';
import { Button } from '../../components/ui/button';
import { Input } from '../../components/ui/input';
import { Switch } from '../../components/ui/switch';
import { Badge } from '../../components/ui/badge';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '../../components/ui/dialog';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '../../components/ui/alert-dialog';
import { TooltipProvider } from '../../components/ui/tooltip';
import { toast } from '../../components/ui/toaster';
import { maskSecret, maskWebhookUrl } from '../../utils/mask';
import { NativeSelect } from '../../components/ui/native-select';
import { Textarea } from '../../components/ui/textarea';
import { PageHeader } from '../../components/common/PageHeader';
import { Field } from '../../components/common/Field';
import { Alert } from '../../components/common/Alert';
import { InfoPanel, InfoRow } from '../../components/common/InfoRow';
import { ResourceCard } from '../../components/common/ResourceCard';
import { SwitchRow } from '../../components/common/SwitchRow';
import {
  EmptyState,
  ErrorState,
  PlaceholderCard,
} from '../../components/common/StatePlaceholder';
import { cn } from '../../lib/utils';
import { control, icon, layout, surface } from '../../lib/styles';

export const methodNames: Record<number, string> = {
  0: '自定义Webhook', 1: 'Server酱', 2: '钉钉', 3: '企业微信', 4: 'Lark (飞书)',
};

export const displaySecret = (value: string): string => maskSecret(value);
export const displayWebhookUrl = (value: string): string => maskWebhookUrl(value);

type NotifyParamValue = string | number | boolean | Record<string, unknown> | null | undefined;
type NotifyParams = Record<string, NotifyParamValue>;

export const notifyParamKeysByMethod: Record<number, Array<keyof NotifyFormValues>> = {
  0: ['url', 'httpMethod', 'contentType', 'needContent', 'titleName', 'contentName', 'body', 'headers', 'notSendNull'],
  1: ['sendKey', 'version', 'notSendNull'],
  2: ['url', 'webhook', 'notSendNull'],
  3: ['corpid', 'corpId', 'corpsecret', 'corpSecret', 'agentid', 'agentId', 'touser', 'toUser', 'notSendNull'],
  4: ['url', 'webhook', 'notSendNull'],
};

const asNotifyParams = (value: unknown): NotifyParams => {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return {};
  }
  return value as NotifyParams;
};

const paramString = (params: NotifyParams, key: string): string => {
  const value = params[key];
  return value === undefined || value === null ? '' : String(value);
};

const parseOptionalJsonObject = (value: unknown): Record<string, unknown> | undefined => {
  if (value === undefined || value === null || value === '') return undefined;
  if (typeof value === 'object' && !Array.isArray(value)) return value as Record<string, unknown>;
  const text = String(value).trim();
  if (!text) return undefined;
  const parsed = JSON.parse(text);
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new Error('JSON 必须是对象');
  }
  return parsed as Record<string, unknown>;
};

const pickNotifyParams = (method: number, params: NotifyParams): NotifyParams => {
  const picked: NotifyParams = {};
  notifyParamKeysByMethod[method]?.forEach((key) => {
    const value = params[key];
    if (value !== undefined && value !== null && value !== '') {
      picked[key] = value;
    }
  });
  return picked;
};

export const normalizeNotifyParams = (method: number, params: NotifyParams): NotifyParams => {
  const normalized = { ...params };
  if (method === 0) {
    normalized.method = normalized.method || normalized.httpMethod || 'POST';
    normalized.contentType = normalized.contentType || 'application/json';
    normalized.needContent = normalized.needContent ?? true;
    normalized.titleName = normalized.titleName || 'title';
    normalized.contentName = normalized.contentName || 'content';
    const body = parseOptionalJsonObject(normalized.body);
    const headers = parseOptionalJsonObject(normalized.headers);
    if (body) {
      normalized.body = JSON.stringify(body);
    } else {
      delete normalized.body;
    }
    if (headers) {
      normalized.headers = headers;
    } else {
      delete normalized.headers;
    }
    delete normalized.httpMethod;
  }
  if (method === 2 || method === 4) {
    normalized.url = normalized.url || normalized.webhook;
    delete normalized.webhook;
  }
  if (method === 3) {
    normalized.corpid = normalized.corpid || normalized.corpId;
    normalized.corpsecret = normalized.corpsecret || normalized.corpSecret;
    normalized.agentid = normalized.agentid || normalized.agentId;
    normalized.touser = normalized.touser || normalized.toUser || '@all';
    delete normalized.corpId;
    delete normalized.corpSecret;
    delete normalized.agentId;
    delete normalized.toUser;
  }
  return normalized;
};

export const getNotifyParamsFromValues = (values: NotifyFormValues): NotifyParams => {
  const params = pickNotifyParams(values.method, values as unknown as NotifyParams);
  return normalizeNotifyParams(values.method, params);
};

export default function Notify() {
  const [list, setList] = useState<NotifyItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [listError, setListError] = useState(false);

  // 弹窗与表单状态
  const [modalVisible, setModalVisible] = useState(false);
  const [editingItem, setEditingItem] = useState<NotifyItem | null>(null);
  const [method, setMethod] = useState(0);
  const [enable, setEnable] = useState(true);
  const [notSendNull, setNotSendNull] = useState(false);
  const [pendingNotifyValues, setPendingNotifyValues] = useState<Partial<NotifyFormValues> | null>(null);

  // 各通道特定输入字段
  const [url, setUrl] = useState('');
  const [httpMethod, setHttpMethod] = useState('POST');
  const [body, setBody] = useState('');
  const [headers, setHeaders] = useState('');
  const [sendKey, setSendKey] = useState('');
  const [version, setVersion] = useState('v3');
  const [corpid, setCorpid] = useState('');
  const [corpsecret, setCorpsecret] = useState('');
  const [agentid, setAgentid] = useState('');
  const [touser, setTouser] = useState('@all');

  const [formError, setFormError] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [testingForm, setTestingForm] = useState(false);

  // 列表行测试与删除确认
  const [testingId, setTestingId] = useState<number | null>(null);
  const [deleteTargetId, setDeleteTargetId] = useState<number | null>(null);

  const listReqRef = useRef(0);

  // 虚拟的 form 兼容对象，满足测试断言和统一取值
  const form = useMemo(() => ({
    resetFields() {
      setUrl('');
      setHttpMethod('POST');
      setBody('');
      setHeaders('');
      setSendKey('');
      setVersion('v3');
      setCorpid('');
      setCorpsecret('');
      setAgentid('');
      setTouser('@all');
      setNotSendNull(false);
      setFormError('');
    },
    setFieldsValue(vals: Partial<NotifyFormValues> | null) {
      if (!vals) return;
      if (vals.method !== undefined) setMethod(vals.method);
      if (vals.enable !== undefined) setEnable(vals.enable);
      if (vals.notSendNull !== undefined) setNotSendNull(vals.notSendNull);
      if (vals.url !== undefined) setUrl(vals.url);
      if (vals.httpMethod !== undefined) setHttpMethod(vals.httpMethod);
      if (vals.body !== undefined) setBody(vals.body);
      if (vals.headers !== undefined) setHeaders(vals.headers);
      if (vals.sendKey !== undefined) setSendKey(vals.sendKey);
      if (vals.version !== undefined) setVersion(vals.version);
      if (vals.corpid !== undefined || vals.corpId !== undefined) setCorpid(vals.corpid || vals.corpId || '');
      if (vals.corpsecret !== undefined || vals.corpSecret !== undefined) setCorpsecret(vals.corpsecret || vals.corpSecret || '');
      if (vals.agentid !== undefined || vals.agentId !== undefined) setAgentid(vals.agentid || vals.agentId || '');
      if (vals.touser !== undefined || vals.toUser !== undefined) setTouser(vals.touser || vals.toUser || '@all');
    },
    getFieldValue(key: string) {
      if (key === 'enable') return enable;
      if (key === 'method') return method;
      return undefined;
    },
    validateFields(): Promise<NotifyFormValues> {
      try {
        if (body.trim()) parseOptionalJsonObject(body);
        if (headers.trim()) parseOptionalJsonObject(headers);
      } catch {
        return Promise.reject(new Error('请输入有效 JSON 对象'));
      }
      return Promise.resolve({
        method,
        enable,
        url,
        httpMethod,
        body,
        headers,
        sendKey,
        version,
        corpid,
        corpsecret,
        agentid,
        touser,
        notSendNull,
      });
    },
  }), [agentid, body, corpid, corpsecret, enable, headers, httpMethod, method, notSendNull, sendKey, touser, url, version]);

  const fetchList = useCallback(async () => {
    const reqID = ++listReqRef.current;
    setLoading(true);
    setListError(false);
    try {
      const res = await notifyGet({ silent: true });
      if (reqID !== listReqRef.current) return;
      setList(res.data || []);
    } catch (err) {
      if (reqID !== listReqRef.current) return;
      setListError(true);
      setList([]);
      console.error('notify fetchList failed', err);
    } finally {
      if (reqID === listReqRef.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchList();
  }, [fetchList]);

  const handleAdd = () => {
    setEditingItem(null);
    form.resetFields();
    setPendingNotifyValues({ method: 0, enable: true });
    setMethod(0);
    setEnable(true);
    setModalVisible(true);
  };

  const handleEdit = (item: NotifyItem) => {
    setEditingItem(item);
    let params: NotifyParams = {};
    try {
      params = asNotifyParams(JSON.parse(item.params || '{}'));
    } catch (err) {
      console.error('notify params parse failed', err);
      toast.error('通知配置解析失败');
    }
    form.resetFields();
    setPendingNotifyValues({ ...params, method: item.method, enable: item.enable === 1 } as Partial<NotifyFormValues>);
    setMethod(item.method);
    setEnable(item.enable === 1);
    setModalVisible(true);
  };

  useEffect(() => {
    if (!modalVisible || !pendingNotifyValues || pendingNotifyValues.method !== method) return;
    form.setFieldsValue(pendingNotifyValues);
    setPendingNotifyValues(null);
  }, [form, method, modalVisible, pendingNotifyValues]);

  const handleMethodChange = (nextMethod: number) => {
    form.resetFields();
    form.setFieldsValue({ method: nextMethod, enable });
    setMethod(nextMethod);
  };

  const handleDelete = async () => {
    if (!deleteTargetId) return;
    try {
      await notifyDelete(deleteTargetId);
      toast.success('删除成功');
      fetchList();
    } catch (err) {
      console.error('notify delete failed', err);
    } finally {
      setDeleteTargetId(null);
    }
  };

  const handleToggleStatus = async (item: NotifyItem, nextEnabled: boolean) => {
    try {
      await notifyPut({ notifyId: item.id, enable: nextEnabled ? 1 : 0 });
      toast.success('状态更新成功');
      fetchList();
    } catch (err) {
      console.error('notify toggle failed', err);
    }
  };

  const handleTest = async () => {
    if (testingForm) return;
    setTestingForm(true);
    setFormError('');
    try {
      const values = await form.validateFields();
      const notifyData = {
        ...(editingItem ? { id: editingItem.id } : {}),
        method: values.method,
        params: JSON.stringify(getNotifyParamsFromValues(values)),
      };
      await notifyPostTest({ notify: notifyData });
      toast.success('测试消息已发送');
    } catch (err: unknown) {
      setFormError((err as Error).message || '测试发送失败');
    } finally {
      setTestingForm(false);
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSubmitting(true);
    setFormError('');
    try {
      const values = await form.validateFields();
      const notifyData = {
        ...(editingItem ? { id: editingItem.id } : {}),
        enable: values.enable ? 1 : 0,
        method: values.method,
        params: JSON.stringify(getNotifyParamsFromValues(values)),
      };
      if (editingItem) {
        await notifyPut({ notify: notifyData });
      } else {
        await notifyPost({ notify: notifyData });
      }
      toast.success(editingItem ? '更新成功' : '新增成功');
      setModalVisible(false);
      fetchList();
    } catch (err: unknown) {
      setFormError((err as Error).message || '提交失败');
    } finally {
      setSubmitting(false);
    }
  };

  const handleTestSend = async (item: NotifyItem) => {
    if (testingId !== null) return;
    setTestingId(item.id);
    try {
      await notifyPostTest({ notify: { id: item.id, method: item.method, params: item.params } });
      toast.success('测试消息已发送');
    } catch (err) {
      console.error('notify test send failed', err);
      toast.error('发送失败');
    } finally {
      setTestingId(null);
    }
  };

  const parseParams = (item: NotifyItem): NotifyParams => {
    try { return asNotifyParams(JSON.parse(item.params || '{}')); } catch { return {}; }
  };

  const getParamSummary = (item: NotifyItem) => {
    const p = parseParams(item);
    switch (item.method) {
      case 0: return displayWebhookUrl(paramString(p, 'url'));
      case 1: return displaySecret(paramString(p, 'sendKey'));
      case 2: return displayWebhookUrl(paramString(p, 'url') || paramString(p, 'webhook'));
      case 3: return paramString(p, 'corpid') || paramString(p, 'corpId') || '—';
      case 4: return displayWebhookUrl(paramString(p, 'url') || paramString(p, 'webhook'));
      default: return '—';
    }
  };

  return (
    <TooltipProvider>
      <div className={layout.page}>
        <PageHeader
          title="通知配置"
          subtitle="配置任务完成、失败和无需同步时的消息渠道"
          actions={
            <Button onClick={handleAdd}>
              <Plus className={cn(icon.md, 'mr-1.5')} />
              新增通知
            </Button>
          }
        />

        {/* 列表主体 */}
        <div className="min-w-0">
          {listError ? (
            <PlaceholderCard>
              <ErrorState
                icon={Bell}
                title="通知配置加载失败"
                onRetry={fetchList}
                loading={loading}
                retryLabel="重试"
              />
            </PlaceholderCard>
          ) : list.length === 0 && !loading ? (
            <div className={surface.placeholder}>
              <EmptyState icon={Bell} title="暂无通知渠道配置，添加后可在任务完成时接收通知" />
            </div>
          ) : (
            <div className={layout.cardGrid}>
              {list.map((item) => {
                const params = parseParams(item);
                const isEnabled = item.enable === 1;

                return (
                  <ResourceCard
                    key={item.id}
                    icon={Bell}
                    title={methodNames[item.method] || `方式 ${item.method}`}
                    subtitle={getParamSummary(item)}
                    badge={
                      <div className="flex items-center gap-2">
                        <Badge variant={isEnabled ? 'success' : 'secondary'}>
                          {isEnabled ? '已启用' : '已禁用'}
                        </Badge>
                        <Switch
                          checked={isEnabled}
                          onCheckedChange={(checked) => handleToggleStatus(item, checked)}
                          aria-label={`${isEnabled ? '禁用' : '启用'}${methodNames[item.method] || '通知'}`}
                        />
                      </div>
                    }
                    actions={
                      <div className="flex items-center justify-end gap-1.5 w-full">
                        <Button
                          variant="ghost"
                          size="sm"
                          loading={testingId === item.id}
                          onClick={() => handleTestSend(item)}
                          className={cn(control.dense, 'px-2 text-slate-600 hover:text-teal-700')}
                        >
                          <Send className={cn(icon.sm, 'mr-1')} />
                          测试发送
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => handleEdit(item)}
                          className={cn(control.dense, 'px-2 text-slate-600 hover:text-slate-900')}
                        >
                          <Edit className={cn(icon.sm, 'mr-1')} />
                          编辑
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => setDeleteTargetId(item.id)}
                          className={cn(
                            control.dense,
                            'px-2 text-rose-600 hover:text-rose-700 hover:bg-rose-50'
                          )}
                        >
                          <Trash2 className={cn(icon.sm, 'mr-1')} />
                          删除
                        </Button>
                      </div>
                    }
                  >
                    <InfoPanel>
                      <InfoRow label="关键参数" mono>
                        <span className="truncate inline-block max-w-full" title={getParamSummary(item)}>
                          {getParamSummary(item)}
                        </span>
                      </InfoRow>
                      <InfoRow label="静默规则">
                        {params.notSendNull ? '无需同步时不发送' : '始终发送'}
                      </InfoRow>
                      <InfoRow label="添加时间">
                        {item.createTime
                          ? dayjs.unix(item.createTime).format('YYYY-MM-DD HH:mm')
                          : '—'}
                      </InfoRow>
                    </InfoPanel>
                  </ResourceCard>
                );
              })}
            </div>
          )}
        </div>

        {/* 新增 / 编辑 通知弹窗 */}
        <Dialog open={modalVisible} onOpenChange={setModalVisible}>
          <DialogContent className="max-w-lg max-h-[90vh]">
            <DialogHeader>
              <DialogTitle>{editingItem ? '编辑通知' : '新增通知'}</DialogTitle>
            </DialogHeader>

            {formError && <Alert>{formError}</Alert>}

            <form onSubmit={handleSubmit} className="space-y-4 my-2">
              <Field label="通知渠道">
                <NativeSelect
                  value={method}
                  onChange={(e) => handleMethodChange(Number(e.target.value))}
                >
                  {Object.entries(methodNames).map(([k, v]) => (
                    <option key={k} value={k}>
                      {v}
                    </option>
                  ))}
                </NativeSelect>
              </Field>

              <SwitchRow
                label="启用状态"
                description="是否激活此渠道以接收通知"
                checked={enable}
                onCheckedChange={setEnable}
              />

              {/* 动态表单字段 */}
              {method === 0 && (
                <div className={cn('space-y-3 pt-1 border-t', surface.divider)}>
                  <Field label="Webhook URL" required>
                    <Input
                      placeholder="https://example.com/api/webhook"
                      value={url}
                      onChange={(e) => setUrl(e.target.value)}
                      required
                    />
                  </Field>

                  <Field label="HTTP 请求方法">
                    <NativeSelect
                      value={httpMethod}
                      onChange={(e) => setHttpMethod(e.target.value)}
                    >
                      <option value="POST">POST</option>
                      <option value="GET">GET</option>
                      <option value="PUT">PUT</option>
                    </NativeSelect>
                  </Field>

                  <Field label="请求体模板 (JSON)">
                    <Textarea
                      name="body"
                      rows={4}
                      placeholder={'{\n  "title": "{title}",\n  "content": "{content}"\n}'}
                      value={body}
                      onChange={(e) => setBody(e.target.value)}
                      className="font-mono text-xs"
                    />
                  </Field>

                  <Field label="自定义请求头 (JSON)">
                    <Textarea
                      name="headers"
                      rows={3}
                      placeholder={'{\n  "Authorization": "Bearer token"\n}'}
                      value={headers}
                      onChange={(e) => setHeaders(e.target.value)}
                      className="font-mono text-xs"
                    />
                  </Field>
                </div>
              )}

              {method === 1 && (
                <div className={cn('space-y-3 pt-1 border-t', surface.divider)}>
                  <Field label="Server酱 SendKey" required>
                    <Input
                      type="password"
                      placeholder="SCTxxxxxxxx"
                      value={sendKey}
                      onChange={(e) => setSendKey(e.target.value)}
                      required
                    />
                  </Field>
                  <Field label="API 版本">
                    <NativeSelect value={version} onChange={(e) => setVersion(e.target.value)}>
                      <option value="v3">v3</option>
                      <option value="v1">v1</option>
                    </NativeSelect>
                  </Field>
                </div>
              )}

              {(method === 2 || method === 4) && (
                <div className={cn('space-y-3 pt-1 border-t', surface.divider)}>
                  <Field
                    label={method === 2 ? '钉钉机器人 Webhook' : '飞书 / Lark 机器人 Webhook'}
                    required
                  >
                    <Input
                      placeholder="https://oapi.dingtalk.com/robot/send?access_token=..."
                      value={url}
                      onChange={(e) => setUrl(e.target.value)}
                      required
                    />
                  </Field>
                </div>
              )}

              {method === 3 && (
                <div className={cn('space-y-3 pt-1 border-t', surface.divider)}>
                  <Field label="企业ID (CorpId)" required>
                    <Input
                      placeholder="wwxxxxxxxx"
                      value={corpid}
                      onChange={(e) => setCorpid(e.target.value)}
                      required
                    />
                  </Field>
                  <Field label="应用Secret (CorpSecret)" required>
                    <Input
                      type="password"
                      placeholder="xxxxxxxx"
                      value={corpsecret}
                      onChange={(e) => setCorpsecret(e.target.value)}
                      required
                    />
                  </Field>
                  <Field label="应用 AgentId" required>
                    <Input
                      placeholder="1000002"
                      value={agentid}
                      onChange={(e) => setAgentid(e.target.value)}
                      required
                    />
                  </Field>
                  <Field label="发送目标成员 (ToUser)">
                    <Input
                      placeholder="@all 或 成员账号"
                      value={touser}
                      onChange={(e) => setTouser(e.target.value)}
                    />
                  </Field>
                </div>
              )}

              {/* 静默通知开关 */}
              <SwitchRow
                label="无需同步时静默"
                description="当本次同步没有产生任何新增或变动文件时，不发送通知打扰"
                checked={notSendNull}
                onCheckedChange={setNotSendNull}
              />

              <DialogFooter className="pt-2">
                <Button type="button" variant="outline" onClick={() => setModalVisible(false)}>
                  取消
                </Button>
                <Button type="button" variant="secondary" onClick={handleTest} loading={testingForm}>
                  <Send className={cn(icon.sm, 'mr-1.5')} />
                  测试发送
                </Button>
                <Button type="submit" loading={submitting}>
                  确定保存
                </Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>

        {/* 删除确认弹窗 */}
        <AlertDialog open={deleteTargetId !== null} onOpenChange={(o) => !o && setDeleteTargetId(null)}>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>确认删除此通知？</AlertDialogTitle>
              <AlertDialogDescription>
                删除后该渠道将不再接收任务完成或失败的推送通知。
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>取消</AlertDialogCancel>
              <AlertDialogAction variant="destructive" onClick={handleDelete}>
                确认删除
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </div>
    </TooltipProvider>
  );
}
