import { useState, useEffect, useRef, useCallback } from 'react';
import type { Key } from 'react';
import {
  Button, Space, Drawer, Select, Input, Form, Switch, InputNumber,
  Row, Col, Divider, TreeSelect, Spin, Tooltip,
} from 'antd';
import { QuestionCircleOutlined } from '@ant-design/icons';
import { jobPost } from '../../api/job';
import { alistGetPath } from '../../api/alist';
import type { AlistItem, JobFormValues, JobItem, PathItem, TreeNode } from '../../types';
import { fileSizeToBytes, fileSizeUnitOptions, splitBytesToFileSize } from './fileSizeUnits';
import {
  methodOptions, methodNames, cronTypeNames, cronFields, defaultCronFields,
  compactItemStyle, compactDividerStyle, defaultExclude,
  parseJobSrcPaths, parseJobDstPaths, normalizeFormPaths,
  formatSchedulePlan, type ScheduleValues,
} from './homeUtils';

export interface JobFormDrawerProps {
  visible: boolean;
  editingJob: JobItem | null;
  alistList: AlistItem[];
  onClose: () => void;
  onSubmit: () => void;
}

export default function JobFormDrawer({
  visible, editingJob, alistList, onClose, onSubmit,
}: JobFormDrawerProps) {
  const [form] = Form.useForm();
  const [srcTreeData, setSrcTreeData] = useState<TreeNode[]>([]);
  const [dstTreeData, setDstTreeData] = useState<TreeNode[]>([]);
  const [srcLoadedKeys, setSrcLoadedKeys] = useState<Key[]>([]);
  const [dstLoadedKeys, setDstLoadedKeys] = useState<Key[]>([]);
  const [treeLoading, setTreeLoading] = useState(false);
  const treeLoadRequestRef = useRef(0);

  const selectedAlistId = Form.useWatch('alistId', form) as number | undefined;

  // Tree data helpers
  const fetchDirChildren = useCallback(async (alistId: number, parentPath: string): Promise<TreeNode[]> => {
    if (!alistId) return [];
    try {
      const res = await alistGetPath(alistId, parentPath);
      const items = res.data || [];
      return (Array.isArray(items) ? items : []).map((item: PathItem) => {
        const name = item.path || item.name || '';
        const fullPath = parentPath === '/' ? `/${name}` : `${parentPath}/${name}`;
        return {
          title: name,
          value: fullPath,
          key: fullPath,
          isLeaf: false,
        };
      });
    } catch {
      return [];
    }
  }, []);

  const updateTreeChildren = (tree: TreeNode[], parentValue: string, children: TreeNode[]): TreeNode[] => {
    return tree.map((node) => {
      if (node.value === parentValue) {
        return { ...node, children };
      }
      if (node.children) {
        return { ...node, children: updateTreeChildren(node.children, parentValue, children) };
      }
      return node;
    });
  };

  // Load root when engine changes
  useEffect(() => {
    if (selectedAlistId) {
      const requestID = ++treeLoadRequestRef.current;
      setTreeLoading(true);
      setSrcLoadedKeys([]);
      setDstLoadedKeys([]);
      fetchDirChildren(selectedAlistId, '/').then((nodes) => {
        if (requestID !== treeLoadRequestRef.current) return;
        const root = [{ title: '/', value: '/', key: '/', children: nodes }];
        setSrcTreeData(root);
        setDstTreeData(structuredClone(root));
      }).finally(() => {
        if (requestID === treeLoadRequestRef.current) setTreeLoading(false);
      });
    } else {
      treeLoadRequestRef.current += 1;
      setSrcTreeData([]);
      setDstTreeData([]);
      setTreeLoading(false);
    }
  }, [selectedAlistId, fetchDirChildren]);

  const onLoadSrcData = async (node: TreeNode) => {
    if (!selectedAlistId || srcLoadedKeys.includes(node.value)) return;
    const alistId = selectedAlistId;
    const requestID = treeLoadRequestRef.current;
    const children = await fetchDirChildren(alistId, node.value);
    if (requestID !== treeLoadRequestRef.current || selectedAlistId !== alistId) return;
    setSrcTreeData((prev) => updateTreeChildren(prev, node.value, children));
    setSrcLoadedKeys((prev) => [...prev, node.value]);
  };

  const onLoadDstData = async (node: TreeNode) => {
    if (!selectedAlistId || dstLoadedKeys.includes(node.value)) return;
    const alistId = selectedAlistId;
    const requestID = treeLoadRequestRef.current;
    const children = await fetchDirChildren(alistId, node.value);
    if (requestID !== treeLoadRequestRef.current || selectedAlistId !== alistId) return;
    setDstTreeData((prev) => updateTreeChildren(prev, node.value, children));
    setDstLoadedKeys((prev) => [...prev, node.value]);
  };

  // Initialize form when opening
  useEffect(() => {
    if (!visible) return;
    form.resetFields();
    if (editingJob) {
      const minFileSize = splitBytesToFileSize(editingJob.minFileSize);
      const maxFileSize = splitBytesToFileSize(editingJob.maxFileSize);
      form.setFieldsValue({
        ...editingJob,
        enable: editingJob.enable === 1,
        useCacheS: editingJob.useCacheS === 1 || editingJob.useCacheS === true,
        useCacheT: editingJob.useCacheT === 1 || editingJob.useCacheT === true,
        srcPath: parseJobSrcPaths(editingJob.srcPath),
        dstPath: parseJobDstPaths(editingJob.dstPath),
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
      setSrcTreeData([]);
      setDstTreeData([]);
      setSrcLoadedKeys([]);
      setDstLoadedKeys([]);
    }
  }, [visible, editingJob, form]);

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields() as JobFormValues;
      const srcPaths = normalizeFormPaths(values.srcPath);
      const dstPaths = normalizeFormPaths(values.dstPath);
      const { minFileSizeUnit, maxFileSizeUnit, ...jobValues } = values;
      const jobData: Record<string, unknown> = {
        ...(editingJob ? { id: editingJob.id } : {}),
        ...jobValues,
        srcPath: srcPaths,
        dstPath: dstPaths,
        enable: values.enable ? 1 : 0,
        useCacheS: values.useCacheS ? 1 : 0,
        useCacheT: values.useCacheT ? 1 : 0,
        minFileSize: fileSizeToBytes(values.minFileSize, minFileSizeUnit),
        maxFileSize: fileSizeToBytes(values.maxFileSize, maxFileSizeUnit),
      };
      await jobPost(jobData);
      onSubmit();
    } catch { /* ignore */ }
  };

  // Schedule preview
  const watchedSchedule = Form.useWatch([], form) as Record<string, unknown> | undefined;
  const isCronValue = watchedSchedule?.isCron as number | undefined;
  const scheduleValues: ScheduleValues = {
    isCron: (watchedSchedule?.isCron as number) ?? 1,
    interval: watchedSchedule?.interval as number,
    second: watchedSchedule?.second as string,
    minute: watchedSchedule?.minute as string,
    hour: watchedSchedule?.hour as string,
    day: watchedSchedule?.day as string,
    month: watchedSchedule?.month as string,
    day_of_week: watchedSchedule?.day_of_week as string,
  };
  const schedulePlan = formatSchedulePlan(scheduleValues);

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
          <Button type="primary" onClick={handleSubmit}>保存</Button>
        </Space>
      }
    >
      <Form className="sync-job-form" form={form} layout="vertical">
        <Form.Item name="alistId" label="引擎" rules={[{ required: true, message: '请选择引擎' }]} style={compactItemStyle}>
          <Select
            placeholder="选择引擎"
            options={alistList.map((a) => ({
              value: a.id,
              label: `${a.userName} - ${a.url}${a.remark ? ` (${a.remark})` : ''}`,
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
                <Form.Item name={field.name} label={field.label} rules={[{ required: true, message: '请输入' }]} style={compactItemStyle}>
                  <Input placeholder={field.placeholder} />
                </Form.Item>
              </Col>
            ))}
          </Row>
        )}
        <Row gutter={12}>
          <Col span={12}>
            <Form.Item
              name="minFileSize"
              label="最小文件大小"
              tooltip="0 表示不限制小文件"
              style={compactItemStyle}
            >
              <Space.Compact style={{ width: '100%' }}>
                <InputNumber
                  min={0}
                  precision={2}
                  style={{ flex: 1 }}
                  placeholder="0 不限"
                />
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
              name="maxFileSize"
              label="最大文件大小"
              tooltip="0 表示不限制大文件"
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
              style={compactItemStyle}
            >
              <Space.Compact style={{ width: '100%' }}>
                <InputNumber
                  min={0}
                  precision={2}
                  style={{ flex: 1 }}
                  placeholder="0 不限"
                />
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
          <Switch />
        </Form.Item>
      </Form>
    </Drawer>
  );
}
