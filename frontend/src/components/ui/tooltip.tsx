import * as React from 'react';
import * as TooltipPrimitive from '@radix-ui/react-tooltip';
import { cn } from '../../lib/utils';

export const TooltipProvider = TooltipPrimitive.Provider;
export const TooltipRoot = TooltipPrimitive.Root;
export const TooltipTrigger = TooltipPrimitive.Trigger;

export const TooltipContent = React.forwardRef<
  React.ElementRef<typeof TooltipPrimitive.Content>,
  React.ComponentPropsWithoutRef<typeof TooltipPrimitive.Content>
>(({ className, sideOffset = 4, ...props }, ref) => (
  <TooltipPrimitive.Content
    ref={ref}
    sideOffset={sideOffset}
    // 与视口边缘留出余量，避免卡片贴边时被裁掉半个字
    collisionPadding={8}
    className={cn(
      // 限高 + 可滚动：悬浮卡片经常用来展示服务端返回的长文本（失败原因、重试记录），
      // 不限高时卡片会长到顶出视口上方，被切掉的开头既看不到也滚不到，
      // 所以这里不能整体裁掉溢出内容
      'z-50 max-h-[min(60vh,20rem)] overflow-y-auto overscroll-contain rounded-md bg-slate-900 px-3 py-1.5 text-xs text-slate-50 shadow-md',
      // 长文本保留服务端换行，并在超长不分词的内容上强制折行
      'max-w-xs whitespace-pre-wrap break-words',
      'animate-in fade-in-0 zoom-in-95 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95 data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2',
      className
    )}
    {...props}
  />
));
TooltipContent.displayName = TooltipPrimitive.Content.displayName;

/**
 * 同时支持两种写法：
 *
 * 1. 简写（title/content 直接传文案，内部自动补 Trigger 与 Content）：
 *    <Tooltip title="说明"><Icon /></Tooltip>
 *
 * 2. 组合式（由调用方自己写 TooltipTrigger / TooltipContent）：
 *    <Tooltip><TooltipTrigger asChild>…</TooltipTrigger><TooltipContent>…</TooltipContent></Tooltip>
 *
 * 未传 title/content 时必须仍然渲染 Radix Root，否则子层的 TooltipTrigger
 * 找不到 context，会直接抛错并让整个页面白屏。
 */
export function Tooltip({
  title,
  content,
  children,
  className,
}: {
  title?: React.ReactNode;
  content?: React.ReactNode;
  children: React.ReactNode;
  className?: string;
}) {
  const tip = title ?? content;
  return (
    <TooltipProvider delayDuration={200}>
      <TooltipPrimitive.Root>
        {tip == null ? (
          children
        ) : (
          <>
            <TooltipPrimitive.Trigger asChild>{children}</TooltipPrimitive.Trigger>
            <TooltipContent className={className}>{tip}</TooltipContent>
          </>
        )}
      </TooltipPrimitive.Root>
    </TooltipProvider>
  );
}
