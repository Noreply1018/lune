import { useEffect, useMemo, useRef, useState } from "react";
import { Check, Clock3, Copy, Loader2, RotateCcw, Save, SendHorizonal } from "lucide-react";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Button } from "@/components/ui/button";
import { toast } from "@/components/Feedback";
import { Input } from "@/components/ui/input";
import { Tabs, TabsList, TabsIndicator, TabsPanel, TabsTab } from "@/components/ui/tabs";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import StatusBadge from "@/components/StatusBadge";
import { CodexQuotaBarsFull, CodexQuotaBarsPendingCompact } from "@/components/CodexQuotaBars";
import ProbeModelChipPicker from "@/components/ProbeModelChipPicker";
import { api } from "@/lib/api";
import { parseCodexQuota, type CodexQuota } from "@/lib/codexQuota";
import { compact, latency, relativeTime } from "@/lib/fmt";
import {
  ensureArray,
  getAccessLabel,
  getAccountHealth,
  getCpaCredentialMeta,
  getCpaQuotaErrorMeta,
  getCpaSubscriptionErrorMeta,
  getExpiryMeta,
  parseQuotaDisplay,
  getRouteSummary,
  hasRuntimeBindingIssue,
  type RouteSummary,
} from "@/lib/lune";
import type { Account, LatencyBucket, PoolMember } from "@/lib/types";
import { cn } from "@/lib/utils";

type Tone = "default" | "success" | "warning" | "danger";

type TabKey = "overview" | "playground" | "diagnostics";

const PRESET_MESSAGES = [
  { label: "你好", value: "你好，请用一句话回复我。" },
  { label: "你是什么模型？", value: "请告诉我你的模型名称和版本。" },
  { label: "自定义", value: "" },
];

type AccountStats = {
  requests: number;
  successRate: number | null;
  inputTokens: number;
  outputTokens: number;
};

export default function AccountDetailSheet({
  member,
  stats,
  priorityIndex,
  poolId,
  resolveToken,
  onAccountUpdated,
  onOpenChange,
}: {
  member: PoolMember | null;
  stats: AccountStats;
  priorityIndex?: number;
  poolId?: number;
  resolveToken: () => Promise<string>;
  onAccountUpdated?: () => void;
  onOpenChange: (open: boolean) => void;
}) {
  const account = member?.account ?? null;
  const open = Boolean(member && account);
  const [tab, setTab] = useState<TabKey>("overview");

  useEffect(() => {
    if (open) setTab("overview");
  }, [open, account?.id]);

  if (!member || !account) {
    return (
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent
          side="right"
          className="w-full max-w-[42rem] border-l border-white/75 bg-[linear-gradient(180deg,rgba(252,250,247,0.97),rgba(246,244,240,0.96))] p-0 data-[side=right]:sm:max-w-[42rem] sm:max-w-[42rem]"
        />
      </Sheet>
    );
  }

  const health = getAccountHealth(account);
  const isCodexCpa =
    account.source_kind === "cpa" && account.cpa_provider.toLowerCase() === "codex";
  const expiry = getExpiryMeta(
    isCodexCpa ? account.cpa_subscription_expires_at ?? null : account.cpa_expired_at ?? null,
  );
  const credential = getCpaCredentialMeta(account);
  const subscriptionError = getCpaSubscriptionErrorMeta(account);
  const quota = parseQuotaDisplay(account.quota_display ?? "");
  const codexQuota = parseCodexQuota(account);
  const models = ensureArray(account.models);
  const routeSummary = getRouteSummary(account, member.enabled, health);

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        className="w-full max-w-[42rem] overflow-hidden border-l border-white/75 bg-[linear-gradient(180deg,rgba(252,250,247,0.97),rgba(246,244,240,0.96))] p-0 data-[side=right]:sm:max-w-[42rem] sm:max-w-[42rem]"
      >
        <div className="flex h-full flex-col">
          <SheetHeader className="gap-2 border-b border-moon-200/55 px-7 py-6">
            <div className="flex items-center gap-2">
              <p className="eyebrow-label">{getAccessLabel(account)}</p>
              {priorityIndex != null ? (
                <span className="rounded-full bg-lunar-100/80 px-2 py-0.5 text-[10px] tracking-[0.08em] text-lunar-700">
                  P{priorityIndex}
                </span>
              ) : null}
            </div>
            <SheetTitle className="text-[1.35rem] font-semibold tracking-[-0.02em] text-moon-800">
              {account.label}
            </SheetTitle>
            <SheetDescription className="text-moon-500">
              当前账号的运行细节、直测与底层字段，分三页查看。
            </SheetDescription>
            <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-moon-500">
              <StatusBadge status={routeSummary.status} />
              <span className="rounded-full bg-moon-100/85 px-2.5 py-1">
                {isCodexCpa ? `Codex${account.cpa_plan_type ? ` · ${account.cpa_plan_type}` : ""}` : quota}
              </span>
              {expiry ? (
                <span
                  className={cn(
                    "rounded-full px-2.5 py-1",
                    expiry.tone === "danger"
                      ? "bg-status-red/10 text-status-red"
                      : expiry.tone === "warning"
                        ? "bg-status-yellow/12 text-status-yellow"
                        : "bg-moon-100/80 text-moon-500",
                  )}
                >
                  {expiry.label}
                </span>
              ) : null}
              {credential ? (
                <span
                  className={cn(
                    "rounded-full px-2.5 py-1",
                    credential.tone === "danger"
                      ? "bg-status-red/10 text-status-red"
                      : "bg-moon-100/80 text-moon-500",
                  )}
                  title={credential.detail}
                >
                  {credential.label}
                </span>
              ) : null}
              {subscriptionError ? (
                <span
                  className="rounded-full bg-status-yellow/12 px-2.5 py-1 text-status-yellow"
                  title={subscriptionError.detail}
                >
                  {subscriptionError.label}
                </span>
              ) : null}
              <span className="inline-flex items-center gap-1 text-moon-400">
                <Clock3 className="size-3" />
                {relativeTime(account.last_checked_at ?? null)}
              </span>
            </div>
          </SheetHeader>

          <Tabs
            value={tab}
            onValueChange={(next) => setTab(next as TabKey)}
            className="flex min-h-0 flex-1 flex-col gap-0"
          >
            <div className="border-b border-moon-200/55 px-7 py-3">
              <TabsList>
                <TabsIndicator />
                <TabsTab value="overview">Overview</TabsTab>
                <TabsTab value="playground">Playground</TabsTab>
                <TabsTab value="diagnostics">Diagnostics</TabsTab>
              </TabsList>
            </div>

            <div className="min-h-0 flex-1 overflow-y-auto px-7 py-6">
              <TabsPanel value="overview">
                <OverviewPanel
                  accountId={account.id}
                  poolId={poolId}
                  stats={stats}
                  models={models}
                  quota={quota}
                  account={account}
                  codexQuota={codexQuota}
                  routeSummary={routeSummary}
                  onAccountUpdated={onAccountUpdated}
                />
              </TabsPanel>
              <TabsPanel value="playground">
                <PlaygroundPanel
                  key={account.id}
                  accountId={account.id}
                  models={models}
                  disabled={!member.enabled}
                  resolveToken={resolveToken}
                />
              </TabsPanel>
              <TabsPanel value="diagnostics">
                <DiagnosticPanel
                  account={account}
                  accountId={account.id}
                  baseUrl={account.runtime?.base_url || account.base_url || "--"}
                  routeSummary={routeSummary}
                  discoveryHealth={health}
                  isCodexCpa={isCodexCpa}
                />
              </TabsPanel>
            </div>
          </Tabs>
        </div>
      </SheetContent>
    </Sheet>
  );
}

