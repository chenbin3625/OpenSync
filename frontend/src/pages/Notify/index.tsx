import { useState, useEffect, useCallback, useRef } from 'react';
import {
  Card, Button, Modal, Form, Input, Select, Switch, Space, Popconfirm, Tag, App, Empty, Typography, Descriptions, Tooltip,
} from 'antd';
import { PlusOutlined, EditOutlined, DeleteOutlined, SendOutlined, ReloadOutlined } from '@ant-design/icons';
import { notifyGet, notifyPost, notifyPut, notifyDelete } from '../../api/notify';
import dayjs from 'dayjs';
import type { NotifyFormValues, NotifyItem } from '../../types';

const { Text } = Typography;

const methodNames: Record<number, string> = {
  0: '自定义Webhook', 1: 'Server酱', 2: '钉钉', 3: '企业微信', 4: 'Lark (飞书)',
};

// Display a secret value, masking it client-side as defense in depth when the
// backend has not already redacted it (e.g. values entered during the same
// session before a refetch).
const displaySecret = (value: string): string => {
  if (!value) return '—';
  if (value.includes('****')) return value;
  return `****${value.slice(-4)}`;
};

const displayWebhookUrl = (value: string): string => {
  if (!value) return '—';
  if (value.includes('****')) return value;
  try {
    const url = new URL(value);
    let masked = false;
    url.searchParams.forEach((paramValue, key) => {
      const lowerKey = key.toLowerCase();
      if (lowerKey.includes('token') || lowerKey.includes('key') || lowerKey.includes('secret')) {
        url.searchParams.set(key, displaySecret(paramValue));
        masked = true;
      }
    });
    const parts = url.pathname.split('/').filter(Boolean);
    if (parts.length > 0 && parts[parts.length - 1].length >= 16) {
      parts[parts.length - 1] = displaySecret(parts[parts.length - 1]);
      url.pathname = `/${parts.join('/')}`;
      masked = true;
    }
    if (!masked && url.search) {
      url.search = '?****';
    }
    return url.toString().replace(/%2A/gi, '*');
  } catch {
    return displaySecret(value);
  }
};

