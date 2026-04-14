import { type FormEvent, useEffect, useState } from "react";
import StatusBadge from "@/components/StatusBadge";
import ConfirmDialog from "@/components/ConfirmDialog";
import PageHeader from "@/components/PageHeader";
import SectionHeading from "@/components/SectionHeading";
import { api } from "@/lib/api";
import { toast } from "@/components/Feedback";
import type { Pool, PoolMember, Account } from "@/lib/types";
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
  Plus,
  ChevronDown,
  ChevronRight,
  MoreHorizontal,
} from "lucide-react";

interface PoolForm {
  label: string;
  strategy: string;
}

const emptyForm: PoolForm = { label: "", strategy: "priority-first-healthy" };

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
    Promise.all([
      api.get<Pool[]>("/pools"),
      api.get<Account[]>("/accounts"),
    ])
      .then(([p, a]) => {
        setPools(p ?? []);
        setAccounts(a ?? []);
      })
      .catch(() => toast("加载池失败", "error"))
      .finally(() => setLoading(false));
  }

  useEffect(load, []);

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

  function openEdit(p: Pool) {
    setEditId(p.id);
    setForm({ label: p.label, strategy: p.strategy });
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

  async function togglePool(p: Pool) {
    const next = !p.enabled;
    setPools((prev) =>
      prev.map((x) => (x.id === p.id ? { ...x, enabled: next } : x)),
    );
    try {
      await api.post(`/pools/${p.id}/${next ? "enable" : "disable"}`);
    } catch {
      setPools((prev) =>
        prev.map((x) => (x.id === p.id ? { ...x, enabled: !next } : x)),
      );
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
      {
        account_id: accountId,
        priority: pool.members.length + 1,
        weight: 10,
      },
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
      <div className="space-y-6">
        <Skeleton className="h-24 rounded-[1.5rem]" />
        <Skeleton className="h-40 rounded-[1.5rem]" />
        <Skeleton className="h-40 rounded-[1.5rem]" />
      </div>
    );
  }

  return (
    <div className="space-y-8">
      <PageHeader
        eyebrow="工作区"
        title="池"
        description="组合上游账号形成路由池，并直接调整成员优先级和权重。"
        meta={
          <span>
            共 {pools.length} 个池 • 成员总数 {pools.reduce((count, pool) => count + pool.members.length, 0)}
          </span>
        }
        actions={
          <Button size="sm" onClick={openCreate}>
            <Plus className="size-4" />
            新增池
          </Button>
        }
      />

      <section className="space-y-4">
        <SectionHeading
          title="池列表"
          description="展开一个池即可管理成员顺序、权重和账号组成。"
        />

        {pools.length === 0 && (
          <p className="rounded-[1.6rem] border border-dashed border-moon-200/80 py-12 text-center text-sm text-moon-400">
            暂未配置池
          </p>
        )}

        <div className="space-y-4">
        {pools.map((pool) => {
          const isOpen = expanded.has(pool.id);
          const memberAccountIds = new Set(
            pool.members.map((m) => m.account_id),
          );
          const availableAccounts = accounts.filter(
            (a) => !memberAccountIds.has(a.id),
          );

          return (
            <div
              key={pool.id}
              className="overflow-hidden rounded-[1.6rem] border border-moon-200/70 bg-white/88 transition-shadow hover:shadow-[0_18px_40px_-36px_rgba(36,43,74,0.45)]"
            >
              <div
                className="flex cursor-pointer flex-col gap-4 px-5 py-5 lg:flex-row lg:items-center lg:justify-between"
                onClick={() => toggleExpand(pool.id)}
              >
                <div className="flex items-start gap-3">
                  <div className="mt-1">
                    {isOpen ? (
                      <ChevronDown className="size-4 text-moon-400" />
                    ) : (
                      <ChevronRight className="size-4 text-moon-400" />
                    )}
                  </div>
                  <div className="space-y-2">
                    <div className="flex flex-wrap items-center gap-3">
                      <span className="font-medium text-moon-800">
                        {pool.label}
                      </span>
                      <StatusBadge
                        status={pool.enabled ? "healthy" : "disabled"}
                        label={pool.enabled ? "Enabled" : "Disabled"}
                      />
                      <code className="rounded-full bg-moon-100 px-2.5 py-1 text-[11px] text-moon-500">
                        {pool.strategy}
                      </code>
                    </div>
                    <p className="text-sm text-moon-500">
                      {pool.members?.length ?? 0} member{pool.members.length === 1 ? "" : "s"} in this pool.
                    </p>
                  </div>
                </div>
                <div
                  className="flex items-center gap-1"
                  onClick={(e) => e.stopPropagation()}
                >
                  <DropdownMenu>
                    <DropdownMenuTrigger
                      render={<Button variant="ghost" size="icon" className="size-8" />}
                    >
                      <MoreHorizontal className="size-4" />
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem onClick={() => openEdit(pool)}>
                        Edit
                      </DropdownMenuItem>
                      <DropdownMenuItem onClick={() => togglePool(pool)}>
                        {pool.enabled ? "Disable" : "Enable"}
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        className="text-destructive"
                        onClick={() => setDeleteTarget(pool)}
                      >
                        Delete
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
              </div>

              {isOpen && (
                <div className="border-t border-moon-200/60 px-5 pb-5 pt-4">
                  {pool.members.length === 0 ? (
                    <p className="py-4 text-center text-sm text-moon-400">
                      No members
                    </p>
                  ) : (
                    <div className="overflow-x-auto">
                      <table className="w-full min-w-[640px] text-sm">
                        <thead>
                          <tr className="text-left text-[11px] font-semibold uppercase tracking-[0.2em] text-moon-400">
                            <th className="pb-3">Priority</th>
                            <th className="pb-3">Account</th>
                            <th className="pb-3 text-right">Weight</th>
                            <th className="pb-3">Status</th>
                            <th className="pb-3 text-right">Actions</th>
                          </tr>
                        </thead>
                        <tbody className="divide-y divide-moon-200/50">
                          {pool.members.map((m) => (
                            <tr key={m.id}>
                              <td className="py-2">
                                <Input
                                  type="number"
                                  className="h-7 w-16 text-center text-xs"
                                  defaultValue={m.priority}
                                  onBlur={(e) =>
                                    updateMember(
                                      pool,
                                      m,
                                      "priority",
                                      Number(e.target.value),
                                    )
                                  }
                                />
                              </td>
                              <td className="py-2 font-medium text-moon-800">
                                {m.account_label}
                              </td>
                              <td className="py-2 text-right">
                                <Input
                                  type="number"
                                  className="ml-auto h-7 w-20 text-center text-xs"
                                  defaultValue={m.weight}
                                  onBlur={(e) =>
                                    updateMember(
                                      pool,
                                      m,
                                      "weight",
                                      Number(e.target.value),
                                    )
                                  }
                                />
                              </td>
                              <td className="py-2">
                                <StatusBadge
                                  status={
                                    m.account_status as
                                      | "healthy"
                                      | "degraded"
                                      | "error"
                                      | "disabled"
                                  }
                                />
                              </td>
                              <td className="py-2 text-right">
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  className="text-xs text-destructive"
                                  onClick={() => removeMember(pool, m)}
                                >
                                  Remove
                                </Button>
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  )}

                  {availableAccounts.length > 0 && (
                    <div className="mt-4 flex items-center gap-2">
                      <Select
                        onValueChange={(v) => v && addMember(pool, Number(v))}
                      >
                        <SelectTrigger className="h-11 w-full rounded-xl border-moon-200 bg-moon-50 sm:w-72">
                          <SelectValue placeholder="Add account to pool" />
                        </SelectTrigger>
                        <SelectContent>
                          {availableAccounts.map((a) => (
                            <SelectItem
                              key={a.id}
                              value={String(a.id)}
                            >
                              {a.label}
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
      </section>

      <Dialog open={showForm} onOpenChange={setShowForm}>
        <DialogContent>
          <form onSubmit={handleSubmit}>
            <DialogHeader>
              <DialogTitle>
                {editId ? "编辑池" : "新增池"}
              </DialogTitle>
            </DialogHeader>

            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="pool-label">Label</Label>
                <Input
                  id="pool-label"
                  value={form.label}
                  onChange={(e) => setForm({ ...form, label: e.target.value })}
                  required
                  placeholder="OpenAI Pool"
                />
              </div>

              <div className="space-y-2">
                <Label>Strategy</Label>
                <Select
                  value={form.strategy}
                  onValueChange={(v) => v && setForm({ ...form, strategy: v })}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="priority-first-healthy">
                      priority-first-healthy
                    </SelectItem>
                    <SelectItem value="round-robin">round-robin</SelectItem>
                    <SelectItem value="weighted-random">
                      weighted-random
                    </SelectItem>
                  </SelectContent>
                </Select>
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
        title="删除池"
        description={`Are you sure you want to delete "${deleteTarget?.label ?? ""}"? This will also remove all member associations.`}
        onConfirm={confirmDelete}
      />
    </div>
  );
}
