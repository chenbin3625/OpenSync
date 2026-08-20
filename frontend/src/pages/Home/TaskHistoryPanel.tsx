import { useMemo } from 'react';
import Button from 'antd/es/button';
import DatePicker from 'antd/es/date-picker';
import Empty from 'antd/es/empty';
import Input from 'antd/es/input';
import Popconfirm from 'antd/es/popconfirm';
import Select from 'antd/es/select';
import Space from 'antd/es/space';
import Table from 'antd/es/table';
import Tag from 'antd/es/tag';
import Tooltip from 'antd/es/tooltip';
import Typography from 'antd/es/typography';
import datePickerZhCN from 'antd/es/date-picker/locale/zh_CN';
import {
  DeleteOutlined, EyeOutlined, InfoCircleOutlined, RedoOutlined, ReloadOutlined,
} from '@ant-design/icons';
import dayjs from 'dayjs';
import type { Dayjs } from 'dayjs';
import type { TaskRecord } from '../../types';
import { taskRecordStatusNames, taskStatusColors } from './homeUtils';

const { Text } = Typography;

export type HistoryTimeRange = [Dayjs | null, Dayjs | null] | null;

const historyStatusOptions = [2, 3, 4, 5, 6, 7, 8].map((value) => ({
  value,
  label: taskRecordStatusNames[value],
}));

export default function TaskHistoryPanel({
  error,
  loading,
  rows,
  total,
  page,
  pageSize,
  keywordInput,
  statusFilter,
  timeRange,
  hasFilters,
  onKeywordInputChange,
  onKeywordSearch,
  onStatusFilterChange,
  onTimeRangeChange,
  onPageChange,
  onResetFilters,
  onRetry,
  onTaskDetail,
  onRetryTask,
  onDeleteTask,
}: {
  error: boolean;
  loading: boolean;
  rows: TaskRecord[];
  total: number;
  page: number;
  pageSize: number;
  keywordInput: string;
  statusFilter: number | undefined;
  timeRange: HistoryTimeRange;
  hasFilters: boolean;
  onKeywordInputChange: (value: string) => void;
  onKeywordSearch: (value: string) => void;
  onStatusFilterChange: (value: number | undefined) => void;
  onTimeRangeChange: (value: HistoryTimeRange) => void;
  onPageChange: (page: number, pageSize: number) => void;
  onResetFilters: () => void;
  onRetry: () => void;
  onTaskDetail?: (taskId: number) => void;
  onRetryTask: (taskId: number) => void;
  onDeleteTask: (taskId: number) => void;
}) {
  const columns = useMemo(() => [
    {
      title: '状态', dataIndex: 'status', key: 'status', width: 120,
      render: (s: number, record: TaskRecord) => {
        const errorReason = typeof record.errMsg === 'string' ? record.errMsg.trim() : '';
        const statusTag = (
          <Tag color={taskStatusColors[s]}>{taskRecordStatusNames[s] || s}</Tag>
        );
        if (taskStatusColors[s] !== 'error' || !errorReason) {
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
      title: '开始时间', dataIndex: 'runTime', key: 'runTime',
      render: (t: number) => t ? dayjs.unix(t).format('YYYY-MM-DD HH:mm:ss') : '-',
    },
    {
      title: '成功', dataIndex: 'successNum', key: 'successNum', width: 80,
      render: (v: number) => v ?? '-',
    },
    {
      title: '失败', dataIndex: 'failNum', key: 'failNum', width: 80,
      render: (v: number) => v ?? '-',
    },
    { title: '总计', dataIndex: 'allNum', key: 'allNum', width: 80 },
    {
      title: '操作', key: 'action', width: 136,
      render: (_: unknown, record: TaskRecord) => (
        <Space size={4} wrap>
          <Tooltip title="详情">
            <Button
              size="small"
              type="text"
              icon={<EyeOutlined />}
              aria-label="详情"
              onClick={() => onTaskDetail?.(record.id)}
            />
          </Tooltip>
          {(record.allNum || 0) > (record.successNum || 0) && (
            <Tooltip title="重试未完成项">
              <Button
                size="small"
                type="text"
                icon={<RedoOutlined />}
                aria-label="重试未完成项"
                onClick={() => onRetryTask(record.id)}
              />
            </Tooltip>
          )}
          {(record.status !== 0 && record.status !== 1) && (
            <Tooltip title="删除">
              <Popconfirm title="确认删除此任务？" onConfirm={() => onDeleteTask(record.id)}>
                <Button
                  size="small"
                  type="text"
                  danger
                  icon={<DeleteOutlined />}
                  aria-label="删除"
                />
              </Popconfirm>
            </Tooltip>
          )}
        </Space>
      ),
    },
  ], [onDeleteTask, onRetryTask, onTaskDetail]);

  const historyBody = error ? (
    <div className="ops-state-block">
      <Empty
        image={Empty.PRESENTED_IMAGE_SIMPLE}
        description={<Text type="secondary">历史任务加载失败</Text>}
      />
      <Button icon={<ReloadOutlined />} className="ops-state-action" onClick={onRetry} loading={loading}>
        重试
      </Button>
    </div>
  ) : rows.length === 0 && !loading ? (
    <Empty
      image={Empty.PRESENTED_IMAGE_SIMPLE}
      description={<Text type="secondary">暂无历史任务记录，执行完成后将在此显示</Text>}
    />
  ) : (
    <Table
      dataSource={rows}
      columns={columns}
      rowKey="id"
      loading={loading}
      pagination={{
        current: page,
        pageSize,
        total,
        onChange: onPageChange,
        showSizeChanger: true,
        showTotal: (t) => `共 ${t} 条`,
      }}
      size="middle"
    />
  );

  return (
    <div className="task-history-panel">
      <Space wrap className="task-history-filters">
        <Input.Search
          placeholder="任务 ID"
          allowClear
          style={{ width: 180 }}
          value={keywordInput}
          onChange={(event) => {
            onKeywordInputChange(event.target.value);
            if (!event.target.value) onKeywordSearch('');
          }}
          onSearch={onKeywordSearch}
        />
        <Select
          placeholder="任务状态"
          allowClear
          style={{ width: 140 }}
          value={statusFilter}
          onChange={(value) => onStatusFilterChange(value)}
          options={historyStatusOptions}
        />
        <DatePicker.RangePicker
          className="task-history-time-range"
          locale={datePickerZhCN}
          value={timeRange || undefined}
          onChange={(value) => onTimeRangeChange((value as HistoryTimeRange) || null)}
          placeholder={['开始日期', '结束日期']}
        />
        <Button onClick={onResetFilters} disabled={!hasFilters}>重置</Button>
      </Space>
      {historyBody}
    </div>
  );
}
