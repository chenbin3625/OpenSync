import { lazy, Suspense, useState, useEffect, useCallback, useMemo } from 'react';
import { useSearchParams } from 'react-router-dom';
import { CircleAlert, Inbox, LayoutDashboard, PlayCircle, History } from 'lucide-react';
import { jobGetJob, jobPut, jobDelete } from '../../api/job';
import { alistGet } from '../../api/alist';
import HomeSidebar from './HomeSidebar';
import HomeOverview from './HomeOverview';
import type { AlistItem, JobItem } from '../../types';
import { buildHomeRouteSearch, readHomeRouteState, type HomeRouteState, type HomeTabKey } from './routeState';
import { formatAlistLabel } from './homeUtils';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '../../components/ui/tabs';
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '../../components/ui/sheet';
import { toast } from '../../components/ui/toaster';
import { Alert } from '../../components/common/Alert';
import { EmptyState, ErrorState, LoadingState } from '../../components/common/StatePlaceholder';
import { cn } from '../../lib/utils';
import { icon, surface } from '../../lib/styles';

const PAGE_SIZE = 12;
const emptyJobs: JobItem[] = [];
const TaskList = lazy(() => import('./TaskList'));
const TaskDetail = lazy(() => import('./TaskDetail'));
const JobFormDrawer = lazy(() => import('./JobFormDrawer'));

type JobPageResult = {
  page: number;
  revision: number;
  list: JobItem[];
  total: number;
  error: boolean;
};

