import { useState, useEffect, useRef, useCallback, useMemo } from 'react';
import { Card, Table, Tag, Button, Space, Popconfirm, App, Progress, Row, Col, Statistic, Empty, Typography, Tooltip, Spin, Pagination, Tabs, List } from 'antd';
import { StopOutlined, ThunderboltOutlined, ClockCircleOutlined, DashboardOutlined } from '@ant-design/icons';
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
  { key: 0, label: '等待', numKey: 'wait' },
  { key: 1, label: '运行中', numKey: 'running' },
  { key: 2, label: '成功', numKey: 'success' },
  { key: 7, label: '失败', numKey: 'fail' },
  { key: -1, label: '其他', numKey: 'other' },
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
  const [nowTick, setNowTick] = useState(() => Math.floor(Date.now() / 1000));

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
  useEffect(() => {
    if (!currentTask) return;
    const tickID = setInterval(() => {
      setNowTick(Math.floor(Date.now() / 1000));
    }, 1000);
    return () => { clearInterval(tickID); };
  }, [currentTask]);
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
      render: (v: number) => v ?? '-',
    },
    {
      title: '失败', dataIndex: 'failNum', key: 'failNum', width: 80,
      render: (v: number) => v ?? '-',
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

  const renderCurrentTaskItems = () => (
    <Spin spinning={tabLoading} size="small">
      {tabTaskList.length === 0 && !tabLoading ? (
        <Empty
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          description={<Text type="secondary">暂无{statusTabs.find(t => t.key === activeTab)?.label}任务</Text>}
        />
      ) : (
        <div style={{ maxHeight: 240, overflowY: 'auto' }}>
          <List
            size="small"
            dataSource={pagedTabTaskList}
            renderItem={(t, i) => {
              let name = t.fileName || '';
              const path = t.dstPath || t.srcPath || '';
              if (!name && path) {
                const cleanPath = path.replace(/\/+$/, '');
                name = cleanPath.split('/').pop() || cleanPath;
              }

              return (
                <List.Item key={t.fileName + '_' + i}>
                  <List.Item.Meta
                    title={(
                      <Space>
                        <Tag color={t.type === 1 ? 'red' : t.type === 2 ? 'orange' : 'blue'}>
                          {typeNames[t.type ?? 0] || '复制'}
                        </Tag>
                        <Tooltip title={<div>{t.dstPath && <div>目标: {t.dstPath}</div>}{t.srcPath && <div>来源: {t.srcPath}</div>}</div>}>
                          <Text ellipsis>{name || '--'}</Text>
                        </Tooltip>
                      </Space>
                    )}
                    description={path && (
                      <Tooltip title={path}>
                        <Text type="secondary" ellipsis>{path}</Text>
                      </Tooltip>
                    )}
                  />
                  <Space>
                    {(t.fileSize || 0) > 0 && <Text type="secondary">{formatSize(t.fileSize || 0)}</Text>}
                    {activeTab === 1 && (
                      <Progress
                        percent={Math.round(Number(t.progress || 0))}
                        size="small"
                        style={{ width: 120 }}
                      />
                    )}
                    {activeTab === 7 && t.errMsg && (
                      <Tooltip title={t.errMsg}>
                        <Text type="danger" ellipsis>失败原因</Text>
                      </Tooltip>
                    )}
                  </Space>
                </List.Item>
              );
            }}
          />
        </div>
      )}
      {tabTaskList.length > TAB_TASK_PAGE_SIZE && (
        <Pagination
          current={tabTaskPage}
          pageSize={TAB_TASK_PAGE_SIZE}
          total={tabTaskList.length}
          onChange={setTabTaskPage}
          showSizeChanger={false}
          size="small"
        />
      )}
    </Spin>
  );

  const displayDuration = currentTask
    ? Math.max(currentTask.duration || 0, currentTask.createTime ? nowTick - currentTask.createTime : 0)
    : 0;

  return (
    <div>
      {currentTask && (
        <div className="page-header">
          <Space>
            <Button icon={<StopOutlined />} danger onClick={handleAbort}>中止</Button>
          </Space>
        </div>
      )}

      {currentTask && (
        <Card
          size="small"
          title="实时进度"
          extra={(
            <Tag color={currentTask.scanFinish ? 'success' : 'processing'}>
              {currentTask.scanFinish ? '扫描完成，同步中' : '进行中'}
            </Tag>
          )}
          style={{ marginBottom: 16 }}
        >
          <Row gutter={[16, 12]}>
            <Col xs={8} sm={4}>
              <Statistic
                title={<Space size={4}><ClockCircleOutlined />耗时</Space>}
                value={formatDuration(displayDuration)}
              />
            </Col>
            <Col xs={8} sm={4}>
              <Statistic
                title={<Space size={4}><DashboardOutlined />平均速度</Space>}
                value={currentTask.speedAvg > 0 ? formatSize(currentTask.speedAvg) : '--'}
                suffix={currentTask.speedAvg > 0 ? '/s' : ''}
              />
            </Col>
            <Col xs={8} sm={4}>
              <Statistic
                title={<Space size={4}><ThunderboltOutlined />瞬时速度</Space>}
                value={currentTask.speed > 0 ? formatSize(currentTask.speed) : '--'}
                suffix={currentTask.speed > 0 ? '/s' : ''}
              />
            </Col>
            <Col xs={8} sm={4}>
              <Statistic
                title="预计剩余"
                value={currentTask.remainTime > 0 ? formatDuration(currentTask.remainTime) : '--'}
              />
            </Col>
            <Col xs={8} sm={4}>
              <Statistic
                title="已传输"
                value={formatSize(currentTask.doneSize || 0)}
              />
            </Col>
            <Col xs={8} sm={4}>
              <Statistic
                title="剩余"
                value={formatSize(currentTask.remainSize || 0)}
              />
            </Col>
          </Row>

          <Tabs
            activeKey={String(activeTab)}
            onChange={(key) => handleTabChange(Number(key))}
            items={statusTabs.map((tab) => ({
              key: String(tab.key),
              label: `${tab.label} (${currentTask.num?.[tab.numKey] || 0})`,
              children: renderCurrentTaskItems(),
            }))}
          />
          <Text type="secondary">
            开始: {currentTask.createTime ? dayjs.unix(currentTask.createTime).format('HH:mm:ss') : '--'}
          </Text>
        </Card>
      )}

      {list.length === 0 && !loading ? (
        <Empty
          description={<Text type="secondary">暂无任务记录，执行同步任务后将在此显示任务进度</Text>}
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