function OverviewPanel({
  accountId,
  poolId,
  stats,
  models,
  quota,
  account,
  codexQuota,
  routeSummary,
  onAccountUpdated,
}: {
  accountId: number;
  poolId?: number;
  stats: AccountStats;
  models: string[];
  quota: string;
  account: Account;
  codexQuota: CodexQuota | null;
  routeSummary: RouteSummary;
  onAccountUpdated?: () => void;
}) {
  const [latencyState, setLatencyState] = useState<
    { status: "loading" } | { status: "ready"; p50: number | null; p95: number | null } | { status: "empty" } | { status: "error" }
  >({ status: "loading" });

  useEffect(() => {
    let cancelled = false;
    setLatencyState({ status: "loading" });
    const poolParam = poolId ? `&pool=${poolId}` : "";
    api
      .get<LatencyBucket[]>(
        `/usage/latency?period=24h&bucket=1d&account=${accountId}${poolParam}`,
      )
      .then((buckets) => {
        if (cancelled) return;
        if (!buckets || buckets.length === 0) {
          setLatencyState({ status: "empty" });
          return;
        }
        const last = buckets[buckets.length - 1];
        setLatencyState({
          status: "ready",
          p50: last.p50 ?? null,
          p95: last.p95 ?? null,
        });
      })
      .catch(() => {
        if (!cancelled) setLatencyState({ status: "error" });
      });
    return () => {
      cancelled = true;
    };
  }, [accountId, poolId]);

  const latencyLabel =
    latencyState.status === "ready" && latencyState.p50 != null
      ? latency(latencyState.p50)
      : latencyState.status === "loading"
        ? "…"
        : "—";
  const latencyHint =
    latencyState.status === "ready" && latencyState.p95 != null
      ? `P95 ${latency(latencyState.p95)}`
      : latencyState.status === "empty"
        ? "24h 无样本"
        : latencyState.status === "error"
          ? "读取失败"
          : "读取中";

  const successLabel =
    stats.requests > 0 && stats.successRate != null
      ? formatSuccessRate(stats.successRate)
      : "—";
  const successTone: Tone =
    stats.successRate == null || stats.requests === 0
      ? "default"
      : stats.successRate >= 0.99
        ? "success"
        : stats.successRate >= 0.95
          ? "default"
          : stats.successRate >= 0.8
            ? "warning"
            : "danger";
  const quotaError = getCpaQuotaErrorMeta(account);

  return (
    <div className="space-y-6">
      <RouteSummaryPanel summary={routeSummary} compact />

      {account.source_kind === "openai_compat" ? (
        <DirectConnectionSection account={account} onAccountUpdated={onAccountUpdated} />
      ) : null}

      <div className="grid gap-3 sm:grid-cols-3">
        <Meter label="今日请求" value={compact(stats.requests)} hint={`${stats.requests} 次`} />
        <Meter label="成功率" value={successLabel} hint={`24h 窗口`} tone={successTone} />
        <Meter
          label="P50 延迟"
          value={latencyLabel}
          hint={latencyHint}
          tone={latencyState.status === "error" ? "warning" : "default"}
        />
      </div>

      <section className="space-y-3">
        <div className="flex items-baseline justify-between gap-2">
          <p className="text-[11px] uppercase tracking-[0.18em] text-moon-400">Models</p>
          <p className="text-[11px] text-moon-400">{models.length} 个</p>
        </div>
        {models.length === 0 ? (
          <div className="rounded-[1.1rem] border border-dashed border-moon-200/65 bg-white/40 px-4 py-6 text-center text-sm text-moon-400">
            当前没有发现可用模型
          </div>
        ) : (
          <div className="flex flex-wrap gap-1.5">
            {models.map((model) => (
              <span
                key={model}
                className="rounded-full bg-moon-100/70 px-2.5 py-1 text-[11px] text-moon-600"
                title={model}
              >
                {model}
              </span>
            ))}
          </div>
        )}
      </section>

      {codexQuota ? (
        <div className="space-y-2">
          <CodexQuotaBarsFull
            quota={codexQuota}
            fetchedAt={account.codex_quota_fetched_at}
            planType={account.cpa_plan_type}
          />
          {quotaError ? (
            <p
              className={cn(
                "px-1 text-xs",
                quotaError.tone === "danger" ? "text-status-red" : "text-status-yellow",
              )}
            >
              {quotaError.label}：{quotaError.detail}
            </p>
          ) : null}
        </div>
      ) : account.cpa_provider === "codex" ? (
        <section className="space-y-2.5 rounded-[1.2rem] border border-moon-200/55 bg-white/60 px-4 py-4">
          <p className="text-[11px] uppercase tracking-[0.18em] text-moon-400">Quota</p>
          <CodexQuotaBarsPendingCompact />
          <p
            className={cn(
              "text-xs",
              quotaError?.tone === "danger"
                ? "text-status-red"
                : quotaError?.tone === "warning"
                  ? "text-status-yellow"
                  : "text-moon-400",
            )}
          >
            {quotaError
              ? `${quotaError.label}：${quotaError.detail}`
              : "额度快照正在同步，完成后会自动填充。"}
          </p>
        </section>
      ) : (
        <section className="space-y-2.5 rounded-[1.2rem] border border-moon-200/55 bg-white/60 px-4 py-4">
          <p className="text-[11px] uppercase tracking-[0.18em] text-moon-400">Quota</p>
          <p className="text-sm text-moon-700">{quota}</p>
        </section>
      )}

      <ProbeConfigSection accountId={account.id} account={account} availableModels={models} />
    </div>
  );
}

