import { useState, useEffect, useCallback } from 'react';
import {
  Card, Button, Modal, Form, Input, Space, Popconfirm, App, Empty, Typography, Descriptions, Tooltip,
} from 'antd';
import {
  PlusOutlined, EditOutlined, DeleteOutlined, CloudServerOutlined, ApiOutlined,
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

export default function Engine() {
  const { message } = App.useApp();
  const [list, setList] = useState<AlistItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [modalVisible, setModalVisible] = useState(false);
  const [editingItem, setEditingItem] = useState<AlistItem | null>(null);
  const [form] = Form.useForm();

  const fetchList = useCallback(async () => {
    setLoading(true);
    try {
      const res = await alistGet();
      setList(res.data || []);
    } catch { /* ignore */ }
    setLoading(false);
  }, []);

  useEffect(() => { fetchList(); }, [fetchList]);

  const handleAdd = () => {
    setEditingItem(null);
    form.resetFields();
    setModalVisible(true);
  };

  const handleEdit = (item: AlistItem) => {
    setEditingItem(item);
    form.setFieldsValue({ url: item.url, remark: item.remark || '' });
    setModalVisible(true);
  };

  const handleDelete = async (id: number) => {
    try {
      await alistDelete(id);
      message.success('删除成功');
      fetchList();
    } catch { /* ignore */ }
  };

  const handleTest = async (item: AlistItem) => {
    try {
      const res = await alistGetPath(item.id, '/');
      if (res.code === 200) {
        message.success('连接测试成功');
      } else {
        message.error('连接失败: ' + (res.msg || '未知错误'));
      }
    } catch {
      message.error('连接测试失败');
    }
  };

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields() as EngineFormValues;
      let url = values.url;
      if (!url.startsWith('http://') && !url.startsWith('https://')) {
        url = 'http://' + url;
      }
      if (editingItem) {
        await alistPut({ id: editingItem.id, url, remark: values.remark || null, token: values.token || undefined });
      } else {
        await alistPost({ url, remark: values.remark || null, token: values.token });
      }
      message.success(editingItem ? '更新成功' : '新增成功');
      setModalVisible(false);
      fetchList();
    } catch { /* ignore */ }
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
        {list.length === 0 && !loading ? (
          <div className="ops-empty-surface">
            <Empty
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
                    <ApiOutlined onClick={() => handleTest(item)} />
                  </Tooltip>,
                  <Tooltip title="编辑" key="edit">
                    <EditOutlined onClick={() => handleEdit(item)} />
                  </Tooltip>,
                  <Popconfirm title="确认删除此引擎？" onConfirm={() => handleDelete(item.id)} key="del">
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
        destroyOnClose
      >
        <Form form={form} layout="vertical">
          <Form.Item name="url" label="地址" rules={[{ required: true, message: '请输入AList地址' }]}>
            <Input placeholder="http://localhost:5244" />
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
