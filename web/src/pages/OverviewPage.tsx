import { useEffect, useMemo, useRef, useState } from "react";
import { ArrowRight, Check, Copy, KeyRound, QrCode, RefreshCw } from "lucide-react";
import EmptyState from "@/components/EmptyState";
import EnvSnippetsDialog from "@/components/EnvSnippetsDialog";
import ErrorState from "@/components/ErrorState";
import PageHeader from "@/components/PageHeader";
import QrCodeDialog from "@/components/QrCodeDialog";
import SectionHeading from "@/components/SectionHeading";
import { useAdminUI } from "@/components/AdminUI";
import { toast } from "@/components/Feedback";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { api } from "@/lib/api";
import { compact } from "@/lib/fmt";
import { derivePoolSnapshot, getApiBaseUrl, type PoolSnapshot } from "@/lib/lune";
import { useRouter } from "@/lib/router";
import { cn } from "@/lib/utils";
import type {
  Account,
  Overview,
  Pool,
  RevealedAccessToken,
  UsageStats,
} from "@/lib/types";

type PoolActivity =
  | {
      status: "ready";
      requestsToday: number;
    }
  | {
      status: "error";
    };

function summarizeModels(models: string[]) {
  if (models.length === 0) {
    return "等待模型发现";
  }
  if (models.length === 1) {
    return models[0];
  }
  if (models.length === 2) {
    return `${models[0]} · ${models[1]}`;
  }
  return `${models[0]} · ${models[1]} · +${models.length - 2}`;
}

function isExpiringSoon(account: Account) {
  if (!account.enabled || !account.cpa_expired_at) {
    return false;
  }
  const expiresAt = new Date(account.cpa_expired_at).getTime();
  if (Number.isNaN(expiresAt)) {
    return false;
  }
  const diff = expiresAt - Date.now();
  const days = diff / (24 * 60 * 60 * 1000);
  return days >= 0 && days <= 7;
}

function getStatusSentence(overview: Overview | null, accounts: Account[], poolSnapshots: PoolSnapshot[]) {
  if (!overview) return "";
  const knownEnabledPools = poolSnapshots.filter((pool) => pool.enabled && pool.health !== "unknown");
  const pendingPools = poolSnapshots.filter((pool) => pool.enabled && pool.health === "unknown").length;
  const activeAccountIds = new Set(knownEnabledPools.flatMap((pool) => pool.activeAccountIds));
  const activeAccounts = accounts.filter((account) => activeAccountIds.has(account.id));
  const expiring = activeAccounts.filter((account) => isExpiringSoon(account)).length;
  const broken = activeAccounts.filter((account) => account.status === "error").length;
  const modelCount = new Set(knownEnabledPools.flatMap((pool) => pool.models)).size;
  const expiringText = expiring === 0 ? "无临近到期账号" : `${expiring} 个账号临近到期`;
  const brokenText = broken === 0 ? "" : ` · ${broken} 个异常账号`;
  const pendingText = pendingPools === 0 ? "" : ` · ${pendingPools} 个 Pool 待确认`;
  return `${modelCount} 个模型可用 · 今日 ${compact(overview.requests_today)} 次请求 · ${expiringText}${brokenText}${pendingText}`;
}

function GlobalAccessCopyAction({
  onCopy,
  ariaLabel,
  disabled,
}: {
  onCopy: () => Promise<void>;
  ariaLabel: string;
  disabled?: boolean;
}) {
  const [status, setStatus] = useState<"idle" | "success" | "error">("idle");
  const timerRef = useRef<number | null>(null);

  useEffect(() => {
    return () => {
      if (timerRef.current) {
        window.clearTimeout(timerRef.current);
      }
    };
  }, []);

  function scheduleReset(delay = 1500) {
    if (timerRef.current) {
      window.clearTimeout(timerRef.current);
    }
    timerRef.current = window.setTimeout(() => {
      setStatus("idle");
      timerRef.current = null;
    }, delay);
  }

  async function handleCopy() {
    if (disabled) {
      return;
    }
    try {
      await onCopy();
      setStatus("success");
      scheduleReset();
    } catch {
      setStatus("error");
      scheduleReset();
    }
  }

  const label = status === "success" ? "已复制" : "复制";
  const hint = status === "error" ? "复制失败，请重试" : null;

  return (
    <div className="relative flex shrink-0 items-center">
      <button
        type="button"
        onClick={handleCopy}
        aria-label={ariaLabel}
        title={ariaLabel}
        disabled={disabled}
        className={cn(
          "inline-flex h-8 min-w-[5.5rem] items-center justify-center gap-1.5 rounded-full px-3 text-xs font-medium transition-[background-color,border-color,color,transform,box-shadow] duration-150",
          "border border-moon-200/55 bg-white/72 text-moon-600 shadow-[inset_0_1px_0_rgba(255,255,255,0.72)]",
          "hover:border-moon-300/70 hover:bg-white hover:text-moon-800 active:scale-[0.985]",
          "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-lunar-300/55 focus-visible:ring-offset-2 focus-visible:ring-offset-white/80",
          disabled && "cursor-not-allowed border-moon-200/45 bg-white/40 text-moon-350 hover:bg-white/40 hover:text-moon-350 active:scale-100",
        )}
      >
        {status === "success" ? <Check className="size-3.5 text-status-green" /> : <Copy className="size-3.5" />}
        <span>{label}</span>
      </button>
      {hint ? (
        <span
          aria-live="polite"
          className="pointer-events-none absolute right-0 top-full mt-1 whitespace-nowrap text-[11px] leading-none text-status-red"
        >
          {hint}
        </span>
      ) : null}
    </div>
  );
}

