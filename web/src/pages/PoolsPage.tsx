import { type FormEvent, useEffect, useState } from "react";
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
  MoreHorizontal,
  Plus,
  Sparkles,
} from "lucide-react";

interface PoolForm {
  label: string;
  strategy: string;
}

const emptyForm: PoolForm = { label: "", strategy: "priority-first-healthy" };

function StrategyLabel({ strategy }: { strategy: string }) {
  const label =
    strategy === "priority-first-healthy"
      ? "优先健康优先级"
      : strategy === "round-robin"
        ? "轮询"
        : strategy === "weighted-random"
          ? "权重随机"
          : strategy;
  return (
    <span className="rounded-full bg-lunar-100/70 px-2.5 py-1 text-[11px] text-lunar-700">
      {label}
    </span>
  );
}

export default function PoolsPage() {
  const [pools, setPools] = useState<Pool[]>([]);
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [loading, setLoading] = useState(true);
  const [expanded, setExpanded] = useState<Set<number>>(new Set());
  const [showForm, setShowForm] = useState(false);
  const [editId, setEditId] = useState<number | null>(null);
  const [form, setForm] = useState<PoolForm>(emptyForm);
  const [deleteTarget, setDeleteTarget] = useState<Pool | null>(null);

  function load() {
    setLoading(true);
    let cancelled = false;

    Promise.all([
      api.get<Pool[]>("/pools").catch(() => {
        if (!cancelled) toast("加载池列表失败", "error");
        return null;
      }),
      api.get<Account[]>("/accounts").catch(() => null),
    ]).then(([poolList, accountList]) => {
      if (cancelled) return;
      if (poolList !== null) setPools(poolList ?? []);
      if (accountList !== null) setAccounts(accountList ?? []);
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

  function toggleExpand(id: number) {
    setExpanded((prev) => {
      const next = new Set(prev);
      next.has(id) ? next.delete(id) : next.add(id);
      return next;
    });
  }

  function openCreate() {
    setEditId(null);
    setForm(emptyForm);
    setShowForm(true);
  }

  function openEdit(pool: Pool) {
    setEditId(pool.id);
    setForm({ label: pool.label, strategy: pool.strategy });
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
    setPools((prev) => prev.map((item) => (item.id === pool.id ? { ...item, enabled: next } : item)));
    try {
      await api.post(`/pools/${pool.id}/${next ? "enable" : "disable"}`);
    } catch {
      setPools((prev) => prev.map((item) => (item.id === pool.id ? { ...item, enabled: !next } : item)));
      toast("更新池失败", "error");
    }
  }

  async function confirmDelete() {
    if (!deleteTarget) return;
    try {
      await api.delete(`/pools/${deleteTarget.id}`);
      toast("池已删除");
      load();
    } catch {
      toast("删除池失败", "error");
    } finally {
      setDeleteTarget(null);
    }
  }

  async function updateMember(
    pool: Pool,
    member: PoolMember,
    field: "priority" | "weight",
    value: number,
  ) {
    const updatedMembers = pool.members.map((m) =>
      m.id === member.id ? { ...m, [field]: value } : m,
    );
    try {
      await api.put(`/pools/${pool.id}`, {
        label: pool.label,
        strategy: pool.strategy,
        members: updatedMembers.map((m) => ({
          account_id: m.account_id,
          priority: m.priority,
          weight: m.weight,
        })),
      });
      load();
    } catch {
      toast("更新成员失败", "error");
    }
  }

  async function addMember(pool: Pool, accountId: number) {
    const members = [
      ...pool.members.map((m) => ({
        account_id: m.account_id,
        priority: m.priority,
        weight: m.weight,
      })),
      { account_id: accountId, priority: pool.members.length + 1, weight: 10 },
    ];
    try {
      await api.put(`/pools/${pool.id}`, {
        label: pool.label,
        strategy: pool.strategy,
        members,
      });
      toast("成员已添加");
      load();
    } catch {
      toast("添加成员失败", "error");
    }
  }

  async function removeMember(pool: Pool, member: PoolMember) {
    const members = pool.members
      .filter((m) => m.id !== member.id)
      .map((m) => ({
        account_id: m.account_id,
        priority: m.priority,
        weight: m.weight,
      }));
    try {
      await api.put(`/pools/${pool.id}`, {
        label: pool.label,
        strategy: pool.strategy,
        members,
      });
      toast("成员已移除");
      load();
    } catch {
      toast("移除成员失败", "error");
    }
  }

  if (loading) {
    return (
      <div className="space-y-8">
        <Skeleton className="h-24 rounded-[1.5rem]" />
        <Skeleton className="h-52 rounded-[1.8rem]" />
        <Skeleton className="h-72 rounded-[1.8rem]" />
      </div>
    );
  }

  const totalMembers = pools.reduce((count, pool) => count + pool.members.length, 0);

  return (
    <div className="space-y-10">
      <PageHeader
        eyebrow="Pools / Composition"
        title="池"
        description="把账号组合成可分发的执行组。"
        meta={
          <>
            <span>池 {pools.length}</span>
            <span>成员 {totalMembers}</span>
            <span>可用账号 {accounts.length}</span>
          </>
        }
        actions={
          <Button size="sm" onClick={openCreate}>
            <Plus className="size-4" />
            新增池
          </Button>
        }
      />

      <section className="grid gap-6 xl:grid-cols-[minmax(0,1.15fr)_minmax(320px,0.85fr)]">
        <div className="surface-section moon-grid px-6 py-6 sm:px-7">
          <p className="eyebrow-label">组合关系</p>
          <h2 className="mt-2 text-[1.15rem] font-semibold tracking-[-0.03em] text-moon-800">
            一个池，连接多个执行成员
          </h2>
          <p className="mt-3 max-w-2xl text-sm leading-7 text-moon-500">
            池不只是一个列表，它决定优先级、权重和承载路径。这里用更接近关系图的方式呈现，而不是一块空白大卡片。
          </p>

          <div className="mt-6 grid gap-3 md:grid-cols-3">
            <div className="rounded-[1.25rem] border border-white/72 bg-white/74 px-4 py-4">
              <p className="kicker">已启用池</p>
              <p className="mt-3 text-[1.5rem] font-semibold tracking-[-0.04em] text-moon-800">
                {pools.filter((pool) => pool.enabled).length}
              </p>
            </div>
            <div className="rounded-[1.25rem] border border-white/72 bg-white/74 px-4 py-4">
              <p className="kicker">成员总数</p>
              <p className="mt-3 text-[1.5rem] font-semibold tracking-[-0.04em] text-moon-800">
                {totalMembers}
              </p>
            </div>
            <div className="rounded-[1.25rem] border border-white/72 bg-white/74 px-4 py-4">
              <p className="kicker">待加入账号</p>
              <p className="mt-3 text-[1.5rem] font-semibold tracking-[-0.04em] text-moon-800">
                {accounts.length - totalMembers > 0 ? accounts.length - totalMembers : 0}
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
                  <Sparkles className="size-3.5" />
                </span>
                <span className="font-medium text-moon-800">{pool.label}</span>
                <span className="text-sm text-moon-500">成员 {pool.members.length}</span>
                <StrategyLabel strategy={pool.strategy} />
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
            <p>优先级更高的成员会先被选择。</p>
            <p>使用权重时，数值越高，分流概率越高。</p>
            <p>展开单个池后可以直接调成员顺序和权重。</p>
          </div>
        </aside>
      </section>

      <section className="space-y-4">
        <SectionHeading
          title="池列表"
          description="每个池都是一组可调度成员，展开后可直接微调关系。"
        />

        {pools.length === 0 ? (
          <div className="surface-card px-6 py-14 text-center">
            <p className="text-base font-medium text-moon-700">尚未配置池</p>
            <p className="mt-2 text-sm text-moon-400">新增一个池，用来组织多个执行账号。</p>
          </div>
        ) : (
          <div className="space-y-4">
            {pools.map((pool) => {
              const isOpen = expanded.has(pool.id);
              const memberAccountIds = new Set(pool.members.map((m) => m.account_id));
              const availableAccounts = accounts.filter((a) => !memberAccountIds.has(a.id));

              return (
                <div key={pool.id} className="surface-card overflow-hidden">
                  <div
                    className="flex cursor-pointer flex-col gap-4 px-5 py-5 lg:flex-row lg:items-center lg:justify-between"
                    onClick={() => toggleExpand(pool.id)}
                  >
                    <div className="flex items-start gap-3">
                      <div className="mt-1 text-moon-400">
                        {isOpen ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" />}
                      </div>
                      <div className="space-y-2">
                        <div className="flex flex-wrap items-center gap-3">
                          <span className="font-medium text-moon-800">{pool.label}</span>
                          <StatusBadge
                            status={pool.enabled ? "healthy" : "disabled"}
                            label={pool.enabled ? "已启用" : "已停用"}
                          />
                          <StrategyLabel strategy={pool.strategy} />
                        </div>
                        <p className="text-sm text-moon-500">
                          成员 {pool.members.length} 个
                        </p>
                      </div>
                    </div>

                    <div className="flex items-center gap-1" onClick={(e) => e.stopPropagation()}>
                      <DropdownMenu>
                        <DropdownMenuTrigger
                          render={<Button variant="ghost" size="icon" className="size-8" />}
                        >
                          <MoreHorizontal className="size-4" />
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem onClick={() => openEdit(pool)}>编辑</DropdownMenuItem>
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

                  {isOpen && (
                    <div className="border-t border-moon-200/60 px-5 pb-5 pt-4">
                      {pool.members.length === 0 ? (
                        <div className="rounded-[1.25rem] border border-dashed border-moon-200/80 px-4 py-10 text-center text-sm text-moon-400">
                          这个池还没有成员。先加入一个账号。
                        </div>
                      ) : (
                        <div className="grid gap-3">
                          {pool.members.map((member) => (
                            <div
                              key={member.id}
                              className="grid gap-3 rounded-[1.15rem] border border-white/72 bg-white/72 px-4 py-4 md:grid-cols-[88px_minmax(0,1fr)_108px_116px_auto]"
                            >
                              <div>
                                <p className="kicker">优先级</p>
                                <Input
                                  type="number"
                                  className="mt-2 h-9 rounded-lg bg-white/90 text-center text-sm"
                                  defaultValue={member.priority}
                                  onBlur={(e) =>
                                    updateMember(pool, member, "priority", Number(e.target.value))
                                  }
                                />
                              </div>

                              <div>
                                <p className="kicker">成员</p>
                                <p className="mt-2 font-medium text-moon-800">{member.account_label}</p>
                              </div>

                              <div>
                                <p className="kicker">权重</p>
                                <Input
                                  type="number"
                                  className="mt-2 h-9 rounded-lg bg-white/90 text-center text-sm"
                                  defaultValue={member.weight}
                                  onBlur={(e) =>
                                    updateMember(pool, member, "weight", Number(e.target.value))
                                  }
                                />
                              </div>

                              <div>
                                <p className="kicker">状态</p>
                                <div className="mt-2">
                                  <StatusBadge
                                    status={
                                      member.account_status as
                                        | "healthy"
                                        | "degraded"
                                        | "error"
                                        | "disabled"
                                    }
                                  />
                                </div>
                              </div>

                              <div className="flex items-end justify-end">
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  className="text-xs text-destructive"
                                  onClick={() => removeMember(pool, member)}
                                >
                                  移除
                                </Button>
                              </div>
                            </div>
                          ))}
                        </div>
                      )}

                      {availableAccounts.length > 0 && (
                        <div className="mt-4 flex items-center gap-2">
                          <Select onValueChange={(v) => v && addMember(pool, Number(v))}>
                            <SelectTrigger className="h-11 w-full rounded-xl border-white/75 bg-white/82 sm:w-72">
                              <SelectValue placeholder="添加账号到池" />
                            </SelectTrigger>
                            <SelectContent>
                              {availableAccounts.map((account) => (
                                <SelectItem key={account.id} value={String(account.id)}>
                                  {account.label}
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
                <Label>策略</Label>
                <Select value={form.strategy} onValueChange={(v) => v && setForm({ ...form, strategy: v })}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="priority-first-healthy">priority-first-healthy</SelectItem>
                    <SelectItem value="round-robin">round-robin</SelectItem>
                    <SelectItem value="weighted-random">weighted-random</SelectItem>
                  </SelectContent>
                </Select>
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
        title="删除池"
        description={`确认删除"${deleteTarget?.label ?? ""}"？关联成员也会一起移除。`}
        onConfirm={confirmDelete}
      />
    </div>
  );
}
