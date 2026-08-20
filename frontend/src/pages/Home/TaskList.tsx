import { useState, useEffect, useRef, useCallback, useMemo, memo, lazy, Suspense, type ReactNode } from 'react';
import App from 'antd/es/app';
import Button from 'antd/es/button';
import Card from 'antd/es/card';
import Empty from 'antd/es/empty';
import Pagination from 'antd/es/pagination';
import Space from 'antd/es/space';
import Spin from 'antd/es/spin';
import Tabs from 'antd/es/tabs';
import Tag from 'antd/es/tag';
import Tooltip from 'antd/es/tooltip';
import Typography from 'antd/es/typography';
import { POLL_INTERVAL_MS } from '../../api/request';
import EllipsisText from './components/EllipsisText';
import {
  StopOutlined, ThunderboltOutlined, ClockCircleOutlined, DashboardOutlined, FolderOpenOutlined,
} from '@ant-design/icons';
import { jobGetTask, jobDeleteTask, jobTaskAction } from '../../api/job';
import type { Dayjs } from 'dayjs';
import type { CurrentTaskView, TaskItem, TaskRecord } from '../../types';
import {
  filterCurrentTaskFromHistory,
  filterRunningTaskRows,
  getTaskItemKey,
  historyHasActiveTask,
  mergeTaskRecords,
  pollIntervalForActiveWork,
  type TaskListView,
} from './taskRows';
import { useRealtimeTask } from './useRealtimeTask';
import { useRealtimeTaskItems } from './useRealtimeTaskItems';
import { canPollCurrentDocument } from './pollingVisibility';
import { displayText, formatSize, taskTypeNames } from './homeUtils';

const TaskHistoryPanel = lazy(() => import('./TaskHistoryPanel'));

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

function formatClock(unix?: number): string {
  if (!unix) return '--';
  const date = new Date(unix * 1000);
  const hh = String(date.getHours()).padStart(2, '0');
  const mm = String(date.getMinutes()).padStart(2, '0');
  const ss = String(date.getSeconds()).padStart(2, '0');
  return `${hh}:${mm}:${ss}`;
}

const TAB_TASK_PAGE_SIZE = 20;

const LiveDuration = memo(function LiveDuration({
  createTime,
  duration,
}: {
  createTime?: number;
  duration: number;
}) {
  const [nowTick, setNowTick] = useState(() => Math.floor(Date.now() / 1000));

  useEffect(() => {
    const tickID = setInterval(() => {
      if (canPollCurrentDocument()) {
        setNowTick(Math.floor(Date.now() / 1000));
      }
    }, 1000);
    return () => { clearInterval(tickID); };
  }, [createTime]);

  const displayDuration = Math.max(
    duration || 0,
    createTime ? nowTick - createTime : 0,
  );
  return <>{formatDuration(displayDuration)}</>;
});

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
  value: ReactNode;
};

function getTaskDisplayName(task: TaskItem): string {
  if (task.fileName) return task.fileName;
  const path = task.dstPath || task.srcPath || '';
  if (!path) return '--';
  const cleanPath = path.replace(/\/+$/, '');
  return cleanPath.split('/').pop() || cleanPath;
}

