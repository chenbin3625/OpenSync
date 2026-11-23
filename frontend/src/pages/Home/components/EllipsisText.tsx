import type { CSSProperties, ReactNode } from 'react';
import { Tooltip } from '../../../components/ui/tooltip';
import { displayText } from '../homeUtils';
import { cn } from '../../../lib/utils';
import { text as textStyles } from '../../../lib/styles';

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
  if (text === '--') return <span className={cn(textStyles.subtle, className)}>--</span>;

  return (
    <Tooltip title={tooltip || text}>
      <span
        className={cn(
          'block min-w-0 wrap-anywhere cursor-default xl:inline-block xl:max-w-(--detail-path-width) xl:truncate xl:align-bottom',
          type === 'secondary' && 'text-slate-500',
          type === 'danger' && 'text-rose-600',
          className
        )}
        style={maxWidth ? { '--detail-path-width': `${maxWidth}px` } as CSSProperties : undefined}
      >
        {text}
      </span>
    </Tooltip>
  );
}
