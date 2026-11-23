import * as React from 'react';
import * as ProgressPrimitive from '@radix-ui/react-progress';
import { cn } from '../../lib/utils';

export const Progress = React.forwardRef<
  React.ElementRef<typeof ProgressPrimitive.Root>,
  React.ComponentPropsWithoutRef<typeof ProgressPrimitive.Root>
>(({ className, value, ...props }, ref) => {
  const clampedValue =
    typeof value === 'number' && Number.isFinite(value)
      ? Math.min(100, Math.max(0, value))
      : null;

  return (
    <ProgressPrimitive.Root
      ref={ref}
      value={clampedValue}
      className={cn('relative h-1.5 w-full overflow-hidden rounded-full bg-tint border border-line-soft', className)}
      {...props}
    >
      <ProgressPrimitive.Indicator
        className="h-full w-full flex-1 bg-teal-600 transition-all duration-300 ease-out"
        style={{ transform: `translateX(-${100 - (clampedValue ?? 0)}%)` }}
      />
    </ProgressPrimitive.Root>
  );
});
Progress.displayName = ProgressPrimitive.Root.displayName;
