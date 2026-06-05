import { useState, useEffect, useRef, useCallback, useMemo, type ReactNode } from 'react';
import { Card, Table, Tag, Button, Space, Popconfirm, App, Progress, Empty, Typography, Tooltip, Spin, Pagination, Tabs, DatePicker, Input, Select } from 'antd';
import {
  DeleteOutlined, EyeOutlined, PauseCircleOutlined, PlayCircleOutlined, ReloadOutlined,
  ThunderboltOutlined, ClockCircleOutlined, DashboardOutlined, FolderOpenOutlined,
} from '@ant-design/icons';
import { jobGetTask, jobGetTaskCurrent, jobDeleteTask, jobTaskAction } from '../../api/job';
import dayjs from 'dayjs';
import type { Dayjs } from 'dayjs';
import type { CurrentTaskData, CurrentTaskView, PageData, TaskItem, TaskRecord } from '../../types';
import {
  filterCurrentTaskFromHistory,
  filterRunningTaskRows,
  getTaskItemKey,
  mergeTaskItems,
  mergeTaskRecords,
  shouldPollRealtime,
  type TaskListView,
} from './taskRows';

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
const TAB_TASK_PAGE_SIZE = 20;

const statusColors: Record<number, string> = {
  0: 'default', 1: 'processing', 2: 'success', 3: 'warning',
  4: 'default', 5: 'warning', 6: 'error', 7: 'error',
};
const statusNames: Record<number, string> = {
  0: '等待中', 1: '运行中', 2: '成功', 3: '部分失败',
  4: '已中止', 5: '超时', 6: '失败', 7: '已停止', 8: '无需同步',
};
const historyStatusOptions = [2, 3, 4, 5, 6, 7, 8].map((value) => ({
  value,
  label: statusNames[value],
}));
type HistoryTimeRange = [Dayjs | null, Dayjs | null] | null;

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

function displayText(value: string | number | null | undefined): string {
  if (value === null || value === undefined || value === '') return '--';
  return String(value);
}

function getTaskDisplayName(task: TaskItem): string {
  if (task.fileName) return task.fileName;
  const path = task.dstPath || task.srcPath || '';
  if (!path) return '--';
  const cleanPath = path.replace(/\/+$/, '');
  return cleanPath.split('/').pop() || cleanPath;
}

function TaskInlineText({
  value,
  tooltip,
  type,
  className,
}: {
  value: string | number | null | undefined;
  tooltip?: ReactNode;
  type?: 'secondary' | 'danger';
  className?: string;
}) {
  const text = displayText(value);
  if (text === '--') return <Text type="secondary" className={className}>--</Text>;
  return (
    <Tooltip title={tooltip || text}>
      <Text type={type} ellipsis className={className}>
        {text}
      </Text>
    </Tooltip>
  );
}

function isCurrentTaskData(data: CurrentTaskData | PageData<TaskItem> | TaskItem[] | null): data is CurrentTaskData {
  return !!data && !Array.isArray(data) && 'doingTask' in data && Array.isArray(data.doingTask);
}

function isTaskItemPage(data: CurrentTaskData | PageData<TaskItem> | TaskItem[] | null): data is PageData<TaskItem> {
  return !!data && !Array.isArray(data) && 'dataList' in data && Array.isArray(data.dataList);
}

