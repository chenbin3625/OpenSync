import { useState, useEffect, useCallback } from 'react';
import { useSearchParams } from 'react-router-dom';
import './Home.css';
import { App, Typography, Tabs, Drawer, Empty } from 'antd';
import { jobGetJob, jobPut, jobDelete } from '../../api/job';
import { alistGet } from '../../api/alist';
import TaskList from './TaskList';
import TaskDetail from './TaskDetail';
import HomeSidebar from './HomeSidebar';
import HomeOverview from './HomeOverview';
import JobFormDrawer from './JobFormDrawer';
import type { AlistItem, JobItem } from '../../types';
import { buildHomeRouteSearch, readHomeRouteState, type HomeRouteState, type HomeTabKey } from './routeState';

export default function Home() {
  const { message } = App.useApp();
  const [searchParams, setSearchParams] = useSearchParams();
  const initialRouteState = readHomeRouteState(searchParams);
  const [list, setList] = useState<JobItem[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize] = useState(12);
  const [loading, setLoading] = useState(false);
  const [listLoaded, setListLoaded] = useState(false);
  const [alistList, setAlistList] = useState<AlistItem[]>([]);
  const [drawerVisible, setDrawerVisible] = useState(false);
  const [editingJob, setEditingJob] = useState<JobItem | null>(null);
  const [selectedJobId, setSelectedJobId] = useState<number | null>(() => initialRouteState.jobId);
  const [activeJobTab, setActiveJobTab] = useState<HomeTabKey>(() => initialRouteState.tab);
  const [taskDetailDrawerTaskId, setTaskDetailDrawerTaskId] = useState<string>('');

  const fetchList = useCallback(async (p = page, ps = pageSize) => {
    setLoading(true);
    try {
      const res = await jobGetJob({ pageSize: ps, pageNum: p });
      setList(res.data?.dataList || []);
      setTotal(res.data?.count || 0);
    } catch { /* ignore */ }
    finally {
      setListLoaded(true);
      setLoading(false);
    }
  }, [page, pageSize]);

  const fetchAlistList = useCallback(async () => {
    try {
      const res = await alistGet();
      setAlistList(res.data || []);
    } catch { /* ignore */ }
  }, []);

  useEffect(() => { fetchAlistList(); }, [fetchAlistList]);
  useEffect(() => { fetchList(); }, [fetchList]);
  useEffect(() => {
    const routeState = readHomeRouteState(searchParams);
    setActiveJobTab((current) => (current === routeState.tab ? current : routeState.tab));
    setSelectedJobId((current) => (current === routeState.jobId ? current : routeState.jobId));
  }, [searchParams]);
  useEffect(() => {
    if (!listLoaded) return;
    let nextJobId = selectedJobId;
    if (list.length === 0) {
      nextJobId = null;
    } else if (!selectedJobId || !list.some((job) => job.id === selectedJobId)) {
      nextJobId = list[0].id;
    }

    if (nextJobId === selectedJobId) return;
    setSelectedJobId(nextJobId);
    setSearchParams(buildHomeRouteSearch(searchParams, {
      tab: activeJobTab,
      jobId: nextJobId,
    }), { replace: true });
  }, [activeJobTab, list, listLoaded, searchParams, selectedJobId, setSearchParams]);

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
    try {
      await jobDelete({ id });
      message.success('删除成功');
      fetchList();
    } catch { /* ignore */ }
  };

  const handleToggle = async (job: JobItem) => {
    try {
      await jobPut({ id: String(job.id), pause: job.enable === 1 });
      message.success('操作成功');
      fetchList();
    } catch { /* ignore */ }
  };

  const handleRun = async (id: number) => {
    try {
      await jobPut({ id: String(id) });
      message.success('已提交执行');
      fetchList();
    } catch { /* ignore */ }
  };

  const handleRunAll = async () => {
    try {
      await jobPut({});
      message.success('已提交执行所有同步任务');
    } catch { /* ignore */ }
  };

  const updateHomeRouteState = useCallback((state: Partial<HomeRouteState>) => {
    const nextRouteState: HomeRouteState = {
      tab: state.tab ?? activeJobTab,
      jobId: state.jobId !== undefined ? state.jobId : selectedJobId,
    };
    setActiveJobTab(nextRouteState.tab);
    setSelectedJobId(nextRouteState.jobId);
    setSearchParams(buildHomeRouteSearch(searchParams, nextRouteState), { replace: true });
  }, [activeJobTab, searchParams, selectedJobId, setSearchParams]);

  const getAlistName = (alistId: number) => {
    const a = alistList.find((x) => x.id === alistId);
    if (!a) return `引擎 #${alistId}`;
    return a.remark ? `${a.userName} (${a.remark})` : a.userName;
  };

  const selectedJob = list.find((job) => job.id === selectedJobId) || null;

  return (
    <div className="sync-manager">
      <HomeSidebar
        list={list}
        loading={loading}
        selectedJobId={selectedJobId}
        total={total}
        page={page}
        pageSize={pageSize}
        onAdd={handleAdd}
        onRunAll={handleRunAll}
        onSelectJob={(jobId) => updateHomeRouteState({ jobId })}
        onClearTaskDetail={() => setTaskDetailDrawerTaskId('')}
        setPage={setPage}
      />

      <main className="sync-manager-content">
        {selectedJob ? (
          <Tabs
            className="sync-main-tabs"
            activeKey={activeJobTab}
            onChange={(key) => updateHomeRouteState({ tab: key as HomeTabKey })}
            items={[
              {
                key: 'overview',
                label: '总览',
                children: activeJobTab === 'overview' ? (
                  <HomeOverview
                    selectedJob={selectedJob}
                    onRun={handleRun}
                    onEdit={handleEdit}
                    onDelete={handleDelete}
                    onToggle={handleToggle}
                    getAlistName={getAlistName}
                  />
                ) : null,
              },
              {
                key: 'realtime',
                label: '实时任务',
                children: activeJobTab === 'realtime' ? (
                  <TaskList
                    key={`realtime-${selectedJob.id}`}
                    jobId={String(selectedJob.id)}
                    view="realtime"
                    onTaskDetail={(taskId) => setTaskDetailDrawerTaskId(String(taskId))}
                  />
                ) : null,
              },
              {
                key: 'history',
                label: '历史任务',
                children: activeJobTab === 'history' ? (
                  <TaskList
                    key={`history-${selectedJob.id}`}
                    jobId={String(selectedJob.id)}
                    view="history"
                    onTaskDetail={(taskId) => setTaskDetailDrawerTaskId(String(taskId))}
                  />
                ) : null,
              },
            ]}
          />
        ) : (
          <div className="sync-manager-empty">
            <Empty
              image={Empty.PRESENTED_IMAGE_SIMPLE}
              description={<Typography.Text type="secondary">暂无同步任务，点击左侧「新建」创建第一个同步任务</Typography.Text>}
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

      <Drawer
        className="task-detail-drawer"
        title={`任务详情 — 任务 #${taskDetailDrawerTaskId}`}
        placement="bottom"
        open={!!taskDetailDrawerTaskId}
        onClose={() => setTaskDetailDrawerTaskId('')}
        styles={{ wrapper: { height: '90vh' }, body: { padding: 16 } }}
        destroyOnHidden
      >
        <TaskDetail key={taskDetailDrawerTaskId} taskId={taskDetailDrawerTaskId} embedded />
      </Drawer>
    </div>
  );
}
