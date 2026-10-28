import { useState } from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import Layout from '../src/components/Layout';
import { Input } from '../src/components/ui/input';
import { Button } from '../src/components/ui/button';
import { Badge } from '../src/components/ui/badge';
import { Switch } from '../src/components/ui/switch';
import { Dialog, DialogContent, DialogTitle, DialogDescription } from '../src/components/ui/dialog';
import { Sheet, SheetContent, SheetTitle, SheetDescription } from '../src/components/ui/sheet';
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from '../src/components/ui/select';
import { PathTreeSelect } from '../src/components/ui/path-tree-select';
import { Tooltip } from '../src/components/ui/tooltip';
import { Toaster, toast } from '../src/components/ui/toaster';
import { surface, table } from '../src/lib/styles';
import '../src/index.css';

export function Fixture() {
  const [dialog, setDialog] = useState(false);
  const [sheet, setSheet] = useState(false);
  return <BrowserRouter><Layout>
    <div className="grid gap-4 max-w-2xl">
      <section className={`${surface.card} p-4 space-y-4`}>
        <h1 className="text-slate-900 font-semibold">同步任务</h1>
        <Input aria-label="任务名称" defaultValue="每日备份" />
        <Select defaultValue="all"><SelectTrigger aria-label="任务状态"><SelectValue /></SelectTrigger>
          <SelectContent><SelectItem value="all">全部状态</SelectItem><SelectItem value="failed">失败任务</SelectItem></SelectContent>
        </Select>
        <PathTreeSelect placeholder="选择源目录" treeData={[{ title: 'media', value: '/media', key: '/media' }]} />
        <div className="flex flex-wrap gap-2">
          <Badge variant="success">成功</Badge><Badge variant="warning">等待</Badge>
          <Badge variant="error">失败</Badge><Badge variant="processing">扫描中</Badge>
        </div>
        <Switch aria-label="启用任务" defaultChecked />
        <div className="flex flex-wrap gap-2">
          <Button onClick={() => setDialog(true)}>打开弹窗</Button>
          <Button variant="outline" onClick={() => setSheet(true)}>打开抽屉</Button>
          <Button variant="destructive">删除任务</Button>
          <Button variant="outline" onClick={() => toast.success('已保存')}>保存通知</Button>
          <Tooltip title="完整目录路径"><Button variant="ghost">路径提示</Button></Tooltip>
        </div>
        <table className={table.root}><thead className={table.head}><tr><th className={table.headCell}>目录</th></tr></thead>
          <tbody className={table.body}><tr><td className={`${table.cell} text-slate-700`}>/media/photos</td></tr></tbody>
        </table>
      </section>
    </div>
    <Dialog open={dialog} onOpenChange={setDialog}><DialogContent>
      <DialogTitle>任务详情</DialogTitle><DialogDescription>同步目录与执行状态</DialogDescription>
      <Input aria-label="目标目录" defaultValue="/backup/photos" />
    </DialogContent></Dialog>
    <Sheet open={sheet} onOpenChange={setSheet}><SheetContent>
      <SheetTitle>编辑任务</SheetTitle><SheetDescription>目录与计划</SheetDescription>
      <Input aria-label="任务备注" defaultValue="每日备份" />
    </SheetContent></Sheet>
  </Layout><Toaster /></BrowserRouter>;
}

createRoot(document.getElementById('root')!).render(<Fixture />);
