import { useState, useEffect, useRef, useCallback, useMemo, type ReactNode } from 'react';
import { Card, Table, Tag, Button, Space, Popconfirm, App, Progress, Empty, Typography, Tooltip, Spin, Pagination, Tabs, DatePicker, Input, Select } from 'antd';
import {
  DeleteOutlined, EyeOutlined, StopOutlined, RedoOutlined,
  ThunderboltOutlined, ClockCircleOutlined, DashboardOutlined, FolderOpenOutlined,
  ReloadOutlined, InfoCircleOutlined,
} from '@ant-design/icons';
import { jobGetTask, jobDeleteTask, jobTaskAction } from '../../api/job';
import dayjs from 'dayjs';
import type { Dayjs } from 'dayjs';
import type { CurrentTaskView, TaskItem, TaskRecord } from '../../types';
import {
  filterCurrentTaskFromHistory,
  filterRunningTaskRows,
  getTaskItemKey,
  mergeTaskRecords,
  shouldPollRealtime,
  type TaskListView,
} from './taskRows';
import { useRealtimeTask } from './useRealtimeTask';
import { useRealtimeTaskItems } from './useRealtimeTaskItems';
import { canPollCurrentDocument } from './pollingVisibility';
import { displayText, formatSize, taskStatusColors, taskTypeNames } from './homeUtils';

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

const TAB_TASK_PAGE_SIZE = 20;

const statusNames: Record<number, string> = {
  0: '等待中', 1: '运行中', 2: '成功', 3: '部分失败',
  4: '已中止', 5: '超时', 6: '失败', 7: '已停止', 8: '无需同步',
};
const historyStatusOptions = [2, 3, 4, 5, 6, 7, 8].map((value) => ({
  value,
  label: statusNames[value],
}));
const historyCompletedStatuses = [2, 3, 4, 5, 6, 7, 8];
type HistoryTimeRange = [Dayjs | null, Dayjs | null] | null;

/** 实时任务 Tab 状态定义 */
const statusTabs = [
  { key: 0, label: '等待', numKey: 'wait' },
  { key: 1, label: '运行中', numKey: 'running' },
  { key: 2, label: '成功', numKey: 'success' },
  { key: 7, label: '失败', numKey: 'fail' },
  { key: -1, label: '其他', numKey: 'other' },
] as const;

type ProgressMetric = {
  key: string;
  label: string;
  icon?: ReactNode;
  value: string;
};

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

function RealtimeTaskItems({
  activeTab,
  loading,
  page,
  pageSize,
  rows,
  total,
  visibleRows,
  onPageChange,
}: {
  activeTab: number;
  loading: boolean;
  page: number;
  pageSize: number;
  rows: TaskItem[];
  total: number;
  visibleRows: TaskItem[];
  onPageChange: (page: number) => void;
}) {
  const activeTabLabel = statusTabs.find((tab) => tab.key === activeTab)?.label || '';

  return (
    <Spin spinning={loading} size="small">
      {rows.length === 0 ? (
        <Empty
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          description={<Text type="secondary">暂无{activeTabLabel}任务</Text>}
        />
      ) : (
        <div className="task-progress-list">
          <div className="task-progress-file-rows">
            {visibleRows.map((task, index) => {
              const name = getTaskDisplayName(task);
              const srcPath = displayText(task.srcPath);
              const dstPath = displayText(task.dstPath);
              const tooltip = (
                <div>
                  <div>文件: {name}</div>
                  <div>来源: {srcPath}</div>
                  <div>目标: {dstPath}</div>
                </div>
              );
              const rowKey = getTaskItemKey(task, index);

              return (
                <div className="task-progress-file-row" key={rowKey}>
                  <Tag color={task.type === 1 ? 'red' : task.type === 2 ? 'orange' : 'blue'}>
                    {taskTypeNames[task.type ?? 0] || '复制'}
                  </Tag>
                  <TaskInlineText className="task-progress-file-name" value={name} tooltip={tooltip} />
                  <TaskInlineText className="task-progress-file-path" value={task.srcPath} tooltip={srcPath} type="secondary" />
                  <TaskInlineText className="task-progress-file-path" value={task.dstPath} tooltip={dstPath} type="secondary" />
                  <Text type="secondary" className="task-progress-file-size">
                    {formatSize(task.fileSize || 0)}
                  </Text>
                  <span className="task-progress-file-state">
                    {activeTab === 1 && (
                      <Progress
                        percent={Math.round(Number(task.progress || 0))}
                        size="small"
                      />
                    )}
                    {activeTab === 7 && task.errMsg && (
                      <Tooltip title={task.errMsg}>
                        <Text type="danger" ellipsis className="task-progress-file-error">失败原因</Text>
                      </Tooltip>
                    )}
                    {activeTab !== 1 && !(activeTab === 7 && task.errMsg) && (
                      <Text type="secondary">
                        {task.createTime ? dayjs.unix(task.createTime).format('HH:mm:ss') : '--'}
                      </Text>
                    )}
                  </span>
                </div>
              );
            })}
          </div>
        </div>
      )}
      {total > 0 && (
        <Pagination
          className="task-progress-pagination"
          current={page}
          pageSize={pageSize}
          total={total}
          onChange={onPageChange}
          showTotal={(count) => `共 ${count} 条`}
          showSizeChanger={false}
          size="small"
        />
      )}
    </Spin>
  );
}

