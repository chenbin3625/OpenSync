import { useId } from 'react';
import { Switch } from '../ui/switch';
import { cn } from '../../lib/utils';
import { surface, text } from '../../lib/styles';

export interface SwitchRowProps {
  label: string;
  description?: string;
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  disabled?: boolean;
  className?: string;
}

/** 开关行：左侧标题 + 说明，右侧开关。统一内嵌面板样式并绑定 label */
export function SwitchRow({
  label,
  description,
  checked,
  onCheckedChange,
  disabled = false,
  className,
}: SwitchRowProps) {
  const id = useId();
  return (
    <div
      className={cn(surface.inset, 'flex items-center justify-between gap-3', className)}
    >
      <div className="min-w-0 space-y-0.5">
        <label htmlFor={id} className={cn(text.label, 'cursor-pointer')}>
          {label}
        </label>
        {description && <p className={text.hint}>{description}</p>}
      </div>
      <Switch
        id={id}
        checked={checked}
        onCheckedChange={onCheckedChange}
        disabled={disabled}
        className="shrink-0"
      />
    </div>
  );
}
