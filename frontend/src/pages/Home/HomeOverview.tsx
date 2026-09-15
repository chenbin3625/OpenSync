import { useState } from 'react';
import {
  Play, Edit, Trash2, Folder, Filter, Server,
  CheckCircle2, PauseCircle, HardDrive, Clock
} from 'lucide-react';
import type { JobItem } from '../../types';
import dayjs from 'dayjs';
import {
  methodNames,
  formatJobPaths,
  countJobPaths,
  formatFileSizeRange,
  formatExcludePreview,
  getJobName,
  formatSchedule,
  formatCache,
} from './homeUtils';
import { Button } from '../../components/ui/button';
import { Badge } from '../../components/ui/badge';
import { Switch } from '../../components/ui/switch';
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
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '../../components/ui/tooltip';
import { SectionHeading } from '../../components/common/PageHeader';
import { InfoRow } from '../../components/common/InfoRow';
import { cn } from '../../lib/utils';
import { icon, surface, text } from '../../lib/styles';

export interface HomeOverviewProps {
  selectedJob: JobItem;
  onRun: (id: number) => void;
  onEdit: (job: JobItem) => void;
  onDelete: (id: number) => void;
  onToggle: (job: JobItem) => void;
  getAlistName: (alistId: number) => string;
}

export default function HomeOverview({
  selectedJob,
  onRun,
  onEdit,
  onDelete,
  onToggle,
  getAlistName,
}: HomeOverviewProps) {
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const isEnabled = selectedJob.enable === 1;
  const fileSizeRange = formatFileSizeRange(selectedJob.minFileSize, selectedJob.maxFileSize);

  return (
    <TooltipProvider>
      <div className="space-y-5">
        {/* 顶部 Hero 卡片 */}
        <div
          className={cn(
            surface.card,
            'p-5 flex flex-col md:flex-row md:items-center justify-between gap-4'
          )}
        >
          <div className="flex items-start gap-4 min-w-0">
            <div className={cn(surface.iconTile, 'h-12 w-12')}>
              {isEnabled ? (
                <CheckCircle2 className={cn(icon.lg, 'text-emerald-600')} />
              ) : (
                <PauseCircle className={cn(icon.lg, 'text-slate-400')} />
              )}
            </div>

            <div className="space-y-2 min-w-0">
              <div className="flex items-center gap-2 flex-wrap">
                <h1 className={cn(text.pageTitle, 'truncate')}>
                  {getJobName(selectedJob)}
                </h1>
                <Badge variant={isEnabled ? 'success' : 'secondary'}>
                  {isEnabled ? '已启用' : '已暂停'}
                </Badge>
                <Badge variant="outline" className="bg-slate-50">
                  {methodNames[selectedJob.method] || selectedJob.method}
                </Badge>
                {selectedJob.isCron !== 2 && (
                  <Switch
                    checked={isEnabled}
                    onCheckedChange={() => onToggle(selectedJob)}
                    className="ml-1"
                    aria-label="切换启用状态"
                  />
                )}
              </div>

              {/* 统计小标签 */}
              <div className={cn('flex items-center gap-4 pt-0.5 flex-wrap', text.muted)}>
                <span className="flex items-center gap-1.5">
                  <Folder className={cn(icon.sm, 'text-slate-400')} />
                  源: <strong className="text-slate-700">{countJobPaths(selectedJob.srcPath)}</strong> 个
                </span>
                <span className="flex items-center gap-1.5">
                  <HardDrive className={cn(icon.sm, 'text-slate-400')} />
                  目标: <strong className="text-slate-700">{countJobPaths(selectedJob.dstPath)}</strong> 个
                </span>
                <span className="flex items-center gap-1.5">
                  <Clock className={cn(icon.sm, 'text-slate-400')} />
                  调度: <strong className="text-slate-700">{formatSchedule(selectedJob)}</strong>
                </span>
              </div>
            </div>
          </div>

          {/* 操作按钮组 */}
          <div className="flex items-center gap-2 shrink-0 flex-wrap">
            <Button onClick={() => onRun(selectedJob.id)}>
              <Play className={cn(icon.md, 'mr-1.5 fill-current')} />
              手动执行
            </Button>
            <Button variant="outline" onClick={() => onEdit(selectedJob)}>
              <Edit className={cn(icon.md, 'mr-1.5')} />
              编辑
            </Button>
            <Button
              variant="ghost"
              className="text-rose-600 hover:text-rose-700 hover:bg-rose-50"
              onClick={() => setDeleteDialogOpen(true)}
            >
              <Trash2 className={cn(icon.md, 'mr-1.5')} />
              删除
            </Button>
          </div>
        </div>

        {/* 任务详情网格 */}
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {/* 路径与引擎 */}
          <div className={cn(surface.card, surface.cardPadding, 'space-y-3.5')}>
            <SectionHeading title="存储与路径" icon={Server} bordered />

            <div className="space-y-3">
              <InfoRow label="所选引擎">{getAlistName(selectedJob.alistId)}</InfoRow>
              <InfoRow label="源目录路径" mono>
                {formatJobPaths(selectedJob.srcPath) || '—'}
              </InfoRow>
              <InfoRow label="目标目录路径" mono>
                {formatJobPaths(selectedJob.dstPath) || '—'}
              </InfoRow>
              <InfoRow label="缓存加速配置">
                {formatCache(selectedJob.useCacheS, selectedJob.useCacheT)}
              </InfoRow>
            </div>
          </div>

          {/* 过滤与调度 */}
          <div className={cn(surface.card, surface.cardPadding, 'space-y-3.5')}>
            <SectionHeading title="规则与调度" icon={Filter} bordered />

            <div className="space-y-3">
              <InfoRow label="执行方式">{formatSchedule(selectedJob)}</InfoRow>
              <InfoRow label="文件大小过滤">{fileSizeRange || '不限制大小'}</InfoRow>
              <InfoRow label="排除规则 (Ignore)">
                {selectedJob.exclude ? (
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <span className="font-mono text-teal-700 cursor-pointer underline underline-offset-2">
                        {formatExcludePreview(selectedJob.exclude)}
                      </span>
                    </TooltipTrigger>
                    <TooltipContent className="max-w-xs font-mono text-xs whitespace-pre-wrap">
                      {selectedJob.exclude}
                    </TooltipContent>
                  </Tooltip>
                ) : (
                  <span className="text-slate-400">无排除规则</span>
                )}
              </InfoRow>
              <InfoRow label="创建时间">
                {selectedJob.createTime
                  ? dayjs.unix(selectedJob.createTime).format('YYYY-MM-DD HH:mm')
                  : '—'}
              </InfoRow>
            </div>
          </div>
        </div>

        {/* 删除确认弹窗 */}
        <AlertDialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>确认删除此同步任务？</AlertDialogTitle>
              <AlertDialogDescription>
                任务删除后无法恢复，历史执行记录将一并清除。请确认是否继续。
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>取消</AlertDialogCancel>
              <AlertDialogAction variant="destructive" onClick={() => onDelete(selectedJob.id)}>
                确认删除
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </div>
    </TooltipProvider>
  );
}