function RealtimeTaskCard({
  activeTab,
  currentTask,
  loading,
  page,
  pageSize,
  rows,
  total,
  visibleRows,
  onStop,
  onPageChange,
  onTabChange,
}: {
  activeTab: number;
  currentTask: CurrentTaskView;
  loading: boolean;
  page: number;
  pageSize: number;
  rows: TaskItem[];
  total: number;
  visibleRows: TaskItem[];
  onStop: () => void;
  onPageChange: (page: number) => void;
  onTabChange: (status: number) => void;
}) {
  const [nowTick, setNowTick] = useState(() => Math.floor(Date.now() / 1000));
  const realtimeTotal = Object.values(currentTask.num || {})
    .reduce((sum, value) => sum + Number(value || 0), 0);
  const scanProgress = currentTask.scan;
  const displayDuration = Math.max(
    currentTask.duration || 0,
    currentTask.createTime ? nowTick - currentTask.createTime : 0,
  );
  const metrics = useMemo<ProgressMetric[]>(() => [
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
  ], [currentTask.doneSize, currentTask.remainSize, currentTask.remainTime, currentTask.speed, currentTask.speedAvg, displayDuration]);

  useEffect(() => {
    const tickID = setInterval(() => {
      if (canPollCurrentDocument()) {
        setNowTick(Math.floor(Date.now() / 1000));
      }
    }, 1000);
    return () => { clearInterval(tickID); };
  }, [currentTask.createTime, currentTask.taskId]);

  // 计算各 tab 标签的计数，确保与分页器使用同步数据源，避免 React 状态更新一帧滞后导致不一致
  const getTabCount = (tabKey: number): number => {
    // 当前激活的 tab：直接使用内容分页器的 total（tabTaskTotal），与分页器完全同步
    if (tabKey === activeTab) {
      return total;
    }
    // "运行中" tab：其内容数据源是 currentTask.doingTask，从这里直接推导
    if (tabKey === 1) {
      return currentTask.doingTask?.length || 0;
    }
    // 其他未激活 tab：使用 currentTask.num 快照作为最佳近似值
    const tab = statusTabs.find((t) => t.key === tabKey);
    return tab ? (currentTask.num?.[tab.numKey] || 0) : 0;
  };

  return (
    <Card
      className="task-progress-card"
      size="small"
      title="实时进度"
      extra={(
        <Space size={8} wrap className="task-progress-card-extra">
          <Tag color={currentTask.scanFinish ? 'success' : 'processing'}>
            {currentTask.scanFinish ? '扫描完成，同步中' : '进行中'}
          </Tag>
        </Space>
      )}
      style={{ marginBottom: 12 }}
    >
      <div className="task-progress-hero">
        <div>
          <div className="task-progress-hero-title">
            <ThunderboltOutlined />
            任务 #{currentTask.taskId}
          </div>
          <div className="task-progress-hero-meta">
            <span>开始: {currentTask.createTime ? dayjs.unix(currentTask.createTime).format('HH:mm:ss') : '--'}</span>
            <span>明细: {realtimeTotal} 条</span>
            <span>已完成: {currentTask.num?.success || 0} 条</span>
            <span>失败: {currentTask.num?.fail || 0} 条</span>
          </div>
        </div>
        <Button icon={<StopOutlined />} onClick={onStop}>
          停止
        </Button>
      </div>

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
        {metrics.map((item) => (
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
        onChange={(key) => onTabChange(Number(key))}
        items={statusTabs.map((tab) => ({
          key: String(tab.key),
          label: `${tab.label} (${getTabCount(tab.key)})`,
          children: tab.key === activeTab ? (
            <RealtimeTaskItems
              activeTab={activeTab}
              loading={loading}
              page={page}
              pageSize={pageSize}
              rows={rows}
              total={total}
              visibleRows={visibleRows}
              onPageChange={onPageChange}
            />
          ) : null,
        }))}
      />
    </Card>
  );
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
  const [historyError, setHistoryError] = useState(false);
  const [queryJobId, setQueryJobId] = useState(jobId);
  const [historyStatusFilter, setHistoryStatusFilter] = useState<number | undefined>(undefined);
  const [historyKeywordInput, setHistoryKeywordInput] = useState('');
  const [historyKeywordFilter, setHistoryKeywordFilter] = useState('');
  const [historyTimeRange, setHistoryTimeRange] = useState<HistoryTimeRange>(null);
  const listRequestRef = useRef(0);
  const listLoadingRequestRef = useRef(0);
  const showRealtime = shouldPollRealtime(view);
  const showHistory = view === 'all' || view === 'history';
  const { currentTask, refreshCurrentTask } = useRealtimeTask(jobId, showRealtime);
  const {
    activeTab,
    setActiveTab,
    tabTaskList,
    pagedTabTaskList,
    tabTaskTotal,
    tabTaskPage,
    setTabTaskPage,
    tabLoading,
  } = useRealtimeTaskItems({
    jobId,
    enabled: showRealtime,
    currentTask,
    pageSize: TAB_TASK_PAGE_SIZE,
  });

  const fetchList = useCallback(async (showLoading = false) => {
    if (!jobId || queryJobId !== jobId) return;
    const requestID = ++listRequestRef.current;
    const loadingRequestID = showLoading ? ++listLoadingRequestRef.current : 0;
    if (showLoading) {
      setLoading(true);
      setHistoryError(false);
    }
    try {
      const params: Record<string, unknown> = { id: jobId, pageSize, pageNum: page };
      if (historyStatusFilter !== undefined) {
        params.status = historyStatusFilter;
      } else {
        params.statusIn = historyCompletedStatuses;
      }
      if (historyKeywordFilter.trim()) params.keyword = historyKeywordFilter.trim();
      if (historyTimeRange?.[0]) params.startTime = historyTimeRange[0].startOf('day').unix();
      if (historyTimeRange?.[1]) params.endTime = historyTimeRange[1].endOf('day').unix();

      const res = await jobGetTask(params, { silent: !showLoading });
      if (requestID === listRequestRef.current) {
        setList((previous) => mergeTaskRecords(previous, res.data?.dataList || []));
        setTotal(res.data?.count || 0);
        setHistoryError(false);
      }
    } catch (err) {
      if (requestID === listRequestRef.current) {
        console.error('task history fetch failed', err);
        if (showLoading) {
          setHistoryError(true);
          setList([]);
          setTotal(0);
        }
      }
    } finally {
      if (showLoading && loadingRequestID === listLoadingRequestRef.current) {
        setLoading(false);
      }
    }
  }, [historyKeywordFilter, historyStatusFilter, historyTimeRange, jobId, page, pageSize, queryJobId]);

  useEffect(() => {
    setQueryJobId(jobId);
    setPage(1);
    setPageSize(20);
    setList([]);
    setTotal(0);
    setHistoryError(false);
    setHistoryStatusFilter(undefined);
    setHistoryKeywordInput('');
    setHistoryKeywordFilter('');
    setHistoryTimeRange(null);
  }, [jobId]);

  useEffect(() => {
    if (showHistory) fetchList(true);
  }, [fetchList, showHistory]);
  useEffect(() => {
    if (!showHistory) return undefined;
    const pollID = setInterval(() => {
      if (canPollCurrentDocument()) fetchList(false);
    }, 3000);
    return () => { clearInterval(pollID); };
  }, [fetchList, showHistory]);
  const handleDeleteTask = useCallback(async (taskId: number) => {
    try {
      await jobDeleteTask(taskId);
      message.success('删除成功');
      fetchList(false);
    } catch (err) {
      console.error('task delete failed', err);
    }
  }, [fetchList, message]);

  const handleTaskAction = useCallback(async (
    taskId: number,
    action: 'stop' | 'retry',
    successText: string,
  ) => {
    try {
      await jobTaskAction(taskId, action);
      message.success(successText);
      fetchList(false);
      refreshCurrentTask();
    } catch (err) {
      console.error('task action failed', err);
    }
  }, [fetchList, message, refreshCurrentTask]);

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

  const columns = useMemo(() => [
    {
      title: '状态', dataIndex: 'status', key: 'status', width: 120,
      render: (s: number, record: TaskRecord) => {
        const errorReason = typeof record.errMsg === 'string' ? record.errMsg.trim() : '';
        const statusTag = (
          <Tag color={taskStatusColors[s]}>{statusNames[s] || s}</Tag>
        );
        if (taskStatusColors[s] !== 'error' || !errorReason) {
          return statusTag;
        }
        return (
          <span className="task-status-with-error">
            {statusTag}
            <Tooltip title={record.errMsg}>
              <InfoCircleOutlined className="task-status-error-tip" aria-label="查看错误原因" />
            </Tooltip>
          </span>
        );
      },
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
          {(record.allNum || 0) > (record.successNum || 0) && (
            <Tooltip title="重试未完成项">
              <Button
                size="small"
                type="text"
                icon={<RedoOutlined />}
                aria-label="重试未完成项"
                onClick={() => handleTaskAction(record.id, 'retry', '已提交重试')}
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
  ], [handleDeleteTask, handleTaskAction, onTaskDetail]);

  const historyList = useMemo(
    () => showRealtime ? filterCurrentTaskFromHistory(list, currentTask) : filterRunningTaskRows(list),
    [currentTask, list, showRealtime],
  );
  const hiddenCurrentTaskCount = list.length - historyList.length;
  const historyTotal = Math.max(0, total - hiddenCurrentTaskCount);

  const realtimeContent = currentTask ? (
    <RealtimeTaskCard
      activeTab={activeTab}
      currentTask={currentTask}
      loading={tabLoading}
      page={tabTaskPage}
      pageSize={TAB_TASK_PAGE_SIZE}
      rows={tabTaskList}
      total={tabTaskTotal}
      visibleRows={pagedTabTaskList}
      onStop={() => handleTaskAction(currentTask.taskId, 'stop', '已停止')}
      onPageChange={setTabTaskPage}
      onTabChange={setActiveTab}
    />
  ) : (
    <Empty
      image={Empty.PRESENTED_IMAGE_SIMPLE}
      description={<Text type="secondary">当前没有正在同步的任务</Text>}
    />
  );

  const historyBody = historyError ? (
    <div style={{ textAlign: 'center', padding: '32px 0' }}>
      <Empty
        image={Empty.PRESENTED_IMAGE_SIMPLE}
        description={<Text type="secondary">历史任务加载失败</Text>}
      />
      <Button icon={<ReloadOutlined />} style={{ marginTop: 16 }} onClick={() => fetchList(true)} loading={loading}>
        重试
      </Button>
    </div>
  ) : historyList.length === 0 && !loading ? (
    <Empty
      image={Empty.PRESENTED_IMAGE_SIMPLE}
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
    <div className="task-history-panel">
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
    </div>
  );

  return (
    <div>
      {showRealtime && realtimeContent}
      {showHistory && historyContent}
    </div>
  );
}
