import { useState, useEffect, useRef, useCallback, useMemo } from 'react';
import { POLL_INTERVAL_MS } from '../../api/request';
import {
  Eye as EyeOutlined,
  RotateCcw as RedoOutlined,
  Trash2 as DeleteOutlined,
  AlertCircle as InfoCircleOutlined,
  Inbox,
} from 'lucide-react';
import { jobGetTask, jobDeleteTask, jobTaskAction } from '../../api/job';
import dayjs from 'dayjs';
import type { Dayjs } from 'dayjs';
import type { CurrentTaskView, TaskItem, TaskRecord } from '../../types';
import {
  filterCurrentTaskFromHistory,
  filterRunningTaskRows,
  mergeTaskRecords,
  type TaskListView,
} from './taskRows';
import { useRealtimeTask } from './useRealtimeTask';
import { useRealtimeTaskItems } from './useRealtimeTaskItems';
import { canPollCurrentDocument } from './pollingVisibility';
import { taskRecordStatusNames, taskStatusColors } from './homeUtils';
import TaskRealtimeHero from './components/TaskRealtimeHero';
import TaskRealtimeRows from './components/TaskRealtimeRows';
import { Button } from '../../components/ui/button';
import { Input } from '../../components/ui/input';
import { Tooltip } from '../../components/ui/tooltip';
import { EmptyState, ErrorState, PlaceholderCard } from '../../components/common/StatePlaceholder';
import { Pagination } from '../../components/common/Pagination';
import { SearchInput } from '../../components/common/SearchInput';
import { StatusBadge } from '../../components/common/StatusBadge';
import { control, icon, layout, surface, table } from '../../lib/styles';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../../components/ui/select';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '../../components/ui/alert-dialog';
import { toast } from '../../components/ui/toaster';
import { cn } from '../../lib/utils';

const TAB_TASK_PAGE_SIZE = 20;

