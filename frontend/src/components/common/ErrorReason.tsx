import { AlertCircle } from 'lucide-react';
import { Tooltip } from '../ui/tooltip';
import { badge, icon } from '../../lib/styles';
import { cn } from '../../lib/utils';

export interface ErrorReasonProps {
  /** 服务端返回的失败原因；为空时不渲染任何入口 */
  errMsg?: string | null;
  /** 是否在图标后附带「失败原因」文案：宽行使用，窄栅格（表格单元格）省略 */
  label?: string;
  className?: string;
}

/**
 * 失败原因入口 + 悬浮卡片。任务列表、任务详情表格与实时明细共用同一套。
 *
 * 约定：
 * - 入口统一是按钮（可聚焦、可 Tab 到），不再用 span 冒充交互元素；
 * - 带动文案时复用标签配方（danger 色调），与页面其它标签同形同色，
 *   不再是一段裸的红字；
 * - 卡片文案用默认字体：失败原因以中文为主，等宽字体在中文上会退化成
 *   字宽不一的回退字体，整段看起来是坏的；
 * - 长文本的限高与滚动由 ui/tooltip 统一处理，这里只负责内容本身。
 * - 只给 aria-label，不给 title：入口同时是 Radix 的 TooltipTrigger，
 *   再挂一个原生 title 会在悬浮时叠出第二个系统气泡，盖住真正要看的失败原因。
 */
export function ErrorReason({ errMsg, label, className }: ErrorReasonProps) {
  const message = typeof errMsg === 'string' ? errMsg.trim() : '';
  if (!message) return null;

  return (
    <Tooltip title={message}>
      <button
        type="button"
        aria-label="查看失败原因"
        className={cn(
          'cursor-pointer',
          label
            ? cn(badge.base, badge.tone.danger, 'hover:bg-rose-200')
            : 'text-rose-500 hover:text-rose-700 shrink-0',
          className
        )}
      >
        <AlertCircle className={icon.sm} />
        {label ? <span>{label}</span> : null}
      </button>
    </Tooltip>
  );
}
