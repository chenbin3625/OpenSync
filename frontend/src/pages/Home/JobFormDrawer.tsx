import React, { useState, useEffect, useRef } from 'react';
import { HelpCircle } from 'lucide-react';
import { jobPost } from '../../api/job';
import type { AlistItem, JobItem } from '../../types';
import { fileSizeToBytes, fileSizeUnitOptions, splitBytesToFileSize } from './fileSizeUnits';
import {
  methodOptions, methodNames, cronTypeNames, cronFields, defaultCronFields,
  defaultExclude, parseJobPathList, normalizeFormPaths,
  formatSchedulePlan, formatAlistLabel, type ScheduleValues,
} from './homeUtils';
import { usePathTree } from './usePathTree';
import { Button } from '../../components/ui/button';
import { Input } from '../../components/ui/input';
import { Switch } from '../../components/ui/switch';
import { PathTreeSelect } from '../../components/ui/path-tree-select';
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetFooter,
} from '../../components/ui/sheet';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '../../components/ui/tooltip';

export interface JobFormDrawerProps {
  visible: boolean;
  editingJob: JobItem | null;
  alistList: AlistItem[];
  onClose: () => void;
  onSubmit: () => void;
}

export const cronFieldRanges: Record<string, [number, number]> = {
  second: [0, 59],
  minute: [0, 59],
  hour: [0, 23],
  day: [1, 31],
  month: [1, 12],
  day_of_week: [0, 6],
};

