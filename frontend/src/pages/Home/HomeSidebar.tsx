import { Plus, PlayCircle, FolderSync } from 'lucide-react';
import type { JobItem } from '../../types';
import { getJobName } from './homeUtils';
import { Button } from '../../components/ui/button';
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
      <div className="flex-1 min-h-[240px] md:min-h-0 overflow-y-auto p-2 space-y-1">
        {list.length === 0 && !loading ? (
          <EmptyState icon={FolderSync} title="暂无同步任务，点击上方新建" size="sm" />
        ) : (
          list.map((job) => {
            const isSelected = selectedJobId === job.id;

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
                  'w-full text-left px-3 py-2 rounded-lg text-sm transition-colors cursor-pointer select-none truncate',
                  isSelected
                    ? 'bg-line text-teal-950 font-semibold'
                    : 'text-slate-700 hover:text-slate-900 hover:bg-line-soft font-medium'
                )}
                title={getJobName(job)}
              >
                {getJobName(job)}
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