export default function Home() {
  const [searchParams, setSearchParams] = useSearchParams();
  const routeState = useMemo(() => readHomeRouteState(searchParams), [searchParams]);
  const { tab: activeJobTab, jobId: selectedJobId, page } = routeState;
  const [listResult, setListResult] = useState<JobPageResult | null>(null);
  const [listRevision, setListRevision] = useState(0);
  const [alistList, setAlistList] = useState<AlistItem[]>([]);
  const [drawerVisible, setDrawerVisible] = useState<boolean | null>(null);
  const [editingJob, setEditingJob] = useState<JobItem | null>(null);
  const [taskDetailDrawerTaskId, setTaskDetailDrawerTaskId] = useState<string>('');

  const pageResult = listResult?.page === page ? listResult : null;
  const list = pageResult?.list ?? emptyJobs;
  const total = listResult?.total ?? 0;
  const maxPage = Math.max(1, Math.ceil(total / PAGE_SIZE));
  const loading = pageResult?.revision !== listRevision || (pageResult?.error === false && page > maxPage);
  const listError = !loading && pageResult?.error === true;
  const selectedJob = list.find((job) => job.id === selectedJobId) ?? list[0] ?? null;

  const refreshList = () => setListRevision((revision) => revision + 1);

  useEffect(() => {
    const controller = new AbortController();
    async function loadJobs() {
      try {
        const res = await jobGetJob({ pageSize: PAGE_SIZE, pageNum: page }, { silent: true, signal: controller.signal });
        if (controller.signal.aborted) return;
        setListResult({
          page,
          revision: listRevision,
          list: res.data?.dataList ?? emptyJobs,
          total: res.data?.count ?? 0,
          error: false,
        });
      } catch (err) {
        if (controller.signal.aborted) return;
        console.error('job list fetch failed', err);
        setListResult((previous) => ({
          page,
          revision: listRevision,
          list: previous?.page === page ? previous.list : emptyJobs,
          total: previous?.total ?? 0,
          error: true,
        }));
      }
    }
    void loadJobs();
    return () => controller.abort();
  }, [listRevision, page]);

  useEffect(() => {
    const controller = new AbortController();
    async function fetchAlistList() {
      try {
        const res = await alistGet({ signal: controller.signal });
        if (controller.signal.aborted) return;
        setAlistList(res.data || []);
      } catch (err) {
        if (controller.signal.aborted) return;
        console.error('alist list fetch failed', err);
      }
    }
    void fetchAlistList();
    return () => controller.abort();
  }, []);

  const updateHomeRouteState = useCallback((state: Partial<HomeRouteState>) => {
    const current = readHomeRouteState(searchParams);
    setSearchParams(buildHomeRouteSearch(searchParams, {
      tab: state.tab ?? current.tab,
      jobId: state.jobId !== undefined ? state.jobId : current.jobId,
      page: state.page ?? current.page,
    }), { replace: true });
  }, [searchParams, setSearchParams]);

  useEffect(() => {
    if (!pageResult || pageResult.error || pageResult.revision !== listRevision) return;
    if (page > maxPage) {
      updateHomeRouteState({ page: maxPage, jobId: null });
      return;
    }
    const nextJobId = selectedJob?.id ?? null;
    if (nextJobId !== selectedJobId) updateHomeRouteState({ jobId: nextJobId });
  }, [listRevision, maxPage, page, pageResult, selectedJob, selectedJobId, updateHomeRouteState]);

  const handleAdd = () => {
    setEditingJob(null);
    setDrawerVisible(true);
  };

  const handleEdit = (job: JobItem) => {
    setEditingJob(job);
    setDrawerVisible(true);
  };

  const handleDrawerSubmit = () => {
    toast.success(editingJob ? '编辑成功，下次任务生效' : '创建成功');
    setDrawerVisible(false);
    refreshList();
  };

  const handleDelete = async (id: number) => {
    try {
      await jobDelete({ id });
      toast.success('删除成功');
      refreshList();
    } catch (err) {
      console.error('job delete failed', err);
    }
  };

  const handleToggle = async (job: JobItem) => {
    try {
      await jobPut({ id: String(job.id), pause: job.enable === 1 });
      toast.success('操作成功');
      refreshList();
    } catch (err) {
      console.error('job toggle failed', err);
    }
  };

  const handleRun = async (id: number) => {
    try {
      await jobPut({ id: String(id) });
      toast.success('已提交执行');
      refreshList();
    } catch (err) {
      console.error('job run failed', err);
    }
  };

  const handleRunAll = async () => {
    try {
      await jobPut({});
      toast.success('已提交执行所有同步任务');
      refreshList();
    } catch (err) {
      console.error('job run all failed', err);
    }
  };

  const handlePageChange = useCallback((nextPage: number) => {
    setTaskDetailDrawerTaskId('');
    updateHomeRouteState({ page: nextPage, jobId: null });
  }, [updateHomeRouteState]);

  const getAlistName = useCallback((alistId: number) => {
    const a = alistList.find((x) => x.id === alistId);
    if (!a) return `引擎 #${alistId}`;
    return formatAlistLabel(a);
  }, [alistList]);

  // 桌面端两栏铺满 main 的剩余高度：md:grow 让容器高度等于 max(内容, 可用空间)，
  // 既不像按视口高度手算那样多出十几像素的整页滚动条，也不会压缩长内容。
  return (
    <div className="grid gap-4 grid-cols-1 md:grid-cols-[300px_minmax(0,1fr)] md:grow md:min-h-0">
      <HomeSidebar
        list={list}
        loading={loading}
        failed={listError}
        selectedJobId={selectedJob?.id ?? null}
        total={total}
        page={page}
        pageSize={PAGE_SIZE}
        onAdd={handleAdd}
        onRunAll={handleRunAll}
        onSelectJob={(jobId) => {
          setTaskDetailDrawerTaskId('');
          updateHomeRouteState({ jobId });
        }}
        setPage={handlePageChange}
      />

      <section aria-label="同步任务内容" className={cn(surface.card, 'min-w-0 p-4 sm:p-5 flex flex-col')}>
        {listError && selectedJob && (
          <Alert className="mb-4">
            同步任务列表更新失败
            <button type="button" onClick={refreshList} className="ml-2 rounded-sm underline focus-visible:outline-2">
              重新加载
            </button>
          </Alert>
        )}

        {selectedJob ? (
          <Tabs
            value={activeJobTab}
            onValueChange={(key) => updateHomeRouteState({ tab: key as HomeTabKey })}
            className="w-full space-y-4"
          >
            <TabsList aria-label="同步任务视图" className="grid w-full grid-cols-3 sm:inline-flex sm:w-auto">
              <TabsTrigger value="overview" className="flex items-center gap-1.5 px-2 text-xs sm:px-3">
                <LayoutDashboard className={cn(icon.sm, 'max-[359px]:hidden')} aria-hidden="true" />
                <span>总览</span>
              </TabsTrigger>
              <TabsTrigger value="realtime" className="flex items-center gap-1.5 px-2 text-xs sm:px-3">
                <PlayCircle className={cn(icon.sm, 'max-[359px]:hidden')} aria-hidden="true" />
                <span>实时任务</span>
              </TabsTrigger>
              <TabsTrigger value="history" className="flex items-center gap-1.5 px-2 text-xs sm:px-3">
                <History className={cn(icon.sm, 'max-[359px]:hidden')} aria-hidden="true" />
                <span>历史任务</span>
              </TabsTrigger>
            </TabsList>

            <TabsContent value="overview">
              <HomeOverview
                selectedJob={selectedJob}
                onRun={handleRun}
                onEdit={handleEdit}
                onDelete={handleDelete}
                onToggle={handleToggle}
                getAlistName={getAlistName}
              />
            </TabsContent>

            <TabsContent value="realtime">
              <Suspense fallback={<LoadingState title="正在加载实时任务" />}>
                <TaskList
                  key={`realtime-${selectedJob.id}`}
                  jobId={String(selectedJob.id)}
                  view="realtime"
                  active={activeJobTab === 'realtime'}
                  onTaskDetail={(taskId) => setTaskDetailDrawerTaskId(String(taskId))}
                />
              </Suspense>
            </TabsContent>

            <TabsContent value="history">
              <Suspense fallback={<LoadingState title="正在加载历史任务" />}>
                <TaskList
                  key={`history-${selectedJob.id}`}
                  jobId={String(selectedJob.id)}
                  view="history"
                  active={activeJobTab === 'history'}
                  onTaskDetail={(taskId) => setTaskDetailDrawerTaskId(String(taskId))}
                />
              </Suspense>
            </TabsContent>
          </Tabs>
        ) : (
          <div className="grid place-items-center flex-1 min-h-[360px]">
            {loading ? (
              <LoadingState title="正在加载同步任务" />
            ) : listError ? (
              <ErrorState icon={CircleAlert} title="同步任务列表加载失败" onRetry={refreshList} />
            ) : (
              <EmptyState icon={Inbox} title="暂无同步任务" />
            )}
          </div>
        )}
      </section>

      {drawerVisible !== null && (
        <Suspense fallback={<LoadingState
          title="正在加载任务表单"
          className="fixed inset-x-4 bottom-4 z-40 mx-auto w-fit rounded-lg border border-line bg-surface px-4 py-3 shadow-sm"
        />}>
          <JobFormDrawer
            visible={drawerVisible}
            editingJob={editingJob}
            alistList={alistList}
            onClose={() => setDrawerVisible(false)}
            onSubmit={handleDrawerSubmit}
          />
        </Suspense>
      )}

      <Sheet
        open={!!taskDetailDrawerTaskId}
        onOpenChange={(open) => !open && setTaskDetailDrawerTaskId('')}
      >
        <SheetContent
          side="bottom"
          className="h-[90dvh] overflow-hidden flex flex-col p-4 sm:p-6"
        >
          <SheetHeader className={cn('pb-3 border-b', surface.divider)}>
            <SheetTitle>任务详情 — 任务 #{taskDetailDrawerTaskId}</SheetTitle>
          </SheetHeader>
          <div className="flex-1 overflow-auto pt-3">
            {taskDetailDrawerTaskId && (
              <Suspense fallback={<LoadingState title="正在加载任务详情" />}>
                <TaskDetail key={taskDetailDrawerTaskId} taskId={taskDetailDrawerTaskId} embedded />
              </Suspense>
            )}
          </div>
        </SheetContent>
      </Sheet>
    </div>
  );
}
