import type { PaginationProps } from './Pagination';
import { Button } from '../ui/button';
import { cn } from '../../lib/utils';
import { text } from '../../lib/styles';

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
        onClick={() => onPageChange(page + 1)}
        disabled={page >= totalPages}
      >
        下一页
      </Button>
    </div>
  );
}
