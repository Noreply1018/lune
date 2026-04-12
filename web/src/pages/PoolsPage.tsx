import { type FormEvent, useEffect, useState } from "react";
import StatusBadge from "@/components/StatusBadge";
import ConfirmDialog from "@/components/ConfirmDialog";
import { api } from "@/lib/api";
import { toast } from "@/components/Feedback";
import type { Pool, PoolMember, Account } from "@/lib/types";
import { Card, CardContent } from "@/components/ui/card";
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
      .catch(() => toast("Failed to load pools", "error"))
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
        toast("Pool updated");
      } else {
        await api.post("/pools", form);
        toast("Pool created");
      }
      setShowForm(false);
      load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "Operation failed", "error");
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
      toast("Failed to update pool", "error");
    }
  }

  async function confirmDelete() {
    if (!deleteTarget) return;
    try {
      await api.delete(`/pools/${deleteTarget.id}`);
      toast("Pool deleted");
      load();
    } catch {
      toast("Failed to delete pool", "error");
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
      toast("Failed to update member", "error");
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
      toast("Member added");
      load();
    } catch {
      toast("Failed to add member", "error");
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
      toast("Member removed");
      load();
    } catch {
      toast("Failed to remove member", "error");
    }
  }

  if (loading) {
    return (
      <div className="space-y-6">
        <Skeleton className="h-7 w-20" />
        <Skeleton className="h-32" />
        <Skeleton className="h-32" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">Pools</h2>
        <Button size="sm" onClick={openCreate}>
          <Plus className="size-4" />
          Add Pool
        </Button>
      </div>

      {pools.length === 0 && (
        <p className="py-10 text-center text-sm text-moon-400">
          No pools configured
        </p>
      )}

      <div className="space-y-3">
        {pools.map((pool) => {
          const isOpen = expanded.has(pool.id);
          const memberAccountIds = new Set(
            pool.members.map((m) => m.account_id),
          );
          const availableAccounts = accounts.filter(
            (a) => !memberAccountIds.has(a.id),
          );

          return (
            <Card
              key={pool.id}
              className="ring-1 ring-moon-200/60 transition-shadow hover:shadow-sm"
            >
              <CardContent className="p-0">
                <div
                  className="flex cursor-pointer items-center justify-between px-5 py-4"
                  onClick={() => toggleExpand(pool.id)}
                >
                  <div className="flex items-center gap-3">
                    {isOpen ? (
                      <ChevronDown className="size-4 text-moon-400" />
                    ) : (
                      <ChevronRight className="size-4 text-moon-400" />
                    )}
                    <span className="font-medium text-moon-800">
                      {pool.label}
                    </span>
                    <code className="rounded bg-moon-100 px-2 py-0.5 text-xs text-moon-500">
                      {pool.strategy}
                    </code>
                    <StatusBadge
                      status={pool.enabled ? "healthy" : "disabled"}
                      label={pool.enabled ? "Enabled" : "Disabled"}
                    />
                  </div>
                  <div
                    className="flex items-center gap-1"
                    onClick={(e) => e.stopPropagation()}
                  >
                    <span className="mr-2 text-xs text-moon-400">
                      {pool.members?.length ?? 0} members
                    </span>
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
                  <div className="border-t border-moon-200/60 px-5 pb-4 pt-3">
                    {pool.members.length === 0 ? (
                      <p className="py-4 text-center text-sm text-moon-400">
                        No members
                      </p>
                    ) : (
                      <table className="w-full text-sm">
                        <thead>
                          <tr className="text-left text-xs font-medium uppercase tracking-wider text-moon-400">
                            <th className="pb-2">Priority</th>
                            <th className="pb-2">Account</th>
                            <th className="pb-2">Weight</th>
                            <th className="pb-2">Status</th>
                            <th className="pb-2 text-right">Actions</th>
                          </tr>
                        </thead>
                        <tbody className="divide-y divide-moon-200/40">
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
                              <td className="py-2">
                                <Input
                                  type="number"
                                  className="h-7 w-16 text-center text-xs"
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
                    )}

                    {availableAccounts.length > 0 && (
                      <div className="mt-3 flex items-center gap-2">
                        <Select
                          onValueChange={(v) => v && addMember(pool, Number(v))}
                        >
                          <SelectTrigger className="h-8 w-48 text-xs">
                            <SelectValue placeholder="+ Add member..." />
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
              </CardContent>
            </Card>
          );
        })}
      </div>

      <Dialog open={showForm} onOpenChange={setShowForm}>
        <DialogContent>
          <form onSubmit={handleSubmit}>
            <DialogHeader>
              <DialogTitle>
                {editId ? "Edit Pool" : "Add Pool"}
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
                Cancel
              </Button>
              <Button type="submit">{editId ? "Save" : "Create"}</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        title="Delete Pool"
        description={`Are you sure you want to delete "${deleteTarget?.label ?? ""}"? This will also remove all member associations.`}
        onConfirm={confirmDelete}
      />
    </div>
  );
}
