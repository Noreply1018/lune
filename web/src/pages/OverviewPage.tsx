import { useEffect, useState } from "react";
import StatCard from "@/components/StatCard";
import StatusBadge from "@/components/StatusBadge";
import DataTable, { type Column } from "@/components/DataTable";
import PageHeader from "@/components/PageHeader";
import SectionHeading from "@/components/SectionHeading";
import { api } from "@/lib/api";
import { pct, latency, compact, relativeTime, shortDate } from "@/lib/fmt";
import type { Overview, RequestLog } from "@/lib/types";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Users,
  Layers,
  Key,
  Activity,
  Zap,
  TrendingUp,
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
];

export default function OverviewPage() {
  const [data, setData] = useState<Overview | null>(null);
  const [loading, setLoading] = useState(true);

  function load() {
    api
      .get<Overview>("/overview")
      .then(setData)
      .catch(() => {})
      .finally(() => setLoading(false));
  }

  useEffect(() => {
    load();
    const interval = setInterval(() => {
      api.get<Overview>("/overview").then(setData).catch(() => {});
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
  const degradedCount = Math.max(totalAccounts - healthyCount, 0);

  return (
    <div className="space-y-8">
      <PageHeader
        eyebrow="Moonlight Console"
        title="Overview"
        description="Operational snapshot for routing health, request reliability, and current system capacity."
        meta={
          <span>
            {healthyCount} healthy accounts across {o?.total_pools ?? 0} pools
            and {o?.total_tokens ?? 0} active tokens.
          </span>
        }
      />

      <section className="grid gap-4 xl:grid-cols-[minmax(0,1.35fr)_minmax(320px,0.95fr)]">
        <StatCard
          label="Request reliability"
          value={pct(o?.success_rate_24h ?? 0)}
          sub="24 hour success rate across the full request stream."
          icon={TrendingUp}
          variant="hero"
          className="bg-[linear-gradient(145deg,rgba(255,255,255,0.96),rgba(240,242,248,0.92))]"
        />
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-1">
          <StatCard
            label="24h request volume"
            value={compact(o?.requests_24h ?? 0)}
            sub="Requests observed in the last day."
            icon={Activity}
          />
          <StatCard
            label="24h token throughput"
            value={compact(totalUsage)}
            sub={`${compact(o?.token_usage_24h?.input ?? 0)} input / ${compact(o?.token_usage_24h?.output ?? 0)} output`}
            icon={Zap}
          />
          <StatCard
            label="Fleet health"
            value={`${healthyCount}/${totalAccounts}`}
            sub={
              degradedCount > 0
                ? `${degradedCount} account${degradedCount > 1 ? "s" : ""} need attention.`
                : "All configured accounts are currently healthy."
            }
            icon={Users}
          />
        </div>
      </section>

      <section className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard
          label="Accounts"
          value={String(totalAccounts)}
          sub="Configured upstream accounts."
          icon={Users}
          variant="compact"
        />
        <StatCard
          label="Pools"
          value={String(o?.total_pools ?? 0)}
          sub="Routing pools available for selection."
          icon={Layers}
          variant="compact"
        />
        <StatCard
          label="Tokens"
          value={String(o?.total_tokens ?? 0)}
          sub="Client access tokens currently active."
          icon={Key}
          variant="compact"
        />
        <StatCard
          label="Failure slice"
          value={`${degradedCount}`}
          sub="Accounts not currently reporting healthy."
          icon={TrendingUp}
          variant="compact"
        />
      </section>

      <section className="grid gap-6 xl:grid-cols-[minmax(320px,0.9fr)_minmax(0,1.25fr)]">
        <div className="space-y-4">
          <SectionHeading
            title="Account Health"
            description="Live account readiness, last check time, and any blocking error detail."
          />
          <div className="overflow-hidden rounded-[1.6rem] border border-moon-200/70 bg-white/85">
            {(!o?.account_health || o.account_health.length === 0) && (
              <p className="py-10 text-center text-sm text-moon-400">
                No accounts configured
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
                      : "Never checked"}
                  </p>
                </div>
                <div className="max-w-sm text-sm text-moon-500 sm:text-right">
                  {a.last_error ? (
                    <p className="text-status-red">{a.last_error}</p>
                  ) : (
                    <p>No active error reported.</p>
                  )}
                </div>
              </div>
            ))}
          </div>
        </div>

        <div className="space-y-4">
          <SectionHeading
            title="Recent Requests"
            description="Latest traffic samples across model aliases, tokens, and upstream accounts."
          />
          <div className="overflow-hidden rounded-[1.6rem] border border-moon-200/70 bg-white/85">
            <DataTable
              columns={requestColumns}
              rows={o?.recent_requests ?? []}
              rowKey={(r) => r.id}
              empty="No recent requests"
            />
          </div>
        </div>
      </section>
    </div>
  );
}
