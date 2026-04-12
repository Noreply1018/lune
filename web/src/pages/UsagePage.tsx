import { useEffect, useState } from "react";
import StatCard from "@/components/StatCard";
import DataTable, { type Column } from "@/components/DataTable";
import StatusBadge from "@/components/StatusBadge";
import { api } from "@/lib/api";
import { toast } from "@/components/Feedback";
import { compact, latency, shortDate } from "@/lib/fmt";
import type { UsageStats, RequestLog, Account, AccessToken } from "@/lib/types";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Activity, ArrowDownToLine, ArrowUpFromLine } from "lucide-react";

const TIME_RANGES = [
  { value: "1h", label: "Last 1h" },
  { value: "24h", label: "Last 24h" },
  { value: "7d", label: "Last 7d" },
  { value: "30d", label: "Last 30d" },
  { value: "all", label: "All time" },
];

const logColumns: Column<RequestLog>[] = [
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
    key: "io",
    header: "In / Out",
    render: (r) =>
      r.input_tokens != null ? (
        <span className="text-moon-500">
          {compact(r.input_tokens)} / {compact(r.output_tokens ?? 0)}
        </span>
      ) : (
        <span className="text-moon-400">-</span>
      ),
  },
  {
    key: "latency",
    header: "Latency",
    render: (r) => (
      <span className="text-moon-500">{latency(r.latency_ms)}</span>
    ),
  },
];

