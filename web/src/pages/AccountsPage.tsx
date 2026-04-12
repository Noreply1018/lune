import { type FormEvent, useEffect, useState } from "react";
import StatusBadge from "@/components/StatusBadge";
import DataTable, { type Column } from "@/components/DataTable";
import ConfirmDialog from "@/components/ConfirmDialog";
import { api } from "@/lib/api";
import { toast } from "@/components/Feedback";
import type { Account } from "@/lib/types";
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
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Plus, MoreHorizontal } from "lucide-react";

interface AccountForm {
  label: string;
  base_url: string;
  api_key: string;
  model_allowlist: string;
  quota_total: number;
  quota_used: number;
  quota_unit: string;
  notes: string;
}

const emptyForm: AccountForm = {
  label: "",
  base_url: "",
  api_key: "",
  model_allowlist: "",
  quota_total: 0,
  quota_used: 0,
  quota_unit: "USD",
  notes: "",
};

export default function AccountsPage() {
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [editId, setEditId] = useState<number | null>(null);
  const [form, setForm] = useState<AccountForm>(emptyForm);
  const [deleteTarget, setDeleteTarget] = useState<Account | null>(null);

  function load() {
    setLoading(true);
    api
      .get<Account[]>("/accounts")
      .then(setAccounts)
      .catch(() => toast("Failed to load accounts", "error"))
      .finally(() => setLoading(false));
  }

  useEffect(load, []);

  function openCreate() {
    setEditId(null);
    setForm(emptyForm);
    setShowForm(true);
  }

  function openEdit(a: Account) {
    setEditId(a.id);
    setForm({
      label: a.label,
      base_url: a.base_url,
      api_key: "",
      model_allowlist: a.model_allowlist?.join(", ") ?? "",
      quota_total: a.quota_total,
      quota_used: a.quota_used,
      quota_unit: a.quota_unit || "USD",
      notes: a.notes,
    });
    setShowForm(true);
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    const body: Record<string, unknown> = {
      label: form.label,
      base_url: form.base_url,
      model_allowlist: form.model_allowlist
        ? form.model_allowlist.split(",").map((s) => s.trim()).filter(Boolean)
        : [],
      quota_total: form.quota_total,
      quota_used: form.quota_used,
      quota_unit: form.quota_unit,
      notes: form.notes,
    };
    if (form.api_key) {
      body.api_key = form.api_key;
    }

    try {
      if (editId) {
        await api.put(`/accounts/${editId}`, body);
        toast("Account updated");
      } else {
        body.api_key = form.api_key;
        await api.post("/accounts", body);
        toast("Account created");
      }
      setShowForm(false);
      load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "Operation failed", "error");
    }
  }

  async function toggleAccount(a: Account) {
    const next = !a.enabled;
    setAccounts((prev) =>
      prev.map((x) => (x.id === a.id ? { ...x, enabled: next } : x)),
    );
    try {
      await api.post(`/accounts/${a.id}/${next ? "enable" : "disable"}`);
    } catch {
      setAccounts((prev) =>
        prev.map((x) => (x.id === a.id ? { ...x, enabled: !next } : x)),
      );
      toast("Failed to update account", "error");
    }
  }

  async function confirmDelete() {
    if (!deleteTarget) return;
    try {
      await api.delete(`/accounts/${deleteTarget.id}`);
      toast("Account deleted");
      load();
    } catch {
      toast("Failed to delete account", "error");
    } finally {
      setDeleteTarget(null);
    }
  }

  const columns: Column<Account>[] = [
    {
      key: "label",
      header: "Label",
      render: (r) => <span className="font-medium text-moon-800">{r.label}</span>,
    },
    {
      key: "base_url",
      header: "Base URL",
      render: (r) => (
        <code className="text-xs text-moon-500">{r.base_url}</code>
      ),
    },
    {
      key: "status",
      header: "Status",
      render: (r) => (
        <StatusBadge status={r.enabled ? r.status : "disabled"} />
      ),
    },
    {
      key: "quota",
      header: "Quota",
      render: (r) =>
        r.quota_total > 0 ? (
          <span className="text-sm text-moon-500">
            {r.quota_unit === "USD" ? "$" : r.quota_unit === "CNY" ? "\u00a5" : ""}
            {r.quota_used} / {r.quota_total}
          </span>
        ) : (
          <span className="text-sm text-moon-400">-</span>
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
            <DropdownMenuItem onClick={() => toggleAccount(r)}>
              {r.enabled ? "Disable" : "Enable"}
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
        <h2 className="text-xl font-semibold">Accounts</h2>
        <Button size="sm" onClick={openCreate}>
          <Plus className="size-4" />
          Add Account
        </Button>
      </div>

      {loading ? (
        <Skeleton className="h-48" />
      ) : (
        <Card className="ring-1 ring-moon-200/60">
          <CardContent className="p-1">
            <DataTable
              columns={columns}
              rows={accounts}
              rowKey={(r) => r.id}
              empty="No accounts configured"
            />
          </CardContent>
        </Card>
      )}

      <Dialog open={showForm} onOpenChange={setShowForm}>
        <DialogContent className="max-w-lg">
          <form onSubmit={handleSubmit}>
            <DialogHeader>
              <DialogTitle>
                {editId ? "Edit Account" : "Add Account"}
              </DialogTitle>
            </DialogHeader>

            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="acc-label">Label</Label>
                <Input
                  id="acc-label"
                  value={form.label}
                  onChange={(e) => setForm({ ...form, label: e.target.value })}
                  required
                  placeholder="OpenAI Main"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="acc-url">Base URL</Label>
                <Input
                  id="acc-url"
                  value={form.base_url}
                  onChange={(e) =>
                    setForm({ ...form, base_url: e.target.value })
                  }
                  required
                  placeholder="https://api.openai.com/v1"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="acc-key">API Key</Label>
                <Input
                  id="acc-key"
                  type="password"
                  value={form.api_key}
                  onChange={(e) =>
                    setForm({ ...form, api_key: e.target.value })
                  }
                  placeholder={
                    editId ? "Leave empty to keep current" : "sk-..."
                  }
                  required={!editId}
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="acc-models">Model Allowlist</Label>
                <Input
                  id="acc-models"
                  value={form.model_allowlist}
                  onChange={(e) =>
                    setForm({ ...form, model_allowlist: e.target.value })
                  }
                  placeholder="gpt-4o, gpt-4o-mini (comma-separated, empty = all)"
                />
              </div>

              <div className="grid grid-cols-3 gap-3">
                <div className="space-y-2">
                  <Label htmlFor="acc-quota-total">Total Quota</Label>
                  <Input
                    id="acc-quota-total"
                    type="number"
                    value={form.quota_total}
                    onChange={(e) =>
                      setForm({
                        ...form,
                        quota_total: Number(e.target.value),
                      })
                    }
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="acc-quota-used">Used</Label>
                  <Input
                    id="acc-quota-used"
                    type="number"
                    value={form.quota_used}
                    onChange={(e) =>
                      setForm({
                        ...form,
                        quota_used: Number(e.target.value),
                      })
                    }
                  />
                </div>
                <div className="space-y-2">
                  <Label>Unit</Label>
                  <Select
                    value={form.quota_unit}
                    onValueChange={(v) => v && setForm({ ...form, quota_unit: v })}
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="USD">USD</SelectItem>
                      <SelectItem value="CNY">CNY</SelectItem>
                      <SelectItem value="tokens">tokens</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>

              <div className="space-y-2">
                <Label htmlFor="acc-notes">Notes</Label>
                <textarea
                  id="acc-notes"
                  className="flex min-h-[60px] w-full rounded-md border border-input bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  value={form.notes}
                  onChange={(e) => setForm({ ...form, notes: e.target.value })}
                  placeholder="Personal account, $20/month plan"
                />
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
        title="Delete Account"
        description={`Are you sure you want to delete "${deleteTarget?.label ?? ""}"? This action cannot be undone.`}
        onConfirm={confirmDelete}
      />
    </div>
  );
}
