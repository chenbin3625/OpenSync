import { useCallback, useEffect, useRef, useState, type Key, type RefObject } from 'react';
import type { PathItem, TreeNode } from '../../types';
import { alistGetPath } from '../../api/alist';
import { buildPathTreeData, mergeTreeData, parseJobPathList } from './homeUtils';

function updateTreeChildren(tree: TreeNode[], parentValue: string, children: TreeNode[]): TreeNode[] {
  return tree.map((node) => {
    if (node.value === parentValue) {
      return { ...node, children: mergeTreeData(children, node.children || []) };
    }
    if (node.children) {
      return { ...node, children: updateTreeChildren(node.children, parentValue, children) };
    }
    return node;
  });
}

async function fetchDirChildren(alistId: number, parentPath: string, signal: AbortSignal): Promise<TreeNode[]> {
  const res = await alistGetPath(alistId, parentPath, { signal, silent: true });
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

type ReadDirectory = (alistId: number, path: string) => Promise<TreeNode[]>;

export function useDirectoryRequests() {
  const inFlightRef = useRef(new Map<string, Promise<TreeNode[]>>());
  const controllerRef = useRef<AbortController | null>(null);

  const readDirectory = useCallback<ReadDirectory>((alist, path) => {
    const key = JSON.stringify([alist, path]);
    const cached = inFlightRef.current.get(key);
    if (cached) return cached;
    const controller = (controllerRef.current ??= new AbortController());
    const promise = fetchDirChildren(alist, path, controller.signal)
      .finally(() => {
        if (controllerRef.current === controller) inFlightRef.current.delete(key);
      });
    inFlightRef.current.set(key, promise);
    return promise;
  }, []);

  const cancelDirectoryRequests = useCallback(() => {
    controllerRef.current?.abort();
    controllerRef.current = null;
    inFlightRef.current.clear();
  }, []);

  useEffect(() => cancelDirectoryRequests, [cancelDirectoryRequests]);

  return { readDirectory, cancelDirectoryRequests };
}

export function usePathTree(
  alistId: number | undefined,
  treeLoadRequestRef: RefObject<number>,
  readDirectory: ReadDirectory,
) {
  const [treeData, setTreeData] = useState<TreeNode[]>([]);
  const [loadedKeys, setLoadedKeys] = useState<Key[]>([]);
  const [loadError, setLoadError] = useState(false);
  const generationRef = useRef(0);

  const loadRoot = useCallback(async (paths: unknown) => {
    const requestID = treeLoadRequestRef.current;
    const generation = ++generationRef.current;
    const pathTree = buildPathTreeData(parseJobPathList(paths));
    setLoadedKeys([]);
    setTreeData(pathTree);
    setLoadError(false);
    if (!alistId) return;
    try {
      const nodes = await readDirectory(alistId, '/');
      if (requestID !== treeLoadRequestRef.current || generation !== generationRef.current) return;
      const root = [{ title: '/', value: '/', key: '/', children: nodes }];
      setTreeData(mergeTreeData(root, pathTree));
      setLoadedKeys(['/']);
    } catch {
      if (requestID === treeLoadRequestRef.current && generation === generationRef.current) {
        setLoadError(true);
      }
    }
  }, [alistId, readDirectory, treeLoadRequestRef]);

  const onLoadData = useCallback(async (node: TreeNode) => {
    if (!alistId) return false;
    if (loadedKeys.includes(node.value)) return true;
    const requestID = treeLoadRequestRef.current;
    const generation = generationRef.current;
    setLoadError(false);
    try {
      const children = await readDirectory(alistId, node.value);
      if (requestID !== treeLoadRequestRef.current || generation !== generationRef.current) return false;
      setTreeData((prev) => updateTreeChildren(prev, node.value, children));
      setLoadedKeys((prev) => prev.includes(node.value) ? prev : [...prev, node.value]);
      return true;
    } catch {
      if (requestID === treeLoadRequestRef.current && generation === generationRef.current) {
        setLoadError(true);
      }
      return false;
    }
  }, [alistId, loadedKeys, readDirectory, treeLoadRequestRef]);

  const clearTree = useCallback(() => {
    generationRef.current += 1;
    setTreeData([]);
    setLoadedKeys([]);
    setLoadError(false);
  }, []);

  return {
    treeData,
    loadedKeys,
    loadRoot,
    onLoadData,
    clearTree,
    treeLoadError: loadError,
  };
}
