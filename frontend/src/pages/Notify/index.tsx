import React, { useState, useEffect, useCallback, useMemo, useRef } from 'react';
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
import { icon, layout, surface } from '../../lib/styles';

export const methodNames: Record<number, string> = {
  0: '自定义Webhook', 1: 'Server酱', 2: '钉钉', 3: '企业微信', 4: 'Lark (飞书)',
};

export const displaySecret = (value: string): string => maskSecret(value);
export const displayWebhookUrl = (value: string): string => maskWebhookUrl(value);

type NotifyParamValue = string | number | boolean | Record<string, unknown> | null | undefined;
type NotifyParams = Record<string, NotifyParamValue>;

type SavedSecretHints = {
  url?: string;
  body?: boolean;
  sendKey?: string;
  corpsecret?: string;
};

type PreservedWebhookParams = {
  contentType?: string;
  needContent?: boolean;
  titleName?: string;
  contentName?: string;
};

const isMaskedSecretPlaceholder = (value: string): boolean => {
  if (!value) return false;
  if (value.includes('****')) return true;
  try {
    return decodeURIComponent(value).includes('****');
  } catch {
    return false;
  }
};

const isValidHttpUrl = (value: string, httpsOnly = false): boolean => {
  try {
    const parsed = new URL(value.trim());
    if (!parsed.host) return false;
    if (httpsOnly) return parsed.protocol === 'https:';
    return parsed.protocol === 'http:' || parsed.protocol === 'https:';
  } catch {
    return false;
  }
};

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

    if (typeof normalized.body === 'string' && isMaskedSecretPlaceholder(normalized.body.trim())) {
      delete normalized.body;
    } else {
      const body = parseOptionalJsonObject(normalized.body);
      if (body) {
        normalized.body = JSON.stringify(body);
      } else {
        delete normalized.body;
      }
    }

    const headers = parseOptionalJsonObject(normalized.headers);
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

  // 编辑态下：记录后端脱敏的密钥占位提示，以及保留隐藏的自定义 Webhook 高级参数
  const [savedSecretHints, setSavedSecretHints] = useState<SavedSecretHints>({});
  const [preservedWebhookParams, setPreservedWebhookParams] = useState<PreservedWebhookParams>({});

  const [formError, setFormError] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [testingForm, setTestingForm] = useState(false);

  // 列表行测试与删除确认
  const [testingId, setTestingId] = useState<number | null>(null);
  const [deleteTargetId, setDeleteTargetId] = useState<number | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState('');

  const listReqRef = useRef(0);

  const editingSameMethod = Boolean(editingItem && editingItem.method === method);

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
      setSavedSecretHints({});
      setPreservedWebhookParams({});
      setFormError('');
    },
    setFieldsValue(vals: Partial<NotifyFormValues> | null) {
      if (!vals) return;
      if (vals.method !== undefined) setMethod(vals.method);
      if (vals.enable !== undefined) setEnable(vals.enable);
      if (vals.notSendNull !== undefined) setNotSendNull(Boolean(vals.notSendNull));
      if (vals.url !== undefined || vals.webhook !== undefined) {
        setUrl(vals.url ?? vals.webhook ?? '');
      }
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
      const trimmedUrl = url.trim();
      const trimmedSendKey = sendKey.trim();
      const trimmedCorpId = corpid.trim();
      const trimmedCorpSecret = corpsecret.trim();
      const trimmedAgentId = agentid.trim();
      const trimmedToUser = touser.trim();
      const trimmedBody = body.trim();
      const trimmedHeaders = headers.trim();

      if (method === 0) {
        if (!trimmedUrl && !(editingSameMethod && savedSecretHints.url)) {
          return Promise.reject(new Error('请输入 Webhook URL'));
        }
        if (trimmedUrl && !isValidHttpUrl(trimmedUrl, false)) {
          return Promise.reject(new Error('请输入合法的 http 或 https 地址'));
        }
        if (trimmedBody && !isMaskedSecretPlaceholder(trimmedBody)) {
          try {
            parseOptionalJsonObject(trimmedBody);
          } catch {
            return Promise.reject(new Error('请求体模板必须是有效 JSON 对象'));
          }
        }
        if (trimmedHeaders) {
          try {
            parseOptionalJsonObject(trimmedHeaders);
          } catch {
            return Promise.reject(new Error('自定义请求头必须是有效 JSON 对象'));
          }
        }
      } else if (method === 1) {
        if (!trimmedSendKey && !(editingSameMethod && savedSecretHints.sendKey)) {
          return Promise.reject(new Error('请输入 Server酱 SendKey'));
        }
      } else if (method === 2 || method === 4) {
        if (!trimmedUrl && !(editingSameMethod && savedSecretHints.url)) {
          return Promise.reject(
            new Error(method === 2 ? '请输入钉钉机器人 Webhook' : '请输入飞书 / Lark 机器人 Webhook')
          );
        }
        if (trimmedUrl && !isValidHttpUrl(trimmedUrl, true)) {
          return Promise.reject(new Error('Webhook 地址必须是合法的 https 链接'));
        }
      } else if (method === 3) {
        if (!trimmedCorpId) {
          return Promise.reject(new Error('请输入企业ID (CorpId)'));
        }
        if (!trimmedCorpSecret && !(editingSameMethod && savedSecretHints.corpsecret)) {
          return Promise.reject(new Error('请输入应用Secret (CorpSecret)'));
        }
        if (!trimmedAgentId) {
          return Promise.reject(new Error('请输入应用 AgentId'));
        }
      }

      return Promise.resolve({
        method,
        enable,
        url: trimmedUrl,
        httpMethod,
        ...(method === 0 ? preservedWebhookParams : {}),
        body: trimmedBody && !isMaskedSecretPlaceholder(trimmedBody) ? trimmedBody : '',
        headers: trimmedHeaders,
        sendKey: trimmedSendKey,
        version,
        corpid: trimmedCorpId,
        corpsecret: trimmedCorpSecret,
        agentid: trimmedAgentId,
        touser: trimmedToUser || '@all',
        notSendNull,
      });
    },
  }), [
    agentid,
    body,
    corpid,
    corpsecret,
    editingSameMethod,
    enable,
    headers,
    httpMethod,
    method,
    notSendNull,
    preservedWebhookParams,
    savedSecretHints,
    sendKey,
    touser,
    url,
    version,
  ]);

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

    const nextHints: SavedSecretHints = {};
    const nextPreserved: PreservedWebhookParams = {};
    const initialValues: Partial<NotifyFormValues> = {
      method: item.method,
      enable: item.enable === 1,
      notSendNull: Boolean(params.notSendNull),
    };

    if (item.method === 0) {
      const savedUrl = paramString(params, 'url') || paramString(params, 'webhook');
      if (isMaskedSecretPlaceholder(savedUrl)) {
        nextHints.url = savedUrl;
        initialValues.url = '';
      } else {
        initialValues.url = savedUrl;
      }

      const rawHttpMethod =
        (typeof params.method === 'string' && params.method.trim()) ||
        (typeof params.httpMethod === 'string' && params.httpMethod.trim()) ||
        'POST';
      initialValues.httpMethod = rawHttpMethod.toUpperCase();

      if (typeof params.contentType === 'string' && params.contentType.trim()) {
        nextPreserved.contentType = params.contentType;
      }
      if (typeof params.needContent === 'boolean') {
        nextPreserved.needContent = params.needContent;
      }
      if (typeof params.titleName === 'string' && params.titleName.trim()) {
        nextPreserved.titleName = params.titleName;
      }
      if (typeof params.contentName === 'string' && params.contentName.trim()) {
        nextPreserved.contentName = params.contentName;
      }

      const savedBody = paramString(params, 'body');
      if (isMaskedSecretPlaceholder(savedBody)) {
        nextHints.body = true;
        initialValues.body = '';
      } else {
        initialValues.body = savedBody;
      }

      if (params.headers && typeof params.headers === 'object' && !Array.isArray(params.headers)) {
        initialValues.headers = JSON.stringify(params.headers, null, 2);
      } else {
        initialValues.headers = paramString(params, 'headers');
      }
    } else if (item.method === 1) {
      const savedSendKey = paramString(params, 'sendKey');
      if (isMaskedSecretPlaceholder(savedSendKey)) {
        nextHints.sendKey = savedSendKey;
        initialValues.sendKey = '';
      } else {
        initialValues.sendKey = savedSendKey;
      }
      // 历史 Server酱配置未显式记录 version 时，后端默认按 v1 发送
      initialValues.version = paramString(params, 'version') || 'v1';
    } else if (item.method === 2 || item.method === 4) {
      const savedUrl = paramString(params, 'url') || paramString(params, 'webhook');
      if (isMaskedSecretPlaceholder(savedUrl)) {
        nextHints.url = savedUrl;
        initialValues.url = '';
      } else {
        initialValues.url = savedUrl;
      }
    } else if (item.method === 3) {
      initialValues.corpid = paramString(params, 'corpid') || paramString(params, 'corpId');
      initialValues.agentid = paramString(params, 'agentid') || paramString(params, 'agentId');
      initialValues.touser = paramString(params, 'touser') || paramString(params, 'toUser') || '@all';
      const savedCorpSecret = paramString(params, 'corpsecret') || paramString(params, 'corpSecret');
      if (isMaskedSecretPlaceholder(savedCorpSecret)) {
        nextHints.corpsecret = savedCorpSecret;
        initialValues.corpsecret = '';
      } else {
        initialValues.corpsecret = savedCorpSecret;
      }
    }

    setSavedSecretHints(nextHints);
    setPreservedWebhookParams(nextPreserved);
    setPendingNotifyValues(initialValues);
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

  const handleDelete = async (e: React.MouseEvent<HTMLButtonElement>) => {
    e.preventDefault();
    if (!deleteTargetId || deleting) return;
    setDeleting(true);
    setDeleteError('');
    try {
      await notifyDelete(deleteTargetId);
      toast.success('删除成功');
      setDeleteTargetId(null);
      fetchList();
    } catch (err: unknown) {
      console.error('notify delete failed', err);
      setDeleteError((err as Error).message || '删除失败，请重试');
    } finally {
      setDeleting(false);
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
        ...(editingItem && editingItem.method === values.method ? { id: editingItem.id } : {}),
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
      // Axios interceptor already displays the backend error toast.
      console.error('notify test send failed', err);
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

  const hasSavedWebhookUrl = editingSameMethod && Boolean(savedSecretHints.url);
  const hasSavedSendKey = editingSameMethod && Boolean(savedSecretHints.sendKey);
  const hasSavedCorpSecret = editingSameMethod && Boolean(savedSecretHints.corpsecret);
  const hasSavedBody = editingSameMethod && Boolean(savedSecretHints.body);

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
                        >
                          <Send className={cn(icon.sm, 'mr-1')} />
                          测试发送
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => handleEdit(item)}
                        >
                          <Edit className={cn(icon.sm, 'mr-1')} />
                          编辑
                        </Button>
                        <Button
                          variant="danger"
                          size="sm"
                          onClick={() => {
                            setDeleteError('');
                            setDeleteTargetId(item.id);
                          }}
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
                  <Field
                    label="Webhook URL"
                    required={!hasSavedWebhookUrl}
                    hint={hasSavedWebhookUrl ? `已保存：${savedSecretHints.url}，留空将保留原地址` : undefined}
                  >
                    <Input
                      placeholder={
                        hasSavedWebhookUrl
                          ? '留空保持已保存的 Webhook URL'
                          : 'https://example.com/api/webhook'
                      }
                      value={url}
                      onChange={(e) => setUrl(e.target.value)}
                      required={!hasSavedWebhookUrl}
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

                  <Field
                    label="请求体模板 (JSON)"
                    hint={
                      hasSavedBody
                        ? '已保存自定义请求体模板；留空将保留原模板，输入新的 JSON 对象将覆盖'
                        : undefined
                    }
                  >
                    <Textarea
                      name="body"
                      rows={4}
                      placeholder={
                        hasSavedBody
                          ? '已保存自定义请求体模板，留空保持不变'
                          : '{\n  "title": "{title}",\n  "content": "{content}"\n}'
                      }
                      value={body}
                      onChange={(e) => setBody(e.target.value)}
                      className="font-mono text-xs"
                    />
                  </Field>

                  <Field
                    label="自定义请求头 (JSON)"
                    hint={
                      editingSameMethod
                        ? '留空将保留已保存请求头；输入 {} 可清空，输入新 JSON 对象将覆盖'
                        : undefined
                    }
                  >
                    <Textarea
                      name="headers"
                      rows={3}
                      placeholder={
                        editingSameMethod
                          ? '留空保持已存请求头不变，输入 {} 可清空'
                          : '{\n  "Authorization": "Bearer token"\n}'
                      }
                      value={headers}
                      onChange={(e) => setHeaders(e.target.value)}
                      className="font-mono text-xs"
                    />
                  </Field>
                </div>
              )}

              {method === 1 && (
                <div className={cn('space-y-3 pt-1 border-t', surface.divider)}>
                  <Field
                    label="Server酱 SendKey"
                    required={!hasSavedSendKey}
                    hint={hasSavedSendKey ? `已保存：${savedSecretHints.sendKey}，留空将保留原密钥` : undefined}
                  >
                    <Input
                      type="password"
                      placeholder={hasSavedSendKey ? '留空保持已保存的 SendKey' : 'SCTxxxxxxxx'}
                      value={sendKey}
                      onChange={(e) => setSendKey(e.target.value)}
                      required={!hasSavedSendKey}
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
                    required={!hasSavedWebhookUrl}
                    hint={hasSavedWebhookUrl ? `已保存：${savedSecretHints.url}，留空将保留原地址` : undefined}
                  >
                    <Input
                      placeholder={
                        hasSavedWebhookUrl
                          ? '留空保持已保存的 Webhook 地址'
                          : 'https://oapi.dingtalk.com/robot/send?access_token=...'
                      }
                      value={url}
                      onChange={(e) => setUrl(e.target.value)}
                      required={!hasSavedWebhookUrl}
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
                  <Field
                    label="应用Secret (CorpSecret)"
                    required={!hasSavedCorpSecret}
                    hint={
                      hasSavedCorpSecret
                        ? `已保存：${savedSecretHints.corpsecret}，留空将保留原密钥`
                        : undefined
                    }
                  >
                    <Input
                      type="password"
                      placeholder={hasSavedCorpSecret ? '留空保持已保存的 CorpSecret' : 'xxxxxxxx'}
                      value={corpsecret}
                      onChange={(e) => setCorpsecret(e.target.value)}
                      required={!hasSavedCorpSecret}
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
        <AlertDialog
          open={deleteTargetId !== null}
          onOpenChange={(o) => {
            if (!o && !deleting) {
              setDeleteTargetId(null);
              setDeleteError('');
            }
          }}
        >
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>确认删除此通知？</AlertDialogTitle>
              <AlertDialogDescription>
                删除后该渠道将不再接收任务完成或失败的推送通知。
              </AlertDialogDescription>
              {deleteError && <Alert>{deleteError}</Alert>}
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel disabled={deleting}>取消</AlertDialogCancel>
              <AlertDialogAction variant="destructive" disabled={deleting} onClick={handleDelete}>
                {deleting ? '删除中...' : '确认删除'}
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </div>
    </TooltipProvider>
  );
}
