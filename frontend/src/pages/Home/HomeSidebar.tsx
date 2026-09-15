import { Plus, PlayCircle, FolderSync } from 'lucide-react';
import type { JobItem } from '../../types';
import {
  methodNames,
  formatJobPaths,
  countJobPaths,
  getJobName,
  formatSchedule,
} from './homeUtils';
import { Button } from '../../components/ui/button';
import { Badge } from '../../components/ui/badge';
import { cn } from '../../lib/utils';

export interface HomeSidebarProps {
  list: JobItem[];
  loading: boolean;
  selectedJobId: number | null;
  total: number;
  page: number;
  pageSize: number;
  onAdd: () => void;
  onRunAll: () => void;
  onSelectJob: (jobId: number) => void;
  onClearTaskDetail: () => void;
  setPage: (page: number) => void;
}

export default function HomeSidebar({
  list,
  loading,
  selectedJobId,
  total,
  page,
  pageSize,
  onAdd,
  onRunAll,
  onSelectJob,
  onClearTaskDetail,
  setPage,
}: HomeSidebarProps) {
  const totalPages = Math.ceil(total / pageSize);

  return (
    <aside className="w-full flex flex-col bg-white rounded-xl border border-slate-200/80 shadow-xs overflow-hidden min-w-0 max-h-[380px] md:max-h-none">
      {/* 顶部按钮工具栏 */}
      <div className="p-3 border-b border-slate-100 flex items-center gap-2">
        <Button size="sm" onClick={onAdd} className="flex-1 shadow-xs">
          <Plus className="h-4 w-4 mr-1" />
          新建任务
        </Button>
        <Button size="sm" variant="outline" onClick={onRunAll} className="flex-1 shadow-xs">
          <PlayCircle className="h-4 w-4 mr-1 text-teal-700" />
          执行全部
        </Button>
      </div>

      {/* 任务列表 */}
      <div className="flex-1 overflow-y-auto p-2 space-y-1.5 min-h-[300px] max-h-[calc(100vh-240px)]">
        {list.length === 0 && !loading ? (
          <div className="py-16 text-center space-y-2">
            <FolderSync className="h-8 w-8 text-slate-300 mx-auto" />
            <p className="text-xs text-slate-400">暂无同步任务，点击上方新建</p>
          </div>
        ) : (
          list.map((job) => {
            const isSelected = selectedJobId === job.id;
            const isEnabled = job.enable === 1;
            const sourcePreview = formatJobPaths(job.srcPath) || '源目录未配置';
            const targetCount = countJobPaths(job.dstPath);

            return (
              <div
                key={job.id}
                role="button"
                tabIndex={0}
                onClick={() => {
                  onSelectJob(job.id);
                  onClearTaskDetail();
                }}
                className={cn(
                  'w-full text-left p-3 rounded-lg border transition-all cursor-pointer select-none space-y-1.5',
                  isSelected
                    ? 'bg-teal-50/70 border-teal-200 shadow-xs'
                    : 'bg-white border-slate-100 hover:border-slate-200 hover:bg-slate-50/80'
                )}
              >
                <div className="flex items-center justify-between gap-2 min-w-0">
                  <span className={cn('text-sm font-semibold truncate', isSelected ? 'text-teal-950' : 'text-slate-800')}>
                    {getJobName(job)}
                  </span>
                  <Badge
                    variant={isEnabled ? 'success' : 'secondary'}
                    className="text-[10px] px-1.5 py-0 shrink-0"
                  >
                    {isEnabled ? '已启用' : '已暂停'}
                  </Badge>
                </div>

                <div className="flex items-center justify-between text-xs text-slate-500 min-w-0">
                  <span className="font-medium text-slate-600">
                    {methodNames[job.method] || job.method}
                  </span>
                  <span className="text-slate-400 truncate max-w-[120px]">
                    {targetCount > 1 ? `${targetCount} 个目标` : formatSchedule(job)}
                  </span>
                </div>

                <div className="text-[11px] text-slate-400 truncate font-mono" title={sourcePreview}>
                  {sourcePreview}
                </div>
              </div>
            );
          })
        )}
      </div>

      {/* 底部紧凑分页 */}
      {total > pageSize && (
        <div className="p-2.5 border-t border-slate-100 flex items-center justify-between text-xs text-slate-500">
          <span>共 {total} 个任务</span>
          <div className="flex items-center gap-1">
            <Button
              variant="outline"
              size="sm"
              disabled={page <= 1}
              onClick={() => setPage(page - 1)}
              className="h-7 px-2 text-xs"
            >
              上一页
            </Button>
            <span className="px-1 font-medium">{page} / {totalPages}</span>
            <Button
              variant="outline"
              size="sm"
              disabled={page >= totalPages}
              onClick={() => setPage(page + 1)}
              className="h-7 px-2 text-xs"
            >
              下一页
            </Button>
          </div>
        </div>
      )}
    </aside>
  );
}
