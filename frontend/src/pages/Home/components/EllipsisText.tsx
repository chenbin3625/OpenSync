import type { ReactNode } from 'react';
import { Tooltip } from '../../../components/ui/tooltip';
import { displayText } from '../homeUtils';
import { cn } from '../../../lib/utils';

export default function EllipsisText({
  value,
  tooltip,
  type,
  className,
  maxWidth,
}: {
  value: string | number | null | undefined;
  tooltip?: ReactNode;
  type?: 'secondary' | 'danger';
  className?: string;
  maxWidth?: number;
}) {
  const text = displayText(value);
  if (text === '--') return <span className={cn('text-slate-400 text-xs', className)}>--</span>;

  return (
    <Tooltip title={tooltip || text}>
      <span
        className={cn(
          'truncate inline-block align-bottom cursor-default',
          type === 'secondary' && 'text-slate-500',
          type === 'danger' && 'text-rose-600',
          className
        )}
        style={maxWidth ? { maxWidth } : undefined}
      >
        {text}
      </span>
    </Tooltip>
  );
}
