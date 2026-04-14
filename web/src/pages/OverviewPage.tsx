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
    header: "时间",
    render: (r) => (
      <span className="text-moon-500">{shortDate(r.created_at)}</span>
    ),
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
    render: (r) => (
      <span className="text-moon-500">{r.access_token_name}</span>
    ),
    tone: "secondary",
  },
  {
    key: "account",
    header: "账号",
    render: (r) => (
      <span className="text-moon-500">{r.account_label}</span>
    ),
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
    key: "tokens",
    header: "Token 用量",
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
  const totalAccounts = o?.total_accounts ?? 0;
  const cpaAccounts = o?.accounts_by_source?.cpa ?? 0;
  const openaiCompatAccounts = o?.accounts_by_source?.openai_compat ?? 0;

  return (
    <div className="space-y-8">
      <PageHeader
        eyebrow="Lune 控制台"
        title="总览"
        actions={
          <Button size="sm" variant="outline" onClick={() => load()}>
            <RefreshCw className="size-4" />
            刷新
          </Button>
        }
      />

      {refreshError && (
        <section className="rounded-[1.5rem] border border-amber-200 bg-amber-50/90 p-4">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex items-start gap-3">
              <AlertTriangle className="mt-0.5 size-4 text-amber-700" />
              <p className="text-sm font-medium text-amber-900">总览刷新失败</p>
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
              <div className="flex items-center gap-3">
                <h2 className="text-3xl font-semibold tracking-tight text-moon-900 sm:text-[2.6rem]">
                  {o?.cpa_status?.label ?? "未连接"}
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
            </div>
            <span className="flex size-11 items-center justify-center rounded-2xl border border-lunar-200 bg-white/80 text-lunar-700">
              <Server className="size-5" />
            </span>
          </div>

          {o?.cpa_status ? (
            <div className="mt-6 flex flex-wrap gap-3">
              <div className="flex items-center gap-2 rounded-full border border-white/80 bg-white/70 px-4 py-2">
                <span className="text-[11px] font-semibold uppercase tracking-[0.18em] text-moon-400">账号单元</span>
                <span className="text-lg font-semibold text-moon-900" style={{ fontFamily: '"Iowan Old Style","Palatino Linotype","Noto Serif SC",Georgia,serif' }}>
                  {compact(cpaAccounts)}
                </span>
              </div>
              <div className="flex items-center gap-2 rounded-full border border-white/80 bg-white/70 px-4 py-2">
                <span className="text-[11px] font-semibold uppercase tracking-[0.18em] text-moon-400">健康</span>
                <span className="text-lg font-semibold text-moon-900" style={{ fontFamily: '"Iowan Old Style","Palatino Linotype","Noto Serif SC",Georgia,serif' }}>
                  {compact(o.cpa_status.accounts_healthy)}
                </span>
              </div>
              <div className="flex items-center gap-2 rounded-full border border-white/80 bg-white/70 px-4 py-2">
                <span className="text-[11px] font-semibold uppercase tracking-[0.18em] text-moon-400">即将到期</span>
                <span className="text-lg font-semibold text-moon-900" style={{ fontFamily: '"Iowan Old Style","Palatino Linotype","Noto Serif SC",Georgia,serif' }}>
                  {compact(o.cpa_status.accounts_expiring)}
                </span>
              </div>
              {o.cpa_status.last_checked_at && (
                <div className="flex items-center gap-2 rounded-full border border-white/80 bg-white/70 px-4 py-2">
                  <span className="text-[11px] font-semibold uppercase tracking-[0.18em] text-moon-400">检查于</span>
                  <span className="text-sm font-medium text-moon-600">{relativeTime(o.cpa_status.last_checked_at)}</span>
                </div>
              )}
            </div>
          ) : (
            <p className="mt-4 text-sm text-moon-500">配置 CPA 服务后可启用 Provider Channel 和 Device Code 登录。</p>
          )}
        </article>

        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-1">
          <StatCard
            label="请求成功率"
            value={pct(o?.success_rate_24h ?? 0)}
            sub="24h"
            icon={ShieldCheck}
            variant="hero"
          />
          <StatCard
            label="24h 请求量"
            value={compact(o?.requests_24h ?? 0)}
            icon={Activity}
          />
          <StatCard
            label="24h Token"
            value={compact(totalUsage)}
            sub={`${compact(o?.token_usage_24h?.input ?? 0)} in / ${compact(o?.token_usage_24h?.output ?? 0)} out`}
            icon={Zap}
          />
        </div>
      </section>

      <section className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard
          label="账号"
          value={String(totalAccounts)}
          icon={Users}
          variant="compact"
        />
        <StatCard
          label="直连 API"
          value={String(openaiCompatAccounts)}
          icon={Globe}
          variant="compact"
        />
        <StatCard
          label="池"
          value={String(o?.total_pools ?? 0)}
          icon={Layers}
          variant="compact"
        />
        <StatCard
          label="令牌"
          value={String(o?.total_tokens ?? 0)}
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
                      ? `最近检查 ${relativeTime(a.last_checked_at)}`
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
