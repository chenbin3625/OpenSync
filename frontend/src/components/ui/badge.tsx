import * as React from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '../../lib/utils';

export const badgeVariants = cva(
  'inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold transition-colors focus:outline-none focus:ring-2 focus:ring-offset-2',
  {
    variants: {
      variant: {
        default: 'border-transparent bg-teal-700 text-white shadow hover:bg-teal-800',
        secondary: 'border-transparent bg-slate-100 text-slate-900 hover:bg-slate-200',
        destructive: 'border-transparent bg-rose-600 text-white shadow hover:bg-rose-700',
        outline: 'text-slate-800 border-slate-300',
        success: 'border-transparent bg-emerald-100 text-emerald-800 border border-emerald-200',
        warning: 'border-transparent bg-amber-100 text-amber-800 border border-amber-200',
        error: 'border-transparent bg-rose-100 text-rose-800 border border-rose-200',
        processing: 'border-transparent bg-blue-100 text-blue-800 border border-blue-200',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  }
);

export interface BadgeProps
  extends React.HTMLAttributes<HTMLDivElement>,
    VariantProps<typeof badgeVariants> {}

export function Badge({ className, variant, ...props }: BadgeProps) {
  return <div className={cn(badgeVariants({ variant }), className)} {...props} />;
}
