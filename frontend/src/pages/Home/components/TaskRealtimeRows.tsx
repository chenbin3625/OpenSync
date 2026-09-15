import { memo } from 'react';
import { AlertCircle, FileText, Loader2 } from 'lucide-react';
import dayjs from 'dayjs';
import type { TaskItem } from '../../../types';
import { getTaskItemKey } from '../taskRows';
import { displayText, formatSize, getTaskDisplayName, taskTypeNames } from '../homeUtils';
import { Badge } from '../../../components/ui/badge';
import { Progress } from '../../../components/ui/progress';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '../../../components/ui/tooltip';
import { CompactPagination } from '../../../components/common/Pagination';
import { EmptyState } from '../../../components/common/StatePlaceholder';
import { cn } from '../../../lib/utils';
import { icon, text } from '../../../lib/styles';

export interface TaskRealtimeRowsProps {
  activeTab: number;
  activeTabLabel: string;
  loading: boolean;
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
  page,
  pageSize,
  rows,
  total,
  visibleRows,
  onPageChange,
}: TaskRealtimeRowsProps) {
  return (
    <TooltipProvider>
      <div className="space-y-3">
        {loading && (
          <div className="flex items-center justify-center py-2 text-xs text-teal-700 gap-1.5 animate-pulse">
            <Loader2 className={cn(icon.sm, 'animate-spin')} />
            <span>更新数据中...</span>
          </div>
        )}

        {rows.length === 0 ? (
          <div className="bg-slate-50 rounded-lg border border-dashed border-slate-200">
            <EmptyState icon={FileText} title={`暂无${activeTabLabel}文件明细`} size="sm" />
          </div>
        ) : (
          <div className="border border-slate-200 rounded-lg divide-y divide-slate-100 overflow-hidden bg-white">
            <div>
              {visibleRows.map((task, index) => {
                const name = getTaskDisplayName(task);
                const srcPath = displayText(task.srcPath);
                const dstPath = displayText(task.dstPath);
                const rowKey = getTaskItemKey(task, index);

                const typeVariant =
                  task.type === 1 ? 'destructive' : task.type === 2 ? 'warning' : 'default';

                return (
                  <div
                    key={rowKey}
                    className="p-3 hover:bg-slate-50 transition-colors flex items-center justify-between gap-3 text-xs min-w-0"
                  >
                    <div className="flex items-center gap-2 min-w-0 flex-1">
                      <Badge variant={typeVariant} className="text-2xs px-1.5 py-0 shrink-0">
                        {taskTypeNames[task.type ?? 0] || '复制'}
                      </Badge>

                      <div className="min-w-0 flex-1 space-y-0.5">
                        <div className="font-medium text-slate-800 truncate" title={name}>
                          {name}
                        </div>
                        <div className="flex items-center gap-2 text-2xs text-slate-400 truncate">
                          <span className="truncate max-w-[45%]" title={srcPath}>
                            源: {srcPath}
                          </span>
                          <span>→</span>
                          <span className="truncate max-w-[45%]" title={dstPath}>
                            目标: {dstPath}
                          </span>
                        </div>
                      </div>
                    </div>

                    <div className="flex items-center gap-4 shrink-0 text-right">
                      <span className="font-mono text-slate-500 w-16">
                        {formatSize(task.fileSize || 0)}
                      </span>

                      <div className="w-28 flex justify-end min-w-0">
                        {activeTab === 1 && (
                          <div className="w-24 space-y-1">
                            <Progress value={Math.round(Number(task.progress || 0))} className="h-1.5" />
                            <div className="text-2xs font-mono text-teal-700 text-right">
                              {Math.round(Number(task.progress || 0))}%
                            </div>
                          </div>
                        )}

                        {activeTab === 7 && task.errMsg && (
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span className="inline-flex items-center gap-1 text-rose-600 cursor-pointer font-medium">
                                <AlertCircle className={icon.sm} />
                                失败原因
                              </span>
                            </TooltipTrigger>
                            <TooltipContent className="max-w-xs text-xs font-mono whitespace-pre-wrap">
                              {task.errMsg}
                            </TooltipContent>
                          </Tooltip>
                        )}

                        {activeTab !== 1 && !(activeTab === 7 && task.errMsg) && (
                          <span className="text-slate-400 font-mono">
                            {task.createTime ? dayjs.unix(task.createTime).format('HH:mm:ss') : '--'}
                          </span>
                        )}
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>
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
    </TooltipProvider>
  );
});

export default TaskRealtimeRows;
