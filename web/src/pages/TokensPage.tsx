import { FormEvent, useEffect, useState } from "react";
import StatusBadge from "../components/StatusBadge";
import DataTable, { type Column } from "../components/DataTable";
import { backendGet, backendPost, backendPut, backendDelete } from "../lib/backend";
import { toast } from "../components/Feedback";
import { compact, shortDate } from "../lib/fmt";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Plus, RefreshCw, Trash2, Copy } from "lucide-react";

type Token = {
  id: number;
  name: string;
  key: string;
  status: number;
  used_quota: number;
  remain_quota: number;
  unlimited_quota: boolean;
  created_time: number;
  expired_time: number;
};

type TokenForm = {
  name: string;
  remain_quota: number;
  unlimited_quota: boolean;
};

const emptyForm: TokenForm = {
  name: "",
  remain_quota: 500000,
  unlimited_quota: false,
};

export default function TokensPage() {
  const [tokens, setTokens] = useState<Token[]>([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [editId, setEditId] = useState<number | null>(null);
  const [form, setForm] = useState<TokenForm>(emptyForm);
  const [deleteTarget, setDeleteTarget] = useState<number | null>(null);

  function load() {
    setLoading(true);
    backendGet<{ data: Token[] }>("/api/token/?p=0&page_size=100")
      .then((d) => setTokens(d.data ?? []))
      .catch(() => toast("加载令牌失败", "error"))
      .finally(() => setLoading(false));
  }

  useEffect(load, []);

  function openCreate() {
    setEditId(null);
    setForm(emptyForm);
    setShowForm(true);
  }

  function openEdit(t: Token) {
    setEditId(t.id);
    setForm({
      name: t.name,
      remain_quota: t.remain_quota,
      unlimited_quota: t.unlimited_quota,
    });
    setShowForm(true);
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    try {
      if (editId) {
        await backendPut("/api/token/", { id: editId, ...form });
        toast("令牌已更新");
      } else {
        await backendPost("/api/token/", form);
        toast("令牌已创建");
      }
      setShowForm(false);
      load();
    } catch {
      toast("操作失败", "error");
    }
  }

  async function confirmDelete() {
    if (deleteTarget === null) return;
    try {
      await backendDelete(`/api/token/${deleteTarget}`);
      toast("已删除");
      load();
    } catch {
      toast("删除失败", "error");
    } finally {
      setDeleteTarget(null);
    }
  }

  async function toggleToken(t: Token) {
    try {
      const newStatus = t.status === 1 ? 2 : 1;
      await backendPut("/api/token/", { ...t, status: newStatus });
      toast(newStatus === 1 ? "已启用" : "已停用");
      load();
    } catch {
      toast("操作失败", "error");
    }
  }

  function copyKey(key: string) {
    navigator.clipboard.writeText(key);
    toast("已复制到剪贴板");
  }

  const columns: Column<Token>[] = [
    {
      key: "name",
      header: "名称",
      render: (r) => <span className="font-medium">{r.name}</span>,
    },
    {
      key: "key",
      header: "Key",
      render: (r) =>
        r.key ? (
          <div className="flex items-center gap-1">
            <code className="text-xs text-muted-foreground">
              sk-...{r.key.slice(-6)}
            </code>
            <Button
              variant="ghost"
              size="icon"
              className="size-6"
              onClick={() => copyKey(r.key)}
            >
              <Copy className="size-3" />
            </Button>
          </div>
        ) : (
          <span className="text-muted-foreground">-</span>
        ),
    },
    {
      key: "status",
      header: "状态",
      render: (r) => (
        <StatusBadge
          status={r.status === 1 ? "ok" : r.status === 3 ? "error" : "disabled"}
          label={r.status === 1 ? "正常" : r.status === 3 ? "过期" : "停用"}
        />
      ),
    },
    {
      key: "quota",
      header: "已用 / 剩余",
      render: (r) =>
        r.unlimited_quota
          ? `${compact(r.used_quota)} / unlimited`
          : `${compact(r.used_quota)} / ${compact(r.remain_quota)}`,
    },
    {
      key: "created",
      header: "创建时间",
      render: (r) => shortDate(new Date(r.created_time * 1000).toISOString()),
    },
    {
      key: "actions",
      header: "",
      render: (r) => (
        <div className="flex gap-1">
          <Button variant="ghost" size="sm" onClick={() => openEdit(r)}>
            编辑
          </Button>
          <Button variant="ghost" size="sm" onClick={() => toggleToken(r)}>
            {r.status === 1 ? "停用" : "启用"}
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="size-8 text-destructive hover:text-destructive"
            onClick={() => setDeleteTarget(r.id)}
          >
            <Trash2 className="size-4" />
          </Button>
        </div>
      ),
    },
  ];

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">令牌</h2>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" onClick={load}>
            <RefreshCw className="size-4" />
            刷新
          </Button>
          <Button size="sm" onClick={openCreate}>
            <Plus className="size-4" />
            新建令牌
          </Button>
        </div>
      </div>

      {loading ? (
        <Skeleton className="h-48" />
      ) : (
        <Card>
          <CardContent className="p-1">
            <DataTable
              columns={columns}
              rows={tokens}
              rowKey={(r) => r.id}
              empty="暂无令牌"
            />
          </CardContent>
        </Card>
      )}

      <Dialog open={showForm} onOpenChange={setShowForm}>
        <DialogContent>
          <form onSubmit={handleSubmit}>
            <DialogHeader>
              <DialogTitle>
                {editId ? "编辑令牌" : "新建令牌"}
              </DialogTitle>
            </DialogHeader>

            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="token-name">名称</Label>
                <Input
                  id="token-name"
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  required
                />
              </div>

              <div className="flex items-center gap-2">
                <Switch
                  checked={form.unlimited_quota}
                  onCheckedChange={(v) =>
                    setForm({ ...form, unlimited_quota: v })
                  }
                />
                <Label>无限额度</Label>
              </div>

              {!form.unlimited_quota && (
                <div className="space-y-2">
                  <Label htmlFor="token-quota">额度</Label>
                  <Input
                    id="token-quota"
                    type="number"
                    value={form.remain_quota}
                    onChange={(e) =>
                      setForm({ ...form, remain_quota: Number(e.target.value) })
                    }
                  />
                </div>
              )}
            </div>

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setShowForm(false)}
              >
                取消
              </Button>
              <Button type="submit">{editId ? "保存" : "创建"}</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <AlertDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>确定删除此令牌？</AlertDialogTitle>
            <AlertDialogDescription>
              此操作不可撤销，删除后令牌将立即失效。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction onClick={confirmDelete}>
              删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