// URL validation rules. Webhook URLs commonly embed access tokens, so require
// HTTPS for notification methods that store a webhook URL directly.
const urlRules = (requireHttps: boolean) => [
  { required: true, message: '请输入 URL' },
  { type: 'url' as const, message: '请输入合法 URL' },
  ...(requireHttps ? [{ pattern: /^https:\/\//i, message: '请使用 https URL 以保护 token' }] : []),
];

type NotifyParamValue = string | number | boolean | Record<string, unknown> | null | undefined;
type NotifyParams = Record<string, NotifyParamValue>;

const notifyParamKeysByMethod: Record<number, Array<keyof NotifyFormValues>> = {
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

const jsonObjectString = (value: unknown): string => {
  if (typeof value === 'string') {
    try {
      const parsed = parseOptionalJsonObject(value);
      return parsed ? JSON.stringify(parsed, null, 2) : '';
    } catch {
      return value;
    }
  }
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return '';
  }
  return JSON.stringify(value, null, 2);
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

const jsonObjectRule = {
  validator(_: unknown, value: string | undefined) {
    try {
      parseOptionalJsonObject(value);
      return Promise.resolve();
    } catch {
      return Promise.reject(new Error('请输入有效 JSON 对象'));
    }
  },
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

const getNotifyParamsFromValues = (values: NotifyFormValues): NotifyParams => {
  const params = pickNotifyParams(values.method, values as unknown as NotifyParams);
  return normalizeNotifyParams(values.method, params);
};

const getNotifyFormParams = (method: number, params: NotifyParams): NotifyParams => {
  const normalized = normalizeNotifyParams(method, params);
  const formParams = pickNotifyParams(method, normalized);
  if (method === 0) {
    formParams.httpMethod = paramString(normalized, 'method');
    formParams.body = jsonObjectString(normalized.body);
    formParams.headers = jsonObjectString(normalized.headers);
  }
  if (method === 2 || method === 4) {
    formParams.url = paramString(normalized, 'url');
  }
  return formParams;
};

const normalizeNotifyParams = (method: number, params: NotifyParams): NotifyParams => {
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

export default function Notify() {
  const { message } = App.useApp();
  const [list, setList] = useState<NotifyItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [listError, setListError] = useState(false);
  const [modalVisible, setModalVisible] = useState(false);
  const [editingItem, setEditingItem] = useState<NotifyItem | null>(null);
  const [form] = Form.useForm();
  const [method, setMethod] = useState(0);
  const [pendingNotifyValues, setPendingNotifyValues] = useState<Partial<NotifyFormValues> | null>(null);
  // Monotonic request counter so a stale list refetch (e.g. after rapid
  // toggle/delete) cannot overwrite a newer response.
  const listReqRef = useRef(0);

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

  useEffect(() => { fetchList(); }, [fetchList]);

  const handleAdd = () => {
    setEditingItem(null);
    form.resetFields();
    setPendingNotifyValues({ method: 0, enable: true });
    setMethod(0);
    setModalVisible(true);
  };

  const handleEdit = (item: NotifyItem) => {
    setEditingItem(item);
    let params: NotifyParams = {};
    try {
      const raw = asNotifyParams(JSON.parse(item.params || '{}'));
      params = getNotifyFormParams(item.method, raw);
    } catch (err) {
      // Stored params are malformed (migrated or externally edited). Fall back
      // to bare method/enable so the modal still opens instead of silently
      // aborting the edit action.
      console.error('notify params parse failed', err);
      message.error('通知配置解析失败，仅显示基本信息');
    }
    form.resetFields();
    setPendingNotifyValues({ ...params, method: item.method, enable: item.enable === 1 } as Partial<NotifyFormValues>);
    setMethod(item.method);
    setModalVisible(true);
  };

  useEffect(() => {
    if (!modalVisible || !pendingNotifyValues || pendingNotifyValues.method !== method) return;
    form.setFieldsValue(pendingNotifyValues);
    setPendingNotifyValues(null);
  }, [form, method, modalVisible, pendingNotifyValues]);

  const handleDelete = async (notifyId: number) => {
    try {
      await notifyDelete(notifyId);
      message.success('删除成功');
      fetchList();
    } catch (err) {
      console.error('notify delete failed', err);
    }
  };

  const handleToggleStatus = async (item: NotifyItem, checked: boolean) => {
    try {
      await notifyPut({ notifyId: item.id, enable: checked ? 1 : 0 });
      message.success('状态更新成功');
      fetchList();
    } catch (err) {
      console.error('notify toggle failed', err);
    }
  };

  const handleTest = async () => {
    try {
      const values = await form.validateFields() as NotifyFormValues;
      const m = values.method;
      // Send the id when editing so the backend can restore redacted secrets
      // from the stored config; for new configs the user enters real values.
      const notifyData = {
        ...(editingItem ? { id: editingItem.id } : {}),
        method: m,
        params: JSON.stringify(getNotifyParamsFromValues(values)),
      };
      await notifyPost({ notify: notifyData });
      message.success('测试消息已发送');
    } catch (err) {
      console.error('notify test failed', err);
    }
  };

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields() as NotifyFormValues;
      const m = values.method;
      const notifyData = {
        ...(editingItem ? { id: editingItem.id } : {}),
        enable: values.enable ? 1 : 0,
        method: m,
        params: JSON.stringify(getNotifyParamsFromValues(values)),
      };
      if (editingItem) {
        await notifyPut({ notify: notifyData });
      } else {
        await notifyPost({ notify: notifyData });
      }
      message.success(editingItem ? '更新成功' : '新增成功');
      setModalVisible(false);
      fetchList();
    } catch (err) {
      console.error('notify submit failed', err);
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

  const handleTestSend = async (item: NotifyItem) => {
    try {
      // Send the id so the backend tests with the stored (real) config; the
      // list params are redacted and would fail if sent verbatim.
      await notifyPost({ notify: { id: item.id, method: item.method, params: item.params } });
      message.success('测试消息已发送');
    } catch (err) {
      console.error('notify test send failed', err);
      message.error('发送失败');
    }
  };

  const handleMethodChange = (nextMethod: number) => {
    const enable = form.getFieldValue('enable') ?? true;
    form.resetFields();
    form.setFieldsValue({ method: nextMethod, enable });
    setMethod(nextMethod);
  };

  const renderMethodFields = () => {
    switch (method) {
      case 0:
        return (
          <>
            <Form.Item name="url" label="URL" rules={urlRules(true)}><Input /></Form.Item>
            <Form.Item name="httpMethod" label="HTTP方法" initialValue="POST">
              <Select options={[{ value: 'GET' }, { value: 'POST' }, { value: 'PUT' }]} />
            </Form.Item>
            <Form.Item name="contentType" hidden initialValue="application/json"><Input /></Form.Item>
            <Form.Item name="needContent" hidden initialValue={true}><Input /></Form.Item>
            <Form.Item name="titleName" hidden initialValue="title"><Input /></Form.Item>
            <Form.Item name="contentName" hidden initialValue="content"><Input /></Form.Item>
            <Form.Item name="body" label="请求体模板" rules={[jsonObjectRule]}>
              <Input.TextArea rows={5} placeholder={'{\n  "title": "{title}",\n  "content": "{content}"\n}'} />
            </Form.Item>
            <Form.Item name="headers" label="请求头 JSON" rules={[jsonObjectRule]}>
              <Input.TextArea rows={4} placeholder={'{\n  "Authorization": "Bearer token"\n}'} />
            </Form.Item>
            <Form.Item name="notSendNull" label="无需同步时不发送" valuePropName="checked"><Switch /></Form.Item>
          </>
        );
      case 1:
        return (
          <>
            <Form.Item name="sendKey" label="SendKey" rules={[{ required: true }]}><Input /></Form.Item>
            <Form.Item name="version" label="版本" initialValue="v3">
              <Select options={[{ value: 'v1', label: 'v1' }, { value: 'v3', label: 'v3' }]} />
            </Form.Item>
            <Form.Item name="notSendNull" label="无需同步时不发送" valuePropName="checked"><Switch /></Form.Item>
          </>
        );
      case 2:
        return (
          <>
            <Form.Item name="url" label="Webhook URL" rules={urlRules(true)}><Input /></Form.Item>
            <Form.Item name="notSendNull" label="无需同步时不发送" valuePropName="checked"><Switch /></Form.Item>
          </>
        );
      case 3:
        return (
          <>
            <Form.Item name="corpid" label="企业ID" rules={[{ required: true }]}><Input /></Form.Item>
            <Form.Item name="corpsecret" label="应用密钥" rules={[{ required: true }]}><Input.Password /></Form.Item>
            <Form.Item name="agentid" label="应用ID" rules={[{ required: true }]}><Input /></Form.Item>
            <Form.Item name="touser" label="发送用户" initialValue="@all"><Input /></Form.Item>
            <Form.Item name="notSendNull" label="无需同步时不发送" valuePropName="checked"><Switch /></Form.Item>
          </>
        );
      case 4:
        return (
          <>
            <Form.Item name="url" label="Webhook URL" rules={urlRules(true)}><Input /></Form.Item>
            <Form.Item name="notSendNull" label="无需同步时不发送" valuePropName="checked"><Switch /></Form.Item>
          </>
        );
      default: return null;
    }
  };

  return (
    <div className="ops-page-surface ops-resource-page">
      <div className="ops-page-header">
        <div className="ops-page-title-block">
          <h2 className="ops-page-title">通知配置</h2>
          <Text className="ops-page-kicker">配置任务完成、失败和无需同步时的消息渠道</Text>
        </div>
        <Space className="ops-page-actions">
          <Button type="primary" icon={<PlusOutlined />} onClick={handleAdd}>新增通知</Button>
        </Space>
      </div>

      <div className="ops-page-main ops-page-panel">
        {listError ? (
          <div className="ops-empty-surface">
            <Empty
              image={Empty.PRESENTED_IMAGE_SIMPLE}
              description={<Text type="secondary">通知配置加载失败</Text>}
            />
            <div className="ops-empty-action">
              <Button icon={<ReloadOutlined />} onClick={fetchList} loading={loading}>重试</Button>
            </div>
          </div>
        ) : list.length === 0 && !loading ? (
          <div className="ops-empty-surface">
            <Empty
              image={Empty.PRESENTED_IMAGE_SIMPLE}
              description={<Text type="secondary">暂无通知渠道配置，添加后可在任务完成时接收通知</Text>}
            />
          </div>
        ) : (
          <div className="ops-resource-grid">
            {list.map((item) => {
              const params = parseParams(item);
              return (
                <Card
                  className="ops-resource-card"
                  hoverable
                  actions={[
                    <Tooltip title="测试发送" key="test">
                      <Button type="text" size="small" icon={<SendOutlined />} aria-label="测试发送" onClick={() => handleTestSend(item)} />
                    </Tooltip>,
                    <Tooltip title="编辑" key="edit">
                      <Button type="text" size="small" icon={<EditOutlined />} aria-label="编辑" onClick={() => handleEdit(item)} />
                    </Tooltip>,
                    <Popconfirm title="确认删除此通知？" onConfirm={() => handleDelete(item.id)} key="del">
                      <Button type="text" size="small" danger icon={<DeleteOutlined />} aria-label="删除" />
                    </Popconfirm>,
                  ]}
                  key={item.id}
                >
                  <div className="ops-resource-card-header">
                    <div className="ops-resource-title">
                      <span className="ops-resource-icon">
                        <SendOutlined />
                      </span>
                      <span>
                        <Text strong className="ops-resource-name">{methodNames[item.method] || `方式${item.method}`}</Text>
                        <Text type="secondary" className="ops-resource-meta">{getParamSummary(item)}</Text>
                      </span>
                    </div>
                    <Space>
                      <Tag color={item.enable === 1 ? 'success' : 'default'}>
                        {item.enable === 1 ? '已启用' : '已禁用'}
                      </Tag>
                      <Switch
                        checked={item.enable === 1}
                        onChange={(checked) => handleToggleStatus(item, checked)}
                        size="small"
                        aria-label={`${item.enable === 1 ? '禁用' : '启用'}${methodNames[item.method] || '通知'}`}
                      />
                    </Space>
                  </div>

                  <Descriptions column={1} size="small">
                    <Descriptions.Item label="配置">
                      <Text type="secondary" ellipsis>{getParamSummary(item)}</Text>
                    </Descriptions.Item>
                    <Descriptions.Item label="静默通知">
                      <Text type="secondary">{params.notSendNull ? '无需同步时不发送' : '始终发送'}</Text>
                    </Descriptions.Item>
                    <Descriptions.Item label="添加时间">
                      <Text type="secondary">
                        {item.createTime ? dayjs.unix(item.createTime).format('YYYY-MM-DD HH:mm') : '—'}
                      </Text>
                    </Descriptions.Item>
                  </Descriptions>
                </Card>
              );
            })}
          </div>
        )}
      </div>

      <Modal
        title={editingItem ? '编辑通知' : '新增通知'}
        open={modalVisible}
        onOk={handleSubmit}
        onCancel={() => setModalVisible(false)}
        width={520}
        forceRender
        footer={(
          <Space>
            <Button onClick={() => setModalVisible(false)}>取消</Button>
            <Button icon={<SendOutlined />} onClick={handleTest}>测试</Button>
            <Button type="primary" onClick={handleSubmit}>确定</Button>
          </Space>
        )}
      >
        <Form form={form} layout="vertical">
          <Form.Item name="method" label="通知方式" rules={[{ required: true }]}>
            <Select onChange={handleMethodChange} options={Object.entries(methodNames).map(([k, v]) => ({ value: Number(k), label: v }))} />
          </Form.Item>
          <Form.Item name="enable" label="启用" valuePropName="checked"><Switch /></Form.Item>
          {renderMethodFields()}
        </Form>
      </Modal>
    </div>
  );
}
