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
}

export function usePathTree(alistId: number | undefined, treeLoadRequestRef: RefObject<number>) {
  const [treeData, setTreeData] = useState<TreeNode[]>([]);
  const [loadedKeysState, setLoadedKeysState] = useState<Key[]>([]);
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
    return buildPathTreeData(parseJobPathList(paths));
  }, [setLoadedKeys]);

  const loadRoot = useCallback(async (paths: unknown) => {
    const pathTree = initializeTree(paths);
    setTreeData(pathTree);
    if (!alistId) return pathTree;
    const requestID = treeLoadRequestRef.current;
    const nodes = await fetchDirChildren(alistId, '/');
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
      const children = await fetchDirChildren(alistId, node.value);
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
  }, [setLoadedKeys]);

  return {
    treeData,
    setTreeData,
    loadedKeys: loadedKeysState,
    setLoadedKeys,
    initializeTree,
    loadRoot,
    onLoadData,
    clearTree,
  };
}
