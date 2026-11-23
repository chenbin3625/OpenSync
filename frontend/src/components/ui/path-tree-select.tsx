import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
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
  loadData?: (node: TreeNode) => Promise<boolean | void>;
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
  depth: number;
  selectedValues: ReadonlySet<string>;
  onToggleSelect: (path: string) => void;
  loadData?: PathTreeSelectProps['loadData'];
  loadedKeys: ReadonlySet<React.Key>;
  search: string;
}

// Memoized: a keystroke in the search box only re-renders the tree once the
// debounced term changes, and unchanged subtrees are skipped entirely.
const TreeNodeItem = React.memo(function TreeNodeItem({
  node,
  depth,
  selectedValues,
  onToggleSelect,
  loadData,
  loadedKeys,
  search,
}: TreeNodeItemProps) {
  const [expanded, setExpanded] = useState(false);
  const [loadingChildren, setLoadingChildren] = useState(false);
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const isLeaf = Boolean(node.isLeaf);
  const isSelected = selectedValues.has(node.value);

  const handleExpand = async () => {
    if (isLeaf || loadingChildren) return;
    if (!expanded && !loadedKeys.has(node.value) && loadData) {
      setLoadingChildren(true);
      try {
        if ((await loadData(node)) === false) return;
      } catch (err) {
        console.error('path tree expand failed', err);
        return;
      } finally {
        if (mountedRef.current) {
          setLoadingChildren(false);
        }
      }
    }
    if (mountedRef.current) {
      setExpanded((prev) => !prev);
    }
  };

  const matchesSearch =
    !search ||
    (node.title || '').toLowerCase().includes(search) ||
    node.value.toLowerCase().includes(search);

  return (
    <div className="min-w-0 text-sm select-none">
      <div
        className={cn(
          'flex min-w-0 items-center gap-1.5 rounded-md pr-1 hover:bg-line-soft transition-colors',
          isSelected && 'bg-line hover:bg-line text-teal-900 font-medium'
        )}
      >
        {isLeaf ? (
          <span className="inline-block size-7 shrink-0" aria-hidden="true" />
        ) : (
          <button
            type="button"
            aria-label={`${expanded ? '折叠' : '展开'} ${node.title || node.value}`}
            aria-expanded={expanded}
            aria-disabled={loadingChildren || undefined}
            className="grid size-7 shrink-0 place-items-center rounded hover:bg-line text-slate-400 hover:text-slate-700 transition-colors focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-brand"
            onClick={() => void handleExpand()}
          >
            {loadingChildren ? (
              <Loader2 className="h-3.5 w-3.5 text-teal-600 motion-safe:animate-spin" aria-hidden="true" />
            ) : expanded ? (
              <ChevronDown className="h-3.5 w-3.5" aria-hidden="true" />
            ) : (
              <ChevronRight className="h-3.5 w-3.5" aria-hidden="true" />
            )}
          </button>
        )}

        <button
          type="button"
          aria-pressed={isSelected}
          onClick={() => onToggleSelect(node.value)}
          className="flex min-h-8 min-w-0 flex-1 items-center gap-1.5 rounded-sm py-1 text-left cursor-pointer focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-brand"
        >
          <span aria-hidden="true" className="grid size-4 shrink-0 place-items-center rounded border border-line-strong">
            {isSelected && <Check className="h-3 w-3 text-teal-700 stroke-[3]" />}
          </span>

          {!isLeaf && expanded ? (
            <FolderOpen className="h-4 w-4 text-teal-600 shrink-0" aria-hidden="true" />
          ) : (
            <Folder className="h-4 w-4 text-slate-400 shrink-0" aria-hidden="true" />
          )}

          <span title={node.value} className={cn('min-w-0 wrap-anywhere', matchesSearch ? 'text-slate-800' : 'text-slate-400')}>
            {node.title || node.value}
          </span>
        </button>
      </div>

      {!isLeaf && expanded && node.children && node.children.length > 0 && (
        <div className={cn('mt-0.5 space-y-0.5', depth < 6 && 'ml-2 border-l border-line pl-1.5')}>
          {node.children.map((child) => (
            <TreeNodeItem
              key={child.value}
              node={child}
              depth={depth + 1}
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
});

// Module-level default keeps the Set identity stable when a caller omits the
// prop, so memoized nodes are not invalidated by a fresh [] on every render.
const emptyLoadedKeys: React.Key[] = [];

export function PathTreeSelect({
  id,
  placeholder = '请选择目录',
  value,
  onChange,
  treeData,
  loadData,
  loadedKeys = emptyLoadedKeys,
  onRefresh,
  loadError = false,
  multiple = true,
  disabled = false,
  loading = false,
  notFoundContent = '请先展开或选择引擎',
  className,
}: PathTreeSelectProps) {
  const triggerRef = useRef<HTMLButtonElement>(null);
  const mountedRef = useRef(true);
  const [open, setOpen] = useState(false);
  // Keep the input responsive while only updating the (expensive) tree filter
  // after the user stops typing.
  const [searchInput, setSearchInput] = useState('');
  const [search, setSearch] = useState('');
  const [refreshing, setRefreshing] = useState(false);
  const [treeVersion, setTreeVersion] = useState(0);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  useEffect(() => {
    const timer = setTimeout(() => setSearch(searchInput.trim().toLowerCase()), 200);
    return () => clearTimeout(timer);
  }, [searchInput]);

  // Reset stale expanded state when the tree root options change or loadedKeys
  // are cleared (e.g. after switching engines or reloading the root directory).
  const rootKeySig = useMemo(() => treeData.map((n) => n.value).join('\0'), [treeData]);
  const prevLoadedCountRef = useRef(loadedKeys.length);
  const prevRootKeySigRef = useRef(rootKeySig);
  useEffect(() => {
    const loadedReset = prevLoadedCountRef.current > 0 && loadedKeys.length === 0;
    const rootChanged = prevRootKeySigRef.current !== '' && rootKeySig !== prevRootKeySigRef.current;
    prevLoadedCountRef.current = loadedKeys.length;
    prevRootKeySigRef.current = rootKeySig;
    if (loadedReset || rootChanged) {
      setTreeVersion((prev) => prev + 1);
    }
  }, [loadedKeys.length, rootKeySig]);

  const handleRefresh = async () => {
    if (!onRefresh || loading || refreshing) return;
    setRefreshing(true);
    try {
      await onRefresh();
      if (mountedRef.current) {
        setTreeVersion((prev) => prev + 1);
      }
    } catch (err) {
      console.error('path tree refresh failed', err);
    } finally {
      if (mountedRef.current) {
        setRefreshing(false);
      }
    }
  };

  const selectedValues = useMemo<string[]>(() => {
    if (!value) return [];
    if (Array.isArray(value)) return value.filter(Boolean);
    return [value];
  }, [value]);
  // Set lookups keep every node's render O(1) instead of scanning the
  // selection/loaded arrays; memoizing the sets also keeps TreeNodeItem memo
  // hits stable between renders.
  const selectedValueSet = useMemo(() => new Set(selectedValues), [selectedValues]);
  const loadedKeySet = useMemo(() => new Set(loadedKeys), [loadedKeys]);

  // Stable identity so memoized tree nodes are not invalidated on every render.
  const onChangeRef = useRef(onChange);
  useEffect(() => {
    onChangeRef.current = onChange;
  }, [onChange]);
  const handleToggleSelect = useCallback((path: string) => {
    if (multiple) {
      const next = selectedValueSet.has(path)
        ? selectedValues.filter((p) => p !== path)
        : [...selectedValues, path];
      onChangeRef.current?.(next);
    } else {
      onChangeRef.current?.(path);
      setOpen(false);
    }
  }, [multiple, selectedValues, selectedValueSet]);

  const handleRemoveTag = (path: string) => {
    if (multiple) {
      onChange?.(selectedValues.filter((p) => p !== path));
    } else {
      onChange?.('');
    }
    triggerRef.current?.focus();
  };

  return (
    <PopoverPrimitive.Root open={open} onOpenChange={setOpen}>
      <div className={cn('min-w-0 space-y-1.5', className)}>
        <PopoverPrimitive.Trigger asChild>
          <button
            id={id}
            ref={triggerRef}
            type="button"
            disabled={disabled}
            className="flex min-h-9 w-full items-center justify-between gap-2 rounded-md border border-line-strong bg-surface px-3 py-1.5 text-left text-sm text-slate-400 shadow-sm cursor-pointer disabled:cursor-not-allowed disabled:bg-tint disabled:opacity-50 focus-visible:outline-2 focus-visible:outline-brand"
          >
            <span className="min-w-0 wrap-anywhere">{placeholder}</span>
            {loading ? (
              <Loader2 className="size-4 shrink-0 text-teal-600 motion-safe:animate-spin" aria-hidden="true" />
            ) : (
              <ChevronDown className="size-4 shrink-0 opacity-60" aria-hidden="true" />
            )}
          </button>
        </PopoverPrimitive.Trigger>
        {selectedValues.length > 0 && (
          <div className="flex min-w-0 flex-wrap items-center gap-1.5">
            {selectedValues.map((path) => (
              <span key={path} className={cn(badge.base, badge.tone.neutral, 'max-w-full gap-1 whitespace-normal pr-1')}>
                <span className="min-w-0 wrap-anywhere">{path}</span>
                {!disabled && (
                  <button
                    type="button"
                    aria-label={`移除 ${path}`}
                    onClick={() => handleRemoveTag(path)}
                    className="grid size-5 shrink-0 place-items-center rounded hover:text-slate-900 focus-visible:outline-2 focus-visible:outline-brand"
                  >
                    <X className="h-3 w-3" aria-hidden="true" />
                  </button>
                )}
              </span>
            ))}
          </div>
        )}
      </div>

      <PopoverPrimitive.Portal>
        <PopoverPrimitive.Content
          aria-label="目录选择"
          sideOffset={4}
          align="start"
          collisionPadding={8}
          className="z-50 flex max-h-[var(--radix-popover-content-available-height)] w-[var(--radix-popover-trigger-width)] min-w-[min(17rem,var(--radix-popover-content-available-width))] max-w-[min(500px,var(--radix-popover-content-available-width))] flex-col rounded-lg border border-line bg-surface p-2.5 shadow-xl animate-in fade-in-0 zoom-in-95 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95"
        >
          <div className="relative mb-2 flex items-center gap-2">
            <Search className="absolute left-2.5 h-3.5 w-3.5 text-slate-400 pointer-events-none" />
            <input
              type="text"
              aria-label="搜索路径"
              placeholder="快速搜索路径..."
              value={searchInput}
              onChange={(e) => setSearchInput(e.target.value)}
              className="h-8 min-w-0 flex-1 rounded-md border border-line bg-tint pl-8 pr-3 text-xs placeholder:text-slate-400 focus:outline-none focus:border-teal-600 focus:bg-surface transition-colors"
            />
            {onRefresh && (
              <Tooltip title="刷新目录">
                <button
                  type="button"
                  aria-label="刷新目录"
                  aria-disabled={loading || refreshing || undefined}
                  onClick={() => void handleRefresh()}
                  className="flex size-8 shrink-0 items-center justify-center rounded-md hover:bg-line-soft aria-disabled:opacity-50 focus-visible:outline-2 focus-visible:outline-brand"
                >
                  <RefreshCw className={cn('size-4', refreshing && 'motion-safe:animate-spin')} aria-hidden="true" />
                </button>
              </Tooltip>
            )}
          </div>

          {loadError && <div role="alert" className="mb-2 text-xs text-rose-700">目录加载失败，请重试或刷新</div>}
          {(loading || refreshing) && <div role="status" className="mb-2 text-xs text-slate-500">正在加载目录</div>}
          <div role="group" aria-label="目录列表" aria-busy={loading || refreshing} className="max-h-64 min-h-0 space-y-0.5 overflow-y-auto pr-1">
            {treeData.length === 0 ? (
              !loading && !refreshing && !loadError && (
                <div className="py-6 text-center text-xs text-slate-400">
                  {notFoundContent}
                </div>
              )
            ) : (
              treeData.map((node) => (
                <TreeNodeItem
                  key={`${treeVersion}:${node.value}`}
                  node={node}
                  depth={0}
                  selectedValues={selectedValueSet}
                  onToggleSelect={handleToggleSelect}
                  loadData={loadData}
                  loadedKeys={loadedKeySet}
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
