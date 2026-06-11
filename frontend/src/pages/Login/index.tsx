import { useState } from 'react';
import './Login.css';
import { Form, Input, Button, Card, Modal, App, Typography, ConfigProvider, theme } from 'antd';
import { UserOutlined, LockOutlined, KeyOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import { login, resetPwd } from '../../api/user';
import { useStore } from '../../stores/useStore';

const { Title, Text } = Typography;

export default function Login() {
  const navigate = useNavigate();
  const { setUserInfo, setAuthChecked } = useStore();
  const { message, modal } = App.useApp();
  const [loading, setLoading] = useState(false);
  const [resetVisible, setResetVisible] = useState(false);
  const [resetLoading, setResetLoading] = useState(false);
  const [form] = Form.useForm();
  const [resetForm] = Form.useForm();

  const handleLogin = async (values: { userName: string; passwd: string }) => {
    setLoading(true);
    try {
      const res = await login(values);
      setUserInfo(res.data);
      setAuthChecked(true);
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
      const res = await resetPwd(values);
      if (res.data) {
        modal.info({
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
    <ConfigProvider
      theme={{
        algorithm: theme.defaultAlgorithm,
        token: {
          colorPrimary: '#2563eb',
          borderRadius: 6,
        },
      }}
    >
      <div className="login-page">
        <div className="login-orbit login-orbit-primary" />
        <div className="login-orbit login-orbit-secondary" />
        <div className="login-backdrop-lines" />
        <Card
          className="login-card"
          styles={{ body: { padding: '32px 36px' } }}
        >
          <div className="login-brand">
            <img className="login-logo" src="/favicon.svg" alt="OpenSync" />
            <Title level={3}>OpenSync</Title>
            <Text type="secondary">AList 自动化同步工具</Text>
          </div>

          <Form form={form} onFinish={handleLogin} layout="vertical" size="large">
            <Form.Item name="userName" rules={[{ required: true, message: '请输入用户名' }]}>
              <Input prefix={<UserOutlined />} placeholder="用户名" />
            </Form.Item>
            <Form.Item name="passwd" rules={[{ required: true, message: '请输入密码' }]}>
              <Input.Password prefix={<LockOutlined />} placeholder="密码" />
            </Form.Item>
            <Form.Item>
              <Button type="primary" htmlType="submit" loading={loading} block>
                登录
              </Button>
            </Form.Item>
            <Form.Item className="login-forgot">
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
          forceRender
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
    </ConfigProvider>
  );
}
