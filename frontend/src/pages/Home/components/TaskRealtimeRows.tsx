import { memo } from 'react';
import { AlertCircle, FileText } from 'lucide-react';
import dayjs from 'dayjs';
import type { TaskItem } from '../../../types';
import { getTaskItemKey } from '../taskRows';
import { displayText, formatSize, getTaskDisplayName, taskTypeNames } from '../homeUtils';
import { Badge } from '../../../components/ui/badge';
import { Progress } from '../../../components/ui/progress';
import { Alert } from '../../../components/common/Alert';
import { ErrorReason } from '../../../components/common/ErrorReason';
import { CompactPagination } from '../../../components/common/CompactPagination';
import { EmptyState, ErrorState, LoadingState } from '../../../components/common/StatePlaceholder';
import { cn } from '../../../lib/utils';
import { text } from '../../../lib/styles';

function normalizeProgress(value: number | string | undefined): number {
  const parsed = typeof value === 'string' ? Number.parseFloat(value) : Number(value);
  if (!Number.isFinite(parsed)) return 0;
  return Math.max(0, Math.min(100, Math.round(parsed)));
}

export interface TaskRealtimeRowsProps {
  activeTab: number;
  activeTabLabel: string;
  loading: boolean;
  error?: boolean;
  onRetry?: () => void;
  page: number;
  pageSize: number;
  rows: TaskItem[];
  total: number;
  visibleRows: TaskItem[];
  onPageChange: (page: number) => void;
}

const TaskRealtimeRows = memo(function TaskRealtimeRows({
  activeTab,
  activeTabLabel,
  loading,
  error = false,
  onRetry,
  page,
  pageSize,
  rows,
  total,
  visibleRows,
  onPageChange,
}: TaskRealtimeRowsProps) {
  return (
    <div className="space-y-3">
      {loading && (
        <LoadingState title="正在更新文件明细" className="py-2 text-xs" />
      )}

      {error && rows.length > 0 && (
        <Alert variant="error">
          {`更新${activeTabLabel}文件明细失败`}
          {onRetry && (
            <button
              type="button"
              onClick={onRetry}
              className="ml-2 rounded-sm underline focus-visible:outline-2"
            >
              重试
            </button>
          )}
        </Alert>
      )}

      {rows.length === 0 ? (
        !loading && (
          <div className="bg-tint rounded-lg border border-dashed border-line">
            {error ? (
              <ErrorState
                icon={AlertCircle}
                title={`加载${activeTabLabel}文件明细失败`}
                onRetry={onRetry}
                retryLabel="重试"
                className="py-8"
              />
            ) : (
              <EmptyState icon={FileText} title={`暂无${activeTabLabel}文件明细`} size="sm" />
            )}
          </div>
        )
      ) : (
        <div className="border border-line rounded-lg divide-y divide-line-soft overflow-hidden bg-surface">
          {visibleRows.map((task, index) => {
            const name = getTaskDisplayName(task);
            const srcPath = displayText(task.srcPath);
            const dstPath = displayText(task.dstPath);
            const rowKey = getTaskItemKey(task, index);
            const itemType = task.type ?? 0;
            const progress = normalizeProgress(task.progress);

            const typeVariant =
              task.type === 1 ? 'error' : task.type === 2 ? 'warning' : 'default';
            const typeLabel = itemType === 0 && task.isPath
              ? '创建'
              : (taskTypeNames[itemType] || String(itemType));

            return (
              <div
                key={rowKey}
                className="p-3 hover:bg-tint transition-colors flex flex-col sm:flex-row sm:items-center justify-between gap-3 text-xs min-w-0"
              >
                <div className="flex items-center gap-2 min-w-0 flex-1">
                  <Badge variant={typeVariant}>
                    {typeLabel}
                  </Badge>
                  {Boolean(task.isPath) && (
                    <Badge variant="secondary">目录</Badge>
                  )}

                  <div className="min-w-0 flex-1 space-y-0.5">
                    <div className="font-medium text-slate-800 break-all sm:truncate" title={name}>
                      {name}
                    </div>
                    <div className="flex flex-col items-start gap-0.5 sm:flex-row sm:items-center sm:gap-2 text-2xs text-slate-400 min-w-0">
                      <span className="min-w-0 max-w-full break-all sm:truncate sm:max-w-[45%]" title={srcPath}>
                        源: {srcPath}
                      </span>
                      <span className="hidden sm:inline shrink-0">→</span>
                      <span className="min-w-0 max-w-full break-all sm:truncate sm:max-w-[45%]" title={dstPath}>
                        目标: {dstPath}
                      </span>
                    </div>
                  </div>
                </div>

                <div className="flex items-center justify-between sm:justify-end gap-4 shrink-0 text-right">
                  <span className="font-mono text-slate-500 w-16">
                    {task.isPath || task.fileSize == null ? '--' : formatSize(task.fileSize)}
                  </span>

                  <div className="w-28 flex justify-end min-w-0">
                    {activeTab === 1 && (
                      <div className="w-24 space-y-1">
                        <Progress value={progress} />
                        <div className="text-2xs font-mono text-teal-700 text-right">
                          {progress}%
                        </div>
                      </div>
                    )}

                    {/* 失败原因入口与任务列表 / 任务详情共用同一组件：同一形状、同一张卡片。
                        判空要和 ErrorReason 内部一致（它会把纯空白也当作没有原因），
                        否则纯空白时走入口分支、组件返回 null，这一格就既没有入口也没有时间戳 */}
                    {activeTab === 7 && task.errMsg?.trim() ? (
                      <ErrorReason errMsg={task.errMsg} label="失败原因" />
                    ) : (
                      activeTab !== 1 && (
                        <span className="text-slate-400 font-mono">
                          {task.createTime ? dayjs.unix(task.createTime).format('HH:mm:ss') : '--'}
                        </span>
                      )
                    )}
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {total > 0 && (
        <div className={cn('pt-2 flex items-center justify-between', text.muted)}>
          <span>共 {total} 条明细</span>
          <CompactPagination
            page={page}
            pageSize={pageSize}
            total={total}
            onPageChange={onPageChange}
            className="gap-1"
          />
        </div>
      )}
    </div>
  );
});

export default TaskRealtimeRows;
