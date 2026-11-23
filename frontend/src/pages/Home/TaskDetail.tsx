import { memo, useState, useEffect, useCallback, useMemo, useRef } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import {
  ArrowLeft,
  AlertCircle as InfoCircleOutlined,
  Inbox,
} from 'lucide-react';
import dayjs from 'dayjs';
import { jobGetTaskItem } from '../../api/job';
import type { TaskItem } from '../../types';
import { POLL_INTERVAL_MS } from '../../api/request';
import { detailHasActiveItem, mergeTaskItems, pollIntervalForActiveWork } from './taskRows';
import EllipsisText from './components/EllipsisText';
import {
  displayText,
  formatSize,
  taskItemStatusColors,
  taskItemStatusNames,
  taskItemStatusOptions,
  taskTypeNames,
} from './homeUtils';
import { canPollCurrentDocument } from './pollingVisibility';
import { Button } from '../../components/ui/button';
import { Badge } from '../../components/ui/badge';
import { Progress } from '../../components/ui/progress';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../../components/ui/select';
import { Pagination } from '../../components/common/Pagination';
import { SearchInput } from '../../components/common/SearchInput';
import { StatusBadge } from '../../components/common/StatusBadge';
import { EmptyState, ErrorState, LoadingState } from '../../components/common/StatePlaceholder';
import { cn } from '../../lib/utils';
import { control, icon, surface, table, text } from '../../lib/styles';

const typeFilterOptions = [
  { label: '全部操作', value: 'ALL' },
  { label: '复制/创建', value: '0' },
  { label: '删除', value: '1' },
  { label: '移动', value: '2' },
];

const objectFilterOptions = [
  { label: '全部对象', value: 'ALL' },
  { label: '文件', value: '0' },
  { label: '目录', value: '1' },
];

const errorFilterOptions = [
  { label: '全部信息', value: 'ALL' },
  { label: '有错误信息', value: '1' },
  { label: '无错误信息', value: '0' },
];

function pathFallback(record: TaskItem): string {
  return displayText(record.fileName || record.dstPath || record.srcPath);
}

function normalizeProgress(value: number | string | undefined): number {
  const parsed = typeof value === 'string' ? Number.parseFloat(value) : Number(value);
  if (!Number.isFinite(parsed)) return 0;
  return Math.max(0, Math.min(100, Math.round(parsed)));
}

const detailCell = 'min-w-0 p-0 xl:px-3 xl:py-2.5';
const pathCell = 'col-span-3 grid grid-cols-[2.5rem_minmax(0,1fr)] gap-x-1 xl:table-cell';

const TaskDetailRow = memo(function TaskDetailRow({ record }: { record: TaskItem }) {
  const status = record.status ?? 0;
  const itemType = record.type ?? 0;
  const progress = normalizeProgress(record.progress);
  const errorReason = typeof record.errMsg === 'string' ? record.errMsg.trim() : '';
  return (
    <tr role="row" className={cn(table.row, 'grid grid-cols-[minmax(0,1fr)_auto_auto] items-start gap-x-2 gap-y-1.5 p-3 xl:table-row xl:p-0')}>
      <td role="cell" className={cn(detailCell, 'col-span-2 row-start-1 font-medium text-slate-800')}>
        <EllipsisText value={pathFallback(record)} maxWidth={200} />
      </td>
      <td role="cell" className={cn(detailCell, pathCell, 'row-start-2 text-slate-600')}>
        <span aria-hidden="true" className="text-slate-500 xl:hidden">来源</span>
        <EllipsisText value={record.srcPath} maxWidth={220} />
      </td>
      <td role="cell" className={cn(detailCell, pathCell, 'row-start-3 text-slate-600')}>
        <span aria-hidden="true" className="text-slate-500 xl:hidden">目标</span>
        <EllipsisText value={record.dstPath} maxWidth={220} />
      </td>
      <td role="cell" className={cn(detailCell, 'row-start-4 self-center font-mono text-slate-600')}>
        {record.isPath || record.fileSize == null ? '--' : formatSize(record.fileSize)}
      </td>
      <td role="cell" className={cn(detailCell, 'row-start-4')}>
        <Badge variant={itemType === 1 ? 'error' : itemType === 2 ? 'warning' : 'default'}>
          {itemType === 0 && record.isPath ? '创建' : (taskTypeNames[itemType] || String(itemType))}
        </Badge>
      </td>
      <td role="cell" className={cn(detailCell, 'row-start-4')}>
        <Badge variant={record.isPath ? 'secondary' : 'outline'}>
          {record.isPath ? '目录' : '文件'}
        </Badge>
      </td>
      <td role="cell" className={cn(detailCell, 'col-start-3 row-start-1 justify-self-end')}>
        {status === 1 ? (
          <div className="flex items-center gap-1.5">
            <Progress value={progress} className="w-12 xl:w-24" />
            <span className="text-2xs font-medium text-slate-500">{progress}%</span>
          </div>
        ) : (
          <StatusBadge
            label={taskItemStatusNames[status] || String(status)}
            color={taskItemStatusColors[status]}
            errMsg={taskItemStatusColors[status] === 'error' && errorReason ? record.errMsg : undefined}
          />
        )}
      </td>
      <td role="cell" className={cn(detailCell, 'col-span-3 row-start-5 font-mono text-2xs text-slate-500')}>
        {record.createTime ? dayjs.unix(record.createTime).format('YYYY-MM-DD HH:mm:ss') : '--'}
      </td>
    </tr>
  );
});

