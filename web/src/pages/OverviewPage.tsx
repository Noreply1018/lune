import { useEffect, useState } from "react";
import StatCard from "@/components/StatCard";
import StatusBadge from "@/components/StatusBadge";
import DataTable, { type Column } from "@/components/DataTable";
import { api } from "@/lib/api";
import { pct, latency, compact, relativeTime, shortDate } from "@/lib/fmt";
import type { Overview, RequestLog } from "@/lib/types";
import { Card, CardContent } from "@/components/ui/card";
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
  },
  {
    key: "model",
    header: "Model",
    render: (r) => <span className="font-medium">{r.model_alias}</span>,
  },
  {
    key: "token",
    header: "Token",
    render: (r) => (
      <span className="text-moon-500">{r.access_token_name}</span>
    ),
  },
  {
    key: "account",
    header: "Account",
    render: (r) => (
      <span className="text-moon-500">{r.account_label}</span>
    ),
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
  },
  {
    key: "latency",
    header: "Latency",
    render: (r) => <span className="text-moon-500">{latency(r.latency_ms)}</span>,
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
        <Skeleton className="h-7 w-28" />
        <div className="grid grid-cols-2 gap-4 lg:grid-cols-3 xl:grid-cols-6">
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="h-28" />
          ))}
        </div>
        <Skeleton className="h-48" />
        <Skeleton className="h-48" />
      </div>
    );
  }

  const o = data;

  return (
    <div className="space-y-8">
      <h2 className="text-xl font-semibold">Overview</h2>

      <div className="grid grid-cols-2 gap-4 lg:grid-cols-3 xl:grid-cols-6">
        <StatCard
          label="Accounts"
          value={`${o?.healthy_accounts ?? 0} / ${o?.total_accounts ?? 0}`}
          sub="healthy"
          icon={Users}
        />
        <StatCard
          label="Pools"
          value={String(o?.total_pools ?? 0)}
          sub="enabled"
          icon={Layers}
        />
        <StatCard
          label="Tokens"
          value={String(o?.total_tokens ?? 0)}
          sub="active"
          icon={Key}
        />
        <StatCard
          label="Requests"
          value={compact(o?.requests_24h ?? 0)}
          sub="24h"
          icon={Activity}
        />
        <StatCard
          label="Token Usage"
          value={compact(
            (o?.token_usage_24h?.input ?? 0) +
              (o?.token_usage_24h?.output ?? 0),
          )}
          sub="24h"
          icon={Zap}
        />
        <StatCard
          label="Success Rate"
          value={pct(o?.success_rate_24h ?? 0)}
          sub="24h"
          icon={TrendingUp}
        />
      </div>

      <section>
        <h3 className="mb-4 text-sm font-medium uppercase tracking-wider text-moon-400">
          Account Health
        </h3>
        <Card className="ring-1 ring-moon-200/60">
          <CardContent className="divide-y divide-moon-200/60 p-0">
            {(!o?.account_health || o.account_health.length === 0) && (
              <p className="py-8 text-center text-sm text-moon-400">
                No accounts configured
              </p>
            )}
            {o?.account_health?.map((a) => (
              <div
                key={a.id}
                className={`flex items-center justify-between px-5 py-3 ${a.status === "disabled" ? "opacity-60" : ""}`}
              >
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
                <div className="text-right">
                  <span className="text-xs text-moon-400">
                    {a.last_checked_at
                      ? `checked ${relativeTime(a.last_checked_at)}`
                      : "never checked"}
                  </span>
                  {a.last_error && (
                    <p className="text-xs text-status-red">{a.last_error}</p>
                  )}
                </div>
              </div>
            ))}
          </CardContent>
        </Card>
      </section>

      <section>
        <h3 className="mb-4 text-sm font-medium uppercase tracking-wider text-moon-400">
          Recent Requests
        </h3>
        <Card className="ring-1 ring-moon-200/60">
          <CardContent className="p-1">
            <DataTable
              columns={requestColumns}
              rows={o?.recent_requests ?? []}
              rowKey={(r) => r.id}
              empty="No recent requests"
            />
          </CardContent>
        </Card>
      </section>
    </div>
  );
}
