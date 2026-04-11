import { useEffect, useState } from "react";
import StatCard from "../components/StatCard";
import DataTable, { type Column } from "../components/DataTable";
import { luneGet } from "../lib/api";
import { oneapiGet } from "../lib/oneapi";
import { compact, shortDate } from "../lib/fmt";

type UsageSummary = {
  total_entries: number;
  successful: number;
  failed: number;
  by_account: Record<string, number>;
  by_token: Record<string, number>;
};

type OneAPILogStat = {
  date: string;
  model_name: string;
  quota: number;
  token_name: string;
  count: number;
};

type OneAPIDashboard = {
  total_quota: number;
  used_quota: number;
  remaining_quota: number;
};

export default function UsagePage() {
  const [usage, setUsage] = useState<UsageSummary | null>(null);
  const [logStats, setLogStats] = useState<OneAPILogStat[]>([]);
  const [dashboard, setDashboard] = useState<OneAPIDashboard | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    Promise.allSettled([
      luneGet<UsageSummary>("/admin/api/usage").then(setUsage),
      oneapiGet<{ data: OneAPILogStat[] }>("/api/log/stat?type=0").then((d) =>
        setLogStats(d.data ?? []),
      ),
      oneapiGet<{ data: OneAPIDashboard }>("/api/user/dashboard").then((d) =>
        setDashboard(d.data ?? null),
      ),
    ]).finally(() => setLoading(false));
  }, []);

  if (loading) {
    return <p className="py-12 text-center text-sm text-paper-300">加载中...</p>;
  }

  const statColumns: Column<OneAPILogStat>[] = [
    { key: "date", header: "日期", render: (r) => shortDate(r.date) },
    { key: "model", header: "模型", render: (r) => r.model_name },
    { key: "count", header: "次数", render: (r) => compact(r.count) },
    {
      key: "quota",
      header: "消耗额度",
      render: (r) => compact(r.quota),
    },
  ];

  const byAccountEntries = usage ? Object.entries(usage.by_account) : [];
  const byTokenEntries = usage ? Object.entries(usage.by_token) : [];

  return (
    <div className="space-y-8">
      <h2 className="text-xl font-semibold">用量</h2>

      {/* ── One-API quota overview ── */}
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-3">
        <StatCard
          label="总额度"
          value={compact(dashboard?.total_quota ?? 0)}
        />
        <StatCard
          label="已用额度"
          value={compact(dashboard?.used_quota ?? 0)}
        />
        <StatCard
          label="剩余额度"
          value={compact(dashboard?.remaining_quota ?? 0)}
        />
      </div>

      {/* ── 7-day model usage (One-API) ── */}
      <section>
        <h3 className="mb-3 text-sm font-medium text-paper-500">
          模型用量明细
        </h3>
        <div className="rounded-xl border border-paper-200 bg-paper-100 p-1">
          <DataTable
            columns={statColumns}
            rows={logStats}
            rowKey={(r) => `${r.date}-${r.model_name}`}
            empty="暂无用量数据"
          />
        </div>
      </section>

      {/* ── Lune-side usage ── */}
      <div className="grid gap-6 md:grid-cols-2">
        <section>
          <h3 className="mb-3 text-sm font-medium text-paper-500">
            按账号统计
          </h3>
          <div className="rounded-xl border border-paper-200 bg-paper-100 p-4 space-y-2">
            {byAccountEntries.length === 0 ? (
              <p className="text-sm text-paper-300">暂无数据</p>
            ) : (
              byAccountEntries.map(([name, count]) => (
                <div key={name} className="flex items-center justify-between text-sm">
                  <span className="text-paper-700">{name}</span>
                  <span className="text-paper-500">{compact(count)}</span>
                </div>
              ))
            )}
          </div>
        </section>

        <section>
          <h3 className="mb-3 text-sm font-medium text-paper-500">
            按令牌统计
          </h3>
          <div className="rounded-xl border border-paper-200 bg-paper-100 p-4 space-y-2">
            {byTokenEntries.length === 0 ? (
              <p className="text-sm text-paper-300">暂无数据</p>
            ) : (
              byTokenEntries.map(([name, count]) => (
                <div key={name} className="flex items-center justify-between text-sm">
                  <span className="text-paper-700">{name}</span>
                  <span className="text-paper-500">{compact(count)}</span>
                </div>
              ))
            )}
          </div>
        </section>
      </div>
    </div>
  );
}
