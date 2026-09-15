import { AlertCircle, CheckCircle2, Info, AlertTriangle } from 'lucide-react';
import { cn } from '../../lib/utils';
import { icon } from '../../lib/styles';

const variants = {
  error: {
    surface: 'bg-rose-50 border-rose-200 text-rose-700',
    icon: AlertCircle,
  },
  warning: {
    surface: 'bg-amber-50 border-amber-200 text-amber-700',
    icon: AlertTriangle,
  },
  success: {
    surface: 'bg-emerald-50 border-emerald-200 text-emerald-700',
    icon: CheckCircle2,
  },
  info: {
    surface: 'bg-line-soft border-line text-slate-700',
    icon: Info,
  },
} as const;

export interface AlertProps {
  variant?: keyof typeof variants;
  children: React.ReactNode;
  className?: string;
}

/** 表单 / 页面级提示条。统一 p-3 rounded-lg + 图标，替代散落的 p-2.5 rounded 变体 */
export function Alert({ variant = 'error', children, className }: AlertProps) {
  const { surface, icon: Icon } = variants[variant];
  return (
    <div
      role={variant === 'error' ? 'alert' : 'status'}
      className={cn(
        'flex items-center gap-2 p-3 rounded-lg border text-xs font-medium',
        surface,
        className
      )}
    >
      <Icon className={cn(icon.md, 'shrink-0')} aria-hidden="true" />
      <span className="min-w-0">{children}</span>
    </div>
  );
}
