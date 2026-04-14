import { useEffect, useState } from "react";
import StatCard from "@/components/StatCard";
import StatusBadge from "@/components/StatusBadge";
import DataTable, { type Column } from "@/components/DataTable";
import PageHeader from "@/components/PageHeader";
import SectionHeading from "@/components/SectionHeading";
import { api } from "@/lib/api";
import { pct, latency, compact, relativeTime, shortDate } from "@/lib/fmt";
import { estimateCost, formatCost } from "@/lib/pricing";
import type { Overview, RequestLog } from "@/lib/types";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import {
  Users,
  Layers,
  Key,
  Activity,
  Zap,
  Server,
  AlertTriangle,
  RefreshCw,
  ShieldCheck,
  Globe,
} from "lucide-react";

const requestColumns: Column<RequestLog>[] = [
  {
    key: "time",
    header: "Time",
    render: (r) => (
      <span className="text-moon-500">{shortDate(r.created_at)}</span>
    ),
    tone: "secondary",
  },
  {
    key: "model",
    header: "Model",
    render: (r) => <span className="font-medium">{r.model_alias}</span>,
    tone: "primary",
  },
  {
    key: "token",
    header: "Token",
    render: (r) => (
      <span className="text-moon-500">{r.access_token_name}</span>
    ),
    tone: "secondary",
  },
  {
    key: "account",
    header: "Account",
    render: (r) => (
      <span className="text-moon-500">{r.account_label}</span>
    ),
    tone: "secondary",
  },
  {
    key: "status",
    header: "Status",
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
    header: "Latency",
    render: (r) => <span className="text-moon-500">{latency(r.latency_ms)}</span>,
    align: "right",
    tone: "numeric",
  },
  {
    key: "tokens",
    header: "Tokens",
    render: (r) => (
      <span className="text-moon-500">
        {r.input_tokens != null
          ? `${compact(r.input_tokens)}/${compact(r.output_tokens ?? 0)}`
          : "-"}
      </span>
    ),
    align: "right",
    tone: "numeric",
  },
  {
    key: "cost",
    header: "Est. Cost",
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
  const [lastSuccessAt, setLastSuccessAt] = useState<string | null>(null);
  const [refreshError, setRefreshError] = useState<string | null>(null);

  function load(silent = false) {
    if (!silent) setLoading(true);
    api
      .get<Overview>("/overview")
      .then((next) => {
        setData(next);
        setLastSuccessAt(new Date().toISOString());
        setRefreshError(null);
      })
      .catch((err) => {
        setRefreshError(err instanceof Error ? err.message : "Refresh failed");
      })
      .finally(() => {
        if (!silent) setLoading(false);
      });
  }

  useEffect(() => {
    load();
    const interval = setInterval(() => {
      load(true);
    }, 10_000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="space-y-8">
        <Skeleton className="h-24 rounded-[1.5rem]" />
        <div className="grid gap-4 xl:grid-cols-[minmax(0,1.35fr)_minmax(320px,0.95fr)]">
          <Skeleton className="h-56 rounded-[1.5rem]" />
          <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-1">
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} className="h-32 rounded-[1.5rem]" />
            ))}
          </div>
        </div>
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-32 rounded-[1.5rem]" />
          ))}
        </div>
        <div className="grid gap-6 xl:grid-cols-[minmax(320px,0.9fr)_minmax(0,1.25fr)]">
          <Skeleton className="h-72 rounded-[1.5rem]" />
          <Skeleton className="h-72 rounded-[1.5rem]" />
        </div>
      </div>
    );
  }

  const o = data;
  const totalUsage =
    (o?.token_usage_24h?.input ?? 0) + (o?.token_usage_24h?.output ?? 0);
  const healthyCount = o?.healthy_accounts ?? 0;
  const totalAccounts = o?.total_accounts ?? 0;
  const stale = Boolean(refreshError && lastSuccessAt);
  const cpaAccounts = o?.accounts_by_source?.cpa ?? 0;
  const openaiCompatAccounts = o?.accounts_by_source?.openai_compat ?? 0;

  return (
    <div className="space-y-8">
      <PageHeader
        eyebrow="Lune 控制台"
        title="总览"
        description="查看 CPA 连接状态、可路由账号单元以及当前请求健康度。"
        meta={
          <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
            <span>
              {healthyCount} 个健康账号，覆盖 {o?.total_pools ?? 0} 个池、{o?.total_tokens ?? 0} 个启用令牌。
            </span>
            <span>
              最近成功刷新：{lastSuccessAt ? relativeTime(lastSuccessAt) : "暂无"}
            </span>
            <span className={stale ? "text-amber-700" : "text-moon-500"}>
              数据状态：{stale ? "已过期" : "实时"}
            </span>
          </div>
        }
        actions={
          <Button size="sm" variant="outline" onClick={() => load()}>
            <RefreshCw className="size-4" />
            立即刷新
          </Button>
        }
      />

      {refreshError && (
        <section className="rounded-[1.5rem] border border-amber-200 bg-amber-50/90 p-4">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex items-start gap-3">
              <AlertTriangle className="mt-0.5 size-4 text-amber-700" />
              <div>
                <p className="text-sm font-medium text-amber-900">总览刷新失败</p>
                <p className="mt-1 text-sm text-amber-800/80">
                  {stale
                    ? `当前展示的是 ${relativeTime(lastSuccessAt!)} 的缓存数据。`
                    : "暂时没有可用的新鲜数据。"}
                </p>
              </div>
            </div>
            <Button size="sm" variant="outline" onClick={() => load()}>
              重试
            </Button>
          </div>
        </section>
      )}

      <section className="grid gap-4 xl:grid-cols-[minmax(0,1.35fr)_minmax(320px,0.95fr)]">
        <article className="relative overflow-hidden rounded-[1.6rem] border border-lunar-200/80 bg-[linear-gradient(145deg,rgba(249,251,255,0.98),rgba(233,240,255,0.96))] p-6 shadow-[0_24px_70px_-45px_rgba(46,76,142,0.45)] sm:p-7">
          <div className="absolute inset-x-0 top-0 h-px bg-gradient-to-r from-transparent via-lunar-500/60 to-transparent" />
          <div className="flex items-start justify-between gap-4">
            <div className="space-y-3">
              <p className="text-[11px] font-semibold uppercase tracking-[0.24em] text-lunar-700">CPA 核心状态</p>
              <div className="space-y-2">
                <div className="flex items-center gap-2">
                  <h2 className="text-3xl font-semibold tracking-tight text-moon-900 sm:text-[2.6rem]">
                    {o?.cpa_status?.label ?? "CPA 服务未连接"}
                  </h2>
                  {o?.cpa_status ? (
                    <StatusBadge
                      status={
                        o.cpa_status.status === "healthy"
                          ? "healthy"
                          : o.cpa_status.status === "error"
                            ? "error"
                            : "degraded"
                      }
                      label={o.cpa_status.status}
                    />
                  ) : (
                    <span className="rounded-full bg-red-100 px-2.5 py-1 text-xs font-medium text-red-700">
                      未配置
                    </span>
                  )}
                </div>
                <p className="max-w-[46ch] text-sm leading-6 text-moon-600">
                  {o?.cpa_status
                    ? `CPA 当前管理 ${o.cpa_status.accounts_total} 个账号单元，其中 ${o.cpa_status.accounts_healthy} 个可正常路由，${o.cpa_status.accounts_expiring} 个即将到期。`
                    : "配置 CPA 服务后，可启用 Provider Channel、Device Code 登录和账号导入能力。"}
                </p>
              </div>
            </div>
            <span className="flex size-11 items-center justify-center rounded-2xl border border-lunar-200 bg-white/80 text-lunar-700">
              <Server className="size-5" />
            </span>
          </div>

          <div className="mt-6 grid gap-3 sm:grid-cols-3">
            <div className="rounded-2xl border border-white/80 bg-white/70 p-4">
              <p className="text-[11px] font-semibold uppercase tracking-[0.22em] text-moon-400">CPA 账号单元</p>
              <p className="mt-2 text-2xl font-semibold text-moon-900" style={{ fontFamily: '"Iowan Old Style","Palatino Linotype","Noto Serif SC",Georgia,serif' }}>
                {compact(cpaAccounts)}
              </p>
              <p className="mt-1 text-xs leading-5 text-moon-500">当前由 CPA 服务托管的可路由单元。</p>
            </div>
            <div className="rounded-2xl border border-white/80 bg-white/70 p-4">
              <p className="text-[11px] font-semibold uppercase tracking-[0.22em] text-moon-400">健康中</p>
              <p className="mt-2 text-2xl font-semibold text-moon-900" style={{ fontFamily: '"Iowan Old Style","Palatino Linotype","Noto Serif SC",Georgia,serif' }}>
                {compact(o?.cpa_status?.accounts_healthy ?? 0)}
              </p>
              <p className="mt-1 text-xs leading-5 text-moon-500">当前可参与路由的 CPA 账号单元。</p>
            </div>
            <div className="rounded-2xl border border-white/80 bg-white/70 p-4">
              <p className="text-[11px] font-semibold uppercase tracking-[0.22em] text-moon-400">最近检查</p>
              <p className="mt-2 text-2xl font-semibold text-moon-900" style={{ fontFamily: '"Iowan Old Style","Palatino Linotype","Noto Serif SC",Georgia,serif' }}>
                {o?.cpa_status?.last_checked_at ? relativeTime(o.cpa_status.last_checked_at) : "从未"}
              </p>
              <p className="mt-1 text-xs leading-5 text-moon-500">控制台最近一次记录到的 CPA 健康检查时间。</p>
            </div>
          </div>
        </article>

        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-1">
          <StatCard
            label="请求成功率"
            value={pct(o?.success_rate_24h ?? 0)}
            sub="过去 24 小时全量请求的成功率。"
            icon={ShieldCheck}
            variant="hero"
          />
          <StatCard
            label="24 小时请求量"
            value={compact(o?.requests_24h ?? 0)}
            sub="最近一天观察到的请求总数。"
            icon={Activity}
          />
          <StatCard
            label="24 小时 Token 吞吐"
            value={compact(totalUsage)}
            sub={`${compact(o?.token_usage_24h?.input ?? 0)} input / ${compact(o?.token_usage_24h?.output ?? 0)} output`}
            icon={Zap}
          />
        </div>
      </section>

      <section className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard
          label="账号总数"
          value={String(totalAccounts)}
          sub="所有来源下已配置的路由账号单元。"
          icon={Users}
          variant="compact"
        />
        <StatCard
          label="直连 API 账号"
          value={String(openaiCompatAccounts)}
          sub="由 Lune 直接管理的 OpenAI-Compatible 账号。"
          icon={Globe}
          variant="compact"
        />
        <StatCard
          label="池"
          value={String(o?.total_pools ?? 0)}
          sub="当前可选的路由池。"
          icon={Layers}
          variant="compact"
        />
        <StatCard
          label="启用令牌"
          value={String(o?.total_tokens ?? 0)}
          sub="当前可用的客户端访问令牌。"
          icon={Key}
          variant="compact"
        />
      </section>

      <section className="grid gap-6 xl:grid-cols-[minmax(320px,0.9fr)_minmax(0,1.25fr)]">
        <div className="space-y-4">
          <SectionHeading
            title="账号健康度"
            description="查看账号是否可用、最近检查时间以及当前阻塞错误。"
          />
          <div className="overflow-hidden rounded-[1.6rem] border border-moon-200/70 bg-white/85">
            {(!o?.account_health || o.account_health.length === 0) && (
              <p className="py-10 text-center text-sm text-moon-400">
                暂未配置账号
              </p>
            )}
            {o?.account_health?.map((a, index) => (
              <div
                key={a.id}
                className={`flex flex-col gap-3 px-5 py-4 sm:flex-row sm:items-start sm:justify-between ${
                  index > 0 ? "border-t border-moon-200/60" : ""
                } ${a.status === "disabled" ? "opacity-60" : ""}`}
              >
                <div className="space-y-2">
                  <div className="flex items-center gap-3">
                    <StatusBadge
                      status={
                        a.status as
                          | "healthy"
                          | "degraded"
                          | "error"
                          | "disabled"
                      }
                    />
                    <span className="font-medium text-moon-800">{a.label}</span>
                  </div>
                  <p className="text-xs uppercase tracking-[0.18em] text-moon-400">
                    {a.last_checked_at
                      ? `Last checked ${relativeTime(a.last_checked_at)}`
                      : "尚未检查"}
                  </p>
                </div>
                <div className="max-w-sm text-sm text-moon-500 sm:text-right">
                  {a.last_error ? (
                    <p className="text-status-red">{a.last_error}</p>
                  ) : (
                    <p>当前未发现错误。</p>
                  )}
                </div>
              </div>
            ))}
          </div>
        </div>

        <div className="space-y-4">
          <SectionHeading
            title="最近请求"
            description="查看最新流量样本，包括模型别名、令牌和上游账号。"
          />
          <div className="overflow-hidden rounded-[1.6rem] border border-moon-200/70 bg-white/85">
            <DataTable
              columns={requestColumns}
              rows={o?.recent_requests ?? []}
              rowKey={(r) => r.id}
              empty="暂无最近请求"
            />
          </div>
        </div>
      </section>
    </div>
  );
}