function DirectConnectionSection({
  account,
  onAccountUpdated,
}: {
  account: Account;
  onAccountUpdated?: () => void;
}) {
  const [baseUrl, setBaseUrl] = useState(account.base_url ?? "");
  const [apiKey, setApiKey] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setBaseUrl(account.base_url ?? "");
    setApiKey("");
    setError(null);
  }, [account.id, account.base_url, account.api_key_masked]);

  const trimmedBaseUrl = baseUrl.trim();
  const trimmedApiKey = apiKey.trim();
  const changed = trimmedBaseUrl !== (account.base_url ?? "").trim() || trimmedApiKey.length > 0;
  const canSave = changed && trimmedBaseUrl.length > 0 && !saving;

  function reset() {
    setBaseUrl(account.base_url ?? "");
    setApiKey("");
    setError(null);
  }

  async function save() {
    if (!canSave) return;
    setSaving(true);
    setError(null);
    try {
      const payload = {
        label: account.label,
        source_kind: account.source_kind,
        base_url: trimmedBaseUrl,
        api_key: trimmedApiKey,
        provider: account.provider,
        enabled: account.enabled,
        notes: account.notes,
        quota_display: account.quota_display,
      };
      await api.put(`/accounts/${account.id}`, payload);
      toast("连接信息已保存");
      setApiKey("");
      onAccountUpdated?.();
    } catch (err) {
      const message = err instanceof Error ? err.message : "连接信息保存失败";
      setError(message);
      toast(message, "error");
    } finally {
      setSaving(false);
    }
  }

  return (
    <section className="space-y-4 rounded-[1.15rem] border border-moon-200/55 bg-white/62 px-4 py-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 space-y-1.5">
          <p className="text-[11px] uppercase tracking-[0.18em] text-moon-400">Connection</p>
          <p className="text-sm leading-6 text-moon-500">
            调整直连账号的 API 地址和 token。留空 token 会保留当前凭据。
          </p>
        </div>
        <span className="rounded-full bg-moon-100/75 px-2.5 py-1 text-[11px] text-moon-500">
          {account.api_key_set ? account.api_key_masked : "Token missing"}
        </span>
      </div>

      <div className="grid gap-4">
        <label className="space-y-2">
          <span className="text-[11px] uppercase tracking-[0.18em] text-moon-400">API Base URL</span>
          <Input
            value={baseUrl}
            onChange={(event) => setBaseUrl(event.target.value)}
            disabled={saving}
            placeholder="https://api.openai.com/v1"
            className="h-10 bg-white/76 text-sm"
          />
        </label>
        <label className="space-y-2">
          <span className="text-[11px] uppercase tracking-[0.18em] text-moon-400">API Token</span>
          <Input
            value={apiKey}
            onChange={(event) => setApiKey(event.target.value)}
            disabled={saving}
            type="password"
            placeholder="Paste a new token only when replacing it"
            className="h-10 bg-white/76 text-sm"
            autoComplete="off"
          />
        </label>
      </div>

      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-[11px] text-moon-400">
          保存后会刷新账号数据；可用模型和健康状态可继续用刷新或自检确认。
        </p>
        <div className="flex items-center gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={reset}
            disabled={!changed || saving}
            className="rounded-full bg-white/50"
          >
            <RotateCcw className="size-3.5" />
            重置
          </Button>
          <Button type="button" size="sm" onClick={save} disabled={!canSave}>
            {saving ? <Loader2 className="size-3.5 animate-spin" /> : <Save className="size-3.5" />}
            保存
          </Button>
        </div>
      </div>
      {error ? <p className="text-xs text-status-red">{error}</p> : null}
    </section>
  );
}

