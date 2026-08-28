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
}

export function usePathTree(alistId: number | undefined, treeLoadRequestRef: RefObject<number>) {
  const [treeData, setTreeData] = useState<TreeNode[]>([]);
  const [loadedKeys, setLoadedKeys] = useState<Key[]>([]);
  const [loadError, setLoadError] = useState(false);
  // Concurrently expanding the same node (or the root) shares one in-flight
  // request instead of issuing duplicates.
  const inFlightRef = useRef(new Map<string, Promise<TreeNode[]>>());

  const requestChildren = useCallback((alist: number, path: string): Promise<TreeNode[]> => {
    const cached = inFlightRef.current.get(path);
    if (cached) return cached;
    const promise = fetchDirChildren(alist, path)
      .catch((err) => {
        // Surface the failure to the UI (the tree shows a load-error state
        // instead of silently rendering an empty directory).
        setLoadError(true);
        throw err;
      })
      .finally(() => {
        inFlightRef.current.delete(path);
      });
    inFlightRef.current.set(path, promise);
    return promise;
  }, []);

  const initializeTree = useCallback((paths: unknown) => {
    setLoadedKeys([]);
    return buildPathTreeData(parseJobPathList(paths));
  }, []);

  const loadRoot = useCallback(async (paths: unknown) => {
    const pathTree = initializeTree(paths);
    setTreeData(pathTree);
    setLoadError(false);
    if (!alistId) return pathTree;
    const requestID = treeLoadRequestRef.current;
    try {
      const nodes = await requestChildren(alistId, '/');
      if (requestID !== treeLoadRequestRef.current) return pathTree;
      const root = [{ title: '/', value: '/', key: '/', children: nodes }];
      const merged = mergeTreeData(root, pathTree);
      setTreeData(merged);
      return merged;
    } catch {
      return pathTree;
    }
  }, [alistId, initializeTree, requestChildren, treeLoadRequestRef]);

  const onLoadData = useCallback(async (node: TreeNode) => {
    if (!alistId || loadedKeys.includes(node.value)) return;
    const requestID = treeLoadRequestRef.current;
    setLoadError(false);
    try {
      const children = await requestChildren(alistId, node.value);
      if (requestID !== treeLoadRequestRef.current) return;
      setTreeData((prev) => updateTreeChildren(prev, node.value, children));
      setLoadedKeys((prev) => [...prev, node.value]);
    } catch {
      /* loadError state is set by requestChildren */
    }
  }, [alistId, loadedKeys, requestChildren, treeLoadRequestRef]);

  const clearTree = useCallback(() => {
    setTreeData([]);
    setLoadedKeys([]);
    setLoadError(false);
  }, []);

  return {
    treeData,
    setTreeData,
    loadedKeys,
    setLoadedKeys,
    initializeTree,
    loadRoot,
    onLoadData,
    clearTree,
    treeLoadError: loadError,
  };
}
