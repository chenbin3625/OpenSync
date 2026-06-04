import { useState, useEffect, useCallback } from 'react';
import type { Key } from 'react';
import {
  Card, Row, Col, Button, Tag, Space, Popconfirm, Pagination, App, Drawer, Select, Input, Form, Switch, InputNumber,
  Empty, Typography, Divider, TreeSelect, Spin, Descriptions, Tooltip,
} from 'antd';
import {
  PlusOutlined, PlayCircleOutlined, DeleteOutlined,
  CaretRightOutlined, EditOutlined, QuestionCircleOutlined,
} from '@ant-design/icons';
import { jobGetJob, jobPost, jobPut, jobDelete } from '../../api/job';
import { alistGet, alistGetPath } from '../../api/alist';
import TaskList from './TaskList';
import TaskDetail from './TaskDetail';
import dayjs from 'dayjs';
import type { AlistItem, JobFormValues, JobItem, PathItem, TreeNode } from '../../types';

const { Text } = Typography;

const statusColors: Record<number, string> = {
  0: 'default', 1: 'processing', 2: 'success', 3: 'warning',
  4: 'default', 5: 'warning', 6: 'error', 7: 'error',
};
const statusLabels: Record<number, string> = {
  0: '禁用', 1: '启用',
};
const methodOptions = [
  {
    name: '仅新增',
    description: '复制源目录中目标端不存在或内容变化的文件，不删除目标端多余文件，适合增量备份。',
  },
  {
    name: '全同步',
    description: '目标目录尽量与源目录保持一致，会复制新增/变更文件，并删除目标端源目录已不存在的文件。',
  },
  {
    name: '移动模式',
    description: '按移动任务处理新增/变更文件，适合把文件从源端迁移到目标端，用于归档或腾挪空间。',
  },
];
const methodNames = methodOptions.map((method) => method.name);
const cronTypeNames = ['间隔(分钟)', 'Cron', '仅手动'];
const cronFields = [
  { name: 'second', label: '秒', placeholder: '0' },
  { name: 'minute', label: '分', placeholder: '*' },
  { name: 'hour', label: '时', placeholder: '*' },
  { name: 'day', label: '日', placeholder: '*' },
  { name: 'month', label: '月', placeholder: '*' },
  { name: 'day_of_week', label: '周', placeholder: '*' },
];
const defaultCronFields = {
  second: '0',
  minute: '0',
  hour: '2',
  day: '*',
  month: '*',
  day_of_week: '*',
};

type ScheduleValues = {
  isCron?: number;
  interval?: number;
  second?: string | null;
  minute?: string | null;
  hour?: string | null;
  day?: string | null;
  month?: string | null;
  day_of_week?: string | null;
};

const cronValue = (value?: string | null, fallback = '*') => {
  const normalized = String(value ?? '').trim();
  return normalized || fallback;
};

const formatTime = (hour?: string | null, minute?: string | null, second?: string | null) => {
  const h = cronValue(hour);
  const m = cronValue(minute);
  const s = cronValue(second, '0');
  if (/^\d+$/.test(h) && /^\d+$/.test(m) && /^\d+$/.test(s)) {
    return `${h.padStart(2, '0')}:${m.padStart(2, '0')}:${s.padStart(2, '0')}`;
  }
  return `${h}:${m}:${s}`;
};

const describeCronPlan = (values: ScheduleValues) => {
  const second = cronValue(values.second, '0');
  const minute = cronValue(values.minute);
  const hour = cronValue(values.hour);
  const day = cronValue(values.day);
  const month = cronValue(values.month);
  const dayOfWeek = cronValue(values.day_of_week);
  const time = formatTime(hour, minute, second);

  if (day === '*' && month === '*' && dayOfWeek === '*' && hour !== '*' && minute !== '*') {
    return `每天 ${time} 执行`;
  }
  if (day !== '*' && month === '*' && dayOfWeek === '*' && hour !== '*' && minute !== '*') {
    return `每月 ${day} 日 ${time} 执行`;
  }
  if (day === '*' && month === '*' && dayOfWeek !== '*' && hour !== '*' && minute !== '*') {
    return `每周 ${dayOfWeek} 的 ${time} 执行`;
  }
  return `按 Cron 表达式 ${[second, minute, hour, day, month, dayOfWeek].join(' ')} 执行`;
};

