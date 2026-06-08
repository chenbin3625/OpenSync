import { useState, useEffect, useCallback } from 'react';
import {
  Card, Button, Modal, Form, Input, Select, Switch, Space, Popconfirm, Tag, App, Empty, Typography, Descriptions, Tooltip,
} from 'antd';
import { PlusOutlined, EditOutlined, DeleteOutlined, SendOutlined } from '@ant-design/icons';
import { notifyGet, notifyPost, notifyPut, notifyDelete } from '../../api/notify';
import dayjs from 'dayjs';
import type { NotifyFormValues, NotifyItem } from '../../types';

const { Text } = Typography;

const methodNames: Record<number, string> = {
  0: '自定义Webhook', 1: 'Server酱', 2: '钉钉', 3: '企业微信', 4: 'Lark (飞书)',
};

type NotifyParams = Record<string, string | number | boolean | null | undefined>;

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

const paramsFromValues = (values: NotifyFormValues): NotifyParams => {
  const params: NotifyParams = { ...values };
  delete params.method;
  delete params.enable;
  return params;
};

const normalizeNotifyParams = (method: number, params: NotifyParams): NotifyParams => {
  const normalized = { ...params };
  if (method === 0) {
    normalized.method = normalized.method || normalized.httpMethod || 'POST';
    normalized.contentType = normalized.contentType || 'application/json';
    normalized.needContent = normalized.needContent ?? true;
    normalized.titleName = normalized.titleName || 'title';
    normalized.contentName = normalized.contentName || 'content';
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
  const [modalVisible, setModalVisible] = useState(false);
  const [editingItem, setEditingItem] = useState<NotifyItem | null>(null);
  const [form] = Form.useForm();
  const [method, setMethod] = useState(0);

  const fetchList = useCallback(async () => {
    setLoading(true);
    try {
      const res = await notifyGet();
      setList(res.data || []);
    } catch { /* ignore */ }
    setLoading(false);
  }, []);

  useEffect(() => { fetchList(); }, [fetchList]);

  const handleAdd = () => {
    setEditingItem(null);
    form.resetFields();
    form.setFieldsValue({ method: 0, enable: true });
    setMethod(0);
    setModalVisible(true);
  };

  const handleEdit = (item: NotifyItem) => {
    setEditingItem(item);
    let params: NotifyParams = {};
    try { params = asNotifyParams(JSON.parse(item.params || '{}')); } catch { /* ignore */ }
    params = normalizeNotifyParams(item.method, params);
    if (item.method === 0) {
      params.httpMethod = paramString(params, 'method');
    }
    form.setFieldsValue({ ...params, method: item.method, enable: item.enable === 1 });
    setMethod(item.method);
    setModalVisible(true);
  };

  const handleDelete = async (notifyId: number) => {
    try {
      await notifyDelete(notifyId);
      message.success('删除成功');
      fetchList();
    } catch { /* ignore */ }
  };

  const handleToggleStatus = async (item: NotifyItem, checked: boolean) => {
    try {
      await notifyPut({ notifyId: item.id, enable: checked ? 1 : 0 });
      message.success('状态更新成功');
      fetchList();
    } catch { /* ignore */ }
  };

  const handleTest = async () => {
    try {
      const values = await form.validateFields() as NotifyFormValues;
      const m = values.method;
      const params = paramsFromValues(values);
      await notifyPost({ notify: { method: m, params: JSON.stringify(normalizeNotifyParams(m, params)) } });
      message.success('测试消息已发送');
    } catch { /* ignore */ }
  };

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields() as NotifyFormValues;
      const m = values.method;
      const params = paramsFromValues(values);
      const notifyData = {
        ...(editingItem ? { id: editingItem.id } : {}),
        enable: values.enable ? 1 : 0,
        method: m,
        params: JSON.stringify(normalizeNotifyParams(m, params)),
      };
      if (editingItem) {
        await notifyPut({ notify: notifyData });
      } else {
        await notifyPost({ notify: notifyData });
      }
      message.success(editingItem ? '更新成功' : '新增成功');
      setModalVisible(false);
      fetchList();
    } catch { /* ignore */ }
  };

  const parseParams = (item: NotifyItem): NotifyParams => {
    try { return asNotifyParams(JSON.parse(item.params || '{}')); } catch { return {}; }
  };

  const getParamSummary = (item: NotifyItem) => {
    const p = parseParams(item);
    switch (item.method) {
      case 0: return paramString(p, 'url') || '—';
      case 1: {
        const sendKey = paramString(p, 'sendKey');
        return sendKey ? `****${sendKey.slice(-4)}` : '—';
      }
      case 2: return paramString(p, 'url') || paramString(p, 'webhook') || '—';
      case 3: return paramString(p, 'corpid') || paramString(p, 'corpId') || '—';
      case 4: return paramString(p, 'url') || paramString(p, 'webhook') || '—';
      default: return '—';
    }
  };

  const handleTestSend = async (item: NotifyItem) => {
    try {
      await notifyPost({ notify: { method: item.method, params: item.params } });
      message.success('测试消息已发送');
    } catch {
      message.error('发送失败');
    }
  };

  const renderMethodFields = () => {
    switch (method) {
      case 0:
        return (
          <>
            <Form.Item name="url" label="URL" rules={[{ required: true }]}><Input /></Form.Item>
            <Form.Item name="httpMethod" label="HTTP方法" initialValue="POST">
              <Select options={[{ value: 'GET' }, { value: 'POST' }, { value: 'PUT' }]} />
            </Form.Item>
            <Form.Item name="contentType" hidden initialValue="application/json"><Input /></Form.Item>
            <Form.Item name="needContent" hidden initialValue={true}><Input /></Form.Item>
            <Form.Item name="titleName" hidden initialValue="title"><Input /></Form.Item>
            <Form.Item name="contentName" hidden initialValue="content"><Input /></Form.Item>
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
            <Form.Item name="url" label="Webhook URL" rules={[{ required: true }]}><Input /></Form.Item>
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
            <Form.Item name="url" label="Webhook URL" rules={[{ required: true }]}><Input /></Form.Item>
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
        {list.length === 0 && !loading ? (
          <div className="ops-empty-surface">
            <Empty
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
                      <SendOutlined onClick={() => handleTestSend(item)} />
                    </Tooltip>,
                    <Tooltip title="编辑" key="edit">
                      <EditOutlined onClick={() => handleEdit(item)} />
                    </Tooltip>,
                    <Popconfirm title="确认删除此通知？" onConfirm={() => handleDelete(item.id)} key="del">
                      <Tooltip title="删除">
                        <DeleteOutlined />
                      </Tooltip>
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
        destroyOnClose
        width={520}
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
            <Select onChange={(v) => setMethod(v)} options={Object.entries(methodNames).map(([k, v]) => ({ value: Number(k), label: v }))} />
          </Form.Item>
          <Form.Item name="enable" label="启用" valuePropName="checked"><Switch /></Form.Item>
          {renderMethodFields()}
        </Form>
      </Modal>
    </div>
  );
}
