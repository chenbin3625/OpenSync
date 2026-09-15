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
import { CompactPagination } from '../../components/common/Pagination';
import { EmptyState } from '../../components/common/StatePlaceholder';
import { cn } from '../../lib/utils';
import { icon, surface } from '../../lib/styles';

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
  return (
    <aside
      className={cn(
        surface.card,
        'w-full flex flex-col overflow-hidden min-w-0 max-h-[380px] md:max-h-none'
      )}
    >
      {/* 顶部按钮工具栏 */}
      <div className={cn('p-3 border-b flex items-center gap-2', surface.divider)}>
        <Button size="sm" onClick={onAdd} className="flex-1">
          <Plus className={cn(icon.md, 'mr-1')} />
          新建任务
        </Button>
        <Button size="sm" variant="outline" onClick={onRunAll} className="flex-1">
          <PlayCircle className={cn(icon.md, 'mr-1 text-teal-700')} />
          执行全部
        </Button>
      </div>

      {/* 任务列表：高度交给 flex 分配（窄屏保底 240px，桌面端跟随卡片高度），
          超出时只在本容器内滚动，不再按视口高度手算 max-height 把整页撑高 */}
      <div className="flex-1 min-h-[240px] md:min-h-0 overflow-y-auto p-2 space-y-1.5">
        {list.length === 0 && !loading ? (
          <EmptyState icon={FolderSync} title="暂无同步任务，点击上方新建" size="sm" />
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
                  // 卡片落在白底上：悬浮用 line-soft，选中必须再深一档（line），
                  // 否则「悬浮到别的任务」比「当前任务」还像被选中；
                  // 也不再用 teal-50/70、tint/80 这类透明度变体，浅底统一取令牌
                  isSelected
                    ? 'bg-line border-line-strong shadow-xs'
                    : 'bg-white border-line-soft hover:border-line hover:bg-line-soft'
                )}
              >
                <div className="flex items-center justify-between gap-2 min-w-0">
                  <span className={cn('text-sm font-semibold truncate', isSelected ? 'text-teal-950' : 'text-slate-800')}>
                    {getJobName(job)}
                  </span>
                  <Badge variant={isEnabled ? 'success' : 'secondary'}>
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

                <div className="text-2xs text-slate-400 truncate font-mono" title={sourcePreview}>
                  {sourcePreview}
                </div>
              </div>
            );
          })
        )}
      </div>

      {/* 底部紧凑分页 */}
      {total > pageSize && (
        <CompactPagination
          page={page}
          pageSize={pageSize}
          total={total}
          onPageChange={setPage}
          className={cn('p-2.5 border-t', surface.divider)}
        />
      )}
    </aside>
  );
}
