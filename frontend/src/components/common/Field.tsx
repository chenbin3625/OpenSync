import { cloneElement, isValidElement, useId } from 'react';
import { HelpCircle } from 'lucide-react';
import { Tooltip } from '../ui/tooltip';
import { cn } from '../../lib/utils';
import { icon, layout, text } from '../../lib/styles';

export interface FieldProps {
  label: string;
  /** 必填标记 */
  required?: boolean;
  /** 标签旁的问号提示，可传富文本 */
  tooltip?: React.ReactNode;
  /** 控件下方的说明文字 */
  hint?: string;
  /** 字段级校验错误，优先于 hint 展示 */
  error?: string;
  /** 控件右侧的后缀，可传单位文案或单位下拉等控件 */
  suffix?: React.ReactNode;
  /** 后缀容器类名，用于自定义后缀宽度等样式 */
  suffixClassName?: string;
  className?: string;
  children: React.ReactNode;
}

/**
 * 统一的表单字段外壳：标签 + 控件 + 提示 / 错误。
 *
 * 通过 htmlFor / id 关联标签与控件（children 为函数时可拿到 id 自行透传）。
 */
export function Field({
  label,
  required = false,
  tooltip,
  hint,
  error,
  suffix,
  suffixClassName,
  className,
  children,
}: FieldProps) {
  const fieldId = useId();
  return (
    <div className={cn(layout.field, className)}>
      <div className="flex items-center gap-1.5">
        <label htmlFor={fieldId} className={text.label}>
          {label}
          {required && (
            <span className="text-rose-600" aria-hidden="true">
              {' '}
              *
            </span>
          )}
        </label>
        {tooltip && (
          <Tooltip title={tooltip}>
            <button
              type="button"
              className="text-slate-400 hover:text-slate-600 cursor-help"
              aria-label={`${label}说明`}
            >
              <HelpCircle className={icon.sm} />
            </button>
          </Tooltip>
        )}
      </div>
      {suffix ? (
        <div className="flex items-center gap-2">
          <div className="flex-1 min-w-0">
            <FieldControl id={fieldId}>{children}</FieldControl>
          </div>
          {typeof suffix === 'string' ? (
            <span className={cn(text.muted, 'w-8 shrink-0 text-left whitespace-nowrap', suffixClassName)}>
              {suffix}
            </span>
          ) : (
            <div className={cn('shrink-0', suffixClassName)}>{suffix}</div>
          )}
        </div>
      ) : (
        <FieldControl id={fieldId}>{children}</FieldControl>
      )}
      {error ? (
        <p className="text-2xs text-rose-600">{error}</p>
      ) : (
        hint && <p className={text.hint}>{hint}</p>
      )}
    </div>
  );
}

/** 把 label 的 id 透传给单个控件子元素，避免调用方手写 id */
function FieldControl({ id, children }: { id: string; children: React.ReactNode }) {
  if (isValidElement<{ id?: string }>(children) && !children.props.id) {
    return cloneElement(children, { id });
  }
  return <>{children}</>;
}
