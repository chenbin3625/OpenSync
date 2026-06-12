import { PlusOutlined, PlayCircleOutlined } from '@ant-design/icons';
import { Button, Tag, Pagination, Empty, Typography, Spin, Menu } from 'antd';
import type { JobItem } from '../../types';
import { jobStatusColors, statusLabels, methodNames, formatJobPaths, countJobPaths, getJobName, formatSchedule } from './homeUtils';

const { Text } = Typography;

export interface HomeSidebarProps {
  list: JobItem[];
  loading: boolean;
  selectedJobId: number | null;
  total: number;
  page: number;
  pageSize: number;
  onAdd: () => void;
  onRunAll: () => void;
  onSelectJob: (jobId: number) => void;
  onClearTaskDetail: () => void;
  setPage: (page: number) => void;
}

function renderSidebarJob(job: JobItem) {
  const sourcePreview = formatJobPaths(job.srcPath) || '源目录未配置';
  const targetCount = countJobPaths(job.dstPath);
  return (
    <div className="sync-sidebar-job">
      <div className="sync-sidebar-job-title">
        <span className="sync-sidebar-job-name">{getJobName(job)}</span>
        <Tag color={jobStatusColors[job.enable] || 'default'}>
          {statusLabels[job.enable] || '未知'}
        </Tag>
      </div>
      <div className="sync-sidebar-job-meta">
        <span>{methodNames[job.method] || job.method}</span>
        <span>{targetCount > 1 ? `${targetCount} 个目标` : formatSchedule(job)}</span>
      </div>
      <span className="sync-sidebar-job-path" title={sourcePreview}>
        {sourcePreview}
      </span>
    </div>
  );
}

export default function HomeSidebar({
  list, loading, selectedJobId, total, page, pageSize,
  onAdd, onRunAll, onSelectJob, onClearTaskDetail, setPage,
}: HomeSidebarProps) {
  return (
    <aside className="sync-manager-sidebar">
      <div className="sync-sidebar-toolbar">
        <Button type="primary" icon={<PlusOutlined />} onClick={onAdd}>新建</Button>
        <Button icon={<PlayCircleOutlined />} onClick={onRunAll}>执行全部</Button>
      </div>

      <div className="sync-sidebar-list">
        {list.length === 0 && !loading ? (
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description={<Text type="secondary">暂无同步任务</Text>}
          />
        ) : (
          <Spin spinning={loading && list.length === 0}>
            <Menu
              mode="inline"
              style={{ borderInlineEnd: 0 }}
              selectedKeys={selectedJobId ? [String(selectedJobId)] : []}
              onClick={({ key }) => {
                onSelectJob(Number(key));
                onClearTaskDetail();
              }}
              items={list.map((job) => ({
                key: String(job.id),
                label: renderSidebarJob(job),
              }))}
            />
          </Spin>
        )}
      </div>

      <div className="sync-sidebar-pagination">
        {total > pageSize && (
          <Pagination current={page} pageSize={pageSize} total={total} onChange={setPage} showSizeChanger={false} size="small" />
        )}
      </div>
    </aside>
  );
}
