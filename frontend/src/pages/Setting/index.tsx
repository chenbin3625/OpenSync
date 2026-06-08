import { useCallback, useEffect, useState } from 'react';
import {
  App, Button, Card, Col, Form, Input, InputNumber, Modal, Row, Space, Tooltip,
} from 'antd';
import { LockOutlined, QuestionCircleOutlined, SaveOutlined } from '@ant-design/icons';
import { editPwd } from '../../api/user';
import { getSystemConfig, updateSystemConfig } from '../../api/system';
import type { SystemSettings } from '../../types';

type PasswordFormValues = {
  oldPasswd: string;
  passwd: string;
  confirm: string;
};

const labelWithTip = (label: string, tip: string) => (
  <Space size={4}>
    {label}
    <Tooltip title={tip}>
      <QuestionCircleOutlined style={{ color: '#8c8c8c' }} />
    </Tooltip>
  </Space>
);

export default function Setting() {
  const { message } = App.useApp();
  const [configForm] = Form.useForm<SystemSettings>();
  const [passwordForm] = Form.useForm<PasswordFormValues>();
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [passwordVisible, setPasswordVisible] = useState(false);
  const [passwordSaving, setPasswordSaving] = useState(false);

  const fetchConfig = useCallback(async () => {
    setLoading(true);
    try {
      const res = await getSystemConfig();
      if (res.data) {
        configForm.setFieldsValue(res.data);
      }
    } catch { /* ignore */ }
    setLoading(false);
  }, [configForm]);

  useEffect(() => { fetchConfig(); }, [fetchConfig]);

  const handleSaveConfig = async (values: SystemSettings) => {
    setSaving(true);
    try {
      const res = await updateSystemConfig(values);
      if (res.data) {
        configForm.setFieldsValue(res.data);
      }
      message.success('系统配置已保存');
    } catch { /* ignore */ }
    setSaving(false);
  };

  const handleChangePassword = async () => {
    try {
      const values = await passwordForm.validateFields();
      setPasswordSaving(true);
      await editPwd({ oldPasswd: values.oldPasswd, passwd: values.passwd });
      message.success('密码修改成功');
      passwordForm.resetFields();
      setPasswordVisible(false);
    } catch { /* ignore */ }
    setPasswordSaving(false);
  };

  return (
    <div className="ops-page-surface ops-resource-page">
      <div className="ops-page-header">
        <div className="ops-page-title-block">
          <h2 className="ops-page-title">系统设置</h2>
          <span className="ops-page-kicker">调整运行参数并维护管理员密码</span>
        </div>
        <div className="ops-page-actions">
          <Button icon={<LockOutlined />} onClick={() => setPasswordVisible(true)}>
            修改密码
          </Button>
        </div>
      </div>

      <div className="ops-page-main">
        <Card className="ops-settings-panel" title={<span className="ops-section-title">运行配置</span>} loading={loading}>
          <Form
            form={configForm}
            layout="vertical"
            onFinish={handleSaveConfig}
          >
            <Row gutter={16}>
              <Col xs={24} md={12}>
                <Form.Item
                  name="expires"
                  label={labelWithTip('登录有效期', '登录 Cookie 的有效天数，保存后对后续登录或刷新 Cookie 生效。')}
                  rules={[{ required: true, message: '请输入登录有效期' }]}
                >
                  <InputNumber min={1} max={365} addonAfter="天" style={{ width: '100%' }} />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item
                  name="taskTimeout"
                  label={labelWithTip('任务超时时间', '单次同步任务允许运行的最长时间，0 表示不设置超时；新任务生效。')}
                  rules={[{ required: true, message: '请输入任务超时时间' }]}
                >
                  <InputNumber min={0} max={8760} addonAfter="小时" style={{ width: '100%' }} />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item
                  name="taskSave"
                  label={labelWithTip('历史任务保留', '任务历史保留天数，0 表示全部保留；保存后会立即清理过期历史。')}
                  rules={[{ required: true, message: '请输入历史任务保留天数' }]}
                >
                  <InputNumber min={0} max={3650} addonAfter="天" style={{ width: '100%' }} />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item
                  name="copyConcurrency"
                  label={labelWithTip('复制并发数', '同一任务内同时执行的文件复制数量；数值越高越吃 AList 和存储端资源，新任务生效。')}
                  rules={[{ required: true, message: '请输入复制并发数' }]}
                >
                  <InputNumber min={1} max={100} style={{ width: '100%' }} />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item
                  name="scanConcurrency"
                  label={labelWithTip('扫描并发数', '同一任务内并发扫描目录的数量，最大值为 20；新任务生效。')}
                  rules={[{ required: true, message: '请输入扫描并发数' }]}
                >
                  <InputNumber min={1} max={20} style={{ width: '100%' }} />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item
                  name="realtimeFinishedItems"
                  label={labelWithTip('完成明细保留数', '任务详情实时视图中保留的已完成明细数量，超过后会批量写入数据库；新任务生效。')}
                  rules={[{ required: true, message: '请输入完成明细保留数' }]}
                >
                  <InputNumber min={100} max={50000} style={{ width: '100%' }} />
                </Form.Item>
              </Col>
              <Col xs={24} md={12}>
                <Form.Item
                  name="maxRetries"
                  label={labelWithTip('最大重试次数', '单个复制项失败后的最大自动重试次数，0 表示不自动重试；新任务生效。')}
                  rules={[{ required: true, message: '请输入最大重试次数' }]}
                >
                  <InputNumber min={0} max={10} style={{ width: '100%' }} />
                </Form.Item>
              </Col>
            </Row>

            <Form.Item style={{ marginBottom: 0 }}>
              <Space>
                <Button type="primary" htmlType="submit" icon={<SaveOutlined />} loading={saving}>
                  保存配置
                </Button>
                <Button onClick={fetchConfig} disabled={loading || saving}>
                  重新加载
                </Button>
              </Space>
            </Form.Item>
          </Form>
        </Card>
      </div>

      <Modal
        title="修改密码"
        open={passwordVisible}
        onOk={handleChangePassword}
        onCancel={() => {
          setPasswordVisible(false);
          passwordForm.resetFields();
        }}
        confirmLoading={passwordSaving}
        destroyOnClose
      >
        <Form form={passwordForm} layout="vertical">
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
        </Form>
      </Modal>
    </div>
  );
}
