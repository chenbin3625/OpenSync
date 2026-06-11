import { Card, Button, Tag, Space, Popconfirm, Switch, Empty, Typography, Descriptions, Tooltip } from 'antd';
import {
  CaretRightOutlined, EditOutlined, DeleteOutlined,
  CheckOutlined, PauseOutlined,
  CalendarOutlined, DatabaseOutlined,
} from '@ant-design/icons';
import type { JobItem } from '../../types';
import dayjs from 'dayjs';
import {
  statusColors, statusLabels, methodNames,
  formatJobPaths, countJobPaths, formatFileSizeRange, formatExcludePreview,
  getJobName, formatSchedule, formatCache,
} from './homeUtils';

const { Text, Title } = Typography;

function JobExcludeText({ value }: { value?: string | null }) {
  const text = String(value ?? '').trim();
  if (!text) return null;

  return (
    <Tooltip
      placement="topLeft"
      classNames={{ root: 'sync-job-exclude-tooltip' }}
      title={<pre className="sync-job-exclude-tooltip-content">{text}</pre>}
    >
      <Text type="secondary" className="sync-job-exclude-preview">
        {formatExcludePreview(text)}
      </Text>
    </Tooltip>
  );
}

export interface HomeOverviewProps {
  selectedJob: JobItem | null;
  onRun: (id: number) => void;
  onEdit: (job: JobItem) => void;
  onDelete: (id: number) => void;
  onToggle: (job: JobItem) => void;
  getAlistName: (alistId: number) => string;
}

export default function HomeOverview({
  selectedJob, onRun, onEdit, onDelete, onToggle, getAlistName,
}: HomeOverviewProps) {
  if (!selectedJob) {
    return (
      <Empty
        image={Empty.PRESENTED_IMAGE_SIMPLE}
        description={<Text type="secondary">请先在左侧选择同步任务</Text>}
      />
    );
  }

  const fileSizeRange = formatFileSizeRange(selectedJob.minFileSize, selectedJob.maxFileSize);

  return (
    <div className="sync-overview">
      <Card className="sync-overview-hero sync-overview-summary">
        <div className={`sync-status-badge ${selectedJob.enable === 1 ? 'is-enabled' : 'is-disabled'}`}>
          {selectedJob.enable === 1 ? <CheckOutlined /> : <PauseOutlined />}
        </div>
        <div className="sync-overview-title">
          <Title level={3} className="sync-overview-heading">{getJobName(selectedJob)}</Title>
          <Space size={6} wrap>
            <Tag color={statusColors[selectedJob.enable] || 'default'}>
              {statusLabels[selectedJob.enable] || '未知'}
            </Tag>
            <Tag>{methodNames[selectedJob.method] || selectedJob.method}</Tag>
            {selectedJob.isCron !== 2 && (
              <Switch
                checked={selectedJob.enable === 1}
                onChange={() => onToggle(selectedJob)}
                size="small"
              />
            )}
          </Space>
          <div className="sync-summary-stat-grid">
            <div className="sync-summary-stat">
              <span className="sync-summary-stat-label">源目录</span>
              <span className="sync-summary-stat-value">{countJobPaths(selectedJob.srcPath)} 个</span>
            </div>
            <div className="sync-summary-stat">
              <span className="sync-summary-stat-label">目标目录</span>
              <span className="sync-summary-stat-value">{countJobPaths(selectedJob.dstPath)} 个</span>
            </div>
            <div className="sync-summary-stat">
              <span className="sync-summary-stat-label">调度</span>
              <span className="sync-summary-stat-value">{formatSchedule(selectedJob)}</span>
            </div>
          </div>
        </div>
        <Space className="sync-overview-actions" wrap>
          <Button icon={<CaretRightOutlined />} onClick={() => onRun(selectedJob.id)}>手动执行</Button>
          <Button
            icon={<EditOutlined />}
            onClick={() => onEdit(selectedJob)}
          >
            编辑
          </Button>
          <Popconfirm title="确认删除此同步任务？" onConfirm={() => onDelete(selectedJob.id)}>
            <Button icon={<DeleteOutlined />} danger>删除</Button>
          </Popconfirm>
        </Space>
      </Card>

      <div className="sync-info-grid">
        <Card
          className="sync-info-card"
          title={<span className="ops-section-title"><DatabaseOutlined />连接信息</span>}
        >
          <Descriptions className="sync-job-descriptions" column={1} size="middle">
            <Descriptions.Item label="引擎">
              <Text type="secondary">{getAlistName(selectedJob.alistId)}</Text>
            </Descriptions.Item>
            <Descriptions.Item label="源目录">
              <Text ellipsis={{ tooltip: formatJobPaths(selectedJob.srcPath) }}>
                {formatJobPaths(selectedJob.srcPath)}
              </Text>
            </Descriptions.Item>
            <Descriptions.Item label="目标">
              <Text ellipsis={{ tooltip: formatJobPaths(selectedJob.dstPath, ' → ') }}>
                {formatJobPaths(selectedJob.dstPath, ' → ')}
              </Text>
            </Descriptions.Item>
          </Descriptions>
        </Card>

        <Card
          className="sync-info-card"
          title={<span className="ops-section-title"><CalendarOutlined />同步策略</span>}
        >
          <Descriptions className="sync-job-descriptions" column={1} size="middle">
            <Descriptions.Item label="调度">
              <Text type="secondary">{formatSchedule(selectedJob)}</Text>
            </Descriptions.Item>
            <Descriptions.Item label="缓存">
              <Text type="secondary">{formatCache(selectedJob.useCacheS, selectedJob.useCacheT)}</Text>
            </Descriptions.Item>
            {fileSizeRange ? (
              <Descriptions.Item label="大小范围">
                <Text type="secondary">{fileSizeRange}</Text>
              </Descriptions.Item>
            ) : null}
            {selectedJob.exclude ? (
              <Descriptions.Item label="排除">
                <JobExcludeText value={selectedJob.exclude} />
              </Descriptions.Item>
            ) : null}
            <Descriptions.Item label="创建">
              <Text type="secondary">
                {selectedJob.createTime ? dayjs.unix(selectedJob.createTime).format('YYYY-MM-DD HH:mm') : '-'}
              </Text>
            </Descriptions.Item>
          </Descriptions>
        </Card>
      </div>
    </div>
  );
}
