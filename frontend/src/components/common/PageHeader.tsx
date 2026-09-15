import type { LucideIcon } from 'lucide-react';
import { cn } from '../../lib/utils';
import { icon, layout, surface, text } from '../../lib/styles';

export interface PageHeaderProps {
  title: string;
  subtitle?: string;
  /** 右侧操作区 */
  actions?: React.ReactNode;
  className?: string;
}

/** 页面级页头：标题 + 副标题 + 操作区，窄屏堆叠。三个配置页与任务页共用 */
export function PageHeader({ title, subtitle, actions, className }: PageHeaderProps) {
  return (
    <div className={cn(layout.pageHeader, className)}>
      <div className="space-y-1 min-w-0">
        <h1 className={text.pageTitle}>{title}</h1>
        {subtitle && <p className={text.pageSubtitle}>{subtitle}</p>}
      </div>
      {actions && <div className={layout.pageHeaderActions}>{actions}</div>}
    </div>
  );
}

export interface SectionHeadingProps {
  title: string;
  icon?: LucideIcon;
  /** 右侧附加内容（计数、操作） */
  extra?: React.ReactNode;
  /** 是否显示下边框（卡片内分区时使用） */
  bordered?: boolean;
  className?: string;
}

/** 卡片 / 区块标题：图标 + 标题，可选下边框 */
export function SectionHeading({
  title,
  icon: Icon,
  extra,
  bordered = false,
  className,
}: SectionHeadingProps) {
  return (
    <div
      className={cn(
        'flex items-center gap-2',
        bordered && cn('pb-2.5 border-b', surface.divider),
        className
      )}
    >
      {Icon && <Icon className={cn(icon.md, 'text-teal-700 shrink-0')} aria-hidden="true" />}
      <h2 className={cn(text.sectionTitle, 'min-w-0 truncate')}>{title}</h2>
      {extra && <div className="ml-auto shrink-0">{extra}</div>}
    </div>
  );
}
