import { type FormEvent, useEffect, useState } from "react";
import ConfirmDialog from "@/components/ConfirmDialog";
import DataTable, { type Column } from "@/components/DataTable";
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
import { ArrowRight, GitBranch, MoreHorizontal, Plus } from "lucide-react";

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
    ]).then(([routeList, poolList, settings]) => {
      if (cancelled) return;
      if (routeList !== null) setRoutes(routeList ?? []);
      if (poolList !== null) setPools(poolList ?? []);
      if (settings !== null) setDefaultPoolId(settings?.default_pool_id ?? null);
    }).finally(() => {
      if (!cancelled) setLoading(false);
    });

    return () => {
      cancelled = true;
    };
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

  function openEdit(route: ModelRoute) {
    setEditId(route.id);
    setForm({
      alias: route.alias,
      target_model: route.target_model,
      pool_id: route.pool_id,
      enabled: route.enabled,
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

  async function toggleRoute(route: ModelRoute) {
    const next = !route.enabled;
    setRoutes((prev) => prev.map((item) => (item.id === route.id ? { ...item, enabled: next } : item)));
    try {
      await api.put(`/routes/${route.id}`, { ...route, enabled: next });
    } catch {
      setRoutes((prev) => prev.map((item) => (item.id === route.id ? { ...item, enabled: !next } : item)));
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

  const enabledPools = pools.filter((pool) => pool.enabled);

  const columns: Column<ModelRoute>[] = [
    {
      key: "alias",
      header: "公开模型名",
      render: (route) => <span className="font-medium text-moon-800">{route.alias}</span>,
      tone: "primary",
    },
    {
      key: "mapping",
      header: "映射结果",
      render: (route) => (
        <div className="flex items-center gap-2 text-sm text-moon-500">
          <code className="rounded bg-moon-100/80 px-2 py-1 text-xs text-moon-700">
            {route.target_model}
          </code>
          <ArrowRight className="size-4 text-moon-300" />
          <span>{route.pool_label || "-"}</span>
        </div>
      ),
      tone: "secondary",
    },
    {
      key: "enabled",
      header: "启用",
      render: (route) => (
        <Switch checked={route.enabled} onCheckedChange={() => toggleRoute(route)} />
      ),
      align: "center",
      tone: "status",
    },
    {
      key: "actions",
      header: "",
      className: "w-10",
      render: (route) => (
        <DropdownMenu>
          <DropdownMenuTrigger
            render={<Button variant="ghost" size="icon" className="size-8" />}
          >
            <MoreHorizontal className="size-4" />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onClick={() => openEdit(route)}>编辑</DropdownMenuItem>
            <DropdownMenuItem
              className="text-destructive"
              onClick={() => setDeleteTarget(route)}
            >
              删除
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      ),
    },
  ];

  return (
    <div className="space-y-10">
      <PageHeader
        eyebrow="Routes / Mapping"
        title="路由"
        description="把公开模型名映射到真实目标模型与承载池。"
        meta={
          <>
            <span>显式路由 {routes.length}</span>
            <span>已启用池 {enabledPools.length}</span>
            <span>默认池 {defaultPoolId !== null ? 1 : 0}</span>
          </>
        }
        actions={
          <Button size="sm" onClick={openCreate}>
            <Plus className="size-4" />
            新增路由
          </Button>
        }
      />

      {loading ? (
        <div className="space-y-6">
          <Skeleton className="h-56 rounded-[1.8rem]" />
          <Skeleton className="h-80 rounded-[1.8rem]" />
        </div>
      ) : (
        <>
          <section className="grid gap-6 xl:grid-cols-[minmax(0,1.05fr)_minmax(340px,0.95fr)]">
            <div className="surface-section px-6 py-6 sm:px-7">
              <p className="eyebrow-label">规则结构</p>
              <h2 className="mt-2 text-[1.15rem] font-semibold tracking-[-0.03em] text-moon-800">
                公开模型名 → 目标模型 / 池
              </h2>
              <p className="mt-3 max-w-2xl text-sm leading-7 text-moon-500">
                路由页不再讲口号，只强调规则本身。默认池负责兜底，显式映射负责精确指向。
              </p>

              <div className="mt-6 space-y-3">
                {routes.slice(0, 4).map((route) => (
                  <div
                    key={route.id}
                    className="flex flex-wrap items-center gap-3 rounded-[1.15rem] border border-white/72 bg-white/74 px-4 py-3"
                  >
                    <code className="rounded bg-moon-100/80 px-2 py-1 text-xs text-moon-700">
                      {route.alias}
                    </code>
                    <ArrowRight className="size-4 text-moon-300" />
                    <code className="rounded bg-lunar-100/70 px-2 py-1 text-xs text-lunar-700">
                      {route.target_model}
                    </code>
                    <span className="text-sm text-moon-500">{route.pool_label || "未指定池"}</span>
                  </div>
                ))}
                {routes.length === 0 && (
                  <div className="rounded-[1.3rem] border border-dashed border-moon-200/80 px-5 py-10 text-center text-sm text-moon-400">
                    还没有显式路由规则。
                  </div>
                )}
              </div>
            </div>

            <aside className="surface-card px-5 py-5">
              <p className="eyebrow-label">默认承载</p>
              <div className="mt-4 space-y-4">
                <div className="rounded-[1.15rem] border border-white/72 bg-white/72 px-4 py-4">
                  <p className="kicker">默认池</p>
                  <p className="mt-3 text-[1.3rem] font-semibold tracking-[-0.04em] text-moon-800">
                    {pools.find((pool) => pool.id === defaultPoolId)?.label ?? "未设置"}
                  </p>
                  <p className="mt-2 text-sm text-moon-500">
                    未命中显式路由时，将落到这里。
                  </p>
                </div>
                <div className="rounded-[1.15rem] border border-white/72 bg-white/72 px-4 py-4">
                  <p className="kicker">规则数</p>
                  <p className="mt-3 text-[1.3rem] font-semibold tracking-[-0.04em] text-moon-800">
                    {routes.length}
                  </p>
                  <p className="mt-2 text-sm text-moon-500">
                    已定义的显式映射。
                  </p>
                </div>
              </div>
            </aside>
          </section>

          <section className="grid gap-6 xl:grid-cols-[minmax(320px,0.6fr)_minmax(0,1.4fr)]">
            <div className="space-y-4">
              <SectionHeading
                title="默认路由"
                description="当模型名未命中显式规则时，使用这里的池。"
              />
              <div className="surface-card px-5 py-5">
                <div className="space-y-4">
                  <div className="flex items-center gap-3 text-sm text-moon-500">
                    <GitBranch className="size-4 text-moon-400" />
                    默认规则只做兜底，不参与显式优先级竞争。
                  </div>
                  <Select
                    value={defaultPoolId !== null ? String(defaultPoolId) : "none"}
                    onValueChange={updateDefaultPool}
                  >
                    <SelectTrigger className="h-11 rounded-xl border-white/75 bg-white/84">
                      <SelectValue placeholder="无" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="none">无</SelectItem>
                      {enabledPools.map((pool) => (
                        <SelectItem key={pool.id} value={String(pool.id)}>
                          {pool.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              </div>
            </div>

            <div className="space-y-4">
              <SectionHeading
                title="显式映射"
                description="查看公开模型名如何被重定向到真实承载路径。"
              />
              <div className="surface-card overflow-hidden">
                <DataTable
                  columns={columns}
                  rows={routes}
                  rowKey={(route) => route.id}
                  empty="暂未配置路由"
                />
              </div>
            </div>
          </section>
        </>
      )}

      <Dialog open={showForm} onOpenChange={setShowForm}>
        <DialogContent>
          <form onSubmit={handleSubmit}>
            <DialogHeader>
              <DialogTitle>{editId ? "编辑路由" : "新增路由"}</DialogTitle>
            </DialogHeader>

            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="route-alias">公开模型名</Label>
                <Input
                  id="route-alias"
                  value={form.alias}
                  onChange={(e) => setForm({ ...form, alias: e.target.value })}
                  required
                  placeholder="gpt-4o"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="route-target">目标模型</Label>
                <Input
                  id="route-target"
                  value={form.target_model}
                  onChange={(e) => setForm({ ...form, target_model: e.target.value })}
                  required
                  placeholder="gpt-4o-2024-11-20"
                />
                {form.alias && form.target_model && form.alias === form.target_model && (
                  <p className="text-xs text-moon-400">别名与目标相同，模型名将原样透传。</p>
                )}
              </div>

              <div className="space-y-2">
                <Label>承载池</Label>
                <Select
                  value={form.pool_id !== null ? String(form.pool_id) : ""}
                  onValueChange={(v) => v && setForm({ ...form, pool_id: Number(v) })}
                >
                  <SelectTrigger>
                    <SelectValue placeholder="选择池" />
                  </SelectTrigger>
                  <SelectContent>
                    {enabledPools.map((pool) => (
                      <SelectItem key={pool.id} value={String(pool.id)}>
                        {pool.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <div className="flex items-center gap-2">
                <Switch checked={form.enabled} onCheckedChange={(v) => setForm({ ...form, enabled: v })} />
                <Label>启用</Label>
              </div>
            </div>

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setShowForm(false)}>
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
