import { useEffect, useRef, useState } from "react";
import DataTable, { type Column } from "@/components/DataTable";
import PageHeader from "@/components/PageHeader";
import SectionHeading from "@/components/SectionHeading";
import StatusBadge from "@/components/StatusBadge";
import { api } from "@/lib/api";
import { compact, latency, pct, relativeTime, shortDate } from "@/lib/fmt";
import { estimateCost, formatCost } from "@/lib/pricing";
import type { Overview, RequestLog } from "@/lib/types";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import {
  Activity,
  AlertTriangle,
  ArrowUpRight,
  Globe,
  Key,
  Layers,
  RefreshCw,
  ShieldCheck,
  Users,
} from "lucide-react";

const requestColumns: Column<RequestLog>[] = [
  {
    key: "time",
    header: "时间",
    render: (r) => <span className="text-moon-500">{shortDate(r.created_at)}</span>,
    tone: "secondary",
  },
  {
    key: "model",
    header: "模型",
    render: (r) => <span className="font-medium">{r.model_alias}</span>,
    tone: "primary",
  },
  {
    key: "token",
    header: "令牌",
    render: (r) => <span className="text-moon-500">{r.access_token_name}</span>,
    tone: "secondary",
  },
  {
    key: "account",
    header: "账号",
    render: (r) => <span className="text-moon-500">{r.account_label}</span>,
    tone: "secondary",
  },
  {
    key: "status",
    header: "状态",
    render: (r) => (
      <StatusBadge
        status={r.success ? "healthy" : "error"}
        label={String(r.status_code)}
      />
    ),
    tone: "status",
  },
  {
    key: "latency",
    header: "延迟",
    render: (r) => <span className="text-moon-500">{latency(r.latency_ms)}</span>,
    align: "right",
    tone: "numeric",
  },
  {
    key: "cost",
    header: "预估成本",
    render: (r) => {
      const cost = r.input_tokens != null
        ? estimateCost(r.target_model || r.model_alias, r.input_tokens, r.output_tokens ?? 0)
        : null;
      return cost !== null
        ? <span className="text-moon-500">{formatCost(cost)}</span>
        : <span className="text-moon-400">-</span>;
    },
    align: "right",
    tone: "numeric",
  },
];

