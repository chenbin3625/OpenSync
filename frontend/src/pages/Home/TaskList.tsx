import { useState, useEffect, useRef, useCallback, useMemo } from 'react';
import { Card, Table, Tag, Button, Space, Popconfirm, App, Progress, Row, Col, Statistic, Empty, Typography, Tooltip, Divider, Spin, Pagination } from 'antd';
import { StopOutlined, UnorderedListOutlined, ThunderboltOutlined, ClockCircleOutlined, DashboardOutlined } from '@ant-design/icons';
import { jobGetTask, jobGetTaskCurrent, jobDeleteTask, jobPut } from '../../api/job';
import dayjs from 'dayjs';
import type { CurrentTaskData, CurrentTaskView, TaskItem, TaskRecord } from '../../types';

const { Text } = Typography;

/** 格式化秒数为可读时间 */
function formatDuration(seconds: number): string {
  if (seconds < 0) seconds = 0;
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const secs = Math.floor(seconds % 60);
  const parts: string[] = [];
  if (days > 0) parts.push(`${days}天`);
  if (hours > 0) parts.push(`${hours}小时`);
  if (minutes > 0) parts.push(`${minutes}分`);
  if (secs > 0 || parts.length === 0) parts.push(`${secs}秒`);
  return parts.join(' ');
}

