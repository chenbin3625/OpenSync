import { useState, useEffect, useCallback, useMemo, useRef } from 'react';
import './Home.css';
import { Table, Tag, Button, Space, Select, Progress, Empty, Typography, Card, Tooltip, Input } from 'antd';
import { ArrowLeftOutlined, InfoCircleOutlined } from '@ant-design/icons';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { jobGetTaskItem } from '../../api/job';
import dayjs from 'dayjs';
import type { TaskItem } from '../../types';
import { displayText, formatSize, taskItemStatusColors, taskTypeNames } from './homeUtils';

const { Text } = Typography;

const taskItemStatusList = [
  '等待中', '进行中', '成功', '取消中', '已取消',
  '出错（将重试）', '失败中', '已失败', '等待重试中', '等待重试前',
];

const statusFilterOptions = taskItemStatusList.map((label, value) => ({ label, value }));

const typeFilterOptions = [
  { label: '复制/创建', value: 0 },
  { label: '删除', value: 1 },
  { label: '移动', value: 2 },
];

const objectFilterOptions = [
  { label: '文件', value: 0 },
  { label: '目录', value: 1 },
];

const errorFilterOptions = [
  { label: '有错误信息', value: 1 },
  { label: '无错误信息', value: 0 },
];

function pathFallback(record: TaskItem): string {
  return displayText(record.fileName || record.dstPath || record.srcPath);
}

function LongText({
  value,
  maxWidth = 260,
  type,
}: {
  value: string | number | null | undefined;
  maxWidth?: number;
  type?: 'secondary' | 'danger';
}) {
  const text = displayText(value);
  if (text === '--') return <Text type="secondary">--</Text>;
  return (
    <Tooltip title={text}>
      <Text
        type={type}
        ellipsis
        style={{ display: 'inline-block', maxWidth, verticalAlign: 'bottom' }}
      >
        {text}
      </Text>
    </Tooltip>
  );
}

type TaskDetailProps = {
  taskId?: number | string;
  embedded?: boolean;
  onBack?: () => void;
};

