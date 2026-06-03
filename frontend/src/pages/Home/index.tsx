import type React from 'react';
import { useState, useEffect, useCallback } from 'react';
import {
  Card, Row, Col, Button, Tag, Space, Popconfirm, Pagination, App, Drawer, Select, Input, Form, Switch, InputNumber,
  Empty, Typography, Divider, TreeSelect, Spin, Descriptions, Tooltip,
} from 'antd';
import {
  PlusOutlined, PlayCircleOutlined, DeleteOutlined,
  CaretRightOutlined, FolderOpenOutlined, FolderOutlined,
  SwapOutlined, ClockCircleOutlined, CloudServerOutlined, EditOutlined,
} from '@ant-design/icons';
import { jobGetJob, jobPost, jobPut, jobDelete } from '../../api/job';
import { alistGet, alistGetPath } from '../../api/alist';
import TaskList from './TaskList';
import TaskDetail from './TaskDetail';
import dayjs from 'dayjs';

const { Text } = Typography;

const statusColors: Record<number, string> = {
  0: 'default', 1: 'processing', 2: 'success', 3: 'warning',
  4: 'default', 5: 'warning', 6: 'error', 7: 'error',
};
const statusLabels: Record<number, string> = {
  0: '禁用', 1: '启用',
};
const methodNames = ['仅新增', '全同步', '移动模式'];
const cronTypeNames = ['间隔(分钟)', 'Cron', '仅手动'];
const cronFields = [
  { name: 'second', label: '秒', placeholder: '0' },
  { name: 'minute', label: '分', placeholder: '*' },
  { name: 'hour', label: '时', placeholder: '*' },
  { name: 'day', label: '日', placeholder: '*' },
  { name: 'month', label: '月', placeholder: '*' },
  { name: 'day_of_week', label: '周', placeholder: '*' },
];

