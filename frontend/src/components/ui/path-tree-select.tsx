import React, { useState, useMemo } from 'react';
import { ChevronRight, ChevronDown, Folder, FolderOpen, X, Check, Search, Loader2, RefreshCw } from 'lucide-react';
import * as PopoverPrimitive from '@radix-ui/react-popover';
import { cn } from '../../lib/utils';
import { badge } from '../../lib/styles';
import { Tooltip } from './tooltip';
import type { TreeNode } from '../../types';

export interface PathTreeSelectProps {
  /** 供外部 label 的 htmlFor 关联触发器 */
  id?: string;
  placeholder?: string;
  value?: string | string[];
  onChange?: (value: string | string[]) => void;
  treeData: TreeNode[];
  loadData?: (node: TreeNode) => Promise<void>;
  loadedKeys?: React.Key[];
  onRefresh?: () => Promise<unknown>;
  loadError?: boolean;
  multiple?: boolean;
  disabled?: boolean;
  loading?: boolean;
  notFoundContent?: React.ReactNode;
  className?: string;
}

interface TreeNodeItemProps {
  node: TreeNode;
  selectedValues: string[];
  onToggleSelect: (path: string) => void;
  loadData?: (node: TreeNode) => Promise<void>;
  loadedKeys: React.Key[];
  search: string;
}

function TreeNodeItem({
  node,
  selectedValues,
  onToggleSelect,
  loadData,
  loadedKeys,
  search,
}: TreeNodeItemProps) {
  const [expanded, setExpanded] = useState(false);
  const [loadingChildren, setLoadingChildren] = useState(false);

  const isSelected = selectedValues.includes(node.value);

  const handleExpand = async (e: React.MouseEvent) => {
    e.stopPropagation();
    if (!expanded && !loadedKeys.includes(node.value) && loadData) {
      setLoadingChildren(true);
      try {
        await loadData(node);
      } finally {
        setLoadingChildren(false);
      }
    }
    setExpanded((prev) => !prev);
  };

  const matchesSearch = !search || node.title.toLowerCase().includes(search.toLowerCase());

  return (
    <div className="min-w-max text-sm select-none">
      <div
        className={cn(
          'flex items-center gap-1.5 py-1 px-2 rounded-md hover:bg-line-soft cursor-pointer transition-colors',
          isSelected && 'bg-line hover:bg-line text-teal-900 font-medium'
        )}
        onClick={() => onToggleSelect(node.value)}
      >
        <button
          type="button"
          aria-label={`${expanded ? '折叠' : '展开'} ${node.title || node.value}`}
          aria-expanded={expanded}
          disabled={loadingChildren}
          className="shrink-0 p-0.5 rounded hover:bg-line text-slate-400 hover:text-slate-700 transition-colors"
          onClick={handleExpand}
        >
          {loadingChildren ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin text-teal-600" />
          ) : expanded ? (
            <ChevronDown className="h-3.5 w-3.5" />
          ) : (
            <ChevronRight className="h-3.5 w-3.5" />
          )}
        </button>

        <div className="flex items-center justify-center h-4 w-4 rounded border border-line-strong mr-0.5 shrink-0 transition-colors">
          {isSelected && <Check className="h-3 w-3 text-teal-700 stroke-[3]" />}
        </div>

        {expanded ? (
          <FolderOpen className="h-4 w-4 text-teal-600 shrink-0" />
        ) : (
          <Folder className="h-4 w-4 text-slate-400 shrink-0" />
        )}

        <span title={node.value} className={cn('whitespace-nowrap', matchesSearch ? 'text-slate-800' : 'text-slate-400')}>
          {node.title || node.value}
        </span>
      </div>

      {expanded && node.children && node.children.length > 0 && (
        <div className="pl-5 border-l border-line ml-3.5 mt-0.5 space-y-0.5">
          {node.children.map((child) => (
            <TreeNodeItem
              key={child.value}
              node={child}
              selectedValues={selectedValues}
              onToggleSelect={onToggleSelect}
              loadData={loadData}
              loadedKeys={loadedKeys}
              search={search}
            />
          ))}
        </div>
      )}
    </div>
  );
}

