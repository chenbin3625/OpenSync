import React, { useState, useMemo } from 'react';
import { ChevronRight, ChevronDown, Folder, FolderOpen, X, Check, Search, Loader2 } from 'lucide-react';
import * as PopoverPrimitive from '@radix-ui/react-popover';
import { cn } from '../../lib/utils';
import type { TreeNode } from '../../types';

export interface PathTreeSelectProps {
  placeholder?: string;
  value?: string | string[];
  onChange?: (value: string | string[]) => void;
  treeData: TreeNode[];
  loadData?: (node: TreeNode) => Promise<void>;
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
  search: string;
}

function TreeNodeItem({
  node,
  selectedValues,
  onToggleSelect,
  loadData,
  search,
}: TreeNodeItemProps) {
  const [expanded, setExpanded] = useState(false);
  const [loadingChildren, setLoadingChildren] = useState(false);

  const isSelected = selectedValues.includes(node.value);

  const handleExpand = async (e: React.MouseEvent) => {
    e.stopPropagation();
    if (!expanded && !node.children && loadData) {
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
    <div className="text-sm select-none">
      <div
        className={cn(
          'flex items-center gap-1.5 py-1 px-2 rounded-md hover:bg-slate-100 cursor-pointer transition-colors',
          isSelected && 'bg-teal-50 text-teal-900 font-medium'
        )}
        onClick={() => onToggleSelect(node.value)}
      >
        <button
          type="button"
          className="p-0.5 rounded hover:bg-slate-200/80 text-slate-400 hover:text-slate-700 transition-colors"
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

        <div className="flex items-center justify-center h-4 w-4 rounded border border-slate-300 mr-0.5 shrink-0 transition-colors">
          {isSelected && <Check className="h-3 w-3 text-teal-700 stroke-[3]" />}
        </div>

        {expanded ? (
          <FolderOpen className="h-4 w-4 text-teal-600 shrink-0" />
        ) : (
          <Folder className="h-4 w-4 text-slate-400 shrink-0" />
        )}

        <span className={cn('truncate', matchesSearch ? 'text-slate-800' : 'text-slate-400')}>
          {node.title || node.value}
        </span>
      </div>

      {expanded && node.children && node.children.length > 0 && (
        <div className="pl-5 border-l border-slate-200 ml-3.5 mt-0.5 space-y-0.5">
          {node.children.map((child) => (
            <TreeNodeItem
              key={child.value}
              node={child}
              selectedValues={selectedValues}
              onToggleSelect={onToggleSelect}
              loadData={loadData}
              search={search}
            />
          ))}
        </div>
      )}
    </div>
  );
}

export function PathTreeSelect({
  placeholder = '请选择目录',
  value,
  onChange,
  treeData,
  loadData,
  multiple = true,
  disabled = false,
  loading = false,
  notFoundContent = '请先展开或选择引擎',
  className,
}: PathTreeSelectProps) {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState('');

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
          role="button"
          tabIndex={disabled ? -1 : 0}
          className={cn(
            'min-h-9 w-full rounded-md border border-slate-300 bg-white px-3 py-1.5 text-sm shadow-sm flex items-center justify-between gap-2 cursor-pointer transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-teal-600 focus-visible:border-teal-600',
            disabled && 'cursor-not-allowed opacity-50 bg-slate-50',
            className
          )}
        >
          <div className="flex flex-wrap items-center gap-1.5 min-w-0 flex-1">
            {selectedValues.length === 0 ? (
              <span className="text-slate-400 select-none text-sm">{placeholder}</span>
            ) : (
              selectedValues.map((path) => (
                <span
                  key={path}
                  className="inline-flex items-center gap-1 rounded bg-slate-100 border border-slate-200 px-2 py-0.5 text-xs text-slate-700 font-medium max-w-[200px]"
                >
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
          className="z-50 w-[var(--radix-popover-trigger-width)] min-w-[320px] max-w-[500px] rounded-lg border border-slate-200 bg-white p-2.5 shadow-xl animate-in fade-in-0 zoom-in-95 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95"
        >
          <div className="relative mb-2 flex items-center">
            <Search className="absolute left-2.5 h-3.5 w-3.5 text-slate-400 pointer-events-none" />
            <input
              type="text"
              placeholder="快速搜索路径..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="h-8 w-full rounded-md border border-slate-200 bg-slate-50 pl-8 pr-3 text-xs placeholder:text-slate-400 focus:outline-none focus:border-teal-600 focus:bg-white transition-colors"
            />
          </div>

          <div className="max-h-64 overflow-y-auto pr-1 space-y-0.5">
            {treeData.length === 0 ? (
              <div className="py-6 text-center text-xs text-slate-400">
                {notFoundContent}
              </div>
            ) : (
              treeData.map((node) => (
                <TreeNodeItem
                  key={node.value}
                  node={node}
                  selectedValues={selectedValues}
                  onToggleSelect={handleToggleSelect}
                  loadData={loadData}
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
