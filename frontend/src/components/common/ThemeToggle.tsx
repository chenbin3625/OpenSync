import { Moon, Sun } from 'lucide-react';
import { useStore } from '../../stores/useStore';
import { Button } from '../ui/button';
import { Tooltip } from '../ui/tooltip';

export function ThemeToggle() {
  const theme = useStore((state) => state.theme);
  const setTheme = useStore((state) => state.setTheme);
  const dark = theme === 'dark';
  return (
    <Tooltip title={dark ? '切换到浅色模式' : '切换到深色模式'}>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        aria-label="深色模式"
        aria-pressed={dark}
        onClick={() => setTheme(dark ? 'light' : 'dark')}
      >
        {dark ? <Sun className="h-4 w-4" aria-hidden="true" /> : <Moon className="h-4 w-4" aria-hidden="true" />}
      </Button>
    </Tooltip>
  );
}
