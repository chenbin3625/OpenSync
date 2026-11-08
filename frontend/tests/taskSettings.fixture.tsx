import { Profiler } from 'react';
import { createRoot } from 'react-dom/client';
import TaskList from '../src/pages/Home/TaskList';
import Setting from '../src/pages/Setting';
import { TooltipProvider } from '../src/components/ui/tooltip';
import '../src/index.css';

const snapshots: string[][] = [];
Object.assign(window, { taskTabSnapshots: snapshots });

createRoot(document.getElementById('root')!).render(
  <main className="p-4 min-w-0">
    <TooltipProvider>
      {new URLSearchParams(location.search).has('settings') ? <Setting /> : (
        <Profiler id="task-list" onRender={() => {
          snapshots.push([...document.querySelectorAll('button')]
            .map((button) => button.textContent || '')
            .filter((text) => /^(等待|运行中)/.test(text)));
        }}>
          <TaskList jobId="1" view="realtime" />
        </Profiler>
      )}
    </TooltipProvider>
  </main>,
);
