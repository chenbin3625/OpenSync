import { useState, useEffect, useCallback, useRef } from 'react';
import {
  Plus, Edit, Trash2, Server, Activity, Copy, Check
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
import { PageHeader } from '../../components/common/PageHeader';
import { Field } from '../../components/common/Field';
import { Alert } from '../../components/common/Alert';
import { InfoPanel, InfoRow } from '../../components/common/InfoRow';
import { ResourceCard } from '../../components/common/ResourceCard';
import {
  EmptyState,
  ErrorState,
  PlaceholderCard,
} from '../../components/common/StatePlaceholder';
import { cn } from '../../lib/utils';
import { icon, layout, surface } from '../../lib/styles';

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
    <div className={layout.page}>
      <PageHeader
        title="引擎管理"
        subtitle="管理 AList / OpenList 连接和路径选择来源"
        actions={
          <Button onClick={handleAdd}>
            <Plus className={cn(icon.md, 'mr-1.5')} />
            新增引擎
          </Button>
        }
      />

      {/* 列表主体 */}
      <div className="min-w-0">
        {listError ? (
          <PlaceholderCard>
            <ErrorState
              icon={Server}
              title="引擎列表加载失败"
              onRetry={fetchList}
              loading={loading}
              retryLabel="重试"
            />
          </PlaceholderCard>
        ) : list.length === 0 && !loading ? (
          <div className={surface.placeholder}>
            <EmptyState icon={Server} title="暂无引擎，请点击右上角添加 AList / OpenList 实例" />
          </div>
        ) : (
          <div className={layout.cardGrid}>
            {list.map((item) => (
              <ResourceCard
                key={item.id}
                icon={Server}
                title={item.userName || 'AList 实例'}
                subtitle={item.remark || '未设置备注'}
                actions={
                  <div className="flex items-center justify-end gap-1.5 w-full">
                    <Button
                      variant="ghost"
                      size="sm"
                      loading={testingId === item.id}
                      onClick={() => handleTest(item)}
                    >
                      <Activity className={cn(icon.sm, 'mr-1')} />
                      测试连接
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => handleEdit(item)}
                    >
                      <Edit className={cn(icon.sm, 'mr-1')} />
                      编辑
                    </Button>
                    <Button
                      variant="danger"
                      size="sm"
                      onClick={() => setDeleteTargetId(item.id)}
                    >
                      <Trash2 className={cn(icon.sm, 'mr-1')} />
                      删除
                    </Button>
                  </div>
                }
              >
                <InfoPanel>
                  <InfoRow label="服务地址" mono>
                    <span className="inline-flex items-center gap-1 min-w-0 max-w-full">
                      <span className="truncate" title={item.url}>
                        {item.url}
                      </span>
                      <button
                        type="button"
                        onClick={() => handleCopyUrl(item)}
                        className="text-slate-400 hover:text-slate-700 p-0.5 rounded shrink-0 transition-colors"
                        aria-label="复制地址"
                        title="复制地址"
                      >
                        {copiedId === item.id ? (
                          <Check className={cn(icon.sm, 'text-emerald-600')} />
                        ) : (
                          <Copy className={icon.sm} />
                        )}
                      </button>
                    </span>
                  </InfoRow>
                  <InfoRow label="添加时间">
                    {item.createTime
                      ? dayjs.unix(item.createTime).format('YYYY-MM-DD HH:mm')
                      : '—'}
                  </InfoRow>
                </InfoPanel>
              </ResourceCard>
            ))}
          </div>
        )}
      </div>

      {/* 新增 / 编辑 引擎 Dialog */}
      <Dialog open={modalVisible} onOpenChange={setModalVisible}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>{editingItem ? '编辑引擎' : '新增引擎'}</DialogTitle>
          </DialogHeader>

          {urlError && <Alert>{urlError}</Alert>}

          <form onSubmit={handleSubmit} className="space-y-3.5 my-2">
            <Field
              label="服务地址"
              required
              hint="支持本地局域网或公网 HTTP / HTTPS 地址"
            >
              <Input
                placeholder="http://192.168.1.100:5244"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                required
              />
            </Field>

            <Field label="备注名称">
              <Input
                placeholder="例如：家庭 NAS / 阿里云盘"
                value={remark}
                onChange={(e) => setRemark(e.target.value)}
              />
            </Field>

            <Field label="访问令牌 (Token)" required={!editingItem}>
              <Input
                type="password"
                placeholder={editingItem ? '留空表示保持原有已存 Token' : '请输入 AList 后台生成的 Token'}
                value={token}
                onChange={(e) => setToken(e.target.value)}
                required={!editingItem}
              />
            </Field>

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
