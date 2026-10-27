import { useEffect, useRef, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { PathTreeSelect } from '../src/components/ui/path-tree-select';
import { usePathTree } from '../src/pages/Home/usePathTree';
import '../src/index.css';

export function Fixture() {
  const requestRef = useRef(0);
  const [engine, setEngine] = useState(1);
  const [value, setValue] = useState<string | string[]>(['/media/movies/2026']);
  const tree = usePathTree(engine, requestRef);
  const { loadRoot } = tree;
  useEffect(() => {
    requestRef.current += 1;
    void loadRoot(['/media/movies/2026']);
  }, [engine, loadRoot]);
  return (
    <main className="w-full max-w-md p-4">
      <button onClick={() => setEngine(2)}>Switch engine</button>
      <PathTreeSelect
        value={value}
        onChange={setValue}
        treeData={tree.treeData}
        loadedKeys={tree.loadedKeys}
        loadData={tree.onLoadData}
        onRefresh={() => tree.loadRoot(value)}
      />
      <output>{JSON.stringify(value)}</output>
    </main>
  );
}

createRoot(document.getElementById('root')!).render(<Fixture />);
