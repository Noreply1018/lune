import { useEffect, useState } from "react";
import StatCard from "../components/StatCard";
import DataTable, { type Column } from "../components/DataTable";
import { luneGet } from "../lib/api";
import { backendGet } from "../lib/backend";
import { compact, shortDate } from "../lib/fmt";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Separator } from "@/components/ui/separator";

type UsageSummary = {
  total_entries: number;
  successful: number;
  failed: number;
  by_account: Record<string, number>;
  by_token: Record<string, number>;
};

type BackendLogStat = {
  date: string;
  model_name: string;
  quota: number;
  token_name: string;
  count: number;
};

type BackendDashboard = {
  total_quota: number;
  used_quota: number;
  remaining_quota: number;
};

export default function UsagePage() {
  const [usage, setUsage] = useState<UsageSummary | null>(null);
  const [logStats, setLogStats] = useState<BackendLogStat[]>([]);
  const [dashboard, setDashboard] = useState<BackendDashboard | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    Promise.allSettled([
      luneGet<UsageSummary>("/admin/api/usage").then(setUsage),
      backendGet<{ data: BackendLogStat[] }>("/api/log/stat?type=0").then((d) =>
        setLogStats(d.data ?? []),
      ),
      backendGet<{ data: BackendDashboard }>("/api/user/dashboard").then((d) =>
        setDashboard(d.data ?? null),
      ),
    ]).finally(() => setLoading(false));
  }, []);

  if (loading) {
    return (
      <div className="space-y-8">
        <Skeleton className="h-7 w-20" />
        <div className="grid grid-cols-2 gap-4 lg:grid-cols-3">
          {Array.from({ length: 3 }).map((_, i) => (
            <Skeleton key={i} className="h-24" />
          ))}
        </div>
        <Skeleton className="h-48" />
      </div>
    );
  }

  const statColumns: Column<BackendLogStat>[] = [
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

      <section>
        <h3 className="mb-3 text-sm font-medium text-muted-foreground">
          模型用量明细
        </h3>
        <Card>
          <CardContent className="p-1">
            <DataTable
              columns={statColumns}
              rows={logStats}
              rowKey={(r) => `${r.date}-${r.model_name}`}
              empty="暂无用量数据"
            />
          </CardContent>
        </Card>
      </section>

      <Separator />

      <div className="grid gap-6 md:grid-cols-2">
        <section>
          <h3 className="mb-3 text-sm font-medium text-muted-foreground">
            按账号统计
          </h3>
          <Card>
            <CardContent className="p-4 space-y-2">
              {byAccountEntries.length === 0 ? (
                <p className="text-sm text-muted-foreground">暂无数据</p>
              ) : (
                byAccountEntries.map(([name, count]) => (
                  <div
                    key={name}
                    className="flex items-center justify-between text-sm"
                  >
                    <span className="font-medium">{name}</span>
                    <span className="text-muted-foreground">
                      {compact(count)}
                    </span>
                  </div>
                ))
              )}
            </CardContent>
          </Card>
        </section>

        <section>
          <h3 className="mb-3 text-sm font-medium text-muted-foreground">
            按令牌统计
          </h3>
          <Card>
            <CardContent className="p-4 space-y-2">
              {byTokenEntries.length === 0 ? (
                <p className="text-sm text-muted-foreground">暂无数据</p>
              ) : (
                byTokenEntries.map(([name, count]) => (
                  <div
                    key={name}
                    className="flex items-center justify-between text-sm"
                  >
                    <span className="font-medium">{name}</span>
                    <span className="text-muted-foreground">
                      {compact(count)}
                    </span>
                  </div>
                ))
              )}
            </CardContent>
          </Card>
        </section>
      </div>
    </div>
  );
}
