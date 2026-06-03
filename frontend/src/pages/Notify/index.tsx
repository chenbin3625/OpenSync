import { useState, useEffect } from 'react';
import {
  Card, Row, Col, Button, Modal, Form, Input, Select, Switch, Space, Popconfirm, Tag, App, Empty, Typography, Descriptions, Tooltip,
} from 'antd';
import { PlusOutlined, EditOutlined, DeleteOutlined, BellOutlined, SendOutlined, PoweroffOutlined } from '@ant-design/icons';
import { notifyGet, notifyPost, notifyPut, notifyDelete } from '../../api/notify';
import dayjs from 'dayjs';

const { Text } = Typography;

const methodNames: Record<number, string> = {
  0: '自定义Webhook', 1: 'Server酱', 2: '钉钉', 3: '企业微信', 4: 'Lark (飞书)',
};

const methodIcons: Record<number, string> = {
  0: '🔗', 1: '🐿️', 2: '💬', 3: '🏢', 4: '🐦',
};

const methodColors: Record<number, string[]> = {
  0: ['#722ed1', '#531dab'],
  1: ['#fa541c', '#d4380d'],
  2: ['#1677ff', '#0958d9'],
  3: ['#13c2c2', '#08979c'],
  4: ['#52c41a', '#389e0d'],
};

