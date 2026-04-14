import { type FormEvent, useEffect, useState } from "react";
import DataTable, { type Column } from "@/components/DataTable";
import ConfirmDialog from "@/components/ConfirmDialog";
import PageHeader from "@/components/PageHeader";
import SectionHeading from "@/components/SectionHeading";
import { api } from "@/lib/api";
import { toast } from "@/components/Feedback";
import type { ModelRoute, Pool, SystemSettings } from "@/lib/types";
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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Plus, MoreHorizontal, Info } from "lucide-react";

interface RouteForm {
  alias: string;
  target_model: string;
  pool_id: number | null;
  enabled: boolean;
}

const emptyForm: RouteForm = {
  alias: "",
  target_model: "",
  pool_id: null,
  enabled: true,
};

export default function RoutesPage() {
  const [routes, setRoutes] = useState<ModelRoute[]>([]);
  const [pools, setPools] = useState<Pool[]>([]);
  const [defaultPoolId, setDefaultPoolId] = useState<number | null>(null);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [editId, setEditId] = useState<number | null>(null);
  const [form, setForm] = useState<RouteForm>(emptyForm);
  const [deleteTarget, setDeleteTarget] = useState<ModelRoute | null>(null);

  function load() {
    setLoading(true);
    let cancelled = false;

    Promise.all([
      api.get<ModelRoute[]>("/routes").catch(() => {
        if (!cancelled) toast("加载路由失败", "error");
        return null;
      }),
      api.get<Pool[]>("/pools").catch(() => null),
      api.get<SystemSettings>("/settings").catch(() => null),
    ]).then(([r, p, s]) => {
      if (cancelled) return;
      if (r !== null) setRoutes(r ?? []);
      if (p !== null) setPools(p ?? []);
      if (s !== null) setDefaultPoolId(s?.default_pool_id ?? null);
    }).finally(() => {
      if (!cancelled) setLoading(false);
    });

    return () => { cancelled = true; };
  }

  useEffect(() => {
    const cancel = load();
    return cancel;
  }, []);

  async function updateDefaultPool(poolId: string | null) {
    if (poolId === null) return;
    const value = poolId === "none" ? null : Number(poolId);
    setDefaultPoolId(value);
    try {
      await api.put("/settings", { default_pool_id: value });
    } catch {
      toast("更新默认池失败", "error");
      load();
    }
  }

  function openCreate() {
    setEditId(null);
    setForm(emptyForm);
    setShowForm(true);
  }

  function openEdit(r: ModelRoute) {
    setEditId(r.id);
    setForm({
      alias: r.alias,
      target_model: r.target_model,
      pool_id: r.pool_id,
      enabled: r.enabled,
    });
    setShowForm(true);
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    try {
      if (editId) {
        await api.put(`/routes/${editId}`, form);
        toast("路由已更新");
      } else {
        await api.post("/routes", form);
        toast("路由已创建");
      }
      setShowForm(false);
      load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "操作失败", "error");
    }
  }

  async function toggleRoute(r: ModelRoute) {
    const next = !r.enabled;
    setRoutes((prev) =>
      prev.map((x) => (x.id === r.id ? { ...x, enabled: next } : x)),
    );
    try {
      await api.put(`/routes/${r.id}`, { ...r, enabled: next });
    } catch {
      setRoutes((prev) =>
        prev.map((x) => (x.id === r.id ? { ...x, enabled: !next } : x)),
      );
      toast("更新路由失败", "error");
    }
  }

  async function confirmDelete() {
    if (!deleteTarget) return;
    try {
      await api.delete(`/routes/${deleteTarget.id}`);
      toast("路由已删除");
      load();
    } catch {
      toast("删除路由失败", "error");
    } finally {
      setDeleteTarget(null);
    }
  }

  const enabledPools = pools.filter((p) => p.enabled);

  const columns: Column<ModelRoute>[] = [
    {
      key: "alias",
      header: "别名",
      render: (r) => (
        <span className="font-medium text-moon-800">{r.alias}</span>
      ),
      tone: "primary",
    },
    {
      key: "target_model",
      header: "目标模型",
      render: (r) => (
        <code className="text-xs text-moon-500">{r.target_model}</code>
      ),
      tone: "secondary",
    },
    {
      key: "pool",
      header: "池",
      render: (r) => (
        <span className="text-moon-500">{r.pool_label || "-"}</span>
      ),
      tone: "secondary",
    },
    {
      key: "enabled",
      header: "启用",
      render: (r) => (
        <Switch
          checked={r.enabled}
          onCheckedChange={() => toggleRoute(r)}
        />
      ),
      align: "center",
      tone: "status",
    },
    {
      key: "actions",
      header: "",
      className: "w-10",
      render: (r) => (
        <DropdownMenu>
          <DropdownMenuTrigger
            render={<Button variant="ghost" size="icon" className="size-8" />}
          >
            <MoreHorizontal className="size-4" />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onClick={() => openEdit(r)}>
              编辑
            </DropdownMenuItem>
            <DropdownMenuItem
              className="text-destructive"
              onClick={() => setDeleteTarget(r)}
            >
              删除
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      ),
    },
  ];

  return (
    <div className="space-y-8">
      <PageHeader
        eyebrow="工作区"
        title="路由"
        description="将公开模型别名映射到目标模型和路由池，并管理默认兜底路径。"
        meta={
          <span>
            共 {routes.length} 条显式路由 • 已启用 {enabledPools.length} 个池
          </span>
        }
        actions={
          <Button size="sm" onClick={openCreate}>
            <Plus className="size-4" />
            新增路由
          </Button>
        }
      />

      {loading ? (
        <div className="space-y-4">
          <Skeleton className="h-28 rounded-[1.5rem]" />
          <Skeleton className="h-72 rounded-[1.5rem]" />
        </div>
      ) : (
        <>
          <section className="space-y-4">
            <SectionHeading
              title="默认路由"
              description="当模型别名未命中显式路由时，使用这里的兜底池。"
            />
            <div className="rounded-[1.6rem] border border-moon-200/70 bg-[linear-gradient(180deg,rgba(255,255,255,0.95),rgba(240,242,248,0.92))] p-5">
              <div className="flex flex-col gap-4 lg:flex-row lg:items-center">
                <Label className="shrink-0 text-sm font-medium text-moon-700">
                  默认池
                </Label>
                <Select
                  value={
                    defaultPoolId !== null ? String(defaultPoolId) : "none"
                  }
                  onValueChange={updateDefaultPool}
                >
                  <SelectTrigger className="h-11 w-full rounded-xl border-moon-200 bg-white/90 lg:w-72">
                    <SelectValue placeholder="无" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="none">无</SelectItem>
                    {enabledPools.map((p) => (
                      <SelectItem key={p.id} value={String(p.id)}>
                        {p.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <span className="text-sm text-moon-500">
                  用于承接未匹配的模型请求。
                </span>
              </div>
            </div>
          </section>

          <section className="space-y-4">
            <SectionHeading
              title="路由表"
              description="查看模型别名、目标模型以及提供服务的池。"
            />
            <div className="overflow-hidden rounded-[1.6rem] border border-moon-200/70 bg-white/85">
              <DataTable
                columns={columns}
                rows={routes}
                rowKey={(r) => r.id}
                empty="暂未配置路由"
              />
            </div>
          </section>

          <div className="flex items-start gap-2 text-sm text-moon-500">
            <Info className="mt-0.5 size-4 shrink-0 text-moon-400" />
            <span>
              上面未列出的模型，会保留原始模型名并走默认池。
            </span>
          </div>
        </>
      )}

      <Dialog open={showForm} onOpenChange={setShowForm}>
        <DialogContent>
          <form onSubmit={handleSubmit}>
            <DialogHeader>
              <DialogTitle>
                {editId ? "编辑路由" : "新增路由"}
              </DialogTitle>
            </DialogHeader>

            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="route-alias">别名</Label>
                <Input
                  id="route-alias"
                  value={form.alias}
                  onChange={(e) =>
                    setForm({ ...form, alias: e.target.value })
                  }
                  required
                  placeholder="gpt-4o"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="route-target">目标模型</Label>
                <Input
                  id="route-target"
                  value={form.target_model}
                  onChange={(e) =>
                    setForm({ ...form, target_model: e.target.value })
                  }
                  required
                  placeholder="gpt-4o"
                />
                {form.alias &&
                  form.target_model &&
                  form.alias === form.target_model && (
                    <p className="text-xs text-moon-400">
                      别名与目标相同 — 模型名将原样透传。
                    </p>
                  )}
              </div>

              <div className="space-y-2">
                <Label>池</Label>
                <Select
                  value={form.pool_id !== null ? String(form.pool_id) : ""}
                  onValueChange={(v) =>
                    v && setForm({ ...form, pool_id: Number(v) })
                  }
                >
                  <SelectTrigger>
                    <SelectValue placeholder="选择池" />
                  </SelectTrigger>
                  <SelectContent>
                    {enabledPools.map((p) => (
                      <SelectItem key={p.id} value={String(p.id)}>
                        {p.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <div className="flex items-center gap-2">
                <Switch
                  checked={form.enabled}
                  onCheckedChange={(v) => setForm({ ...form, enabled: v })}
                />
                <Label>启用</Label>
              </div>
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

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        title="删除路由"
        description={`确认删除“${deleteTarget?.alias ?? ""}”这条路由吗？`}
        onConfirm={confirmDelete}
      />
    </div>
  );
}
