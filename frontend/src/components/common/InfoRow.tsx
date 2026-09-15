import { cn } from '../../lib/utils';
import { surface, text } from '../../lib/styles';

export interface InfoRowProps {
  /** 键名 */
  label: string;
  /** 值，传 ReactNode 以支持徽标、按钮等 */
  children: React.ReactNode;
  /** 值是否等宽显示（路径、ID 等） */
  mono?: boolean;
  className?: string;
}

/** 键值行：左键名右值，键名固定不收缩。替代各页面手写的 flex justify-between 组合 */
export function InfoRow({ label, children, mono = false, className }: InfoRowProps) {
  return (
    <div className={cn('flex items-start justify-between gap-4', className)}>
      <span className={cn(text.subtle, 'shrink-0')}>{label}</span>
      <span
        className={cn(
          'min-w-0 text-right',
          mono ? cn(text.mono, 'break-all') : 'text-xs text-slate-600'
        )}
      >
        {children}
      </span>
    </div>
  );
}

/** 键值行的承载面板（卡片内的灰色内嵌区） */
export function InfoPanel({
  children,
  className,
}: {
  children: React.ReactNode;
  className?: string;
}) {
  return <div className={cn(surface.inset, 'space-y-1.5', className)}>{children}</div>;
}