// ProbeConfigSection edits `account.probe_models` — the list of models the
// Pool-detail self-check button will try. Empty list means "fall back to the
// latest discovered model" on the client. Changes persist immediately via the
// /probe-models endpoint; there's no separate save button because the chip
// input already has explicit add/remove actions.
function ProbeConfigSection({
  accountId,
  account,
  availableModels,
}: {
  accountId: number;
  account: Account;
  availableModels: string[];
}) {
  const [models, setModels] = useState<string[]>(() => ensureArray(account.probe_models));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setModels(ensureArray(account.probe_models));
  }, [account.probe_models, accountId]);

  async function persist(next: string[]) {
    // Snapshot pre-optimistic state so a failed PUT can roll back the chip
    // list — otherwise the UI happily shows models the server never accepted
    // and the next parent refresh would silently overwrite them with the
    // real (unchanged) values, looking like a confusing ghost edit.
    const prev = models;
    setModels(next);
    setSaving(true);
    setError(null);
    try {
      await api.put(`/accounts/${accountId}/probe-models`, { models: next });
    } catch (err) {
      setModels(prev);
      setError(err instanceof Error ? err.message : "保存失败");
    } finally {
      setSaving(false);
    }
  }

  return (
    <section className="space-y-2.5 rounded-[1.2rem] border border-moon-200/55 bg-white/60 px-4 py-4">
      <div className="flex items-baseline justify-between gap-2">
        <p className="text-[11px] uppercase tracking-[0.18em] text-moon-400">自检配置</p>
        <p className="text-[11px] text-moon-400">
          {saving ? "保存中…" : models.length > 0 ? `${models.length} 个模型` : "默认最新模型"}
        </p>
      </div>
      <ProbeModelChipPicker
        value={models}
        available={availableModels}
        onChange={persist}
      />
      <p className="text-[11px] text-moon-400">
        Pool 详情页的“自检”按钮会按列表顺序测试，只要有一个通过就记为健康；留空时会自动选择默认最新模型。
      </p>
      {error ? <p className="text-xs text-status-red">{error}</p> : null}
    </section>
  );
}

