import { useCallback, useRef, useState, type Key, type RefObject } from 'react';
import type { PathItem, TreeNode } from '../../types';
import { alistGetPath } from '../../api/alist';
import { buildPathTreeData, mergeTreeData, parseJobPathList } from './homeUtils';

function updateTreeChildren(tree: TreeNode[], parentValue: string, children: TreeNode[]): TreeNode[] {
  return tree.map((node) => {
    if (node.value === parentValue) {
      return { ...node, children };
    }
    if (node.children) {
      return { ...node, children: updateTreeChildren(node.children, parentValue, children) };
    }
    return node;
  });
}

async function fetchDirChildren(alistId: number, parentPath: string): Promise<TreeNode[]> {
  if (!alistId) return [];
  const res = await alistGetPath(alistId, parentPath);
  const items = res.data || [];
  const nodes = (Array.isArray(items) ? items : [])
    .map((item: PathItem): TreeNode | null => {
      const name = item.path || item.name || '';
      if (!name.trim()) return null;
      const fullPath = parentPath === '/' ? `/${name}` : `${parentPath}/${name}`;
      return {
        title: name,
        value: fullPath,
        key: fullPath,
        isLeaf: false,
      };
    })
    .filter((node): node is TreeNode => node !== null);
  return nodes.sort((left, right) => String(left.title).localeCompare(String(right.title)));
}

export function usePathTree(alistId: number | undefined, treeLoadRequestRef: RefObject<number>) {
  const [treeData, setTreeData] = useState<TreeNode[]>([]);
  const [loadedKeysState, setLoadedKeysState] = useState<Key[]>([]);
  const [treeError, setTreeError] = useState(false);
  // Mirrored in a ref so onLoadData can check "already loaded" without
  // re-creating its identity on every expansion (which churns TreeSelect's
  // loadData prop and can trigger redundant loads).
  const loadedKeysRef = useRef<Set<Key>>(new Set());
  const loadingKeysRef = useRef<Set<Key>>(new Set());

  const setLoadedKeys = useCallback((keys: Key[] | ((prev: Key[]) => Key[])) => {
    if (typeof keys === 'function') {
      setLoadedKeysState((prev) => {
        const next = keys(prev);
        loadedKeysRef.current = new Set(next);
        return next;
      });
      return;
    }
    loadedKeysRef.current = new Set(keys);
    setLoadedKeysState(keys);
  }, []);

  const initializeTree = useCallback((paths: unknown) => {
    setLoadedKeys([]);
    setTreeError(false);
    return buildPathTreeData(parseJobPathList(paths));
  }, [setLoadedKeys]);

  const loadRoot = useCallback(async (paths: unknown) => {
    const pathTree = initializeTree(paths);
    setTreeData(pathTree);
    if (!alistId) return pathTree;
    const requestID = treeLoadRequestRef.current;
    let nodes: TreeNode[];
    try {
      nodes = await fetchDirChildren(alistId, '/');
    } catch {
      if (requestID === treeLoadRequestRef.current) setTreeError(true);
      return pathTree;
    }
    if (requestID !== treeLoadRequestRef.current) return pathTree;
    const root = [{ title: '/', value: '/', key: '/', children: nodes }];
    const merged = mergeTreeData(root, pathTree);
    setTreeData(merged);
    return merged;
  }, [alistId, initializeTree, treeLoadRequestRef]);

  const onLoadData = useCallback(async (node: TreeNode) => {
    if (!alistId || loadedKeysRef.current.has(node.value)) return;
    if (loadingKeysRef.current.has(node.value)) return;
    loadingKeysRef.current.add(node.value);
    try {
      const requestID = treeLoadRequestRef.current;
      let children: TreeNode[];
      try {
        children = await fetchDirChildren(alistId, node.value);
      } catch {
        if (requestID === treeLoadRequestRef.current) setTreeError(true);
        return;
      }
      if (requestID !== treeLoadRequestRef.current) return;
      setLoadedKeysState((prev) => [...prev, node.value]);
      loadedKeysRef.current.add(node.value);
      setTreeData((prev) => updateTreeChildren(prev, node.value, children));
    } finally {
      loadingKeysRef.current.delete(node.value);
    }
  }, [alistId, treeLoadRequestRef]);

  const clearTree = useCallback(() => {
    setTreeData([]);
    setLoadedKeys([]);
    setTreeError(false);
  }, [setLoadedKeys]);

  return {
    treeData,
    setTreeData,
    loadedKeys: loadedKeysState,
    treeError,
    setLoadedKeys,
    initializeTree,
    loadRoot,
    onLoadData,
    clearTree,
  };
}
