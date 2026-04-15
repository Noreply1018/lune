import { type FormEvent, useCallback, useEffect, useState } from "react";
import ConfirmDialog from "@/components/ConfirmDialog";
import PageHeader from "@/components/PageHeader";
import SectionHeading from "@/components/SectionHeading";
import StatusBadge from "@/components/StatusBadge";
import { api } from "@/lib/api";
import { toast } from "@/components/Feedback";
import type { Account, Pool, PoolMember } from "@/lib/types";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
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
import {
  ChevronDown,
  ChevronRight,
  Layers,
  MoreHorizontal,
  Plus,
  Trash2,
  Users,
} from "lucide-react";

/* ------------------------------------------------------------------ */
/*  Types                                                              */
/* ------------------------------------------------------------------ */

interface PoolForm {
  label: string;
  priority: number;
  enabled: boolean;
}

const emptyForm: PoolForm = { label: "", priority: 0, enabled: true };

/* ------------------------------------------------------------------ */
/*  Helpers                                                            */
/* ------------------------------------------------------------------ */

function PriorityBadge({ priority }: { priority: number }) {
  return (
    <span className="rounded-full bg-lunar-100/70 px-2.5 py-1 text-[11px] font-medium text-lunar-700">
      优先级 {priority}
    </span>
  );
}

function AccountHealthLabel({
  healthy,
  total,
}: {
  healthy: number;
  total: number;
}) {
  const allHealthy = total > 0 && healthy === total;
  const noneHealthy = total > 0 && healthy === 0;
  return (
    <span
      className={
        noneHealthy
          ? "text-status-red"
          : allHealthy
            ? "text-status-green"
            : "text-moon-600"
      }
    >
      {healthy}/{total}
    </span>
  );
}

/* ------------------------------------------------------------------ */
/*  Page                                                               */
/* ------------------------------------------------------------------ */