const formatSchedulePlan = (values: ScheduleValues) => {
  if (values.isCron === 0) return `每 ${values.interval || 0} 分钟执行一次`;
  if (values.isCron === 1) return describeCronPlan(values);
  return '不自动执行，只能手动触发';
};
const compactItemStyle = { marginBottom: 12 };
const compactDividerStyle = { margin: '8px 0 12px' };

const parseJobPathList = (value: unknown): string[] => {
  if (Array.isArray(value)) {
    return value.map((item) => String(item).trim()).filter(Boolean);
  }
  const raw = String(value ?? '').trim();
  if (!raw) return [];
  try {
    const parsed = JSON.parse(raw);
    if (Array.isArray(parsed)) {
      return parsed.map((item) => String(item).trim()).filter(Boolean);
    }
  } catch { /* plain single path */ }
  return [raw];
};

const normalizeFormPaths = (value: string | string[] | undefined): string[] => {
  const paths = Array.isArray(value) ? value : [value];
  return paths.map((item) => String(item ?? '').trim()).filter(Boolean);
};

const parseJobSrcPaths = parseJobPathList;
const parseJobDstPaths = parseJobPathList;

const formatJobPaths = (value: unknown, separator = '、') => {
  const paths = parseJobPathList(value);
  return paths.length > 0 ? paths.join(separator) : '';
};

const defaultExclude = `# macOS
.DS_Store
._*
.AppleDouble/
.LSOverride
.Spotlight-V100/
.Trashes/
.TemporaryItems/
.fseventsd/
.DocumentRevisions-V100/

# Windows
Thumbs.db
ehthumbs.db
ehthumbs_vista.db
Desktop.ini
$RECYCLE.BIN/
RECYCLER/
System Volume Information/

# Linux / NAS
lost+found/
@eaDir/
#recycle/
@Recycle/
.Recycle/
.Trash-*/
.Trash/

# 临时文件 / 下载未完成文件
*.tmp
*.temp
*.log
*.bak
*.old
*.orig
*.part
*.crdownload
*.download
*.swp
*.swo
*.swn
*~
~*
~$*
*.lock
.~lock.*#

# 缓存目录
.cache/
cache/
tmp/
temp/
logs/
log/

# 开发相关缓存 / 依赖
node_modules/
.npm/
.yarn/
.pnpm-store/
__pycache__/
*.pyc
*.pyo
.pytest_cache/
.mypy_cache/
.ruff_cache/
.tox/
.venv/
venv/
env/
.idea/
.vscode/
*.iml

# 版本控制目录
.git/
.svn/
.hg/

# 构建产物
.sass-cache/
.gradle/
build/
dist/
target/
coverage/
.next/
.nuxt/
.turbo/`;

