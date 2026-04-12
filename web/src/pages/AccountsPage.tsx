import { FormEvent, useEffect, useState } from "react";
import StatusBadge from "../components/StatusBadge";
import DataTable, { type Column } from "../components/DataTable";
import { luneGet, lunePost, lunePut } from "../lib/api";
import { toast } from "../components/Feedback";
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
import { Plus, RefreshCw } from "lucide-react";

type Account = {
  id: string;
  label: string;
  credential: string;
  enabled: boolean;
  status: string;
};

type AccountForm = {
  id: string;
  label: string;
  credential: string;
  enabled: boolean;
};

const emptyForm: AccountForm = {
  id: "",
  label: "",
  credential: "",
  enabled: true,
};

export default function AccountsPage() {
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [editId, setEditId] = useState<string | null>(null);
  const [form, setForm] = useState<AccountForm>(emptyForm);

  function load() {
    setLoading(true);
    luneGet<{ accounts: Account[] }>("/admin/api/accounts")
      .then((d) => setAccounts(d.accounts ?? []))
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
      id: a.id,
      label: a.label,
      credential: "",
      enabled: a.enabled,
    });
    setShowForm(true);
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    try {
      if (editId) {
        await lunePut(`/admin/api/accounts/${editId}`, form);
        toast("Account updated");
      } else {
        await lunePost("/admin/api/accounts", form);
        toast("Account created");
      }
      setShowForm(false);
      load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "Operation failed", "error");
    }
  }

  async function toggleAccount(a: Account) {
    try {
      const action = a.enabled ? "disable" : "enable";
      await lunePost(`/admin/api/accounts/${a.id}/${action}`);
      toast(a.enabled ? "Disabled" : "Enabled");
      load();
    } catch {
      toast("Operation failed", "error");
    }
  }

  const columns: Column<Account>[] = [
    {
      key: "label",
      header: "Label",
      render: (r) => <span className="font-medium">{r.label}</span>,
    },
    {
      key: "id",
      header: "ID",
      render: (r) => (
        <code className="text-xs text-muted-foreground">{r.id}</code>
      ),
    },
    {
      key: "credential",
      header: "Credential",
      render: (r) => (
        <code className="text-xs text-muted-foreground">
          {r.credential || "-"}
        </code>
      ),
    },
    {
      key: "status",
      header: "Status",
      render: (r) => (
        <StatusBadge
          status={
            !r.enabled
              ? "disabled"
              : r.status === "healthy" || r.status === "active" || r.status === "ready"
                ? "ok"
                : "error"
          }
          label={!r.enabled ? "Disabled" : r.status}
        />
      ),
    },
    {
      key: "actions",
      header: "",
      render: (r) => (
        <div className="flex gap-1">
          <Button variant="ghost" size="sm" onClick={() => openEdit(r)}>
            Edit
          </Button>
          <Button variant="ghost" size="sm" onClick={() => toggleAccount(r)}>
            {r.enabled ? "Disable" : "Enable"}
          </Button>
        </div>
      ),
    },
  ];

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">Accounts</h2>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" onClick={load}>
            <RefreshCw className="size-4" />
            Refresh
          </Button>
          <Button size="sm" onClick={openCreate}>
            <Plus className="size-4" />
            New Account
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
              rows={accounts}
              rowKey={(r) => r.id}
              empty="No accounts"
            />
          </CardContent>
        </Card>
      )}

      <Dialog open={showForm} onOpenChange={setShowForm}>
        <DialogContent>
          <form onSubmit={handleSubmit}>
            <DialogHeader>
              <DialogTitle>
                {editId ? "Edit Account" : "New Account"}
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
                  placeholder="My API Key"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="acc-credential">API Key</Label>
                <Input
                  id="acc-credential"
                  type="password"
                  value={form.credential}
                  onChange={(e) =>
                    setForm({ ...form, credential: e.target.value })
                  }
                  placeholder={editId ? "Leave empty to keep current" : "sk-..."}
                />
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
    </div>
  );
}