export default function UsagePage() {
  const [stats, setStats] = useState<UsageStats | null>(null);
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [tokensList, setTokensList] = useState<AccessToken[]>([]);
  const [loading, setLoading] = useState(true);

  const [range, setRange] = useState("24h");
  const [filterToken, setFilterToken] = useState("all");
  const [filterAccount, setFilterAccount] = useState("all");
  const [filterModel, setFilterModel] = useState("all");
  const [page, setPage] = useState(1);

  function buildQuery() {
    const params = new URLSearchParams();
    params.set("range", range);
    params.set("page", String(page));
    params.set("page_size", "50");
    if (filterToken !== "all") params.set("token", filterToken);
    if (filterAccount !== "all") params.set("account", filterAccount);
    if (filterModel !== "all") params.set("model", filterModel);
    return params.toString();
  }

  function loadStats() {
    api
      .get<UsageStats>(`/usage?${buildQuery()}`)
      .then(setStats)
      .catch(() => toast("Failed to load usage data", "error"))
      .finally(() => setLoading(false));
  }

  useEffect(() => {
    Promise.all([
      api.get<Account[]>("/accounts").catch(() => []),
      api.get<AccessToken[]>("/tokens").catch(() => []),
    ]).then(([a, t]) => {
      setAccounts(a ?? []);
      setTokensList(t ?? []);
    });
  }, []);

  useEffect(() => {
    setLoading(true);
    loadStats();
  }, [range, filterToken, filterAccount, filterModel, page]);

  const models = Array.from(
    new Set(stats?.logs?.items?.map((l) => l.model_alias).filter(Boolean) ?? []),
  );

  const totalPages = stats?.logs
    ? Math.ceil(stats.logs.total / stats.logs.page_size)
    : 1;

  const byAccountCols: Column<UsageStats["by_account"][0]>[] = [
    {
      key: "account",
      header: "Account",
      render: (r) => (
        <span className="font-medium text-moon-800">{r.account_label}</span>
      ),
    },
    {
      key: "requests",
      header: "Requests",
      render: (r) => compact(r.requests),
    },
    {
      key: "input",
      header: "Input",
      render: (r) => compact(r.input_tokens),
    },
    {
      key: "output",
      header: "Output",
      render: (r) => compact(r.output_tokens),
    },
    {
      key: "total",
      header: "Total",
      render: (r) => compact(r.input_tokens + r.output_tokens),
    },
  ];

  const byTokenCols: Column<UsageStats["by_token"][0]>[] = [
    {
      key: "token",
      header: "Token",
      render: (r) => (
        <span className="font-medium text-moon-800">{r.token_name}</span>
      ),
    },
    {
      key: "requests",
      header: "Requests",
      render: (r) => compact(r.requests),
    },
    {
      key: "input",
      header: "Input",
      render: (r) => compact(r.input_tokens),
    },
    {
      key: "output",
      header: "Output",
      render: (r) => compact(r.output_tokens),
    },
    {
      key: "total",
      header: "Total",
      render: (r) => compact(r.input_tokens + r.output_tokens),
    },
  ];

  return (
    <div className="space-y-8">
      <h2 className="text-xl font-semibold">Usage</h2>

      {/* Filters */}
      <div className="flex flex-wrap items-center gap-3">
        <Select value={range} onValueChange={(v) => { if (v) { setRange(v); setPage(1); } }}>
          <SelectTrigger className="w-36">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {TIME_RANGES.map((t) => (
              <SelectItem key={t.value} value={t.value}>
                {t.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select value={filterToken} onValueChange={(v) => { if (v) { setFilterToken(v); setPage(1); } }}>
          <SelectTrigger className="w-40">
            <SelectValue placeholder="Token" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All Tokens</SelectItem>
            {tokensList.map((t) => (
              <SelectItem key={t.id} value={t.name}>
                {t.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select value={filterAccount} onValueChange={(v) => { if (v) { setFilterAccount(v); setPage(1); } }}>
          <SelectTrigger className="w-40">
            <SelectValue placeholder="Account" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All Accounts</SelectItem>
            {accounts.map((a) => (
              <SelectItem key={a.id} value={String(a.id)}>
                {a.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select value={filterModel} onValueChange={(v) => { if (v) { setFilterModel(v); setPage(1); } }}>
          <SelectTrigger className="w-40">
            <SelectValue placeholder="Model" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All Models</SelectItem>
            {models.map((m) => (
              <SelectItem key={m} value={m}>
                {m}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {loading && !stats ? (
        <div className="space-y-4">
          <div className="grid grid-cols-3 gap-4">
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} className="h-28" />
            ))}
          </div>
          <Skeleton className="h-48" />
        </div>
      ) : (
        <>
          {/* Summary Cards */}
          <div className="grid grid-cols-3 gap-4">
            <StatCard
              label="Requests"
              value={compact(stats?.total_requests ?? 0)}
              icon={Activity}
            />
            <StatCard
              label="Input Tokens"
              value={compact(stats?.total_input_tokens ?? 0)}
              icon={ArrowDownToLine}
            />
            <StatCard
              label="Output Tokens"
              value={compact(stats?.total_output_tokens ?? 0)}
              icon={ArrowUpFromLine}
            />
          </div>

          {/* By Account */}
          <section>
            <h3 className="mb-4 text-sm font-medium uppercase tracking-wider text-moon-400">
              Usage by Account
            </h3>
            <Card className="ring-1 ring-moon-200/60">
              <CardContent className="p-1">
                <DataTable
                  columns={byAccountCols}
                  rows={stats?.by_account ?? []}
                  rowKey={(r) => r.account_id}
                  empty="No data"
                />
              </CardContent>
            </Card>
          </section>

          {/* By Token */}
          <section>
            <h3 className="mb-4 text-sm font-medium uppercase tracking-wider text-moon-400">
              Usage by Token
            </h3>
            <Card className="ring-1 ring-moon-200/60">
              <CardContent className="p-1">
                <DataTable
                  columns={byTokenCols}
                  rows={stats?.by_token ?? []}
                  rowKey={(r) => r.token_name}
                  empty="No data"
                />
              </CardContent>
            </Card>
          </section>

          {/* Request Log */}
          <section>
            <h3 className="mb-4 text-sm font-medium uppercase tracking-wider text-moon-400">
              Request Log
            </h3>
            <Card className="ring-1 ring-moon-200/60">
              <CardContent className="p-1">
                <DataTable
                  columns={logColumns}
                  rows={stats?.logs?.items ?? []}
                  rowKey={(r) => r.id}
                  empty="No requests logged"
                />
              </CardContent>
            </Card>

            {totalPages > 1 && (
              <div className="mt-4 flex items-center justify-center gap-4">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={page <= 1}
                  onClick={() => setPage((p) => p - 1)}
                >
                  Prev
                </Button>
                <span className="text-sm text-moon-500">
                  Page {page} of {totalPages}
                </span>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={page >= totalPages}
                  onClick={() => setPage((p) => p + 1)}
                >
                  Next
                </Button>
              </div>
            )}
          </section>
        </>
      )}
    </div>
  );
}