export default function OverviewPage() {
  const [data, setData] = useState<Overview | null>(null);
  const [initialLoading, setInitialLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [refreshError, setRefreshError] = useState<string | null>(null);
  const dataRef = useRef<Overview | null>(null);
  const mountedRef = useRef(true);
  const requestIdRef = useRef(0);

  function load(options?: { background?: boolean }) {
    const background = options?.background ?? false;
    const hasData = dataRef.current !== null;
    const requestId = requestIdRef.current + 1;
    requestIdRef.current = requestId;

    if (background && hasData) {
      setRefreshing(true);
    } else if (!hasData) {
      setInitialLoading(true);
    }

    api
      .get<Overview>("/overview")
      .then((next) => {
        if (!mountedRef.current || requestId !== requestIdRef.current) return;
        dataRef.current = next;
        setData(next);
        setRefreshError(null);
      })
      .catch((err) => {
        if (!mountedRef.current || requestId !== requestIdRef.current) return;
        setRefreshError(err instanceof Error ? err.message : "刷新失败");
      })
      .finally(() => {
        if (!mountedRef.current || requestId !== requestIdRef.current) return;
        if (background && hasData) {
          setRefreshing(false);
        } else if (!hasData) {
          setInitialLoading(false);
        }
      });
  }

  useEffect(() => {
    mountedRef.current = true;
    load();
    const interval = setInterval(() => load({ background: true }), 10_000);
    return () => {
      mountedRef.current = false;
      clearInterval(interval);
    };
  }, []);

  if (initialLoading) {
    return (
      <div className="space-y-8">
        <Skeleton className="h-28 rounded-[1.5rem]" />
        <Skeleton className="h-[28rem] rounded-[2rem]" />
        <div className="grid gap-6 xl:grid-cols-[minmax(320px,0.9fr)_minmax(0,1.15fr)]">
          <Skeleton className="h-80 rounded-[1.6rem]" />
          <Skeleton className="h-80 rounded-[1.6rem]" />
        </div>
      </div>
    );
  }

  const overview = data;
  const totalUsage =
    (overview?.token_usage_24h?.input ?? 0) +
    (overview?.token_usage_24h?.output ?? 0);
  const cpaAccounts = overview?.accounts_by_source?.cpa ?? 0;
  const openaiCompatAccounts = overview?.accounts_by_source?.openai_compat ?? 0;
  const serviceStatus = overview?.cpa_status;
  const sideSummary = [
    {
      label: "控制面",
      value: serviceStatus?.label ?? "未配置",
      tone: "primary" as const,
    },
    {
      label: "健康账号",
      value: String(serviceStatus?.accounts_healthy ?? 0),
    },
    {
      label: "即将到期",
      value: String(serviceStatus?.accounts_expiring ?? 0),
    },
    {
      label: "直接接入",
      value: String(openaiCompatAccounts),
    },
  ];
  const compactMetrics = [
    {
      label: "可用账号",
      value: compact(overview?.healthy_accounts ?? 0),
      icon: Users,
      featured: true,
    },
    {
      label: "池",
      value: compact(overview?.total_pools ?? 0),
      icon: Layers,
    },
    {
      label: "令牌",
      value: compact(overview?.total_tokens ?? 0),
      icon: Key,
    },
    {
      label: "Token",
      value: compact(totalUsage),
      icon: Activity,
    },
  ];
  return (
    <div className="space-y-10">
      <PageHeader
        eyebrow="Overview / Console"
        title="总览"
        description="先看状态，再看流量。"
        meta={
          <>
            <span className="inline-flex items-center gap-2 rounded-full border border-white/70 bg-white/56 px-3 py-1.5">
              账号 {overview?.total_accounts ?? 0}
            </span>
            <span className="inline-flex items-center gap-2 rounded-full border border-white/70 bg-white/56 px-3 py-1.5">
              池 {overview?.total_pools ?? 0}
            </span>
            <span className="inline-flex items-center gap-2 rounded-full border border-white/70 bg-white/56 px-3 py-1.5">
              访问令牌 {overview?.total_tokens ?? 0}
            </span>
            <span className="inline-flex items-center gap-2 text-moon-400">
              {refreshing
                ? "数据更新中"
                : serviceStatus?.last_checked_at
                  ? `更新于 ${relativeTime(serviceStatus.last_checked_at)}`
                  : "等待首次同步"}
            </span>
            <Button
              size="sm"
              variant="outline"
              onClick={() => load({ background: true })}
              disabled={refreshing}
              className="h-7 rounded-full border-white/72 bg-white/62 px-3.5 text-[12px] shadow-[0_10px_24px_-20px_rgba(33,40,63,0.2)] disabled:opacity-100"
            >
              <RefreshCw className={refreshing ? "size-3.5 animate-spin" : "size-3.5"} />
              {refreshing ? "更新中" : "刷新"}
            </Button>
          </>
        }
        ornament={
          <>
            <div className="absolute inset-0 rounded-[999px] bg-[radial-gradient(circle_at_76%_28%,rgba(255,255,255,0.34),rgba(255,255,255,0)_24%),radial-gradient(circle_at_82%_38%,rgba(134,125,193,0.1),rgba(134,125,193,0)_36%)]" />
            <div className="absolute right-2 top-1 size-[7.5rem] rounded-full border border-moon-300/15" />
            <div className="absolute right-7 top-4 size-[5.4rem] rounded-full border border-lunar-200/20" />
            <div className="absolute right-[4.9rem] top-[1.45rem] size-[2.6rem] rounded-full border border-white/22" />
            <div className="absolute right-[8.75rem] top-[1.2rem] h-[4.6rem] w-[11rem] rounded-[999px] border border-moon-300/8" />
            <div className="absolute right-[3.8rem] top-[2.2rem] h-px w-[10rem] bg-[linear-gradient(90deg,rgba(152,160,183,0),rgba(152,160,183,0.24),rgba(152,160,183,0.02),rgba(152,160,183,0))]" />
            <div className="absolute right-[7.4rem] top-[4.2rem] h-px w-[6.4rem] bg-[linear-gradient(90deg,rgba(152,160,183,0),rgba(152,160,183,0.16),rgba(152,160,183,0))]" />
            <div className="absolute right-[6.8rem] top-[1.6rem] size-[3px] rounded-full bg-white/40 shadow-[0_0_10px_rgba(255,255,255,0.24)]" />
            <div className="absolute right-[10.4rem] top-[3.45rem] size-[3px] rounded-full bg-lunar-200/50 shadow-[0_0_12px_rgba(197,192,236,0.22)]" />
            <div className="absolute right-[2.7rem] top-[5rem] size-[2px] rounded-full bg-moon-300/60" />
            <svg
              viewBox="0 0 320 126"
              className="absolute right-0 top-0 h-full w-full text-moon-300/32"
              fill="none"
            >
              <path
                d="M54 84c28-35 72-53 137-53 39 0 77 8 115 23"
                stroke="currentColor"
                strokeWidth="1"
                strokeLinecap="round"
                strokeDasharray="2 8"
              />
              <path
                d="M122 100c17-18 42-28 74-29 25-1 53 3 85 14"
                stroke="currentColor"
                strokeWidth="1"
                strokeLinecap="round"
              />
              <path
                d="M188 22c32 12 58 32 78 60"
                stroke="currentColor"
                strokeWidth="0.9"
                strokeLinecap="round"
                strokeOpacity="0.75"
              />
              <path
                d="M90 52c26-12 56-18 91-18"
                stroke="currentColor"
                strokeWidth="0.9"
                strokeLinecap="round"
                strokeOpacity="0.6"
              />
              <circle cx="268" cy="56" r="2.2" fill="currentColor" fillOpacity="0.42" />
              <circle cx="221" cy="34" r="1.6" fill="currentColor" fillOpacity="0.32" />
            </svg>
          </>
        }
      />

      {refreshError && (
        <section className="surface-card border-amber-200/70 bg-amber-50/80 px-4 py-4">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex items-start gap-3">
              <AlertTriangle className="mt-0.5 size-4 text-amber-700" />
              <div>
                <p className="text-sm font-medium text-amber-900">总览刷新失败</p>
                <p className="mt-1 text-sm text-amber-800/80">{refreshError}</p>
              </div>
            </div>
            <Button size="sm" variant="outline" onClick={() => load({ background: true })}>
              重试
            </Button>
          </div>
        </section>
      )}

      <section
        className="surface-section hero-glow relative overflow-hidden px-6 py-6 transition-[opacity,filter] duration-300 sm:px-7 sm:py-7"
        style={{
          opacity: refreshing ? 0.985 : 1,
          filter: refreshing ? "saturate(0.97)" : "none",
        }}
        aria-busy={refreshing}
      >
        <div className="absolute right-[-3rem] top-[-2rem] h-44 w-44 rounded-full bg-[radial-gradient(circle,rgba(255,255,255,0.96),rgba(255,255,255,0)_68%)] blur-xl" />
        <div className="absolute left-[42%] top-12 h-48 w-48 rounded-full bg-[radial-gradient(circle,rgba(134,125,193,0.18),rgba(134,125,193,0)_72%)] blur-3xl" />
        <div className="grid gap-7 xl:grid-cols-[minmax(0,1.22fr)_minmax(300px,0.78fr)]">
          <div className="space-y-7">
            <div className="space-y-4">
              <p className="eyebrow-label">Moonlight Surface</p>
              <div className="space-y-2.5">
                <h2 className="font-editorial text-[2.4rem] font-semibold tracking-[-0.065em] text-moon-800 sm:text-[3.45rem]">
                  今夜的网关
                </h2>
                <p className="max-w-xl text-sm leading-7 text-moon-500 sm:text-[15px]">
                  先看当下，再决定下一步。
                </p>
              </div>
            </div>

            <div className="grid gap-3 sm:grid-cols-3">
              <div className="rounded-[1.25rem] border border-white/72 bg-white/62 px-4 py-4 shadow-[inset_0_1px_0_rgba(255,255,255,0.82)]">
                <p className="kicker">服务状态</p>
                <div className="mt-3 flex items-center gap-3">
                  {serviceStatus ? (
                    <StatusBadge
                      status={
                        serviceStatus.status === "healthy"
                          ? "healthy"
                          : serviceStatus.status === "error"
                            ? "error"
                            : "degraded"
                      }
                    />
                  ) : (
                    <StatusBadge status="disabled" label="未配置" />
                  )}
                </div>
                <p className="mt-3 text-sm text-moon-500">
                  {serviceStatus?.last_checked_at
                    ? `最近检查 ${relativeTime(serviceStatus.last_checked_at)}`
                    : "尚未连接 CPA 控制面"}
                </p>
              </div>

              <div className="rounded-[1.25rem] border border-white/72 bg-white/62 px-4 py-4 shadow-[inset_0_1px_0_rgba(255,255,255,0.82)]">
                <p className="kicker">供给概况</p>
                <p className="mt-3 text-[1.5rem] font-semibold tracking-[-0.05em] text-moon-800">
                  {overview?.total_accounts ?? 0}
                </p>
                <p className="mt-2 text-sm text-moon-500">
                  直连 {openaiCompatAccounts} · CPA {cpaAccounts}
                </p>
              </div>

              <div className="rounded-[1.25rem] border border-white/72 bg-white/62 px-4 py-4 shadow-[inset_0_1px_0_rgba(255,255,255,0.82)]">
                <p className="kicker">24h 请求</p>
                <p className="mt-3 text-[1.5rem] font-semibold tracking-[-0.05em] text-moon-800">
                  {compact(overview?.requests_24h ?? 0)}
                </p>
                <p className="mt-2 text-sm text-moon-500">
                  成功率 {pct(overview?.success_rate_24h ?? 0)}
                </p>
              </div>
            </div>

            <div className="grid gap-x-5 gap-y-3 border-t border-white/55 pt-4 md:grid-cols-[minmax(0,1.25fr)_repeat(3,minmax(0,0.9fr))]">
              {compactMetrics.map((item) => (
                <div
                  key={item.label}
                  className={
                    item.featured
                      ? "rounded-[1.2rem] border border-white/72 bg-[linear-gradient(180deg,rgba(255,255,255,0.72),rgba(247,245,251,0.6))] px-4 py-3.5 shadow-[inset_0_1px_0_rgba(255,255,255,0.85)]"
                      : "px-1 py-2"
                  }
                >
                  <div className="flex items-center justify-between gap-3">
                    <p className="text-[11px] tracking-[0.16em] text-moon-400">{item.label}</p>
                    <item.icon className={item.featured ? "size-4 text-moon-400" : "size-3.5 text-moon-300"} />
                  </div>
                  <p
                    className={
                      item.featured
                        ? "mt-2 text-[1.1rem] font-semibold tracking-[-0.045em] text-moon-800"
                        : "mt-1.5 text-base font-medium tracking-[-0.03em] text-moon-700"
                    }
                  >
                    {item.value}
                  </p>
                </div>
              ))}
            </div>
          </div>

          <aside className="space-y-3">
            <div className="rounded-[1.5rem] border border-white/74 bg-[linear-gradient(180deg,rgba(255,255,255,0.78),rgba(246,243,251,0.64))] px-5 py-5 shadow-[inset_0_1px_0_rgba(255,255,255,0.84)]">
              <div className="flex items-start justify-between gap-3 border-b border-moon-200/55 pb-3.5">
                <div>
                  <p className="kicker">侧边摘要</p>
                  <p className="mt-1 text-sm text-moon-500">控制面与供给状态</p>
                </div>
                <div className="flex size-8 items-center justify-center rounded-full bg-white/72">
                  <ShieldCheck className="size-4 text-lunar-600" />
                </div>
              </div>

              <div className="space-y-3.5 pt-4">
                {sideSummary.map((item) => (
                  <div
                    key={item.label}
                    className="flex items-baseline justify-between gap-4"
                  >
                    <span className="text-sm text-moon-500">{item.label}</span>
                    <span
                      className={
                        item.tone === "primary"
                          ? "text-[15px] font-medium tracking-[-0.02em] text-moon-800"
                          : "font-medium text-moon-700"
                      }
                    >
                      {item.value}
                    </span>
                  </div>
                ))}
              </div>
            </div>

            <div className="rounded-[1.5rem] border border-white/72 bg-[linear-gradient(180deg,rgba(244,241,251,0.88),rgba(255,255,255,0.72))] px-5 py-5 shadow-[inset_0_1px_0_rgba(255,255,255,0.82)]">
              <p className="kicker">24 小时摘要</p>
              <div className="mt-4 space-y-3.5">
                <div className="flex items-center justify-between gap-4">
                  <span className="text-sm text-moon-500">请求</span>
                  <span className="text-lg font-semibold tracking-[-0.04em] text-moon-800">
                    {compact(overview?.requests_24h ?? 0)}
                  </span>
                </div>
                <div className="flex items-center justify-between gap-4">
                  <span className="text-sm text-moon-500">成功率</span>
                  <span className="text-lg font-semibold tracking-[-0.04em] text-moon-800">
                    {pct(overview?.success_rate_24h ?? 0)}
                  </span>
                </div>
                <div className="flex items-center justify-between gap-4">
                  <span className="text-sm text-moon-500">输入 / 输出</span>
                  <span className="font-medium text-moon-700">
                    {compact(overview?.token_usage_24h?.input ?? 0)} / {compact(overview?.token_usage_24h?.output ?? 0)}
                  </span>
                </div>
              </div>
            </div>
          </aside>
        </div>
      </section>

      <section className="grid gap-6 xl:grid-cols-[minmax(320px,0.88fr)_minmax(0,1.12fr)]">
        <div className="space-y-4">
          <SectionHeading
            title="账号健康"
            description="最近检查结果、当前可用性与阻塞错误。"
          />
          <div className="surface-card overflow-hidden">
            {(!overview?.account_health || overview.account_health.length === 0) && (
              <div className="px-6 py-12 text-center text-sm text-moon-400">
                还没有可观察的账号
              </div>
            )}
            {overview?.account_health?.map((account, index) => (
              <div
                key={account.id}
                className={`flex flex-col gap-3 px-5 py-4 sm:flex-row sm:items-start sm:justify-between ${
                  index > 0 ? "border-t border-moon-200/60" : ""
                } ${account.status === "disabled" ? "opacity-60" : ""}`}
              >
                <div className="space-y-2">
                  <div className="flex items-center gap-3">
                    <StatusBadge
                      status={
                        account.status as "healthy" | "degraded" | "error" | "disabled"
                      }
                    />
                    <span className="font-medium text-moon-800">{account.label}</span>
                  </div>
                  <p className="text-sm text-moon-500">
                    {account.last_checked_at
                      ? `最近检查 ${relativeTime(account.last_checked_at)}`
                      : "尚未检查"}
                  </p>
                </div>
                <div className="max-w-sm text-sm text-moon-500 sm:text-right">
                  {account.last_error ? (
                    <p className="text-status-red">{account.last_error}</p>
                  ) : (
                    <p>未发现阻塞错误。</p>
                  )}
                </div>
              </div>
            ))}
          </div>
        </div>

        <div className="space-y-4">
          <SectionHeading
            title="最近请求"
            description="最新流量样本，用来快速判断路由、令牌与状态码。"
            action={
              <span className="inline-flex items-center gap-1 text-sm text-moon-500">
                最近 24h
                <ArrowUpRight className="size-4" />
              </span>
            }
          />
          <div className="surface-card overflow-hidden">
            <div className="flex items-center justify-between border-b border-moon-200/60 px-4 py-3">
              <div className="flex flex-wrap gap-x-5 gap-y-2 text-sm text-moon-500">
                <span className="inline-flex items-center gap-2">
                  <Globe className="size-4 text-moon-400" />
                  直连 {openaiCompatAccounts}
                </span>
                <span className="inline-flex items-center gap-2">
                  <Users className="size-4 text-moon-400" />
                  CPA {cpaAccounts}
                </span>
              </div>
            </div>
            <DataTable
              columns={requestColumns}
              rows={overview?.recent_requests ?? []}
              rowKey={(r) => r.id}
              empty="暂无最近请求"
            />
          </div>
        </div>
      </section>
    </div>
  );
}
