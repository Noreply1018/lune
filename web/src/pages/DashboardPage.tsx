import { useEffect, useState } from "react";
import StatCard from "../components/StatCard";
import StatusBadge from "../components/StatusBadge";
import DataTable, { type Column } from "../components/DataTable";
import { luneGet } from "../lib/api";
import { pct, latency, shortDate } from "../lib/fmt";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";

type Overview = {
  accounts_total: number;
  active_accounts: number;
  pools_total: number;
  api_keys_total: number;
  total_requests: number;
  success_requests: number;
  success_rate: number;
  average_latency_ms: number;
};

type LogRecord = {
  id: number;
  created_at: string;
  model_alias: string;
  target_model: string;
  account_id: string;
  status: string;
  latency_ms: number;
  access_token_name: string;
};

const logColumns: Column<LogRecord>[] = [
  { key: "time", header: "Time", render: (r) => shortDate(r.created_at) },
  { key: "model", header: "Model", render: (r) => r.model_alias },
  { key: "account", header: "Account", render: (r) => r.account_id },
  {
    key: "status",
    header: "Status",
    render: (r) => (
      <StatusBadge
        status={r.status === "success" ? "ok" : "error"}
        label={r.status}
      />
    ),
  },
  { key: "latency", header: "Latency", render: (r) => latency(r.latency_ms) },
];

export default function DashboardPage() {
  const [overview, setOverview] = useState<Overview | null>(null);
  const [logs, setLogs] = useState<LogRecord[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    Promise.allSettled([
      luneGet<Overview>("/admin/api/overview").then(setOverview),
      luneGet<{ logs: LogRecord[] }>("/admin/api/logs?limit=10").then((d) =>
        setLogs(d.logs ?? []),
      ),
    ]).finally(() => setLoading(false));
  }, []);

  if (loading) {
    return (
      <div className="space-y-8">
        <Skeleton className="h-7 w-20" />
        <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-24" />
          ))}
        </div>
        <Skeleton className="h-48" />
      </div>
    );
  }

  const o = overview;

  return (
    <div className="space-y-8">
      <h2 className="text-xl font-semibold">Overview</h2>

      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatCard
          label="Total Requests"
          value={String(o?.total_requests ?? 0)}
          sub={`Success ${o?.success_requests ?? 0}`}
        />
        <StatCard label="Success Rate" value={pct(o?.success_rate ?? 0)} />
        <StatCard
          label="Avg Latency"
          value={latency(o?.average_latency_ms ?? 0)}
        />
        <StatCard
          label="Active Accounts"
          value={`${o?.active_accounts ?? 0} / ${o?.accounts_total ?? 0}`}
          sub={`${o?.pools_total ?? 0} pools`}
        />
      </div>

      <section>
        <h3 className="mb-3 text-sm font-medium text-muted-foreground">
          Recent Requests
        </h3>
        <Card>
          <CardContent className="p-1">
            <DataTable columns={logColumns} rows={logs} rowKey={(r) => r.id} />
          </CardContent>
        </Card>
      </section>
    </div>
  );
}
