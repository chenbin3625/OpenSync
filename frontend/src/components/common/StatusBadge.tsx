import { Info } from 'lucide-react';
import { Badge } from '../ui/badge';
import { Tooltip } from '../ui/tooltip';
import { cn } from '../../lib/utils';
import { icon } from '../../lib/styles';

/** homeUtils 里的状态色名到 Badge variant 的映射 */
const variantByColor = {
  success: 'success',
  error: 'error',
  warning: 'warning',
  processing: 'processing',
  default: 'secondary',
} as const;

export type StatusColor = keyof typeof variantByColor;

export interface StatusBadgeProps {
  /** 状态文案 */
  label: string;
  /** homeUtils 中的状态色名 */
  color?: string;
  /** 失败原因，存在时在徽标右侧显示可悬浮的问号 */
  errMsg?: string;
  className?: string;
}

/** 状态徽标 + 失败原因提示。任务列表与任务详情共用 */
export function StatusBadge({ label, color = 'default', errMsg, className }: StatusBadgeProps) {
  const variant = variantByColor[color as StatusColor] ?? 'secondary';
  return (
    <span className={cn('inline-flex items-center gap-1 max-w-full', className)}>
      <Badge variant={variant}>{label}</Badge>
      {errMsg && (
        <Tooltip title={errMsg}>
          <button
            type="button"
            aria-label="查看失败原因"
            className="text-rose-500 hover:text-rose-700 cursor-pointer shrink-0"
          >
            <Info className={icon.sm} />
          </button>
        </Tooltip>
      )}
    </span>
  );
}
