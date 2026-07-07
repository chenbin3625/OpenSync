import { useState, useEffect, useCallback, useRef } from 'react';
import {
  Card, Button, Modal, Form, Input, Space, Popconfirm, App, Empty, Typography, Descriptions, Tooltip,
} from 'antd';
import {
  PlusOutlined, EditOutlined, DeleteOutlined, CloudServerOutlined, ApiOutlined, ReloadOutlined,
} from '@ant-design/icons';
import { alistGet, alistGetPath, alistPost, alistPut, alistDelete } from '../../api/alist';
import dayjs from 'dayjs';
import type { AlistItem } from '../../types';

const { Text } = Typography;

interface EngineFormValues {
  url: string;
  remark?: string;
  token?: string;
}

const isLocalAlistHost = (host: string) => {
  const normalized = host.toLowerCase();
  return normalized === 'localhost' || normalized === '127.0.0.1' ||
    normalized === '::1' || normalized === '[::1]';
};

const validateAlistURL = (_: unknown, value?: string) => {
  if (!value) return Promise.resolve();
  try {
    const url = new URL(value);
    if (url.protocol === 'https:') return Promise.resolve();
    if (url.protocol === 'http:' && isLocalAlistHost(url.hostname)) return Promise.resolve();
  } catch {
    return Promise.reject(new Error('请输入合法 URL'));
  }
  return Promise.reject(new Error('非本机地址请使用 HTTPS'));
};

export default function Engine() {
  const { message } = App.useApp();
  const [list, setList] = useState<AlistItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [listError, setListError] = useState(false);
  const [modalVisible, setModalVisible] = useState(false);
  const [editingItem, setEditingItem] = useState<AlistItem | null>(null);
  const [form] = Form.useForm();
  const listReqRef = useRef(0);

  const fetchList = useCallback(async () => {
    const reqID = ++listReqRef.current;
    setLoading(true);
    setListError(false);
    try {
      const res = await alistGet();
      if (reqID !== listReqRef.current) return;
      setList(res.data || []);
    } catch (err) {
      if (reqID !== listReqRef.current) return;
      setList([]);
      setListError(true);
      console.error('alist fetchList failed', err);
    } finally {
      if (reqID === listReqRef.current) setLoading(false);
    }
  }, []);

  useEffect(() => { fetchList(); }, [fetchList]);

  const handleAdd = () => {
    setEditingItem(null);
    form.resetFields();
    setModalVisible(true);
  };

  const handleEdit = (item: AlistItem) => {
    setEditingItem(item);
    form.resetFields();
    form.setFieldsValue({ url: item.url, remark: item.remark || '', token: undefined });
    setModalVisible(true);
  };

  const handleDelete = async (id: number) => {
    try {
      await alistDelete(id);
      message.success('删除成功');
      fetchList();
    } catch (err) {
      console.error('alist delete failed', err);
    }
  };

  const handleTest = async (item: AlistItem) => {
    try {
      await alistGetPath(item.id, '/', { silent: true });
      message.success('连接测试成功');
    } catch (err) {
      console.error('alist test failed', err);
      message.error('连接测试失败');
    }
  };

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields() as EngineFormValues;
      const url = values.url.trim().replace(/\/+$/, '');
      if (editingItem) {
        await alistPut({ id: editingItem.id, url, remark: values.remark || null, token: values.token || undefined });
      } else {
        await alistPost({ url, remark: values.remark || null, token: values.token });
      }
      message.success(editingItem ? '更新成功' : '新增成功');
      setModalVisible(false);
      fetchList();
    } catch (err) {
      console.error('alist submit failed', err);
    }
  };

  return (
    <div className="ops-page-surface ops-resource-page">
      <div className="ops-page-header">
        <div className="ops-page-title-block">
          <h2 className="ops-page-title">引擎管理</h2>
          <Text className="ops-page-kicker">管理 AList / OpenList 连接和路径选择来源</Text>
        </div>
        <Space className="ops-page-actions">
          <Button type="primary" icon={<PlusOutlined />} onClick={handleAdd}>新增引擎</Button>
        </Space>
      </div>

      <div className="ops-page-main ops-page-panel">
        {listError ? (
          <div className="ops-empty-surface">
            <Empty
              image={Empty.PRESENTED_IMAGE_SIMPLE}
              description={<Text type="secondary">引擎列表加载失败</Text>}
            />
            <div style={{ marginTop: 16, textAlign: 'center' }}>
              <Button icon={<ReloadOutlined />} onClick={fetchList} loading={loading}>重试</Button>
            </div>
          </div>
        ) : list.length === 0 && !loading ? (
          <div className="ops-empty-surface">
            <Empty
              image={Empty.PRESENTED_IMAGE_SIMPLE}
              description={<Text type="secondary">暂无引擎，请先添加 AList 引擎实例</Text>}
            />
          </div>
        ) : (
          <div className="ops-resource-grid">
            {list.map((item) => (
              <Card
                className="ops-resource-card"
                hoverable
                actions={[
                  <Tooltip title="测试连接" key="test">
                    <Button type="text" size="small" icon={<ApiOutlined />} aria-label="测试连接" onClick={() => handleTest(item)} />
                  </Tooltip>,
                  <Tooltip title="编辑" key="edit">
                    <Button type="text" size="small" icon={<EditOutlined />} aria-label="编辑" onClick={() => handleEdit(item)} />
                  </Tooltip>,
                  <Popconfirm title="确认删除此引擎？" onConfirm={() => handleDelete(item.id)} key="del">
                    <Button type="text" size="small" danger icon={<DeleteOutlined />} aria-label="删除" />
                  </Popconfirm>,
                ]}
                key={item.id}
              >
                <div className="ops-resource-card-header">
                  <div className="ops-resource-title">
                    <span className="ops-resource-icon">
                      <CloudServerOutlined />
                    </span>
                    <span>
                      <Text strong className="ops-resource-name">{item.userName || 'AList'}</Text>
                      <Text type="secondary" className="ops-resource-meta">{item.remark || '未设置备注'}</Text>
                    </span>
                  </div>
                </div>

                <Descriptions column={1} size="small">
                  <Descriptions.Item label="地址">
                    <Text copyable>{item.url}</Text>
                  </Descriptions.Item>
                  <Descriptions.Item label="添加时间">
                    <Text type="secondary">
                      {item.createTime ? dayjs.unix(item.createTime).format('YYYY-MM-DD HH:mm') : '—'}
                    </Text>
                  </Descriptions.Item>
                </Descriptions>
              </Card>
            ))}
          </div>
        )}
      </div>

      <Modal
        title={editingItem ? '编辑引擎' : '新增引擎'}
        open={modalVisible}
        onOk={handleSubmit}
        onCancel={() => setModalVisible(false)}
        forceRender
      >
        <Form form={form} layout="vertical">
          <Form.Item
            name="url"
            label="地址"
            rules={[
              { required: true, message: '请输入AList地址' },
              { type: 'url' as const, message: '请输入合法 URL' },
              { validator: validateAlistURL },
            ]}
          >
            <Input placeholder="https://alist.example.com" />
          </Form.Item>
          <Form.Item name="remark" label="备注">
            <Input placeholder="可选备注" />
          </Form.Item>
          <Form.Item
            name="token"
            label="令牌"
            rules={editingItem ? [] : [{ required: true, message: '请输入令牌' }]}
          >
            <Input.Password placeholder={editingItem ? '留空则不修改' : 'AList令牌'} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
