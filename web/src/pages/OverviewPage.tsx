import { useEffect, useMemo, useRef, useState } from "react";
import { AlertCircle, ArrowRight, Check, ChevronDown, Copy, KeyRound, QrCode, RefreshCw } from "lucide-react";
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
import { compact, latency } from "@/lib/fmt";
import { derivePoolSnapshot, getApiBaseUrl, type PoolSnapshot } from "@/lib/lune";
import { useRouter } from "@/lib/router";
import { cn } from "@/lib/utils";
import type {
  LatencyBucket,
  Overview,
  OverviewAlert,
  Pool,
  RevealedAccessToken,
} from "@/lib/types";

const MIN_REQUESTS_FOR_SUCCESS_RATE = 5;
const LATENCY_PERIOD = "1h";
const LATENCY_BUCKET = "5m";

type PoolLatencyState =
  | {
      status: "ready";
      buckets: LatencyBucket[];
      currentP95: number | null;
    }
  | {
      status: "empty";
    }
  | {
      status: "error";
    };

function formatSuccessRate(value: number) {
  const percent = value * 100;
  if (percent >= 99.95) {
    return "100%";
  }
  return `${percent.toFixed(1).replace(/\.0$/, "")}%`;
}

function getStatusSentence(overview: Overview | null, poolSnapshots: PoolSnapshot[]) {
  if (!overview) return "";
  const parts: string[] = [];
  const enabledPools = poolSnapshots.filter((pool) => pool.enabled && pool.health !== "disabled");
  const knownPools = enabledPools.filter((pool) => pool.health !== "unknown");
  const pendingPools = enabledPools.length - knownPools.length;
  const routablePools = knownPools.filter((pool) => pool.health === "healthy" || pool.health === "degraded");

  if (overview.alerts.length > 0) {
    const leadAlert = getAlertSummary(overview.alerts[0]);
    parts.push(
      overview.alerts.length === 1
        ? leadAlert
        : `${overview.alerts.length} 条提醒待处理 · ${leadAlert}`,
    );
  }

  if (knownPools.length > 0) {
    const poolText = `${routablePools.length} / ${knownPools.length} 个 Pool 可路由`;
    parts.push(pendingPools > 0 ? `${poolText} · ${pendingPools} 个待确认` : poolText);
  } else if (enabledPools.length > 0) {
    parts.push("Pool 状态确认中");
  }

  if (overview.requests_today >= MIN_REQUESTS_FOR_SUCCESS_RATE) {
    parts.push(`今日成功率 ${formatSuccessRate(overview.success_rate_today)}`);
  }

  if (overview.requests_today > 0) {
    parts.push(`今日 ${compact(overview.requests_today)} 次请求`);
  } else {
    parts.push("今日暂无请求");
  }

  return parts.join(" · ");
}

function extractQuotedLabel(message: string) {
  const match = message.match(/"([^"]+)"/);
  return match?.[1] ?? null;
}

function getAlertSummary(alert: OverviewAlert) {
  const label = extractQuotedLabel(alert.message);

  switch (alert.type) {
    case "account_expiring":
    case "expiring":
      return label ? `${label} 7 天内到期` : "有账号即将到期";
    case "account_error":
    case "error":
      return label ? `${label} 健康异常` : "有账号健康异常";
    case "pool_unhealthy":
      return label ? `${label} 需要处理` : "有 Pool 状态异常";
    default:
      return alert.message;
  }
}

function getAlertDestination(alert: OverviewAlert) {
  if (alert.pool_id) {
    return `/admin/pools/${alert.pool_id}`;
  }
  return "/admin/settings";
}

function getAlertActionLabel(alert: OverviewAlert) {
  return alert.pool_id ? "查看 Pool" : "前往 Settings";
}

function StatusDot({ health }: { health: PoolSnapshot["health"] }) {
  return (
    <span
      className={cn(
        "size-2 rounded-full",
        health === "healthy"
          ? "bg-status-green"
          : health === "degraded"
            ? "bg-status-yellow"
            : health === "error"
              ? "bg-status-red"
              : "bg-moon-300",
      )}
    />
  );
}

