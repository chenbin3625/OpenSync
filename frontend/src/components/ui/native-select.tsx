import * as React from 'react';
import { cn } from '../../lib/utils';

export type NativeSelectProps = React.SelectHTMLAttributes<HTMLSelectElement>;

/**
 * 原生下拉框。用于表单里需要原生 change 事件 / 受控 value 的场景，
 * 视觉与 Input、ui/select 的 SelectTrigger 保持一致（含焦点环）。
 */
export const NativeSelect = React.forwardRef<HTMLSelectElement, NativeSelectProps>(
  ({ className, ...props }, ref) => (
    <select
      className={cn(
        'flex h-9 w-full rounded-md border border-slate-300 bg-white px-3 py-1 text-sm text-slate-800 shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-teal-600 focus-visible:border-teal-600 disabled:cursor-not-allowed disabled:opacity-50',
        className
      )}
      ref={ref}
      {...props}
    />
  )
);
NativeSelect.displayName = 'NativeSelect';
