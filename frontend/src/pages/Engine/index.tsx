import { useState, useEffect, useCallback, useRef } from 'react';
import {
  Plus, Edit, Trash2, Server, Activity, RefreshCw, Copy, Check
} from 'lucide-react';
import { alistGet, alistGetPath, alistPost, alistPut, alistDelete } from '../../api/alist';
import dayjs from 'dayjs';
import type { AlistItem } from '../../types';
import { Button } from '../../components/ui/button';
import { Input } from '../../components/ui/input';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '../../components/ui/dialog';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '../../components/ui/alert-dialog';
import { toast } from '../../components/ui/toaster';

export const validateAlistURL = (value?: string): boolean => {
  if (!value) return false;
  try {
    const url = new URL(value);
    return url.protocol === 'http:' || url.protocol === 'https:';
  } catch {
    return false;
  }
};

export default function Engine() {
  const [list, setList] = useState<AlistItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [listError, setListError] = useState(false);

  // 新增 / 编辑 弹窗状态
  const [modalVisible, setModalVisible] = useState(false);
  const [editingItem, setEditingItem] = useState<AlistItem | null>(null);
  const [url, setUrl] = useState('');
  const [remark, setRemark] = useState('');
  const [token, setToken] = useState('');
  const [urlError, setUrlError] = useState('');
  const [submitting, setSubmitting] = useState(false);

  // 删除确认弹窗状态
  const [deleteTargetId, setDeleteTargetId] = useState<number | null>(null);

  // 测试连接状态与复制提示
  const [testingId, setTestingId] = useState<number | null>(null);
  const [copiedId, setCopiedId] = useState<number | null>(null);

  const listReqRef = useRef(0);

  const fetchList = useCallback(async () => {
    const reqID = ++listReqRef.current;
    setLoading(true);
    setListError(false);
    try {
      const res = await alistGet({ silent: true });
      if (reqID !== listReqRef.current) return;
      setList(res.data || []);
    } catch (err) {
      if (reqID !== listReqRef.current) return;
      setList([]);
      setListError(true);
      console.error('alist fetchList failed', err);
    } finally {
      if (reqID === listReqRef.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchList();
  }, [fetchList]);

  const handleAdd = () => {
    setEditingItem(null);
    setUrl('');
    setRemark('');
    setToken('');
    setUrlError('');
    setModalVisible(true);
  };

  const handleEdit = (item: AlistItem) => {
    setEditingItem(item);
    setUrl(item.url || '');
    setRemark(item.remark || '');
    setToken('');
    setUrlError('');
    setModalVisible(true);
  };

  const handleDeleteConfirm = async () => {
    if (!deleteTargetId) return;
    try {
      await alistDelete(deleteTargetId);
      toast.success('删除成功');
      fetchList();
    } catch (err) {
      console.error('alist delete failed', err);
    } finally {
      setDeleteTargetId(null);
    }
  };

  const handleTest = async (item: AlistItem) => {
    if (testingId !== null) return;
    setTestingId(item.id);
    try {
      await alistGetPath(item.id, '/', { silent: true });
      toast.success('连接测试成功');
    } catch (err) {
      console.error('alist test failed', err);
      toast.error('连接测试失败');
    } finally {
      setTestingId(null);
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setUrlError('');

    const trimmedUrl = url.trim().replace(/\/+$/, '');
    if (!trimmedUrl) {
      setUrlError('请输入 AList 地址');
      return;
    }
    if (!validateAlistURL(trimmedUrl)) {
      setUrlError('请输入合法的 http 或 https 地址');
      return;
    }
    if (!editingItem && !token.trim()) {
      setUrlError('请输入 AList 访问令牌');
      return;
    }

    setSubmitting(true);
    try {
      if (editingItem) {
        await alistPut({
          id: editingItem.id,
          url: trimmedUrl,
          remark: remark.trim() || null,
          token: token.trim() || undefined,
        });
      } else {
        await alistPost({
          url: trimmedUrl,
          remark: remark.trim() || null,
          token: token.trim(),
        });
      }
      toast.success(editingItem ? '更新成功' : '新增成功');
      setModalVisible(false);
      fetchList();
    } catch (err) {
      console.error('alist submit failed', err);
    } finally {
      setSubmitting(false);
    }
  };

  const handleCopyUrl = (item: AlistItem) => {
    navigator.clipboard.writeText(item.url);
    setCopiedId(item.id);
    toast.info('地址已复制到剪贴板');
    setTimeout(() => setCopiedId(null), 2000);
  };

  return (
    <div className="grid content-start gap-4 min-w-0 min-h-[calc(100vh-90px)]">
      {/* 顶部标题栏 */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 pb-2 border-b border-slate-200/80">
        <div className="space-y-1 min-w-0">
          <h1 className="text-xl font-bold tracking-tight text-slate-900">引擎管理</h1>
          <p className="text-sm text-slate-500">
            管理 AList / OpenList 连接和路径选择来源
          </p>
        </div>
        <div className="flex shrink-0 items-center justify-end gap-2">
          <Button onClick={handleAdd} className="shadow-xs">
            <Plus className="h-4 w-4 mr-1.5" />
            新增引擎
          </Button>
        </div>
      </div>

      {/* 列表主体 */}
      <div className="min-w-0">
        {listError ? (
          <div className="py-16 text-center space-y-3 bg-white rounded-xl border border-slate-200">
            <p className="text-sm text-slate-500">引擎列表加载失败</p>
            <Button variant="outline" size="sm" onClick={fetchList} loading={loading}>
              <RefreshCw className="h-3.5 w-3.5 mr-1.5" />
              重试
            </Button>
          </div>
        ) : list.length === 0 && !loading ? (
          <div className="py-16 text-center space-y-3 bg-white rounded-xl border border-dashed border-slate-200">
            <Server className="h-10 w-10 text-slate-300 mx-auto" />
            <p className="text-sm text-slate-500">暂无引擎，请点击右上角添加 AList / OpenList 实例</p>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 items-stretch">
            {list.map((item) => (
              <div
                key={item.id}
                className="bg-white rounded-xl border border-slate-200/90 shadow-xs hover:shadow-md transition-shadow flex flex-col justify-between p-5 space-y-4 h-full"
              >
                <div className="space-y-3">
                  <div className="flex items-start justify-between gap-3">
                    <div className="flex items-center gap-2.5 min-w-0">
                      <div className="h-9 w-9 rounded-lg bg-teal-50 text-teal-700 flex items-center justify-center shrink-0">
                        <Server className="h-4 w-4" />
                      </div>
                      <div className="min-w-0">
                        <h2 className="font-semibold text-slate-900 text-sm truncate">
                          {item.userName || 'AList 实例'}
                        </h2>
                        <p className="text-xs text-slate-400 truncate">
                          {item.remark || '未设置备注'}
                        </p>
                      </div>
                    </div>
                  </div>

                  <div className="space-y-1.5 text-xs text-slate-600 bg-slate-50 p-3 rounded-lg border border-slate-100">
                    <div className="flex items-center justify-between gap-2">
                      <span className="text-slate-400 shrink-0">服务地址:</span>
                      <div className="flex items-center gap-1 min-w-0">
                        <span className="font-mono text-slate-700 truncate" title={item.url}>
                          {item.url}
                        </span>
                        <button
                          type="button"
                          onClick={() => handleCopyUrl(item)}
                          className="text-slate-400 hover:text-slate-700 p-0.5 rounded shrink-0 transition-colors"
                          title="复制地址"
                        >
                          {copiedId === item.id ? (
                            <Check className="h-3 w-3 text-emerald-600" />
                          ) : (
                            <Copy className="h-3 w-3" />
                          )}
                        </button>
                      </div>
                    </div>

                    <div className="flex items-center justify-between">
                      <span className="text-slate-400">添加时间:</span>
                      <span className="text-slate-600">
                        {item.createTime ? dayjs.unix(item.createTime).format('YYYY-MM-DD HH:mm') : '—'}
                      </span>
                    </div>
                  </div>
                </div>

                {/* 底部快捷操作 */}
                <div className="flex items-center justify-end gap-1.5 pt-2 border-t border-slate-100">
                  <Button
                    variant="ghost"
                    size="sm"
                    loading={testingId === item.id}
                    onClick={() => handleTest(item)}
                    className="text-slate-600 hover:text-teal-700 text-xs h-7 px-2"
                  >
                    <Activity className="h-3.5 w-3.5 mr-1" />
                    测试连接
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => handleEdit(item)}
                    className="text-slate-600 hover:text-slate-900 text-xs h-7 px-2"
                  >
                    <Edit className="h-3.5 w-3.5 mr-1" />
                    编辑
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => setDeleteTargetId(item.id)}
                    className="text-red-600 hover:text-red-700 hover:bg-red-50 text-xs h-7 px-2"
                  >
                    <Trash2 className="h-3.5 w-3.5 mr-1" />
                    删除
                  </Button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* 新增 / 编辑 引擎 Dialog */}
      <Dialog open={modalVisible} onOpenChange={setModalVisible}>
        <DialogContent className="max-w-md" title={editingItem ? '编辑引擎' : '新增引擎'}>
          <DialogHeader>
            <DialogTitle>{editingItem ? '编辑引擎' : '新增引擎'}</DialogTitle>
          </DialogHeader>

          {urlError && (
            <div className="p-2.5 rounded bg-rose-50 border border-rose-200 text-rose-700 text-xs font-medium">
              {urlError}
            </div>
          )}

          <form onSubmit={handleSubmit} className="space-y-3.5 my-2">
            <div className="space-y-1">
              <label className="text-xs font-medium text-slate-700">
                服务地址 <span className="text-red-500">*</span>
              </label>
              <Input
                placeholder="http://192.168.1.100:5244"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                required
              />
              <p className="text-[11px] text-slate-400">支持本地局域网或公网 HTTP / HTTPS 地址</p>
            </div>

            <div className="space-y-1">
              <label className="text-xs font-medium text-slate-700">备注名称</label>
              <Input
                placeholder="例如：家庭 NAS / 阿里云盘"
                value={remark}
                onChange={(e) => setRemark(e.target.value)}
              />
            </div>

            <div className="space-y-1">
              <label className="text-xs font-medium text-slate-700">
                访问令牌 (Token) {!editingItem && <span className="text-red-500">*</span>}
              </label>
              <Input
                type="password"
                placeholder={editingItem ? '留空表示保持原有已存 Token' : '请输入 AList 后台生成的 Token'}
                value={token}
                onChange={(e) => setToken(e.target.value)}
                required={!editingItem}
              />
            </div>

            <DialogFooter className="pt-2">
              <Button type="button" variant="outline" onClick={() => setModalVisible(false)}>
                取消
              </Button>
              <Button type="submit" loading={submitting}>
                确定保存
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* 删除确认弹窗 */}
      <AlertDialog open={deleteTargetId !== null} onOpenChange={(o) => !o && setDeleteTargetId(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>确认删除此引擎？</AlertDialogTitle>
            <AlertDialogDescription>
              删除后，依赖此引擎的同步任务将无法继续连接该存储端。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction variant="destructive" onClick={handleDeleteConfirm}>
              确认删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
