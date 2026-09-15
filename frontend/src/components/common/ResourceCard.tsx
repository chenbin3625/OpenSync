import type { LucideIcon } from 'lucide-react';
import { cn } from '../../lib/utils';
import { icon, surface, text } from '../../lib/styles';

export interface ResourceCardProps {
  icon: LucideIcon;
  title: string;
  subtitle?: string;
  /** 标题行右侧内容，通常是状态徽标 */
  badge?: React.ReactNode;
  /** 卡片主体（键值面板等） */
  children?: React.ReactNode;
  /** 底部操作区 */
  actions?: React.ReactNode;
  className?: string;
}

/** 资源卡片：引擎、通知渠道等列表项共用。图标底座 + 标题 + 主体 + 底部操作 */
export function ResourceCard({
  icon: Icon,
  title,
  subtitle,
  badge,
  children,
  actions,
  className,
}: ResourceCardProps) {
  return (
    <div
      className={cn(
        surface.card,
        surface.cardPadding,
        surface.cardHover,
        'flex flex-col justify-between gap-4 h-full',
        className
      )}
    >
      <div className="space-y-4 min-w-0">
        <div className="flex items-start gap-3">
          <div className={cn(surface.iconTile, 'h-9 w-9')}>
            <Icon className={icon.md} aria-hidden="true" />
          </div>
          <div className="min-w-0 flex-1 space-y-0.5">
            <h2 className={cn(text.sectionTitle, 'truncate')}>{title}</h2>
            {subtitle && <p className={cn(text.subtle, 'truncate')}>{subtitle}</p>}
          </div>
          {badge && <div className="shrink-0">{badge}</div>}
        </div>
        {children}
      </div>
      {actions && (
        <div className={cn('flex items-center gap-1.5 pt-3 border-t', surface.divider)}>
          {actions}
        </div>
      )}
    </div>
  );
}