export default function Home() {
  const { message } = App.useApp();
  const [list, setList] = useState<any[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize] = useState(12);
  const [loading, setLoading] = useState(false);
  const [alistList, setAlistList] = useState<any[]>([]);
  const [drawerVisible, setDrawerVisible] = useState(false);
  const [editingJob, setEditingJob] = useState<any>(null);
  const [taskDrawerJobId, setTaskDrawerJobId] = useState<string>('');
  const [taskDetailDrawerTaskId, setTaskDetailDrawerTaskId] = useState<string>('');
  const [form] = Form.useForm();

  // Directory tree state (fully controlled)
  const [srcTreeData, setSrcTreeData] = useState<any[]>([]);
  const [dstTreeData, setDstTreeData] = useState<any[]>([]);
  const [srcLoadedKeys, setSrcLoadedKeys] = useState<React.Key[]>([]);
  const [dstLoadedKeys, setDstLoadedKeys] = useState<React.Key[]>([]);
  const [treeLoading, setTreeLoading] = useState(false);

  const fetchList = async (p = page, ps = pageSize) => {
    setLoading(true);
    try {
      const res: any = await jobGetJob({ pageSize: ps, pageNum: p });
      setList(res.data?.dataList || []);
      setTotal(res.data?.count || 0);
    } catch { /* ignore */ }
    setLoading(false);
  };

  const fetchAlistList = async () => {
    try {
      const res: any = await alistGet();
      setAlistList(res.data || []);
    } catch { /* ignore */ }
  };

  useEffect(() => { fetchList(); fetchAlistList(); }, []);
  useEffect(() => { fetchList(); }, [page, pageSize]);

  // Load root directory when alistId changes
  const selectedAlistId = Form.useWatch('alistId', form);

  const fetchDirChildren = useCallback(async (alistId: number, parentPath: string): Promise<any[]> => {
    if (!alistId) return [];
    try {
      const res: any = await alistGetPath(alistId, parentPath);
      const items = res.data || [];
      return (Array.isArray(items) ? items : []).map((item: any) => {
        const name = item.path || item.name || '';
        const fullPath = parentPath === '/' ? `/${name}` : `${parentPath}/${name}`;
        return {
          title: name,
          value: fullPath,
          key: fullPath,
          isLeaf: false,
        };
      });
    } catch {
      return [];
    }
  }, []);

  // Helper to insert children into tree data at the correct parent node
  const updateTreeChildren = (tree: any[], parentValue: string, children: any[]): any[] => {
    return tree.map((node) => {
      if (node.value === parentValue) {
        return { ...node, children };
      }
      if (node.children) {
        return { ...node, children: updateTreeChildren(node.children, parentValue, children) };
      }
      return node;
    });
  };

  // Load root when engine changes
  useEffect(() => {
    if (selectedAlistId) {
      setTreeLoading(true);
      setSrcLoadedKeys([]);
      setDstLoadedKeys([]);
      fetchDirChildren(selectedAlistId, '/').then((nodes) => {
        const root = [{ title: '/', value: '/', key: '/', children: nodes }];
        setSrcTreeData(root);
        setDstTreeData(JSON.parse(JSON.stringify(root)));
      }).finally(() => {
        setTreeLoading(false);
      });
    } else {
      setSrcTreeData([]);
      setDstTreeData([]);
    }
  }, [selectedAlistId, fetchDirChildren]);

  const onLoadSrcData = async (node: any) => {
    if (!selectedAlistId || srcLoadedKeys.includes(node.value)) return;
    const children = await fetchDirChildren(selectedAlistId, node.value);
    setSrcTreeData((prev) => updateTreeChildren(prev, node.value, children));
    setSrcLoadedKeys((prev) => [...prev, node.value]);
  };

  const onLoadDstData = async (node: any) => {
    if (!selectedAlistId || dstLoadedKeys.includes(node.value)) return;
    const children = await fetchDirChildren(selectedAlistId, node.value);
    setDstTreeData((prev) => updateTreeChildren(prev, node.value, children));
    setDstLoadedKeys((prev) => [...prev, node.value]);
  };

  const handleAdd = () => {
    setEditingJob(null);
    form.resetFields();
    form.setFieldsValue({
      enable: true,
      method: 0,
      isCron: 0,
      interval: 1440,
      useCacheS: false,
      useCacheT: false,
      scanIntervalS: 0,
      scanIntervalT: 0,
      second: '0',
      minute: '*',
      hour: '*',
      day: '*',
      month: '*',
      day_of_week: '*',
    });
    setSrcTreeData([]);
    setDstTreeData([]);
    setSrcLoadedKeys([]);
    setDstLoadedKeys([]);
    setDrawerVisible(true);
  };

  const handleEdit = (job: any) => {
    setEditingJob(job);
    form.resetFields();
    form.setFieldsValue({
      ...job,
      enable: job.enable === 1,
      useCacheS: job.useCacheS === 1 || job.useCacheS === true,
      useCacheT: job.useCacheT === 1 || job.useCacheT === true,
      dstPath: String(job.dstPath || '').split(':').filter(Boolean),
      second: job.second || '0',
      minute: job.minute || '*',
      hour: job.hour || '*',
      day: job.day || '*',
      month: job.month || '*',
      day_of_week: job.day_of_week || '*',
    });
    setSrcLoadedKeys([]);
    setDstLoadedKeys([]);
    setDrawerVisible(true);
  };

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();
      const dstPaths = Array.isArray(values.dstPath) ? values.dstPath : [values.dstPath];
      const jobData = {
        ...(editingJob ? { id: editingJob.id } : {}),
        ...values,
        dstPath: dstPaths.filter(Boolean).join(':'),
        enable: values.enable ? 1 : 0,
        useCacheS: values.useCacheS ? 1 : 0,
        useCacheT: values.useCacheT ? 1 : 0,
      };
      await jobPost(jobData);
      message.success(editingJob ? '编辑成功' : '创建成功');
      setDrawerVisible(false);
      fetchList();
    } catch { /* ignore */ }
  };

  const handleDelete = async (id: number) => {
    try {
      await jobDelete({ id });
      message.success('删除成功');
      fetchList();
    } catch { /* ignore */ }
  };

  const handleToggle = async (job: any) => {
    try {
      await jobPut({ id: String(job.id), pause: job.enable === 1 });
      message.success('操作成功');
      fetchList();
    } catch { /* ignore */ }
  };

  const handleRun = async (id: number) => {
    try {
      await jobPut({ id: String(id) });
      message.success('已提交执行');
      fetchList();
    } catch { /* ignore */ }
  };

  const handleRunAll = async () => {
    try {
      await jobPut({});
      message.success('已提交执行所有作业');
    } catch { /* ignore */ }
  };

  const openTaskDrawer = (jobId: number | string) => {
    setTaskDetailDrawerTaskId('');
    setTaskDrawerJobId(String(jobId));
  };

  const closeTaskDrawer = () => {
    setTaskDetailDrawerTaskId('');
    setTaskDrawerJobId('');
  };

  const getAlistName = (alistId: number) => {
    const a = alistList.find((x: any) => x.id === alistId);
    if (!a) return `引擎 #${alistId}`;
    return a.remark ? `${a.userName} (${a.remark})` : a.userName;
  };

  const formatSchedule = (job: any) => {
    if (job.isCron === 0) return `每 ${job.interval} 分钟`;
    if (job.isCron === 1) {
      return `Cron: ${[job.second, job.minute, job.hour, job.day, job.month, job.day_of_week].map((v) => v || '*').join(' ')}`;
    }
    return '仅手动触发';
  };

  const formatCache = (useS: number, useT: number) => {
    const s = useS ? '源端✓' : '源端✗';
    const t = useT ? '目标✓' : '目标✗';
    return `${s} / ${t}`;
  };

  const isCronValue = Form.useWatch('isCron', form);

  return (
    <div>
      <Card>
        <div className="page-header">
          <h2>作业管理</h2>
          <Space>
            <Button icon={<PlayCircleOutlined />} onClick={handleRunAll}>执行全部</Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={handleAdd}>新建作业</Button>
          </Space>
        </div>

        {list.length === 0 && !loading ? (
          <Empty
            image={<FolderOpenOutlined style={{ fontSize: 64, color: 'var(--ant-color-text-quaternary)' }} />}
            styles={{ image: { height: 80 } }}
            description={<Text type="secondary">暂无作业，点击上方「新建作业」创建第一个同步任务</Text>}
            className="empty-state"
          />
        ) : (
          <>
            <Row gutter={[16, 16]}>
              {list.map((job: any) => (
                <Col xs={24} md={12} key={job.id}>
                  <Card
                    hoverable
                    style={{ cursor: 'pointer' }}
                    styles={{ body: { padding: '20px 24px' } }}
                    onClick={() => openTaskDrawer(job.id)}
                    actions={[
                      <Tooltip title="手动执行" key="run">
                        <CaretRightOutlined
                          onClick={(e) => { e.stopPropagation(); handleRun(job.id); }}
                          style={{ color: job.enable === 1 ? 'var(--ant-color-primary)' : undefined }}
                        />
                      </Tooltip>,
                      <Tooltip title={job.enable === 1 ? '启用中，不可编辑' : '编辑'} key="edit">
                        <EditOutlined
                          onClick={(e) => { e.stopPropagation(); if (job.enable !== 1) handleEdit(job); }}
                          style={job.enable === 1 ? { color: 'var(--ant-color-text-disabled)', cursor: 'not-allowed' } : undefined}
                        />
                      </Tooltip>,
                      <Popconfirm title="确认删除此作业？" onConfirm={() => handleDelete(job.id)} key="del">
                        <Tooltip title="删除">
                          <DeleteOutlined
                            onClick={(e) => e.stopPropagation()}
                            style={{ color: 'var(--ant-color-error)' }}
                          />
                        </Tooltip>
                      </Popconfirm>,
                    ]}
                  >
                    {/* Header */}
                    <div className="card-item-header">
                      <div className="card-item-icon" style={{
                        background: job.enable === 1
                          ? 'linear-gradient(135deg, #1677ff 0%, #0958d9 100%)'
                          : 'linear-gradient(135deg, #8c8c8c 0%, #595959 100%)',
                      }}>
                        <SwapOutlined style={{ fontSize: 20, color: '#fff' }} />
                      </div>
                      <div className="card-item-info" style={{ flex: 1 }}>
                        <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                          <Text strong style={{ fontSize: 15 }}>
                            {job.remark || `作业 #${job.id}`}
                          </Text>
                          <Tag color={statusColors[job.enable] || 'default'} style={{ margin: 0, fontSize: 11 }}>
                            {statusLabels[job.enable] || '未知'}
                          </Tag>
                          <Tag style={{ margin: 0, fontSize: 11 }}>{methodNames[job.method] || job.method}</Tag>
                        </div>
                        <div style={{ fontSize: 12, color: 'var(--ant-color-text-secondary)', marginTop: 3, display: 'flex', alignItems: 'center', gap: 4 }}>
                          <CloudServerOutlined />
                          <span>{getAlistName(job.alistId)}</span>
                        </div>
                      </div>
                      <Switch
                        checked={job.enable === 1}
                        onChange={(_, e) => { e.stopPropagation(); handleToggle(job); }}
                        onClick={(_, e) => e.stopPropagation()}
                        size="small"
                      />
                    </div>

                    {/* Body */}
                    <Descriptions column={1} size="small" styles={{ label: { color: 'var(--ant-color-text-secondary)', width: 70, fontSize: 12 }, content: { fontSize: 13 } }}>
                      <Descriptions.Item label={<span><FolderOutlined style={{ marginRight: 4 }} />源目录</span>}>
                        <Text ellipsis={{ tooltip: job.srcPath }} style={{ fontSize: 13 }}>{job.srcPath}</Text>
                      </Descriptions.Item>
                      <Descriptions.Item label={<span><FolderOpenOutlined style={{ marginRight: 4 }} />目标</span>}>
                        <Text ellipsis={{ tooltip: job.dstPath }} style={{ fontSize: 13 }}>
                          {String(job.dstPath).replace(/:/g, ' → ')}
                        </Text>
                      </Descriptions.Item>
                      <Descriptions.Item label={<span><ClockCircleOutlined style={{ marginRight: 4 }} />调度</span>}>
                        <Text type="secondary" className="desc-text-sm">{formatSchedule(job)}</Text>
                      </Descriptions.Item>
                      <Descriptions.Item label="缓存">
                        <Text type="secondary" className="desc-text-sm">{formatCache(job.useCacheS, job.useCacheT)}</Text>
                      </Descriptions.Item>
                      {job.exclude && (
                        <Descriptions.Item label="排除">
                          <Text type="secondary" ellipsis={{ tooltip: job.exclude }} style={{ fontSize: 12, maxWidth: 200 }}>{job.exclude}</Text>
                        </Descriptions.Item>
                      )}
                      <Descriptions.Item label="创建">
                        <Text type="secondary" className="desc-text-sm">
                          {job.createTime ? dayjs.unix(job.createTime).format('YYYY-MM-DD HH:mm') : '—'}
                        </Text>
                      </Descriptions.Item>
                    </Descriptions>
                  </Card>
                </Col>
              ))}
            </Row>
            {total > pageSize && (
              <div style={{ textAlign: 'center', marginTop: 20 }}>
                <Pagination current={page} pageSize={pageSize} total={total} onChange={setPage} showSizeChanger={false} />
              </div>
            )}
          </>
        )}
      </Card>

      <Drawer
        title={editingJob ? '编辑作业' : '新建作业'}
        open={drawerVisible}
        onClose={() => setDrawerVisible(false)}
        styles={{ wrapper: { width: 580 } }}
        destroyOnClose
        extra={
          <Space>
            <Button onClick={() => setDrawerVisible(false)}>取消</Button>
            <Button type="primary" onClick={handleSubmit}>保存</Button>
          </Space>
        }
      >
        <Form form={form} layout="vertical">
          <Form.Item name="alistId" label="引擎" rules={[{ required: true, message: '请选择引擎' }]}>
            <Select
              placeholder="选择引擎"
              options={alistList.map((a: any) => ({
                value: a.id,
                label: `${a.userName} - ${a.url}${a.remark ? ` (${a.remark})` : ''}`,
              }))}
            />
          </Form.Item>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="srcPath" label="源目录" rules={[{ required: true, message: '请选择源目录' }]}>
                <TreeSelect
                  placeholder="选择源目录"
                  treeData={srcTreeData}
                  loadData={onLoadSrcData as any}
                  treeDefaultExpandAll
                  showSearch
                  treeNodeFilterProp="title"
                  styles={{ popup: { root: { maxHeight: 300, overflow: 'auto' } } }}
                  suffixIcon={treeLoading ? <Spin size="small" /> : undefined}
                  notFoundContent={selectedAlistId ? '请先展开目录' : '请先选择引擎'}
                />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="dstPath" label="目标目录" rules={[{ required: true, message: '请选择目标目录' }]}>
                <TreeSelect
                  placeholder="选择目标目录"
                  treeData={dstTreeData}
                  loadData={onLoadDstData as any}
                  treeDefaultExpandAll
                  multiple
                  showSearch
                  treeNodeFilterProp="title"
                  styles={{ popup: { root: { maxHeight: 300, overflow: 'auto' } } }}
                  suffixIcon={treeLoading ? <Spin size="small" /> : undefined}
                  notFoundContent={selectedAlistId ? '请先展开目录' : '请先选择引擎'}
                />
              </Form.Item>
            </Col>
          </Row>
          <Form.Item name="remark" label="备注">
            <Input placeholder="可选备注" />
          </Form.Item>

          <Divider style={{ fontSize: 13 }}>同步配置</Divider>
          <Row gutter={16}>
            <Col span={8}>
              <Form.Item name="method" label="同步方式">
                <Select options={methodNames.map((n, i) => ({ value: i, label: n }))} />
              </Form.Item>
            </Col>
            <Col span={8}>
              <Form.Item name="isCron" label="调度方式">
                <Select
                  options={cronTypeNames.map((n, i) => ({ value: i, label: n }))}
                  onChange={(value) => {
                    if (value === 0) form.setFieldsValue({ interval: 1440 });
                    if (value === 1) {
                      form.setFieldsValue({
                        second: form.getFieldValue('second') || '0',
                        minute: form.getFieldValue('minute') || '*',
                        hour: form.getFieldValue('hour') || '*',
                        day: form.getFieldValue('day') || '*',
                        month: form.getFieldValue('month') || '*',
                        day_of_week: form.getFieldValue('day_of_week') || '*',
                      });
                    }
                  }}
                />
              </Form.Item>
            </Col>
            <Col span={8}>
              {isCronValue === 0 && (
                <Form.Item name="interval" label="间隔(分钟)" rules={[{ required: true, message: '请输入' }]}>
                  <InputNumber min={1} style={{ width: '100%' }} />
                </Form.Item>
              )}
            </Col>
          </Row>
          {isCronValue === 1 && (
            <Row gutter={12}>
              {cronFields.map((field) => (
                <Col span={8} key={field.name}>
                  <Form.Item name={field.name} label={field.label} rules={[{ required: true, message: '请输入' }]}>
                    <Input placeholder={field.placeholder} />
                  </Form.Item>
                </Col>
              ))}
            </Row>
          )}

          <Divider style={{ fontSize: 13 }}>缓存与扫描</Divider>
          <Row gutter={16}>
            <Col span={12}>
              <Card size="small" styles={{ body: { padding: '12px 16px' } }}>
                <Space orientation="vertical" style={{ width: '100%' }} size={12}>
                  <Text strong style={{ fontSize: 13 }}>源端</Text>
                  <Form.Item name="useCacheS" label="缓存" valuePropName="checked" style={{ marginBottom: 0 }}>
                    <Switch checkedChildren="使用" unCheckedChildren="不使用" />
                  </Form.Item>
                  <Form.Item name="scanIntervalS" label="扫描间隔(秒)" style={{ marginBottom: 0 }}>
                    <InputNumber min={0} style={{ width: '100%' }} placeholder="0 表示默认" />
                  </Form.Item>
                </Space>
              </Card>
            </Col>
            <Col span={12}>
              <Card size="small" styles={{ body: { padding: '12px 16px' } }}>
                <Space orientation="vertical" style={{ width: '100%' }} size={12}>
                  <Text strong style={{ fontSize: 13 }}>目标端</Text>
                  <Form.Item name="useCacheT" label="缓存" valuePropName="checked" style={{ marginBottom: 0 }}>
                    <Switch checkedChildren="使用" unCheckedChildren="不使用" />
                  </Form.Item>
                  <Form.Item name="scanIntervalT" label="扫描间隔(秒)" style={{ marginBottom: 0 }}>
                    <InputNumber min={0} style={{ width: '100%' }} placeholder="0 表示默认" />
                  </Form.Item>
                </Space>
              </Card>
            </Col>
          </Row>

          <div style={{ marginTop: 16 }}>
            <Form.Item name="exclude" label="排除项" tooltip="gitignore 语法，多个用冒号分隔">
              <Input.TextArea placeholder="如 *.tmp : .git/" rows={2} />
            </Form.Item>
          </div>

          <Divider />
          <Form.Item name="enable" label="启用" valuePropName="checked">
            <Switch />
          </Form.Item>
        </Form>
      </Drawer>

      <Drawer
        title={`任务列表 — 作业 #${taskDrawerJobId}`}
        open={!!taskDrawerJobId}
        onClose={closeTaskDrawer}
        styles={{ wrapper: { width: 'min(900px, 90vw)' } }}
        destroyOnClose
      >
        <TaskList
          jobId={taskDrawerJobId}
          onTaskDetail={(taskId) => setTaskDetailDrawerTaskId(String(taskId))}
        />
      </Drawer>

      <Drawer
        title={`任务详情 — 任务 #${taskDetailDrawerTaskId}`}
        open={!!taskDetailDrawerTaskId}
        onClose={() => setTaskDetailDrawerTaskId('')}
        styles={{ wrapper: { width: 'min(760px, 86vw)' } }}
        destroyOnClose
      >
        <TaskDetail key={taskDetailDrawerTaskId} taskId={taskDetailDrawerTaskId} embedded />
      </Drawer>
    </div>
  );
}
