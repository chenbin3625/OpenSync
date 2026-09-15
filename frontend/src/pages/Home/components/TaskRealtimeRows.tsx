import { memo } from 'react';
import { AlertCircle, FileText, Loader2 } from 'lucide-react';
import dayjs from 'dayjs';
import type { TaskItem } from '../../../types';
import { getTaskItemKey } from '../taskRows';
import { displayText, formatSize, getTaskDisplayName, taskTypeNames } from '../homeUtils';
import { Badge } from '../../../components/ui/badge';
import { Progress } from '../../../components/ui/progress';
import { Button } from '../../../components/ui/button';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '../../../components/ui/tooltip';

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
  const totalPages = Math.ceil(total / pageSize);

  return (
    <TooltipProvider>
      <div className="space-y-3">
        {loading && (
          <div className="flex items-center justify-center py-2 text-xs text-teal-700 gap-1.5 animate-pulse">
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
            <span>更新数据中...</span>
          </div>
        )}

        {rows.length === 0 ? (
          <div className="py-12 text-center space-y-2 bg-slate-50/50 rounded-lg border border-dashed border-slate-200">
            <FileText className="h-8 w-8 text-slate-300 mx-auto" />
            <p className="text-xs text-slate-400">暂无{activeTabLabel}文件明细</p>
          </div>
        ) : (
          <div className="border border-slate-200/80 rounded-lg divide-y divide-slate-100 overflow-hidden bg-white">
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
                    className="p-3 hover:bg-slate-50/80 transition-colors flex items-center justify-between gap-3 text-xs min-w-0"
                  >
                    <div className="flex items-center gap-2.5 min-w-0 flex-1">
                      <Badge variant={typeVariant} className="text-[10px] px-1.5 py-0 shrink-0">
                        {taskTypeNames[task.type ?? 0] || '复制'}
                      </Badge>

                      <div className="min-w-0 flex-1 space-y-0.5">
                        <div className="font-medium text-slate-800 truncate" title={name}>
                          {name}
                        </div>
                        <div className="flex items-center gap-2 text-[11px] text-slate-400 truncate">
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
                            <div className="text-[10px] font-mono text-teal-700 text-right">
                              {Math.round(Number(task.progress || 0))}%
                            </div>
                          </div>
                        )}

                        {activeTab === 7 && task.errMsg && (
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span className="inline-flex items-center gap-1 text-red-600 cursor-pointer font-medium">
                                <AlertCircle className="h-3 w-3" />
                                失败原因
                              </span>
                            </TooltipTrigger>
                            <TooltipContent className="max-w-xs text-xs font-mono text-rose-100 bg-rose-950">
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
          <div className="pt-2 flex items-center justify-between text-xs text-slate-500">
            <span>共 {total} 条明细</span>
            {totalPages > 1 && (
              <div className="flex items-center gap-1">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={page <= 1}
                  onClick={() => onPageChange(page - 1)}
                  className="h-7 px-2 text-xs"
                >
                  上一页
                </Button>
                <span className="px-2 font-medium">{page} / {totalPages}</span>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={page >= totalPages}
                  onClick={() => onPageChange(page + 1)}
                  className="h-7 px-2 text-xs"
                >
                  下一页
                </Button>
              </div>
            )}
          </div>
        )}
      </div>
    </TooltipProvider>
  );
});

export default TaskRealtimeRows;
