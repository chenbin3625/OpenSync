import { useMemo, useState, useEffect, type ReactNode } from 'react';
import {
  Clock, Gauge, FolderSearch, StopCircle, Zap
} from 'lucide-react';
import dayjs from 'dayjs';
import type { CurrentTaskView } from '../../../types';
import { canPollCurrentDocument } from '../pollingVisibility';
import { formatDuration, formatSize } from '../homeUtils';
import { Button } from '../../../components/ui/button';
import { Badge } from '../../../components/ui/badge';
import { cn } from '../../../lib/utils';
import { icon, surface, text } from '../../../lib/styles';

export type ProgressMetric = {
  key: string;
  label: string;
  icon?: ReactNode;
  value: string;
};

export interface TaskRealtimeHeroProps {
  currentTask: CurrentTaskView;
  onStop: () => void;
  children?: ReactNode;
}

export default function TaskRealtimeHero({
  currentTask,
  onStop,
  children,
}: TaskRealtimeHeroProps) {
  const [nowTick, setNowTick] = useState(() => Math.floor(Date.now() / 1000));
  const realtimeTotal = Object.values(currentTask.num || {})
    .reduce((sum, value) => sum + Number(value || 0), 0);
  const scanProgress = currentTask.scan;
  const displayDuration = Math.max(
    currentTask.duration || 0,
    currentTask.createTime ? nowTick - currentTask.createTime : 0,
  );

  const metrics = useMemo<ProgressMetric[]>(() => [
    {
      key: 'duration',
      label: '耗时',
      icon: <Clock className={cn(icon.sm, 'text-slate-400')} />,
      value: formatDuration(displayDuration),
    },
    {
      key: 'speedAvg',
      label: '平均速度',
      icon: <Gauge className={cn(icon.sm, 'text-slate-400')} />,
      value: currentTask.speedAvg > 0 ? `${formatSize(currentTask.speedAvg)}/s` : '--',
    },
    {
      key: 'speed',
      label: '瞬时速度',
      icon: <Zap className={cn(icon.sm, 'text-slate-400')} />,
      value: currentTask.speed > 0 ? `${formatSize(currentTask.speed)}/s` : '--',
    },
    {
      key: 'remainTime',
      label: '预计剩余',
      value: currentTask.remainTime > 0 ? formatDuration(currentTask.remainTime) : '--',
    },
    {
      key: 'doneSize',
      label: '已传输',
      value: formatSize(currentTask.doneSize || 0),
    },
    {
      key: 'remainSize',
      label: '剩余数据',
      value: formatSize(currentTask.remainSize || 0),
    },
  ], [currentTask.doneSize, currentTask.remainSize, currentTask.remainTime, currentTask.speed, currentTask.speedAvg, displayDuration]);

  useEffect(() => {
    const tickID = setInterval(() => {
      if (canPollCurrentDocument()) {
        setNowTick(Math.floor(Date.now() / 1000));
      }
    }, 1000);
    return () => { clearInterval(tickID); };
  }, []);

  return (
    <div className={cn(surface.card, surface.cardPadding, 'space-y-5')}>
      {/* 头部标题与状态 */}
      <div
        className={cn(
          'flex flex-col sm:flex-row sm:items-center justify-between gap-4 pb-4 border-b',
          surface.divider
        )}
      >
        <div className="space-y-1">
          {/* 标题行固定单行：标题过长时省略，状态标签始终完整可见 */}
          <div className="flex items-center gap-2 min-w-0">
            <div className={cn(surface.iconTile, 'h-8 w-8')}>
              <Zap className={icon.md} />
            </div>
            <h2 className={cn(text.sectionTitle, 'truncate min-w-0')}>
              任务 #{currentTask.taskId} 实时运行状态
            </h2>
            <Badge variant={currentTask.scanFinish ? 'success' : 'processing'}>
              {currentTask.scanFinish ? '扫描完成，同步中' : '目录扫描中'}
            </Badge>
          </div>

          <div className={cn('flex items-center gap-3 pt-1 flex-wrap', text.muted)}>
            <span>开始: {currentTask.createTime ? dayjs.unix(currentTask.createTime).format('HH:mm:ss') : '--'}</span>
            <span>•</span>
            <span>总计: <strong className="text-slate-700">{realtimeTotal}</strong> 项</span>
            <span>•</span>
            <span>已完成: <strong className="text-emerald-600">{currentTask.num?.success || 0}</strong> 项</span>
            <span>•</span>
            <span>失败: <strong className="text-rose-600">{currentTask.num?.fail || 0}</strong> 项</span>
          </div>
        </div>

        <Button
          variant="danger"
          size="sm"
          onClick={onStop}
        >
          <StopCircle className={cn(icon.md, 'mr-1.5')} />
          中止任务
        </Button>
      </div>

      {/* 目录扫描指示 */}
      {scanProgress && (
        <div className={cn(surface.inset, 'flex items-center justify-between text-xs')}>
          <div className="flex items-center gap-2 font-medium text-slate-700 whitespace-nowrap">
            <FolderSearch className={cn(icon.md, 'text-teal-700')} />
            <span>目录树扫描进度</span>
          </div>
          <span className="text-slate-500 font-mono">已探索 {scanProgress.totalDirs} 个目录</span>
        </div>
      )}

      {/* 核心指标卡片矩阵 */}
      <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-3">
        {metrics.map((item) => (
          <div
            key={item.key}
            className={cn(surface.inset, 'flex flex-col justify-between space-y-1 min-w-0')}
          >
            <div className="flex items-center gap-1.5 text-2xs text-slate-400 font-medium whitespace-nowrap">
              {item.icon}
              <span>{item.label}</span>
            </div>
            <div className={cn(text.stat, 'truncate')}>{item.value}</div>
          </div>
        ))}
      </div>

      {/* 子内容（Tab 与文件列表） */}
      {children}
    </div>
  );
}