const RealtimeTaskItems = memo(function RealtimeTaskItems({
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
                  <EllipsisText className="task-progress-file-name" value={name} tooltip={tooltip} />
                  <EllipsisText className="task-progress-file-path" value={task.srcPath} tooltip={srcPath} type="secondary" />
                  <EllipsisText className="task-progress-file-path" value={task.dstPath} tooltip={dstPath} type="secondary" />
                  <Text type="secondary" className="task-progress-file-size">
                    {formatSize(task.fileSize || 0)}
                  </Text>
                  <span className="task-progress-file-state">
                    {activeTab === 1 && (
                      <span
                        className="task-progress-meter"
                        role="progressbar"
                        aria-valuemin={0}
                        aria-valuemax={100}
                        aria-valuenow={Math.round(Number(task.progress || 0))}
                      >
                        <span
                          className="task-progress-meter-bar"
                          style={{ transform: `scaleX(${Math.min(1, Math.max(0, Number(task.progress || 0) / 100))})` }}
                        />
                      </span>
                    )}
                    {activeTab === 7 && task.errMsg && (
                      <Tooltip title={task.errMsg}>
                        <Text type="danger" ellipsis className="task-progress-file-error">失败原因</Text>
                      </Tooltip>
                    )}
                    {activeTab !== 1 && !(activeTab === 7 && task.errMsg) && (
                      <Text type="secondary">
                        {formatClock(task.createTime)}
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
});

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
  const realtimeTotal = Object.values(currentTask.num || {})
    .reduce((sum, value) => sum + Number(value || 0), 0);
  const scanProgress = currentTask.scan;
  const metrics = useMemo<ProgressMetric[]>(() => [
    {
      key: 'duration',
      label: '耗时',
      icon: <ClockCircleOutlined />,
      value: <LiveDuration createTime={currentTask.createTime} duration={currentTask.duration} />,
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
  ], [currentTask.createTime, currentTask.doneSize, currentTask.duration, currentTask.remainSize, currentTask.remainTime, currentTask.speed, currentTask.speedAvg]);

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
    >
      <div className="task-progress-hero">
        <div>
          <div className="task-progress-hero-title">
            <ThunderboltOutlined />
            任务 #{currentTask.taskId}
          </div>
          <div className="task-progress-hero-meta">
            <span>开始: {formatClock(currentTask.createTime)}</span>
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
  view,
  active = true,
}: {
  jobId: string;
  onTaskDetail?: (taskId: number) => void;
  view: TaskListView;
  active?: boolean;
}) {
  const { message } = App.useApp();
  const [list, setList] = useState<TaskRecord[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [loading, setLoading] = useState(false);
  const [historyError, setHistoryError] = useState(false);
  const [historyStatusFilter, setHistoryStatusFilter] = useState<number | undefined>(undefined);
  const [historyKeywordInput, setHistoryKeywordInput] = useState('');
  const [historyKeywordFilter, setHistoryKeywordFilter] = useState('');
  const [historyTimeRange, setHistoryTimeRange] = useState<HistoryTimeRange>(null);
  const listRequestRef = useRef(0);
  const listLoadingRequestRef = useRef(0);
  const listAbortRef = useRef<AbortController | null>(null);
  const listFetchingRef = useRef(false);
  const showRealtime = view === 'realtime' && active;
  const showHistory = view === 'history' && active;
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
    if (!jobId) return;
    // Keep the view responsive when filters/page changes race a slow request.
    // Abort the stale browser request and let the latest request own the state;
    // the request ID guard below prevents the old response from winning.
    if (listFetchingRef.current) listAbortRef.current?.abort();
    listFetchingRef.current = true;
    const controller = new AbortController();
    listAbortRef.current = controller;
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

      const res = await jobGetTask(params, { silent: !showLoading, signal: controller.signal });
      if (requestID === listRequestRef.current) {
        setList((previous) => mergeTaskRecords(previous, res.data?.dataList || []));
        setTotal(res.data?.count || 0);
        setHistoryError(false);
      }
    } catch (err) {
      if (controller.signal.aborted) return; // cancelled, not a real failure
      if (requestID === listRequestRef.current) {
        console.error('task history fetch failed', err);
        if (showLoading) {
          setHistoryError(true);
          setList([]);
          setTotal(0);
        }
      }
    } finally {
      if (listAbortRef.current === controller) {
        listFetchingRef.current = false;
        listAbortRef.current = null;
      }
      if (showLoading && loadingRequestID === listLoadingRequestRef.current) {
        setLoading(false);
      }
    }
  }, [historyKeywordFilter, historyStatusFilter, historyTimeRange, jobId, page, pageSize]);

  // Cancel any in-flight history request when the component unmounts. TaskList
  // is remounted via `key` when the selected job changes, so all local state
  // (including the history filters above) is already reset by React.
  useEffect(() => () => { listAbortRef.current?.abort(); }, []);

  useEffect(() => {
    if (showHistory) fetchList(true);
  }, [fetchList, showHistory]);
  useEffect(() => {
    if (!showHistory) return undefined;
    const pollID = setInterval(() => {
      if (canPollCurrentDocument()) fetchList(false);
    }, pollIntervalForActiveWork(historyHasActiveTask(list), POLL_INTERVAL_MS));
    return () => { clearInterval(pollID); };
  }, [fetchList, list, showHistory]);
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

  const historyContent = (
    <Suspense fallback={<div className="task-history-panel task-history-panel-pending" />}>
      <TaskHistoryPanel
        error={historyError}
        loading={loading}
        rows={historyList}
        total={historyTotal}
        page={page}
        pageSize={pageSize}
        keywordInput={historyKeywordInput}
        statusFilter={historyStatusFilter}
        timeRange={historyTimeRange}
        hasFilters={hasHistoryFilters}
        onKeywordInputChange={setHistoryKeywordInput}
        onKeywordSearch={handleHistoryKeywordSearch}
        onStatusFilterChange={(value) => {
          setHistoryStatusFilter(value);
          setPage(1);
        }}
        onTimeRangeChange={(value) => {
          setHistoryTimeRange(value);
          setPage(1);
        }}
        onPageChange={(nextPage, nextSize) => {
          setPage(nextPage);
          setPageSize(nextSize);
        }}
        onResetFilters={resetHistoryFilters}
        onRetry={() => fetchList(true)}
        onTaskDetail={onTaskDetail}
        onRetryTask={(taskId) => { void handleTaskAction(taskId, 'retry', '已提交重试'); }}
        onDeleteTask={(taskId) => { void handleDeleteTask(taskId); }}
      />
    </Suspense>
  );

  return (
    <div>
      {showRealtime && realtimeContent}
      {showHistory && historyContent}
    </div>
  );
}
