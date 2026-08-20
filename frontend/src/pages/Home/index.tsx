import { useState, useEffect, useCallback, useRef, useMemo, lazy, Suspense } from 'react';
import { useSearchParams } from 'react-router-dom';
import './Home.css';
import Alert from 'antd/es/alert';
import App from 'antd/es/app';
import Drawer from 'antd/es/drawer';
import Empty from 'antd/es/empty';
import Spin from 'antd/es/spin';
import Tabs from 'antd/es/tabs';
import Typography from 'antd/es/typography';
import { jobGetJob, jobPut, jobDelete } from '../../api/job';
import { alistGet } from '../../api/alist';
import HomeSidebar from './HomeSidebar';
import HomeOverview from './HomeOverview';
import type { AlistItem, JobItem } from '../../types';
import { buildHomeRouteSearch, readHomeRouteState, type HomeRouteState, type HomeTabKey } from './routeState';
import { formatAlistLabel } from './homeUtils';

const TaskList = lazy(() => import('./TaskList'));
const TaskDetail = lazy(() => import('./TaskDetail'));
const JobFormDrawer = lazy(() => import('./JobFormDrawer'));

const PAGE_SIZE = 12;

export default function Home() {
  const { message } = App.useApp();
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

  // Clamp `page` back to the max valid page when the loaded total no longer
  // supports it (e.g. deleting the last jobs on page > 1 leaves total ≤ PAGE_SIZE).
  // Setting `page` in the route triggers `fetchList` via the page-driven effect,
  // and once `page <= maxPage` this effect stops firing (no infinite loop).
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
    message.success(editingJob ? '编辑成功，下次任务生效' : '创建成功');
    setDrawerVisible(false);
    fetchList();
  };

  const handleDelete = async (id: number) => {
    const snapshot = list;
    const snapshotTotal = total;
    setList((prev) => prev.filter((item) => item.id !== id));
    setTotal((prev) => Math.max(0, prev - 1));
    try {
      await jobDelete({ id });
      message.success('删除成功');
      fetchList();
    } catch (err) {
      setList(snapshot);
      setTotal(snapshotTotal);
      console.error('job delete failed', err);
    }
  };

  const handleToggle = async (job: JobItem) => {
    const nextEnable = job.enable === 1 ? 0 : 1;
    setList((prev) => prev.map((item) => (
      item.id === job.id ? { ...item, enable: nextEnable } : item
    )));
    try {
      await jobPut({ id: String(job.id), pause: job.enable === 1 });
      message.success('操作成功');
    } catch (err) {
      setList((prev) => prev.map((item) => (
        item.id === job.id ? { ...item, enable: job.enable } : item
      )));
      console.error('job toggle failed', err);
    }
  };

  const handleRun = async (id: number) => {
    updateHomeRouteState({ tab: 'realtime', jobId: id });
    try {
      await jobPut({ id: String(id) });
      message.success('已提交执行');
    } catch (err) {
      console.error('job run failed', err);
    }
  };

  const handleRunAll = async () => {
    try {
      await jobPut({});
      message.success('已提交执行所有同步任务');
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

  return (
    <div className="sync-manager">
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

      <main className="sync-manager-content">
        {listError && (
          <Alert
            type="error"
            showIcon
            message="同步任务列表加载失败"
            className="task-feedback"
          />
        )}
        {selectedJob ? (
          <Tabs
            activeKey={activeJobTab}
            onChange={(key) => updateHomeRouteState({ tab: key as HomeTabKey })}
            destroyInactiveTabPane
            items={[
              {
                key: 'overview',
                label: '总览',
                children: (
                  <HomeOverview
                    selectedJob={selectedJob}
                    onRun={handleRun}
                    onEdit={handleEdit}
                    onDelete={handleDelete}
                    onToggle={handleToggle}
                    getAlistName={getAlistName}
                  />
                ),
              },
              {
                key: 'realtime',
                label: '实时任务',
                children: (
                  <Suspense fallback={<div className="task-feedback"><Spin size="small" /></div>}>
                    <TaskList
                      key={`realtime-${selectedJob.id}`}
                      jobId={String(selectedJob.id)}
                      view="realtime"
                      active
                      onTaskDetail={(taskId) => setTaskDetailDrawerTaskId(String(taskId))}
                    />
                  </Suspense>
                ),
              },
              {
                key: 'history',
                label: '历史任务',
                children: (
                  <Suspense fallback={<div className="task-feedback"><Spin size="small" /></div>}>
                    <TaskList
                      key={`history-${selectedJob.id}`}
                      jobId={String(selectedJob.id)}
                      view="history"
                      active
                      onTaskDetail={(taskId) => setTaskDetailDrawerTaskId(String(taskId))}
                    />
                  </Suspense>
                ),
              },
            ]}
          />
        ) : (
          <div className="sync-manager-empty">
            <Empty
              image={Empty.PRESENTED_IMAGE_SIMPLE}
              description={<Typography.Text type="secondary">暂无同步任务，点击「新建」创建第一个同步任务</Typography.Text>}
            />
          </div>
        )}
      </main>

      {drawerVisible && (
        <Suspense fallback={null}>
          <JobFormDrawer
            visible={drawerVisible}
            editingJob={editingJob}
            alistList={alistList}
            onClose={() => setDrawerVisible(false)}
            onSubmit={handleDrawerSubmit}
          />
        </Suspense>
      )}

      <Drawer
        className="task-detail-drawer"
        title={`任务详情 — 任务 #${taskDetailDrawerTaskId}`}
        placement="bottom"
        open={!!taskDetailDrawerTaskId}
        onClose={() => setTaskDetailDrawerTaskId('')}
        styles={{ wrapper: { height: '90vh' }, body: { padding: 16 } }}
        destroyOnHidden
      >
        {taskDetailDrawerTaskId ? (
          <Suspense fallback={<div className="task-feedback"><Spin size="small" /></div>}>
            <TaskDetail key={taskDetailDrawerTaskId} taskId={taskDetailDrawerTaskId} embedded />
          </Suspense>
        ) : null}
      </Drawer>
    </div>
  );
}