/** 格式化文件大小 */
function formatSize(bytes: number): string {
  if (bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  let size = bytes;
  while (size >= 1024 && i < units.length - 1) {
    size /= 1024;
    i++;
  }
  return `${size.toFixed(i === 0 ? 0 : 2)} ${units[i]}`;
}

const typeNames: Record<number, string> = { 0: '复制', 1: '删除', 2: '移动' };
const TAB_TASK_PAGE_SIZE = 10;

const statusColors: Record<number, string> = {
  0: 'default', 1: 'processing', 2: 'success', 3: 'warning',
  4: 'default', 5: 'warning', 6: 'error', 7: 'error',
};
const statusNames: Record<number, string> = {
  0: '等待中', 1: '运行中', 2: '成功', 3: '部分失败',
  4: '已中止', 5: '超时', 6: '失败', 7: '已停止', 8: '无需同步',
};

/** 实时任务 Tab 状态定义 */
const statusTabs = [
  { key: 0, label: '等待', color: '#8c8c8c', numKey: 'wait' },
  { key: 1, label: '运行中', color: '#1677ff', numKey: 'running' },
  { key: 2, label: '成功', color: '#52c41a', numKey: 'success' },
  { key: 7, label: '失败', color: '#f5222d', numKey: 'fail' },
  { key: -1, label: '其他', color: '#faad14', numKey: 'other' },
] as const;

function getTaskCreateTime(task: TaskItem): number {
  const createTime = Number(task.createTime);
  return Number.isFinite(createTime) ? createTime : 0;
}

function isCurrentTaskData(data: CurrentTaskData | TaskItem[] | null): data is CurrentTaskData {
  return !!data && !Array.isArray(data) && Array.isArray(data.doingTask);
}

export default function TaskList({ jobId, onTaskDetail }: { jobId: string; onTaskDetail?: (taskId: number) => void }) {
  const { message } = App.useApp();
  const [list, setList] = useState<TaskRecord[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [loading, setLoading] = useState(false);
  const [currentTask, setCurrentTask] = useState<CurrentTaskView | null>(null);
  const prevTaskRef = useRef<CurrentTaskView | null>(null);
  const [activeTab, setActiveTab] = useState<number>(1);
  const [tabTaskList, setTabTaskList] = useState<TaskItem[]>([]);
  const [tabLoading, setTabLoading] = useState(false);
  const [tabTaskPage, setTabTaskPage] = useState(1);

  /** 计算速度与剩余信息 */
  const calcProgress = useCallback((cur: CurrentTaskData) => {
    const doingTask = cur.doingTask || [];
    const doingSize = doingTask.reduce((sum, obj) => {
      const progress = Number(obj.progress || 0);
      return sum + (obj.fileSize || 0) * progress / 100.0;
    }, 0);
    const sizeMap = cur.size || {};
    const remainSize = (sizeMap.running || 0) - doingSize + (sizeMap.wait || 0);
    const doneSize = (sizeMap.success || 0) + doingSize;

    // 瞬时速度
    let speed = 0;
    const prev = prevTaskRef.current;
    if (prev && cur.duration !== prev.duration) {
      speed = (doneSize - (prev.doneSize || 0)) / (cur.duration - prev.duration);
    }

    // 平均速度
    let speedAvg = 0;
    if (cur.firstSync && cur.duration > 0) {
      const syncDuration = cur.duration - (cur.firstSync - cur.createTime);
      if (syncDuration > 0) speedAvg = doneSize / syncDuration;
    }

    // 预计剩余时间
    let remainTime = 0;
    if (speedAvg > 0 && remainSize > 0) {
      remainTime = Math.ceil(remainSize / speedAvg);
    }

    return { remainSize, doneSize, speed, speedAvg, remainTime };
  }, []);

  const fetchList = useCallback(async () => {
    if (!jobId) return;
    setLoading(true);
    try {
      const res = await jobGetTask({ id: jobId, pageSize, pageNum: page });
      setList(res.data?.dataList || []);
      setTotal(res.data?.count || 0);
    } catch { /* ignore */ }
    setLoading(false);
  }, [jobId, page, pageSize]);

  const fetchCurrent = useCallback(async () => {
    if (!jobId) return;
    try {
      const res = await jobGetTaskCurrent({ id: jobId });
      const data = res.data || null;
      if (isCurrentTaskData(data)) {
        const progress = calcProgress(data);
        prevTaskRef.current = { ...data, ...progress };
        setCurrentTask({ ...data, ...progress });
      } else {
        setCurrentTask(null);
        prevTaskRef.current = null;
      }
    } catch { setCurrentTask(null); prevTaskRef.current = null; }
  }, [calcProgress, jobId]);

  /** 获取 Tab 对应状态的任务列表 */
  const fetchTabTasks = useCallback(async (status: number, task: CurrentTaskView | null) => {
    if (!jobId || !task) return;
    // 运行中 Tab 直接用 doingTask，不请求后端
    if (status === 1) {
      setTabTaskList(task.doingTask || []);
      return;
    }
    setTabLoading(true);
    try {
      const res = await jobGetTaskCurrent({ id: jobId, status });
      setTabTaskList(Array.isArray(res.data) ? res.data : []);
    } catch { setTabTaskList([]); }
    setTabLoading(false);
  }, [jobId]);

  const handleTabChange = (status: number) => {
    if (status === activeTab) return;
    setActiveTab(status);
    setTabTaskPage(1);
  };

  const pagedTabTaskList = useMemo(() => {
    const sortedList = [...tabTaskList].sort((a, b) => getTaskCreateTime(b) - getTaskCreateTime(a));
    const start = (tabTaskPage - 1) * TAB_TASK_PAGE_SIZE;
    return sortedList.slice(start, start + TAB_TASK_PAGE_SIZE);
  }, [tabTaskList, tabTaskPage]);

  useEffect(() => {
    const maxPage = Math.max(1, Math.ceil(tabTaskList.length / TAB_TASK_PAGE_SIZE));
    if (tabTaskPage > maxPage) {
      setTabTaskPage(maxPage);
    }
  }, [tabTaskList.length, tabTaskPage]);

  useEffect(() => { fetchList(); }, [fetchList]);
  useEffect(() => {
    fetchCurrent();
    const pollID = setInterval(fetchCurrent, 3000);
    return () => { clearInterval(pollID); };
  }, [fetchCurrent]);
  // Tab 数据加载：currentTask 变化时更新运行中 Tab，切换 Tab 时加载对应数据
  useEffect(() => {
    if (!currentTask) {
      setTabTaskList([]);
      return;
    }
    fetchTabTasks(activeTab, currentTask);
  }, [activeTab, currentTask, fetchTabTasks]);

  const handleDeleteTask = async (taskId: number) => {
    try {
      await jobDeleteTask(taskId);
      message.success('删除成功');
      fetchList();
    } catch { /* ignore */ }
  };

  const handleAbort = async () => {
    try {
      await jobPut({ id: jobId, pause: true, abort: true });
      message.success('已中止');
    } catch { /* ignore */ }
  };

  const columns = [
    {
      title: '状态', dataIndex: 'status', key: 'status', width: 100,
      render: (s: number) => <Tag color={statusColors[s]}>{statusNames[s] || s}</Tag>,
    },
    {
      title: '开始时间', dataIndex: 'runTime', key: 'runTime',
      render: (t: number) => t ? dayjs.unix(t).format('YYYY-MM-DD HH:mm:ss') : '-',
    },
    {
      title: '成功', dataIndex: 'successNum', key: 'successNum', width: 80,
      render: (v: number) => <Text style={{ color: '#52c41a' }}>{v ?? '-'}</Text>,
    },
    {
      title: '失败', dataIndex: 'failNum', key: 'failNum', width: 80,
      render: (v: number) => <Text style={{ color: '#f5222d' }}>{v ?? '-'}</Text>,
    },
    { title: '总计', dataIndex: 'allNum', key: 'allNum', width: 80 },
    {
      title: '操作', key: 'action', width: 120,
      render: (_: unknown, record: TaskRecord) => (
        <Space>
          <Button size="small" onClick={() => onTaskDetail?.(record.id)}>详情</Button>
          <Popconfirm title="确认删除此任务？" onConfirm={() => handleDeleteTask(record.id)}>
            <Button size="small" danger>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div>
      {currentTask && (
        <div className="page-header" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
          <Space>
            <Button icon={<StopOutlined />} danger onClick={handleAbort}>中止</Button>
          </Space>
        </div>
      )}

      {currentTask && (
        <Card
          size="small"
          style={{ marginBottom: 16 }}
          styles={{
            body: { padding: '16px 20px' },
            header: { minHeight: 40, fontSize: 14 },
          }}
        >
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 12 }}>
            <Text strong style={{ fontSize: 14 }}>实时进度</Text>
            <Tag color={currentTask.scanFinish ? 'success' : 'processing'}>
              {currentTask.scanFinish ? '扫描完成，同步中' : '进行中'}
            </Tag>
          </div>

          {/* 统计指标 */}
          <Row gutter={[16, 12]}>
            <Col xs={8} sm={4}>
              <Statistic
                title={<span><ClockCircleOutlined style={{ marginRight: 4 }} />耗时</span>}
                value={formatDuration(currentTask.duration || 0)}
                valueStyle={{ fontSize: 14, color: 'var(--ant-color-text)' }}
              />
            </Col>
            <Col xs={8} sm={4}>
              <Statistic
                title={<span><DashboardOutlined style={{ marginRight: 4 }} />平均速度</span>}
                value={currentTask.speedAvg > 0 ? formatSize(currentTask.speedAvg) : '--'}
                suffix={currentTask.speedAvg > 0 ? '/s' : ''}
                valueStyle={{ fontSize: 14, color: 'var(--ant-color-text)' }}
              />
            </Col>
            <Col xs={8} sm={4}>
              <Statistic
                title={<span><ThunderboltOutlined style={{ marginRight: 4 }} />瞬时速度</span>}
                value={currentTask.speed > 0 ? formatSize(currentTask.speed) : '--'}
                suffix={currentTask.speed > 0 ? '/s' : ''}
                valueStyle={{ fontSize: 14, color: 'var(--ant-color-text)' }}
              />
            </Col>
            <Col xs={8} sm={4}>
              <Statistic
                title="预计剩余"
                value={currentTask.remainTime > 0 ? formatDuration(currentTask.remainTime) : '--'}
                valueStyle={{ fontSize: 14, color: 'var(--ant-color-text)' }}
              />
            </Col>
            <Col xs={8} sm={4}>
              <Statistic
                title="已传输"
                value={formatSize(currentTask.doneSize || 0)}
                valueStyle={{ fontSize: 14, color: '#52c41a' }}
              />
            </Col>
            <Col xs={8} sm={4}>
              <Statistic
                title="剩余"
                value={formatSize(currentTask.remainSize || 0)}
                valueStyle={{ fontSize: 14, color: 'var(--ant-color-text-secondary)' }}
              />
            </Col>
          </Row>

          {/* 状态 Tab 切换 + 任务列表 */}
          <Divider style={{ margin: '12px 0' }} />
          <div style={{ display: 'flex', alignItems: 'center', gap: 0, borderBottom: '1px solid var(--ant-color-border-secondary)', marginBottom: 12 }}>
            {statusTabs.map((tab) => {
              const count = currentTask.num?.[tab.numKey] || 0;
              const isActive = activeTab === tab.key;
              return (
                <div
                  key={tab.key}
                  onClick={() => handleTabChange(tab.key)}
                  style={{
                    padding: '8px 16px',
                    cursor: 'pointer',
                    display: 'flex',
                    alignItems: 'center',
                    gap: 6,
                    borderBottom: isActive ? `2px solid ${tab.color}` : '2px solid transparent',
                    marginBottom: -1,
                    transition: 'all 0.2s',
                    color: isActive ? tab.color : 'var(--ant-color-text-secondary)',
                    fontWeight: isActive ? 600 : 400,
                    fontSize: 13,
                    userSelect: 'none',
                  }}
                >
                  {tab.label}
                  <span style={{
                    fontSize: 12,
                    background: isActive ? tab.color : 'var(--ant-color-fill-quaternary)',
                    color: isActive ? '#fff' : 'var(--ant-color-text-secondary)',
                    borderRadius: 10,
                    padding: '0 6px',
                    lineHeight: '18px',
                    minWidth: 20,
                    textAlign: 'center',
                  }}>{count}</span>
                </div>
              );
            })}
            <div style={{ flex: 1 }} />
            <Text type="secondary" style={{ fontSize: 12 }}>
              开始: {currentTask.createTime ? dayjs.unix(currentTask.createTime).format('HH:mm:ss') : '--'}
            </Text>
          </div>

          {/* Tab 内容 - 任务列表 */}
          <Spin spinning={tabLoading} size="small">
            {tabTaskList.length === 0 && !tabLoading ? (
              <Empty
                image={Empty.PRESENTED_IMAGE_SIMPLE}
                description={<Text type="secondary" style={{ fontSize: 12 }}>暂无{statusTabs.find(t => t.key === activeTab)?.label}任务</Text>}
                style={{ padding: '16px 0' }}
              />
            ) : (
              <div style={{ maxHeight: 240, overflowY: 'auto' }}>
                {pagedTabTaskList.map((t, i) => (
                  <div
                    key={t.fileName + '_' + i}
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 10,
                      padding: '6px 8px',
                      marginBottom: 2,
                      borderRadius: 6,
                      background: i % 2 === 0 ? 'var(--ant-color-fill-quaternary)' : 'transparent',
                    }}
                  >
                    <Tag
                      color={t.type === 1 ? 'red' : t.type === 2 ? 'orange' : 'blue'}
                      style={{ margin: 0, fontSize: 11, minWidth: 36, textAlign: 'center' }}
                    >
                      {typeNames[t.type ?? 0] || '复制'}
                    </Tag>
                    <Tooltip title={<div>{t.dstPath && <div>目标: {t.dstPath}</div>}{t.srcPath && <div>来源: {t.srcPath}</div>}</div>}>
                      {(() => {
                        // 优先使用 fileName，否则从 dstPath 或 srcPath 提取
                          let name = t.fileName || '';
                          const path = t.dstPath || t.srcPath || '';

                        // 如果 fileName 为空，从路径中提取文件名
                        if (!name && path) {
                          // 去除末尾的 /
                          const cleanPath = path.replace(/\/+$/, '');
                          name = cleanPath.split('/').pop() || cleanPath;
                        }

                        return (
                          <div style={{ flex: 1, minWidth: 0, lineHeight: 1.4 }}>
                            <Text style={{ fontSize: 12, display: 'block' }} ellipsis>{name || '--'}</Text>
                            {path && (
                              <Tooltip title={path}>
                                <Text type="secondary" style={{ fontSize: 11, display: 'block' }} ellipsis>{path}</Text>
                              </Tooltip>
                            )}
                          </div>
                        );
                      })()}
                    </Tooltip>
                    {(t.fileSize || 0) > 0 && (
                      <Text type="secondary" style={{ fontSize: 11, flexShrink: 0 }}>{formatSize(t.fileSize || 0)}</Text>
                    )}
                    {activeTab === 1 && (
                      <Progress
                        percent={Math.round(Number(t.progress || 0))}
                        size="small"
                        style={{ width: 120, flexShrink: 0 }}
                        strokeColor={Number(t.progress || 0) >= 100 ? '#52c41a' : '#1677ff'}
                      />
                    )}
                    {activeTab === 7 && t.errMsg && (
                      <Tooltip title={t.errMsg}>
                        <Text type="danger" style={{ fontSize: 11, maxWidth: 120 }} ellipsis>失败原因</Text>
                      </Tooltip>
                    )}
                  </div>
                ))}
              </div>
            )}
            {tabTaskList.length > TAB_TASK_PAGE_SIZE && (
              <div style={{ display: 'flex', justifyContent: 'flex-end', paddingTop: 10 }}>
                <Pagination
                  current={tabTaskPage}
                  pageSize={TAB_TASK_PAGE_SIZE}
                  total={tabTaskList.length}
                  onChange={setTabTaskPage}
                  showSizeChanger={false}
                  size="small"
                />
              </div>
            )}
          </Spin>
        </Card>
      )}

      {list.length === 0 && !loading ? (
        <Empty
          image={<UnorderedListOutlined style={{ fontSize: 64, color: '#bbb' }} />}
          styles={{ image: { height: 80 } }}
          description={<Text type="secondary">暂无任务记录，执行同步任务后将在此显示任务进度</Text>}
          className="empty-state-compact"
        />
      ) : (
        <Table
          dataSource={list}
          columns={columns}
          rowKey="id"
          loading={loading}
          pagination={{
            current: page,
            pageSize,
            total,
            onChange: (p, ps) => { setPage(p); setPageSize(ps); },
            showSizeChanger: true,
            showTotal: (t) => `共 ${t} 条`,
          }}
          size="middle"
        />
      )}
    </div>
  );
}