function HealthDistributionBar({
  snapshot,
}: {
  snapshot: PoolSnapshot;
}) {
  const counts = snapshot.memberStatusCounts;
  const total = counts?.total ?? 0;

  if (!counts || total === 0) {
    return (
      <div className="h-1.5 overflow-hidden rounded-full bg-moon-150/85">
        <div className="h-full w-full rounded-full bg-[linear-gradient(90deg,rgba(200,204,220,0.44),rgba(222,225,236,0.7))]" />
      </div>
    );
  }

  const segments = [
    { key: "healthy", value: counts.healthy, className: "bg-status-green" },
    { key: "degraded", value: counts.degraded, className: "bg-status-yellow" },
    { key: "error", value: counts.error, className: "bg-status-red" },
    { key: "disabled", value: counts.disabled, className: "bg-moon-300/80" },
    { key: "unknown", value: counts.unknown, className: "bg-moon-200/90" },
  ].filter((segment) => segment.value > 0);

  return (
    <div className="flex h-1.5 overflow-hidden rounded-full bg-moon-150/85">
      {segments.map((segment) => (
        <div
          key={segment.key}
          className={segment.className}
          style={{ width: `${(segment.value / total) * 100}%` }}
        />
      ))}
    </div>
  );
}

function buildSparklinePath(values: number[], width: number, height: number) {
  if (values.length === 0) {
    return "";
  }

  const min = Math.min(...values);
  const max = Math.max(...values);
  const range = max - min || 1;

  if (values.length === 1) {
    const y = height - ((values[0] - min) / range) * height;
    return `M 20 ${y.toFixed(2)} L ${Math.max(width - 20, 21)} ${y.toFixed(2)}`;
  }

  return values
    .map((value, index) => {
      const x = values.length === 1 ? width / 2 : (index / (values.length - 1)) * width;
      const y = height - ((value - min) / range) * height;
      return `${index === 0 ? "M" : "L"} ${x.toFixed(2)} ${y.toFixed(2)}`;
    })
    .join(" ");
}