export function PathTreeSelect({
  id,
  placeholder = '请选择目录',
  value,
  onChange,
  treeData,
  loadData,
  loadedKeys = [],
  onRefresh,
  loadError = false,
  multiple = true,
  disabled = false,
  loading = false,
  notFoundContent = '请先展开或选择引擎',
  className,
}: PathTreeSelectProps) {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState('');
  const [refreshing, setRefreshing] = useState(false);
  const [treeVersion, setTreeVersion] = useState(0);

  const handleRefresh = async () => {
    if (!onRefresh || refreshing) return;
    setRefreshing(true);
    try {
      await onRefresh();
      setTreeVersion((prev) => prev + 1);
    } finally {
      setRefreshing(false);
    }
  };

  const selectedValues = useMemo<string[]>(() => {
    if (!value) return [];
    if (Array.isArray(value)) return value.filter(Boolean);
    return [value];
  }, [value]);

  const handleToggleSelect = (path: string) => {
    if (multiple) {
      const next = selectedValues.includes(path)
        ? selectedValues.filter((p) => p !== path)
        : [...selectedValues, path];
      onChange?.(next);
    } else {
      onChange?.(path);
      setOpen(false);
    }
  };

  const handleRemoveTag = (e: React.MouseEvent, path: string) => {
    e.stopPropagation();
    if (multiple) {
      onChange?.(selectedValues.filter((p) => p !== path));
    } else {
      onChange?.('');
    }
  };

  return (
    <PopoverPrimitive.Root open={open} onOpenChange={setOpen}>
      <PopoverPrimitive.Trigger asChild disabled={disabled}>
        <div
          id={id}
          role="button"
          tabIndex={disabled ? -1 : 0}
          className={cn(
            'min-h-9 w-full rounded-md border border-line-strong bg-surface px-3 py-1.5 text-sm shadow-sm flex items-center justify-between gap-2 cursor-pointer transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-teal-600 focus-visible:border-teal-600',
            disabled && 'cursor-not-allowed opacity-50 bg-tint',
            className
          )}
        >
          <div className="flex flex-wrap items-center gap-1.5 min-w-0 flex-1">
            {selectedValues.length === 0 ? (
              <span className="text-slate-400 select-none text-sm">{placeholder}</span>
            ) : (
              // 已选路径胶囊与全局标签共用同一形状与中性色调，仅保留删除按钮所需的内距
              selectedValues.map((path) => (
                <span key={path} className={cn(badge.base, badge.tone.neutral, 'max-w-[200px] pr-1')}>
                  <span className="truncate" title={path}>
                    {path}
                  </span>
                  {!disabled && (
                    <button
                      type="button"
                      onClick={(e) => handleRemoveTag(e, path)}
                      className="hover:text-slate-900 rounded p-0.5 transition-colors"
                    >
                      <X className="h-3 w-3" />
                    </button>
                  )}
                </span>
              ))
            )}
          </div>

          <div className="flex items-center gap-1 shrink-0 text-slate-400">
            {loading ? (
              <Loader2 className="h-4 w-4 animate-spin text-teal-600" />
            ) : (
              <ChevronDown className="h-4 w-4 opacity-60" />
            )}
          </div>
        </div>
      </PopoverPrimitive.Trigger>

      <PopoverPrimitive.Portal>
        <PopoverPrimitive.Content
          sideOffset={4}
          align="start"
          className="z-50 w-[var(--radix-popover-trigger-width)] max-w-[min(500px,calc(100vw-1rem))] rounded-lg border border-line bg-surface p-2.5 shadow-xl animate-in fade-in-0 zoom-in-95 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95"
        >
          <div className="relative mb-2 flex items-center gap-2">
            <Search className="absolute left-2.5 h-3.5 w-3.5 text-slate-400 pointer-events-none" />
            <input
              type="text"
              placeholder="快速搜索路径..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="h-8 min-w-0 flex-1 rounded-md border border-line bg-tint pl-8 pr-3 text-xs placeholder:text-slate-400 focus:outline-none focus:border-teal-600 focus:bg-surface transition-colors"
            />
            {onRefresh && (
              <Tooltip title="刷新目录">
                <button
                  type="button"
                  aria-label="刷新目录"
                  disabled={loading || refreshing}
                  onClick={() => void handleRefresh()}
                  className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md hover:bg-line-soft disabled:opacity-50"
                >
                  <RefreshCw className={cn('h-4 w-4', refreshing && 'animate-spin')} />
                </button>
              </Tooltip>
            )}
          </div>

          {loadError && <div role="alert" className="mb-2 text-xs text-rose-700">目录加载失败，请重试或刷新</div>}
          <div className="max-h-64 overflow-auto pr-1 space-y-0.5">
            {treeData.length === 0 ? (
              <div className="py-6 text-center text-xs text-slate-400">
                {notFoundContent}
              </div>
            ) : (
              treeData.map((node) => (
                <TreeNodeItem
                  key={`${treeVersion}:${node.value}`}
                  node={node}
                  selectedValues={selectedValues}
                  onToggleSelect={handleToggleSelect}
                  loadData={loadData}
                  loadedKeys={loadedKeys}
                  search={search}
                />
              ))
            )}
          </div>
        </PopoverPrimitive.Content>
      </PopoverPrimitive.Portal>
    </PopoverPrimitive.Root>
  );
}
