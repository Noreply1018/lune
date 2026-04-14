import { type FormEvent, useEffect, useRef, useState } from "react";
import StatusBadge from "@/components/StatusBadge";
import DataTable, { type Column } from "@/components/DataTable";
import ConfirmDialog from "@/components/ConfirmDialog";
import PageHeader from "@/components/PageHeader";
import SectionHeading from "@/components/SectionHeading";
import { api } from "@/lib/api";
import { toast } from "@/components/Feedback";
import type { Account, CpaService, LatencyBucket, LoginSession, RemoteAccount } from "@/lib/types";
import { useRouter } from "@/lib/router";
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
import { Plus, MoreHorizontal, Globe, Server, KeyRound, Download, ExternalLink, Loader2, AlertTriangle, Clock, Zap, ArrowRight, ShieldCheck } from "lucide-react";
import type { TestConnectionResult } from "@/lib/types";


const PROVIDER_TEMPLATES = [
  { value: "openai", label: "OpenAI", base_url: "https://api.openai.com/v1" },
  { value: "deepseek", label: "DeepSeek", base_url: "https://api.deepseek.com/v1" },
  { value: "groq", label: "Groq", base_url: "https://api.groq.com/openai/v1" },
  { value: "mistral", label: "Mistral", base_url: "https://api.mistral.ai/v1" },
  { value: "together", label: "Together AI", base_url: "https://api.together.xyz/v1" },
  { value: "openrouter", label: "OpenRouter", base_url: "https://openrouter.ai/api/v1" },
  { value: "moonshot", label: "Moonshot", base_url: "https://api.moonshot.cn/v1" },
  { value: "custom", label: "自定义", base_url: "" },
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
  const label = status === "expired" ? "已过期" : status === "today" ? "今日到期" : "即将到期";
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
  const { navigate } = useRouter();
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [cpaService, setCpaService] = useState<CpaService | null>(null);
  const [loading, setLoading] = useState(true);

  // source selection
  const [showSourcePicker, setShowSourcePicker] = useState(false);
  const [sourceStep, setSourceStep] = useState<"root" | "cpa">("root");

  // forms
  const [showForm, setShowForm] = useState(false);
  const [editId, setEditId] = useState<number | null>(null);
  const [editSourceKind, setEditSourceKind] = useState<"openai_compat" | "cpa">("openai_compat");
  const [openaiForm, setOpenaiForm] = useState<OpenAIForm>(emptyOpenAIForm);
  const [cpaForm, setCpaForm] = useState<CpaForm>(emptyCpaForm);
  const [deleteTarget, setDeleteTarget] = useState<Account | null>(null);
  const [testing, setTesting] = useState(false);

  // CPA device code login
  const [showCpaLogin, setShowCpaLogin] = useState(false);
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
    let cancelled = false;

    Promise.all([
      api.get<Account[]>("/accounts").catch(() => {
        if (!cancelled) toast("加载账号列表失败", "error");
        return null;
      }),
      api.get<CpaService | null>("/cpa/service").catch(() => null),
    ]).then(([accs, svc]) => {
      if (cancelled) return;
      if (accs !== null) setAccounts(accs ?? []);
      setCpaService(svc ?? null);
    }).finally(() => {
      if (!cancelled) setLoading(false);
    });

    return () => { cancelled = true; };
  }

  useEffect(() => {
    const cancel = load();
    return cancel;
  }, []);

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
    setSourceStep("root");
    setShowSourcePicker(true);
  }

  function pickSource(kind: "openai_compat" | "cpa") {
    setEditSourceKind(kind);
    setShowSourcePicker(false);

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
          toast("账号已更新");
        } else {
          await api.post("/accounts", body);
          toast("账号已创建");
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
          toast("账号已更新");
        } else {
          body.api_key = openaiForm.api_key;
          await api.post("/accounts", body);
          toast("账号已创建");
        }
      }
      setShowForm(false);
      load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "操作失败", "error");
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
      toast("更新账号失败", "error");
    }
  }

  async function confirmDelete() {
    if (!deleteTarget) return;
    try {
      await api.delete(`/accounts/${deleteTarget.id}`);
      toast("账号已删除");
      load();
    } catch {
      toast("删除账号失败", "error");
    } finally {
      setDeleteTarget(null);
    }
  }

  async function handleTestConnection() {
    if (!openaiForm.base_url || !openaiForm.api_key) return;
    setTesting(true);
    try {
      const result = await api.post<TestConnectionResult>("/accounts/test-connection", {
        base_url: openaiForm.base_url,
        api_key: openaiForm.api_key,
      });
      if (result.reachable) {
        toast(`连接成功，延迟 ${result.latency_ms}ms — 发现 ${result.models?.length ?? 0} 个模型`);
      } else {
        toast(result.error || "连接失败", "error");
      }
    } catch (err) {
      toast(err instanceof Error ? err.message : "测试失败", "error");
    } finally {
      setTesting(false);
    }
  }

  const columns: Column<Account>[] = [
    {
      key: "label",
      header: "标签",
      render: (r) => <span className="font-medium text-moon-800">{r.label}</span>,
      tone: "primary",
    },
    {
      key: "source",
      header: "来源",
      render: (r) =>
        r.source_kind === "cpa" ? (
          <span
            className="rounded-md bg-lunar-100/60 px-2 py-0.5 text-xs font-medium text-lunar-700"
            title={r.cpa_account_key ? "凭据型 CPA 账号" : "CPA Provider 通道"}
          >
            CPA - {r.cpa_provider}
          </span>
        ) : (
          <span className="rounded-md bg-moon-100/60 px-2 py-0.5 text-xs font-medium text-moon-600">
            OpenAI 兼容
          </span>
        ),
    },
    {
      key: "unit_type",
      header: "单元类型",
      render: (r) =>
        r.source_kind === "cpa" ? (
          <span className="text-sm text-moon-600">
            {r.cpa_account_key ? "凭据型账号" : "Provider 通道"}
          </span>
        ) : (
          <span className="text-sm text-moon-600">直连 API 凭据</span>
        ),
      tone: "secondary",
    },
    {
      key: "endpoint",
      header: "接入地址",
      render: (r) => (
        <code className="text-xs text-moon-500">
          {r.runtime?.base_url ?? r.base_url}
        </code>
      ),
      tone: "secondary",
    },
    {
      key: "status",
      header: "状态",
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
      key: "budget",
      header: "额度",
      render: (r) =>
        r.source_kind === "cpa" ? (
          <span className="text-sm text-moon-400">由 CPA 管理</span>
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
      header: "延迟 (24h)",
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
              编辑
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => toggleAccount(r)}>
              {r.enabled ? "停用" : "启用"}
            </DropdownMenuItem>
            <DropdownMenuItem
              className="text-destructive"
              onClick={() => setDeleteTarget(r)}
            >
              删除
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      ),
    },
  ];

  const enabledAccounts = accounts.filter((account) => account.enabled).length;
  const cpaAccounts = accounts.filter((account) => account.source_kind === "cpa").length;
  const directAccounts = accounts.filter((account) => account.source_kind === "openai_compat").length;
  const expiringAccounts = accounts.filter(
    (account) =>
      account.source_kind === "cpa" &&
      (() => {
        const status = expiryStatus(account.cpa_expired_at);
        return status === "today" || status === "soon" || status === "expired";
      })(),
  ).length;

  return (
    <div className="space-y-10">
      <PageHeader
        eyebrow="Accounts / Resources"
        title="账号"
        description="管理上游执行单元，包括直连 API 与 CPA 托管账号。"
        meta={
          <>
            <span>总数 {accounts.length}</span>
            <span>已启用 {enabledAccounts}</span>
            <span>CPA {cpaAccounts}</span>
          </>
        }
        actions={
          <Button size="sm" onClick={openCreate}>
            <Plus className="size-4" />
            新增账号
          </Button>
        }
      />

      <section className="grid gap-6 xl:grid-cols-[minmax(0,1.18fr)_minmax(320px,0.82fr)]">
        <div className="surface-section px-5 py-5 sm:px-6 sm:py-6">
          <div className="flex items-start justify-between gap-4">
            <div className="space-y-2">
              <p className="eyebrow-label">资源面板</p>
              <h2 className="text-[1.15rem] font-semibold tracking-[-0.03em] text-moon-800 sm:text-[1.25rem]">
                这里维护所有可参与路由的执行单元
              </h2>
              <p className="max-w-2xl text-sm leading-7 text-moon-500">
                直连账号负责稳定接入，CPA 账号负责托管凭据与订阅型入口。这一页更像资源清单，而不是营销式概览。
              </p>
            </div>
            <span className="flex size-12 items-center justify-center rounded-[1.2rem] border border-white/75 bg-white/70 text-lunar-700">
              <KeyRound className="size-5" />
            </span>
          </div>

          <div className="mt-6 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            <div className="rounded-[1.25rem] border border-white/72 bg-white/68 px-4 py-4">
              <p className="kicker">总账号数</p>
              <p className="mt-3 text-[1.55rem] font-semibold tracking-[-0.05em] text-moon-800">
                {accounts.length}
              </p>
            </div>
            <div className="rounded-[1.25rem] border border-white/72 bg-white/68 px-4 py-4">
              <p className="kicker">已启用</p>
              <p className="mt-3 text-[1.55rem] font-semibold tracking-[-0.05em] text-moon-800">
                {enabledAccounts}
              </p>
            </div>
            <div className="rounded-[1.25rem] border border-white/72 bg-white/68 px-4 py-4">
              <p className="kicker">Direct API</p>
              <p className="mt-3 text-[1.55rem] font-semibold tracking-[-0.05em] text-moon-800">
                {directAccounts}
              </p>
            </div>
            <div className="rounded-[1.25rem] border border-white/72 bg-white/68 px-4 py-4">
              <p className="kicker">CPA 托管</p>
              <p className="mt-3 text-[1.55rem] font-semibold tracking-[-0.05em] text-moon-800">
                {cpaAccounts}
              </p>
            </div>
          </div>
        </div>

        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-1">
          <div className="surface-card px-5 py-5">
            <p className="eyebrow-label">行动面板</p>
            <div className="mt-3 space-y-3">
              <button
                type="button"
                onClick={openCreate}
                className="flex w-full items-center justify-between rounded-[1.05rem] border border-white/70 bg-white/72 px-4 py-3 text-left transition hover:border-lunar-300"
              >
                <span>
                  <span className="block text-sm font-medium text-moon-800">新增账号</span>
                  <span className="mt-1 block text-xs text-moon-500">新增直连 API 或 CPA 托管单元。</span>
                </span>
                <ArrowRight className="size-4 text-moon-400" />
              </button>
              <button
                type="button"
                onClick={() => navigate("/admin/cpa-service")}
                className="flex w-full items-center justify-between rounded-[1.05rem] border border-white/70 bg-white/72 px-4 py-3 text-left transition hover:border-lunar-300"
              >
                <span>
                  <span className="block text-sm font-medium text-moon-800">CPA 服务</span>
                  <span className="mt-1 block text-xs text-moon-500">
                    {cpaService ? "查看服务健康与远端导入。" : "配置后才能新增托管账号。"}
                  </span>
                </span>
                <ExternalLink className="size-4 text-moon-400" />
              </button>
            </div>
          </div>

          <div className="surface-card px-5 py-5">
            <p className="eyebrow-label">资源健康</p>
            <div className="mt-4 space-y-3 text-sm text-moon-500">
              <div className="flex items-center justify-between gap-3 border-b border-moon-200/60 pb-3">
                <span>CPA 服务</span>
                <span className="font-medium text-moon-700">
                  {cpaService ? (cpaService.enabled ? "已连接" : "已配置 / 已停用") : "未配置"}
                </span>
              </div>
              <div className="flex items-center justify-between gap-3 border-b border-moon-200/60 pb-3">
                <span>临近到期</span>
                <span className="font-medium text-moon-700">{expiringAccounts}</span>
              </div>
              <div className="flex items-center justify-between gap-3">
                <span>启用比例</span>
                <span className="font-medium text-moon-700">
                  {accounts.length > 0 ? `${Math.round((enabledAccounts / accounts.length) * 100)}%` : "暂无"}
                </span>
              </div>
            </div>
          </div>
        </div>
      </section>

      <section className="space-y-4">
        <SectionHeading
          title="账号列表"
          description="每一行都是一个可路由执行单元。先看来源与状态，再判断预算与延迟。"
        />

        {loading ? (
          <Skeleton className="h-64 rounded-[1.5rem]" />
        ) : (
          <div className="surface-card overflow-hidden">
            <div className="flex flex-col gap-2 border-b border-moon-200/60 px-4 py-3 sm:flex-row sm:items-end sm:justify-between">
              <div>
                <p className="eyebrow-label">资源清单</p>
                <p className="mt-1 text-sm text-moon-500">
                  先看标签、来源和状态，再判断预算与最近延迟。
                </p>
              </div>
              <p className="text-sm text-moon-500">
                Direct API {directAccounts} / CPA {cpaAccounts}
              </p>
            </div>
            <DataTable
              columns={columns}
              rows={accounts}
              rowKey={(r) => r.id}
              empty="暂未配置账号"
            />
          </div>
        )}
      </section>

      {/* Source picker dialog */}
      <Dialog open={showSourcePicker} onOpenChange={setShowSourcePicker}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>{sourceStep === "root" ? "新增账号" : "选择 CPA 接入方式"}</DialogTitle>
          </DialogHeader>
          {sourceStep === "root" ? (
            <div className="space-y-4 py-4">
              <p className="text-sm text-moon-500">
                先选择账号来源。若选择 CPA，下一步再选择具体接入方式。
              </p>
              <div className="grid gap-3">
                <button
                  type="button"
                  onClick={() => pickSource("openai_compat")}
                  className="flex items-start gap-4 rounded-2xl border border-moon-200 px-5 py-4 text-left transition hover:border-lunar-400 hover:bg-lunar-50/30"
                >
                  <span className="mt-0.5 flex size-11 items-center justify-center rounded-2xl bg-moon-100 text-moon-500">
                    <Globe className="size-5" />
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="flex items-center justify-between gap-3">
                      <span className="font-medium text-moon-800">OpenAI-Compatible</span>
                      <ArrowRight className="size-4 text-moon-300" />
                    </span>
                    <span className="mt-1 block text-sm text-moon-500">
                      直接接入任意 OpenAI-Compatible API。
                    </span>
                  </span>
                </button>
                <button
                  type="button"
                  onClick={() => cpaService && setSourceStep("cpa")}
                  disabled={!cpaService}
                  className={`flex items-start gap-4 rounded-2xl border px-5 py-4 text-left transition ${
                    cpaService
                      ? "border-moon-200 hover:border-lunar-400 hover:bg-lunar-50/30"
                      : "cursor-not-allowed border-moon-200 bg-moon-50/80 opacity-80"
                  }`}
                >
                  <span className="mt-0.5 flex size-11 items-center justify-center rounded-2xl bg-lunar-100/70 text-lunar-600">
                    <Server className="size-5" />
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="flex items-center justify-between gap-3">
                      <span className="font-medium text-moon-800">CPA</span>
                      {cpaService ? (
                        <ArrowRight className="size-4 text-moon-300" />
                      ) : (
                        <span className="rounded-full bg-amber-100 px-2 py-0.5 text-[11px] font-medium text-amber-700">
                          需先配置
                        </span>
                      )}
                    </span>
                    <span className="mt-1 block text-sm text-moon-500">
                      通过 CPA 服务创建 Provider Channel 或凭据型账号。
                    </span>
                    {!cpaService && (
                      <span className="mt-3 block text-xs text-moon-500">
                        请先配置 CPA 服务，再回来选择接入方式。
                      </span>
                    )}
                  </span>
                </button>
              </div>
              {!cpaService && (
                <div className="rounded-2xl border border-amber-200 bg-amber-50/80 p-4">
                  <div className="flex items-start gap-3">
                    <AlertTriangle className="mt-0.5 size-4 text-amber-600" />
                    <div className="space-y-3">
                      <div>
                        <p className="text-sm font-medium text-amber-900">当前无法使用 CPA 入口</p>
                        <p className="mt-1 text-sm text-amber-800/80">
                          需要先配置 CPA 服务，才能创建 CPA Channel、执行 Codex 登录或导入已有账号。
                        </p>
                      </div>
                      <Button
                        type="button"
                        size="sm"
                        onClick={() => {
                          setShowSourcePicker(false);
                          navigate("/admin/cpa-service");
                        }}
                      >
                        前往 CPA 服务
                      </Button>
                    </div>
                  </div>
                </div>
              )}
            </div>
          ) : (
            <div className="space-y-4 py-4">
              <div className="rounded-2xl border border-lunar-200/80 bg-lunar-50/40 p-4">
                <p className="text-xs font-semibold uppercase tracking-[0.22em] text-lunar-700">CPA 服务</p>
                <div className="mt-2 flex items-center justify-between gap-3">
                  <div>
                    <p className="text-sm font-medium text-moon-800">{cpaService?.label}</p>
                    <p className="text-xs text-moon-500">{cpaService?.base_url}</p>
                  </div>
                  <span className="inline-flex items-center gap-1 rounded-full bg-white px-2.5 py-1 text-[11px] font-medium text-lunar-700">
                    <ShieldCheck className="size-3.5" />
                    已就绪
                  </span>
                </div>
              </div>
              <div className="grid gap-3">
                <button
                  type="button"
                  onClick={() => pickSource("cpa")}
                  className="flex items-start gap-4 rounded-2xl border border-moon-200 px-5 py-4 text-left transition hover:border-lunar-400 hover:bg-lunar-50/30"
                >
                  <span className="mt-0.5 flex size-11 items-center justify-center rounded-2xl bg-lunar-100/70 text-lunar-600">
                    <Server className="size-5" />
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="font-medium text-moon-800">Provider Channel</span>
                    <span className="mt-1 block text-sm text-moon-500">
                      为单个 Provider 创建路由通道，并由 CPA 负责刷新与执行。
                    </span>
                  </span>
                </button>
                <button
                  type="button"
                  onClick={() => {
                    setShowSourcePicker(false);
                    startCpaOAuthLogin();
                  }}
                  className="flex items-start gap-4 rounded-2xl border border-moon-200 px-5 py-4 text-left transition hover:border-lunar-400 hover:bg-lunar-50/30"
                >
                  <span className="mt-0.5 flex size-11 items-center justify-center rounded-2xl bg-moon-100 text-moon-500">
                    <KeyRound className="size-5" />
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="font-medium text-moon-800">Codex 登录</span>
                    <span className="mt-1 block text-sm text-moon-500">
                      通过 Codex OAuth 登录，创建凭据型 CPA 账号。
                    </span>
                  </span>
                </button>
                <button
                  type="button"
                  onClick={() => {
                    setShowSourcePicker(false);
                    openImportDialog();
                  }}
                  className="flex items-start gap-4 rounded-2xl border border-moon-200 px-5 py-4 text-left transition hover:border-lunar-400 hover:bg-lunar-50/30"
                >
                  <span className="mt-0.5 flex size-11 items-center justify-center rounded-2xl bg-moon-100 text-moon-500">
                    <Download className="size-5" />
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="font-medium text-moon-800">导入已有账号</span>
                    <span className="mt-1 block text-sm text-moon-500">
                      从本地 CPA 凭据目录中导入已存在的凭据型账号。
                    </span>
                  </span>
                </button>
              </div>
              <DialogFooter className="pt-1">
                <Button type="button" variant="outline" onClick={() => setSourceStep("root")}>
                  返回
                </Button>
              </DialogFooter>
            </div>
          )}
        </DialogContent>
      </Dialog>

      {/* Account form dialog */}
      <Dialog open={showForm} onOpenChange={setShowForm}>
        <DialogContent className="max-w-lg">
          <form onSubmit={handleSubmit}>
            <DialogHeader>
              <DialogTitle>
                {editId
                  ? "编辑账号"
                  : editSourceKind === "cpa"
                    ? "新增 CPA Provider Channel"
                    : "新增账号"}
              </DialogTitle>
            </DialogHeader>

            <div className="space-y-4 py-4">
              {editSourceKind === "cpa" ? (
                /* CPA form */
                <>
                  <div className="space-y-2">
                    <Label>CPA 服务</Label>
                    <Input
                      value={cpaService?.label ?? "未配置"}
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
                          <SelectValue placeholder="选择 Provider..." />
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
                    <Label htmlFor="cpa-label">标签</Label>
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
                    <Label htmlFor="cpa-models">模型白名单</Label>
                    <Input
                      id="cpa-models"
                      value={cpaForm.model_allowlist}
                      onChange={(e) =>
                        setCpaForm({
                          ...cpaForm,
                          model_allowlist: e.target.value,
                        })
                      }
                      placeholder="gpt-4o, gpt-4.1（逗号分隔，留空表示全部）"
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="cpa-notes">备注</Label>
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
                        <SelectValue placeholder="选择 Provider..." />
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
                    <Label htmlFor="acc-label">标签</Label>
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
                        editId ? "留空则保留当前密钥" : "sk-..."
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
                    disabled={testing || !openaiForm.base_url || !openaiForm.api_key}
                  >
                    {testing ? <Loader2 className="size-4 animate-spin" /> : <Zap className="size-4" />}
                    测试连接
                  </Button>
                  {!editId && (!openaiForm.base_url || !openaiForm.api_key) && (
                      <p className="text-xs text-moon-500">
                      请先填写 Base URL 和 API Key，再进行连接测试。
                    </p>
                  )}
                  {editId && !openaiForm.api_key && (
                    <p className="text-xs text-moon-500">
                      若要测试连接，请输入新的 API Key。留空会保留当前密钥，但这里无法直接测试。
                    </p>
                  )}

                  <div className="space-y-2">
                    <Label htmlFor="acc-models">模型白名单</Label>
                    <Input
                      id="acc-models"
                      value={openaiForm.model_allowlist}
                      onChange={(e) =>
                        setOpenaiForm({
                          ...openaiForm,
                          model_allowlist: e.target.value,
                        })
                      }
                      placeholder="gpt-4o, gpt-4o-mini（逗号分隔，留空表示全部）"
                    />
                  </div>

                  <div className="grid grid-cols-3 gap-3">
                    <div className="space-y-2">
                      <Label htmlFor="acc-quota-total">总额度</Label>
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
                      <Label htmlFor="acc-quota-used">已用</Label>
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
                      <Label>单位</Label>
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
                    <Label htmlFor="acc-notes">备注</Label>
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
        title="删除账号"
        description={`确认删除“${deleteTarget?.label ?? ""}”吗？此操作不可撤销。`}
        onConfirm={confirmDelete}
      />

      {/* CPA Codex Login Dialog */}
      <Dialog open={showCpaLogin} onOpenChange={(open) => {
        if (!open) {
          if (loginSession && (loginSession.status === "pending" || loginSession.status === "scanning")) {
            api.post(`/accounts/cpa/login-sessions/${loginSession.id}/cancel`).catch(() => {});
          }
          if (pollRef.current) clearInterval(pollRef.current);
          pollRef.current = null;
        }
        setShowCpaLogin(open);
      }}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>CPA Codex 登录</DialogTitle>
          </DialogHeader>
          {(() => {
            const status = getDeviceCodeDialogStatus(loginSession, loginLoading);

            return (
              <div className="space-y-5 py-4">
                <div className="rounded-[1.35rem] border border-white/75 bg-white/62 px-4 py-4">
                  <div className="grid gap-4 sm:grid-cols-3">
                    <div>
                      <p className="eyebrow-label">CPA 服务</p>
                      <p className="mt-2 text-sm font-medium text-moon-800">{cpaService?.label ?? "未配置"}</p>
                    </div>
                    <div>
                      <p className="eyebrow-label">授权方式</p>
                      <p className="mt-2 text-sm font-medium text-moon-800">Codex OAuth</p>
                    </div>
                    <div>
                      <p className="eyebrow-label">结果</p>
                      <p className="mt-2 text-sm font-medium text-moon-800">创建凭据型 CPA 账号</p>
                    </div>
                  </div>
                </div>

                <div className="space-y-3">
                  <div className="rounded-[1.25rem] border border-white/72 bg-[linear-gradient(180deg,rgba(243,239,250,0.82),rgba(255,255,255,0.72))] px-4 py-4">
                    <div className="flex items-start justify-between gap-3">
                      <div>
                        <p className="text-sm font-medium text-moon-800">{status.title}</p>
                        <p className="mt-1 text-sm leading-6 text-moon-500">{status.description}</p>
                      </div>
                      {loginSession?.expires_at && loginSession.status !== "succeeded" ? (
                        <span className="shrink-0 rounded-full border border-white/75 bg-white/75 px-2.5 py-1">
                          <CountdownTimer expiresAt={loginSession.expires_at} />
                        </span>
                      ) : null}
                    </div>
                  </div>

                  {loginLoading && !loginSession ? (
                    <div className="flex items-center gap-2 text-sm text-moon-500">
                      <Loader2 className="size-4 animate-spin" />
                      正在创建登录会话...
                    </div>
                  ) : null}

                  {loginSession && loginSession.status === "pending" && loginSession.auth_url ? (
                    <div className="rounded-[1.15rem] border border-white/72 bg-white/72 px-4 py-5">
                      <p className="text-sm font-medium text-moon-800">请在浏览器中完成 OpenAI 授权</p>
                      <p className="mt-2 text-sm text-moon-500">点击下方按钮打开授权页面，在浏览器中完成登录后此处将自动更新。</p>
                      <div className="mt-4 flex items-center justify-center">
                        <Button
                          onClick={() => loginSession.auth_url && window.open(loginSession.auth_url, "_blank")}
                        >
                          <ExternalLink className="size-4" />
                          打开授权页
                        </Button>
                      </div>
                    </div>
                  ) : null}

                  {loginSession?.status === "pending" ? (
                    <div className="flex items-center gap-2 rounded-[1rem] border border-white/72 bg-white/68 px-4 py-3 text-sm text-moon-500">
                      <Loader2 className="size-4 animate-spin" />
                      正在等待授权完成...
                    </div>
                  ) : null}

                  {loginSession?.status === "scanning" ? (
                    <div className="flex items-center gap-2 rounded-[1rem] border border-white/72 bg-white/68 px-4 py-3 text-sm text-moon-600">
                      <Loader2 className="size-4 animate-spin" />
                      授权已完成，正在扫描并导入账号...
                    </div>
                  ) : null}

                  {loginSession?.status === "succeeded" && loginSession.account ? (
                    <div className="rounded-[1.1rem] border border-status-green/20 bg-status-green/10 px-4 py-4 text-sm text-moon-700">
                      <p className="font-medium text-moon-800">已创建账号</p>
                      <div className="mt-2 space-y-1">
                        <p>标签：{loginSession.account.label}</p>
                        <p>邮箱：{loginSession.account.cpa_email || "-"}</p>
                        <p>套餐：{loginSession.account.cpa_plan_type || "-"}</p>
                      </div>
                    </div>
                  ) : null}

                  {(loginSession?.status === "failed" || loginSession?.status === "expired") ? (
                    <div className="rounded-[1.1rem] border border-status-red/20 bg-status-red/10 px-4 py-4 text-sm text-moon-700">
                      <p className="font-medium text-moon-800">
                        {loginSession.status === "expired" ? "已过期" : "登录失败"}
                      </p>
                      <p className="mt-1">
                        {loginSession.error_message || (loginSession.status === "expired" ? "登录已过期，请重新开始。" : "请重试")}
                      </p>
                    </div>
                  ) : null}
                </div>
              </div>
            );
          })()}
          <DialogFooter>
            {loginSession?.status === "succeeded" ? (
              <Button onClick={() => { setShowCpaLogin(false); load(); }}>完成</Button>
            ) : (
              <Button variant="outline" onClick={() => {
                if (loginSession && (loginSession.status === "pending" || loginSession.status === "scanning")) {
                  api.post(`/accounts/cpa/login-sessions/${loginSession.id}/cancel`).catch(() => {});
                }
                if (pollRef.current) clearInterval(pollRef.current);
                pollRef.current = null;
                setShowCpaLogin(false);
              }}>取消</Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Import Dialog */}
      <Dialog open={showImport} onOpenChange={setShowImport}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>导入 CPA 账号</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="rounded-2xl border border-moon-200/70 bg-moon-50/70 p-4 text-sm text-moon-500">
              扫描本地 CPA 凭据目录中的现有账号，并将选中的凭据型账号接入当前工作区。
            </div>
            {importLoading && remoteAccounts.length === 0 ? (
              <div className="flex items-center justify-center py-8">
                <Loader2 className="size-6 animate-spin text-moon-400" />
              </div>
            ) : remoteAccounts.length === 0 ? (
              <p className="py-4 text-center text-sm text-moon-500">在 cpa-auth 目录中未发现可导入账号。</p>
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
                        {ra.provider} - {ra.plan_type || "未知套餐"}
                        {ra.expired_at && ` | 到期：${new Date(ra.expired_at).toLocaleDateString()}`}
                      </p>
                    </div>
                    {ra.already_imported && (
                      <span className="rounded-md bg-moon-100 px-2 py-0.5 text-[10px] font-medium text-moon-500">
                        已导入
                      </span>
                    )}
                  </label>
                ))}
              </div>
            )}
            {selectedKeys.size === 1 && (
              <div className="space-y-2">
                <Label htmlFor="import-label">自定义标签</Label>
                <Input
                  id="import-label"
                  value={importLabel}
                  onChange={(e) => setImportLabel(e.target.value)}
                  placeholder="选填。留空则使用自动命名。"
                />
                <p className="text-xs text-moon-500">
                  仅在导入单个账号时生效。
                </p>
              </div>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowImport(false)}>取消</Button>
            <Button
              disabled={selectedKeys.size === 0 || importLoading}
              onClick={handleImport}
            >
              {importLoading ? <Loader2 className="size-4 animate-spin" /> : `导入（${selectedKeys.size}）`}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );

  // --- CPA Codex Login ---
  function startCpaOAuthLogin() {
    if (!cpaService) return;
    // cancel existing session if still active
    if (loginSession && (loginSession.status === "pending" || loginSession.status === "scanning")) {
      api.post(`/accounts/cpa/login-sessions/${loginSession.id}/cancel`).catch(() => {});
    }
    if (pollRef.current) clearInterval(pollRef.current);
    pollRef.current = null;
    setLoginSession(null);
    setLoginLoading(true);
    setShowCpaLogin(true);

    api.post<LoginSession>("/accounts/cpa/login-sessions", { service_id: cpaService.id })
      .then((session) => {
        setLoginSession(session);
        setLoginLoading(false);
        if (session.auth_url) {
          window.open(session.auth_url, "_blank");
        }
        // start polling
        if (pollRef.current) clearInterval(pollRef.current);
        const interval = 5000;
        pollRef.current = setInterval(() => {
          api.get<LoginSession>(`/accounts/cpa/login-sessions/${session.id}`)
            .then((updated) => {
              setLoginSession(updated);
              if (updated.status !== "pending" && updated.status !== "scanning") {
                if (pollRef.current) clearInterval(pollRef.current);
                pollRef.current = null;
              }
            })
            .catch(() => {
              setLoginSession((prev) =>
                prev ? { ...prev, status: "failed", error_message: "登录会话已丢失，请重新开始。" } : null
              );
              if (pollRef.current) clearInterval(pollRef.current);
              pollRef.current = null;
            });
        }, interval);
      })
      .catch((err) => {
        setLoginLoading(false);
        toast(err instanceof Error ? err.message : "无法发起登录流程", "error");
        setShowCpaLogin(false);
      });
  }

  // --- Import ---
  function openImportDialog() {
    if (!cpaService) return;
    setRemoteAccounts([]);
    setSelectedKeys(new Set());
    setImportLabel("");
    setImportLoading(true);
    setShowImport(true);

    api.get<RemoteAccount[]>("/cpa/service/remote-accounts")
      .then((accs) => setRemoteAccounts(accs ?? []))
      .catch((err) => toast(err instanceof Error ? err.message : "扫描账号失败", "error"))
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
        toast("账号已导入");
      } else {
        const result = await api.post<{ imported: number; skipped: number; errors: string[] }>("/accounts/cpa/import/batch", {
          service_id: cpaService.id,
          account_keys: [...selectedKeys],
        });
        toast(`已导入 ${result.imported} 个，跳过 ${result.skipped} 个${result.errors?.length ? `，${result.errors.length} 个失败` : ""}`);
      }
      setShowImport(false);
      load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "导入失败", "error");
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
      if (diff <= 0) { setRemaining("已过期"); return; }
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

function getDeviceCodeDialogStatus(session: LoginSession | null, loginLoading: boolean): {
  tone: "neutral" | "success" | "error" | "pending";
  title: string;
  description: string;
} {
  if (loginLoading && !session) {
    return {
      tone: "pending",
      title: "正在发起登录...",
      description: "请稍候",
    };
  }

  if (!session) {
    return {
      tone: "neutral",
      title: "准备开始授权",
      description: "请稍候",
    };
  }

  if (session.status === "pending") {
    return {
      tone: "pending",
      title: "等待授权",
      description: "请在浏览器中完成授权",
    };
  }

  if (session.status === "scanning") {
    return {
      tone: "pending",
      title: "正在导入",
      description: "授权已完成，正在扫描并导入账号...",
    };
  }

  if (session.status === "succeeded") {
    return {
      tone: "success",
      title: "授权成功",
      description: "账号已创建",
    };
  }

  if (session.status === "expired") {
    return {
      tone: "error",
      title: "已过期",
      description: session.error_message || "登录已过期，请重新开始。",
    };
  }

  // failed / cancelled
  return {
    tone: "error",
    title: "登录失败",
    description: session.error_message || "请重试",
  };
}
