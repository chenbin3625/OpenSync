import { Search, X } from 'lucide-react';
import { Input } from '../ui/input';
import { cn } from '../../lib/utils';
import { control, icon } from '../../lib/styles';

export interface SearchInputProps {
  value: string;
  onChange: (value: string) => void;
  /** 回车或点击搜索时触发 */
  onSearch?: () => void;
  onClear?: () => void;
  placeholder?: string;
  /** 容器宽度 class，默认 w-48 */
  width?: string;
  className?: string;
}

/** 带前缀图标与清除按钮的搜索框。列表页与详情页筛选条共用 */
export function SearchInput({
  value,
  onChange,
  onSearch,
  onClear,
  placeholder = '搜索',
  width = 'w-48',
  className,
}: SearchInputProps) {
  const handleClear = () => {
    onChange('');
    onClear?.();
  };

  return (
    <div className={cn('relative', width, className)}>
      <Input
        value={value}
        onChange={(event) => onChange(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === 'Enter') onSearch?.();
        }}
        placeholder={placeholder}
        aria-label={placeholder}
        className={cn(control.compact, 'pr-8')}
        prefixIcon={<Search className={cn(icon.sm, 'text-slate-400')} />}
      />
      {value && (
        <button
          type="button"
          onClick={handleClear}
          aria-label="清除搜索"
          className="absolute inset-y-0 right-2 flex items-center text-slate-400 hover:text-slate-600"
        >
          <X className={icon.sm} />
        </button>
      )}
    </div>
  );
}
