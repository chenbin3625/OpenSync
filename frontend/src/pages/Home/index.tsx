import { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import { useSearchParams } from 'react-router-dom';
import { Inbox, LayoutDashboard, PlayCircle, History } from 'lucide-react';
import { jobGetJob, jobPut, jobDelete } from '../../api/job';
import { alistGet } from '../../api/alist';
import TaskList from './TaskList';
import TaskDetail from './TaskDetail';
import HomeSidebar from './HomeSidebar';
import HomeOverview from './HomeOverview';
import JobFormDrawer from './JobFormDrawer';
import type { AlistItem, JobItem } from '../../types';
import { buildHomeRouteSearch, readHomeRouteState, type HomeRouteState, type HomeTabKey } from './routeState';
import { formatAlistLabel } from './homeUtils';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '../../components/ui/tabs';
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '../../components/ui/sheet';
import { toast } from '../../components/ui/toaster';
import { Alert } from '../../components/common/Alert';
import { EmptyState } from '../../components/common/StatePlaceholder';
import { cn } from '../../lib/utils';
import { icon, surface } from '../../lib/styles';

const PAGE_SIZE = 12;

export default function Home() {
  const [searchParams, setSearchParams] = useSearchParams();
  const routeState = useMemo(() => readHomeRouteState(searchParams), [searchParams]);
  const { tab: activeJobTab, jobId: selectedJobId, page } = routeState;
  const [list, setList] = useState<JobItem[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [listLoaded, setListLoaded] = useState(false);
  const [listError, setListError] = useState(false);
  const [alistList, setAlistList] = useState<AlistItem[]>([]);
  const [drawerVisible, setDrawerVisible] = useState(false);
  const [editingJob, setEditingJob] = useState<JobItem | null>(null);
  const [taskDetailDrawerTaskId, setTaskDetailDrawerTaskId] = useState<string>('');
  const listRequestRef = useRef(0);

  const fetchList = useCallback(async (p = page, ps = PAGE_SIZE) => {
    const requestID = ++listRequestRef.current;
    setLoading(true);
    setListError(false);
    try {
      const res = await jobGetJob({ pageSize: ps, pageNum: p }, { silent: true });
      if (requestID !== listRequestRef.current) return;
      setList(res.data?.dataList || []);
      setTotal(res.data?.count || 0);
    } catch (err) {
      if (requestID !== listRequestRef.current) return;
      setList([]);
      setTotal(0);
      setListError(true);
      console.error('job list fetch failed', err);
    } finally {
      if (requestID === listRequestRef.current) {
        setListLoaded(true);
        setLoading(false);
      }
    }
  }, [page]);

  const fetchAlistList = useCallback(async () => {
    try {
      const res = await alistGet();
      setAlistList(res.data || []);
    } catch (err) {
      console.error('alist list fetch failed', err);
    }
  }, []);

  useEffect(() => { fetchAlistList(); }, [fetchAlistList]);
  useEffect(() => { fetchList(); }, [fetchList]);

  const updateHomeRouteState = useCallback((state: Partial<HomeRouteState>) => {
    const current = readHomeRouteState(searchParams);
    setSearchParams(buildHomeRouteSearch(searchParams, {
      tab: state.tab ?? current.tab,
      jobId: state.jobId !== undefined ? state.jobId : current.jobId,
      page: state.page ?? current.page,
    }), { replace: true });
  }, [searchParams, setSearchParams]);

  useEffect(() => {
    if (!listLoaded) return;
    let nextJobId = selectedJobId;
    if (list.length === 0) {
      nextJobId = null;
    } else if (!selectedJobId || !list.some((job) => job.id === selectedJobId)) {
      nextJobId = list[0].id;
    }

    if (nextJobId === selectedJobId) return;
    updateHomeRouteState({ jobId: nextJobId });
  }, [list, listLoaded, selectedJobId, updateHomeRouteState]);

  useEffect(() => {
    if (!listLoaded) return;
    const maxPage = Math.max(1, Math.ceil(total / PAGE_SIZE));
    if (page > maxPage) {
      updateHomeRouteState({ page: maxPage });
    }
  }, [listLoaded, total, page, updateHomeRouteState]);

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
    fetchList();
  };

  const handleDelete = async (id: number) => {
    try {
      await jobDelete({ id });
      toast.success('删除成功');
      fetchList();
    } catch (err) {
      console.error('job delete failed', err);
    }
  };

  const handleToggle = async (job: JobItem) => {
    try {
      await jobPut({ id: String(job.id), pause: job.enable === 1 });
      toast.success('操作成功');
      fetchList();
    } catch (err) {
      console.error('job toggle failed', err);
    }
  };

  const handleRun = async (id: number) => {
    try {
      await jobPut({ id: String(id) });
      toast.success('已提交执行');
      fetchList();
    } catch (err) {
      console.error('job run failed', err);
    }
  };

  const handleRunAll = async () => {
    try {
      await jobPut({});
      toast.success('已提交执行所有同步任务');
      fetchList();
    } catch (err) {
      console.error('job run all failed', err);
    }
  };

  const handlePageChange = useCallback((nextPage: number) => {
    updateHomeRouteState({ page: nextPage, jobId: null });
  }, [updateHomeRouteState]);

  const getAlistName = useCallback((alistId: number) => {
    const a = alistList.find((x) => x.id === alistId);
    if (!a) return `引擎 #${alistId}`;
    return formatAlistLabel(a);
  }, [alistList]);

  const selectedJob = list.find((job) => job.id === selectedJobId) || null;

  // 桌面端两栏铺满 main 的剩余高度：md:grow 让容器高度等于 max(内容, 可用空间)，
  // 既不像按视口高度手算那样多出十几像素的整页滚动条，也不会压缩长内容。
  return (
    <div className="grid gap-4 grid-cols-1 md:grid-cols-[300px_minmax(0,1fr)] md:grow md:min-h-0">
      <HomeSidebar
        list={list}
        loading={loading}
        selectedJobId={selectedJobId}
        total={total}
        page={page}
        pageSize={PAGE_SIZE}
        onAdd={handleAdd}
        onRunAll={handleRunAll}
        onSelectJob={(jobId) => updateHomeRouteState({ jobId })}
        onClearTaskDetail={() => setTaskDetailDrawerTaskId('')}
        setPage={handlePageChange}
      />

      <main className="min-w-0">
        {listError && <Alert className="mb-4">同步任务列表加载失败</Alert>}

        {selectedJob ? (
          <Tabs
            value={activeJobTab}
            onValueChange={(key) => updateHomeRouteState({ tab: key as HomeTabKey })}
            className="w-full space-y-4"
          >
            <TabsList>
              <TabsTrigger value="overview" className="flex items-center gap-1.5 text-xs">
                <LayoutDashboard className={icon.sm} />
                <span>总览</span>
              </TabsTrigger>
              <TabsTrigger value="realtime" className="flex items-center gap-1.5 text-xs">
                <PlayCircle className={icon.sm} />
                <span>实时任务</span>
              </TabsTrigger>
              <TabsTrigger value="history" className="flex items-center gap-1.5 text-xs">
                <History className={icon.sm} />
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
              <TaskList
                key={`realtime-${selectedJob.id}`}
                jobId={String(selectedJob.id)}
                view="realtime"
                active={activeJobTab === 'realtime'}
                onTaskDetail={(taskId) => setTaskDetailDrawerTaskId(String(taskId))}
              />
            </TabsContent>

            <TabsContent value="history">
              <TaskList
                key={`history-${selectedJob.id}`}
                jobId={String(selectedJob.id)}
                view="history"
                active={activeJobTab === 'history'}
                onTaskDetail={(taskId) => setTaskDetailDrawerTaskId(String(taskId))}
              />
            </TabsContent>
          </Tabs>
        ) : (
          <div className={cn(surface.card, 'grid place-items-center min-h-[360px]')}>
            <EmptyState
              icon={Inbox}
              title="暂无同步任务，点击「新建」创建第一个同步任务"
            />
          </div>
        )}
      </main>

      <JobFormDrawer
        visible={drawerVisible}
        editingJob={editingJob}
        alistList={alistList}
        onClose={() => setDrawerVisible(false)}
        onSubmit={handleDrawerSubmit}
      />

      <Sheet
        open={!!taskDetailDrawerTaskId}
        onOpenChange={(open) => !open && setTaskDetailDrawerTaskId('')}
      >
        <SheetContent
          side="bottom"
          className="h-[90vh] overflow-hidden flex flex-col p-4 sm:p-6"
        >
          <SheetHeader className={cn('pb-3 border-b', surface.divider)}>
            <SheetTitle>任务详情 — 任务 #{taskDetailDrawerTaskId}</SheetTitle>
          </SheetHeader>
          <div className="flex-1 overflow-auto pt-3">
            {taskDetailDrawerTaskId && (
              <TaskDetail key={taskDetailDrawerTaskId} taskId={taskDetailDrawerTaskId} embedded />
            )}
          </div>
        </SheetContent>
      </Sheet>
    </div>
  );
}

