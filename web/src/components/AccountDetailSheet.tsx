import { useEffect, useMemo, useRef, useState } from "react";
import { Check, Clock3, Copy, Loader2, SendHorizonal } from "lucide-react";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsIndicator, TabsPanel, TabsTab } from "@/components/ui/tabs";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import StatusBadge from "@/components/StatusBadge";
import { CodexQuotaBarsFull } from "@/components/CodexQuotaBars";
import ProbeModelChipPicker from "@/components/ProbeModelChipPicker";
import { api } from "@/lib/api";
import { parseCodexQuota, type CodexQuota } from "@/lib/codexQuota";
import { compact, latency, relativeTime } from "@/lib/fmt";
import {
  ensureArray,
  getAccessLabel,
  getAccountHealth,
  getCpaCredentialMeta,
  getCpaSubscriptionErrorMeta,
  getExpiryMeta,
  parseQuotaDisplay,
} from "@/lib/lune";
import type { Account, LatencyBucket, PoolMember } from "@/lib/types";
import { cn } from "@/lib/utils";

type Tone = "default" | "success" | "warning" | "danger";

type TabKey = "overview" | "playground" | "debug";

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
  onOpenChange,
}: {
  member: PoolMember | null;
  stats: AccountStats;
  priorityIndex?: number;
  poolId?: number;
  resolveToken: () => Promise<string>;
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
              <StatusBadge status={health === "unknown" ? "degraded" : health} />
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
                <TabsTab value="debug">Debug</TabsTab>
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
              <TabsPanel value="debug">
                <DebugPanel
                  accountId={account.id}
                  baseUrl={account.runtime?.base_url || account.base_url || "--"}
                  subscriptionExpiry={isCodexCpa ? account.cpa_subscription_expires_at ?? null : null}
                  subscriptionLastError={isCodexCpa ? account.cpa_subscription_last_error ?? "" : ""}
                  credentialExpiry={account.cpa_expired_at ?? null}
                  credentialStatus={
                    account.source_kind === "cpa" ? account.cpa_credential_status ?? "unknown" : null
                  }
                  credentialReason={account.source_kind === "cpa" ? account.cpa_credential_reason ?? "" : ""}
                  credentialLastError={
                    account.source_kind === "cpa" ? account.cpa_credential_last_error ?? "" : ""
                  }
                  credentialCheckedAt={
                    account.source_kind === "cpa" ? account.cpa_credential_checked_at ?? null : null
                  }
                  lastCheckedAt={account.last_checked_at ?? null}
                  lastError={account.last_error ?? null}
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
}: {
  accountId: number;
  poolId?: number;
  stats: AccountStats;
  models: string[];
  quota: string;
  account: Account;
  codexQuota: CodexQuota | null;
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

  return (
    <div className="space-y-6">
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
        <CodexQuotaBarsFull
          quota={codexQuota}
          fetchedAt={account.codex_quota_fetched_at}
          planType={account.cpa_plan_type}
        />
      ) : (
        <section className="space-y-2.5 rounded-[1.2rem] border border-moon-200/55 bg-white/60 px-4 py-4">
          <p className="text-[11px] uppercase tracking-[0.18em] text-moon-400">Quota</p>
          <p className="text-sm text-moon-700">{quota}</p>
          {account.cpa_provider === "codex" ? (
            <p className="text-xs text-moon-400">
              CPA 尚未返回配额快照，下次健康检查后会自动填充。
            </p>
          ) : null}
        </section>
      )}

      <ProbeConfigSection accountId={account.id} account={account} availableModels={models} />
    </div>
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

function DebugPanel({
  accountId,
  baseUrl,
  subscriptionExpiry,
  subscriptionLastError,
  credentialExpiry,
  credentialStatus,
  credentialReason,
  credentialLastError,
  credentialCheckedAt,
  lastCheckedAt,
  lastError,
}: {
  accountId: number;
  baseUrl: string;
  subscriptionExpiry: string | null;
  subscriptionLastError: string;
  credentialExpiry: string | null;
  credentialStatus: string | null;
  credentialReason: string;
  credentialLastError: string;
  credentialCheckedAt: string | null;
  lastCheckedAt: string | null;
  lastError: string | null;
}) {
  return (
    <div className="space-y-4 text-sm">
      <DebugRow label="Account ID" value={String(accountId)} copyable />
      <DebugRow label="Runtime Base URL" value={baseUrl} copyable breakAll />
      {subscriptionExpiry ? (
        <DebugRow
          label="ChatGPT Subscription Expires"
          value={`${subscriptionExpiry} · ${relativeTime(subscriptionExpiry)}`}
        />
      ) : null}
      {subscriptionLastError ? (
        <DebugRow
          label="ChatGPT Subscription Error"
          value={subscriptionLastError}
          tone="danger"
          breakAll
        />
      ) : null}
      {credentialExpiry ? (
        <DebugRow
          label="CPA Token Expires"
          value={`${credentialExpiry} · ${relativeTime(credentialExpiry)}`}
        />
      ) : null}
      {credentialStatus ? (
        <>
          <DebugRow label="CPA Credential Status" value={credentialStatus} />
          {credentialReason ? <DebugRow label="CPA Credential Reason" value={credentialReason} /> : null}
          {credentialLastError ? (
            <DebugRow label="CPA Credential Error" value={credentialLastError} tone="danger" />
          ) : null}
          <DebugRow label="CPA Credential Checked" value={relativeTime(credentialCheckedAt) || "--"} />
        </>
      ) : null}
      <DebugRow label="Last Checked" value={relativeTime(lastCheckedAt) || "--"} />
      <DebugRow
        label="Last Error"
        value={lastError || "--"}
        tone={lastError ? "danger" : "default"}
        breakAll
      />
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
