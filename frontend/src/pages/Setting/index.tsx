import { useState } from 'react';
import { Card, Form, Input, Button, App, Typography, Descriptions, Space } from 'antd';
import { UserOutlined, LockOutlined } from '@ant-design/icons';
import { editPwd } from '../../api/user';
import { useStore } from '../../stores/useStore';

const { Text } = Typography;

export default function Setting() {
  const { message } = App.useApp();
  const userInfo = useStore((s) => s.userInfo);
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (values: { passwd: string; oldPasswd: string }) => {
    setLoading(true);
    try {
      await editPwd(values);
      message.success('密码修改成功');
      form.resetFields();
    } catch { /* ignore */ }
    setLoading(false);
  };

  return (
    <Card className="page-card">
      <div className="page-header">
        <h2>系统设置</h2>
      </div>

      <Card title={<Space><UserOutlined />用户信息</Space>} style={{ marginBottom: 16 }}>
        <Descriptions column={1} size="small">
          <Descriptions.Item label="用户名">
            <Text strong>{userInfo?.userName}</Text>
          </Descriptions.Item>
        </Descriptions>
      </Card>

      <Card title={<Space><LockOutlined />修改密码</Space>}>
            <Form form={form} onFinish={handleSubmit} layout="vertical" style={{ maxWidth: 400 }}>
              <Form.Item name="oldPasswd" label="旧密码" rules={[{ required: true, message: '请输入旧密码' }]}>
                <Input.Password prefix={<LockOutlined />} placeholder="请输入旧密码" />
              </Form.Item>
              <Form.Item name="passwd" label="新密码" rules={[{ required: true, message: '请输入新密码' }]}>
                <Input.Password prefix={<LockOutlined />} placeholder="请输入新密码" />
              </Form.Item>
              <Form.Item
                name="confirm"
                label="确认密码"
                dependencies={['passwd']}
                rules={[
                  { required: true, message: '请确认密码' },
                  ({ getFieldValue }) => ({
                    validator(_, value) {
                      if (!value || getFieldValue('passwd') === value) return Promise.resolve();
                      return Promise.reject(new Error('两次密码不一致'));
                    },
                  }),
                ]}
              >
                <Input.Password prefix={<LockOutlined />} placeholder="再次输入新密码" />
              </Form.Item>
              <Form.Item>
                <Button type="primary" htmlType="submit" loading={loading}>
                  修改密码
                </Button>
              </Form.Item>
            </Form>
      </Card>
    </Card>
  );
}
