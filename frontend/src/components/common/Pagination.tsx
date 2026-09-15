import { ChevronLeft, ChevronRight } from 'lucide-react';
import { Button } from '../ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../ui/select';
import { cn } from '../../lib/utils';
import { control, icon, text } from '../../lib/styles';

const PAGE_SIZE_OPTIONS = [10, 20, 50, 100];

export interface PaginationProps {
  page: number;
  pageSize: number;
  total: number;
  onPageChange: (page: number) => void;
  /** 传入则显示每页条数选择器 */
  onPageSizeChange?: (pageSize: number) => void;
  /** 总数单位文案，如「条」「条记录」 */
  unit?: string;
  className?: string;
}

/** 完整分页器：总数 + 每页条数 + 上下页。列表页与详情页共用 */
export function Pagination({
  page,
  pageSize,
  total,
  onPageChange,
  onPageSizeChange,
  unit = '条',
  className,
}: PaginationProps) {
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  return (
    <div
      className={cn(
        'flex flex-col sm:flex-row items-center justify-between gap-3 pt-3',
        text.muted,
        className
      )}
    >
      <div className="flex items-center gap-2">
        <span>
          共 <span className="font-semibold text-slate-700">{total}</span> {unit}
        </span>
        {onPageSizeChange && (
          <Select
            value={String(pageSize)}
            onValueChange={(value) => onPageSizeChange(Number(value))}
          >
            <SelectTrigger className={cn(control.dense, 'w-28')} aria-label="每页条数">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {PAGE_SIZE_OPTIONS.map((size) => (
                <SelectItem key={size} value={String(size)}>
                  {size} 条/页
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      </div>
      <div className="flex items-center gap-2">
        <Button
          variant="outline"
          size="iconSm"
          onClick={() => onPageChange(page - 1)}
          disabled={page <= 1}
          aria-label="上一页"
        >
          <ChevronLeft className={icon.sm} />
        </Button>
        <span className="tabular-nums">
          {page} / {totalPages}
        </span>
        <Button
          variant="outline"
          size="iconSm"
          onClick={() => onPageChange(page + 1)}
          disabled={page >= totalPages}
          aria-label="下一页"
        >
          <ChevronRight className={icon.sm} />
        </Button>
      </div>
    </div>
  );
}

/** 紧凑分页器：文字按钮，用于侧边栏与实时明细等窄容器 */
export function CompactPagination({
  page,
  pageSize,
  total,
  onPageChange,
  className,
}: Omit<PaginationProps, 'onPageSizeChange' | 'unit'>) {
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  if (total <= pageSize) return null;
  return (
    <div className={cn('flex items-center justify-between gap-2', text.muted, className)}>
      <Button
        variant="outline"
        size="sm"
        className={cn(control.dense, 'px-2')}
        onClick={() => onPageChange(page - 1)}
        disabled={page <= 1}
      >
        上一页
      </Button>
      <span className="tabular-nums">
        {page} / {totalPages}
      </span>
      <Button
        variant="outline"
        size="sm"
        className={cn(control.dense, 'px-2')}
        onClick={() => onPageChange(page + 1)}
        disabled={page >= totalPages}
      >
        下一页
      </Button>
    </div>
  );
}
