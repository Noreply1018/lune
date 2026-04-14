import { useEffect, useState } from "react";
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
  const [loading, setLoading] = useState(true);
  const [refreshError, setRefreshError] = useState<string | null>(null);

  function load(silent = false) {
    if (!silent) setLoading(true);
    api
      .get<Overview>("/overview")
      .then((next) => {
        setData(next);
        setRefreshError(null);
      })
      .catch((err) => {
        setRefreshError(err instanceof Error ? err.message : "刷新失败");
      })
      .finally(() => {
        if (!silent) setLoading(false);
      });
  }

  useEffect(() => {
    load();
    const interval = setInterval(() => load(true), 10_000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
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

  return (
    <div className="space-y-10">
      <PageHeader
        eyebrow="Overview / Console"
        title="总览"
        description="一眼看清供给、健康与最近 24 小时流量。"
        meta={
          <>
            <span>账号 {overview?.total_accounts ?? 0}</span>
            <span>池 {overview?.total_pools ?? 0}</span>
            <span>访问令牌 {overview?.total_tokens ?? 0}</span>
          </>
        }
        actions={
          <Button size="sm" variant="outline" onClick={() => load()}>
            <RefreshCw className="size-4" />
            刷新
          </Button>
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
            <Button size="sm" variant="outline" onClick={() => load()}>
              重试
            </Button>
          </div>
        </section>
      )}

      <section className="surface-section hero-glow relative overflow-hidden px-6 py-6 sm:px-7 sm:py-7">
        <div className="absolute right-[-3rem] top-[-2rem] h-44 w-44 rounded-full bg-[radial-gradient(circle,rgba(255,255,255,0.96),rgba(255,255,255,0)_68%)] blur-xl" />
        <div className="absolute left-[42%] top-12 h-48 w-48 rounded-full bg-[radial-gradient(circle,rgba(134,125,193,0.18),rgba(134,125,193,0)_72%)] blur-3xl" />
        <div className="grid gap-8 xl:grid-cols-[minmax(0,1.18fr)_minmax(320px,0.82fr)]">
          <div className="space-y-8">
            <div className="space-y-4">
              <p className="eyebrow-label">Moonlight Surface</p>
              <div className="space-y-3">
                <h2 className="font-editorial text-[2.4rem] font-semibold tracking-[-0.065em] text-moon-800 sm:text-[3.45rem]">
                  今夜的网关
                </h2>
                <p className="max-w-2xl text-sm leading-7 text-moon-500 sm:text-[15px]">
                  当前控制面、账号供给与最近请求都收拢在这一屏里。先看状态，再看流量，不让首页变成一面卡片墙。
                </p>
              </div>
            </div>

            <div className="grid gap-3 sm:grid-cols-3">
              <div className="rounded-[1.25rem] border border-white/72 bg-white/62 px-4 py-4">
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

              <div className="rounded-[1.25rem] border border-white/72 bg-white/62 px-4 py-4">
                <p className="kicker">供给概况</p>
                <p className="mt-3 text-[1.5rem] font-semibold tracking-[-0.05em] text-moon-800">
                  {overview?.total_accounts ?? 0}
                </p>
                <p className="mt-2 text-sm text-moon-500">
                  直连 {openaiCompatAccounts} · CPA {cpaAccounts}
                </p>
              </div>

              <div className="rounded-[1.25rem] border border-white/72 bg-white/62 px-4 py-4">
                <p className="kicker">24h 请求</p>
                <p className="mt-3 text-[1.5rem] font-semibold tracking-[-0.05em] text-moon-800">
                  {compact(overview?.requests_24h ?? 0)}
                </p>
                <p className="mt-2 text-sm text-moon-500">
                  成功率 {pct(overview?.success_rate_24h ?? 0)}
                </p>
              </div>
            </div>

            <div className="grid gap-3 md:grid-cols-4">
              {[
                {
                  label: "可用账号",
                  value: compact(overview?.healthy_accounts ?? 0),
                  icon: Users,
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
              ].map((item) => (
                <div
                  key={item.label}
                  className="rounded-[1.15rem] border border-white/70 bg-white/56 px-4 py-3"
                >
                  <div className="flex items-center justify-between gap-3">
                    <p className="text-xs tracking-[0.16em] text-moon-400">{item.label}</p>
                    <item.icon className="size-4 text-moon-400" />
                  </div>
                  <p className="mt-2 text-lg font-semibold tracking-[-0.04em] text-moon-800">
                    {item.value}
                  </p>
                </div>
              ))}
            </div>
          </div>

          <aside className="space-y-4">
            <div className="rounded-[1.45rem] border border-white/72 bg-white/70 px-5 py-5">
              <div className="flex items-center justify-between gap-3 border-b border-moon-200/60 pb-4">
                <div>
                  <p className="kicker">当前控制平面</p>
                  <p className="mt-1 text-sm text-moon-500">Default CPA 与账号健康摘要</p>
                </div>
                <ShieldCheck className="size-4 text-lunar-600" />
              </div>

              <div className="space-y-4 pt-4">
                <div className="flex items-center justify-between gap-4">
                  <span className="text-sm text-moon-500">服务</span>
                  <span className="font-medium text-moon-700">
                    {serviceStatus?.label ?? "未配置"}
                  </span>
                </div>
                <div className="flex items-center justify-between gap-4">
                  <span className="text-sm text-moon-500">健康账号</span>
                  <span className="font-medium text-moon-700">
                    {serviceStatus?.accounts_healthy ?? 0}
                  </span>
                </div>
                <div className="flex items-center justify-between gap-4">
                  <span className="text-sm text-moon-500">即将到期</span>
                  <span className="font-medium text-moon-700">
                    {serviceStatus?.accounts_expiring ?? 0}
                  </span>
                </div>
                <div className="flex items-center justify-between gap-4">
                  <span className="text-sm text-moon-500">直接接入</span>
                  <span className="font-medium text-moon-700">
                    {openaiCompatAccounts}
                  </span>
                </div>
              </div>
            </div>

            <div className="rounded-[1.45rem] border border-white/72 bg-[linear-gradient(180deg,rgba(243,239,250,0.92),rgba(255,255,255,0.74))] px-5 py-5">
              <p className="kicker">24 小时摘要</p>
              <div className="mt-4 space-y-4">
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
