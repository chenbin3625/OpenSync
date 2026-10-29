import { createRoot } from 'react-dom/client';
import TaskRealtimeHero from '../src/pages/Home/components/TaskRealtimeHero';
import TaskRealtimeRows from '../src/pages/Home/components/TaskRealtimeRows';
import type { CurrentTaskView, TaskItem } from '../src/types';
import '../src/index.css';

const currentTask: CurrentTaskView = {
  taskId: 42,
  scanFinish: true,
  createTime: Math.floor(Date.now() / 1000) - 9720,
  duration: 9720,
  num: { wait: 2158, running: 2, success: 51, fail: 0, other: 0 },
  size: { wait: 0, running: 0, success: 0, fail: 0, other: 0 },
  doneSize: 443.02 * 1024 ** 3,
  remainSize: 19.27 * 1024 ** 4,
  speed: 40 * 1024 ** 2,
  speedAvg: 48.95 * 1024 ** 2,
  remainTime: 4 * 86400 + 18 * 3600 + 40 * 60 + 59,
};
const rows: TaskItem[] = [
  { id: 1, fileName: 'Family.Holiday.2026.2160p.UHD.BluRay.REMUX.HEVC.DTS-HD.MA.7.1.mkv', fileSize: 43.72 * 1024 ** 3, progress: 39 },
  { id: 2, fileName: 'L'.repeat(160) + '.mkv', fileSize: 16.34 * 1024 ** 3, progress: 33 },
].map((item) => ({ ...item, status: 1, type: 0, srcPath: `/source/${item.fileName}`, dstPath: `/backup/${item.fileName}` }));

createRoot(document.getElementById('root')!).render(
  <main className="p-4 min-w-0">
    <TaskRealtimeHero currentTask={currentTask} onStop={() => {}}>
      <TaskRealtimeRows activeTab={1} activeTabLabel="running" loading={false}
        page={1} pageSize={10} rows={rows} visibleRows={rows} total={rows.length}
        onPageChange={() => {}} />
    </TaskRealtimeHero>
  </main>,
);
