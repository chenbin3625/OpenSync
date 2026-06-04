import { useState, useEffect, useCallback } from 'react';
import {
  Card, Row, Col, Button, Modal, Form, Input, Space, Popconfirm, App, Empty, Typography, Descriptions, Tooltip,
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
    <Card className="page-card">
      <div className="page-header">
        <h2>引擎管理</h2>
        <Space>
          <Button type="primary" icon={<PlusOutlined />} onClick={handleAdd}>新增引擎</Button>
        </Space>
      </div>

      {list.length === 0 && !loading ? (
        <Empty
          description={<Text type="secondary">暂无引擎，请先添加 AList 引擎实例</Text>}
        />
      ) : (
        <Row gutter={[16, 16]}>
          {list.map((item) => (
            <Col xs={24} md={12} key={item.id}>
              <Card
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
              >
                <Space>
                  <CloudServerOutlined />
                  <Text strong>{item.userName || 'AList'}</Text>
                  {item.remark && <Text type="secondary">{item.remark}</Text>}
                </Space>

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
            </Col>
          ))}
        </Row>
      )}

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
    </Card>
  );
}
