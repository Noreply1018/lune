import { type FormEvent, useEffect, useRef, useState } from "react";
import StatusBadge from "@/components/StatusBadge";
import DataTable, { type Column } from "@/components/DataTable";
import ConfirmDialog from "@/components/ConfirmDialog";
import PageHeader from "@/components/PageHeader";
import SectionHeading from "@/components/SectionHeading";
import { api } from "@/lib/api";
import { toast } from "@/components/Feedback";
import type { Account, CpaService, LatencyBucket, LoginSession, RemoteAccount } from "@/lib/types";
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
import { Plus, MoreHorizontal, Globe, Server, KeyRound, Download, Copy, ExternalLink, Loader2, AlertTriangle, Clock, Zap } from "lucide-react";
import type { TestConnectionResult } from "@/lib/types";


const PROVIDER_TEMPLATES = [
  { value: "openai", label: "OpenAI", base_url: "https://api.openai.com/v1" },
  { value: "deepseek", label: "DeepSeek", base_url: "https://api.deepseek.com/v1" },
  { value: "groq", label: "Groq", base_url: "https://api.groq.com/openai/v1" },
  { value: "mistral", label: "Mistral", base_url: "https://api.mistral.ai/v1" },
  { value: "together", label: "Together AI", base_url: "https://api.together.xyz/v1" },
  { value: "openrouter", label: "OpenRouter", base_url: "https://openrouter.ai/api/v1" },
  { value: "moonshot", label: "Moonshot", base_url: "https://api.moonshot.cn/v1" },
  { value: "custom", label: "Custom", base_url: "" },
];

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

function expiryStatus(isoDate: string | null): "ok" | "soon" | "today" | "expired" | null {
  if (!isoDate) return null;
  const now = Date.now();
  const exp = new Date(isoDate).getTime();
  if (Number.isNaN(exp)) return null;
  const diff = exp - now;
  if (diff <= 0) return "expired";
  if (diff <= 24 * 60 * 60 * 1000) return "today";
  if (diff <= 7 * 24 * 60 * 60 * 1000) return "soon";
  return "ok";
}

function Sparkline({ data }: { data: LatencyBucket[] }) {
  if (data.length < 2) return <span className="text-xs text-moon-400">-</span>;
  const values = data.map((d) => d.p50);
  const max = Math.max(...values);
  const min = Math.min(...values);
  const range = max - min || 1;
  const w = 80;
  const h = 24;
  const points = values
    .map((v, i) => `${(i / (values.length - 1)) * w},${h - ((v - min) / range) * (h - 4) - 2}`)
    .join(" ");
  const avg = Math.round(values.reduce((a, b) => a + b, 0) / values.length);
  return (
    <span className="inline-flex items-center gap-1.5" title={`24h p50 avg: ${avg}ms`}>
      <svg width={w} height={h} className="shrink-0">
        <polyline
          points={points}
          fill="none"
          stroke="currentColor"
          strokeWidth="1.5"
          className="text-lunar-500"
        />
      </svg>
      <span className="text-[11px] text-moon-500">{avg}ms</span>
    </span>
  );
}

