import { useState, useEffect, useCallback, useRef } from 'react';
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
import { EmptyState, ErrorState } from '../../components/common/StatePlaceholder';
import { cn } from '../../lib/utils';
import { control, icon, layout, surface, table, text } from '../../lib/styles';

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

type TaskDetailProps = {
  taskId?: number | string;
  embedded?: boolean;
  onBack?: () => void;
};

export default function TaskDetail({ taskId: taskIdProp, embedded = false, onBack }: TaskDetailProps = {}) {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const routeTaskId = searchParams.get('taskId') || '';
  const taskId = taskIdProp !== undefined && taskIdProp !== null ? String(taskIdProp) : routeTaskId;

  const [list, setList] = useState<TaskItem[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const [statusFilter, setStatusFilter] = useState<number | undefined>(undefined);
  const [typeFilter, setTypeFilter] = useState<number | undefined>(undefined);
  const [objectFilter, setObjectFilter] = useState<number | undefined>(undefined);
  const [errorFilter, setErrorFilter] = useState<number | undefined>(undefined);
  const [keywordInput, setKeywordInput] = useState('');
  const [keywordFilter, setKeywordFilter] = useState('');
  const requestRef = useRef(0);
  const loadingRequestRef = useRef(0);
  const abortRef = useRef<AbortController | null>(null);

  const fetchData = useCallback(async (options?: { silent?: boolean }) => {
    if (!taskId) return;
    const showLoading = !options?.silent;
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    const requestID = ++requestRef.current;
    const loadingRequestID = showLoading ? ++loadingRequestRef.current : 0;
    if (showLoading) {
      setLoading(true);
      setError(false);
    }
    try {
      const params: Record<string, unknown> = {
        taskId,
        pageSize,
        pageNum: page,
      };
      if (statusFilter !== undefined) params.status = statusFilter;
      if (typeFilter !== undefined) params.type = typeFilter;
      if (objectFilter !== undefined) params.isPath = objectFilter;
      if (errorFilter !== undefined) params.hasError = errorFilter;
      if (keywordFilter.trim()) params.keyword = keywordFilter.trim();
      const res = await jobGetTaskItem(params, { signal: controller.signal, silent: options?.silent });
      if (requestID !== requestRef.current || controller.signal.aborted) return;
      const data = res.data;
      const rawList = Array.isArray(data?.dataList) ? data.dataList : [];
      const items = rawList.map((item) => {
        const prog = typeof item.progress === 'string' ? parseInt(item.progress, 10) : (item.progress || 0);
        return { ...item, progress: Math.max(0, Math.min(prog || 0, 100)) };
      });
      setList(items);
      setTotal(data?.count || 0);
    } catch (err) {
      if (controller.signal.aborted) return;
      if (requestID !== requestRef.current) return;
      if (options?.silent) {
        console.error('task detail polling failed', err);
        return;
      }
      setError(true);
      setList([]);
      setTotal(0);
      console.error('task detail fetch failed', err);
    } finally {
      if (showLoading && loadingRequestID === loadingRequestRef.current) {
        setLoading(false);
      }
    }
  }, [errorFilter, keywordFilter, objectFilter, page, pageSize, statusFilter, taskId, typeFilter]);

  useEffect(() => { fetchData(); }, [fetchData]);
  useEffect(() => () => { abortRef.current?.abort(); }, []);

  useEffect(() => {
    if (!taskId) return undefined;
    const pollID = setInterval(() => {
      if (canPollCurrentDocument()) fetchData({ silent: true });
    }, POLL_INTERVAL_MS);
    return () => { clearInterval(pollID); };
  }, [taskId, fetchData]);

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
        'flex flex-col',
        embedded && 'border-0 shadow-none p-0 rounded-none'
      )}
    >
      {/* 顶部标题与筛选栏 */}
      <div
        className={cn(
          'flex flex-col md:flex-row md:items-center justify-between gap-3 pb-4 border-b',
          surface.divider
        )}
      >
        {!embedded && (
          <div className="flex items-center gap-3">
            <Button
              variant="outline"
              size="sm"
              onClick={() => (onBack ? onBack() : navigate(-1))}
              className="flex items-center gap-1.5"
            >
              <ArrowLeft className={icon.md} />
              <span>返回</span>
            </Button>
            <h2 className={text.pageTitle}>任务详情</h2>
          </div>
        )}

        <div className={layout.filterBar}>
          {/* 关键字搜索 */}
          <SearchInput
            value={keywordInput}
            placeholder="文件 / 路径 / 错误"
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
            <SelectTrigger className={cn(control.compact, 'w-28')}>
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
            <SelectTrigger className={cn(control.compact, 'w-28')}>
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
            <SelectTrigger className={cn(control.compact, 'w-24')}>
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
            <SelectTrigger className={cn(control.compact, 'w-28')}>
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

          <Button variant="ghost" size="sm" onClick={resetFilters} className="text-slate-500">
            重置
          </Button>
        </div>
      </div>

      {/* 内容区域 */}
      {error ? (
        <ErrorState
          icon={InfoCircleOutlined}
          title="文件详情加载失败"
          onRetry={() => fetchData()}
          retryLabel="重试"
        />
      ) : list.length === 0 && !loading ? (
        <EmptyState icon={Inbox} title="暂无文件详情记录" />
      ) : (
        <div className="flex-1 overflow-auto mt-4">
          <div className={table.wrapper}>
            <table className={table.root}>
              <thead className={table.head}>
                <tr>
                  <th className={cn(table.headCell, 'min-w-[200px]')}>文件名/目录</th>
                  <th className={cn(table.headCell, 'min-w-[220px]')}>来源目录</th>
                  <th className={cn(table.headCell, 'min-w-[220px]')}>目标目录</th>
                  <th className={cn(table.headCell, 'w-24')}>文件大小</th>
                  <th className={cn(table.headCell, 'w-20')}>操作类型</th>
                  <th className={cn(table.headCell, 'w-16')}>对象</th>
                  <th className={cn(table.headCell, 'min-w-[170px]')}>状态</th>
                  <th className={cn(table.headCell, 'w-36')}>创建时间</th>
                </tr>
              </thead>
              <tbody className={table.body}>
                {list.map((record) => {
                  const status = record.status ?? 0;
                  const errorReason = typeof record.errMsg === 'string' ? record.errMsg.trim() : '';
                  return (
                    <tr key={record.id} className={table.row}>
                      <td className={cn(table.cell, 'font-medium text-slate-800')}>
                        <EllipsisText value={pathFallback(record)} maxWidth={200} />
                      </td>
                      <td className={cn(table.cell, 'text-slate-500')}>
                        <EllipsisText value={record.srcPath} maxWidth={220} />
                      </td>
                      <td className={cn(table.cell, 'text-slate-500')}>
                        <EllipsisText value={record.dstPath} maxWidth={220} />
                      </td>
                      <td className={cn(table.cell, 'text-slate-600 font-mono')}>
                        {record.fileSize == null ? '--' : formatSize(record.fileSize)}
                      </td>
                      <td className={table.cell}>
                        {(() => {
                          const itemType = record.type ?? 0;
                          return (
                            <Badge
                              variant={
                                itemType === 1
                                  ? 'destructive'
                                  : itemType === 2
                                  ? 'warning'
                                  : 'default'
                              }
                            >
                              {itemType === 0 && record.isPath ? '创建' : (taskTypeNames[itemType] || String(itemType))}
                            </Badge>
                          );
                        })()}
                      </td>
                      <td className={table.cell}>
                        <Badge variant={record.isPath ? 'secondary' : 'outline'}>
                          {record.isPath ? '目录' : '文件'}
                        </Badge>
                      </td>
                      <td className={table.cell}>
                        {status === 1 ? (
                          <div className="flex items-center gap-2">
                            <Progress value={Number(record.progress || 0)} className="h-1.5 w-24" />
                            <span className="text-2xs font-medium text-slate-500">{record.progress || 0}%</span>
                          </div>
                        ) : (
                          <StatusBadge
                            label={taskItemStatusNames[status] || String(status)}
                            color={taskItemStatusColors[status]}
                            errMsg={
                              taskItemStatusColors[status] === 'error' && errorReason
                                ? record.errMsg
                                : undefined
                            }
                          />
                        )}
                      </td>
                      <td className={cn(table.cell, 'text-slate-400 font-mono text-2xs')}>
                        {record.createTime ? dayjs.unix(record.createTime).format('YYYY-MM-DD HH:mm:ss') : '--'}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>

          {/* 分页控制器 */}
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
        </div>
      )}
    </div>
  );
}