function LatencySparkline({
  state,
}: {
  state: PoolLatencyState | undefined;
}) {
  if (!state || state.status !== "ready" || state.buckets.length === 0 || state.currentP95 == null) {
    return (
      <svg viewBox="0 0 120 24" className="h-6 w-full" aria-hidden="true">
        <path
          d="M 1 12 L 119 12"
          fill="none"
          stroke="rgba(189, 194, 214, 0.72)"
          strokeWidth="1.5"
          strokeLinecap="round"
        />
      </svg>
    );
  }

  const values = state.buckets.map((bucket) => bucket.p95);
  const path = buildSparklinePath(values, 120, 20);

  return (
    <svg viewBox="0 0 120 24" className="h-6 w-full" aria-hidden="true">
      <path
        d="M 1 20 L 119 20"
        fill="none"
        stroke="rgba(212, 216, 230, 0.64)"
        strokeWidth="1"
        strokeLinecap="round"
      />
      <path
        d={path}
        fill="none"
        stroke="rgba(134, 125, 193, 0.66)"
        strokeWidth="1.7"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

function getLatencyLabel(state: PoolLatencyState | undefined, pool: PoolSnapshot) {
  if (!pool.enabled) {
    return "已停用";
  }
  if (pool.health === "unknown") {
    return "状态待确认";
  }
  if (!state) {
    return "读取最近延迟";
  }
  if (state.status === "empty") {
    return "最近 1h 暂无样本";
  }
  if (state.status === "error") {
    return "延迟数据暂不可用";
  }
  if (state.currentP95 == null) {
    return "最近 1h 暂无样本";
  }
  return `P95 ${latency(state.currentP95)}`;
}

function AlertsBar({
  alerts,
  onNavigate,
}: {
  alerts: OverviewAlert[];
  onNavigate: (href: string) => void;
}) {
  const [expanded, setExpanded] = useState(false);

  if (alerts.length === 0) {
    return null;
  }

  const singleAlert = alerts.length === 1 ? alerts[0] : null;
  const summary = singleAlert ? getAlertSummary(singleAlert) : `${alerts.length} 条需要处理的提醒`;

  return (
    <section className="surface-section fade-rise relative overflow-hidden border-lunar-200/42 bg-[linear-gradient(180deg,rgba(255,250,239,0.84),rgba(247,244,252,0.9))] px-4.5 py-4 sm:px-5">
      <div className="absolute inset-x-4 top-0 h-px bg-[linear-gradient(90deg,transparent,rgba(192,154,85,0.42),transparent)]" />
      <div className="flex flex-col gap-3">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex min-w-0 items-center gap-3">
            <span className="inline-flex size-8 shrink-0 items-center justify-center rounded-full border border-white/80 bg-white/72 text-status-yellow shadow-[inset_0_1px_0_rgba(255,255,255,0.85)]">
              <AlertCircle className="size-4" />
            </span>
            <div className="min-w-0">
              <p className="text-[11px] uppercase tracking-[0.18em] text-moon-400">提醒</p>
              <p className="truncate text-sm text-moon-700">{summary}</p>
            </div>
          </div>

          <div className="flex items-center gap-2">
            {singleAlert ? (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => onNavigate(getAlertDestination(singleAlert))}
                className="rounded-full px-3 text-moon-600 hover:bg-white/70 hover:text-moon-800"
              >
                {getAlertActionLabel(singleAlert)}
              </Button>
            ) : (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setExpanded((current) => !current)}
                className="rounded-full px-3 text-moon-600 hover:bg-white/70 hover:text-moon-800"
              >
                {expanded ? "收起" : "展开"}
                <ChevronDown className={cn("size-4 transition-transform", expanded && "rotate-180")} />
              </Button>
            )}
          </div>
        </div>

        {!singleAlert && expanded ? (
          <div className="space-y-2 border-t border-moon-200/45 pt-3">
            {alerts.map((alert, index) => (
              <button
                key={`${alert.type}:${alert.message}:${index}`}
                type="button"
                onClick={() => onNavigate(getAlertDestination(alert))}
                className="flex w-full items-center justify-between gap-4 rounded-[1rem] border border-white/64 bg-white/55 px-3.5 py-3 text-left transition-colors hover:bg-white/80"
              >
                <div className="min-w-0">
                  <p className="text-sm text-moon-700">{getAlertSummary(alert)}</p>
                  <p className="mt-1 truncate text-[12px] text-moon-400">{alert.message}</p>
                </div>
                <span className="shrink-0 text-[12px] font-medium text-moon-500">
                  {getAlertActionLabel(alert)}
                </span>
              </button>
            ))}
          </div>
        ) : null}
      </div>
    </section>
  );
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
  const [poolLatency, setPoolLatency] = useState<Record<number, PoolLatencyState>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [snippetsOpen, setSnippetsOpen] = useState(false);
  const [qrOpen, setQrOpen] = useState(false);
  const [revealedGlobalToken, setRevealedGlobalToken] = useState<string | null>(null);
  const orderedPoolSnapshots = useMemo(
    () => pools.map((pool) => poolSnapshots[pool.id] ?? derivePoolSnapshot(pool)),
    [pools, poolSnapshots],
  );
  const latencyTargets = useMemo(
    () => orderedPoolSnapshots.filter((pool) => pool.enabled),
    [orderedPoolSnapshots],
  );
  const latencyTargetsKey = useMemo(
    () => latencyTargets.map((pool) => `${pool.id}:${pool.health}:${pool.enabled ? 1 : 0}`).join("|"),
    [latencyTargets],
  );
  const firstPoolModel = orderedPoolSnapshots.find((pool) => pool.enabled && pool.models.length > 0)?.models[0];
  const baseUrl = getApiBaseUrl();
  const hasGlobalToken = Boolean(overview?.global_token_id);
  const statusLine = useMemo(
    () => getStatusSentence(overview, orderedPoolSnapshots),
    [overview, orderedPoolSnapshots],
  );
  const title = "Pool Overview";

  function load() {
    setLoading(true);
    setError(null);
    setPoolLatency({});

    Promise.all([
      api.get<Overview>("/overview"),
      api.get<Pool[]>("/pools"),
    ])
      .then(([overviewData, poolData]) => {
        setOverview(overviewData);
        setPools(poolData ?? []);
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

    if (latencyTargets.length === 0) {
      setPoolLatency({});
      return () => {
        cancelled = true;
      };
    }

    Promise.all(
      latencyTargets.map(async (pool) => {
        try {
          const buckets = await api.get<LatencyBucket[]>(
            `/usage/latency?period=${LATENCY_PERIOD}&bucket=${LATENCY_BUCKET}&pool=${pool.id}`,
          );
          if (!buckets?.length) {
            return [pool.id, { status: "empty" } satisfies PoolLatencyState] as const;
          }
          const currentP95 = buckets[buckets.length - 1]?.p95 ?? null;
          return [pool.id, { status: "ready", buckets, currentP95 } satisfies PoolLatencyState] as const;
        } catch {
          return [pool.id, { status: "error" } satisfies PoolLatencyState] as const;
        }
      }),
    ).then((entries) => {
      if (!cancelled) {
        setPoolLatency(Object.fromEntries(entries));
      }
    });

    return () => {
      cancelled = true;
    };
  }, [latencyTargetsKey]);

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
            <Skeleton className="h-20 rounded-[1.5rem]" />
            <div className="grid gap-4 lg:grid-cols-2">
              {Array.from({ length: 2 }).map((_, index) => (
                <Skeleton key={index} className="h-52 rounded-[1.8rem]" />
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
        className={overview?.alerts?.length ? "border-status-yellow/18" : undefined}
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

      {overview?.alerts?.length ? (
        <AlertsBar alerts={overview.alerts} onNavigate={navigate} />
      ) : null}

      <section className="grid gap-6 xl:grid-cols-[minmax(0,1.65fr)_20rem] xl:items-start">
        <div className="space-y-4">
          <SectionHeading
            title="Pools"
            description="按健康与最近延迟快速判断。"
          />
          <div className="grid gap-4 lg:grid-cols-2">
            {orderedPoolSnapshots.map((pool, index) => {
              const counts = pool.memberStatusCounts;
              const accountSummary =
                pool.health === "disabled"
                  ? "已停用，不参与当前工作面"
                  : pool.activeAccountCount == null || pool.availableAccountCount == null
                    ? "成员状态待确认"
                    : counts && counts.total > 0
                      ? `${counts.total} 账号 · ${pool.availableAccountCount} 可用`
                      : `${pool.activeAccountCount} 账号 · ${pool.availableAccountCount} 可用`;
              const animationClass = index % 3 === 0 ? "fade-rise-delay-1" : index % 3 === 1 ? "fade-rise-delay-2" : "fade-rise-delay-3";

              return (
                <button
                  key={pool.id}
                  type="button"
                  onClick={() => navigate(`/admin/pools/${pool.id}`)}
                  className={cn(
                    "surface-section fade-rise text-left transition-[transform,border-color,box-shadow] duration-200 hover:-translate-y-0.5 hover:border-lunar-300/48 hover:shadow-[0_28px_68px_-54px_rgba(61,68,105,0.34)]",
                    animationClass,
                  )}
                >
                  <div className="px-4 py-4 sm:px-4.5 sm:py-4.5">
                    <div className="flex items-start justify-between gap-4">
                      <div className="space-y-2">
                        <div className="flex items-center gap-2">
                          <StatusDot health={pool.health} />
                          <p className="text-sm font-medium text-moon-800">{pool.label}</p>
                        </div>
                        <p className="text-sm text-moon-500">{accountSummary}</p>
                      </div>
                      <ArrowRight className="mt-0.5 size-4 text-moon-400" />
                    </div>

                    <div className="mt-4 space-y-2.5">
                      <HealthDistributionBar snapshot={pool} />
                      <div className="flex items-center justify-between gap-3 border-t border-moon-200/50 pt-3">
                        <div className="min-w-0 flex-1">
                          <LatencySparkline state={poolLatency[pool.id]} />
                        </div>
                        <p className="shrink-0 text-[12px] font-medium text-moon-600">
                          {getLatencyLabel(poolLatency[pool.id], pool)}
                        </p>
                      </div>
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