export default function TaskList({
  jobId,
  onTaskDetail,
  view = 'all',
}: {
  jobId: string;
  onTaskDetail?: (taskId: number) => void;
  view?: TaskListView;
}) {
  const { message } = App.useApp();
  const [list, setList] = useState<TaskRecord[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [loading, setLoading] = useState(false);
  const [historyStatusFilter, setHistoryStatusFilter] = useState<number | undefined>(undefined);
  const [historyKeywordInput, setHistoryKeywordInput] = useState('');
  const [historyKeywordFilter, setHistoryKeywordFilter] = useState('');
  const [historyTimeRange, setHistoryTimeRange] = useState<HistoryTimeRange>(null);
  const [currentTask, setCurrentTask] = useState<CurrentTaskView | null>(null);
  const prevTaskRef = useRef<CurrentTaskView | null>(null);
  const [activeTab, setActiveTab] = useState<number>(1);
  const [tabTaskList, setTabTaskList] = useState<TaskItem[]>([]);
  const [tabTaskTotal, setTabTaskTotal] = useState(0);
  const [tabLoading, setTabLoading] = useState(false);
  const [tabTaskPage, setTabTaskPage] = useState(1);
  const [nowTick, setNowTick] = useState(() => Math.floor(Date.now() / 1000));
  const activeTabRef = useRef(activeTab);
  const tabRequestRef = useRef(0);
  const listRequestRef = useRef(0);
  const listLoadingRequestRef = useRef(0);
  const lastLoadedTabRef = useRef<{ status: number; taskCreateTime?: number; page: number } | null>(null);
  const showRealtime = shouldPollRealtime(view);
  const showHistory = view === 'all' || view === 'history';

  useEffect(() => {
    activeTabRef.current = activeTab;
  }, [activeTab]);

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

  const fetchList = useCallback(async (showLoading = false) => {
    if (!jobId) return;
    const requestID = ++listRequestRef.current;
    const loadingRequestID = showLoading ? ++listLoadingRequestRef.current : 0;
    if (showLoading) setLoading(true);
    try {
      const params: Record<string, unknown> = { id: jobId, pageSize, pageNum: page };
      if (historyStatusFilter !== undefined) params.status = historyStatusFilter;
      if (historyKeywordFilter.trim()) params.keyword = historyKeywordFilter.trim();
      if (historyTimeRange?.[0]) params.startTime = historyTimeRange[0].startOf('day').unix();
      if (historyTimeRange?.[1]) params.endTime = historyTimeRange[1].endOf('day').unix();

      const res = await jobGetTask(params);
      if (requestID === listRequestRef.current) {
        setList((previous) => mergeTaskRecords(previous, res.data?.dataList || []));
        setTotal(res.data?.count || 0);
      }
    } catch { /* ignore */ }
    if (showLoading && loadingRequestID === listLoadingRequestRef.current) {
      setLoading(false);
    }
  }, [historyKeywordFilter, historyStatusFilter, historyTimeRange, jobId, page, pageSize]);

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
    } catch { /* keep the last visible realtime snapshot on transient polling errors */ }
  }, [calcProgress, jobId]);

  /** 获取 Tab 对应状态的任务列表 */
  const fetchTabTasks = useCallback(async (status: number, task: CurrentTaskView | null, pageNum: number, showLoading = false) => {
    if (!jobId || !task) return;
    const requestID = ++tabRequestRef.current;
    if (showLoading) {
      setTabTaskList([]);
      setTabTaskTotal(0);
      setTabLoading(true);
    }
    // 运行中 Tab 直接用 doingTask，不请求后端
    if (status === 1) {
      if (activeTabRef.current === status) {
        const doingTask = task.doingTask || [];
        setTabTaskList((previous) => mergeTaskItems(previous, doingTask));
        setTabTaskTotal(doingTask.length);
        setTabLoading(false);
      }
      return;
    }
    try {
      const res = await jobGetTaskCurrent({
        id: jobId,
        status,
        pageSize: TAB_TASK_PAGE_SIZE,
        pageNum,
      });
      if (activeTabRef.current === status && requestID === tabRequestRef.current) {
        const data = res.data || null;
        if (isTaskItemPage(data)) {
          setTabTaskList((previous) => mergeTaskItems(previous, data.dataList || []));
          setTabTaskTotal(data.count || 0);
        } else {
          const rows = Array.isArray(data) ? data : [];
          setTabTaskList((previous) => mergeTaskItems(previous, rows));
          setTabTaskTotal(rows.length);
        }
      }
    } catch {
      if (showLoading && activeTabRef.current === status && requestID === tabRequestRef.current) {
        setTabTaskList([]);
        setTabTaskTotal(0);
      }
    }
    if (activeTabRef.current === status && requestID === tabRequestRef.current) {
      setTabLoading(false);
    }
  }, [jobId]);

  const handleTabChange = (status: number) => {
    if (status === activeTab) return;
    setActiveTab(status);
    setTabTaskPage(1);
  };

  const pagedTabTaskList = useMemo(() => {
    const sortedList = [...tabTaskList].sort((a, b) => getTaskCreateTime(b) - getTaskCreateTime(a));
    if (activeTab !== 1) return sortedList;
    const start = (tabTaskPage - 1) * TAB_TASK_PAGE_SIZE;
    return sortedList.slice(start, start + TAB_TASK_PAGE_SIZE);
  }, [activeTab, tabTaskList, tabTaskPage]);

  useEffect(() => {
    const totalRows = activeTab === 1 ? tabTaskList.length : tabTaskTotal;
    const maxPage = Math.max(1, Math.ceil(totalRows / TAB_TASK_PAGE_SIZE));
    if (tabTaskPage > maxPage) {
      setTabTaskPage(maxPage);
    }
  }, [activeTab, tabTaskList.length, tabTaskPage, tabTaskTotal]);

  useEffect(() => {
    setPage(1);
    setTabTaskPage(1);
    setList([]);
    setTotal(0);
    setCurrentTask(null);
    setTabTaskList([]);
    setTabTaskTotal(0);
    prevTaskRef.current = null;
    lastLoadedTabRef.current = null;
  }, [jobId]);

  useEffect(() => {
    if (showHistory) fetchList(true);
  }, [fetchList, showHistory]);
  useEffect(() => {
    if (!showHistory) return undefined;
    const pollID = setInterval(() => fetchList(false), 3000);
    return () => { clearInterval(pollID); };
  }, [fetchList, showHistory]);
  useEffect(() => {
    if (!showRealtime) return undefined;
    fetchCurrent();
    const pollID = setInterval(fetchCurrent, 3000);
    return () => { clearInterval(pollID); };
  }, [fetchCurrent, showRealtime]);
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
      setTabTaskTotal(0);
      setTabLoading(false);
      lastLoadedTabRef.current = null;
      return;
    }
    const lastLoadedTab = lastLoadedTabRef.current;
    const showLoading = !lastLoadedTab ||
      lastLoadedTab.status !== activeTab ||
      lastLoadedTab.taskCreateTime !== currentTask.createTime ||
      lastLoadedTab.page !== tabTaskPage;
    lastLoadedTabRef.current = {
      status: activeTab,
      taskCreateTime: currentTask.createTime,
      page: tabTaskPage,
    };
    fetchTabTasks(activeTab, currentTask, tabTaskPage, showLoading);
  }, [activeTab, currentTask, fetchTabTasks, tabTaskPage]);

  const handleDeleteTask = async (taskId: number) => {
    try {
      await jobDeleteTask(taskId);
      message.success('删除成功');
      fetchList(false);
    } catch { /* ignore */ }
  };

  const handleTaskAction = async (
    taskId: number,
    action: 'pause' | 'resume' | 'restart' | 'retryFailed',
    successText: string,
  ) => {
    try {
      await jobTaskAction(taskId, action);
      message.success(successText);
      fetchList(false);
      fetchCurrent();
    } catch { /* ignore */ }
  };

  const handleHistoryKeywordSearch = (value: string) => {
    setHistoryKeywordFilter(value.trim());
    setPage(1);
  };

  const resetHistoryFilters = () => {
    setHistoryStatusFilter(undefined);
    setHistoryKeywordInput('');
    setHistoryKeywordFilter('');
    setHistoryTimeRange(null);
    setPage(1);
  };

  const hasHistoryFilters = historyStatusFilter !== undefined ||
    !!historyKeywordFilter ||
    !!historyTimeRange?.[0] ||
    !!historyTimeRange?.[1];

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
      title: '操作', key: 'action', width: 136,
      render: (_: unknown, record: TaskRecord) => (
        <Space size={4} wrap>
          <Tooltip title="详情">
            <Button
              size="small"
              type="text"
              icon={<EyeOutlined />}
              aria-label="详情"
              onClick={() => onTaskDetail?.(record.id)}
            />
          </Tooltip>
          {record.status === 7 && (
            <Tooltip title="继续">
              <Button
                size="small"
                type="text"
                icon={<PlayCircleOutlined />}
                aria-label="继续"
                onClick={() => handleTaskAction(record.id, 'resume', '已提交继续执行')}
              />
            </Tooltip>
          )}
          {(record.failNum || 0) > 0 && (
            <Tooltip title="重试失败">
              <Button
                size="small"
                type="text"
                icon={<ReloadOutlined />}
                aria-label="重试失败"
                onClick={() => handleTaskAction(record.id, 'retryFailed', '已提交失败项重试')}
              />
            </Tooltip>
          )}
          <Tooltip title="删除">
            <Popconfirm title="确认删除此任务？" onConfirm={() => handleDeleteTask(record.id)}>
              <Button
                size="small"
                type="text"
                danger
                icon={<DeleteOutlined />}
                aria-label="删除"
              />
            </Popconfirm>
          </Tooltip>
        </Space>
      ),
    },
  ];

  const renderCurrentTaskItems = () => (
    <Spin spinning={tabLoading && tabTaskList.length === 0} size="small">
      {tabTaskList.length === 0 && !tabLoading ? (
        <Empty
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          description={<Text type="secondary">暂无{statusTabs.find(t => t.key === activeTab)?.label}任务</Text>}
        />
      ) : (
        <div className="task-progress-list">
          <div className="task-progress-file-rows">
            {pagedTabTaskList.map((t, i) => {
              const name = getTaskDisplayName(t);
              const srcPath = displayText(t.srcPath);
              const dstPath = displayText(t.dstPath);
              const tooltip = (
                <div>
                  <div>文件: {name}</div>
                  <div>来源: {srcPath}</div>
                  <div>目标: {dstPath}</div>
                </div>
              );
              const rowKey = getTaskItemKey(t, i);

              return (
                <div className="task-progress-file-row" key={rowKey}>
                  <Tag color={t.type === 1 ? 'red' : t.type === 2 ? 'orange' : 'blue'}>
                    {typeNames[t.type ?? 0] || '复制'}
                  </Tag>
                  <TaskInlineText className="task-progress-file-name" value={name} tooltip={tooltip} />
                  <TaskInlineText className="task-progress-file-path" value={t.srcPath} tooltip={srcPath} type="secondary" />
                  <TaskInlineText className="task-progress-file-path" value={t.dstPath} tooltip={dstPath} type="secondary" />
                  <Text type="secondary" className="task-progress-file-size">
                    {formatSize(t.fileSize || 0)}
                  </Text>
                  <span className="task-progress-file-state">
                    {activeTab === 1 && (
                      <Progress
                        percent={Math.round(Number(t.progress || 0))}
                        size="small"
                      />
                    )}
                    {activeTab === 7 && t.errMsg && (
                      <Tooltip title={t.errMsg}>
                        <Text type="danger" ellipsis className="task-progress-file-error">失败原因</Text>
                      </Tooltip>
                    )}
                    {activeTab !== 1 && !(activeTab === 7 && t.errMsg) && (
                      <Text type="secondary">
                        {t.createTime ? dayjs.unix(t.createTime).format('HH:mm:ss') : '--'}
                      </Text>
                    )}
                  </span>
                </div>
              );
            })}
          </div>
        </div>
      )}
      {tabTaskTotal > 0 && (
        <Pagination
          className="task-progress-pagination"
          current={tabTaskPage}
          pageSize={TAB_TASK_PAGE_SIZE}
          total={tabTaskTotal}
          onChange={setTabTaskPage}
          showTotal={(t) => `共 ${t} 条`}
          showSizeChanger={false}
          size="small"
        />
      )}
    </Spin>
  );

  const historyList = useMemo(
    () => showRealtime ? filterCurrentTaskFromHistory(list, currentTask) : filterRunningTaskRows(list),
    [currentTask, list, showRealtime],
  );
  const hiddenCurrentTaskCount = list.length - historyList.length;
  const historyTotal = Math.max(0, total - hiddenCurrentTaskCount);

  const displayDuration = currentTask
    ? Math.max(currentTask.duration || 0, currentTask.createTime ? nowTick - currentTask.createTime : 0)
    : 0;
  const scanProgress = currentTask?.scan;
  const progressMetrics = currentTask ? [
    {
      key: 'duration',
      label: '耗时',
      icon: <ClockCircleOutlined />,
      value: formatDuration(displayDuration),
    },
    {
      key: 'speedAvg',
      label: '平均速度',
      icon: <DashboardOutlined />,
      value: currentTask.speedAvg > 0 ? `${formatSize(currentTask.speedAvg)}/s` : '--',
    },
    {
      key: 'speed',
      label: '瞬时速度',
      icon: <ThunderboltOutlined />,
      value: currentTask.speed > 0 ? `${formatSize(currentTask.speed)}/s` : '--',
    },
    {
      key: 'remainTime',
      label: '预计剩余',
      value: currentTask.remainTime > 0 ? formatDuration(currentTask.remainTime) : '--',
    },
    {
      key: 'doneSize',
      label: '已传输',
      value: formatSize(currentTask.doneSize || 0),
    },
    {
      key: 'remainSize',
      label: '剩余',
      value: formatSize(currentTask.remainSize || 0),
    },
  ] : [];

  const realtimeContent = currentTask ? (
    <>
      <div className="page-header">
        <Space>
          <Button
            icon={<PauseCircleOutlined />}
            onClick={() => handleTaskAction(currentTask.taskId, 'pause', '已暂停')}
          >
            暂停
          </Button>
        </Space>
      </div>
      <Card
        className="task-progress-card"
        size="small"
        title="实时进度"
        extra={(
          <Space size={8} wrap className="task-progress-card-extra">
            <Text type="secondary" className="task-progress-start-time">
              开始: {currentTask.createTime ? dayjs.unix(currentTask.createTime).format('HH:mm:ss') : '--'}
            </Text>
            <Tag color={currentTask.scanFinish ? 'success' : 'processing'}>
              {currentTask.scanFinish ? '扫描完成，同步中' : '进行中'}
            </Tag>
          </Space>
        )}
        style={{ marginBottom: 12 }}
      >
        {scanProgress && (
          <div className="task-progress-scan">
            <div className="task-progress-scan-header">
              <Text strong className="task-progress-scan-title"><FolderOpenOutlined /> 目录扫描</Text>
              <Space size={10} wrap className="task-progress-scan-counts">
                <Text type="secondary">{scanProgress.totalDirs} 个目录</Text>
              </Space>
            </div>
          </div>
        )}

        <div className="task-progress-metrics">
          {progressMetrics.map((item) => (
            <div className="task-progress-metric" key={item.key}>
              <Text type="secondary" className="task-progress-metric-label">
                {item.icon}
                {item.label}
              </Text>
              <Text strong className="task-progress-metric-value">{item.value}</Text>
            </div>
          ))}
        </div>

        <Tabs
          className="task-progress-tabs"
          size="small"
          activeKey={String(activeTab)}
          onChange={(key) => handleTabChange(Number(key))}
          items={statusTabs.map((tab) => ({
            key: String(tab.key),
            label: `${tab.label} (${currentTask.num?.[tab.numKey] || 0})`,
            children: tab.key === activeTab ? renderCurrentTaskItems() : null,
          }))}
        />
      </Card>
    </>
  ) : (
    <Empty
      image={Empty.PRESENTED_IMAGE_SIMPLE}
      description={<Text type="secondary">当前没有正在同步的任务</Text>}
    />
  );

  const historyBody = historyList.length === 0 && !loading ? (
    <Empty
      description={<Text type="secondary">暂无历史任务记录，执行完成后将在此显示</Text>}
    />
  ) : (
    <Table
      dataSource={historyList}
      columns={columns}
      rowKey="id"
      loading={loading}
      pagination={{
        current: page,
        pageSize,
        total: historyTotal,
        onChange: (p, ps) => { setPage(p); setPageSize(ps); },
        showSizeChanger: true,
        showTotal: (t) => `共 ${t} 条`,
      }}
      size="middle"
    />
  );

  const historyContent = (
    <>
      <Space wrap className="task-history-filters">
        <Input.Search
          placeholder="任务 ID"
          allowClear
          style={{ width: 180 }}
          value={historyKeywordInput}
          onChange={(event) => {
            setHistoryKeywordInput(event.target.value);
            if (!event.target.value) handleHistoryKeywordSearch('');
          }}
          onSearch={handleHistoryKeywordSearch}
        />
        <Select
          placeholder="任务状态"
          allowClear
          style={{ width: 140 }}
          value={historyStatusFilter}
          onChange={(value) => {
            setHistoryStatusFilter(value);
            setPage(1);
          }}
          options={historyStatusOptions}
        />
        <DatePicker.RangePicker
          className="task-history-time-range"
          value={historyTimeRange || undefined}
          onChange={(value) => {
            setHistoryTimeRange(value as HistoryTimeRange);
            setPage(1);
          }}
          placeholder={['开始日期', '结束日期']}
        />
        <Button onClick={resetHistoryFilters} disabled={!hasHistoryFilters}>重置</Button>
      </Space>
      {historyBody}
    </>
  );

  return (
    <div>
      {showRealtime && realtimeContent}
      {showHistory && historyContent}
    </div>
  );
}