export default function Home() {
  const { message } = App.useApp();
  const [list, setList] = useState<JobItem[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize] = useState(12);
  const [loading, setLoading] = useState(false);
  const [alistList, setAlistList] = useState<AlistItem[]>([]);
  const [drawerVisible, setDrawerVisible] = useState(false);
  const [editingJob, setEditingJob] = useState<JobItem | null>(null);
  const [taskDrawerJobId, setTaskDrawerJobId] = useState<string>('');
  const [taskDetailDrawerTaskId, setTaskDetailDrawerTaskId] = useState<string>('');
  const [form] = Form.useForm();

  // Directory tree state (fully controlled)
  const [srcTreeData, setSrcTreeData] = useState<TreeNode[]>([]);
  const [dstTreeData, setDstTreeData] = useState<TreeNode[]>([]);
  const [srcLoadedKeys, setSrcLoadedKeys] = useState<Key[]>([]);
  const [dstLoadedKeys, setDstLoadedKeys] = useState<Key[]>([]);
  const [treeLoading, setTreeLoading] = useState(false);

  const fetchList = useCallback(async (p = page, ps = pageSize) => {
    setLoading(true);
    try {
      const res = await jobGetJob({ pageSize: ps, pageNum: p });
      setList(res.data?.dataList || []);
      setTotal(res.data?.count || 0);
    } catch { /* ignore */ }
    setLoading(false);
  }, [page, pageSize]);

  const fetchAlistList = useCallback(async () => {
    try {
      const res = await alistGet();
      setAlistList(res.data || []);
    } catch { /* ignore */ }
  }, []);

  useEffect(() => { fetchAlistList(); }, [fetchAlistList]);
  useEffect(() => { fetchList(); }, [fetchList]);

  // Load root directory when alistId changes
  const selectedAlistId = Form.useWatch('alistId', form) as number | undefined;

  const fetchDirChildren = useCallback(async (alistId: number, parentPath: string): Promise<TreeNode[]> => {
    if (!alistId) return [];
    try {
      const res = await alistGetPath(alistId, parentPath);
      const items = res.data || [];
      return (Array.isArray(items) ? items : []).map((item: PathItem) => {
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
  const updateTreeChildren = (tree: TreeNode[], parentValue: string, children: TreeNode[]): TreeNode[] => {
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

  const onLoadSrcData = async (node: TreeNode) => {
    if (!selectedAlistId || srcLoadedKeys.includes(node.value)) return;
    const children = await fetchDirChildren(selectedAlistId, node.value);
    setSrcTreeData((prev) => updateTreeChildren(prev, node.value, children));
    setSrcLoadedKeys((prev) => [...prev, node.value]);
  };

  const onLoadDstData = async (node: TreeNode) => {
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
      isCron: 1,
      interval: 1440,
      useCacheS: false,
      useCacheT: false,
      scanIntervalS: 0,
      scanIntervalT: 0,
      ...defaultCronFields,
      exclude: defaultExclude,
    });
    setSrcTreeData([]);
    setDstTreeData([]);
    setSrcLoadedKeys([]);
    setDstLoadedKeys([]);
    setDrawerVisible(true);
  };

  const handleEdit = (job: JobItem) => {
    setEditingJob(job);
    form.resetFields();
    form.setFieldsValue({
      ...job,
      enable: job.enable === 1,
      useCacheS: job.useCacheS === 1 || job.useCacheS === true,
      useCacheT: job.useCacheT === 1 || job.useCacheT === true,
      srcPath: parseJobSrcPaths(job.srcPath),
      dstPath: parseJobDstPaths(job.dstPath),
      second: job.second || defaultCronFields.second,
      minute: job.minute || defaultCronFields.minute,
      hour: job.hour || defaultCronFields.hour,
      day: job.day || defaultCronFields.day,
      month: job.month || defaultCronFields.month,
      day_of_week: job.day_of_week || defaultCronFields.day_of_week,
    });
    setSrcLoadedKeys([]);
    setDstLoadedKeys([]);
    setDrawerVisible(true);
  };

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields() as JobFormValues;
      const srcPaths = normalizeFormPaths(values.srcPath);
      const dstPaths = normalizeFormPaths(values.dstPath);
      const jobData: Record<string, unknown> = {
        ...(editingJob ? { id: editingJob.id } : {}),
        ...values,
        srcPath: srcPaths,
        dstPath: dstPaths,
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

  const handleToggle = async (job: JobItem) => {
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
      message.success('已提交执行所有同步任务');
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
    const a = alistList.find((x) => x.id === alistId);
    if (!a) return `引擎 #${alistId}`;
    return a.remark ? `${a.userName} (${a.remark})` : a.userName;
  };

  const formatSchedule = (job: JobItem) => {
    if (job.isCron === 0) return `每 ${job.interval} 分钟`;
    if (job.isCron === 1) {
      return describeCronPlan(job);
    }
    return '仅手动触发';
  };

  const formatCache = (useS: number | boolean, useT: number | boolean) => {
    const s = useS ? '源端✓' : '源端✗';
    const t = useT ? '目标✓' : '目标✗';
    return `${s} / ${t}`;
  };

  const isCronValue = Form.useWatch('isCron', form) as number | undefined;
  const intervalValue = Form.useWatch('interval', form) as number | undefined;
  const secondValue = Form.useWatch('second', form) as string | undefined;
  const minuteValue = Form.useWatch('minute', form) as string | undefined;
  const hourValue = Form.useWatch('hour', form) as string | undefined;
  const dayValue = Form.useWatch('day', form) as string | undefined;
  const monthValue = Form.useWatch('month', form) as string | undefined;
  const dayOfWeekValue = Form.useWatch('day_of_week', form) as string | undefined;
  const scheduleValues: ScheduleValues = {
    isCron: isCronValue ?? 1,
    interval: intervalValue,
    second: secondValue,
    minute: minuteValue,
    hour: hourValue,
    day: dayValue,
    month: monthValue,
    day_of_week: dayOfWeekValue,
  };
  const schedulePlan = formatSchedulePlan(scheduleValues);

  return (
    <div>
      <Card className="page-card">
        <div className="page-header">
          <h2>任务管理</h2>
          <Space>
            <Button icon={<PlayCircleOutlined />} onClick={handleRunAll}>执行全部</Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={handleAdd}>新建同步任务</Button>
          </Space>
        </div>

        {list.length === 0 && !loading ? (
          <Empty
            description={<Text type="secondary">暂无同步任务，点击上方「新建同步任务」创建第一个同步任务</Text>}
          />
        ) : (
          <>
            <Row gutter={[16, 16]}>
              {list.map((job) => (
                <Col xs={24} md={12} key={job.id}>
                  <Card
                    hoverable
                    className="sync-job-card"
                    title={job.remark || `同步任务 #${job.id}`}
                    extra={
                      <Space>
                        <Tag color={statusColors[job.enable] || 'default'}>{statusLabels[job.enable] || '未知'}</Tag>
                        <Tag>{methodNames[job.method] || job.method}</Tag>
                        {job.isCron !== 2 && (
                          <Switch
                            checked={job.enable === 1}
                            onChange={(_, e) => { e.stopPropagation(); handleToggle(job); }}
                            onClick={(_, e) => e.stopPropagation()}
                            size="small"
                          />
                        )}
                      </Space>
                    }
                    onClick={() => openTaskDrawer(job.id)}
                    actions={[
                      <Tooltip title="手动执行" key="run">
                        <CaretRightOutlined
                          onClick={(e) => { e.stopPropagation(); handleRun(job.id); }}
                        />
                      </Tooltip>,
                      <Tooltip title={job.enable === 1 ? '启用中，不可编辑' : '编辑'} key="edit">
                        <EditOutlined
                          onClick={(e) => { e.stopPropagation(); if (job.enable !== 1) handleEdit(job); }}
                        />
                      </Tooltip>,
                      <Popconfirm title="确认删除此同步任务？" onConfirm={() => handleDelete(job.id)} key="del">
                        <Tooltip title="删除">
                          <DeleteOutlined
                            onClick={(e) => e.stopPropagation()}
                          />
                        </Tooltip>
                      </Popconfirm>,
                    ]}
                  >
                    <Descriptions className="sync-job-descriptions" column={1} size="small">
                      <Descriptions.Item label="引擎">
                        <Text type="secondary">{getAlistName(job.alistId)}</Text>
                      </Descriptions.Item>
                      <Descriptions.Item label="源目录">
                        <Text ellipsis={{ tooltip: formatJobPaths(job.srcPath) }}>{formatJobPaths(job.srcPath)}</Text>
                      </Descriptions.Item>
                      <Descriptions.Item label="目标">
                        <Text ellipsis={{ tooltip: formatJobPaths(job.dstPath, ' → ') }}>
                          {formatJobPaths(job.dstPath, ' → ')}
                        </Text>
                      </Descriptions.Item>
                      <Descriptions.Item label="调度">
                        <Text type="secondary">{formatSchedule(job)}</Text>
                      </Descriptions.Item>
                      <Descriptions.Item label="缓存">
                        <Text type="secondary">{formatCache(job.useCacheS, job.useCacheT)}</Text>
                      </Descriptions.Item>
                      {job.exclude && (
                        <Descriptions.Item label="排除">
                          <Text type="secondary" ellipsis={{ tooltip: job.exclude }}>{job.exclude}</Text>
                        </Descriptions.Item>
                      )}
                      <Descriptions.Item label="创建">
                        <Text type="secondary">
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
        title={editingJob ? '编辑同步任务' : '新建同步任务'}
        open={drawerVisible}
        onClose={() => setDrawerVisible(false)}
        styles={{ wrapper: { width: 580 }, body: { padding: 16 } }}
        destroyOnClose
        extra={
          <Space>
            <Button onClick={() => setDrawerVisible(false)}>取消</Button>
            <Button type="primary" onClick={handleSubmit}>保存</Button>
          </Space>
        }
      >
        <Form form={form} layout="vertical">
          <Form.Item name="alistId" label="引擎" rules={[{ required: true, message: '请选择引擎' }]} style={compactItemStyle}>
            <Select
              placeholder="选择引擎"
              options={alistList.map((a) => ({
                value: a.id,
                label: `${a.userName} - ${a.url}${a.remark ? ` (${a.remark})` : ''}`,
              }))}
            />
          </Form.Item>
          <Row gutter={12}>
            <Col span={12}>
              <Form.Item name="srcPath" label="源目录" rules={[{ required: true, message: '请选择源目录' }]} style={compactItemStyle}>
                <TreeSelect
                  placeholder="选择源目录"
                  treeData={srcTreeData}
                  loadData={(node) => onLoadSrcData(node as TreeNode)}
                  treeDefaultExpandAll
                  multiple
                  treeCheckable
                  showCheckedStrategy={TreeSelect.SHOW_PARENT}
                  maxTagCount="responsive"
                  allowClear
                  showSearch
                  treeNodeFilterProp="title"
                  styles={{ popup: { root: { maxHeight: 300, overflow: 'auto' } } }}
                  suffixIcon={treeLoading ? <Spin size="small" /> : undefined}
                  notFoundContent={selectedAlistId ? '请先展开目录' : '请先选择引擎'}
                />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="dstPath" label="目标目录" rules={[{ required: true, message: '请选择目标目录' }]} style={compactItemStyle}>
                <TreeSelect
                  placeholder="选择目标目录"
                  treeData={dstTreeData}
                  loadData={(node) => onLoadDstData(node as TreeNode)}
                  treeDefaultExpandAll
                  multiple
                  maxTagCount="responsive"
                  allowClear
                  showSearch
                  treeNodeFilterProp="title"
                  styles={{ popup: { root: { maxHeight: 300, overflow: 'auto' } } }}
                  suffixIcon={treeLoading ? <Spin size="small" /> : undefined}
                  notFoundContent={selectedAlistId ? '请先展开目录' : '请先选择引擎'}
                />
              </Form.Item>
            </Col>
          </Row>
          <Form.Item name="remark" label="备注" style={compactItemStyle}>
            <Input placeholder="可选备注" />
          </Form.Item>

          <Divider style={compactDividerStyle}>同步配置</Divider>
          <Row gutter={12}>
            <Col span={12}>
              <Form.Item
                name="method"
                style={compactItemStyle}
                label={(
                  <Space size={4}>
                    同步方式
                    <Tooltip
                      title={(
                        <Space direction="vertical" size={2}>
                          {methodOptions.map((method) => (
                            <span key={method.name}>
                              <strong>{method.name}：</strong>{method.description}
                            </span>
                          ))}
                        </Space>
                      )}
                    >
                      <QuestionCircleOutlined style={{ color: '#8c8c8c' }} />
                    </Tooltip>
                  </Space>
                )}
              >
                <Select options={methodNames.map((n, i) => ({ value: i, label: n }))} />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item
                name="isCron"
                style={compactItemStyle}
                label={(
                  <Space size={4}>
                    调度方式
                    <Tooltip title={`预计执行计划：${schedulePlan}`}>
                      <QuestionCircleOutlined style={{ color: '#8c8c8c' }} />
                    </Tooltip>
                  </Space>
                )}
              >
                <Select
                  options={cronTypeNames.map((n, i) => ({ value: i, label: n }))}
                  onChange={(value) => {
                    if (value === 0) form.setFieldsValue({ interval: 1440 });
                    if (value === 1) {
                      form.setFieldsValue({
                        second: form.getFieldValue('second') || defaultCronFields.second,
                        minute: form.getFieldValue('minute') === '*' ? defaultCronFields.minute : form.getFieldValue('minute') || defaultCronFields.minute,
                        hour: form.getFieldValue('hour') === '*' ? defaultCronFields.hour : form.getFieldValue('hour') || defaultCronFields.hour,
                        day: form.getFieldValue('day') || defaultCronFields.day,
                        month: form.getFieldValue('month') || defaultCronFields.month,
                        day_of_week: form.getFieldValue('day_of_week') || defaultCronFields.day_of_week,
                      });
                    }
                  }}
                />
              </Form.Item>
            </Col>
            <Col span={12}>
              {isCronValue === 0 && (
                <Form.Item name="interval" label="间隔(分钟)" rules={[{ required: true, message: '请输入' }]} style={compactItemStyle}>
                  <InputNumber min={1} style={{ width: '100%' }} />
                </Form.Item>
              )}
            </Col>
          </Row>
          {isCronValue === 1 && (
            <Row gutter={8}>
              {cronFields.map((field) => (
                <Col span={4} key={field.name}>
                  <Form.Item name={field.name} label={field.label} rules={[{ required: true, message: '请输入' }]} style={compactItemStyle}>
                    <Input placeholder={field.placeholder} />
                  </Form.Item>
                </Col>
              ))}
            </Row>
          )}

          <Divider style={compactDividerStyle}>缓存与扫描</Divider>
          <Row gutter={12}>
            <Col span={6}>
              <Form.Item name="useCacheS" label="源端缓存" valuePropName="checked" style={compactItemStyle}>
                <Switch checkedChildren="使用" unCheckedChildren="不使用" />
              </Form.Item>
            </Col>
            <Col span={6}>
              <Form.Item name="scanIntervalS" label="源端扫描(秒)" style={compactItemStyle}>
                <InputNumber min={0} style={{ width: '100%' }} placeholder="0 默认" />
              </Form.Item>
            </Col>
            <Col span={6}>
              <Form.Item name="useCacheT" label="目标缓存" valuePropName="checked" style={compactItemStyle}>
                <Switch checkedChildren="使用" unCheckedChildren="不使用" />
              </Form.Item>
            </Col>
            <Col span={6}>
              <Form.Item name="scanIntervalT" label="目标扫描(秒)" style={compactItemStyle}>
                <InputNumber min={0} style={{ width: '100%' }} placeholder="0 默认" />
              </Form.Item>
            </Col>
          </Row>

          <div style={{ marginTop: 4 }}>
            <Form.Item name="exclude" label="排除项" tooltip="gitignore 语法，每行一条" style={compactItemStyle}>
              <Input.TextArea placeholder={'如\n*.tmp\n.git/'} rows={8} />
            </Form.Item>
          </div>

          <Divider style={compactDividerStyle} />
          <Form.Item name="enable" label="启用" valuePropName="checked" style={compactItemStyle}>
            <Switch />
          </Form.Item>
        </Form>
      </Drawer>

      <Drawer
        title={`任务列表 — 同步任务 #${taskDrawerJobId}`}
        open={!!taskDrawerJobId}
        onClose={closeTaskDrawer}
        styles={{ wrapper: { width: 'min(1180px, 96vw)' } }}
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
        styles={{ wrapper: { width: 'min(1040px, 94vw)' } }}
        destroyOnClose
      >
        <TaskDetail key={taskDetailDrawerTaskId} taskId={taskDetailDrawerTaskId} embedded />
      </Drawer>
    </div>
  );
}