const historyStatusOptions = [2, 3, 4, 5, 6, 7, 8].map((value) => ({
  value,
  label: taskRecordStatusNames[value],
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
  const getTabCount = (tabKey: number): number => {
    if (tabKey === activeTab) {
      return total;
    }
    if (tabKey === 1) {
      return currentTask.doingTask?.length || 0;
    }
    const tab = statusTabs.find((t) => t.key === tabKey);
    return tab ? (currentTask.num?.[tab.numKey] || 0) : 0;
  };

  const activeTabLabel = statusTabs.find((tab) => tab.key === activeTab)?.label || '';

  return (
    <div>
      <TaskRealtimeHero currentTask={currentTask} onStop={onStop}>
        <div className="space-y-3 pt-2">
          <div className="flex border-b border-slate-200 gap-2 overflow-x-auto">
            {statusTabs.map((tab) => {
              const isActive = tab.key === activeTab;
              return (
                <button
                  key={tab.key}
                  type="button"
                  onClick={() => onTabChange(tab.key)}
                  className={cn(
                    'pb-2 px-3 text-xs font-medium border-b-2 transition-all flex items-center gap-1.5 shrink-0',
                    isActive
                      ? 'border-teal-700 text-teal-800 font-semibold'
                      : 'border-transparent text-slate-500 hover:text-slate-800'
                  )}
                >
                  <span>{tab.label}</span>
                  <span
                    className={cn(
                      'px-1.5 py-0.5 rounded-full text-2xs',
                      isActive ? 'bg-teal-100 text-teal-800 font-bold' : 'bg-slate-100 text-slate-600'
                    )}
                  >
                    {getTabCount(tab.key)}
                  </span>
                </button>
              );
            })}
          </div>

          <TaskRealtimeRows
            activeTab={activeTab}
            activeTabLabel={activeTabLabel}
            loading={loading}
            page={page}
            pageSize={pageSize}
            rows={rows}
            total={total}
            visibleRows={visibleRows}
            onPageChange={onPageChange}
          />
        </div>
      </TaskRealtimeHero>
    </div>
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
  const [deleteConfirmTaskId, setDeleteConfirmTaskId] = useState<number | null>(null);

  const listRequestRef = useRef(0);
  const listLoadingRequestRef = useRef(0);
  const listAbortRef = useRef<AbortController | null>(null);
  const listFetchingRef = useRef(false);
  const inFlightParamsRef = useRef<string | null>(null);
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
    const paramsKey = JSON.stringify([
      jobId, pageSize, page, historyStatusFilter, historyKeywordFilter,
      historyTimeRange?.[0]?.valueOf(), historyTimeRange?.[1]?.valueOf(),
    ]);
    if (listFetchingRef.current && inFlightParamsRef.current === paramsKey) return;
    listAbortRef.current?.abort();
    listFetchingRef.current = true;
    inFlightParamsRef.current = paramsKey;
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
      if (controller.signal.aborted) return;
      if (requestID === listRequestRef.current) {
        console.error('task history fetch failed', err);
        if (showLoading) {
          setHistoryError(true);
          setList([]);
          setTotal(0);
        }
      }
    } finally {
      listFetchingRef.current = false;
      inFlightParamsRef.current = null;
      if (showLoading && loadingRequestID === listLoadingRequestRef.current) {
        setLoading(false);
      }
    }
  }, [historyKeywordFilter, historyStatusFilter, historyTimeRange, jobId, page, pageSize]);

  useEffect(() => () => { listAbortRef.current?.abort(); }, []);

  useEffect(() => {
    if (showHistory) fetchList(true);
  }, [fetchList, showHistory]);

  useEffect(() => {
    if (!showHistory) return undefined;
    const pollID = setInterval(() => {
      if (canPollCurrentDocument()) fetchList(false);
    }, POLL_INTERVAL_MS);
    return () => { clearInterval(pollID); };
  }, [fetchList, showHistory]);

  const handleDeleteTask = useCallback(async (taskId: number) => {
    try {
      await jobDeleteTask(taskId);
      toast.success('删除成功');
      setDeleteConfirmTaskId(null);
      fetchList(false);
    } catch (err) {
      console.error('task delete failed', err);
    }
  }, [fetchList]);

  const handleTaskAction = useCallback(async (
    taskId: number,
    action: 'stop' | 'retry',
    successText: string,
  ) => {
    try {
      await jobTaskAction(taskId, action);
      toast.success(successText);
      fetchList(false);
      refreshCurrentTask();
    } catch (err) {
      console.error('task action failed', err);
    }
  }, [fetchList, refreshCurrentTask]);

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
    <PlaceholderCard>
      <EmptyState icon={Inbox} title="当前没有正在同步的任务" />
    </PlaceholderCard>
  );

  const historyBody = historyError ? (
    <ErrorState
      icon={InfoCircleOutlined}
      title="历史任务加载失败"
      onRetry={() => fetchList(true)}
      loading={loading}
      retryLabel="重试"
    />
  ) : historyList.length === 0 && !loading ? (
    <EmptyState icon={Inbox} title="暂无历史任务记录，执行完成后将在此显示" />
  ) : (
    <div className="space-y-3">
      <div className={table.wrapper}>
        <table className={table.root}>
          <thead className={table.head}>
            <tr>
              <th className={cn(table.headCell, 'w-28')}>状态</th>
              <th className={cn(table.headCell, 'min-w-[170px]')}>开始时间</th>
              <th className={cn(table.headCell, 'w-20')}>成功</th>
              <th className={cn(table.headCell, 'w-20')}>失败</th>
              <th className={cn(table.headCell, 'w-20')}>总计</th>
              <th className={cn(table.headCell, 'w-32 text-right pr-4')}>操作</th>
            </tr>
          </thead>
          <tbody className={table.body}>
            {historyList.map((record) => {
              const s = record.status;
              const errorReason = typeof record.errMsg === 'string' ? record.errMsg.trim() : '';
              return (
                <tr key={record.id} className={table.row}>
                  <td className={table.cell}>
                    <StatusBadge
                      label={taskRecordStatusNames[s] || String(s)}
                      color={taskStatusColors[s]}
                      errMsg={
                        taskStatusColors[s] === 'error' && errorReason ? record.errMsg : undefined
                      }
                    />
                  </td>
                  <td className={cn(table.cell, 'text-slate-600 font-mono')}>
                    {record.runTime ? dayjs.unix(record.runTime).format('YYYY-MM-DD HH:mm:ss') : '-'}
                  </td>
                  <td className={cn(table.cell, 'text-emerald-600 font-semibold font-mono')}>
                    {record.successNum ?? '-'}
                  </td>
                  <td className={cn(table.cell, 'text-rose-600 font-semibold font-mono')}>
                    {record.failNum ?? '-'}
                  </td>
                  <td className={cn(table.cell, 'text-slate-700 font-mono')}>
                    {record.allNum ?? '-'}
                  </td>
                  <td className={cn(table.cell, 'text-right pr-4')}>
                    <div className="flex items-center justify-end gap-1">
                      <Tooltip title="详情">
                        <Button
                          variant="ghost"
                          size="iconSm"
                          className="text-slate-500 hover:text-slate-900"
                          aria-label="详情"
                          onClick={() => onTaskDetail?.(record.id)}
                        >
                          <EyeOutlined className={icon.sm} />
                        </Button>
                      </Tooltip>

                      {(record.allNum || 0) > (record.successNum || 0) && (
                        <Tooltip title="重试未完成项">
                          <Button
                            variant="ghost"
                            size="iconSm"
                            className="text-teal-600 hover:text-teal-800"
                            aria-label="重试未完成项"
                            onClick={() => handleTaskAction(record.id, 'retry', '已提交重试')}
                          >
                            <RedoOutlined className={icon.sm} />
                          </Button>
                        </Tooltip>
                      )}

                      <Tooltip title="删除">
                        <Button
                          variant="ghost"
                          size="iconSm"
                          className="text-rose-500 hover:text-rose-700 hover:bg-rose-50"
                          aria-label="删除"
                          onClick={() => setDeleteConfirmTaskId(record.id)}
                        >
                          <DeleteOutlined className={icon.sm} />
                        </Button>
                      </Tooltip>
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      {/* 分页控制 */}
      <Pagination
        page={page}
        pageSize={pageSize}
        total={historyTotal}
        onPageChange={(next) => setPage(next)}
        onPageSizeChange={(size) => {
          setPageSize(size);
          setPage(1);
        }}
      />
    </div>
  );

  const historyContent = (
    <div className={cn(surface.card, surface.cardPadding, 'space-y-4')}>
      {/* 筛选栏 */}
      <div className={layout.filterBar}>
        <SearchInput
          value={historyKeywordInput}
          placeholder="任务 ID"
          width="w-40"
          onChange={(value) => {
            setHistoryKeywordInput(value);
            if (!value) handleHistoryKeywordSearch('');
          }}
          onSearch={() => handleHistoryKeywordSearch(historyKeywordInput)}
          onClear={() => handleHistoryKeywordSearch('')}
        />

        <Select
          value={historyStatusFilter !== undefined ? String(historyStatusFilter) : 'ALL'}
          onValueChange={(val) => {
            setHistoryStatusFilter(val === 'ALL' ? undefined : Number(val));
            setPage(1);
          }}
        >
          <SelectTrigger className={cn(control.compact, 'w-28')}>
            <SelectValue placeholder="任务状态" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="ALL">全部状态</SelectItem>
            {historyStatusOptions.map((opt) => (
              <SelectItem key={opt.value} value={String(opt.value)}>
                {opt.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <div className="flex items-center gap-1.5 text-xs">
          <Input
            type="date"
            aria-label="开始日期"
            value={historyTimeRange?.[0] ? historyTimeRange[0].format('YYYY-MM-DD') : ''}
            onChange={(e) => {
              const start = e.target.value ? dayjs(e.target.value) : null;
              setHistoryTimeRange([start, historyTimeRange?.[1] || null]);
              setPage(1);
            }}
            className={cn(control.compact, 'w-auto px-2')}
          />
          <span className="text-slate-400">至</span>
          <Input
            type="date"
            aria-label="结束日期"
            value={historyTimeRange?.[1] ? historyTimeRange[1].format('YYYY-MM-DD') : ''}
            onChange={(e) => {
              const end = e.target.value ? dayjs(e.target.value) : null;
              setHistoryTimeRange([historyTimeRange?.[0] || null, end]);
              setPage(1);
            }}
            className={cn(control.compact, 'w-auto px-2')}
          />
        </div>

        <Button
          variant="ghost"
          size="sm"
          onClick={resetHistoryFilters}
          disabled={!hasHistoryFilters}
          className="text-slate-500"
        >
          重置
        </Button>
      </div>

      {historyBody}

      {/* 删除确认弹窗 */}
      <AlertDialog open={deleteConfirmTaskId !== null} onOpenChange={(open) => !open && setDeleteConfirmTaskId(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>确认删除任务</AlertDialogTitle>
            <AlertDialogDescription>
              确定要删除任务 #{deleteConfirmTaskId} 的历史记录吗？此操作无法撤销。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={() => deleteConfirmTaskId && handleDeleteTask(deleteConfirmTaskId)}
            >
              删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );

  return (
    <div>
      {showRealtime && realtimeContent}
      {showHistory && historyContent}
    </div>
  );
}

