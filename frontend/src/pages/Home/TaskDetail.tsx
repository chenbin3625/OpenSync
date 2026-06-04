import { useState, useEffect, useCallback } from 'react';
import { Table, Tag, Button, Space, Select, Popover, Progress, Empty, Typography, Card } from 'antd';
import { ArrowLeftOutlined, FileSearchOutlined } from '@ant-design/icons';
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

const statusFilterOptions = [
  { label: '成功', value: 2 },
  { label: '失败', value: 7 },
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
  }, [page, pageSize, statusFilter, taskId]);

  useEffect(() => { fetchData(); }, [fetchData]);

  const columns = [
    {
      title: '文件名/目录',
      dataIndex: 'fileName',
      key: 'fileName',
      render: (_: unknown, record: TaskItem) =>
        record.fileName || record.dstPath || '--',
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
        if (val === 0) return record.isPath ? '创建' : '复制';
        return typeNames[val] || String(val);
      },
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 140,
      render: (status: number, record: TaskItem) => {
        if (status === 1) {
          const pct = Number(record.progress || 0);
          return <Progress percent={pct} size="small" />;
        }
        if (status === 7 && record.errMsg) {
          return (
            <Popover title="错误原因" content={record.errMsg} trigger="hover">
              <Tag color={statusColors[status]}>失败，<span style={{ color: '#1677ff', cursor: 'pointer' }}>原因</span></Tag>
            </Popover>
          );
        }
        return (
          <Tag color={statusColors[status]}>
            {taskItemStatusList[status] || String(status)}
          </Tag>
        );
      },
    },
  ];

  const expandedRowRender = (record: TaskItem) => (
    <div style={{ padding: '8px 0' }}>
      {record.type !== 1 && (
        <div style={{ marginBottom: 8 }}>
          <Text strong>来源目录：</Text>
          <Text style={{ wordBreak: 'break-all' }}>{record.srcPath || '--'}</Text>
        </div>
      )}
      <div style={{ marginBottom: 8 }}>
        <Text strong>目标目录：</Text>
        <Text style={{ wordBreak: 'break-all' }}>{record.dstPath || '--'}</Text>
      </div>
      <div>
        <Text strong>创建时间：</Text>
        <Text>
          {record.createTime
            ? dayjs.unix(record.createTime).format('YYYY-MM-DD HH:mm:ss')
            : '--'}
        </Text>
      </div>
    </div>
  );

  const content = (
    <>
      <div className="page-header">
        {embedded ? (
          <span />
        ) : (
          <Space>
            <Button icon={<ArrowLeftOutlined />} onClick={() => (onBack ? onBack() : navigate(-1 as never))}>返回</Button>
            <h2>任务详情</h2>
          </Space>
        )}
        <Space>
          <Select
            placeholder="筛选状态"
            allowClear
            style={{ width: 160 }}
            value={statusFilter}
            onChange={(v) => { setStatusFilter(v); setPage(1); }}
            options={statusFilterOptions}
          />
        </Space>
      </div>

      {list.length === 0 && !loading ? (
        <Empty
          image={<FileSearchOutlined style={{ fontSize: 64, color: '#bbb' }} />}
          styles={{ image: { height: 80 } }}
          description={<Text type="secondary">暂无文件详情记录</Text>}
          className="empty-state-compact"
        />
      ) : (
        <Table
          dataSource={list}
          columns={columns}
          rowKey="id"
          loading={loading}
          expandable={{
            expandedRowRender,
          }}
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
    </>
  );

  if (embedded) {
    return <div>{content}</div>;
  }

  return (
    <Card>
      {content}
    </Card>
  );
}
