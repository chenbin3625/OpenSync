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
        <div className="bg-white rounded-xl border border-slate-200/90 shadow-xs p-6 flex flex-col md:flex-row md:items-center justify-between gap-6">
          <div className="flex items-start gap-4">
            <div className="h-12 w-12 rounded-xl bg-teal-50 text-teal-700 border border-teal-100 flex items-center justify-center shrink-0">
              {isEnabled ? (
                <CheckCircle2 className="h-6 w-6 text-emerald-600" />
              ) : (
                <PauseCircle className="h-6 w-6 text-slate-400" />
              )}
            </div>

            <div className="space-y-2">
              <div className="flex items-center gap-2.5 flex-wrap">
                <h1 className="text-xl font-bold text-slate-900 tracking-tight truncate">
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
              <div className="flex items-center gap-4 text-xs text-slate-500 pt-0.5 flex-wrap">
                <span className="flex items-center gap-1.5">
                  <Folder className="h-3.5 w-3.5 text-slate-400" />
                  源: <strong className="text-slate-700">{countJobPaths(selectedJob.srcPath)}</strong> 个
                </span>
                <span className="flex items-center gap-1.5">
                  <HardDrive className="h-3.5 w-3.5 text-slate-400" />
                  目标: <strong className="text-slate-700">{countJobPaths(selectedJob.dstPath)}</strong> 个
                </span>
                <span className="flex items-center gap-1.5">
                  <Clock className="h-3.5 w-3.5 text-slate-400" />
                  调度: <strong className="text-slate-700">{formatSchedule(selectedJob)}</strong>
                </span>
              </div>
            </div>
          </div>

          {/* 操作按钮组 */}
          <div className="flex items-center gap-2 shrink-0 flex-wrap">
            <Button
              onClick={() => onRun(selectedJob.id)}
              className="shadow-xs"
            >
              <Play className="h-4 w-4 mr-1.5 fill-current" />
              手动执行
            </Button>
            <Button
              variant="outline"
              onClick={() => onEdit(selectedJob)}
            >
              <Edit className="h-4 w-4 mr-1.5" />
              编辑
            </Button>
            <Button
              variant="ghost"
              className="text-red-600 hover:text-red-700 hover:bg-red-50"
              onClick={() => setDeleteDialogOpen(true)}
            >
              <Trash2 className="h-4 w-4 mr-1.5" />
              删除
            </Button>
          </div>
        </div>

        {/* 任务详情网格 */}
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {/* 路径与引擎 */}
          <div className="bg-white rounded-xl border border-slate-200/80 p-5 space-y-3.5 shadow-xs">
            <div className="flex items-center gap-2 pb-2.5 border-b border-slate-100 font-semibold text-sm text-slate-900">
              <Server className="h-4 w-4 text-teal-700" />
              <span>存储与路径</span>
            </div>

            <div className="space-y-3 text-xs">
              <div className="flex items-start justify-between gap-4">
                <span className="text-slate-400 shrink-0">所选引擎</span>
                <span className="font-medium text-slate-800 text-right">{getAlistName(selectedJob.alistId)}</span>
              </div>

              <div className="flex items-start justify-between gap-4">
                <span className="text-slate-400 shrink-0">源目录路径</span>
                <span className="font-mono text-slate-700 text-right break-all max-w-[70%]">
                  {formatJobPaths(selectedJob.srcPath) || '—'}
                </span>
              </div>

              <div className="flex items-start justify-between gap-4">
                <span className="text-slate-400 shrink-0">目标目录路径</span>
                <span className="font-mono text-slate-700 text-right break-all max-w-[70%]">
                  {formatJobPaths(selectedJob.dstPath) || '—'}
                </span>
              </div>

              <div className="flex items-start justify-between gap-4">
                <span className="text-slate-400 shrink-0">缓存加速配置</span>
                <span className="text-slate-700">{formatCache(selectedJob.useCacheS, selectedJob.useCacheT)}</span>
              </div>
            </div>
          </div>

          {/* 过滤与调度 */}
          <div className="bg-white rounded-xl border border-slate-200/80 p-5 space-y-3.5 shadow-xs">
            <div className="flex items-center gap-2 pb-2.5 border-b border-slate-100 font-semibold text-sm text-slate-900">
              <Filter className="h-4 w-4 text-teal-700" />
              <span>规则与调度</span>
            </div>

            <div className="space-y-3 text-xs">
              <div className="flex items-start justify-between gap-4">
                <span className="text-slate-400 shrink-0">执行方式</span>
                <span className="font-medium text-slate-800">{formatSchedule(selectedJob)}</span>
              </div>

              <div className="flex items-start justify-between gap-4">
                <span className="text-slate-400 shrink-0">文件大小过滤</span>
                <span className="text-slate-700">{fileSizeRange || '不限制大小'}</span>
              </div>

              <div className="flex items-start justify-between gap-4">
                <span className="text-slate-400 shrink-0">排除规则 (Ignore)</span>
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
              </div>

              <div className="flex items-start justify-between gap-4">
                <span className="text-slate-400 shrink-0">创建时间</span>
                <span className="text-slate-600">
                  {selectedJob.createTime ? dayjs.unix(selectedJob.createTime).format('YYYY-MM-DD HH:mm') : '—'}
                </span>
              </div>
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