export function validateCronField(value: string, min: number, max: number): Promise<void> {
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

export default function JobFormDrawer({
  visible, editingJob, alistList, onClose, onSubmit,
}: JobFormDrawerProps) {
  const [treeLoading, setTreeLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const submittingRef = useRef(false);
  const submitAbortRef = useRef<AbortController | null>(null);
  const treeLoadRequestRef = useRef(0);

  // 表单受控状态
  const [alistId, setAlistId] = useState<number | undefined>(undefined);
  const [srcPath, setSrcPath] = useState<string | string[]>([]);
  const [dstPath, setDstPath] = useState<string | string[]>([]);
  const [remark, setRemark] = useState('');
  const [method, setMethod] = useState(0);
  const [isCron, setIsCron] = useState(1);
  const [interval, setIntervalVal] = useState(1440);
  const [enable, setEnable] = useState(true);

  // Cron 6 字段
  const [cronState, setCronState] = useState({ ...defaultCronFields });

  // 大小限制
  const [minFileSize, setMinFileSize] = useState(0);
  const [minFileSizeUnit, setMinFileSizeUnit] = useState('MB');
  const [maxFileSize, setMaxFileSize] = useState(0);
  const [maxFileSizeUnit, setMaxFileSizeUnit] = useState('MB');

  // 缓存与扫描
  const [useCacheS, setUseCacheS] = useState(false);
  const [scanIntervalS, setScanIntervalS] = useState(0);
  const [useCacheT, setUseCacheT] = useState(false);
  const [scanIntervalT, setScanIntervalT] = useState(0);

  // 排除规则
  const [exclude, setExclude] = useState(defaultExclude);
  const [formError, setFormError] = useState('');

  const editingJobId = editingJob?.id;
  const editingJobSrcPath = editingJob?.srcPath;
  const editingJobDstPath = editingJob?.dstPath;

  const {
    treeData: srcTreeData,
    loadRoot: loadSrcRoot,
    clearTree: clearSrcTree,
    onLoadData: onLoadSrcData,
    treeLoadError: srcTreeError,
  } = usePathTree(alistId, treeLoadRequestRef);

  const {
    treeData: dstTreeData,
    loadRoot: loadDstRoot,
    clearTree: clearDstTree,
    onLoadData: onLoadDstData,
    treeLoadError: dstTreeError,
  } = usePathTree(alistId, treeLoadRequestRef);

  const treeNotFoundContent = alistId
    ? (srcTreeError || dstTreeError ? '目录加载失败，请检查引擎连接' : '请展开目录选择')
    : '请先选择引擎';

  // 引擎切换时加载根目录
  useEffect(() => {
    if (alistId) {
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
  }, [alistId, editingJobId, editingJobDstPath, editingJobSrcPath, loadSrcRoot, loadDstRoot, clearSrcTree, clearDstTree]);

  // 打开弹窗时初始化表单值
  useEffect(() => {
    if (!visible) return;
    submittingRef.current = false;
    setSubmitting(false);
    setFormError('');

    if (editingJob) {
      setAlistId(editingJob.alistId);
      setSrcPath(parseJobPathList(editingJob.srcPath));
      setDstPath(parseJobPathList(editingJob.dstPath));
      setRemark(editingJob.remark || '');
      setMethod(editingJob.method ?? 0);
      setIsCron(editingJob.isCron ?? 1);
      setIntervalVal(editingJob.interval ?? 1440);
      setEnable(editingJob.enable === 1);
      setUseCacheS(Boolean(editingJob.useCacheS));
      setUseCacheT(Boolean(editingJob.useCacheT));
      setScanIntervalS(editingJob.scanIntervalS ?? 0);
      setScanIntervalT(editingJob.scanIntervalT ?? 0);
      setExclude(editingJob.exclude ?? defaultExclude);

      const minObj = splitBytesToFileSize(editingJob.minFileSize);
      setMinFileSize(minObj.value);
      setMinFileSizeUnit(minObj.unit);

      const maxObj = splitBytesToFileSize(editingJob.maxFileSize);
      setMaxFileSize(maxObj.value);
      setMaxFileSizeUnit(maxObj.unit);

      setCronState({
        second: editingJob.second || defaultCronFields.second,
        minute: editingJob.minute || defaultCronFields.minute,
        hour: editingJob.hour || defaultCronFields.hour,
        day: editingJob.day || defaultCronFields.day,
        month: editingJob.month || defaultCronFields.month,
        day_of_week: editingJob.day_of_week || defaultCronFields.day_of_week,
      });
    } else {
      setAlistId(alistList[0]?.id);
      setSrcPath([]);
      setDstPath([]);
      setRemark('');
      setMethod(0);
      setIsCron(1);
      setIntervalVal(1440);
      setEnable(true);
      setUseCacheS(false);
      setUseCacheT(false);
      setScanIntervalS(0);
      setScanIntervalT(0);
      setMinFileSize(0);
      setMinFileSizeUnit('MB');
      setMaxFileSize(0);
      setMaxFileSizeUnit('MB');
      setExclude(defaultExclude);
      setCronState({ ...defaultCronFields });
      clearSrcTree();
      clearDstTree();
    }
  }, [visible, editingJob, alistList, clearSrcTree, clearDstTree]);

  // 取消提交
  useEffect(() => {
    if (visible) return undefined;
    submitAbortRef.current?.abort();
    submitAbortRef.current = null;
    submittingRef.current = false;
    setSubmitting(false);
    return undefined;
  }, [visible]);

  // 调度方案文本预览
  const scheduleValues: ScheduleValues = {
    isCron,
    interval,
    ...cronState,
  };
  const schedulePlan = formatSchedulePlan(scheduleValues);

  // 校验 Cron
  const validateCron = async () => {
    if (isCron !== 1) return true;
    for (const [key, range] of Object.entries(cronFieldRanges)) {
      const val = (cronState as Record<string, string>)[key];
      try {
        await validateCronField(val, range[0], range[1]);
      } catch (err) {
        setFormError(`Cron 字段 [${key}] 校验失败: ${(err as Error).message}`);
        return false;
      }
    }
    return true;
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (submittingRef.current) return;
    setFormError('');

    if (!alistId) {
      setFormError('请选择存储引擎');
      return;
    }
    const normalizedSrc = normalizeFormPaths(srcPath);
    const normalizedDst = normalizeFormPaths(dstPath);
    if (normalizedSrc.length === 0) {
      setFormError('请选择源目录');
      return;
    }
    if (normalizedDst.length === 0) {
      setFormError('请选择目标目录');
      return;
    }

    const cronValid = await validateCron();
    if (!cronValid) return;

    // 大小校验
    const minBytes = fileSizeToBytes(minFileSize, minFileSizeUnit);
    const maxBytes = fileSizeToBytes(maxFileSize, maxFileSizeUnit);
    if (maxBytes > 0 && minBytes > maxBytes) {
      setFormError('最大文件大小必须大于等于最小文件大小');
      return;
    }

    submittingRef.current = true;
    setSubmitting(true);
    const controller = new AbortController();
    submitAbortRef.current = controller;

    try {
      const jobData: Record<string, unknown> = {
        ...(editingJob ? { id: editingJob.id } : {}),
        alistId,
        remark: remark.trim() || null,
        srcPath: normalizedSrc,
        dstPath: normalizedDst,
        method,
        isCron,
        interval,
        enable: isCron === 2 ? 1 : (enable ? 1 : 0),
        useCacheS: useCacheS ? 1 : 0,
        useCacheT: useCacheT ? 1 : 0,
        scanIntervalS: Number(scanIntervalS),
        scanIntervalT: Number(scanIntervalT),
        minFileSize: minBytes,
        maxFileSize: maxBytes,
        exclude: exclude.trim() || null,
        ...cronState,
      };

      await jobPost(jobData, { signal: controller.signal });
      if (controller.signal.aborted) return;
      onSubmit();
    } catch (err) {
      if (controller.signal.aborted) return;
      setFormError((err as Error).message || '保存任务失败');
      console.error('job submit failed', err);
    } finally {
      if (submitAbortRef.current === controller) {
        submitAbortRef.current = null;
        submittingRef.current = false;
        setSubmitting(false);
      }
    }
  };

  return (
    <TooltipProvider>
      <Sheet open={visible} onOpenChange={(open) => !open && onClose()}>
        <SheetContent side="right" className="w-full sm:max-w-xl overflow-y-auto p-6 space-y-6">
          <SheetHeader>
            <SheetTitle>{editingJob ? '编辑同步任务' : '新建同步任务'}</SheetTitle>
          </SheetHeader>

          {formError && (
            <div className="p-3 rounded-lg bg-rose-50 border border-rose-200 text-rose-700 text-xs font-medium">
              {formError}
            </div>
          )}

          <form onSubmit={handleSubmit} className="space-y-5">
            {/* 引擎选择 */}
            <div className="space-y-1.5">
              <label className="text-xs font-medium text-slate-700 block">
                存储引擎 <span className="text-red-500">*</span>
              </label>
              <select
                value={alistId ?? ''}
                onChange={(e) => setAlistId(Number(e.target.value))}
                className="flex h-9 w-full rounded-md border border-slate-300 bg-white px-3 py-1 text-sm shadow-sm text-slate-800 focus:outline-none focus:ring-1 focus:ring-teal-600"
                required
              >
                <option value="" disabled>请选择引擎</option>
                {alistList.map((a) => (
                  <option key={a.id} value={a.id}>
                    {formatAlistLabel(a, { includeUrl: true })}
                  </option>
                ))}
              </select>
            </div>

            {/* 源与目标目录树 */}
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <label className="text-xs font-medium text-slate-700 flex items-center justify-between">
                  <span>源目录 <span className="text-red-500">*</span></span>
                  <span className="text-[11px] text-slate-400">支持多选</span>
                </label>
                <PathTreeSelect
                  placeholder="选择源目录..."
                  value={srcPath}
                  onChange={setSrcPath}
                  treeData={srcTreeData}
                  loadData={onLoadSrcData}
                  loading={treeLoading}
                  notFoundContent={treeNotFoundContent}
                />
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-medium text-slate-700 flex items-center justify-between">
                  <span>目标目录 <span className="text-red-500">*</span></span>
                  <span className="text-[11px] text-slate-400">支持多选</span>
                </label>
                <PathTreeSelect
                  placeholder="选择目标目录..."
                  value={dstPath}
                  onChange={setDstPath}
                  treeData={dstTreeData}
                  loadData={onLoadDstData}
                  loading={treeLoading}
                  notFoundContent={treeNotFoundContent}
                />
              </div>
            </div>

            {/* 备注 */}
            <div className="space-y-1.5">
              <label className="text-xs font-medium text-slate-700 block">任务备注</label>
              <Input
                placeholder="例如：相册每日备份 / 电影库镜像"
                value={remark}
                onChange={(e) => setRemark(e.target.value)}
              />
            </div>

            {/* 同步配置分隔 */}
            <div className="pt-2 border-t border-slate-200">
              <h3 className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-3">同步与调度</h3>

              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                {/* 同步方式 */}
                <div className="space-y-1.5">
                  <div className="flex items-center gap-1.5">
                    <label className="text-xs font-medium text-slate-700">同步方式</label>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <HelpCircle className="h-3.5 w-3.5 text-slate-400 cursor-help" />
                      </TooltipTrigger>
                      <TooltipContent className="max-w-xs space-y-1 text-xs">
                        {methodOptions.map((opt) => (
                          <div key={opt.name}>
                            <strong>{opt.name}：</strong>{opt.description}
                          </div>
                        ))}
                      </TooltipContent>
                    </Tooltip>
                  </div>
                  <select
                    value={method}
                    onChange={(e) => setMethod(Number(e.target.value))}
                    className="flex h-9 w-full rounded-md border border-slate-300 bg-white px-3 py-1 text-sm shadow-sm text-slate-800"
                  >
                    {methodNames.map((name, idx) => (
                      <option key={name} value={idx}>{name}</option>
                    ))}
                  </select>
                </div>

                {/* 调度方式 */}
                <div className="space-y-1.5">
                  <div className="flex items-center gap-1.5">
                    <label className="text-xs font-medium text-slate-700">调度方式</label>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <HelpCircle className="h-3.5 w-3.5 text-slate-400 cursor-help" />
                      </TooltipTrigger>
                      <TooltipContent className="text-xs">
                        当前计划：{schedulePlan}
                      </TooltipContent>
                    </Tooltip>
                  </div>
                  <select
                    value={isCron}
                    onChange={(e) => {
                      const val = Number(e.target.value);
                      setIsCron(val);
                      if (val === 2) setEnable(true);
                      if (val === 0) setIntervalVal(1440);
                    }}
                    className="flex h-9 w-full rounded-md border border-slate-300 bg-white px-3 py-1 text-sm shadow-sm text-slate-800"
                  >
                    {cronTypeNames.map((name, idx) => (
                      <option key={name} value={idx}>{name}</option>
                    ))}
                  </select>
                </div>
              </div>

              {/* 分钟间隔配置 */}
              {isCron === 0 && (
                <div className="mt-3 space-y-1.5">
                  <label className="text-xs font-medium text-slate-700 block">执行间隔（分钟）</label>
                  <Input
                    type="number"
                    min={1}
                    value={interval}
                    onChange={(e) => setIntervalVal(Number(e.target.value))}
                    required
                  />
                </div>
              )}

              {/* Cron 6 字段配置 */}
              {isCron === 1 && (
                <div className="mt-3 space-y-1.5 bg-slate-50 p-3 rounded-lg border border-slate-200/80">
                  <span className="text-[11px] font-semibold text-slate-500 block">
                    Cron 表达式配置 (秒 分 时 日 月 周)
                  </span>
                  <div className="grid grid-cols-6 gap-1.5">
                    {cronFields.map((field) => (
                      <div key={field.name} className="space-y-0.5 text-center">
                        <label className="text-[10px] text-slate-400 block truncate">{field.label}</label>
                        <input
                          type="text"
                          value={(cronState as Record<string, string>)[field.name]}
                          onChange={(e) =>
                            setCronState((prev) => ({ ...prev, [field.name]: e.target.value }))
                          }
                          className="h-7 w-full text-center font-mono text-xs rounded border border-slate-200 bg-white focus:outline-none focus:border-teal-600"
                          placeholder={field.placeholder}
                          required
                        />
                      </div>
                    ))}
                  </div>
                  <p className="text-[10px] text-teal-700 font-mono pt-1 truncate">
                    预估计划: {schedulePlan}
                  </p>
                </div>
              )}
            </div>

            {/* 文件大小过滤 */}
            <div className="pt-2 border-t border-slate-200 space-y-3">
              <h3 className="text-xs font-semibold text-slate-400 uppercase tracking-wider">文件过滤</h3>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div className="space-y-1.5">
                  <label className="text-xs font-medium text-slate-700 flex items-center gap-1">
                    <span>最小文件大小</span>
                    <span className="text-[11px] text-slate-400 font-normal">(0 不限)</span>
                  </label>
                  <div className="flex items-center gap-1">
                    <Input
                      type="number"
                      name="minFileSize"
                      min={0}
                      step="any"
                      value={minFileSize}
                      onChange={(e) => setMinFileSize(Number(e.target.value))}
                    />
                    <select
                      value={minFileSizeUnit}
                      onChange={(e) => setMinFileSizeUnit(e.target.value)}
                      className="h-9 rounded-md border border-slate-300 bg-white px-2 text-xs"
                    >
                      {fileSizeUnitOptions.map((opt) => (
                        <option key={opt.value} value={opt.value}>{opt.label}</option>
                      ))}
                    </select>
                  </div>
                </div>

                <div className="space-y-1.5">
                  <label className="text-xs font-medium text-slate-700 flex items-center gap-1">
                    <span>最大文件大小</span>
                    <span className="text-[11px] text-slate-400 font-normal">(0 不限)</span>
                  </label>
                  <div className="flex items-center gap-1">
                    <Input
                      type="number"
                      name="maxFileSize"
                      min={0}
                      step="any"
                      value={maxFileSize}
                      onChange={(e) => setMaxFileSize(Number(e.target.value))}
                    />
                    <select
                      value={maxFileSizeUnit}
                      onChange={(e) => setMaxFileSizeUnit(e.target.value)}
                      className="h-9 rounded-md border border-slate-300 bg-white px-2 text-xs"
                    >
                      {fileSizeUnitOptions.map((opt) => (
                        <option key={opt.value} value={opt.value}>{opt.label}</option>
                      ))}
                    </select>
                  </div>
                </div>
              </div>

              {/* 排除项 */}
              <div className="space-y-1.5">
                <label className="text-xs font-medium text-slate-700 flex items-center gap-1">
                  <span>排除规则 (.gitignore 格式)</span>
                  <span className="text-[11px] text-slate-400 font-normal">每行一条规则</span>
                </label>
                <textarea
                  rows={4}
                  value={exclude}
                  onChange={(e) => setExclude(e.target.value)}
                  className="w-full rounded-md border border-slate-300 bg-white p-2.5 font-mono text-xs shadow-sm focus:outline-none focus:ring-1 focus:ring-teal-600"
                  placeholder={'如\n*.tmp\n.git/'}
                />
              </div>
            </div>

            {/* 缓存与扫描设置 */}
            <div className="pt-2 border-t border-slate-200 space-y-3">
              <h3 className="text-xs font-semibold text-slate-400 uppercase tracking-wider">缓存加速</h3>
              <div className="grid grid-cols-2 gap-4">
                <div className="flex items-center justify-between p-3 rounded-lg bg-slate-50 border border-slate-100">
                  <span className="text-xs font-medium text-slate-700">源端缓存</span>
                  <Switch checked={useCacheS} onCheckedChange={setUseCacheS} />
                </div>
                <div className="flex items-center justify-between p-3 rounded-lg bg-slate-50 border border-slate-100">
                  <span className="text-xs font-medium text-slate-700">目标缓存</span>
                  <Switch checked={useCacheT} onCheckedChange={setUseCacheT} />
                </div>
              </div>
            </div>

            {/* 启用开关 */}
            <div className="pt-2 border-t border-slate-200 flex items-center justify-between p-3.5 rounded-lg bg-slate-50 border border-slate-100">
              <div className="space-y-0.5">
                <span className="text-sm font-medium text-slate-800">任务启用状态</span>
                <p className="text-xs text-slate-400">是否开启定时自动触发（手动任务默认始终保持就绪）</p>
              </div>
              <Switch
                disabled={isCron === 2}
                checked={isCron === 2 ? true : enable}
                onCheckedChange={setEnable}
              />
            </div>

            <SheetFooter className="pt-4 border-t border-slate-200">
              <Button type="button" variant="outline" onClick={onClose}>
                取消
              </Button>
              <Button type="submit" loading={submitting}>
                保存任务配置
              </Button>
            </SheetFooter>
          </form>
        </SheetContent>
      </Sheet>
    </TooltipProvider>
  );
}
