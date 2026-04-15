import { useEffect, useState } from "react";
import DataTable, { type Column } from "@/components/DataTable";
import PageHeader from "@/components/PageHeader";
import SectionHeading from "@/components/SectionHeading";
import StatusBadge from "@/components/StatusBadge";
import { api } from "@/lib/api";
import { toast } from "@/components/Feedback";
import { compact, latency, shortDate } from "@/lib/fmt";
import { estimateCost, formatCost } from "@/lib/pricing";
import type {
  Account,
  AccessToken,
  LatencyBucket,
  RequestLog,
  UsageLogPage,
  UsageStats,
} from "@/lib/types";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Activity,
  ArrowDownToLine,
  ArrowUpFromLine,
  BarChart3,
  CheckCircle2,
  DollarSign,
  Filter,
  Waves,
} from "lucide-react";
import {
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";

/** Backend returns UsageStats fields at top level + logs as a nested field. */
type UsageResponse = UsageStats & { logs: UsageLogPage };

const TIME_RANGES = [
  { value: "1h", label: "最近 1 小时" },
  { value: "24h", label: "最近 24 小时" },
  { value: "7d", label: "最近 7 天" },
  { value: "30d", label: "最近 30 天" },
  { value: "all", label: "全部时间" },
];

const logColumns: Column<RequestLog>[] = [
  {
    key: "time",
    header: "时间",
    render: (r) => <span className="text-moon-500">{shortDate(r.created_at)}</span>,
    tone: "secondary",
  },
  {
    key: "model",
    header: "模型",
    render: (r) => <span className="font-medium">{r.model_requested}</span>,
    tone: "primary",
  },
  {
    key: "token",
    header: "令牌",
    render: (r) => <span className="text-moon-500">{r.access_token_name}</span>,
    tone: "secondary",
  },
  {
    key: "account",
    header: "账号",
    render: (r) => <span className="text-moon-500">{r.account_label}</span>,
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
    key: "io",
    header: "输入 / 输出",
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
    header: "延迟",
    render: (r) => <span className="text-moon-500">{latency(r.latency_ms)}</span>,
    align: "right",
    tone: "numeric",
  },
  {
    key: "cost",
    header: "预估成本",
    render: (r) => {
      const cost = r.input_tokens != null
        ? estimateCost(r.model_actual || r.model_requested, r.input_tokens, r.output_tokens ?? 0)
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
  const [resp, setResp] = useState<UsageResponse | null>(null);
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
    let cancelled = false;
    api
      .get<UsageResponse>(`/usage?${buildQuery()}`)
      .then((d) => {
        if (!cancelled) setResp(d);
      })
      .catch(() => {
        if (!cancelled) toast("加载用量数据失败", "error");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }

  useEffect(() => {
    let cancelled = false;
    Promise.all([
      api.get<Account[]>("/accounts").catch(() => []),
      api.get<AccessToken[]>("/tokens").catch(() => []),
    ]).then(([a, t]) => {
      if (cancelled) return;
      setAccounts(a ?? []);
      setTokensList(t ?? []);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    setLoading(true);
    const cancel = loadStats();
    return cancel;
  }, [range, filterToken, filterAccount, filterModel, filterSource, page]);

  useEffect(() => {
    if (viewTab !== "latency") return;
    let cancelled = false;
    setLatencyLoading(true);
    const params = new URLSearchParams({ period: range, bucket: latencyBucket });
    if (filterModel !== "all") params.set("model", filterModel);
    api
      .get<LatencyBucket[]>(`/usage/latency?${params}`)
      .then((d) => {
        if (!cancelled) setLatencyData(d ?? []);
      })
      .catch(() => {
        if (!cancelled) toast("加载延迟数据失败", "error");
      })
      .finally(() => {
        if (!cancelled) setLatencyLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [viewTab, range, filterModel, latencyBucket]);

  const logs = resp?.logs;
  const models = Array.from(
    new Set(logs?.items?.map((l) => l.model_requested).filter(Boolean) ?? []),
  );
  const totalPages = logs ? Math.ceil(logs.total / logs.page_size) : 1;
  const totalCost = (logs?.items ?? []).reduce((sum, r) => {
    if (r.input_tokens == null) return sum;
    const cost = estimateCost(r.model_actual || r.model_requested, r.input_tokens, r.output_tokens ?? 0);
    return sum + (cost ?? 0);
  }, 0);
  const activeFilters = [
    range !== "24h",
    filterToken !== "all",
    filterAccount !== "all",
    filterModel !== "all",
    filterSource !== "all",
  ].filter(Boolean).length;

  const byAccountCols: Column<UsageStats["by_account"][0]>[] = [
    {
      key: "account",
      header: "账号",
      render: (r) => <span className="font-medium text-moon-800">{r.account_label}</span>,
    },
    {
      key: "requests",
      header: "请求数",
      render: (r) => compact(r.requests),
      align: "right",
      tone: "numeric",
    },
    {
      key: "input",
      header: "输入",
      render: (r) => compact(r.input_tokens),
      align: "right",
      tone: "numeric",
    },
    {
      key: "output",
      header: "输出",
      render: (r) => compact(r.output_tokens),
      align: "right",
      tone: "numeric",
    },
    {
      key: "total",
      header: "合计",
      render: (r) => compact(r.input_tokens + r.output_tokens),
      align: "right",
      tone: "numeric",
    },
  ];

  const byTokenCols: Column<UsageStats["by_token"][0]>[] = [
    {
      key: "token",
      header: "令牌",
      render: (r) => <span className="font-medium text-moon-800">{r.token_name}</span>,
    },
    {
      key: "requests",
      header: "请求数",
      render: (r) => compact(r.requests),
      align: "right",
      tone: "numeric",
    },
    {
      key: "input",
      header: "输入",
      render: (r) => compact(r.input_tokens),
      align: "right",
      tone: "numeric",
    },
    {
      key: "output",
      header: "输出",
      render: (r) => compact(r.output_tokens),
      align: "right",
      tone: "numeric",
    },
    {
      key: "total",
      header: "合计",
      render: (r) => compact(r.input_tokens + r.output_tokens),
      align: "right",
      tone: "numeric",
    },
  ];

  const successRatePercent = resp?.success_rate != null
    ? `${(resp.success_rate * 100).toFixed(1)}%`
    : "-";

  return (
    <div className="space-y-10">
      <PageHeader
        eyebrow="Usage / Analytics"
        title="用量"
        description="查看请求量、Token 与成本走势。"
        meta={
          <>
            <span>日志 {logs?.total ?? 0}</span>
            <span>筛选 {activeFilters}</span>
            <span>{TIME_RANGES.find((item) => item.value === range)?.label ?? range}</span>
          </>
        }
      />

      <section className="surface-section px-5 py-5 sm:px-6 sm:py-6">
        <div className="flex flex-col gap-5 xl:flex-row xl:items-end xl:justify-between">
          <div className="space-y-2">
            <p className="eyebrow-label">分析工具条</p>
            <h2 className="text-[1.1rem] font-semibold tracking-[-0.03em] text-moon-800">
              按范围筛选，再看请求、延迟与成本分布
            </h2>
            <p className="text-sm text-moon-500">
              这里不做欢迎语，只保留分析所需的上下文。
            </p>
          </div>

          <div className="inline-flex items-center gap-2 rounded-full border border-white/75 bg-white/70 px-3 py-2 text-sm text-moon-500">
            <Filter className="size-4 text-moon-400" />
            当前已启用 {activeFilters} 个筛选条件
          </div>
        </div>

        <div className="mt-5 grid gap-3 md:grid-cols-2 xl:grid-cols-5">
          <Select value={range} onValueChange={(v) => v && (setRange(v), setPage(1))}>
            <SelectTrigger className="h-11 rounded-xl border-white/75 bg-white/82">
              <SelectValue placeholder="时间范围" />
            </SelectTrigger>
            <SelectContent>
              {TIME_RANGES.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <Select value={filterAccount} onValueChange={(v) => v && (setFilterAccount(v), setPage(1))}>
            <SelectTrigger className="h-11 rounded-xl border-white/75 bg-white/82">
              <SelectValue placeholder="账号" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部账号</SelectItem>
              {accounts.map((a) => (
                <SelectItem key={a.id} value={String(a.id)}>
                  {a.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <Select value={filterToken} onValueChange={(v) => v && (setFilterToken(v), setPage(1))}>
            <SelectTrigger className="h-11 rounded-xl border-white/75 bg-white/82">
              <SelectValue placeholder="令牌" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部令牌</SelectItem>
              {tokensList.map((t) => (
                <SelectItem key={t.id} value={t.name}>
                  {t.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <Select value={filterModel} onValueChange={(v) => v && (setFilterModel(v), setPage(1))}>
            <SelectTrigger className="h-11 rounded-xl border-white/75 bg-white/82">
              <SelectValue placeholder="模型" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部模型</SelectItem>
              {models.map((m) => (
                <SelectItem key={m} value={m}>
                  {m}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <Select value={filterSource} onValueChange={(v) => v && (setFilterSource(v), setPage(1))}>
            <SelectTrigger className="h-11 rounded-xl border-white/75 bg-white/82">
              <SelectValue placeholder="来源" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部来源</SelectItem>
              <SelectItem value="openai_compat">OpenAI 兼容</SelectItem>
              <SelectItem value="cpa">CPA</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </section>

      {loading && !resp ? (
        <div className="space-y-6">
          <Skeleton className="h-[22rem] rounded-[2rem]" />
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-5">
            {Array.from({ length: 5 }).map((_, index) => (
              <Skeleton key={index} className="h-32 rounded-[1.4rem]" />
            ))}
          </div>
          <div className="grid gap-6 xl:grid-cols-2">
            <Skeleton className="h-80 rounded-[1.6rem]" />
            <Skeleton className="h-80 rounded-[1.6rem]" />
          </div>
        </div>
      ) : (
        <>
          <section className="grid gap-6 xl:grid-cols-[minmax(0,1.2fr)_minmax(320px,0.8fr)]">
            <div className="surface-section overflow-hidden px-5 py-5 sm:px-6 sm:py-6">
              <div className="flex flex-wrap items-end justify-between gap-4 border-b border-moon-200/60 pb-4">
                <div>
                  <p className="eyebrow-label">主图表</p>
                  <h2 className="mt-1 text-[1.1rem] font-semibold tracking-[-0.03em] text-moon-800">
                    延迟趋势
                  </h2>
                </div>
                <Select value={latencyBucket} onValueChange={(v) => v && setLatencyBucket(v)}>
                  <SelectTrigger className="h-9 w-24 rounded-lg border-white/75 bg-white/84 text-sm">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="5m">5m</SelectItem>
                    <SelectItem value="1h">1h</SelectItem>
                    <SelectItem value="1d">1d</SelectItem>
                  </SelectContent>
                </Select>
              </div>

              <div className="mt-5 h-[320px]">
                {latencyLoading ? (
                  <Skeleton className="h-full rounded-[1.2rem]" />
                ) : latencyData.length === 0 ? (
                  <div className="panel-muted flex h-full flex-col items-center justify-center rounded-[1.4rem] border border-dashed border-moon-200/80 text-center">
                    <Waves className="size-6 text-moon-300" />
                    <p className="mt-4 text-sm font-medium text-moon-600">当前范围内没有延迟样本</p>
                    <p className="mt-1 text-sm text-moon-400">调整时间范围或筛选条件后再查看。</p>
                  </div>
                ) : (
                  <ResponsiveContainer width="100%" height="100%">
                    <LineChart data={latencyData}>
                      <CartesianGrid strokeDasharray="3 3" stroke="rgba(197,201,216,0.55)" />
                      <XAxis
                        dataKey="bucket"
                        tick={{ fontSize: 11, fill: "#98a0b7" }}
                        tickFormatter={(v: string) => {
                          if (v.length > 13) return v.slice(11, 16);
                          if (v.length > 10) return v.slice(11);
                          return v.slice(5);
                        }}
                      />
                      <YAxis
                        tick={{ fontSize: 11, fill: "#98a0b7" }}
                        label={{
                          value: "ms",
                          angle: -90,
                          position: "insideLeft",
                          style: { fontSize: 11, fill: "#98a0b7" },
                        }}
                      />
                      <Tooltip
                        contentStyle={{
                          borderRadius: "0.95rem",
                          border: "1px solid rgba(197,201,216,0.6)",
                          background: "rgba(255,255,255,0.94)",
                          fontSize: "12px",
                        }}
                        formatter={(value) => [`${value}ms`]}
                      />
                      <Legend wrapperStyle={{ fontSize: "12px" }} />
                      <Line type="monotone" dataKey="p50" stroke="#867dc1" strokeWidth={2.3} dot={false} name="p50" />
                      <Line type="monotone" dataKey="p95" stroke="#c09a55" strokeWidth={2.1} dot={false} name="p95" />
                      <Line type="monotone" dataKey="p99" stroke="#be7476" strokeWidth={2.1} dot={false} name="p99" />
                    </LineChart>
                  </ResponsiveContainer>
                )}
              </div>
            </div>

            <aside className="grid gap-4 md:grid-cols-2 xl:grid-cols-1">
              {[
                {
                  label: "请求",
                  value: compact(resp?.total_requests ?? 0),
                  sub: "当前筛选范围内的请求数",
                  icon: Activity,
                },
                {
                  label: "成功率",
                  value: successRatePercent,
                  sub: "成功请求占比",
                  icon: CheckCircle2,
                },
                {
                  label: "输入 Token",
                  value: compact(resp?.total_input_tokens ?? 0),
                  sub: "请求侧输入量",
                  icon: ArrowDownToLine,
                },
                {
                  label: "输出 Token",
                  value: compact(resp?.total_output_tokens ?? 0),
                  sub: "模型返回量",
                  icon: ArrowUpFromLine,
                },
                {
                  label: "预估成本",
                  value: totalCost > 0 ? formatCost(totalCost) : "-",
                  sub: "按当前页日志估算",
                  icon: DollarSign,
                },
              ].map((item) => (
                <div key={item.label} className="surface-card px-5 py-5">
                  <div className="flex items-center justify-between gap-3">
                    <p className="text-xs tracking-[0.16em] text-moon-400">{item.label}</p>
                    <item.icon className="size-4 text-moon-400" />
                  </div>
                  <p className="mt-3 text-[1.7rem] font-semibold tracking-[-0.05em] text-moon-800">
                    {item.value}
                  </p>
                  <p className="mt-2 text-sm text-moon-500">{item.sub}</p>
                </div>
              ))}
            </aside>
          </section>

          <section className="grid gap-6 xl:grid-cols-2">
            <div className="space-y-4">
              <SectionHeading
                title="按账号分布"
                description="查看当前负载主要由哪些上游账号承接。"
              />
              <div className="surface-card overflow-hidden">
                <DataTable
                  columns={byAccountCols}
                  rows={resp?.by_account ?? []}
                  rowKey={(r) => r.account_id}
                  empty="当前没有账号维度数据"
                />
              </div>
            </div>

            <div className="space-y-4">
              <SectionHeading
                title="按令牌分布"
                description="查看客户端请求在各访问令牌上的分布。"
              />
              <div className="surface-card overflow-hidden">
                <DataTable
                  columns={byTokenCols}
                  rows={resp?.by_token ?? []}
                  rowKey={(r) => r.token_name}
                  empty="当前没有令牌维度数据"
                />
              </div>
            </div>
          </section>

          <section className="space-y-4">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <SectionHeading
                title="日志与延迟明细"
                description="切换查看请求样本或延迟曲线。"
              />
              <div className="flex gap-1 rounded-full border border-white/75 bg-white/70 p-1">
                {(["requests", "latency"] as const).map((tab) => (
                  <button
                    key={tab}
                    type="button"
                    onClick={() => setViewTab(tab)}
                    className={`rounded-full px-4 py-2 text-sm transition ${
                      viewTab === tab
                        ? "bg-moon-800 text-moon-50"
                        : "text-moon-500 hover:bg-moon-100/80"
                    }`}
                  >
                    {tab === "requests" ? "请求日志" : "延迟曲线"}
                  </button>
                ))}
              </div>
            </div>

            {viewTab === "requests" ? (
              <div className="surface-card overflow-hidden">
                <div className="flex flex-wrap items-end justify-between gap-3 border-b border-moon-200/60 px-4 py-3">
                  <div>
                    <p className="eyebrow-label">Request Log</p>
                    <p className="mt-1 text-sm text-moon-500">
                      最新样本按时间倒序排列，便于追踪路由与错误。
                    </p>
                  </div>
                  <span className="text-sm text-moon-500">
                    第 {page} / {totalPages} 页
                  </span>
                </div>
                <DataTable
                  columns={logColumns}
                  rows={logs?.items ?? []}
                  rowKey={(r) => r.id}
                  empty="当前没有请求日志"
                />
                {totalPages > 1 && (
                  <div className="flex items-center justify-center gap-4 border-t border-moon-200/60 px-4 py-4">
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={page <= 1}
                      onClick={() => setPage((p) => p - 1)}
                    >
                      上一页
                    </Button>
                    <span className="text-sm text-moon-500">
                      第 {page} / {totalPages} 页
                    </span>
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={page >= totalPages}
                      onClick={() => setPage((p) => p + 1)}
                    >
                      下一页
                    </Button>
                  </div>
                )}
              </div>
            ) : (
              <div className="surface-card px-5 py-5 sm:px-6 sm:py-6">
                <div className="flex items-center gap-3">
                  <BarChart3 className="size-4 text-moon-400" />
                  <p className="text-sm text-moon-500">
                    上方主图已作为本页核心分析区，这里保留为辅助查看入口。
                  </p>
                </div>
                <div className="mt-4 h-[320px]">
                  {latencyLoading ? (
                    <Skeleton className="h-full rounded-[1.2rem]" />
                  ) : latencyData.length === 0 ? (
                    <div className="panel-muted flex h-full items-center justify-center rounded-[1.4rem] border border-dashed border-moon-200/80 text-sm text-moon-400">
                      当前范围内暂无延迟数据
                    </div>
                  ) : (
                    <ResponsiveContainer width="100%" height="100%">
                      <LineChart data={latencyData}>
                        <CartesianGrid strokeDasharray="3 3" stroke="rgba(197,201,216,0.55)" />
                        <XAxis dataKey="bucket" tick={{ fontSize: 11, fill: "#98a0b7" }} />
                        <YAxis tick={{ fontSize: 11, fill: "#98a0b7" }} />
                        <Tooltip />
                        <Line type="monotone" dataKey="p50" stroke="#867dc1" strokeWidth={2.2} dot={false} />
                        <Line type="monotone" dataKey="p95" stroke="#c09a55" strokeWidth={2} dot={false} />
                        <Line type="monotone" dataKey="p99" stroke="#be7476" strokeWidth={2} dot={false} />
                      </LineChart>
                    </ResponsiveContainer>
                  )}
                </div>
              </div>
            )}
          </section>
        </>
      )}
    </div>
  );
}