function ExpiryBadge({ date }: { date: string | null }) {
  const status = expiryStatus(date);
  if (!status || status === "ok") return null;
  const label = status === "expired" ? "Expired" : status === "today" ? "Expiring today" : "Expiring soon";
  const cls = status === "expired" || status === "today"
    ? "bg-red-100 text-red-700"
    : "bg-yellow-100 text-yellow-700";
  return (
    <span className={`ml-2 inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[10px] font-medium ${cls}`}>
      <AlertTriangle className="size-3" />
      {label}
    </span>
  );
}

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
  const [testing, setTesting] = useState(false);

  // device code login
  const [showDeviceCode, setShowDeviceCode] = useState(false);
  const [loginSession, setLoginSession] = useState<LoginSession | null>(null);
  const [loginLoading, setLoginLoading] = useState(false);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  // import
  const [showImport, setShowImport] = useState(false);
  const [remoteAccounts, setRemoteAccounts] = useState<RemoteAccount[]>([]);
  const [importLoading, setImportLoading] = useState(false);
  const [selectedKeys, setSelectedKeys] = useState<Set<string>>(new Set());
  const [importLabel, setImportLabel] = useState("");

  // sparkline data per account
  const [sparklines, setSparklines] = useState<Record<number, LatencyBucket[]>>({});

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

  useEffect(() => {
    if (accounts.length === 0) return;
    const ids = accounts.map((a) => a.id);
    Promise.all(
      ids.map((id) =>
        api
          .get<LatencyBucket[]>(`/usage/latency?period=24h&bucket=1h&account=${id}`)
          .then((data): [number, LatencyBucket[]] => [id, data ?? []])
          .catch((): [number, LatencyBucket[]] => [id, []]),
      ),
    ).then((results) => {
      const map: Record<number, LatencyBucket[]> = {};
      for (const [id, data] of results) map[id] = data;
      setSparklines(map);
    });
  }, [accounts]);

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

  async function handleTestConnection() {
    if (!openaiForm.base_url) return;
    setTesting(true);
    try {
      const result = await api.post<TestConnectionResult>("/accounts/test-connection", {
        base_url: openaiForm.base_url,
        api_key: openaiForm.api_key,
      });
      if (result.reachable) {
        toast(`Connected in ${result.latency_ms}ms — ${result.models?.length ?? 0} models available`);
      } else {
        toast(result.error || "Connection failed", "error");
      }
    } catch (err) {
      toast(err instanceof Error ? err.message : "Test failed", "error");
    } finally {
      setTesting(false);
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
          <span
            className="rounded-md bg-lunar-100/60 px-2 py-0.5 text-xs font-medium text-lunar-700"
            title="Provider channel — managed by CPA service, credentials auto-refreshed"
          >
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
        <span className="inline-flex items-center">
          <StatusBadge status={r.enabled ? r.status : "disabled"} />
          {r.source_kind === "cpa" && r.cpa_expired_at && (
            <ExpiryBadge date={r.cpa_expired_at} />
          )}
        </span>
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
      key: "latency",
      header: "Latency (24h)",
      render: (r) => <Sparkline data={sparklines[r.id] ?? []} />,
      tone: "secondary",
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
        <DialogContent className="max-w-lg">
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
            <button
              type="button"
              onClick={() => { setShowSourcePicker(false); startDeviceCodeLogin(); }}
              className="flex flex-col items-center gap-3 rounded-xl border border-moon-200 p-5 text-center transition hover:border-lunar-400 hover:bg-lunar-50/30"
            >
              <KeyRound className="size-8 text-moon-400" />
              <div>
                <p className="font-medium text-moon-800">Login with Device Code</p>
                <p className="mt-1 text-xs text-moon-500">
                  Authenticate via OpenAI OAuth device flow.
                </p>
              </div>
            </button>
            <button
              type="button"
              onClick={() => { setShowSourcePicker(false); openImportDialog(); }}
              className="flex flex-col items-center gap-3 rounded-xl border border-moon-200 p-5 text-center transition hover:border-lunar-400 hover:bg-lunar-50/30"
            >
              <Download className="size-8 text-moon-400" />
              <div>
                <p className="font-medium text-moon-800">Import from CPA</p>
                <p className="mt-1 text-xs text-moon-500">
                  Import existing accounts from cpa-auth directory.
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
                    <Label htmlFor="acc-provider">Provider</Label>
                    <Select
                      value={openaiForm.provider || "custom"}
                      onValueChange={(v) => {
                        if (!v) return;
                        const tpl = PROVIDER_TEMPLATES.find((t) => t.value === v);
                        setOpenaiForm({
                          ...openaiForm,
                          provider: v === "custom" ? "" : v,
                          base_url: tpl?.base_url ?? "",
                          label: v === "custom" ? "" : (!openaiForm.label || PROVIDER_TEMPLATES.some((t) => t.label === openaiForm.label) ? (tpl?.label ?? "") : openaiForm.label),
                        });
                      }}
                    >
                      <SelectTrigger>
                        <SelectValue placeholder="Select provider..." />
                      </SelectTrigger>
                      <SelectContent>
                        {PROVIDER_TEMPLATES.map((p) => (
                          <SelectItem key={p.value} value={p.value}>
                            {p.label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>

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

                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="w-full"
                    onClick={handleTestConnection}
                    disabled={testing || !openaiForm.base_url}
                  >
                    {testing ? <Loader2 className="size-4 animate-spin" /> : <Zap className="size-4" />}
                    Test Connection
                  </Button>

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

      {/* Device Code Login Dialog */}
      <Dialog open={showDeviceCode} onOpenChange={(open) => {
        if (!open) {
          if (pollRef.current) clearInterval(pollRef.current);
          pollRef.current = null;
        }
        setShowDeviceCode(open);
      }}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>Device Code Login</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-4">
            {loginLoading && !loginSession && (
              <div className="flex items-center justify-center py-8">
                <Loader2 className="size-6 animate-spin text-moon-400" />
              </div>
            )}
            {loginSession?.status === "pending" && (
              <>
                <div className="space-y-2">
                  <p className="text-sm text-moon-600">Open this URL:</p>
                  <div className="flex items-center gap-2">
                    <code className="flex-1 rounded-md bg-moon-50 px-3 py-2 text-sm">{loginSession.verification_uri}</code>
                    <Button size="icon" variant="outline" onClick={() => window.open(loginSession.verification_uri, "_blank")}>
                      <ExternalLink className="size-4" />
                    </Button>
                  </div>
                </div>
                <div className="space-y-2">
                  <p className="text-sm text-moon-600">Enter this code:</p>
                  <div className="flex items-center gap-2">
                    <code className="flex-1 rounded-lg bg-moon-50 px-4 py-3 text-center text-lg font-mono font-bold tracking-widest">{loginSession.user_code}</code>
                    <Button size="icon" variant="outline" onClick={() => {
                      navigator.clipboard.writeText(loginSession.user_code ?? "");
                      toast("Code copied");
                    }}>
                      <Copy className="size-4" />
                    </Button>
                  </div>
                </div>
                <div className="flex items-center gap-2 text-sm text-moon-500">
                  <Loader2 className="size-4 animate-spin" />
                  <span>Waiting for authorization...</span>
                  {loginSession.expires_at && (
                    <span className="ml-auto">
                      <CountdownTimer expiresAt={loginSession.expires_at} />
                    </span>
                  )}
                </div>
              </>
            )}
            {loginSession?.status === "authorized" && (
              <div className="flex items-center gap-2 py-4 text-sm text-moon-600">
                <Loader2 className="size-4 animate-spin" />
                Authorized, finalizing account...
              </div>
            )}
            {loginSession?.status === "succeeded" && (
              <div className="space-y-3">
                <div className="rounded-lg bg-green-50 p-4 text-sm text-green-800">
                  Account created successfully!
                </div>
                {loginSession.account && (
                  <div className="space-y-1 text-sm text-moon-600">
                    <p><span className="font-medium">Label:</span> {loginSession.account.label}</p>
                    <p><span className="font-medium">Email:</span> {loginSession.account.cpa_email}</p>
                    <p><span className="font-medium">Plan:</span> {loginSession.account.cpa_plan_type}</p>
                  </div>
                )}
              </div>
            )}
            {(loginSession?.status === "failed" || loginSession?.status === "expired") && (
              <div className="space-y-3">
                <div className="rounded-lg bg-red-50 p-4 text-sm text-red-800">
                  {loginSession.error_message || "Login failed"}
                </div>
                <Button variant="outline" onClick={startDeviceCodeLogin}>Retry</Button>
              </div>
            )}
          </div>
          <DialogFooter>
            {loginSession?.status === "succeeded" ? (
              <Button onClick={() => { setShowDeviceCode(false); load(); }}>Done</Button>
            ) : (
              <Button variant="outline" onClick={() => {
                if (loginSession && (loginSession.status === "pending" || loginSession.status === "authorized")) {
                  api.post(`/accounts/cpa/login-sessions/${loginSession.id}/cancel`).catch(() => {});
                }
                if (pollRef.current) clearInterval(pollRef.current);
                pollRef.current = null;
                setShowDeviceCode(false);
              }}>Cancel</Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Import Dialog */}
      <Dialog open={showImport} onOpenChange={setShowImport}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>Import CPA Accounts</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-4">
            {importLoading && remoteAccounts.length === 0 ? (
              <div className="flex items-center justify-center py-8">
                <Loader2 className="size-6 animate-spin text-moon-400" />
              </div>
            ) : remoteAccounts.length === 0 ? (
              <p className="py-4 text-center text-sm text-moon-500">No accounts found in cpa-auth directory.</p>
            ) : (
              <div className="max-h-64 space-y-2 overflow-y-auto">
                {remoteAccounts.map((ra) => (
                  <label
                    key={ra.account_key}
                    className={`flex items-center gap-3 rounded-lg border p-3 transition ${ra.already_imported ? "cursor-not-allowed border-moon-100 bg-moon-50 opacity-60" : "cursor-pointer border-moon-200 hover:border-lunar-400"}`}
                  >
                    <input
                      type="checkbox"
                      disabled={ra.already_imported}
                      checked={selectedKeys.has(ra.account_key)}
                      onChange={(e) => {
                        const next = new Set(selectedKeys);
                        if (e.target.checked) next.add(ra.account_key);
                        else next.delete(ra.account_key);
                        setSelectedKeys(next);
                      }}
                      className="size-4"
                    />
                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-medium text-moon-800">{ra.email}</p>
                      <p className="text-xs text-moon-500">
                        {ra.provider} - {ra.plan_type || "unknown"}
                        {ra.expired_at && ` | Expires: ${new Date(ra.expired_at).toLocaleDateString()}`}
                      </p>
                    </div>
                    {ra.already_imported && (
                      <span className="rounded-md bg-moon-100 px-2 py-0.5 text-[10px] font-medium text-moon-500">
                        Already imported
                      </span>
                    )}
                  </label>
                ))}
              </div>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowImport(false)}>Cancel</Button>
            <Button
              disabled={selectedKeys.size === 0 || importLoading}
              onClick={handleImport}
            >
              {importLoading ? <Loader2 className="size-4 animate-spin" /> : `Import (${selectedKeys.size})`}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );

  // --- Device Code Login ---
  function startDeviceCodeLogin() {
    if (!cpaService) {
      toast("Please configure a CPA Service first", "error");
      return;
    }
    setLoginSession(null);
    setLoginLoading(true);
    setShowDeviceCode(true);

    api.post<LoginSession>("/accounts/cpa/login-sessions", { service_id: cpaService.id })
      .then((session) => {
        setLoginSession(session);
        setLoginLoading(false);
        // start polling
        if (pollRef.current) clearInterval(pollRef.current);
        const interval = (session.poll_interval_seconds ?? 5) * 1000;
        pollRef.current = setInterval(() => {
          api.get<LoginSession>(`/accounts/cpa/login-sessions/${session.id}`)
            .then((updated) => {
              setLoginSession(updated);
              if (updated.status !== "pending" && updated.status !== "authorized") {
                if (pollRef.current) clearInterval(pollRef.current);
                pollRef.current = null;
              }
            })
            .catch(() => {
              setLoginSession((prev) =>
                prev ? { ...prev, status: "failed", error_message: "Session lost. Please start again." } : null
              );
              if (pollRef.current) clearInterval(pollRef.current);
              pollRef.current = null;
            });
        }, interval);
      })
      .catch((err) => {
        setLoginLoading(false);
        toast(err instanceof Error ? err.message : "Failed to start login", "error");
        setShowDeviceCode(false);
      });
  }

  // --- Import ---
  function openImportDialog() {
    if (!cpaService) {
      toast("Please configure a CPA Service first", "error");
      return;
    }
    setRemoteAccounts([]);
    setSelectedKeys(new Set());
    setImportLabel("");
    setImportLoading(true);
    setShowImport(true);

    api.get<RemoteAccount[]>("/cpa/service/remote-accounts")
      .then((accs) => setRemoteAccounts(accs ?? []))
      .catch((err) => toast(err instanceof Error ? err.message : "Failed to scan accounts", "error"))
      .finally(() => setImportLoading(false));
  }

  async function handleImport() {
    if (!cpaService || selectedKeys.size === 0) return;
    setImportLoading(true);
    try {
      if (selectedKeys.size === 1) {
        const key = [...selectedKeys][0];
        await api.post("/accounts/cpa/import", {
          service_id: cpaService.id,
          account_key: key,
          label: importLabel,
          enabled: true,
        });
        toast("Account imported");
      } else {
        const result = await api.post<{ imported: number; skipped: number; errors: string[] }>("/accounts/cpa/import/batch", {
          service_id: cpaService.id,
          account_keys: [...selectedKeys],
        });
        toast(`Imported ${result.imported}, skipped ${result.skipped}${result.errors?.length ? `, ${result.errors.length} errors` : ""}`);
      }
      setShowImport(false);
      load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "Import failed", "error");
    } finally {
      setImportLoading(false);
    }
  }
}

function CountdownTimer({ expiresAt }: { expiresAt: string }) {
  const [remaining, setRemaining] = useState("");
  useEffect(() => {
    function update() {
      const diff = new Date(expiresAt).getTime() - Date.now();
      if (diff <= 0) { setRemaining("Expired"); return; }
      const min = Math.floor(diff / 60000);
      const sec = Math.floor((diff % 60000) / 1000);
      setRemaining(`${min}:${sec.toString().padStart(2, "0")}`);
    }
    update();
    const id = setInterval(update, 1000);
    return () => clearInterval(id);
  }, [expiresAt]);
  return <span className="inline-flex items-center gap-1 font-mono text-xs"><Clock className="size-3" />{remaining}</span>;
}
