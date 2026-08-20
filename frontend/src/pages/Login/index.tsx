import { useCallback, useEffect, useState } from 'react';
import './Login.css';
import { App, Button, Card, Form, Input, Modal, Typography } from 'antd';
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
  const [setupTokenRequired, setSetupTokenRequired] = useState(false);
  const [canSetup, setCanSetup] = useState(false);
  const [resetVisible, setResetVisible] = useState(false);
  const [resetLoading, setResetLoading] = useState(false);
  const [form] = Form.useForm();
  const [resetForm] = Form.useForm();

  const applyInitStatus = useCallback((data: { initialized: boolean; setupTokenRequired?: boolean }) => {
    setInitialized(data.initialized);
    if (data.initialized) {
      setSetupTokenRequired(false);
      setCanSetup(false);
      return;
    }
    setSetupTokenRequired(!!data.setupTokenRequired);
    setCanSetup(!data.setupTokenRequired);
    form.resetFields(['userName', 'passwd', 'confirmPasswd']);
  }, [form]);

  useEffect(() => {
    let active = true;
    getInitStatus()
      .then((res) => {
        if (!active) return;
        applyInitStatus(res.data);
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
  }, [applyInitStatus]);

  const handleSetupTokenBlur = async () => {
    const token = String(form.getFieldValue('setupToken') || '').trim();
    if (!token) {
      setCanSetup(false);
      setSetupTokenRequired(true);
      return;
    }
    try {
      const res = await getInitStatus(token);
      applyInitStatus(res.data);
    } catch {
      setCanSetup(false);
    }
  };

  const handleLogin = async (values: {
    userName: string;
    passwd: string;
    confirmPasswd?: string;
    setupToken?: string;
  }) => {
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

      const setupToken = String(values.setupToken || '').trim();
      if (!setupToken) {
        message.error('请输入 setup token（见服务器日志中的 OpenSync setup token）');
        return;
      }

      const res = await initializeUser({
        userName: values.userName,
        passwd: values.passwd,
        setupToken,
      });
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

  const showSetupForm = !initialized;

  return (
    <div className="login-page">
      <Card className="login-card">
        <div className="login-brand">
          <img className="login-logo" src="/favicon.svg" alt="OpenSync" />
          <Title level={3}>OpenSync</Title>
          <Text type="secondary">{initialized ? 'AList 自动化同步工具' : '创建管理员账号'}</Text>
        </div>

        <Form form={form} onFinish={handleLogin} layout="vertical" size="large">
          {showSetupForm && (
            <>
              <Form.Item
                name="setupToken"
                rules={[{ required: true, message: '请输入 setup token' }]}
                extra={setupTokenRequired ? '首次部署时，setup token 会写入服务器日志（OpenSync setup token）' : undefined}
              >
                <Input.Password
                  prefix={<KeyOutlined />}
                  placeholder="Setup Token"
                  onBlur={handleSetupTokenBlur}
                />
              </Form.Item>
              {!canSetup && setupTokenRequired && (
                <Text type="secondary">验证 setup token 后可创建管理员账号。</Text>
              )}
            </>
          )}
          <Form.Item name="userName" rules={[{ required: true, message: '请输入用户名' }]}>
            <Input prefix={<UserOutlined />} placeholder="用户名" disabled={showSetupForm && !canSetup} />
          </Form.Item>
          <Form.Item name="passwd" rules={[{ required: true, message: '请输入密码' }]}>
            <Input.Password prefix={<LockOutlined />} placeholder="密码" disabled={showSetupForm && !canSetup} />
          </Form.Item>
          {showSetupForm && (
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
              <Input.Password prefix={<LockOutlined />} placeholder="确认密码" disabled={!canSetup} />
            </Form.Item>
          )}
          <Form.Item>
            <Button
              type="primary"
              htmlType="submit"
              loading={checkingInit || loading}
              block
              disabled={showSetupForm && !canSetup}
            >
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
  );
}
