import { useState, useEffect, useRef } from 'react';
import {
  Button, Space, Drawer, Select, Input, Form, Switch, InputNumber,
  Row, Col, Divider, TreeSelect, Spin, Tooltip,
} from 'antd';
import { QuestionCircleOutlined } from '@ant-design/icons';
import { jobPost } from '../../api/job';
import type { AlistItem, JobFormValues, JobItem, TreeNode } from '../../types';
import { fileSizeToBytes, fileSizeUnitOptions, splitBytesToFileSize } from './fileSizeUnits';
import {
  methodOptions, methodNames, cronTypeNames, cronFields, defaultCronFields,
  compactItemStyle, compactDividerStyle, defaultExclude,
  parseJobPathList, normalizeFormPaths,
  formatSchedulePlan, formatAlistLabel, type ScheduleValues,
} from './homeUtils';
import { usePathTree } from './usePathTree';

export interface JobFormDrawerProps {
  visible: boolean;
  editingJob: JobItem | null;
  alistList: AlistItem[];
  onClose: () => void;
  onSubmit: () => void;
}

// Per-field value ranges for the six cron fields (robfig/cron with seconds).
const cronFieldRanges: Record<string, [number, number]> = {
  second: [0, 59],
  minute: [0, 59],
  hour: [0, 23],
  day: [1, 31],
  month: [1, 12],
  day_of_week: [0, 6],
};

// Validates a single cron field value against its allowed range. Supports the
// common robfig/cron syntax: *, */n, n, n-m, n-m/s, and comma-separated lists.
function validateCronField(value: string, min: number, max: number): Promise<void> {
  if (!value) return Promise.resolve();
  const parts = value.split(',');
  for (const part of parts) {
    const trimmed = part.trim();
    if (trimmed === '*') continue;
    const stepMatch = trimmed.match(/^\*\/(\d+)$/);
    if (stepMatch) {
      const step = Number(stepMatch[1]);
      if (step < 1 || step > max) return Promise.reject(new Error(`步长需在 1-${max}`));
      continue;
    }
    const rangeMatch = trimmed.match(/^(\d+)-(\d+)(?:\/(\d+))?$/);
    if (rangeMatch) {
      const a = Number(rangeMatch[1]);
      const b = Number(rangeMatch[2]);
      if (a < min || a > max || b < min || b > max || a > b) {
        return Promise.reject(new Error(`范围需在 ${min}-${max}`));
      }
      if (rangeMatch[3]) {
        const step = Number(rangeMatch[3]);
        if (step < 1 || step > max) return Promise.reject(new Error(`步长需在 1-${max}`));
      }
      continue;
    }
    if (/^\d+$/.test(trimmed)) {
      const n = Number(trimmed);
      if (n < min || n > max) return Promise.reject(new Error(`值需在 ${min}-${max}`));
      continue;
    }
    return Promise.reject(new Error('仅支持数字、* / , - 组合'));
  }
  return Promise.resolve();
}

const cronFieldRules = (fieldName: string) => {
  const range = cronFieldRanges[fieldName];
  if (!range) return [{ required: true, message: '请输入' }];
  return [
    { required: true, message: '请输入' },
    { pattern: /^[\d*,/-]+$/, message: '仅支持数字、* / , -' },
    {
      validator: (_: unknown, value: string) => validateCronField(value, range[0], range[1]),
    },
  ];
};

