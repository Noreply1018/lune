import { type FormEvent, useEffect, useState } from "react";
import StatusBadge from "@/components/StatusBadge";
import DataTable, { type Column } from "@/components/DataTable";
import ConfirmDialog from "@/components/ConfirmDialog";
import PageHeader from "@/components/PageHeader";
import SectionHeading from "@/components/SectionHeading";
import { api } from "@/lib/api";
import { toast } from "@/components/Feedback";
import type { Account, CpaService } from "@/lib/types";
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
import { Plus, MoreHorizontal, Globe, Server } from "lucide-react";


const CPA_PROVIDERS = [
  { value: "codex", label: "Codex (ChatGPT Plus/Pro)" },
  { value: "claude", label: "Claude" },
  { value: "gemini", label: "Gemini" },
  { value: "gemini-cli", label: "Gemini CLI" },
  { value: "vertex", label: "Vertex AI" },
  { value: "aistudio", label: "AI Studio" },
  { value: "openai", label: "OpenAI" },
  { value: "qwen", label: "Qwen" },
  { value: "kimi", label: "Kimi" },
  { value: "iflow", label: "iFlow (GLM)" },
  { value: "antigravity", label: "Antigravity" },
];

interface OpenAIForm {
  label: string;
  provider: string;
  base_url: string;
  api_key: string;
  model_allowlist: string;
  quota_total: number;
  quota_used: number;
  quota_unit: string;
  notes: string;
}

interface CpaForm {
  label: string;
  cpa_provider: string;
  model_allowlist: string;
  notes: string;
}

const emptyOpenAIForm: OpenAIForm = {
  label: "",
  provider: "",
  base_url: "",
  api_key: "",
  model_allowlist: "",
  quota_total: 0,
  quota_used: 0,
  quota_unit: "USD",
  notes: "",
};

const emptyCpaForm: CpaForm = {
  label: "",
  cpa_provider: "",
  model_allowlist: "",
  notes: "",
};

