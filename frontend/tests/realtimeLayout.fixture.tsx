import { createRoot } from 'react-dom/client';
import TaskRealtimeHero from '../src/pages/Home/components/TaskRealtimeHero';
import TaskRealtimeRows from '../src/pages/Home/components/TaskRealtimeRows';
import HomeOverview from '../src/pages/Home/HomeOverview';
import HomeSidebar from '../src/pages/Home/HomeSidebar';
import type { CurrentTaskView, JobItem, TaskItem } from '../src/types';
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
const jobs: JobItem[] = ['家庭影音资料与旅行照片每日自动备份到云盘的同步任务', 'BackupJob'.repeat(20)].map((remark, index) => ({
  id: index + 1, remark, enable: 1, srcPath: '/source', dstPath: '/backup', alistId: 1,
  useCacheS: false, useCacheT: false, method: 0, interval: 3600, isCron: 0,
}));

createRoot(document.getElementById('root')!).render(
  <main className="p-4 min-w-0">
    <div className="grid gap-4 grid-cols-1 md:grid-cols-[300px_minmax(0,1fr)]">
      <HomeSidebar list={jobs} loading={false} selectedJobId={1} total={jobs.length}
        page={1} pageSize={12} onAdd={() => {}} onRunAll={() => {}} onSelectJob={() => {}}
        onClearTaskDetail={() => {}} setPage={() => {}} />
      <section className="min-w-0 p-4 sm:p-5">
        <TaskRealtimeHero currentTask={currentTask} onStop={() => {}}>
          <TaskRealtimeRows activeTab={1} activeTabLabel="running" loading={false}
            page={1} pageSize={10} rows={rows} visibleRows={rows} total={rows.length}
            onPageChange={() => {}} />
        </TaskRealtimeHero>
        {jobs.map((job) => (
          <HomeOverview key={job.id} selectedJob={job} getAlistName={() => 'OpenList'}
            onRun={() => {}} onEdit={() => {}} onDelete={() => {}} onToggle={() => {}} />
        ))}
      </section>
    </div>
  </main>,
);