export default function JobFormDrawer({
  visible, editingJob, alistList, onClose, onSubmit,
}: JobFormDrawerProps) {
  const [form] = Form.useForm();
  const [treeLoading, setTreeLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const submittingRef = useRef(false);
  const submitAbortRef = useRef<AbortController | null>(null);
  const treeLoadRequestRef = useRef(0);

  const selectedAlistId = Form.useWatch('alistId', form) as number | undefined;
  const editingJobId = editingJob?.id;
  const editingJobSrcPath = editingJob?.srcPath;
  const editingJobDstPath = editingJob?.dstPath;
  const {
    treeData: srcTreeData,
    loadRoot: loadSrcRoot,
    clearTree: clearSrcTree,
    onLoadData: onLoadSrcData,
    setLoadedKeys: setSrcLoadedKeys,
  } = usePathTree(selectedAlistId, treeLoadRequestRef);
  const {
    treeData: dstTreeData,
    loadRoot: loadDstRoot,
    clearTree: clearDstTree,
    onLoadData: onLoadDstData,
    setLoadedKeys: setDstLoadedKeys,
  } = usePathTree(selectedAlistId, treeLoadRequestRef);

  useEffect(() => {
    if (selectedAlistId) {
      const requestID = ++treeLoadRequestRef.current;
      setTreeLoading(true);
      Promise.all([
        loadSrcRoot(editingJobSrcPath),
        loadDstRoot(editingJobDstPath),
      ]).finally(() => {
        if (requestID === treeLoadRequestRef.current) setTreeLoading(false);
      });
    } else {
      treeLoadRequestRef.current += 1;
      clearSrcTree();
      clearDstTree();
      setTreeLoading(false);
    }
  }, [selectedAlistId, editingJobId, editingJobDstPath, editingJobSrcPath, loadSrcRoot, loadDstRoot, clearSrcTree, clearDstTree]);

  // Initialize form when opening
  useEffect(() => {
    if (!visible) return;
    submittingRef.current = false;
    setSubmitting(false);
    form.resetFields();
    if (editingJob) {
      const minFileSize = splitBytesToFileSize(editingJob.minFileSize);
      const maxFileSize = splitBytesToFileSize(editingJob.maxFileSize);
      form.setFieldsValue({
        ...editingJob,
        enable: editingJob.enable === 1,
        useCacheS: editingJob.useCacheS === 1 || editingJob.useCacheS === true,
        useCacheT: editingJob.useCacheT === 1 || editingJob.useCacheT === true,
        srcPath: parseJobPathList(editingJob.srcPath),
        dstPath: parseJobPathList(editingJob.dstPath),
        second: editingJob.second || defaultCronFields.second,
        minute: editingJob.minute || defaultCronFields.minute,
        hour: editingJob.hour || defaultCronFields.hour,
        day: editingJob.day || defaultCronFields.day,
        month: editingJob.month || defaultCronFields.month,
        day_of_week: editingJob.day_of_week || defaultCronFields.day_of_week,
        minFileSize: minFileSize.value,
        minFileSizeUnit: minFileSize.unit,
        maxFileSize: maxFileSize.value,
        maxFileSizeUnit: maxFileSize.unit,
      });
      setSrcLoadedKeys([]);
      setDstLoadedKeys([]);
    } else {
      form.setFieldsValue({
        enable: true,
        method: 0,
        isCron: 1,
        interval: 1440,
        useCacheS: false,
        useCacheT: false,
        scanIntervalS: 0,
        scanIntervalT: 0,
        minFileSize: 0,
        minFileSizeUnit: 'MB',
        maxFileSize: 0,
        maxFileSizeUnit: 'MB',
        ...defaultCronFields,
        exclude: defaultExclude,
      });
      clearSrcTree();
      clearDstTree();
      setSrcLoadedKeys([]);
      setDstLoadedKeys([]);
    }
  }, [visible, editingJob, form, clearSrcTree, clearDstTree, setSrcLoadedKeys, setDstLoadedKeys]);

  useEffect(() => {
    if (visible) return undefined;
    submitAbortRef.current?.abort();
    submitAbortRef.current = null;
    submittingRef.current = false;
    setSubmitting(false);
    return undefined;
  }, [visible]);

  const handleSubmit = async () => {
    if (submittingRef.current) return;
    let values: JobFormValues;
    try {
      values = await form.validateFields() as JobFormValues;
    } catch {
      // Validation errors are shown inline by Ant Design; nothing else to do.
      return;
    }
    submittingRef.current = true;
    setSubmitting(true);
    const controller = new AbortController();
    submitAbortRef.current = controller;
    try {
      const srcPaths = normalizeFormPaths(values.srcPath);
      const dstPaths = normalizeFormPaths(values.dstPath);
      const { minFileSizeUnit, maxFileSizeUnit, ...jobValues } = values;
      const jobData: Record<string, unknown> = {
        ...(editingJob ? { id: editingJob.id } : {}),
        ...jobValues,
        srcPath: srcPaths,
        dstPath: dstPaths,
        enable: values.isCron === 2 ? 1 : (values.enable ? 1 : 0),
        useCacheS: values.useCacheS ? 1 : 0,
        useCacheT: values.useCacheT ? 1 : 0,
        minFileSize: fileSizeToBytes(values.minFileSize, minFileSizeUnit),
        maxFileSize: fileSizeToBytes(values.maxFileSize, maxFileSizeUnit),
      };
      await jobPost(jobData, { signal: controller.signal });
      if (controller.signal.aborted) return;
      onSubmit();
    } catch (err) {
      if (controller.signal.aborted) return;
      // API errors are surfaced by the request interceptor; log unexpected
      // (non-API) failures so they are not silently swallowed.
      console.error('job submit failed', err);
    } finally {
      if (submitAbortRef.current === controller) {
        submitAbortRef.current = null;
        submittingRef.current = false;
        setSubmitting(false);
      }
    }
  };

  // Schedule preview — watch only the schedule fields so typing in the exclude
  // textarea or remark no longer re-renders the whole drawer.
  const isCronValue = Form.useWatch('isCron', form) as number | undefined;
  const intervalValue = Form.useWatch('interval', form) as number | undefined;
  const secondValue = Form.useWatch('second', form) as string | undefined;
  const minuteValue = Form.useWatch('minute', form) as string | undefined;
  const hourValue = Form.useWatch('hour', form) as string | undefined;
  const dayValue = Form.useWatch('day', form) as string | undefined;
  const monthValue = Form.useWatch('month', form) as string | undefined;
  const dayOfWeekValue = Form.useWatch('day_of_week', form) as string | undefined;
  const scheduleValues: ScheduleValues = {
    isCron: isCronValue ?? 1,
    interval: intervalValue,
    second: secondValue,
    minute: minuteValue,
    hour: hourValue,
    day: dayValue,
    month: monthValue,
    day_of_week: dayOfWeekValue,
  };
  const schedulePlan = formatSchedulePlan(scheduleValues);

  useEffect(() => {
    if (isCronValue === 2 && form.getFieldValue('enable') !== true) {
      form.setFieldsValue({ enable: true });
    }
  }, [form, isCronValue]);

  return (
    <Drawer
      className="sync-job-drawer"
      title={editingJob ? '编辑同步任务' : '新建同步任务'}
      open={visible}
      onClose={onClose}
      forceRender
      styles={{ wrapper: { width: 580 }, body: { padding: 16 } }}
      extra={
        <Space>
          <Button onClick={onClose}>取消</Button>
          <Button type="primary" onClick={handleSubmit} loading={submitting} disabled={submitting}>保存</Button>
        </Space>
      }
    >
      <Form className="sync-job-form" form={form} layout="vertical">
        <Form.Item name="alistId" label="引擎" rules={[{ required: true, message: '请选择引擎' }]} style={compactItemStyle}>
          <Select
            placeholder="选择引擎"
            options={alistList.map((a) => ({
              value: a.id,
              label: formatAlistLabel(a, { includeUrl: true }),
            }))}
          />
        </Form.Item>
        <Row gutter={12}>
          <Col span={12}>
            <Form.Item name="srcPath" label="源目录" rules={[{ required: true, message: '请选择源目录' }]} style={compactItemStyle}>
              <TreeSelect
                placeholder="选择源目录"
                treeData={srcTreeData}
                loadData={(node) => onLoadSrcData(node as TreeNode)}
                treeDefaultExpandAll
                multiple
                treeCheckable
                showCheckedStrategy={TreeSelect.SHOW_PARENT}
                maxTagCount="responsive"
                allowClear
                showSearch
                treeNodeFilterProp="title"
                styles={{ popup: { root: { maxHeight: 300, overflow: 'auto' } } }}
                suffixIcon={treeLoading ? <Spin size="small" /> : undefined}
                notFoundContent={selectedAlistId ? '请先展开目录' : '请先选择引擎'}
              />
            </Form.Item>
          </Col>
          <Col span={12}>
            <Form.Item name="dstPath" label="目标目录" rules={[{ required: true, message: '请选择目标目录' }]} style={compactItemStyle}>
              <TreeSelect
                placeholder="选择目标目录"
                treeData={dstTreeData}
                loadData={(node) => onLoadDstData(node as TreeNode)}
                treeDefaultExpandAll
                multiple
                maxTagCount="responsive"
                allowClear
                showSearch
                treeNodeFilterProp="title"
                styles={{ popup: { root: { maxHeight: 300, overflow: 'auto' } } }}
                suffixIcon={treeLoading ? <Spin size="small" /> : undefined}
                notFoundContent={selectedAlistId ? '请先展开目录' : '请先选择引擎'}
              />
            </Form.Item>
          </Col>
        </Row>
        <Form.Item name="remark" label="备注" style={compactItemStyle}>
          <Input placeholder="可选备注" />
        </Form.Item>

        <Divider style={compactDividerStyle}>同步配置</Divider>
        <Row gutter={12}>
          <Col span={12}>
            <Form.Item
              name="method"
              style={compactItemStyle}
              label={(
                <Space size={4}>
                  同步方式
                  <Tooltip
                    title={(
                      <Space direction="vertical" size={2}>
                        {methodOptions.map((method) => (
                          <span key={method.name}>
                            <strong>{method.name}：</strong>{method.description}
                          </span>
                        ))}
                      </Space>
                    )}
                  >
                    <QuestionCircleOutlined />
                  </Tooltip>
                </Space>
              )}
            >
              <Select options={methodNames.map((n, i) => ({ value: i, label: n }))} />
            </Form.Item>
          </Col>
          <Col span={12}>
            <Form.Item
              name="isCron"
              style={compactItemStyle}
              label={(
                <Space size={4}>
                  调度方式
                  <Tooltip title={`预计执行计划：${schedulePlan}`}>
                    <QuestionCircleOutlined />
                  </Tooltip>
                </Space>
              )}
            >
              <Select
                options={cronTypeNames.map((n, i) => ({ value: i, label: n }))}
                onChange={(value) => {
                  if (value === 0) form.setFieldsValue({ interval: 1440 });
                  if (value === 2) form.setFieldsValue({ enable: true });
                  if (value === 1) {
                    form.setFieldsValue({
                      second: form.getFieldValue('second') || defaultCronFields.second,
                      minute: form.getFieldValue('minute') === '*' ? defaultCronFields.minute : form.getFieldValue('minute') || defaultCronFields.minute,
                      hour: form.getFieldValue('hour') === '*' ? defaultCronFields.hour : form.getFieldValue('hour') || defaultCronFields.hour,
                      day: form.getFieldValue('day') || defaultCronFields.day,
                      month: form.getFieldValue('month') || defaultCronFields.month,
                      day_of_week: form.getFieldValue('day_of_week') || defaultCronFields.day_of_week,
                    });
                  }
                }}
              />
            </Form.Item>
          </Col>
          <Col span={12}>
            {isCronValue === 0 && (
              <Form.Item name="interval" label="间隔(分钟)" rules={[{ required: true, message: '请输入' }]} style={compactItemStyle}>
                <InputNumber min={1} style={{ width: '100%' }} />
              </Form.Item>
            )}
          </Col>
        </Row>
        {isCronValue === 1 && (
          <Row gutter={8}>
            {cronFields.map((field) => (
              <Col span={4} key={field.name}>
                <Form.Item name={field.name} label={field.label} rules={cronFieldRules(field.name)} style={compactItemStyle}>
                  <Input placeholder={field.placeholder} />
                </Form.Item>
              </Col>
            ))}
          </Row>
        )}
        <Row gutter={12}>
          <Col span={12}>
            <Form.Item
              label="最小文件大小"
              tooltip="0 表示不限制小文件"
              style={compactItemStyle}
            >
              <Space.Compact style={{ width: '100%' }}>
                <Form.Item name="minFileSize" noStyle>
                  <InputNumber
                    min={0}
                    precision={2}
                    style={{ flex: 1 }}
                    placeholder="0 不限"
                  />
                </Form.Item>
                <Form.Item name="minFileSizeUnit" noStyle>
                  <Select
                    options={fileSizeUnitOptions}
                    style={{ width: 84 }}
                    onChange={() => form.validateFields(['maxFileSize']).catch(() => undefined)}
                  />
                </Form.Item>
              </Space.Compact>
            </Form.Item>
          </Col>
          <Col span={12}>
            <Form.Item
              label="最大文件大小"
              tooltip="0 表示不限制大文件"
              style={compactItemStyle}
            >
              <Space.Compact style={{ width: '100%' }}>
                <Form.Item
                  name="maxFileSize"
                  noStyle
                  dependencies={['minFileSize', 'minFileSizeUnit', 'maxFileSizeUnit']}
                  rules={[
                    ({ getFieldValue }) => ({
                      validator(_, value) {
                        const minSize = fileSizeToBytes(getFieldValue('minFileSize'), getFieldValue('minFileSizeUnit'));
                        const maxSize = fileSizeToBytes(value, getFieldValue('maxFileSizeUnit'));
                        if (maxSize > 0 && minSize > maxSize) {
                          return Promise.reject(new Error('最大文件大小必须大于等于最小文件大小'));
                        }
                        return Promise.resolve();
                      },
                    }),
                  ]}
                >
                  <InputNumber
                    min={0}
                    precision={2}
                    style={{ flex: 1 }}
                    placeholder="0 不限"
                  />
                </Form.Item>
                <Form.Item name="maxFileSizeUnit" noStyle>
                  <Select
                    options={fileSizeUnitOptions}
                    style={{ width: 84 }}
                    onChange={() => form.validateFields(['maxFileSize']).catch(() => undefined)}
                  />
                </Form.Item>
              </Space.Compact>
            </Form.Item>
          </Col>
        </Row>

        <Divider style={compactDividerStyle}>缓存与扫描</Divider>
        <Row gutter={12}>
          <Col span={6}>
            <Form.Item name="useCacheS" label="源端缓存" valuePropName="checked" style={compactItemStyle}>
              <Switch checkedChildren="使用" unCheckedChildren="不使用" />
            </Form.Item>
          </Col>
          <Col span={6}>
            <Form.Item name="scanIntervalS" label="源端扫描(秒)" style={compactItemStyle}>
              <InputNumber min={0} style={{ width: '100%' }} placeholder="0 默认" />
            </Form.Item>
          </Col>
          <Col span={6}>
            <Form.Item name="useCacheT" label="目标缓存" valuePropName="checked" style={compactItemStyle}>
              <Switch checkedChildren="使用" unCheckedChildren="不使用" />
            </Form.Item>
          </Col>
          <Col span={6}>
            <Form.Item name="scanIntervalT" label="目标扫描(秒)" style={compactItemStyle}>
              <InputNumber min={0} style={{ width: '100%' }} placeholder="0 默认" />
            </Form.Item>
          </Col>
        </Row>

        <div style={{ marginTop: 4 }}>
          <Form.Item name="exclude" label="排除项" tooltip="gitignore 语法，每行一条" style={compactItemStyle}>
            <Input.TextArea placeholder={'如\n*.tmp\n.git/'} rows={8} />
          </Form.Item>
        </div>

        <Divider style={compactDividerStyle} />
        <Form.Item name="enable" label="启用" valuePropName="checked" style={compactItemStyle}>
          <Switch disabled={isCronValue === 2} />
        </Form.Item>
      </Form>
    </Drawer>
  );
}