export default function AccountsPage() {
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [cpaService, setCpaService] = useState<CpaService | null>(null);
  const [loading, setLoading] = useState(true);

  // source selection
  const [showSourcePicker, setShowSourcePicker] = useState(false);

  // forms
  const [showForm, setShowForm] = useState(false);
  const [editId, setEditId] = useState<number | null>(null);
  const [editSourceKind, setEditSourceKind] = useState<"openai_compat" | "cpa">("openai_compat");
  const [openaiForm, setOpenaiForm] = useState<OpenAIForm>(emptyOpenAIForm);
  const [cpaForm, setCpaForm] = useState<CpaForm>(emptyCpaForm);
  const [deleteTarget, setDeleteTarget] = useState<Account | null>(null);

  function load() {
    setLoading(true);
    Promise.all([
      api.get<Account[]>("/accounts"),
      api.get<CpaService | null>("/cpa/service"),
    ])
      .then(([accs, svc]) => {
        setAccounts(accs ?? []);
        setCpaService(svc ?? null);
      })
      .catch(() => toast("Failed to load accounts", "error"))
      .finally(() => setLoading(false));
  }

  useEffect(load, []);

  function openCreate() {
    setEditId(null);
    setShowSourcePicker(true);
  }

  function pickSource(kind: "openai_compat" | "cpa") {
    setEditSourceKind(kind);
    setShowSourcePicker(false);

    if (kind === "cpa" && !cpaService) {
      toast("Please configure a CPA Service first", "error");
      return;
    }

    if (kind === "openai_compat") {
      setOpenaiForm(emptyOpenAIForm);
    } else {
      setCpaForm(emptyCpaForm);
    }
    setShowForm(true);
  }

  function openEdit(a: Account) {
    setEditId(a.id);
    setEditSourceKind(a.source_kind);

    if (a.source_kind === "cpa") {
      setCpaForm({
        label: a.label,
        cpa_provider: a.cpa_provider,
        model_allowlist: a.model_allowlist?.join(", ") ?? "",
        notes: a.notes,
      });
    } else {
      setOpenaiForm({
        label: a.label,
        provider: a.provider ?? "",
        base_url: a.base_url,
        api_key: "",
        model_allowlist: a.model_allowlist?.join(", ") ?? "",
        quota_total: a.quota_total,
        quota_used: a.quota_used,
        quota_unit: a.quota_unit || "USD",
        notes: a.notes,
      });
    }
    setShowForm(true);
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();

    try {
      if (editSourceKind === "cpa") {
        const body: Record<string, unknown> = {
          source_kind: "cpa",
          label: cpaForm.label,
          cpa_service_id: cpaService?.id,
          cpa_provider: cpaForm.cpa_provider,
          model_allowlist: cpaForm.model_allowlist
            ? cpaForm.model_allowlist.split(",").map((s) => s.trim()).filter(Boolean)
            : [],
          notes: cpaForm.notes,
        };
        if (editId) {
          await api.put(`/accounts/${editId}`, body);
          toast("Account updated");
        } else {
          await api.post("/accounts", body);
          toast("Account created");
        }
      } else {
        const body: Record<string, unknown> = {
          source_kind: "openai_compat",
          label: openaiForm.label,
          provider: openaiForm.provider,
          base_url: openaiForm.base_url,
          model_allowlist: openaiForm.model_allowlist
            ? openaiForm.model_allowlist.split(",").map((s) => s.trim()).filter(Boolean)
            : [],
          quota_total: openaiForm.quota_total,
          quota_used: openaiForm.quota_used,
          quota_unit: openaiForm.quota_unit,
          notes: openaiForm.notes,
        };
        if (openaiForm.api_key) {
          body.api_key = openaiForm.api_key;
        }
        if (editId) {
          await api.put(`/accounts/${editId}`, body);
          toast("Account updated");
        } else {
          body.api_key = openaiForm.api_key;
          await api.post("/accounts", body);
          toast("Account created");
        }
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
      tone: "primary",
    },
    {
      key: "source",
      header: "Source",
      render: (r) =>
        r.source_kind === "cpa" ? (
          <span className="rounded-md bg-lunar-100/60 px-2 py-0.5 text-xs font-medium text-lunar-700">
            CPA - {r.cpa_provider}
          </span>
        ) : (
          <span className="rounded-md bg-moon-100/60 px-2 py-0.5 text-xs font-medium text-moon-600">
            OpenAI compat
          </span>
        ),
    },
    {
      key: "endpoint",
      header: "Endpoint",
      render: (r) => (
        <code className="text-xs text-moon-500">
          {r.runtime?.base_url ?? r.base_url}
        </code>
      ),
      tone: "secondary",
    },
    {
      key: "status",
      header: "Status",
      render: (r) => (
        <StatusBadge status={r.enabled ? r.status : "disabled"} />
      ),
      tone: "status",
    },
    {
      key: "quota",
      header: "Quota",
      render: (r) =>
        r.source_kind === "cpa" ? (
          <span className="text-sm text-moon-400">-</span>
        ) : r.quota_total > 0 ? (
          <span className="text-sm text-moon-500">
            {r.quota_unit === "USD" ? "$" : r.quota_unit === "CNY" ? "\u00a5" : ""}
            {r.quota_used} / {r.quota_total}
          </span>
        ) : (
          <span className="text-sm text-moon-400">-</span>
        ),
      align: "right",
      tone: "numeric",
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
    <div className="space-y-8">
      <PageHeader
        eyebrow="Workspace"
        title="Accounts"
        description="Manage upstream provider accounts, their current health, and spending constraints."
        meta={
          <span>
            {accounts.length} configured account{accounts.length === 1 ? "" : "s"} •{" "}
            {accounts.filter((account) => account.enabled).length} enabled
          </span>
        }
        actions={
          <Button size="sm" onClick={openCreate}>
            <Plus className="size-4" />
            Add Account
          </Button>
        }
      />

      <section className="space-y-4">
        <SectionHeading
          title="Account Registry"
          description="Each row represents one upstream credential set and its operational quota."
        />

        {loading ? (
          <Skeleton className="h-64 rounded-[1.5rem]" />
        ) : (
          <div className="overflow-hidden rounded-[1.6rem] border border-moon-200/70 bg-white/85">
            <DataTable
              columns={columns}
              rows={accounts}
              rowKey={(r) => r.id}
              empty="No accounts configured"
            />
          </div>
        )}
      </section>

      {/* Source picker dialog */}
      <Dialog open={showSourcePicker} onOpenChange={setShowSourcePicker}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>Choose Source</DialogTitle>
          </DialogHeader>
          <div className="grid grid-cols-2 gap-3 py-4">
            <button
              type="button"
              onClick={() => pickSource("openai_compat")}
              className="flex flex-col items-center gap-3 rounded-xl border border-moon-200 p-5 text-center transition hover:border-lunar-400 hover:bg-lunar-50/30"
            >
              <Globe className="size-8 text-moon-400" />
              <div>
                <p className="font-medium text-moon-800">OpenAI-Compatible</p>
                <p className="mt-1 text-xs text-moon-500">
                  Direct connection to any OpenAI-compatible API.
                </p>
              </div>
            </button>
            <button
              type="button"
              onClick={() => pickSource("cpa")}
              className="flex flex-col items-center gap-3 rounded-xl border border-moon-200 p-5 text-center transition hover:border-lunar-400 hover:bg-lunar-50/30"
            >
              <Server className="size-8 text-moon-400" />
              <div>
                <p className="font-medium text-moon-800">CPA Provider Channel</p>
                <p className="mt-1 text-xs text-moon-500">
                  Route through a CPA service provider.
                </p>
              </div>
            </button>
          </div>
        </DialogContent>
      </Dialog>

      {/* Account form dialog */}
      <Dialog open={showForm} onOpenChange={setShowForm}>
        <DialogContent className="max-w-lg">
          <form onSubmit={handleSubmit}>
            <DialogHeader>
              <DialogTitle>
                {editId
                  ? "Edit Account"
                  : editSourceKind === "cpa"
                    ? "Add CPA Provider Channel"
                    : "Add Account"}
              </DialogTitle>
            </DialogHeader>

            <div className="space-y-4 py-4">
              {editSourceKind === "cpa" ? (
                /* CPA form */
                <>
                  <div className="space-y-2">
                    <Label>CPA Service</Label>
                    <Input
                      value={cpaService?.label ?? "Not configured"}
                      disabled
                      className="bg-moon-50"
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="cpa-provider">Provider</Label>
                    {editId ? (
                      <Input value={cpaForm.cpa_provider} disabled className="bg-moon-50" />
                    ) : (
                      <Select
                        value={cpaForm.cpa_provider}
                        onValueChange={(v) => {
                          if (!v) return;
                          setCpaForm({
                            ...cpaForm,
                            cpa_provider: v,
                            label: cpaForm.label || `CPA - ${CPA_PROVIDERS.find((p) => p.value === v)?.label?.split(" ")[0] ?? v}`,
                          });
                        }}
                      >
                        <SelectTrigger>
                          <SelectValue placeholder="Select provider..." />
                        </SelectTrigger>
                        <SelectContent>
                          {CPA_PROVIDERS.map((p) => (
                            <SelectItem key={p.value} value={p.value}>
                              {p.label}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    )}
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="cpa-label">Label</Label>
                    <Input
                      id="cpa-label"
                      value={cpaForm.label}
                      onChange={(e) =>
                        setCpaForm({ ...cpaForm, label: e.target.value })
                      }
                      required
                      placeholder="CPA - Codex"
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="cpa-models">Model Allowlist</Label>
                    <Input
                      id="cpa-models"
                      value={cpaForm.model_allowlist}
                      onChange={(e) =>
                        setCpaForm({
                          ...cpaForm,
                          model_allowlist: e.target.value,
                        })
                      }
                      placeholder="gpt-4o, gpt-4.1 (comma-separated, empty = all)"
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="cpa-notes">Notes</Label>
                    <textarea
                      id="cpa-notes"
                      className="flex min-h-[60px] w-full rounded-md border border-input bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                      value={cpaForm.notes}
                      onChange={(e) =>
                        setCpaForm({ ...cpaForm, notes: e.target.value })
                      }
                      placeholder="Codex models via local CPA"
                    />
                  </div>
                </>
              ) : (
                /* OpenAI-Compatible form */
                <>
                  <div className="space-y-2">
                    <Label htmlFor="acc-label">Label</Label>
                    <Input
                      id="acc-label"
                      value={openaiForm.label}
                      onChange={(e) =>
                        setOpenaiForm({ ...openaiForm, label: e.target.value })
                      }
                      required
                      placeholder="OpenAI Main"
                    />
                  </div>

                  <div className="space-y-2">
                    <Label htmlFor="acc-url">Base URL</Label>
                    <Input
                      id="acc-url"
                      value={openaiForm.base_url}
                      onChange={(e) =>
                        setOpenaiForm({
                          ...openaiForm,
                          base_url: e.target.value,
                        })
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
                      value={openaiForm.api_key}
                      onChange={(e) =>
                        setOpenaiForm({
                          ...openaiForm,
                          api_key: e.target.value,
                        })
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
                      value={openaiForm.model_allowlist}
                      onChange={(e) =>
                        setOpenaiForm({
                          ...openaiForm,
                          model_allowlist: e.target.value,
                        })
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
                        value={openaiForm.quota_total}
                        onChange={(e) =>
                          setOpenaiForm({
                            ...openaiForm,
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
                        value={openaiForm.quota_used}
                        onChange={(e) =>
                          setOpenaiForm({
                            ...openaiForm,
                            quota_used: Number(e.target.value),
                          })
                        }
                      />
                    </div>
                    <div className="space-y-2">
                      <Label>Unit</Label>
                      <Select
                        value={openaiForm.quota_unit}
                        onValueChange={(v) =>
                          v && setOpenaiForm({ ...openaiForm, quota_unit: v })
                        }
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
                      value={openaiForm.notes}
                      onChange={(e) =>
                        setOpenaiForm({ ...openaiForm, notes: e.target.value })
                      }
                      placeholder="Personal account, $20/month plan"
                    />
                  </div>
                </>
              )}
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
