import { useState, useEffect, useCallback, useMemo } from 'react';
import './Home.css';
import { Table, Tag, Button, Space, Select, Progress, Empty, Typography, Card, Tooltip, Input } from 'antd';
import { ArrowLeftOutlined, InfoCircleOutlined } from '@ant-design/icons';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { jobGetTaskItem } from '../../api/job';
import dayjs from 'dayjs';
import type { TaskItem } from '../../types';

const { Text } = Typography;

const taskItemStatusList = [
  '等待中', '进行中', '成功', '取消中', '已取消',
  '出错（将重试）', '失败中', '已失败', '等待重试中', '等待重试前',
];

const statusColors: Record<number, string> = {
  0: 'default', 1: 'processing', 2: 'success', 3: 'warning',
  4: 'default', 5: 'error', 6: 'error', 7: 'error', 8: 'default', 9: 'default',
};

const typeNames: Record<number, string> = {
  0: '复制', 1: '删除', 2: '移动',
};

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

function formatSize(val: number | null): string {
  if (val == null) return '--';
  if (val === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  let size = val;
  while (size >= 1024 && i < units.length - 1) {
    size /= 1024;
    i++;
  }
  return `${size.toFixed(i === 0 ? 0 : 2)} ${units[i]}`;
}

function displayText(value: string | number | null | undefined): string {
  if (value === null || value === undefined || value === '') return '--';
  return String(value);
}

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
  const [statusFilter, setStatusFilter] = useState<number | undefined>(undefined);
  const [typeFilter, setTypeFilter] = useState<number | undefined>(undefined);
  const [objectFilter, setObjectFilter] = useState<number | undefined>(undefined);
  const [errorFilter, setErrorFilter] = useState<number | undefined>(undefined);
  const [keywordInput, setKeywordInput] = useState('');
  const [keywordFilter, setKeywordFilter] = useState('');

  const fetchData = useCallback(async () => {
    if (!taskId) return;
    setLoading(true);
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
      const res = await jobGetTaskItem(params);
      const data = res.data;
      const items = (data?.dataList || []).map((item) => {
        const prog = typeof item.progress === 'string' ? parseInt(item.progress, 10) : (item.progress || 0);
        return { ...item, progress: Math.min(prog || 0, 100) };
      });
      setList(items);
      setTotal(data?.count || 0);
    } catch {
      /* ignore */
    }
    setLoading(false);
  }, [errorFilter, keywordFilter, objectFilter, page, pageSize, statusFilter, taskId, typeFilter]);

  useEffect(() => { fetchData(); }, [fetchData]);

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
      render: (val: number | null) => formatSize(val),
    },
    {
      title: '操作类型',
      dataIndex: 'type',
      key: 'type',
      width: 100,
      render: (val: number, record: TaskItem) => {
        const label = val === 0 && record.isPath ? '创建' : (typeNames[val] || String(val));
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
          <Tag color={statusColors[status]}>
            {taskItemStatusList[status] || String(status)}
          </Tag>
        );
        if (statusColors[status] !== 'error' || !errorReason) {
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
            <Button icon={<ArrowLeftOutlined />} onClick={() => (onBack ? onBack() : navigate(-1 as never))}>返回</Button>
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

      {list.length === 0 && !loading ? (
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
