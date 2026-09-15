import { useState, useEffect, useRef, useCallback, useMemo } from 'react';
import { POLL_INTERVAL_MS } from '../../api/request';
import {
  Eye as EyeOutlined,
  RotateCcw as RedoOutlined,
  Trash2 as DeleteOutlined,
  RefreshCw as ReloadOutlined,
  AlertCircle as InfoCircleOutlined,
  Search,
  X,
  ChevronLeft,
  ChevronRight,
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
import { Badge } from '../../components/ui/badge';
import { Tooltip } from '../../components/ui/tooltip';
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
                      'px-1.5 py-0.5 rounded-full text-[10px]',
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
  const totalPages = Math.max(1, Math.ceil(historyTotal / pageSize));

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
    <div className="py-16 text-center space-y-2 bg-white rounded-xl border border-slate-200/80">
      <Inbox className="h-10 w-10 text-slate-300 mx-auto" />
      <p className="text-sm text-slate-400">当前没有正在同步的任务</p>
    </div>
  );

  const historyBody = historyError ? (
    <div className="py-16 text-center space-y-3">
      <div className="inline-flex h-12 w-12 items-center justify-center rounded-full bg-rose-50 text-rose-600">
        <InfoCircleOutlined className="h-6 w-6" />
      </div>
      <p className="text-sm font-medium text-slate-600">历史任务加载失败</p>
      <Button variant="outline" size="sm" onClick={() => fetchList(true)} disabled={loading} className="gap-1.5">
        <ReloadOutlined className={cn('h-3.5 w-3.5', loading && 'animate-spin')} />
        <span>重试</span>
      </Button>
    </div>
  ) : historyList.length === 0 && !loading ? (
    <div className="py-16 text-center space-y-2">
      <Inbox className="h-10 w-10 text-slate-300 mx-auto" />
      <p className="text-sm text-slate-400">暂无历史任务记录，执行完成后将在此显示</p>
    </div>
  ) : (
    <div className="space-y-3">
      <div className="overflow-x-auto border border-slate-200 rounded-lg">
        <table className="w-full text-left text-xs divide-y divide-slate-200">
          <thead className="bg-slate-50 text-slate-600 uppercase font-medium tracking-wider">
            <tr>
              <th className="px-3 py-2.5 w-28">状态</th>
              <th className="px-3 py-2.5 min-w-[170px]">开始时间</th>
              <th className="px-3 py-2.5 w-20">成功</th>
              <th className="px-3 py-2.5 w-20">失败</th>
              <th className="px-3 py-2.5 w-20">总计</th>
              <th className="px-3 py-2.5 w-32 text-right pr-4">操作</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100 bg-white">
            {historyList.map((record) => {
              const s = record.status;
              const errorReason = typeof record.errMsg === 'string' ? record.errMsg.trim() : '';
              const statusTag = (
                <Badge
                  variant={
                    s === 2
                      ? 'success'
                      : s === 6 || s === 7 || s === 8
                      ? 'error'
                      : s === 1
                      ? 'processing'
                      : 'secondary'
                  }
                >
                  {taskRecordStatusNames[s] || s}
                </Badge>
              );

              return (
                <tr key={record.id} className="hover:bg-slate-50/70 transition-colors">
                  <td className="px-3 py-2.5">
                    {taskStatusColors[s] !== 'error' || !errorReason ? (
                      statusTag
                    ) : (
                      <span className="inline-flex items-center gap-1 max-w-full">
                        {statusTag}
                        <Tooltip title={record.errMsg}>
                          <InfoCircleOutlined
                            className="h-4 w-4 text-rose-500 hover:text-rose-700 cursor-pointer shrink-0"
                            aria-label="查看错误原因"
                          />
                        </Tooltip>
                      </span>
                    )}
                  </td>
                  <td className="px-3 py-2.5 text-slate-600 font-mono">
                    {record.runTime ? dayjs.unix(record.runTime).format('YYYY-MM-DD HH:mm:ss') : '-'}
                  </td>
                  <td className="px-3 py-2.5 text-emerald-600 font-semibold font-mono">
                    {record.successNum ?? '-'}
                  </td>
                  <td className="px-3 py-2.5 text-rose-600 font-semibold font-mono">
                    {record.failNum ?? '-'}
                  </td>
                  <td className="px-3 py-2.5 text-slate-700 font-mono">
                    {record.allNum ?? '-'}
                  </td>
                  <td className="px-3 py-2.5 text-right pr-4">
                    <div className="flex items-center justify-end gap-1">
                      <Tooltip title="详情">
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-7 w-7 p-0 text-slate-500 hover:text-slate-900"
                          aria-label="详情"
                          onClick={() => onTaskDetail?.(record.id)}
                        >
                          <EyeOutlined className="h-3.5 w-3.5" />
                        </Button>
                      </Tooltip>

                      {(record.allNum || 0) > (record.successNum || 0) && (
                        <Tooltip title="重试未完成项">
                          <Button
                            variant="ghost"
                            size="sm"
                            className="h-7 w-7 p-0 text-teal-600 hover:text-teal-800"
                            aria-label="重试未完成项"
                            onClick={() => handleTaskAction(record.id, 'retry', '已提交重试')}
                          >
                            <RedoOutlined className="h-3.5 w-3.5" />
                          </Button>
                        </Tooltip>
                      )}

                      <Tooltip title="删除">
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-7 w-7 p-0 text-rose-500 hover:text-rose-700 hover:bg-rose-50"
                          aria-label="删除"
                          onClick={() => setDeleteConfirmTaskId(record.id)}
                        >
                          <DeleteOutlined className="h-3.5 w-3.5" />
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
      <div className="flex flex-col sm:flex-row items-center justify-between gap-3 pt-2 text-xs text-slate-500">
        <div>
          共 <span className="font-semibold text-slate-700">{historyTotal}</span> 条
        </div>
        <div className="flex items-center gap-2">
          <Select
            value={String(pageSize)}
            onValueChange={(v) => {
              setPageSize(Number(v));
              setPage(1);
            }}
          >
            <SelectTrigger className="h-7 w-24 text-xs">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="10">10 条/页</SelectItem>
              <SelectItem value="20">20 条/页</SelectItem>
              <SelectItem value="50">50 条/页</SelectItem>
              <SelectItem value="100">100 条/页</SelectItem>
            </SelectContent>
          </Select>

          <div className="flex items-center gap-1">
            <Button
              variant="outline"
              size="sm"
              disabled={page <= 1 || loading}
              onClick={() => setPage((p) => Math.max(1, p - 1))}
              className="h-7 w-7 p-0"
            >
              <ChevronLeft className="h-3.5 w-3.5" />
            </Button>
            <span className="px-2 text-xs">
              {page} / {totalPages}
            </span>
            <Button
              variant="outline"
              size="sm"
              disabled={page >= totalPages || loading}
              onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
              className="h-7 w-7 p-0"
            >
              <ChevronRight className="h-3.5 w-3.5" />
            </Button>
          </div>
        </div>
      </div>
    </div>
  );

  const historyContent = (
    <div className="bg-white rounded-xl border border-slate-200/80 shadow-xs p-4 sm:p-5 space-y-4">
      {/* 筛选栏 */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="relative w-40">
          <Input
            placeholder="任务 ID"
            value={historyKeywordInput}
            onChange={(event) => {
              setHistoryKeywordInput(event.target.value);
              if (!event.target.value) handleHistoryKeywordSearch('');
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter') handleHistoryKeywordSearch(historyKeywordInput);
            }}
            className="h-8 text-xs pr-7"
            prefixIcon={<Search className="h-3.5 w-3.5 text-slate-400" />}
          />
          {historyKeywordInput && (
            <button
              type="button"
              onClick={() => {
                setHistoryKeywordInput('');
                handleHistoryKeywordSearch('');
              }}
              className="absolute right-2 top-2 text-slate-400 hover:text-slate-600"
            >
              <X className="h-3.5 w-3.5" />
            </button>
          )}
        </div>

        <Select
          value={historyStatusFilter !== undefined ? String(historyStatusFilter) : 'ALL'}
          onValueChange={(val) => {
            setHistoryStatusFilter(val === 'ALL' ? undefined : Number(val));
            setPage(1);
          }}
        >
          <SelectTrigger className="h-8 w-28 text-xs">
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
          <input
            type="date"
            aria-label="开始日期"
            value={historyTimeRange?.[0] ? historyTimeRange[0].format('YYYY-MM-DD') : ''}
            onChange={(e) => {
              const start = e.target.value ? dayjs(e.target.value) : null;
              setHistoryTimeRange([start, historyTimeRange?.[1] || null]);
              setPage(1);
            }}
            className="h-8 rounded-md border border-slate-300 bg-white px-2 py-1 text-xs text-slate-800 shadow-xs focus:outline-none focus:ring-1 focus:ring-teal-600 focus:border-teal-600"
          />
          <span className="text-slate-400">至</span>
          <input
            type="date"
            aria-label="结束日期"
            value={historyTimeRange?.[1] ? historyTimeRange[1].format('YYYY-MM-DD') : ''}
            onChange={(e) => {
              const end = e.target.value ? dayjs(e.target.value) : null;
              setHistoryTimeRange([historyTimeRange?.[0] || null, end]);
              setPage(1);
            }}
            className="h-8 rounded-md border border-slate-300 bg-white px-2 py-1 text-xs text-slate-800 shadow-xs focus:outline-none focus:ring-1 focus:ring-teal-600 focus:border-teal-600"
          />
        </div>

        <Button
          variant="ghost"
          size="sm"
          onClick={resetHistoryFilters}
          disabled={!hasHistoryFilters}
          className="h-8 text-xs text-slate-500"
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

