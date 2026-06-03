import { useState } from 'react';
import { Form, Input, Button, Card, Modal, App, Typography, theme } from 'antd';
import { UserOutlined, LockOutlined, KeyOutlined, SyncOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import { login, resetPwd } from '../../api/user';
import { useStore } from '../../stores/useStore';

const { Title, Text } = Typography;

export default function Login() {
  const navigate = useNavigate();
  const { setUserInfo } = useStore();
  const { message } = App.useApp();
  const { token } = theme.useToken();
  const [loading, setLoading] = useState(false);
  const [resetVisible, setResetVisible] = useState(false);
  const [resetLoading, setResetLoading] = useState(false);
  const [form] = Form.useForm();
  const [resetForm] = Form.useForm();

  const handleLogin = async (values: { userName: string; passwd: string }) => {
    setLoading(true);
    try {
      const res = await login(values) as unknown as { data: { id: number; userName: string; createTime: number } };
      setUserInfo(res.data);
      message.success('登录成功');
      navigate('/home');
    } catch {
      // Error handled by interceptor
    } finally {
      setLoading(false);
    }
  };

  const handleReset = async (values: { userName: string; key: string; passwd?: string }) => {
    setResetLoading(true);
    try {
      const res = await resetPwd(values) as unknown as { data?: string };
      if (res.data) {
        Modal.info({
          title: '重置成功',
          content: `新密码为：${res.data}，请妥善保管。`,
        });
      } else {
        message.success('密码重置成功');
      }
      setResetVisible(false);
      resetForm.resetFields();
    } catch {
      // Error handled by interceptor
    } finally {
      setResetLoading(false);
    }
  };

  return (
    <div style={{
      display: 'flex',
      justifyContent: 'center',
      alignItems: 'center',
      minHeight: '100vh',
      background: `linear-gradient(135deg, ${token.colorPrimaryBg} 0%, ${token.colorBgLayout} 50%, ${token.colorBgContainer} 100%)`,
    }}>
      <Card
        style={{
          width: 420,
          borderRadius: token.borderRadiusLG,
          boxShadow: token.boxShadowSecondary,
        }}
        styles={{ body: { padding: '32px 36px' } }}
      >
        <div style={{ textAlign: 'center', marginBottom: 32 }}>
          <SyncOutlined style={{ fontSize: 40, color: token.colorPrimary, marginBottom: 12 }} spin />
          <Title level={3} style={{ margin: 0 }}>OpenSync</Title>
          <Text type="secondary">AList 自动化同步工具</Text>
        </div>

        <Form form={form} onFinish={handleLogin} layout="vertical" size="large">
          <Form.Item name="userName" rules={[{ required: true, message: '请输入用户名' }]}>
            <Input prefix={<UserOutlined />} placeholder="用户名" />
          </Form.Item>
          <Form.Item name="passwd" rules={[{ required: true, message: '请输入密码' }]}>
            <Input.Password prefix={<LockOutlined />} placeholder="密码" />
          </Form.Item>
          <Form.Item style={{ marginBottom: 12 }}>
            <Button type="primary" htmlType="submit" loading={loading} block>
              登录
            </Button>
          </Form.Item>
          <Form.Item style={{ textAlign: 'center', marginBottom: 0 }}>
            <Button type="link" onClick={() => setResetVisible(true)}>
              忘记密码？
            </Button>
          </Form.Item>
        </Form>
      </Card>

      <Modal
        title="重置密码"
        open={resetVisible}
        onCancel={() => setResetVisible(false)}
        footer={null}
      >
        <Form form={resetForm} onFinish={handleReset} layout="vertical">
          <Form.Item name="userName" rules={[{ required: true, message: '请输入用户名' }]}>
            <Input prefix={<UserOutlined />} placeholder="用户名" />
          </Form.Item>
          <Form.Item name="key" rules={[{ required: true, message: '请输入加密秘钥' }]}>
            <Input prefix={<KeyOutlined />} placeholder="加密秘钥 (data/secret.key)" />
          </Form.Item>
          <Form.Item name="passwd">
            <Input.Password prefix={<LockOutlined />} placeholder="新密码 (留空则自动生成)" />
          </Form.Item>
          <Form.Item>
            <Button type="primary" htmlType="submit" loading={resetLoading} block>
              重置密码
            </Button>
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
