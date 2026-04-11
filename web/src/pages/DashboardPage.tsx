import { useEffect, useState } from "react";
import StatCard from "../components/StatCard";
import StatusBadge from "../components/StatusBadge";
import DataTable, { type Column } from "../components/DataTable";
import { luneGet } from "../lib/api";
import { backendGet } from "../lib/backend";
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

type BackendChannel = {
  id: number;
  name: string;
  status: number;
  type: number;
};

const logColumns: Column<LogRecord>[] = [
  { key: "time", header: "时间", render: (r) => shortDate(r.created_at) },
  { key: "model", header: "模型", render: (r) => r.model_alias },
  { key: "account", header: "账号", render: (r) => r.account_id },
  {
    key: "status",
    header: "状态",
    render: (r) => (
      <StatusBadge
        status={r.status === "success" ? "ok" : "error"}
        label={r.status}
      />
    ),
  },
  { key: "latency", header: "延迟", render: (r) => latency(r.latency_ms) },
];

export default function DashboardPage() {
  const [overview, setOverview] = useState<Overview | null>(null);
  const [logs, setLogs] = useState<LogRecord[]>([]);
  const [channels, setChannels] = useState<BackendChannel[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    Promise.allSettled([
      luneGet<Overview>("/admin/api/overview").then(setOverview),
      luneGet<{ logs: LogRecord[] }>("/admin/api/logs?limit=10").then((d) =>
        setLogs(d.logs ?? []),
      ),
      backendGet<{ data: BackendChannel[] }>("/api/channel/?p=0&page_size=100").then(
        (d) => setChannels(d.data ?? []),
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
      <h2 className="text-xl font-semibold">总览</h2>

      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatCard
          label="总请求"
          value={String(o?.total_requests ?? 0)}
          sub={`成功 ${o?.success_requests ?? 0}`}
        />
        <StatCard label="成功率" value={pct(o?.success_rate ?? 0)} />
        <StatCard
          label="平均延迟"
          value={latency(o?.average_latency_ms ?? 0)}
        />
        <StatCard
          label="活跃账号"
          value={`${o?.active_accounts ?? 0} / ${o?.accounts_total ?? 0}`}
          sub={`${o?.pools_total ?? 0} 号池`}
        />
      </div>

      <section>
        <h3 className="mb-3 text-sm font-medium text-muted-foreground">
          渠道状态
        </h3>
        {channels.length === 0 ? (
          <p className="text-sm text-muted-foreground">暂无渠道数据</p>
        ) : (
          <div className="flex flex-wrap gap-2">
            {channels.map((ch) => (
              <div
                key={ch.id}
                className="flex items-center gap-2 rounded-lg border px-3 py-1.5 text-sm"
              >
                <span className="font-medium">{ch.name}</span>
                <StatusBadge
                  status={ch.status === 1 ? "ok" : "disabled"}
                  label={ch.status === 1 ? "正常" : "停用"}
                />
              </div>
            ))}
          </div>
        )}
      </section>

      <section>
        <h3 className="mb-3 text-sm font-medium text-muted-foreground">
          最近请求
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
