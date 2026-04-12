import { type FormEvent, useEffect, useState } from "react";
import DataTable, { type Column } from "@/components/DataTable";
import ConfirmDialog from "@/components/ConfirmDialog";
import { api } from "@/lib/api";
import { toast } from "@/components/Feedback";
import type { ModelRoute, Pool, SystemSettings } from "@/lib/types";
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
    Promise.all([
      api.get<ModelRoute[]>("/routes"),
      api.get<Pool[]>("/pools"),
      api.get<SystemSettings>("/settings"),
    ])
      .then(([r, p, s]) => {
        setRoutes(r ?? []);
        setPools(p ?? []);
        setDefaultPoolId(s?.default_pool_id ?? null);
      })
      .catch(() => toast("Failed to load routes", "error"))
      .finally(() => setLoading(false));
  }

  useEffect(load, []);

  async function updateDefaultPool(poolId: string | null) {
    if (poolId === null) return;
    const value = poolId === "none" ? null : Number(poolId);
    setDefaultPoolId(value);
    try {
      await api.put("/settings", { default_pool_id: value });
    } catch {
      toast("Failed to update default pool", "error");
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
        toast("Route updated");
      } else {
        await api.post("/routes", form);
        toast("Route created");
      }
      setShowForm(false);
      load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "Operation failed", "error");
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
      toast("Failed to update route", "error");
    }
  }

  async function confirmDelete() {
    if (!deleteTarget) return;
    try {
      await api.delete(`/routes/${deleteTarget.id}`);
      toast("Route deleted");
      load();
    } catch {
      toast("Failed to delete route", "error");
    } finally {
      setDeleteTarget(null);
    }
  }

  const enabledPools = pools.filter((p) => p.enabled);

  const columns: Column<ModelRoute>[] = [
    {
      key: "alias",
      header: "Alias",
      render: (r) => (
        <span className="font-medium text-moon-800">{r.alias}</span>
      ),
    },
    {
      key: "target_model",
      header: "Target Model",
      render: (r) => (
        <code className="text-xs text-moon-500">{r.target_model}</code>
      ),
    },
    {
      key: "pool",
      header: "Pool",
      render: (r) => (
        <span className="text-moon-500">{r.pool_label || "-"}</span>
      ),
    },
    {
      key: "enabled",
      header: "Enabled",
      render: (r) => (
        <Switch
          checked={r.enabled}
          onCheckedChange={() => toggleRoute(r)}
        />
      ),
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
              Edit
            </DropdownMenuItem>
            <DropdownMenuItem
              className="text-destructive"
              onClick={() => setDeleteTarget(r)}
            >
              Delete
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      ),
    },
  ];

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">Routes</h2>
        <Button size="sm" onClick={openCreate}>
          <Plus className="size-4" />
          Add Route
        </Button>
      </div>

      {loading ? (
        <div className="space-y-4">
          <Skeleton className="h-16" />
          <Skeleton className="h-48" />
        </div>
      ) : (
        <>
          <Card className="ring-1 ring-moon-200/60">
            <CardContent className="flex items-center gap-4 px-5 py-4">
              <Label className="shrink-0 text-sm font-medium text-moon-600">
                Default Pool
              </Label>
              <Select
                value={
                  defaultPoolId !== null ? String(defaultPoolId) : "none"
                }
                onValueChange={updateDefaultPool}
              >
                <SelectTrigger className="w-56">
                  <SelectValue placeholder="None" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="none">None</SelectItem>
                  {enabledPools.map((p) => (
                    <SelectItem key={p.id} value={String(p.id)}>
                      {p.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <span className="text-xs text-moon-400">
                Catch-all for unmatched models
              </span>
            </CardContent>
          </Card>

          <Card className="ring-1 ring-moon-200/60">
            <CardContent className="p-1">
              <DataTable
                columns={columns}
                rows={routes}
                rowKey={(r) => r.id}
                empty="No routes configured"
              />
            </CardContent>
          </Card>

          <div className="flex items-start gap-2 text-xs text-moon-400">
            <Info className="mt-0.5 size-3.5 shrink-0" />
            <span>
              Models not listed above will route through the default pool
              with the original model name.
            </span>
          </div>
        </>
      )}

      <Dialog open={showForm} onOpenChange={setShowForm}>
        <DialogContent>
          <form onSubmit={handleSubmit}>
            <DialogHeader>
              <DialogTitle>
                {editId ? "Edit Route" : "Add Route"}
              </DialogTitle>
            </DialogHeader>

            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="route-alias">Alias</Label>
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
                <Label htmlFor="route-target">Target Model</Label>
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
                      Alias and target are the same — model name is passed
                      through unchanged.
                    </p>
                  )}
              </div>

              <div className="space-y-2">
                <Label>Pool</Label>
                <Select
                  value={form.pool_id !== null ? String(form.pool_id) : ""}
                  onValueChange={(v) =>
                    v && setForm({ ...form, pool_id: Number(v) })
                  }
                >
                  <SelectTrigger>
                    <SelectValue placeholder="Select a pool" />
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
                <Label>Enabled</Label>
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
        title="Delete Route"
        description={`Are you sure you want to delete the route for "${deleteTarget?.alias ?? ""}"?`}
        onConfirm={confirmDelete}
      />
    </div>
  );
}
