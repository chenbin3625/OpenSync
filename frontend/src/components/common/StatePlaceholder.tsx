import type { LucideIcon } from 'lucide-react';
import { RefreshCw } from 'lucide-react';
import { Button } from '../ui/button';
import { cn } from '../../lib/utils';
import { icon, surface, text } from '../../lib/styles';

export interface EmptyStateProps {
  icon: LucideIcon;
  /** 空状态文案 */
  title: string;
  /** sm 用于侧边栏、卡片内的小区域；md 用于整页占位 */
  size?: 'sm' | 'md';
  className?: string;
}

/** 统一的空状态占位：图标 + 文案，替代各页面手写的 py-16 text-center 组合 */
export function EmptyState({ icon: Icon, title, size = 'md', className }: EmptyStateProps) {
  const compact = size === 'sm';
  return (
    <div
      className={cn(
        'text-center space-y-2',
        compact ? 'py-8' : 'py-16',
        className
      )}
    >
      <Icon
        className={cn('mx-auto text-slate-300', compact ? icon.lg : icon.placeholder)}
        aria-hidden="true"
      />
      <p className={compact ? text.subtle : text.muted}>{title}</p>
    </div>
  );
}

export interface ErrorStateProps {
  /** 错误文案 */
  title: string;
  icon: LucideIcon;
  onRetry?: () => void;
  retryLabel?: string;
  loading?: boolean;
  className?: string;
}

/** 统一的错误态占位：圆形图标 + 文案 + 重试按钮 */
export function ErrorState({
  title,
  icon: Icon,
  onRetry,
  retryLabel = '重新加载',
  loading = false,
  className,
}: ErrorStateProps) {
  return (
    <div className={cn('py-16 text-center space-y-3', className)}>
      <div className="inline-flex h-12 w-12 items-center justify-center rounded-full bg-rose-50 text-rose-600">
        <Icon className="h-6 w-6" aria-hidden="true" />
      </div>
      <p className="text-sm font-medium text-slate-600">{title}</p>
      {onRetry && (
        <div>
          <Button
            variant="outline"
            size="sm"
            className="gap-1.5"
            onClick={onRetry}
            disabled={loading}
          >
            <RefreshCw className={cn(icon.sm, loading && 'animate-spin')} aria-hidden="true" />
            {retryLabel}
          </Button>
        </div>
      )}
    </div>
  );
}

/** 空状态 / 错误态外层卡片，供整页占位使用 */
export function PlaceholderCard({
  children,
  className,
}: {
  children: React.ReactNode;
  className?: string;
}) {
  return <div className={cn(surface.card, className)}>{children}</div>;
}
