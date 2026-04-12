import { type FormEvent, useEffect, useState } from "react";
import DataTable, { type Column } from "@/components/DataTable";
import CopyButton from "@/components/CopyButton";
import UsageBar from "@/components/UsageBar";
import ConfirmDialog from "@/components/ConfirmDialog";
import { api } from "@/lib/api";
import { toast } from "@/components/Feedback";
import { relativeTime } from "@/lib/fmt";
import type { AccessToken, AccessTokenCreated } from "@/lib/types";
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
import { Plus, MoreHorizontal, Check } from "lucide-react";

interface TokenForm {
  name: string;
  token: string;
  quota_tokens: number;
}

const emptyForm: TokenForm = { name: "", token: "", quota_tokens: 0 };

export default function TokensPage() {
  const [tokens, setTokens] = useState<AccessToken[]>([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [editId, setEditId] = useState<number | null>(null);
  const [form, setForm] = useState<TokenForm>(emptyForm);
  const [deleteTarget, setDeleteTarget] = useState<AccessToken | null>(null);
  const [created, setCreated] = useState<AccessTokenCreated | null>(null);

  function load() {
    setLoading(true);
    api
      .get<AccessToken[]>("/tokens")
      .then(setTokens)
      .catch(() => toast("Failed to load tokens", "error"))
      .finally(() => setLoading(false));
  }

  useEffect(load, []);

  function openCreate() {
    setEditId(null);
    setForm(emptyForm);
    setShowForm(true);
  }

  function openEdit(t: AccessToken) {
    setEditId(t.id);
    setForm({
      name: t.name,
      token: "",
      quota_tokens: t.quota_tokens,
    });
    setShowForm(true);
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    try {
      if (editId) {
        await api.put(`/tokens/${editId}`, {
          name: form.name,
          quota_tokens: form.quota_tokens,
        });
        toast("Token updated");
        setShowForm(false);
        load();
      } else {
        const body: Record<string, unknown> = {
          name: form.name,
          quota_tokens: form.quota_tokens,
        };
        if (form.token) body.token = form.token;
        const result = await api.post<AccessTokenCreated>("/tokens", body);
        setShowForm(false);
        setCreated(result);
        load();
      }
    } catch (err) {
      toast(err instanceof Error ? err.message : "Operation failed", "error");
    }
  }

  async function toggleToken(t: AccessToken) {
    const next = !t.enabled;
    setTokens((prev) =>
      prev.map((x) => (x.id === t.id ? { ...x, enabled: next } : x)),
    );
    try {
      await api.post(`/tokens/${t.id}/${next ? "enable" : "disable"}`);
    } catch {
      setTokens((prev) =>
        prev.map((x) => (x.id === t.id ? { ...x, enabled: !next } : x)),
      );
      toast("Failed to update token", "error");
    }
  }

  async function confirmDelete() {
    if (!deleteTarget) return;
    try {
      await api.delete(`/tokens/${deleteTarget.id}`);
      toast("Token deleted");
      load();
    } catch {
      toast("Failed to delete token", "error");
    } finally {
      setDeleteTarget(null);
    }
  }

  const envVars = created
    ? `export OPENAI_BASE_URL=http://127.0.0.1:7788/v1\nexport OPENAI_API_KEY=${created.token}`
    : "";

  const columns: Column<AccessToken>[] = [
    {
      key: "name",
      header: "Name",
      render: (r) => (
        <span className="font-medium text-moon-800">{r.name}</span>
      ),
    },
    {
      key: "token",
      header: "Token",
      render: (r) => (
        <div className="flex items-center gap-1">
          <code className="text-xs text-moon-500">{r.token_masked}</code>
          <CopyButton value={r.token_masked} />
        </div>
      ),
    },
    {
      key: "usage",
      header: "Usage",
      className: "min-w-[160px]",
      render: (r) => (
        <UsageBar used={r.used_tokens} total={r.quota_tokens} />
      ),
    },
    {
      key: "last_used",
      header: "Last Used",
      render: (r) => (
        <span className="text-moon-500">
          {relativeTime(r.last_used_at)}
        </span>
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
            <DropdownMenuItem onClick={() => toggleToken(r)}>
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
        <h2 className="text-xl font-semibold">Tokens</h2>
        <Button size="sm" onClick={openCreate}>
          <Plus className="size-4" />
          Create Token
        </Button>
      </div>

      {loading ? (
        <Skeleton className="h-48" />
      ) : (
        <Card className="ring-1 ring-moon-200/60">
          <CardContent className="p-1">
            <DataTable
              columns={columns}
              rows={tokens}
              rowKey={(r) => r.id}
              empty="No tokens created"
            />
          </CardContent>
        </Card>
      )}

      {/* Create / Edit dialog */}
      <Dialog open={showForm} onOpenChange={setShowForm}>
        <DialogContent>
          <form onSubmit={handleSubmit}>
            <DialogHeader>
              <DialogTitle>
                {editId ? "Edit Token" : "Create Access Token"}
              </DialogTitle>
            </DialogHeader>

            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="token-name">Name</Label>
                <Input
                  id="token-name"
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  required
                  placeholder="my-project"
                />
              </div>

              {!editId && (
                <div className="space-y-2">
                  <Label htmlFor="token-value">
                    Token{" "}
                    <span className="font-normal text-moon-400">
                      (auto-generated if empty)
                    </span>
                  </Label>
                  <Input
                    id="token-value"
                    value={form.token}
                    onChange={(e) =>
                      setForm({ ...form, token: e.target.value })
                    }
                    placeholder="sk-lune-..."
                  />
                </div>
              )}

              <div className="space-y-2">
                <Label htmlFor="token-quota">Token Quota</Label>
                <Input
                  id="token-quota"
                  type="number"
                  value={form.quota_tokens}
                  onChange={(e) =>
                    setForm({
                      ...form,
                      quota_tokens: Number(e.target.value),
                    })
                  }
                />
                <p className="text-xs text-moon-400">
                  0 = unlimited
                </p>
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

      {/* Token created success dialog */}
      <Dialog open={created !== null} onOpenChange={(o) => !o && setCreated(null)}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Check className="size-5 text-status-green" />
              Token Created
            </DialogTitle>
          </DialogHeader>

          <div className="space-y-4 py-2">
            <p className="text-sm text-moon-500">
              Make sure to copy your token now. You won't be able to see it
              again.
            </p>

            <div className="flex items-center gap-2 rounded-lg bg-moon-100 px-4 py-3">
              <code className="flex-1 break-all text-sm font-medium text-moon-800">
                {created?.token}
              </code>
              <CopyButton value={created?.token ?? ""} label="Copy" />
            </div>

            <div className="space-y-2">
              <p className="text-xs font-medium uppercase tracking-wider text-moon-400">
                Quick Setup
              </p>
              <div className="rounded-lg bg-moon-800 p-4">
                <pre className="whitespace-pre-wrap text-xs text-moon-100">
                  {envVars}
                </pre>
              </div>
              <CopyButton value={envVars} label="Copy Env Vars" />
            </div>
          </div>

          <DialogFooter>
            <Button onClick={() => setCreated(null)}>Done</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        title="Delete Token"
        description={`Are you sure you want to delete "${deleteTarget?.name ?? ""}"? This token will be immediately invalidated.`}
        onConfirm={confirmDelete}
      />
    </div>
  );
}