function GlobalAccessRow({
  label,
  value,
  onCopy,
  copyAriaLabel,
  disabled,
}: {
  label: string;
  value: string;
  onCopy: () => Promise<void>;
  copyAriaLabel: string;
  disabled?: boolean;
}) {
  return (
    <div className="space-y-2.5 border-b border-moon-200/40 pb-4 last:border-b-0 last:pb-0">
      <p className="text-[11px] uppercase tracking-[0.18em] text-moon-400">{label}</p>
      <div className="flex items-center gap-3">
        <p className={cn("min-w-0 flex-1 break-all text-sm text-moon-700", disabled && "text-moon-400")}>
          {value}
        </p>
        <GlobalAccessCopyAction onCopy={onCopy} ariaLabel={copyAriaLabel} disabled={disabled} />
      </div>
    </div>
  );
}

export default function OverviewPage() {
  const { openAddAccount, dataVersion, poolSnapshots } = useAdminUI();
  const { navigate } = useRouter();
  const [overview, setOverview] = useState<Overview | null>(null);
  const [pools, setPools] = useState<Pool[]>([]);
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [poolActivity, setPoolActivity] = useState<Record<number, PoolActivity>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [snippetsOpen, setSnippetsOpen] = useState(false);
  const [qrOpen, setQrOpen] = useState(false);
  const [revealedGlobalToken, setRevealedGlobalToken] = useState<string | null>(null);
  const orderedPoolSnapshots = useMemo(
    () => pools.map((pool) => poolSnapshots[pool.id] ?? derivePoolSnapshot(pool)),
    [pools, poolSnapshots],
  );
  const activePoolStatsTargets = useMemo(
    () => orderedPoolSnapshots.filter(
      (pool) => pool.enabled && pool.health !== "unknown" && pool.health !== "disabled",
    ),
    [orderedPoolSnapshots],
  );
  const activePoolStatsKey = useMemo(
    () => activePoolStatsTargets.map((pool) => `${pool.id}:${pool.health}:${pool.enabled ? 1 : 0}`).join("|"),
    [activePoolStatsTargets],
  );
  const firstPoolModel = orderedPoolSnapshots.find((pool) => pool.enabled && pool.models.length > 0)?.models[0];

  function load() {
    setLoading(true);
    setError(null);
    setPoolActivity({});

    Promise.all([
      api.get<Overview>("/overview"),
      api.get<Pool[]>("/pools"),
      api.get<Account[]>("/accounts"),
    ])
      .then(async ([overviewData, poolData, accountData]) => {
        const safePools = poolData ?? [];
        setOverview(overviewData);
        setPools(safePools);
        setAccounts(accountData ?? []);
      })
      .catch((err) => {
        setError(err instanceof Error ? err.message : "总览加载失败");
      })
      .finally(() => setLoading(false));
  }

  useEffect(() => {
    load();
  }, [dataVersion]);

  useEffect(() => {
    setRevealedGlobalToken(null);
  }, [overview?.global_token_id]);

  useEffect(() => {
    let cancelled = false;

    if (activePoolStatsTargets.length === 0) {
      setPoolActivity({});
      return () => {
        cancelled = true;
      };
    }

    Promise.all(
      activePoolStatsTargets.map(async (pool) => {
        try {
          const stats = await api.get<UsageStats>(`/pools/${pool.id}/stats?window=today`);
          return [pool.id, { status: "ready", requestsToday: stats.total_requests }] as const;
        } catch {
          return [pool.id, { status: "error" }] as const;
        }
      }),
    ).then((entries) => {
      if (!cancelled) {
        setPoolActivity(Object.fromEntries(entries));
      }
    });

    return () => {
      cancelled = true;
    };
  }, [activePoolStatsKey]);

  const baseUrl = getApiBaseUrl();
  const hasGlobalToken = Boolean(overview?.global_token_id);
  const statusLine = useMemo(
    () => getStatusSentence(overview, accounts, orderedPoolSnapshots),
    [overview, accounts, orderedPoolSnapshots],
  );
  const title = "Pool Overview";

  async function revealGlobalTokenValue(): Promise<string> {
    if (revealedGlobalToken) {
      return revealedGlobalToken;
    }
    const revealed = await api.post<RevealedAccessToken>("/tokens/global/reveal");
    setRevealedGlobalToken(revealed.token);
    return revealed.token;
  }

  async function copyToClipboard(value: string) {
    await navigator.clipboard.writeText(value);
  }

  async function copyGlobalToken() {
    const token = await revealGlobalTokenValue();
    await copyToClipboard(token);
  }

  async function openSnippetsWithToken() {
    try {
      await revealGlobalTokenValue();
      setSnippetsOpen(true);
    } catch (err) {
      toast(err instanceof Error ? err.message : "读取全局 Token 失败", "error");
    }
  }

  async function openQrWithToken() {
    try {
      await revealGlobalTokenValue();
      setQrOpen(true);
    } catch (err) {
      toast(err instanceof Error ? err.message : "读取全局 Token 失败", "error");
    }
  }

  if (loading) {
    return (
      <div className="space-y-8">
        <Skeleton className="h-36 rounded-[2rem]" />
        <div className="grid gap-6 xl:grid-cols-[minmax(0,1.65fr)_20rem] xl:items-start">
          <div className="space-y-4">
            <div className="space-y-2">
              <Skeleton className="h-5 w-24 rounded-full" />
              <Skeleton className="h-4 w-52 rounded-full" />
            </div>
            <div className="grid gap-4 lg:grid-cols-2">
              {Array.from({ length: 2 }).map((_, index) => (
                <Skeleton key={index} className="h-44 rounded-[1.8rem]" />
              ))}
            </div>
          </div>
          <Skeleton className="h-56 rounded-[1.8rem]" />
        </div>
      </div>
    );
  }

  if (error) {
    return <ErrorState message={error} onRetry={load} />;
  }

  if (!pools.length) {
    return (
      <EmptyState
        eyebrow="First Run"
        title="开始之前，先接入第一个账号。"
        description="v3 的管理面板以 Pool 为中心。添加账号后，Lune 会自动生成可用的 API 地址与 Token。"
        action={<Button onClick={() => openAddAccount()}>添加账号</Button>}
      />
    );
  }

  return (
    <div className="space-y-8">
      <PageHeader
        title={title}
        description={statusLine}
        actions={
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={load}
            aria-label="刷新首页状态"
            className="rounded-full text-moon-400 hover:bg-white/65 hover:text-moon-700"
          >
            <RefreshCw className="size-4" />
          </Button>
        }
        ornament={
          <div className="relative h-full w-full overflow-hidden rounded-[2.4rem]">
            <div className="absolute right-[4.5rem] top-[1.2rem] size-[4.5rem] rounded-full border border-white/36 bg-[radial-gradient(circle_at_30%_30%,rgba(255,255,255,0.62),rgba(248,244,253,0.5)_42%,rgba(210,202,234,0.16)_72%,rgba(210,202,234,0)_100%)] opacity-34" />
            <div className="absolute right-[1.6rem] top-[1.45rem] h-[4.5rem] w-[4.5rem] rounded-full border border-lunar-200/14" />
            <div className="absolute right-[0.95rem] top-[2.2rem] h-[3.3rem] w-[3.3rem] rounded-full border border-lunar-200/10" />
            <div className="absolute inset-y-4 right-[1.2rem] w-[12rem] rounded-full border border-lunar-200/8" />
            <div className="absolute right-[2.8rem] top-[4.8rem] h-px w-[8rem] bg-[linear-gradient(90deg,rgba(134,125,193,0),rgba(134,125,193,0.09),rgba(134,125,193,0))]" />
            <div className="absolute right-[2.25rem] top-[2rem] space-y-2 opacity-20">
              <span className="block h-[2px] w-[2px] rounded-full bg-lunar-300/45" />
              <span className="ml-6 block h-[2px] w-[2px] rounded-full bg-lunar-300/25" />
              <span className="ml-2 block h-[2px] w-[2px] rounded-full bg-lunar-300/35" />
            </div>
          </div>
        }
      />

      <section className="grid gap-6 xl:grid-cols-[minmax(0,1.65fr)_20rem] xl:items-start">
        <div className="space-y-4">
          <SectionHeading
            title="Pools"
            description="按健康、模型与今日活动浏览。"
          />
          <div className="grid gap-4 lg:grid-cols-2">
            {orderedPoolSnapshots.map((pool) => {
              const activity = poolActivity[pool.id];
              return (
                <button
                  key={pool.id}
                  type="button"
                  onClick={() => navigate(`/admin/pools/${pool.id}`)}
                  className="surface-section fade-rise fade-rise-delay-1 text-left transition-[transform,border-color,box-shadow] duration-200 hover:-translate-y-0.5 hover:border-lunar-300/48 hover:shadow-[0_28px_68px_-54px_rgba(61,68,105,0.34)]"
                >
                  <div className="px-4 py-4 sm:px-4.5 sm:py-4.5">
                    <div className="flex items-start justify-between gap-4">
                      <div className="space-y-2.5">
                        <div className="flex items-center gap-2">
                          <span
                            className={`size-2 rounded-full ${
                              pool.health === "healthy"
                                ? "bg-status-green"
                                : pool.health === "degraded"
                                  ? "bg-status-yellow"
                                  : pool.health === "error"
                                    ? "bg-status-red"
                                    : "bg-moon-300"
                            }`}
                          />
                          <p className="text-sm font-medium text-moon-800">{pool.label}</p>
                        </div>
                        {pool.health === "disabled" ? (
                          <p className="text-sm text-moon-400">已停用，不参与当前工作面</p>
                        ) : pool.activeAccountCount == null || pool.healthyAccountCount == null ? (
                          <p className="text-sm text-moon-400">成员状态待确认</p>
                        ) : (
                          <p className="text-sm text-moon-500">
                            {pool.activeAccountCount} 账号 · {pool.healthyAccountCount} 可用
                          </p>
                        )}
                      </div>
                      <ArrowRight className="mt-0.5 size-4 text-moon-400" />
                    </div>

                    <div className="mt-4 border-t border-moon-200/50 pt-3">
                      <p className="text-sm text-moon-700">
                        {pool.health === "disabled"
                          ? "当前不提供能力"
                          : pool.health === "unknown"
                            ? "能力状态待确认"
                            : summarizeModels(pool.models).split(" · ").join(" / ")}
                      </p>
                      {pool.health === "disabled" ? (
                        <p className="mt-2.5 text-sm text-moon-400">不参与今日活动统计</p>
                      ) : pool.health === "unknown" ? (
                        <p className="mt-2.5 text-sm text-moon-400">活动状态待确认</p>
                      ) : activity?.status === "ready" ? (
                        <p className="mt-2.5 text-sm text-moon-500">
                          今日 {compact(activity.requestsToday)} 请求
                        </p>
                      ) : (
                        <p className="mt-2.5 text-sm text-moon-400">活动数据暂不可用</p>
                      )}
                    </div>
                  </div>
                </button>
              );
            })}
          </div>
        </div>

        <aside className="surface-section fade-rise relative overflow-hidden px-4.5 py-4.5 sm:px-5 sm:py-5">
          <div className="absolute inset-x-4 top-0 h-px moon-divider opacity-50" />
          <div className="space-y-4">
            <div>
              <p className="text-sm font-semibold text-moon-800">Global Access</p>
            </div>
            <div className="space-y-4">
              <GlobalAccessRow
                label="API 地址"
                value={baseUrl}
                onCopy={() => copyToClipboard(baseUrl)}
                copyAriaLabel="复制 API 地址"
              />
              <GlobalAccessRow
                label="API Key"
                value={hasGlobalToken ? overview?.global_token_masked ?? "未配置全局 Token" : "未配置全局 Token"}
                onCopy={copyGlobalToken}
                copyAriaLabel="复制 API Key"
                disabled={!hasGlobalToken}
              />
            </div>

            {!hasGlobalToken ? (
              <p className="text-sm text-moon-500">
                未配置全局 Token，请前往 Settings 创建或启用后再使用。
              </p>
            ) : null}

            <div className="flex flex-wrap gap-2.5 pt-1">
              <Button
                variant="outline"
                size="sm"
                onClick={openSnippetsWithToken}
                disabled={!hasGlobalToken}
                className="rounded-full border-moon-200/55 bg-white/45"
              >
                <KeyRound className="size-3.5" />
                Env Snippets
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={openQrWithToken}
                disabled={!hasGlobalToken}
                className="rounded-full border-moon-200/55 bg-white/45"
              >
                <QrCode className="size-3.5" />
                QR
              </Button>
            </div>
          </div>
        </aside>
      </section>

      <EnvSnippetsDialog
        open={snippetsOpen}
        onOpenChange={setSnippetsOpen}
        title="Global Env Snippets"
        baseUrl={baseUrl}
        token={revealedGlobalToken ?? ""}
        model={firstPoolModel}
      />
      <QrCodeDialog
        open={qrOpen}
        onOpenChange={setQrOpen}
        title="Global Token QR"
        baseUrl={baseUrl}
        token={revealedGlobalToken ?? ""}
      />
    </div>
  );
}