function PlaygroundPanel({
  accountId,
  models,
  disabled,
  resolveToken,
}: {
  accountId: number;
  models: string[];
  disabled: boolean;
  resolveToken: () => Promise<string>;
}) {
  const [selectedModel, setSelectedModel] = useState<string | undefined>(models[0]);
  const [message, setMessage] = useState(PRESET_MESSAGES[0].value);
  const [reply, setReply] = useState("");
  const [usageText, setUsageText] = useState("");
  const [durationMs, setDurationMs] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [lastCurl, setLastCurl] = useState<string | null>(null);

  useEffect(() => {
    if (!selectedModel && models.length > 0) {
      setSelectedModel(models[0]);
    }
  }, [models, selectedModel]);

  async function runTest(custom?: string) {
    const content = (custom ?? message).trim();
    if (!content || !selectedModel || loading || disabled) return;
    setLoading(true);
    setError(null);
    setReply("");
    setUsageText("");
    setDurationMs(null);
    setLastCurl(null);

    let token = "";
    try {
      token = await resolveToken();
      if (!token) {
        throw new Error("未找到可用的 Token");
      }
      const started = performance.now();
      const res = await fetch("/v1/chat/completions", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
          "X-Lune-Account-Id": String(accountId),
        },
        body: JSON.stringify({
          model: selectedModel,
          messages: [{ role: "user", content }],
          stream: false,
        }),
      });
      const elapsed = performance.now() - started;
      const data = (await res.json().catch(() => null)) as
        | {
            choices?: Array<{ message?: { content?: string } }>;
            usage?: { prompt_tokens?: number; completion_tokens?: number; total_tokens?: number };
            error?: { message?: string };
          }
        | null;

      if (!res.ok) {
        const errMsg = data?.error?.message ?? `请求失败 (${res.status})`;
        throw new Error(errMsg);
      }

      setReply(data?.choices?.[0]?.message?.content ?? "");
      const u = data?.usage;
      if (u) {
        setUsageText(
          `${u.prompt_tokens ?? 0} in · ${u.completion_tokens ?? 0} out · ${u.total_tokens ?? 0} total`,
        );
      }
      setDurationMs(elapsed);
    } catch (err) {
      setError(err instanceof Error ? err.message : "测试失败");
      const origin = window.location.origin;
      const maskedToken = token ? `${token.slice(0, 6)}…` : "<TOKEN>";
      const bodyJson = JSON.stringify({
        model: selectedModel ?? "",
        messages: [{ role: "user", content }],
      });
      // Wrap the body in shell single quotes; escape any embedded ' as '\''
      const shellSafeBody = bodyJson.replace(/'/g, "'\\''");
      const curl = `curl ${origin}/v1/chat/completions \\
  -H 'Authorization: Bearer ${maskedToken}' \\
  -H 'X-Lune-Account-Id: ${accountId}' \\
  -H 'Content-Type: application/json' \\
  -d '${shellSafeBody}'`;
      setLastCurl(curl);
    } finally {
      setLoading(false);
    }
  }

  const hasModel = Boolean(selectedModel);
  const canSend = !loading && !disabled && hasModel && message.trim().length > 0;

  return (
    <div className="space-y-5">
      <section className="space-y-3">
        <div className="flex items-center justify-between gap-3">
          <p className="text-[11px] uppercase tracking-[0.18em] text-moon-400">Model</p>
          <p className="text-[11px] text-moon-400">{models.length} 个可选</p>
        </div>
        {models.length === 0 ? (
          <p className="rounded-[1rem] border border-dashed border-moon-200/65 bg-white/40 px-4 py-4 text-sm text-moon-400">
            当前账号没有模型，无法直测。
          </p>
        ) : (
          <Select
            value={selectedModel}
            onValueChange={(value) => setSelectedModel(value ?? undefined)}
          >
            <SelectTrigger className="w-full">
              <SelectValue placeholder="选择模型" />
            </SelectTrigger>
            <SelectContent>
              {models.map((model) => (
                <SelectItem key={model} value={model}>
                  {model}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      </section>

      <section className="space-y-3">
        <p className="text-[11px] uppercase tracking-[0.18em] text-moon-400">Message</p>
        <div className="flex flex-wrap gap-2">
          {PRESET_MESSAGES.map((preset) => {
            const presetDisabled = loading || disabled;
            return (
              <button
                key={preset.label}
                type="button"
                onClick={() => setMessage(preset.value)}
                disabled={presetDisabled}
                className={cn(
                  "rounded-full border border-moon-200/55 bg-white/75 px-3 py-1.5 text-[12.5px] text-moon-600 transition-colors hover:bg-white hover:text-moon-800",
                  presetDisabled &&
                    "cursor-not-allowed opacity-55 hover:bg-white/75 hover:text-moon-600",
                )}
              >
                {preset.label}
              </button>
            );
          })}
        </div>
        <textarea
          value={message}
          onChange={(event) => setMessage(event.target.value)}
          disabled={disabled}
          className="min-h-20 w-full rounded-[1rem] border border-moon-200/60 bg-white/78 px-3 py-3 text-sm text-moon-700 outline-none ring-0 transition focus:border-lunar-300 disabled:opacity-55"
        />
        <div className="flex items-center justify-end">
          <Button onClick={() => runTest()} disabled={!canSend}>
            {loading ? <Loader2 className="size-4 animate-spin" /> : <SendHorizonal className="size-4" />}
            {loading ? "测试中" : "发送"}
          </Button>
        </div>
      </section>

      <section className="space-y-3">
        <div className="flex items-center justify-between gap-2">
          <p className="text-[11px] uppercase tracking-[0.18em] text-moon-400">Response</p>
          <p className="text-[11px] text-moon-400">
            {[selectedModel, durationMs != null ? latency(durationMs) : null, usageText]
              .filter(Boolean)
              .join(" · ") || "等待响应"}
          </p>
        </div>
        {error ? (
          <div className="space-y-2 rounded-[1rem] border border-status-red/30 bg-red-50/70 px-4 py-3 text-sm text-status-red">
            <p className="font-medium">{error}</p>
            {lastCurl ? (
              <div className="flex items-center justify-between gap-2">
                <p className="truncate text-[11px] text-moon-500">curl 命令已生成，可定位底层问题</p>
                <CopyButton value={lastCurl} label="复制 curl" />
              </div>
            ) : null}
          </div>
        ) : reply ? (
          <div className="space-y-2 rounded-[1rem] border border-moon-200/55 bg-white/70 px-4 py-3">
            <p className="whitespace-pre-wrap text-sm leading-6 text-moon-700">{reply}</p>
          </div>
        ) : (
          <p className="rounded-[1rem] border border-dashed border-moon-200/60 bg-white/40 px-4 py-4 text-sm text-moon-400">
            选一条预置或写你的问题，发送后这里会显示回复。
          </p>
        )}
      </section>
    </div>
  );
}

function DiagnosticPanel({
  account,
  accountId,
  baseUrl,
  routeSummary,
  discoveryHealth,
  isCodexCpa,
}: {
  account: Account;
  accountId: number;
  baseUrl: string;
  routeSummary: RouteSummary;
  discoveryHealth: string;
  isCodexCpa: boolean;
}) {
  const dimensions = buildDiagnosticDimensions(account, discoveryHealth, isCodexCpa);

  return (
    <div className="space-y-5 text-sm">
      <RouteSummaryPanel summary={routeSummary} />

      {dimensions.map((dimension) => (
        <DiagnosticSection key={dimension.title} title={dimension.title}>
          <DiagnosticRow label="当前状态" value={dimension.status} tone={dimension.tone} />
          <DiagnosticRow label="最近检查" value={dimension.checkedAt} />
          <DiagnosticRow
            label="原因 / 最近错误"
            value={dimension.reason}
            tone={dimension.tone === "danger" ? "danger" : "default"}
            breakAll
          />
          <DiagnosticRow label="建议操作" value={dimension.action} />
        </DiagnosticSection>
      ))}

      <details className="rounded-[1.15rem] border border-moon-200/55 bg-white/50 px-4 py-3">
        <summary className="cursor-pointer text-[11px] uppercase tracking-[0.18em] text-moon-400">
          高级信息
        </summary>
        <div className="mt-4 space-y-3">
          <DebugRow label="Account ID" value={String(accountId)} copyable />
          <DebugRow label="Source Kind" value={account.source_kind} />
          <DebugRow label="Runtime Base URL" value={baseUrl} copyable breakAll />
          {account.source_kind === "openai_compat" ? (
            <DebugRow
              label="API Key"
              value={account.api_key_set ? account.api_key_masked : "Missing"}
            />
          ) : null}
          <DebugRow label="Discovery Health" value={discoveryHealth} />
          <DebugRow label="Route Badge" value={routeSummary.label} />
          <DebugRow label="Serving Status" value={account.serving_status || "healthy"} />
          <DebugRow label="Failure Count" value={String(account.failure_count ?? 0)} />
          <DebugRow label="Cooldown Until" value={account.cooldown_until || "--"} />
          <DebugRow label="Last Checked" value={relativeTime(account.last_checked_at ?? null) || "--"} />
          <DebugRow
            label="Last Error"
            value={account.last_error || "--"}
            tone={account.last_error ? "danger" : "default"}
            breakAll
          />
          {account.source_kind === "cpa" ? (
            <>
              <DebugRow label="CPA Account Key" value={account.cpa_account_key || "--"} copyable breakAll />
              <DebugRow label="Provider" value={account.cpa_provider || "--"} />
              <DebugRow label="Raw Credential Status" value={account.cpa_credential_status || "--"} />
              <DebugRow label="Raw Credential Reason" value={account.cpa_credential_reason || "--"} />
              <DebugRow label="Raw Quota Status" value={account.cpa_quota_status || "unknown"} />
              <DebugRow label="Raw Subscription Status" value={account.cpa_subscription_status || "unknown"} />
            </>
          ) : null}
        </div>
      </details>
    </div>
  );
}

function RouteSummaryPanel({
  summary,
  compact = false,
}: {
  summary: RouteSummary;
  compact?: boolean;
}) {
  return (
    <section className="rounded-[1.15rem] border border-moon-200/55 bg-white/62 px-4 py-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 space-y-1.5">
          <p className="text-[11px] uppercase tracking-[0.18em] text-moon-400">Route</p>
          <p className="text-base font-semibold tracking-[-0.01em] text-moon-800">
            {summary.label} · {summary.reason}
          </p>
          <p className="text-sm leading-6 text-moon-500">{summary.impact}</p>
        </div>
        <StatusBadge status={summary.status} />
      </div>
      {!compact && summary.actions.length > 0 ? (
        <div className="mt-4 flex flex-wrap gap-2">
          {summary.actions.map((action) => (
            <span
              key={action}
              className="rounded-full bg-moon-100/75 px-2.5 py-1 text-[11px] text-moon-500"
            >
              {action}
            </span>
          ))}
        </div>
      ) : null}
    </section>
  );
}

type DiagnosticDimension = {
  title: string;
  status: string;
  checkedAt: string;
  reason: string;
  action: string;
  tone?: "default" | "warning" | "danger";
};

function buildDiagnosticDimensions(
  account: Account,
  discoveryHealth: string,
  isCodexCpa: boolean,
): DiagnosticDimension[] {
  if (account.source_kind !== "cpa") {
    return [
      connectionDimension(account),
      credentialDimension(account),
      routeDimension(account, discoveryHealth),
      servingDimension(account, discoveryHealth),
      modelsDimension(account),
    ];
  }
  return [
    runtimeBindingDimension(account),
    credentialDimension(account),
    ...(isCodexCpa ? [subscriptionDimension(account)] : []),
    quotaDimension(account),
    servingDimension(account, discoveryHealth),
  ];
}

function connectionDimension(account: Account): DiagnosticDimension {
  return {
    title: "Connection",
    status: account.base_url || "--",
    checkedAt: relativeTime(account.last_checked_at ?? null) || "--",
    reason: account.base_url
      ? "当前请求会直接发送到这个地址。"
      : "未配置 API Base URL。",
    action: account.base_url ? "保持当前连接配置" : "补充 API Base URL",
    tone: account.base_url ? "default" : "warning",
  };
}

function runtimeBindingDimension(account: Account): DiagnosticDimension {
  if (account.source_kind !== "cpa") {
    return {
      title: "Runtime Binding",
      status: "直连 Bearer",
      checkedAt: relativeTime(account.last_checked_at ?? null) || "--",
      reason: "直连账号不经过 CPA runtime auth file 绑定。",
      action: "保持当前 API key 配置",
    };
  }
  if (hasRuntimeBindingIssue(account)) {
    return {
      title: "Runtime Binding",
      status: "未确认",
      checkedAt: relativeTime(account.last_checked_at ?? null) || "--",
      reason: "CPA HTTP provider 当前无法验证 per-request auth pinning。",
      action: "检查 CPA runtime，并在 provider pinning 支持后再接普通流量",
      tone: "danger",
    };
  }
  return {
    title: "Runtime Binding",
    status: "已确认",
    checkedAt: relativeTime(account.last_checked_at ?? null) || "--",
    reason: "Lune 可确认请求使用目标 CPA auth file。",
    action: "保持监控",
  };
}

function credentialDimension(account: Account): DiagnosticDimension {
  if (account.source_kind !== "cpa") {
    return {
      title: "Credential",
      status: account.api_key_set ? "API key 已配置" : "API key 缺失",
      checkedAt: relativeTime(account.last_checked_at ?? null) || "--",
      reason: account.api_key_set ? "直连凭据由用户提供。" : "没有可用 API key。",
      action: account.api_key_set ? "保持监控" : "补充 API key",
      tone: account.api_key_set ? "default" : "danger",
    };
  }
  const meta = getCpaCredentialMeta(account);
  const status = account.cpa_credential_status || "unknown";
  const healthy = status === "ok" || status === "auth_suspect";
  return {
    title: "凭据",
    status: meta?.label || (status === "ok" ? "凭据正常" : status),
    checkedAt: relativeTime(account.cpa_credential_checked_at ?? null) || "--",
    reason: meta?.detail || account.cpa_credential_reason || "没有记录到凭据异常。",
    action:
      status === "needs_login" || status === "refresh_failed"
        ? "重新登录"
        : status === "runtime_error"
          ? "检查 CPA runtime"
          : status === "runtime_pending"
            ? "稍后刷新状态"
            : status === "auth_suspect"
              ? "运行直测确认"
              : "保持监控",
    tone: healthy ? (status === "auth_suspect" ? "warning" : "default") : "danger",
  };
}

function routeDimension(account: Account, discoveryHealth: string): DiagnosticDimension {
  const route = getRouteSummary(account, true, discoveryHealth);
  return {
    title: "Route",
    status: route.label,
    checkedAt: relativeTime(account.last_checked_at ?? null) || "--",
    reason: route.reason,
    action: route.actions[0] || "保持监控",
    tone: route.status === "error" ? "danger" : route.status === "degraded" ? "warning" : "default",
  };
}

function subscriptionDimension(account: Account): DiagnosticDimension {
  const status = account.cpa_subscription_status || "unknown";
  const active = status === "active";
  return {
    title: "订阅",
    status: subscriptionStatusLabel(status),
    checkedAt: relativeTime(account.cpa_subscription_fetched_at ?? null) || "--",
    reason: account.cpa_subscription_last_error || account.cpa_subscription_expires_at || "订阅快照可用。",
    action: active ? "保持监控" : status === "pending" ? "稍后刷新订阅" : "刷新订阅",
    tone: active ? "default" : status === "pending" || status === "error" || status === "unknown" ? "warning" : "danger",
  };
}

function quotaDimension(account: Account): DiagnosticDimension {
  if (account.source_kind !== "cpa") {
    return {
      title: "额度",
      status: parseQuotaDisplay(account.quota_display ?? ""),
      checkedAt: relativeTime(account.last_checked_at ?? null) || "--",
      reason: "直连账号使用用户维护的额度展示。",
      action: "需要时更新账号备注或额度展示",
    };
  }
  const meta = getCpaQuotaErrorMeta(account);
  const status = account.cpa_quota_status || "unknown";
  return {
    title: "额度",
    status: meta?.label || quotaStatusLabel(status),
    checkedAt: relativeTime(account.cpa_quota_checked_at ?? null) || "--",
    reason:
      meta?.detail ||
      (account.codex_quota_json ? "已保存最近一次额度快照。" : "没有可用额度快照。"),
    action: status === "blocked" ? "更换账号或等待额度恢复" : "刷新额度",
    tone: status === "blocked" ? "danger" : status === "error" || status === "unknown" ? "warning" : "default",
  };
}

function servingDimension(account: Account, discoveryHealth: string): DiagnosticDimension {
  const status = account.serving_status || "healthy";
  return {
    title: "Serving",
    status: servingStatusLabel(status),
    checkedAt: relativeTime(account.last_checked_at ?? null) || "--",
    reason: account.last_error || `模型发现状态：${discoveryHealth}`,
    action: status === "healthy" ? "保持监控" : status === "cooldown" ? "等待冷却结束或运行自检" : "运行自检并查看最近错误",
    tone: status === "error" ? "danger" : status === "cooldown" ? "warning" : "default",
  };
}

function modelsDimension(account: Account): DiagnosticDimension {
  const count = account.models?.length ?? 0;
  return {
    title: "Models",
    status: count > 0 ? `${count} 个模型` : "暂无模型",
    checkedAt: relativeTime(account.last_checked_at ?? null) || "--",
    reason: count > 0 ? account.models.join(", ") : "尚未发现可路由模型。",
    action: count > 0 ? "保持监控" : "刷新模型",
  };
}

function subscriptionStatusLabel(status: string): string {
  switch (status) {
    case "active":
      return "订阅有效";
    case "expired":
      return "已过期";
    case "free":
      return "Free";
    case "pending":
      return "订阅刷新中";
    case "error":
      return "订阅获取失败";
    default:
      return "订阅未知";
  }
}

function quotaStatusLabel(status: string): string {
  switch (status) {
    case "ok":
      return "额度可用";
    case "blocked":
      return "额度已用尽";
    case "pending":
      return "额度刷新中";
    case "error":
      return "额度查询失败";
    default:
      return "额度未知";
  }
}

function servingStatusLabel(status: string): string {
  switch (status) {
    case "cooldown":
      return "服务冷却中";
    case "error":
      return "服务异常";
    case "unknown":
      return "服务状态未知";
    default:
      return "服务正常";
  }
}

function DiagnosticSection({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="rounded-[1.15rem] border border-moon-200/55 bg-white/60 px-4 py-4">
      <p className="mb-3 text-[11px] uppercase tracking-[0.18em] text-moon-400">{title}</p>
      <div className="space-y-3">{children}</div>
    </section>
  );
}

function DiagnosticRow({
  label,
  value,
  tone = "default",
  breakAll,
}: {
  label: string;
  value: string;
  tone?: "default" | "warning" | "danger";
  breakAll?: boolean;
}) {
  return (
    <div className="grid gap-1.5 border-b border-moon-200/45 pb-3 last:border-b-0 sm:grid-cols-[7rem_1fr] sm:gap-4">
      <p className="text-[11px] uppercase tracking-[0.18em] text-moon-400">{label}</p>
      <p
        className={cn(
          "text-sm leading-6",
          tone === "danger"
            ? "text-status-red"
            : tone === "warning"
              ? "text-status-yellow"
              : "text-moon-700",
          breakAll ? "break-all" : "",
        )}
      >
        {value || "--"}
      </p>
    </div>
  );
}

function DebugRow({
  label,
  value,
  tone = "default",
  breakAll,
  copyable,
}: {
  label: string;
  value: string;
  tone?: "default" | "danger";
  breakAll?: boolean;
  copyable?: boolean;
}) {
  return (
    <div className="space-y-1.5 border-b border-moon-200/45 pb-3 last:border-b-0">
      <div className="flex items-center justify-between gap-3">
        <p className="text-[11px] uppercase tracking-[0.18em] text-moon-400">{label}</p>
        {copyable ? <CopyButton value={value} label="复制" /> : null}
      </div>
      <p
        className={cn(
          "text-sm leading-6",
          tone === "danger" ? "text-status-red" : "text-moon-700",
          breakAll ? "break-all" : "",
        )}
      >
        {value}
      </p>
    </div>
  );
}

function CopyButton({ value, label = "复制" }: { value: string; label?: string }) {
  const [copied, setCopied] = useState(false);
  const timerRef = useRef<number | null>(null);
  useEffect(() => {
    return () => {
      if (timerRef.current) window.clearTimeout(timerRef.current);
    };
  }, []);

  async function handleCopy() {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      if (timerRef.current) window.clearTimeout(timerRef.current);
      timerRef.current = window.setTimeout(() => setCopied(false), 1400);
    } catch {
      setCopied(false);
    }
  }

  return (
    <button
      type="button"
      onClick={handleCopy}
      className="inline-flex items-center gap-1 text-xs text-moon-400 transition-colors hover:text-moon-700"
    >
      {copied ? <Check className="size-3.5 text-status-green" /> : <Copy className="size-3.5" />}
      <span>{copied ? "已复制" : label}</span>
    </button>
  );
}

function Meter({
  label,
  value,
  hint,
  tone = "default",
}: {
  label: string;
  value: string;
  hint?: string;
  tone?: Tone;
}) {
  const toneClass = useMemo(() => {
    switch (tone) {
      case "success":
        return "border-status-green/25 bg-[linear-gradient(180deg,rgba(231,247,235,0.72),rgba(245,250,247,0.68))] text-status-green";
      case "warning":
        return "border-status-yellow/30 bg-[linear-gradient(180deg,rgba(252,245,224,0.75),rgba(250,247,238,0.68))] text-status-yellow";
      case "danger":
        return "border-status-red/25 bg-[linear-gradient(180deg,rgba(252,231,230,0.72),rgba(250,243,243,0.68))] text-status-red";
      default:
        return "border-moon-200/60 bg-white/70 text-moon-700";
    }
  }, [tone]);

  return (
    <div className={cn("rounded-[1.15rem] border px-4 py-3.5", toneClass)}>
      <p className="text-[10.5px] uppercase tracking-[0.18em] text-moon-400">{label}</p>
      <p className="mt-1.5 font-editorial text-[1.4rem] font-semibold tabular-nums tracking-[-0.01em]">
        {value}
      </p>
      {hint ? <p className="mt-0.5 text-[11px] text-moon-400">{hint}</p> : null}
    </div>
  );
}

function formatSuccessRate(value: number): string {
  const percent = value * 100;
  if (percent >= 99.95) return "100%";
  return `${percent.toFixed(1).replace(/\.0$/, "")}%`;
}
