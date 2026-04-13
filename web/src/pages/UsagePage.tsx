import { useEffect, useState } from "react";
import StatCard from "@/components/StatCard";
import DataTable, { type Column } from "@/components/DataTable";
import StatusBadge from "@/components/StatusBadge";
import PageHeader from "@/components/PageHeader";
import SectionHeading from "@/components/SectionHeading";
import { api } from "@/lib/api";
import { toast } from "@/components/Feedback";
import { compact, latency, shortDate } from "@/lib/fmt";
import type { UsageStats, RequestLog, Account, AccessToken, LatencyBucket } from "@/lib/types";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Activity, ArrowDownToLine, ArrowUpFromLine, DollarSign } from "lucide-react";
import { estimateCost, formatCost } from "@/lib/pricing";
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer } from "recharts";

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
    align: "right",
    tone: "numeric",
  },
  {
    key: "latency",
    header: "Latency",
    render: (r) => (
      <span className="text-moon-500">{latency(r.latency_ms)}</span>
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

export default function UsagePage() {
  const [stats, setStats] = useState<UsageStats | null>(null);
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [tokensList, setTokensList] = useState<AccessToken[]>([]);
  const [loading, setLoading] = useState(true);

  const [range, setRange] = useState("24h");
  const [filterToken, setFilterToken] = useState("all");
  const [filterAccount, setFilterAccount] = useState("all");
  const [filterModel, setFilterModel] = useState("all");
  const [filterSource, setFilterSource] = useState("all");
  const [page, setPage] = useState(1);
  const [viewTab, setViewTab] = useState<"requests" | "latency">("requests");
  const [latencyData, setLatencyData] = useState<LatencyBucket[]>([]);
  const [latencyBucket, setLatencyBucket] = useState("1h");
  const [latencyLoading, setLatencyLoading] = useState(false);

  function buildQuery() {
    const params = new URLSearchParams();
    params.set("range", range);
    params.set("page", String(page));
    params.set("page_size", "50");
    if (filterToken !== "all") params.set("token", filterToken);
    if (filterAccount !== "all") params.set("account", filterAccount);
    if (filterModel !== "all") params.set("model", filterModel);
    if (filterSource !== "all") params.set("source_kind", filterSource);
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
  }, [range, filterToken, filterAccount, filterModel, filterSource, page]);

  useEffect(() => {
    if (viewTab !== "latency") return;
    setLatencyLoading(true);
    const params = new URLSearchParams({ period: range, bucket: latencyBucket });
    if (filterModel !== "all") params.set("model", filterModel);
    api
      .get<LatencyBucket[]>(`/usage/latency?${params}`)
      .then((d) => setLatencyData(d ?? []))
      .catch(() => toast("Failed to load latency data", "error"))
      .finally(() => setLatencyLoading(false));
  }, [viewTab, range, filterModel, latencyBucket]);

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
      align: "right",
      tone: "numeric",
    },
    {
      key: "input",
      header: "Input",
      render: (r) => compact(r.input_tokens),
      align: "right",
      tone: "numeric",
    },
    {
      key: "output",
      header: "Output",
      render: (r) => compact(r.output_tokens),
      align: "right",
      tone: "numeric",
    },
    {
      key: "total",
      header: "Total",
      render: (r) => compact(r.input_tokens + r.output_tokens),
      align: "right",
      tone: "numeric",
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
      align: "right",
      tone: "numeric",
    },
    {
      key: "input",
      header: "Input",
      render: (r) => compact(r.input_tokens),
      align: "right",
      tone: "numeric",
    },
    {
      key: "output",
      header: "Output",
      render: (r) => compact(r.output_tokens),
      align: "right",
      tone: "numeric",
    },
    {
      key: "total",
      header: "Total",
      render: (r) => compact(r.input_tokens + r.output_tokens),
      align: "right",
      tone: "numeric",
    },
  ];

  const totalCost = (stats?.logs?.items ?? []).reduce((sum, r) => {
    if (r.input_tokens == null) return sum;
    const c = estimateCost(r.target_model || r.model_alias, r.input_tokens, r.output_tokens ?? 0);
    return sum + (c ?? 0);
  }, 0);
  const activeFilters = [
    range !== "24h",
    filterToken !== "all",
    filterAccount !== "all",
    filterModel !== "all",
    filterSource !== "all",
  ].filter(Boolean).length;

  return (
    <div className="space-y-8">
      <PageHeader
        eyebrow="Moonlight Console"
        title="Usage"
        description="Analyze request volume, token spend, and routing behavior over time without leaving the admin workspace."
        meta={
          <span>
            {stats?.logs?.total ?? 0} log entries in the selected range
            {activeFilters > 0 ? ` • ${activeFilters} active filter${activeFilters > 1 ? "s" : ""}` : ""}
          </span>
        }
      />

      <section className="rounded-[1.6rem] border border-moon-200/70 bg-[linear-gradient(180deg,rgba(255,255,255,0.94),rgba(240,242,248,0.92))] p-4 sm:p-5">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div className="space-y-1">
            <p className="text-[11px] font-semibold uppercase tracking-[0.22em] text-lunar-600">
              Control Surface
            </p>
            <h2 className="text-lg font-semibold text-moon-800">
              Slice usage by time range, token, account, or model alias
            </h2>
            <p className="text-sm text-moon-500">
              Filters update the summary, ranking tables, and request log together.
            </p>
          </div>
          <div className="text-sm text-moon-500">
            Current range:{" "}
            <span className="font-medium text-moon-700">
              {TIME_RANGES.find((item) => item.value === range)?.label ?? range}
            </span>
          </div>
        </div>

        <div className="mt-5 grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
          <Select value={range} onValueChange={(v) => { if (v) { setRange(v); setPage(1); } }}>
            <SelectTrigger className="h-11 w-full rounded-xl border-moon-200 bg-white/90">
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
            <SelectTrigger className="h-11 w-full rounded-xl border-moon-200 bg-white/90">
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
            <SelectTrigger className="h-11 w-full rounded-xl border-moon-200 bg-white/90">
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
            <SelectTrigger className="h-11 w-full rounded-xl border-moon-200 bg-white/90">
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

          <Select value={filterSource} onValueChange={(v) => { if (v) { setFilterSource(v); setPage(1); } }}>
            <SelectTrigger className="h-11 w-full rounded-xl border-moon-200 bg-white/90">
              <SelectValue placeholder="Source" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All Sources</SelectItem>
              <SelectItem value="openai_compat">OpenAI Compatible</SelectItem>
              <SelectItem value="cpa">CPA</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </section>

      {loading && !stats ? (
        <div className="space-y-4">
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} className="h-36 rounded-[1.5rem]" />
            ))}
          </div>
          <div className="grid gap-6 xl:grid-cols-2">
            <Skeleton className="h-72 rounded-[1.5rem]" />
            <Skeleton className="h-72 rounded-[1.5rem]" />
          </div>
          <Skeleton className="h-96 rounded-[1.5rem]" />
        </div>
      ) : (
        <>
          <section className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
            <StatCard
              label="Requests"
              value={compact(stats?.total_requests ?? 0)}
              sub="Matching requests in the active range."
              icon={Activity}
              variant="hero"
            />
            <StatCard
              label="Input Tokens"
              value={compact(stats?.total_input_tokens ?? 0)}
              sub="Inbound tokens before routing."
              icon={ArrowDownToLine}
            />
            <StatCard
              label="Output Tokens"
              value={compact(stats?.total_output_tokens ?? 0)}
              sub="Generated tokens returned downstream."
              icon={ArrowUpFromLine}
            />
            <StatCard
              label="Est. Cost"
              value={totalCost > 0 ? formatCost(totalCost) : "-"}
              sub="Estimated cost for this page."
              icon={DollarSign}
            />
          </section>

          <section className="grid gap-6 xl:grid-cols-2">
            <div className="space-y-4">
              <SectionHeading
                title="Usage by Account"
                description="Which upstream accounts are carrying the current workload."
              />
              <div className="overflow-hidden rounded-[1.6rem] border border-moon-200/70 bg-white/85">
                <DataTable
                  columns={byAccountCols}
                  rows={stats?.by_account ?? []}
                  rowKey={(r) => r.account_id}
                  empty="No data"
                />
              </div>
            </div>

            <div className="space-y-4">
              <SectionHeading
                title="Usage by Token"
                description="Client-side demand distribution across issued access tokens."
              />
              <div className="overflow-hidden rounded-[1.6rem] border border-moon-200/70 bg-white/85">
                <DataTable
                  columns={byTokenCols}
                  rows={stats?.by_token ?? []}
                  rowKey={(r) => r.token_name}
                  empty="No data"
                />
              </div>
            </div>
          </section>

          {/* Tab switcher */}
          <div className="flex gap-1">
            {(["requests", "latency"] as const).map((tab) => (
              <button
                key={tab}
                type="button"
                onClick={() => setViewTab(tab)}
                className={`rounded-lg px-4 py-2 text-sm font-medium transition ${
                  viewTab === tab
                    ? "bg-moon-800 text-moon-100"
                    : "bg-moon-100 text-moon-500 hover:bg-moon-200"
                }`}
              >
                {tab === "requests" ? "Requests" : "Latency"}
              </button>
            ))}
          </div>

          {viewTab === "requests" ? (
            <section className="space-y-4">
              <SectionHeading
                title="Request Log"
                description="Detailed request samples with latency, token flow, and upstream account outcomes."
                action={
                  totalPages > 1 ? (
                    <span className="text-sm text-moon-500">
                      Page {page} of {totalPages}
                    </span>
                  ) : null
                }
              />
              <div className="overflow-hidden rounded-[1.6rem] border border-moon-200/70 bg-white/85">
                  <DataTable
                    columns={logColumns}
                    rows={stats?.logs?.items ?? []}
                    rowKey={(r) => r.id}
                    empty="No requests logged"
                  />
              </div>

              {totalPages > 1 && (
                <div className="flex items-center justify-center gap-4">
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
          ) : (
            <section className="space-y-4">
              <div className="flex items-center justify-between">
                <SectionHeading
                  title="Latency Distribution"
                  description="p50 / p95 / p99 latency over time."
                />
                <Select value={latencyBucket} onValueChange={(v) => v && setLatencyBucket(v)}>
                  <SelectTrigger className="h-9 w-24 rounded-lg border-moon-200 bg-white/90 text-sm">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="5m">5m</SelectItem>
                    <SelectItem value="1h">1h</SelectItem>
                    <SelectItem value="1d">1d</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="overflow-hidden rounded-[1.6rem] border border-moon-200/70 bg-white/85 p-5">
                {latencyLoading ? (
                  <Skeleton className="h-[300px] rounded-xl" />
                ) : latencyData.length === 0 ? (
                  <div className="flex h-[300px] items-center justify-center text-sm text-moon-400">
                    No latency data for this range
                  </div>
                ) : (
                  <ResponsiveContainer width="100%" height={300}>
                    <LineChart data={latencyData}>
                      <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
                      <XAxis
                        dataKey="bucket"
                        tick={{ fontSize: 11, fill: "#9ca3af" }}
                        tickFormatter={(v: string) => {
                          if (v.length > 13) return v.slice(11, 16);
                          if (v.length > 10) return v.slice(11);
                          return v.slice(5);
                        }}
                      />
                      <YAxis
                        tick={{ fontSize: 11, fill: "#9ca3af" }}
                        label={{ value: "ms", angle: -90, position: "insideLeft", style: { fontSize: 11, fill: "#9ca3af" } }}
                      />
                      <Tooltip
                        contentStyle={{ borderRadius: "0.75rem", fontSize: "12px" }}
                        formatter={(value) => [`${value}ms`]}
                      />
                      <Legend wrapperStyle={{ fontSize: "12px" }} />
                      <Line type="monotone" dataKey="p50" stroke="#7c86b8" strokeWidth={2} dot={false} name="p50" />
                      <Line type="monotone" dataKey="p95" stroke="#e0a030" strokeWidth={2} dot={false} name="p95" />
                      <Line type="monotone" dataKey="p99" stroke="#e05050" strokeWidth={2} dot={false} name="p99" />
                    </LineChart>
                  </ResponsiveContainer>
                )}
              </div>
            </section>
          )}
        </>
      )}
    </div>
  );
}