export default function TaskDetail({ taskId: taskIdProp, embedded = false, onBack }: TaskDetailProps = {}) {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const routeTaskId = searchParams.get('taskId') || '';
  const taskId = taskIdProp !== undefined && taskIdProp !== null ? String(taskIdProp) : routeTaskId;

  const [list, setList] = useState<TaskItem[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const [statusFilter, setStatusFilter] = useState<number | undefined>(undefined);
  const [typeFilter, setTypeFilter] = useState<number | undefined>(undefined);
  const [objectFilter, setObjectFilter] = useState<number | undefined>(undefined);
  const [errorFilter, setErrorFilter] = useState<number | undefined>(undefined);
  const [keywordInput, setKeywordInput] = useState('');
  const [keywordFilter, setKeywordFilter] = useState('');
  const requestRef = useRef(0);
  const abortRef = useRef<AbortController | null>(null);

  const fetchData = useCallback(async (options?: { silent?: boolean }) => {
    if (!taskId) return;
    // Cancel any in-flight request before starting a new one so a slow earlier
    // request (different filter/page) cannot overwrite fresh state.
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    const requestID = ++requestRef.current;
    setLoading(true);
    setError(false);
    try {
      const params: Record<string, unknown> = {
        taskId,
        pageSize,
        pageNum: page,
      };
      if (statusFilter !== undefined) params.status = statusFilter;
      if (typeFilter !== undefined) params.type = typeFilter;
      if (objectFilter !== undefined) params.isPath = objectFilter;
      if (errorFilter !== undefined) params.hasError = errorFilter;
      if (keywordFilter.trim()) params.keyword = keywordFilter.trim();
      const res = await jobGetTaskItem(params, { signal: controller.signal, silent: options?.silent });
      if (requestID !== requestRef.current || controller.signal.aborted) return;
      const data = res.data;
      // Guard against a malformed response shape: a non-array dataList would
      // otherwise throw inside .map and be silently swallowed.
      const rawList = Array.isArray(data?.dataList) ? data.dataList : [];
      const items = rawList.map((item) => {
        const prog = typeof item.progress === 'string' ? parseInt(item.progress, 10) : (item.progress || 0);
        return { ...item, progress: Math.max(0, Math.min(prog || 0, 100)) };
      });
      setList(items);
      setTotal(data?.count || 0);
    } catch (err) {
      if (controller.signal.aborted) return; // ignore cancellation, not a real error
      if (requestID !== requestRef.current) return;
      if (options?.silent) {
        console.error('task detail polling failed', err);
        return;
      }
      // Surface the failure instead of leaving stale data on screen: clear the
      // list so the table no longer shows rows that do not match the filters.
      setError(true);
      setList([]);
      setTotal(0);
      console.error('task detail fetch failed', err);
    } finally {
      if (requestID === requestRef.current && !controller.signal.aborted) {
        setLoading(false);
      }
    }
  }, [errorFilter, keywordFilter, objectFilter, page, pageSize, statusFilter, taskId, typeFilter]);

  useEffect(() => { fetchData(); }, [fetchData]);
  // Cancel any in-flight request when the component unmounts.
  useEffect(() => () => { abortRef.current?.abort(); }, []);

  // Poll while there are in-progress items so the progress bars stay live.
  useEffect(() => {
    if (!taskId) return undefined;
    const hasRunning = list.some((item) => item.status === 1);
    if (!hasRunning) return undefined;
    const pollID = setInterval(() => { fetchData({ silent: true }); }, 3000);
    return () => { clearInterval(pollID); };
  }, [list, taskId, fetchData]);

  const columns = useMemo(() => [
    {
      title: '文件名/目录',
      dataIndex: 'fileName',
      key: 'fileName',
      width: 220,
      render: (_: unknown, record: TaskItem) =>
        <LongText value={pathFallback(record)} maxWidth={200} />,
    },
    {
      title: '来源目录',
      dataIndex: 'srcPath',
      key: 'srcPath',
      width: 260,
      render: (val: string | null) => <LongText value={val} maxWidth={240} />,
    },
    {
      title: '目标目录',
      dataIndex: 'dstPath',
      key: 'dstPath',
      width: 260,
      render: (val: string | null) => <LongText value={val} maxWidth={240} />,
    },
    {
      title: '文件大小',
      dataIndex: 'fileSize',
      key: 'fileSize',
      width: 120,
      render: (val: number | null) => val == null ? '--' : formatSize(val),
    },
    {
      title: '操作类型',
      dataIndex: 'type',
      key: 'type',
      width: 100,
      render: (val: number, record: TaskItem) => {
        const label = val === 0 && record.isPath ? '创建' : (taskTypeNames[val] || String(val));
        const color = val === 1 ? 'red' : val === 2 ? 'orange' : 'blue';
        return <Tag color={color}>{label}</Tag>;
      },
    },
    {
      title: '对象',
      dataIndex: 'isPath',
      key: 'isPath',
      width: 80,
      render: (val: number | undefined) => (
        <Tag color={val ? 'cyan' : 'default'}>{val ? '目录' : '文件'}</Tag>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 190,
      render: (status: number, record: TaskItem) => {
        if (status === 1) {
          const pct = Number(record.progress || 0);
          return (
            <Tooltip title={`进行中 ${pct}%`}>
              <Progress percent={pct} size="small" />
            </Tooltip>
          );
        }
        const errorReason = typeof record.errMsg === 'string' ? record.errMsg.trim() : '';
        const statusTag = (
          <Tag color={taskItemStatusColors[status]}>
            {taskItemStatusList[status] || String(status)}
          </Tag>
        );
        if (taskItemStatusColors[status] !== 'error' || !errorReason) {
          return statusTag;
        }
        return (
          <span className="task-status-with-error">
            {statusTag}
            <Tooltip title={record.errMsg}>
              <InfoCircleOutlined className="task-status-error-tip" aria-label="查看错误原因" />
            </Tooltip>
          </span>
        );
      },
    },
    {
      title: '创建时间',
      dataIndex: 'createTime',
      key: 'createTime',
      width: 170,
      render: (val: number | undefined) => (
        val ? dayjs.unix(val).format('YYYY-MM-DD HH:mm:ss') : '--'
      ),
    },
  ], []);

  const handleKeywordSearch = (value: string) => {
    setKeywordFilter(value.trim());
    setPage(1);
  };

  const resetFilters = () => {
    setStatusFilter(undefined);
    setTypeFilter(undefined);
    setObjectFilter(undefined);
    setErrorFilter(undefined);
    setKeywordInput('');
    setKeywordFilter('');
    setPage(1);
  };

  const content = (
    <div className={embedded ? 'task-detail-panel is-embedded' : 'task-detail-panel'}>
      <div className="page-header task-detail-header">
        {embedded ? (
          <span />
        ) : (
          <Space className="task-detail-title">
            <Button icon={<ArrowLeftOutlined />} onClick={() => (onBack ? onBack() : navigate(-1))}>返回</Button>
            <h2>任务详情</h2>
          </Space>
        )}
        <Space wrap className="task-detail-filters">
          <Input.Search
            placeholder="文件 / 路径 / 错误"
            allowClear
            style={{ width: 200 }}
            value={keywordInput}
            onChange={(e) => {
              setKeywordInput(e.target.value);
              if (!e.target.value) handleKeywordSearch('');
            }}
            onSearch={handleKeywordSearch}
          />
          <Select
            placeholder="筛选状态"
            allowClear
            style={{ width: 140 }}
            value={statusFilter}
            onChange={(v) => { setStatusFilter(v); setPage(1); }}
            options={statusFilterOptions}
          />
          <Select
            placeholder="操作类型"
            allowClear
            style={{ width: 130 }}
            value={typeFilter}
            onChange={(v) => { setTypeFilter(v); setPage(1); }}
            options={typeFilterOptions}
          />
          <Select
            placeholder="文件/目录"
            allowClear
            style={{ width: 120 }}
            value={objectFilter}
            onChange={(v) => { setObjectFilter(v); setPage(1); }}
            options={objectFilterOptions}
          />
          <Select
            placeholder="错误信息"
            allowClear
            style={{ width: 130 }}
            value={errorFilter}
            onChange={(v) => { setErrorFilter(v); setPage(1); }}
            options={errorFilterOptions}
          />
          <Button onClick={resetFilters}>重置</Button>
        </Space>
      </div>

      {error ? (
        <div style={{ textAlign: 'center', padding: '32px 0' }}>
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description={<Text type="secondary">文件详情加载失败</Text>}
          />
          <Button style={{ marginTop: 16 }} onClick={() => fetchData()}>重试</Button>
        </div>
      ) : list.length === 0 && !loading ? (
        <Empty
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          description={<Text type="secondary">暂无文件详情记录</Text>}
        />
      ) : (
        <Table
          className="task-detail-table"
          dataSource={list}
          columns={columns}
          rowKey="id"
          loading={loading}
          scroll={{ x: 1410 }}
          pagination={{
            current: page,
            pageSize,
            total,
            showSizeChanger: true,
            pageSizeOptions: ['10', '20', '50', '100'],
            onChange: (p, ps) => { setPage(p); setPageSize(ps); },
            showTotal: (t) => `共 ${t} 条`,
          }}
          size="middle"
        />
      )}
    </div>
  );

  if (embedded) {
    return <div>{content}</div>;
  }

  return (
    <Card className="page-card">
      {content}
    </Card>
  );
}
