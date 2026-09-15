import * as React from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '../../lib/utils';
import { badge } from '../../lib/styles';

/**
 * 徽标形状与尺寸来自 lib/styles 的 badge 配方，此处只做「语义色名 -> 色调」的映射。
 * variant 名保持与状态色体系一致：success / warning / error / processing 由 StatusBadge
 * 直接映射，default / secondary / outline 供页面按语义选用。
 */
export const badgeVariants = cva(badge.base, {
  variants: {
    variant: {
      default: badge.tone.brand,
      secondary: badge.tone.neutral,
      outline: badge.tone.outline,
      success: badge.tone.success,
      warning: badge.tone.warning,
      error: badge.tone.danger,
      processing: badge.tone.info,
    },
  },
  defaultVariants: {
    variant: 'default',
  },
});

export interface BadgeProps
  extends React.HTMLAttributes<HTMLSpanElement>,
    VariantProps<typeof badgeVariants> {}

export function Badge({ className, variant, ...props }: BadgeProps) {
  // 用 span 承载：徽标常被放进段落或 StatusBadge 的 span 容器里，div 会造成非法嵌套
  return <span className={cn(badgeVariants({ variant }), className)} {...props} />;
}