export default function PoolsPage() {
  const [pools, setPools] = useState<Pool[]>([]);
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [loading, setLoading] = useState(true);

  // Expand / collapse
  const [expanded, setExpanded] = useState<Set<number>>(new Set());
  const [memberMap, setMemberMap] = useState<Record<number, PoolMember[]>>({});
  const [memberLoading, setMemberLoading] = useState<Set<number>>(new Set());

  // Pool form dialog
  const [showForm, setShowForm] = useState(false);
  const [editId, setEditId] = useState<number | null>(null);
  const [form, setForm] = useState<PoolForm>(emptyForm);

  // Delete confirm
  const [deleteTarget, setDeleteTarget] = useState<Pool | null>(null);

  /* ---- data loading ---- */

  const load = useCallback(() => {
    setLoading(true);
    let cancelled = false;

    Promise.all([
      api.get<Pool[]>("/pools").catch(() => {
        if (!cancelled) toast("加载池列表失败", "error");
        return null;
      }),
      api.get<Account[]>("/accounts").catch(() => null),
    ])
      .then(([poolList, accountList]) => {
        if (cancelled) return;
        if (poolList !== null) setPools(poolList ?? []);
        if (accountList !== null) setAccounts(accountList ?? []);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    const cancel = load();
    return cancel;
  }, [load]);

  /* ---- load members for a pool ---- */

  async function loadMembers(poolId: number) {
    setMemberLoading((prev) => new Set(prev).add(poolId));
    try {
      const members = await api.get<PoolMember[]>(
        `/pools/${poolId}/members`,
      );
      setMemberMap((prev) => ({ ...prev, [poolId]: members ?? [] }));
    } catch {
      toast("加载成员失败", "error");
    } finally {
      setMemberLoading((prev) => {
        const next = new Set(prev);
        next.delete(poolId);
        return next;
      });
    }
  }

  /* ---- expand / collapse ---- */

  function toggleExpand(poolId: number) {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(poolId)) {
        next.delete(poolId);
      } else {
        next.add(poolId);
        // Load members on first expand
        if (!memberMap[poolId]) {
          loadMembers(poolId);
        }
      }
      return next;
    });
  }

  /* ---- pool CRUD ---- */

  function openCreate() {
    setEditId(null);
    setForm(emptyForm);
    setShowForm(true);
  }

  function openEdit(pool: Pool) {
    setEditId(pool.id);
    setForm({
      label: pool.label,
      priority: pool.priority,
      enabled: pool.enabled,
    });
    setShowForm(true);
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    try {
      if (editId) {
        await api.put(`/pools/${editId}`, form);
        toast("池已更新");
      } else {
        await api.post("/pools", form);
        toast("池已创建");
      }
      setShowForm(false);
      load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "操作失败", "error");
    }
  }

  async function togglePool(pool: Pool) {
    const next = !pool.enabled;
    setPools((prev) =>
      prev.map((item) =>
        item.id === pool.id ? { ...item, enabled: next } : item,
      ),
    );
    try {
      await api.post(`/pools/${pool.id}/${next ? "enable" : "disable"}`);
    } catch {
      setPools((prev) =>
        prev.map((item) =>
          item.id === pool.id ? { ...item, enabled: !next } : item,
        ),
      );
      toast("更新池状态失败", "error");
    }
  }

  async function confirmDelete() {
    if (!deleteTarget) return;
    try {
      await api.delete(`/pools/${deleteTarget.id}`);
      toast("池已删除");
      setExpanded((prev) => {
        const next = new Set(prev);
        next.delete(deleteTarget.id);
        return next;
      });
      setMemberMap((prev) => {
        const next = { ...prev };
        delete next[deleteTarget.id];
        return next;
      });
      load();
    } catch {
      toast("删除池失败", "error");
    } finally {
      setDeleteTarget(null);
    }
  }

  /* ---- member operations ---- */

  async function addMember(poolId: number, accountId: number) {
    try {
      await api.post(`/pools/${poolId}/members`, { account_id: accountId });
      toast("成员已添加");
      await loadMembers(poolId);
      load(); // refresh pool aggregate counts
    } catch {
      toast("添加成员失败", "error");
    }
  }

  async function removeMember(poolId: number, memberId: number) {
    try {
      await api.delete(`/pools/${poolId}/members/${memberId}`);
      toast("成员已移除");
      await loadMembers(poolId);
      load();
    } catch {
      toast("移除成员失败", "error");
    }
  }

  async function updateMemberPosition(
    poolId: number,
    memberId: number,
    position: number,
  ) {
    try {
      await api.put(`/pools/${poolId}/members/${memberId}`, { position });
      await loadMembers(poolId);
    } catch {
      toast("更新位置失败", "error");
    }
  }

  async function toggleMember(
    poolId: number,
    member: PoolMember,
  ) {
    const next = !member.enabled;
    // Optimistic update
    setMemberMap((prev) => ({
      ...prev,
      [poolId]: (prev[poolId] ?? []).map((m) =>
        m.id === member.id ? { ...m, enabled: next } : m,
      ),
    }));
    try {
      await api.put(`/pools/${poolId}/members/${member.id}`, {
        enabled: next,
      });
    } catch {
      // Rollback
      setMemberMap((prev) => ({
        ...prev,
        [poolId]: (prev[poolId] ?? []).map((m) =>
          m.id === member.id ? { ...m, enabled: !next } : m,
        ),
      }));
      toast("更新成员状态失败", "error");
    }
  }

  /* ---- loading skeleton ---- */

  if (loading) {
    return (
      <div className="space-y-8">
        <Skeleton className="h-24 rounded-[1.5rem]" />
        <Skeleton className="h-52 rounded-[1.8rem]" />
        <Skeleton className="h-72 rounded-[1.8rem]" />
      </div>
    );
  }

  /* ---- derived stats ---- */

  const totalAccounts = pools.reduce((n, p) => n + p.account_count, 0);
  const totalHealthy = pools.reduce((n, p) => n + p.healthy_account_count, 0);
  const allModels = new Set(pools.flatMap((p) => p.models));

  return (
    <div className="space-y-10">
      {/* ---- header ---- */}
      <PageHeader
        eyebrow="Pools / Composition"
        title="池"
        description="把账号组合成可分发的执行组。"
        meta={
          <>
            <span>池 {pools.length}</span>
            <span>
              健康账号 {totalHealthy}/{totalAccounts}
            </span>
            <span>模型 {allModels.size}</span>
          </>
        }
        actions={
          <Button size="sm" onClick={openCreate}>
            <Plus className="size-4" />
            新增池
          </Button>
        }
      />

      {/* ---- summary section ---- */}
      <section className="grid gap-6 xl:grid-cols-[minmax(0,1.15fr)_minmax(320px,0.85fr)]">
        <div className="surface-section moon-grid px-6 py-6 sm:px-7">
          <p className="eyebrow-label">组合关系</p>
          <h2 className="mt-2 text-[1.15rem] font-semibold tracking-[-0.03em] text-moon-800">
            一个池，连接多个执行成员
          </h2>
          <p className="mt-3 max-w-2xl text-sm leading-7 text-moon-500">
            池决定账号的调度顺序。优先级数值越小越优先；成员按 position 顺序选取。
          </p>

          <div className="mt-6 grid gap-3 md:grid-cols-3">
            <div className="rounded-[1.25rem] border border-white/72 bg-white/74 px-4 py-4">
              <p className="kicker">已启用池</p>
              <p className="mt-3 text-[1.5rem] font-semibold tracking-[-0.04em] text-moon-800">
                {pools.filter((p) => p.enabled).length}
              </p>
            </div>
            <div className="rounded-[1.25rem] border border-white/72 bg-white/74 px-4 py-4">
              <p className="kicker">健康账号</p>
              <p className="mt-3 text-[1.5rem] font-semibold tracking-[-0.04em] text-moon-800">
                {totalHealthy}/{totalAccounts}
              </p>
            </div>
            <div className="rounded-[1.25rem] border border-white/72 bg-white/74 px-4 py-4">
              <p className="kicker">可用模型</p>
              <p className="mt-3 text-[1.5rem] font-semibold tracking-[-0.04em] text-moon-800">
                {allModels.size}
              </p>
            </div>
          </div>

          <div className="mt-6 space-y-3">
            {pools.slice(0, 3).map((pool) => (
              <div
                key={pool.id}
                className="flex flex-wrap items-center gap-3 rounded-[1.1rem] border border-white/70 bg-white/68 px-4 py-3"
              >
                <span className="inline-flex size-7 items-center justify-center rounded-full bg-lunar-100 text-lunar-700">
                  <Layers className="size-3.5" />
                </span>
                <span className="font-medium text-moon-800">{pool.label}</span>
                <span className="text-sm text-moon-500">
                  成员{" "}
                  <AccountHealthLabel
                    healthy={pool.healthy_account_count}
                    total={pool.account_count}
                  />
                </span>
                <PriorityBadge priority={pool.priority} />
              </div>
            ))}
            {pools.length === 0 && (
              <div className="rounded-[1.3rem] border border-dashed border-moon-200/80 px-5 py-10 text-center text-sm text-moon-400">
                还没有任何池，先创建一个组合入口。
              </div>
            )}
          </div>
        </div>

        <aside className="surface-card px-5 py-5">
          <p className="eyebrow-label">设计提示</p>
          <div className="mt-4 space-y-4 text-sm leading-6 text-moon-500">
            <p>
              <strong className="text-moon-700">优先级</strong>{" "}
              数值越小越优先（0 最高）。路由时从优先级最高的池开始匹配。
            </p>
            <p>
              <strong className="text-moon-700">位置 (position)</strong>{" "}
              决定池内成员的选取顺序。
            </p>
            <p>
              <strong className="text-moon-700">成员启停</strong>{" "}
              可单独禁用成员而不移出池。
            </p>
            <p>展开单个池后可以直接管理成员、调顺序。</p>
          </div>
        </aside>
      </section>

      {/* ---- pool list ---- */}
      <section className="space-y-4">
        <SectionHeading
          title="池列表"
          description="每个池都是一组可调度成员，展开后可直接管理。"
        />

        {pools.length === 0 ? (
          <div className="surface-card px-6 py-14 text-center">
            <p className="text-base font-medium text-moon-700">尚未配置池</p>
            <p className="mt-2 text-sm text-moon-400">
              新增一个池，用来组织多个执行账号。
            </p>
          </div>
        ) : (
          <div className="space-y-4">
            {pools.map((pool) => {
              const isOpen = expanded.has(pool.id);
              const members = memberMap[pool.id] ?? [];
              const membersLoading = memberLoading.has(pool.id);
              const memberAccountIds = new Set(
                members.map((m) => m.account_id),
              );
              const availableAccounts = accounts.filter(
                (a) => !memberAccountIds.has(a.id),
              );

              return (
                <div key={pool.id} className="surface-card overflow-hidden">
                  {/* ---- pool row ---- */}
                  <div
                    className="flex cursor-pointer flex-col gap-4 px-5 py-5 lg:flex-row lg:items-center lg:justify-between"
                    onClick={() => toggleExpand(pool.id)}
                  >
                    <div className="flex items-start gap-3">
                      <div className="mt-1 text-moon-400">
                        {isOpen ? (
                          <ChevronDown className="size-4" />
                        ) : (
                          <ChevronRight className="size-4" />
                        )}
                      </div>
                      <div className="space-y-2">
                        <div className="flex flex-wrap items-center gap-3">
                          <span className="font-medium text-moon-800">
                            {pool.label}
                          </span>
                          <StatusBadge
                            status={pool.enabled ? "healthy" : "disabled"}
                            label={pool.enabled ? "已启用" : "已停用"}
                          />
                          <PriorityBadge priority={pool.priority} />
                        </div>
                        <div className="flex flex-wrap items-center gap-4 text-sm text-moon-500">
                          <span>
                            账号{" "}
                            <AccountHealthLabel
                              healthy={pool.healthy_account_count}
                              total={pool.account_count}
                            />
                          </span>
                          {pool.models.length > 0 && (
                            <span>模型 {pool.models.length}</span>
                          )}
                        </div>
                      </div>
                    </div>

                    <div
                      className="flex items-center gap-2"
                      onClick={(e) => e.stopPropagation()}
                    >
                      <Switch
                        size="sm"
                        checked={pool.enabled}
                        onCheckedChange={() => togglePool(pool)}
                      />
                      <DropdownMenu>
                        <DropdownMenuTrigger
                          render={
                            <Button
                              variant="ghost"
                              size="icon"
                              className="size-8"
                            />
                          }
                        >
                          <MoreHorizontal className="size-4" />
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem onClick={() => openEdit(pool)}>
                            编辑
                          </DropdownMenuItem>
                          <DropdownMenuItem onClick={() => togglePool(pool)}>
                            {pool.enabled ? "停用" : "启用"}
                          </DropdownMenuItem>
                          <DropdownMenuItem
                            className="text-destructive"
                            onClick={() => setDeleteTarget(pool)}
                          >
                            删除
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </div>
                  </div>

                  {/* ---- expanded members panel ---- */}
                  {isOpen && (
                    <div className="border-t border-moon-200/60 px-5 pb-5 pt-4">
                      {membersLoading ? (
                        <div className="space-y-3">
                          <Skeleton className="h-16 rounded-[1.15rem]" />
                          <Skeleton className="h-16 rounded-[1.15rem]" />
                        </div>
                      ) : members.length === 0 ? (
                        <div className="rounded-[1.25rem] border border-dashed border-moon-200/80 px-4 py-10 text-center text-sm text-moon-400">
                          这个池还没有成员。先加入一个账号。
                        </div>
                      ) : (
                        <div className="grid gap-3">
                          {members.map((member) => (
                            <div
                              key={member.id}
                              className="grid gap-3 rounded-[1.15rem] border border-white/72 bg-white/72 px-4 py-4 md:grid-cols-[72px_minmax(0,1fr)_96px_auto]"
                            >
                              <div>
                                <p className="kicker">位置</p>
                                <Input
                                  type="number"
                                  className="mt-2 h-9 rounded-lg bg-white/90 text-center text-sm"
                                  defaultValue={member.position}
                                  key={`pos-${member.id}-${member.position}`}
                                  onBlur={(e) => {
                                    const val = Number(e.target.value);
                                    if (val !== member.position) {
                                      updateMemberPosition(
                                        pool.id,
                                        member.id,
                                        val,
                                      );
                                    }
                                  }}
                                />
                              </div>

                              <div className="min-w-0">
                                <p className="kicker">成员</p>
                                <div className="mt-2 flex flex-wrap items-center gap-2">
                                  <span className="font-medium text-moon-800">
                                    {member.account?.label ??
                                      `账号 #${member.account_id}`}
                                  </span>
                                  {member.account && (
                                    <StatusBadge
                                      status={member.account.status}
                                    />
                                  )}
                                  {member.account?.provider && (
                                    <span className="text-xs text-moon-400">
                                      {member.account.provider}
                                    </span>
                                  )}
                                </div>
                              </div>

                              <div>
                                <p className="kicker">启用</p>
                                <div className="mt-2.5">
                                  <Switch
                                    size="sm"
                                    checked={member.enabled}
                                    onCheckedChange={() =>
                                      toggleMember(pool.id, member)
                                    }
                                  />
                                </div>
                              </div>

                              <div className="flex items-end justify-end">
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  className="text-xs text-destructive"
                                  onClick={() =>
                                    removeMember(pool.id, member.id)
                                  }
                                >
                                  <Trash2 className="mr-1 size-3" />
                                  移除
                                </Button>
                              </div>
                            </div>
                          ))}
                        </div>
                      )}

                      {/* ---- add member ---- */}
                      {availableAccounts.length > 0 && (
                        <div className="mt-4 flex items-center gap-2">
                          <Select
                            onValueChange={(v) =>
                              v && addMember(pool.id, Number(v))
                            }
                          >
                            <SelectTrigger className="h-11 w-full rounded-xl border-white/75 bg-white/82 sm:w-72">
                              <SelectValue placeholder="添加账号到池" />
                            </SelectTrigger>
                            <SelectContent>
                              {availableAccounts.map((account) => (
                                <SelectItem
                                  key={account.id}
                                  value={String(account.id)}
                                >
                                  <span className="flex items-center gap-2">
                                    <Users className="size-3.5 text-moon-400" />
                                    {account.label}
                                    {account.provider && (
                                      <span className="text-xs text-moon-400">
                                        ({account.provider})
                                      </span>
                                    )}
                                  </span>
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                        </div>
                      )}
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </section>

      {/* ---- create / edit dialog ---- */}
      <Dialog open={showForm} onOpenChange={setShowForm}>
        <DialogContent>
          <form onSubmit={handleSubmit}>
            <DialogHeader>
              <DialogTitle>{editId ? "编辑池" : "新增池"}</DialogTitle>
            </DialogHeader>

            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="pool-label">标签</Label>
                <Input
                  id="pool-label"
                  value={form.label}
                  onChange={(e) => setForm({ ...form, label: e.target.value })}
                  required
                  placeholder="主力池"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="pool-priority">优先级</Label>
                <Input
                  id="pool-priority"
                  type="number"
                  value={form.priority}
                  onChange={(e) =>
                    setForm({ ...form, priority: Number(e.target.value) })
                  }
                  placeholder="0"
                />
                <p className="text-xs text-moon-400">
                  数值越小优先级越高，0 为最高。
                </p>
              </div>

              <div className="flex items-center gap-3">
                <Switch
                  id="pool-enabled"
                  checked={form.enabled}
                  onCheckedChange={(checked) =>
                    setForm({ ...form, enabled: Boolean(checked) })
                  }
                />
                <Label htmlFor="pool-enabled">启用</Label>
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

      {/* ---- delete confirm ---- */}
      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        title="删除池"
        description={`确认删除"${deleteTarget?.label ?? ""}"？关联成员也会一起移除。`}
        onConfirm={confirmDelete}
      />
    </div>
  );
}
