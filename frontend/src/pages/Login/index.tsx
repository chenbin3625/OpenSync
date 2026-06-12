import { useEffect, useState } from 'react';
import './Login.css';
import { Form, Input, Button, Card, Modal, App, Typography, ConfigProvider, theme } from 'antd';
import { UserOutlined, LockOutlined, KeyOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router-dom';
import { getInitStatus, initializeUser, login, resetPwd } from '../../api/user';
import { useStore } from '../../stores/useStore';

const { Title, Text } = Typography;

export default function Login() {
  const navigate = useNavigate();
  const { setUserInfo, setAuthChecked } = useStore();
  const { message, modal } = App.useApp();
  const [loading, setLoading] = useState(false);
  const [checkingInit, setCheckingInit] = useState(true);
  const [initialized, setInitialized] = useState(true);
  const [resetVisible, setResetVisible] = useState(false);
  const [resetLoading, setResetLoading] = useState(false);
  const [form] = Form.useForm();
  const [resetForm] = Form.useForm();

  useEffect(() => {
    let active = true;
    getInitStatus()
      .then((res) => {
        if (!active) return;
        setInitialized(res.data.initialized);
        if (!res.data.initialized) {
          form.resetFields();
        }
      })
      .catch(() => {
        // Error handled by interceptor
      })
      .finally(() => {
        if (active) setCheckingInit(false);
      });
    return () => {
      active = false;
    };
  }, [form]);

  const handleLogin = async (values: { userName: string; passwd: string; confirmPasswd?: string }) => {
    setLoading(true);
    try {
      if (initialized) {
        const res = await login({ userName: values.userName, passwd: values.passwd });
        setUserInfo(res.data);
        setAuthChecked(true);
        message.success('登录成功');
        navigate('/home');
        return;
      }

      const res = await initializeUser({ userName: values.userName, passwd: values.passwd });
      const { recoveryKey, ...userInfo } = res.data;
      setUserInfo(userInfo);
      setAuthChecked(true);
      modal.info({
        title: '初始化成功',
        content: `请立即保存恢复密钥：${recoveryKey}。该密钥仅展示一次，忘记密码时需要使用。`,
        onOk: () => navigate('/home'),
      });
    } catch {
      // Error handled by interceptor
    } finally {
      setLoading(false);
    }
  };

  const handleReset = async (values: { userName: string; recoveryKey: string; passwd: string; confirmPasswd?: string }) => {
    setResetLoading(true);
    try {
      const res = await resetPwd({
        userName: values.userName,
        recoveryKey: values.recoveryKey,
        passwd: values.passwd,
      });
      modal.info({
        title: '密码重置成功',
        content: `请立即保存恢复密钥：${res.data}。旧恢复密钥已失效。`,
      });
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
            <Text type="secondary">{initialized ? 'AList 自动化同步工具' : '创建管理员账号'}</Text>
          </div>

          <Form form={form} onFinish={handleLogin} layout="vertical" size="large">
            <Form.Item name="userName" rules={[{ required: true, message: '请输入用户名' }]}>
              <Input prefix={<UserOutlined />} placeholder="用户名" />
            </Form.Item>
            <Form.Item name="passwd" rules={[{ required: true, message: '请输入密码' }]}>
              <Input.Password prefix={<LockOutlined />} placeholder="密码" />
            </Form.Item>
            {!initialized && (
              <Form.Item
                name="confirmPasswd"
                dependencies={['passwd']}
                rules={[
                  { required: true, message: '请确认密码' },
                  ({ getFieldValue }) => ({
                    validator(_, value) {
                      if (!value || getFieldValue('passwd') === value) {
                        return Promise.resolve();
                      }
                      return Promise.reject(new Error('两次输入的密码不一致'));
                    },
                  }),
                ]}
              >
                <Input.Password prefix={<LockOutlined />} placeholder="确认密码" />
              </Form.Item>
            )}
            <Form.Item>
              <Button type="primary" htmlType="submit" loading={checkingInit || loading} block>
                {initialized ? '登录' : '创建管理员账号'}
              </Button>
            </Form.Item>
            {initialized && (
              <Form.Item className="login-forgot">
                <Button type="link" onClick={() => setResetVisible(true)}>
                  忘记密码？
                </Button>
              </Form.Item>
            )}
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
            <Form.Item name="recoveryKey" rules={[{ required: true, message: '请输入恢复密钥' }]}>
              <Input prefix={<KeyOutlined />} placeholder="恢复密钥" />
            </Form.Item>
            <Form.Item name="passwd" rules={[{ required: true, message: '请输入新密码' }]}>
              <Input.Password prefix={<LockOutlined />} placeholder="新密码" />
            </Form.Item>
            <Form.Item
              name="confirmPasswd"
              dependencies={['passwd']}
              rules={[
                { required: true, message: '请确认新密码' },
                ({ getFieldValue }) => ({
                  validator(_, value) {
                    if (!value || getFieldValue('passwd') === value) {
                      return Promise.resolve();
                    }
                    return Promise.reject(new Error('两次输入的密码不一致'));
                  },
                }),
              ]}
            >
              <Input.Password prefix={<LockOutlined />} placeholder="确认新密码" />
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