const normalizeNotifyParams = (method: number, params: Record<string, any>) => {
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
  const [list, setList] = useState<any[]>([]);
  const [loading, setLoading] = useState(false);
  const [modalVisible, setModalVisible] = useState(false);
  const [editingItem, setEditingItem] = useState<any>(null);
  const [form] = Form.useForm();
  const [method, setMethod] = useState(0);

  const fetchList = async () => {
    setLoading(true);
    try {
      const res: any = await notifyGet();
      setList(res.data || []);
    } catch { /* ignore */ }
    setLoading(false);
  };

  useEffect(() => { fetchList(); }, []);

  const handleAdd = () => {
    setEditingItem(null);
    form.resetFields();
    form.setFieldsValue({ method: 0, enable: true });
    setMethod(0);
    setModalVisible(true);
  };

  const handleEdit = (item: any) => {
    setEditingItem(item);
    let params: any = {};
    try { params = JSON.parse(item.params || '{}'); } catch { /* ignore */ }
    params = normalizeNotifyParams(item.method, params);
    if (item.method === 0) {
      params.httpMethod = params.method;
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

  const handleToggleStatus = async (item: any) => {
    try {
      await notifyPut({ notifyId: item.id, enable: item.enable === 1 ? 0 : 1 });
      message.success('状态更新成功');
      fetchList();
    } catch { /* ignore */ }
  };

  const handleTest = async () => {
    try {
      const values = await form.validateFields();
      const { method: m, enable: _, ...params } = values;
      await notifyPost({ notify: { method: m, params: JSON.stringify(normalizeNotifyParams(m, params)) } });
      message.success('测试消息已发送');
    } catch { /* ignore */ }
  };

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();
      const { method: m, enable: e, ...params } = values;
      const notifyData = {
        ...(editingItem ? { id: editingItem.id } : {}),
        enable: e ? 1 : 0,
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

  const parseParams = (item: any) => {
    try { return JSON.parse(item.params || '{}'); } catch { return {}; }
  };

  const getParamSummary = (item: any) => {
    const p = parseParams(item);
    switch (item.method) {
      case 0: return p.url || '—';
      case 1: return p.sendKey ? `****${p.sendKey.slice(-4)}` : '—';
      case 2: return p.url || p.webhook || '—';
      case 3: return p.corpid || p.corpId || '—';
      case 4: return p.url || p.webhook || '—';
      default: return '—';
    }
  };

  const handleTestSend = async (item: any) => {
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
    <Card>
      <div className="page-header">
        <h2>通知配置</h2>
        <Space>
          <Button type="primary" icon={<PlusOutlined />} onClick={handleAdd}>新增通知</Button>
        </Space>
      </div>

      {list.length === 0 && !loading ? (
        <Empty
          image={<BellOutlined style={{ fontSize: 64, color: 'var(--ant-color-text-quaternary)' }} />}
          styles={{ image: { height: 80 } }}
          description={<Text type="secondary">暂无通知渠道配置，添加后可在任务完成时接收通知</Text>}
          className="empty-state"
        />
      ) : (
        <Row gutter={[16, 16]}>
          {list.map((item: any) => {
            const params = parseParams(item);
            const colors = methodColors[item.method] || ['#8c8c8c', '#595959'];
            return (
              <Col xs={24} md={12} key={item.id}>
                <Card
                  hoverable
                  styles={{ body: { padding: '20px 24px' } }}
                  actions={[
                    <Tooltip title="测试发送" key="test">
                      <SendOutlined onClick={() => handleTestSend(item)} style={{ color: '#1677ff' }} />
                    </Tooltip>,
                    <Tooltip title="编辑" key="edit">
                      <EditOutlined onClick={() => handleEdit(item)} />
                    </Tooltip>,
                    <Popconfirm title={item.enable === 1 ? '确认禁用？' : '确认启用？'} onConfirm={() => handleToggleStatus(item)} key="toggle">
                      <Tooltip title={item.enable === 1 ? '禁用' : '启用'}>
                        <PoweroffOutlined style={{ color: item.enable === 1 ? '#faad14' : '#52c41a' }} />
                      </Tooltip>
                    </Popconfirm>,
                    <Popconfirm title="确认删除此通知？" onConfirm={() => handleDelete(item.id)} key="del">
                      <Tooltip title="删除">
                        <DeleteOutlined style={{ color: '#ff4d4f' }} />
                      </Tooltip>
                    </Popconfirm>,
                  ]}
                >
                  <div className="card-item-header">
                    <div className="card-item-icon" style={{
                      background: `linear-gradient(135deg, ${colors[0]} 0%, ${colors[1]} 100%)`,
                      fontSize: 22,
                    }}>
                      {methodIcons[item.method] || '📢'}
                    </div>
                    <div className="card-item-info">
                      <div className="card-item-title">
                        {methodNames[item.method] || `方式${item.method}`}
                      </div>
                      <div style={{ marginTop: 3 }}>
                        <Tag color={item.enable === 1 ? 'success' : 'default'} style={{ margin: 0, fontSize: 11 }}>
                          {item.enable === 1 ? '已启用' : '已禁用'}
                        </Tag>
                      </div>
                    </div>
                  </div>

                  <Descriptions column={1} size="small" styles={{ label: { color: 'var(--ant-color-text-secondary)', width: 70 }, content: { fontSize: 13 } }}>
                    <Descriptions.Item label="配置">
                      <Text type="secondary" ellipsis style={{ fontSize: 13, maxWidth: 200 }}>{getParamSummary(item)}</Text>
                    </Descriptions.Item>
                    <Descriptions.Item label="静默通知">
                      <Text type="secondary" className="desc-text-sm">{params.notSendNull ? '无需同步时不发送' : '始终发送'}</Text>
                    </Descriptions.Item>
                    <Descriptions.Item label="添加时间">
                      <Text type="secondary" className="desc-text-sm">
                        {item.createTime ? dayjs.unix(item.createTime).format('YYYY-MM-DD HH:mm') : '—'}
                      </Text>
                    </Descriptions.Item>
                  </Descriptions>
                </Card>
              </Col>
            );
          })}
        </Row>
      )}

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
        <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
          <Form.Item name="method" label="通知方式" rules={[{ required: true }]}>
            <Select onChange={(v) => setMethod(v)} options={Object.entries(methodNames).map(([k, v]) => ({ value: Number(k), label: v }))} />
          </Form.Item>
          <Form.Item name="enable" label="启用" valuePropName="checked"><Switch /></Form.Item>
          {renderMethodFields()}
        </Form>
      </Modal>
    </Card>
  );
}