type TaskDetailProps = {
  taskId?: number | string;
  embedded?: boolean;
  onBack?: () => void;
};

type TaskItemPage = {
  filters: Record<string, unknown>;
  page: number;
  pageSize: number;
  items: TaskItem[];
  count: number;
  error: boolean;
};

export default function TaskDetail({ taskId: taskIdProp, embedded = false, onBack }: TaskDetailProps = {}) {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const routeTaskId = searchParams.get('taskId') || '';
  const taskId = taskIdProp !== undefined && taskIdProp !== null ? String(taskIdProp) : routeTaskId;

  const [pageResult, setPageResult] = useState<TaskItemPage | null>(null);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [statusFilter, setStatusFilter] = useState<number | undefined>(undefined);
  const [typeFilter, setTypeFilter] = useState<number | undefined>(undefined);
  const [objectFilter, setObjectFilter] = useState<number | undefined>(undefined);
  const [errorFilter, setErrorFilter] = useState<number | undefined>(undefined);
  const [keywordInput, setKeywordInput] = useState('');
  const [keywordFilter, setKeywordFilter] = useState('');
  const abortRef = useRef<AbortController | null>(null);
  const pollDelayRef = useRef(POLL_INTERVAL_MS);
  const prevTaskIdRef = useRef(taskId);

  useEffect(() => {
    if (prevTaskIdRef.current === taskId) return;
    prevTaskIdRef.current = taskId;
    abortRef.current?.abort();
    abortRef.current = null;
    pollDelayRef.current = POLL_INTERVAL_MS;
    setPageResult(null);
    setPage(1);
    setStatusFilter(undefined);
    setTypeFilter(undefined);
    setObjectFilter(undefined);
    setErrorFilter(undefined);
    setKeywordInput('');
    setKeywordFilter('');
  }, [taskId]);

  const filters = useMemo(() => ({
    taskId,
    status: statusFilter,
    type: typeFilter,
    isPath: objectFilter,
    hasError: errorFilter,
    keyword: keywordFilter || undefined,
  }), [errorFilter, keywordFilter, objectFilter, statusFilter, taskId, typeFilter]);
  const filteredResult = pageResult?.filters === filters ? pageResult : null;
  const currentPage = filteredResult?.page === page && filteredResult.pageSize === pageSize ? filteredResult : null;
  const loading = Boolean(taskId && !currentPage);
  const total = filteredResult?.count ?? 0;
  const hasFilters = Boolean(keywordFilter) || [statusFilter, typeFilter, objectFilter, errorFilter].some((value) => value !== undefined);

  const fetchData = useCallback(async (options?: { silent?: boolean }) => {
    if (!taskId) return;
    if (options?.silent && abortRef.current) return;
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    try {
      const res = await jobGetTaskItem({ ...filters, pageSize, pageNum: page }, { signal: controller.signal, silent: true });
      if (controller.signal.aborted) return;
      const data = res.data;
      const rawList = Array.isArray(data?.dataList) ? data.dataList : [];
      const items = rawList.map((item) => {
        const progress = normalizeProgress(item.progress);
        return item.progress === progress ? item : { ...item, progress };
      });
      const count = data?.count ?? 0;
      setPageResult((previous) => {
        if (previous?.filters === filters && previous.page === page && previous.pageSize === pageSize && !previous.error) {
          const merged = mergeTaskItems(previous.items, items);
          if (merged === previous.items && count === previous.count) return previous;
          return { ...previous, items: merged, count };
        }
        return { filters, page, pageSize, items, count, error: false };
      });
      const lastPage = Math.max(1, Math.ceil(count / pageSize));
      if (page > lastPage) setPage(lastPage);
      // Poll fast only while this page can still change; a page of finished
      // rows (and the usual "task is done" case) falls back to the idle rate.
      pollDelayRef.current = pollIntervalForActiveWork(detailHasActiveItem(items), POLL_INTERVAL_MS);
    } catch (err) {
      if (controller.signal.aborted) return;
      if (options?.silent) {
        console.error('task detail polling failed', err);
        return;
      }
      setPageResult({ filters, page, pageSize, items: [], count: 0, error: true });
      console.error('task detail fetch failed', err);
    } finally {
      if (abortRef.current === controller) abortRef.current = null;
    }
  }, [filters, page, pageSize, taskId]);

  useEffect(() => {
    pollDelayRef.current = POLL_INTERVAL_MS;
    void fetchData();
    return () => abortRef.current?.abort();
  }, [fetchData]);

  useEffect(() => {
    if (!taskId) return undefined;
    pollDelayRef.current = POLL_INTERVAL_MS;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let cancelled = false;
    // Chain timeouts so the current adaptive delay is picked up on the next
    // tick without recreating the timer on every list update.
    const schedule = () => {
      if (cancelled) return;
      timer = setTimeout(async () => {
        if (!cancelled && canPollCurrentDocument()) {
          await fetchData({ silent: true });
        }
        schedule();
      }, pollDelayRef.current);
    };
    schedule();
    return () => {
      cancelled = true;
      if (timer) clearTimeout(timer);
    };
  }, [taskId, fetchData]);

  const handleBack = () => {
    if (onBack) {
      onBack();
      return;
    }
    const historyIdx = typeof window !== 'undefined' && typeof window.history.state?.idx === 'number'
      ? window.history.state.idx
      : null;
    const canGoBack = typeof window !== 'undefined' &&
      window.history.length > 1 &&
      (historyIdx === null ? true : historyIdx > 0);
    if (canGoBack) {
      navigate(-1);
    } else {
      navigate('/home', { replace: true });
    }
  };

  const handleKeywordSearch = (value: string) => {
    setKeywordFilter(value.trim());
    setPage(1);
  };

  const resetFilters = () => {
    setStatusFilter(undefined);
    setTypeFilter(undefined);
    setObjectFilter(undefined);
    setErrorFilter(undefined);
    setKeywordInput('');
    setKeywordFilter('');
    setPage(1);
  };

  return (
    <div
      className={cn(
        surface.card,
        surface.cardPadding,
        'flex min-w-0 flex-col',
        embedded && 'border-0 shadow-none p-0 rounded-none'
      )}
    >
      {/* 顶部标题与筛选栏 */}
      <div
        className={cn(
          'flex flex-col xl:flex-row xl:items-center justify-between gap-3 pb-4 border-b',
          surface.divider
        )}
      >
        {!embedded && (
          <div className="flex items-center gap-3">
            <Button
              variant="outline"
              size="sm"
              onClick={handleBack}
              className="flex items-center gap-1.5"
            >
              <ArrowLeft className={icon.md} />
              <span>返回</span>
            </Button>
            <h2 className={text.pageTitle}>任务详情</h2>
          </div>
        )}

        <div className="grid min-w-0 grid-cols-2 gap-2 sm:flex sm:flex-wrap sm:items-center" role="group" aria-label="文件明细筛选">
          {/* 关键字搜索 */}
          <SearchInput
            value={keywordInput}
            placeholder="文件 / 路径 / 错误"
            className="col-span-2 w-full sm:w-48"
            onChange={(value) => {
              setKeywordInput(value);
              if (!value) handleKeywordSearch('');
            }}
            onSearch={() => handleKeywordSearch(keywordInput)}
            onClear={() => handleKeywordSearch('')}
          />

          {/* 状态筛选 */}
          <Select
            value={statusFilter !== undefined ? String(statusFilter) : 'ALL'}
            onValueChange={(val) => {
              setStatusFilter(val === 'ALL' ? undefined : Number(val));
              setPage(1);
            }}
          >
            <SelectTrigger aria-label="文件状态" className={cn(control.compact, 'min-w-0 w-full sm:w-28')}>
              <SelectValue placeholder="筛选状态" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="ALL">全部状态</SelectItem>
              {taskItemStatusOptions.map((opt) => (
                <SelectItem key={opt.value} value={String(opt.value)}>
                  {opt.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          {/* 操作类型 */}
          <Select
            value={typeFilter !== undefined ? String(typeFilter) : 'ALL'}
            onValueChange={(val) => {
              setTypeFilter(val === 'ALL' ? undefined : Number(val));
              setPage(1);
            }}
          >
            <SelectTrigger aria-label="操作类型" className={cn(control.compact, 'min-w-0 w-full sm:w-28')}>
              <SelectValue placeholder="操作类型" />
            </SelectTrigger>
            <SelectContent>
              {typeFilterOptions.map((opt) => (
                <SelectItem key={opt.value} value={opt.value}>
                  {opt.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          {/* 对象类型 */}
          <Select
            value={objectFilter !== undefined ? String(objectFilter) : 'ALL'}
            onValueChange={(val) => {
              setObjectFilter(val === 'ALL' ? undefined : Number(val));
              setPage(1);
            }}
          >
            <SelectTrigger aria-label="对象类型" className={cn(control.compact, 'min-w-0 w-full sm:w-28')}>
              <SelectValue placeholder="文件/目录" />
            </SelectTrigger>
            <SelectContent>
              {objectFilterOptions.map((opt) => (
                <SelectItem key={opt.value} value={opt.value}>
                  {opt.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          {/* 错误信息 */}
          <Select
            value={errorFilter !== undefined ? String(errorFilter) : 'ALL'}
            onValueChange={(val) => {
              setErrorFilter(val === 'ALL' ? undefined : Number(val));
              setPage(1);
            }}
          >
            <SelectTrigger aria-label="错误信息" className={cn(control.compact, 'min-w-0 w-full sm:w-28')}>
              <SelectValue placeholder="错误信息" />
            </SelectTrigger>
            <SelectContent>
              {errorFilterOptions.map((opt) => (
                <SelectItem key={opt.value} value={opt.value}>
                  {opt.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <Button variant="ghost" size="sm" onClick={resetFilters}>
            重置
          </Button>
        </div>
      </div>

      <div className="mt-4 min-w-0">
        {loading ? (
          <LoadingState className="min-h-40" title="正在加载文件明细" />
        ) : currentPage?.error ? (
          <ErrorState
            icon={InfoCircleOutlined}
            title="文件详情加载失败"
            onRetry={() => {
              setPageResult(null);
              void fetchData();
            }}
            retryLabel="重试"
          />
        ) : !currentPage?.items.length ? (
          <EmptyState icon={Inbox} title={hasFilters ? '没有符合筛选条件的文件明细' : '暂无文件详情记录'} />
        ) : (
          <div className={cn(table.wrapper, 'min-w-0')}>
            <table role="table" aria-label="文件明细" className={cn(table.root, 'block xl:table')}>
              <thead role="rowgroup" className={cn(table.head, 'sr-only xl:not-sr-only xl:table-header-group')}>
                <tr role="row">
                  <th scope="col" className={cn(table.headCell, 'min-w-[200px]')}>文件名/目录</th>
                  <th scope="col" className={cn(table.headCell, 'min-w-[220px]')}>来源目录</th>
                  <th scope="col" className={cn(table.headCell, 'min-w-[220px]')}>目标目录</th>
                  <th scope="col" className={cn(table.headCell, 'w-24')}>文件大小</th>
                  <th scope="col" className={cn(table.headCell, 'w-20')}>操作类型</th>
                  <th scope="col" className={cn(table.headCell, 'w-16')}>对象</th>
                  <th scope="col" className={cn(table.headCell, 'min-w-[170px]')}>状态</th>
                  <th scope="col" className={cn(table.headCell, 'w-36')}>创建时间</th>
                </tr>
              </thead>
              <tbody role="rowgroup" className={cn(table.body, 'block xl:table-row-group')}>
                {currentPage.items.map((record) => <TaskDetailRow key={record.id} record={record} />)}
              </tbody>
            </table>
          </div>
        )}

        {total > 0 && (
          <Pagination
            page={page}
            pageSize={pageSize}
            total={total}
            unit="条记录"
            onPageChange={(next) => setPage(next)}
            onPageSizeChange={(size) => {
              setPageSize(size);
              setPage(1);
            }}
          />
        )}
      </div>
    </div>
  );
}
