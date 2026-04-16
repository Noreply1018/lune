import { Fragment, useEffect, useMemo, useRef, useState } from "react";
import { ChevronDown, RefreshCw } from "lucide-react";
import ErrorState from "@/components/ErrorState";
import PageHeader from "@/components/PageHeader";
import SectionHeading from "@/components/SectionHeading";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { api } from "@/lib/api";
import { compact, latency, pct, relativeTime, shortDate, tokenCount } from "@/lib/fmt";
import type {
  NotificationChannel,
  NotificationDelivery,
  Overview,
  Pool,
  RequestLog,
  UsageLogPage,
  UsageStats,
} from "@/lib/types";
import { cn } from "@/lib/utils";

type TimeRange = "24h" | "7d" | "30d";

type UsageResponse = UsageStats & {
  logs: UsageLogPage;
};

type UsageBundle = {
  summary: UsageStats;
  logs: RequestLog[];
  total: number;
  truncated: boolean;
};

type NotificationStatusFilter = "all" | "success" | "failed" | "dropped" | "test";

type TrendBucket = {
  key: string;
  label: string;
  count: number;
  successRate: number;
};

const RANGE_OPTIONS: Array<{ value: TimeRange; label: string }> = [
  { value: "24h", label: "24h" },
  { value: "7d", label: "7d" },
  { value: "30d", label: "30d" },
];

const PAGE_SIZE = 250;
const MAX_PAGES = 12;

function parseAdminDate(iso: string) {
  if (!iso) return new Date("");
  const safeIso =
    iso.includes("T") || iso.includes("Z") || iso.includes("+")
      ? iso
      : iso.replace(" ", "T") + "Z";
  return new Date(safeIso);
}

function getRequestTokens(log: RequestLog) {
  return (log.input_tokens ?? 0) + (log.output_tokens ?? 0);
}

function getRequestSummary(log: RequestLog) {
  return `chat/completions · ${log.stream ? "stream" : "non-stream"}`;
}

function getRangeStart(range: TimeRange) {
  const now = new Date();
  if (range === "24h") {
    const start = new Date(now);
    start.setMinutes(0, 0, 0);
    start.setHours(start.getHours() - 23);
    return start;
  }
  const start = new Date(now);
  start.setHours(0, 0, 0, 0);
  start.setDate(start.getDate() - (range === "7d" ? 6 : 29));
  return start;
}

function buildTrendBuckets(logs: RequestLog[], range: TimeRange): TrendBucket[] {
  const start = getRangeStart(range);
  const now = new Date();
  const bucketCount = range === "24h" ? 24 : range === "7d" ? 7 : 30;
  const buckets = Array.from({ length: bucketCount }, (_, index) => {
    const date = new Date(start);
    if (range === "24h") {
      date.setHours(start.getHours() + index, 0, 0, 0);
    } else {
      date.setDate(start.getDate() + index);
      date.setHours(0, 0, 0, 0);
    }
    const key = range === "24h"
      ? `${date.getFullYear()}-${date.getMonth()}-${date.getDate()}-${date.getHours()}`
      : `${date.getFullYear()}-${date.getMonth()}-${date.getDate()}`;
    const label = range === "24h"
      ? `${String(date.getHours()).padStart(2, "0")}:00`
      : `${date.getMonth() + 1}/${date.getDate()}`;
    return {
      key,
      label,
      count: 0,
      successRate: 0,
      successCount: 0,
      total: 0,
    };
  });

  const indexByKey = new Map(buckets.map((bucket, index) => [bucket.key, index]));
  logs.forEach((log) => {
    const date = parseAdminDate(log.created_at);
    if (Number.isNaN(date.getTime()) || date < start || date > now) {
      return;
    }
    const key = range === "24h"
      ? `${date.getFullYear()}-${date.getMonth()}-${date.getDate()}-${date.getHours()}`
      : `${date.getFullYear()}-${date.getMonth()}-${date.getDate()}`;
    const bucketIndex = indexByKey.get(key);
    if (bucketIndex == null) {
      return;
    }
    buckets[bucketIndex].count += 1;
    buckets[bucketIndex].total += 1;
    if (log.success) {
      buckets[bucketIndex].successCount += 1;
    }
  });

  return buckets.map((bucket) => ({
    key: bucket.key,
    label: bucket.label,
    count: bucket.count,
    successRate: bucket.total > 0 ? bucket.successCount / bucket.total : 0,
  }));
}

function buildDistribution<T extends string | number>(
  logs: RequestLog[],
  getKey: (log: RequestLog) => T,
  getLabel: (key: T) => string,
) {
  const counts = new Map<T, number>();
  logs.forEach((log) => {
    const key = getKey(log);
    counts.set(key, (counts.get(key) ?? 0) + 1);
  });
  return Array.from(counts.entries())
    .map(([key, requests]) => ({ key, label: getLabel(key), requests }))
    .sort((a, b) => b.requests - a.requests)
    .slice(0, 6);
}

function buildPolyline(values: number[], height: number, maxValue: number) {
  if (values.length === 0) return "";
  return values
    .map((value, index) => {
      const x = values.length === 1 ? 0 : (index / (values.length - 1)) * 100;
      const y = maxValue <= 0 ? height : height - (value / maxValue) * height;
      return `${x},${y.toFixed(2)}`;
    })
    .join(" ");
}

function SummaryMetric({
  label,
  value,
  hint,
}: {
  label: string;
  value: string;
  hint?: string;
}) {
  return (
    <div className="space-y-2 px-1 py-1">
      <p className="text-[11px] uppercase tracking-[0.18em] text-moon-400">{label}</p>
      <p className="text-[1.55rem] font-semibold tracking-[-0.04em] text-moon-800">{value}</p>
      {hint ? <p className="text-sm text-moon-500">{hint}</p> : null}
    </div>
  );
}

function TrendBars({
  title,
  description,
  buckets,
}: {
  title: string;
  description: string;
  buckets: TrendBucket[];
}) {
  const maxCount = Math.max(...buckets.map((bucket) => bucket.count), 1);
  const midIndex = Math.floor((buckets.length - 1) / 2);

  return (
    <section className="surface-section px-5 py-5">
      <SectionHeading title={title} description={description} />
      <div className="mt-5">
        <div className="flex h-44 items-end gap-1.5">
          {buckets.map((bucket) => (
            <div
              key={bucket.key}
              title={`${bucket.label} · ${bucket.count} 请求`}
              className="group flex min-w-0 flex-1 items-end"
            >
              <div
                className="w-full rounded-t-[0.7rem] bg-[linear-gradient(180deg,rgba(134,125,193,0.64),rgba(134,125,193,0.18))] transition-opacity duration-200 group-hover:opacity-90"
                style={{ height: `${Math.max((bucket.count / maxCount) * 100, bucket.count > 0 ? 6 : 2)}%` }}
              />
            </div>
          ))}
        </div>
        <div className="mt-3 flex items-center justify-between text-xs text-moon-400">
          <span>{buckets[0]?.label}</span>
          <span>{buckets[midIndex]?.label}</span>
          <span>{buckets[buckets.length - 1]?.label}</span>
        </div>
      </div>
    </section>
  );
}

function SuccessRateLine({
  buckets,
}: {
  buckets: TrendBucket[];
}) {
  const values = buckets.map((bucket) => Math.round(bucket.successRate * 100));
  const polyline = buildPolyline(values, 120, 100);
  const midIndex = Math.floor((buckets.length - 1) / 2);

  return (
    <section className="surface-section px-5 py-5">
      <SectionHeading title="成功率趋势" description="看最近一段时间是否在抖动。" />
      <div className="mt-5 space-y-3">
        <div className="relative overflow-hidden rounded-[1.4rem] border border-moon-200/55 bg-[linear-gradient(180deg,rgba(255,255,255,0.64),rgba(244,241,250,0.58))] px-3 py-3">
          <div className="absolute inset-0 grid grid-rows-3 opacity-50">
            {Array.from({ length: 3 }).map((_, index) => (
              <div key={index} className="border-b border-moon-200/35 last:border-b-0" />
            ))}
          </div>
          <svg viewBox="0 0 100 120" className="relative h-36 w-full overflow-visible">
            <polyline
              fill="none"
              stroke="rgba(134,125,193,0.88)"
              strokeWidth="2.4"
              strokeLinejoin="round"
              strokeLinecap="round"
              points={polyline}
            />
          </svg>
        </div>
        <div className="flex items-center justify-between text-xs text-moon-400">
          <span>{buckets[0]?.label}</span>
          <span>{buckets[midIndex]?.label}</span>
          <span>{buckets[buckets.length - 1]?.label}</span>
        </div>
      </div>
    </section>
  );
}

function DistributionPanel({
  title,
  description,
  rows,
}: {
  title: string;
  description: string;
  rows: Array<{ label: string; requests: number }>;
}) {
  const maxValue = Math.max(...rows.map((row) => row.requests), 1);

  return (
    <section className="surface-section px-5 py-5">
      <SectionHeading title={title} description={description} />
      <div className="mt-5 space-y-3">
        {rows.length ? rows.map((row) => (
          <div key={row.label} className="space-y-1.5">
            <div className="flex items-center justify-between gap-3 text-sm">
              <span className="truncate text-moon-700">{row.label}</span>
              <span className="shrink-0 text-moon-400">{compact(row.requests)}</span>
            </div>
            <div className="h-2 rounded-full bg-moon-100/85">
              <div
                className="h-full rounded-full bg-[linear-gradient(90deg,rgba(134,125,193,0.7),rgba(134,125,193,0.24))]"
                style={{ width: `${(row.requests / maxValue) * 100}%` }}
              />
            </div>
          </div>
        )) : (
          <p className="text-sm text-moon-400">当前筛选下暂无分布数据。</p>
        )}
      </div>
    </section>
  );
}

async function loadUsageBundle(range: TimeRange): Promise<UsageBundle> {
  const firstPage = await api.get<UsageResponse>(`/usage?range=${range}&page=1&page_size=${PAGE_SIZE}`);
  const firstLogs = firstPage.logs?.items ?? [];
  const total = firstPage.logs?.total ?? firstLogs.length;
  const totalPages = Math.ceil(total / PAGE_SIZE);
  const pagesToFetch = Math.min(totalPages, MAX_PAGES);

  if (pagesToFetch <= 1) {
    return {
      summary: firstPage,
      logs: firstLogs,
      total,
      truncated: totalPages > MAX_PAGES,
    };
  }

  const remainingPages = await Promise.all(
    Array.from({ length: pagesToFetch - 1 }, (_, index) =>
      api.get<UsageResponse>(`/usage?range=${range}&page=${index + 2}&page_size=${PAGE_SIZE}`),
    ),
  );

  return {
    summary: firstPage,
    logs: [
      ...firstLogs,
      ...remainingPages.flatMap((page) => page.logs?.items ?? []),
    ],
    total,
    truncated: totalPages > MAX_PAGES,
  };
}

export default function ActivityPage() {
  const [range, setRange] = useState<TimeRange>("24h");
  const [overview, setOverview] = useState<Overview | null>(null);
  const [pools, setPools] = useState<Pool[]>([]);
  const [usage, setUsage] = useState<UsageBundle | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [lastUpdated, setLastUpdated] = useState<string | null>(null);
  const [filterPool, setFilterPool] = useState("all");
  const [filterStatus, setFilterStatus] = useState("all");
  const [filterModel, setFilterModel] = useState("all");
  const [expandedRowId, setExpandedRowId] = useState<number | null>(null);
  const [notificationChannels, setNotificationChannels] = useState<NotificationChannel[]>([]);
  const [notificationDeliveries, setNotificationDeliveries] = useState<NotificationDelivery[]>([]);
  const [notificationFilterChannel, setNotificationFilterChannel] = useState("all");
  const [notificationFilterEvent, setNotificationFilterEvent] = useState("all");
  const [notificationFilterStatus, setNotificationFilterStatus] =
    useState<NotificationStatusFilter>("all");
  const [notificationExpandedId, setNotificationExpandedId] = useState<number | null>(null);
  const [notificationLoadingMore, setNotificationLoadingMore] = useState(false);
  const [notificationHasMore, setNotificationHasMore] = useState(false);
  const loadRequestIdRef = useRef(0);
  const notificationRequestIdRef = useRef(0);

  function load() {
    const requestId = ++loadRequestIdRef.current;
    setLoading(true);
    setError(null);
    loadNotificationDeliveries(true);
    Promise.all([
      api.get<Overview>("/overview"),
      api.get<Pool[]>("/pools"),
      api.get<NotificationChannel[]>("/notifications/channels"),
      loadUsageBundle(range),
    ])
      .then(([overviewData, poolData, channelData, usageData]) => {
        if (requestId !== loadRequestIdRef.current) {
          return;
        }
        setOverview(overviewData);
        setPools(poolData ?? []);
        setNotificationChannels(channelData ?? []);
        setUsage(usageData);
        setLastUpdated(new Date().toISOString());
      })
      .catch((err) => {
        if (requestId !== loadRequestIdRef.current) {
          return;
        }
        setError(err instanceof Error ? err.message : "Activity 加载失败");
      })
      .finally(() => {
        if (requestId === loadRequestIdRef.current) {
          setLoading(false);
        }
      });
  }

  function loadNotificationDeliveries(reset = true) {
    const requestId = ++notificationRequestIdRef.current;
    const before =
      !reset && notificationDeliveries.length
        ? notificationDeliveries[notificationDeliveries.length - 1]
        : null;
    const params = new URLSearchParams();
    params.set("limit", "40");
    if (notificationFilterChannel !== "all") {
      params.set("channel_id", notificationFilterChannel);
    }
    if (notificationFilterEvent !== "all") {
      params.set("event", notificationFilterEvent);
    }
    if (notificationFilterStatus !== "all") {
      if (notificationFilterStatus === "test") {
        params.set("triggered_by", "test");
      } else {
        params.set("status", notificationFilterStatus);
      }
    }
    if (before) {
      params.set("before", before.created_at);
      params.set("before_id", String(before.id));
    }
    if (reset) {
      setNotificationLoadingMore(false);
    } else {
      setNotificationLoadingMore(true);
    }
    api.get<NotificationDelivery[]>(`/notifications/deliveries?${params.toString()}`)
      .then((items) => {
        if (requestId !== notificationRequestIdRef.current) {
          return;
        }
        setNotificationDeliveries((current) =>
          reset ? (items ?? []) : [...current, ...(items ?? [])],
        );
        setNotificationHasMore((items?.length ?? 0) >= 40);
      })
      .catch((err) => {
        if (requestId !== notificationRequestIdRef.current) {
          return;
        }
        setError(err instanceof Error ? err.message : "通知历史加载失败");
      })
      .finally(() => {
        if (requestId === notificationRequestIdRef.current) {
          setNotificationLoadingMore(false);
        }
      });
  }

  useEffect(() => {
    load();
    const timer = window.setInterval(load, 15000);
    return () => window.clearInterval(timer);
  }, [range]);

  useEffect(() => {
    loadNotificationDeliveries(true);
  }, [notificationFilterChannel, notificationFilterEvent, notificationFilterStatus]);

  const poolMap = useMemo(
    () => new Map(pools.map((pool) => [pool.id, pool.label])),
    [pools],
  );

  const modelOptions = useMemo(() => {
    const values = new Set<string>();
    usage?.logs.forEach((log) => {
      const model = log.model_actual || log.model_requested;
      if (model) {
        values.add(model);
      }
    });
    return Array.from(values).sort();
  }, [usage]);

  const notificationEvents = useMemo(() => {
    return Array.from(new Set(notificationDeliveries.map((item) => item.event))).sort();
  }, [notificationDeliveries]);

  const filteredLogs = useMemo(() => {
    const logs = usage?.logs ?? [];
    return logs.filter((log) => {
      if (filterPool !== "all" && String(log.pool_id) !== filterPool) {
        return false;
      }
      if (filterStatus === "success" && !log.success) {
        return false;
      }
      if (filterStatus === "error" && log.success) {
        return false;
      }
      if (
        filterModel !== "all" &&
        log.model_actual !== filterModel &&
        log.model_requested !== filterModel
      ) {
        return false;
      }
      return true;
    });
  }, [filterModel, filterPool, filterStatus, usage]);

  const filtersActive =
    filterPool !== "all" || filterStatus !== "all" || filterModel !== "all";

  const trendBuckets = useMemo(
    () => buildTrendBuckets(filteredLogs, range),
    [filteredLogs, range],
  );

  const poolDistribution = useMemo(
    () =>
      buildDistribution(
        filteredLogs,
        (log) => log.pool_id,
        (key) => poolMap.get(Number(key)) ?? `Pool #${key}`,
      ),
    [filteredLogs, poolMap],
  );

  const modelDistribution = useMemo(
    () =>
      buildDistribution(
        filteredLogs,
        (log) => log.model_actual || log.model_requested || "unknown",
        (key) => String(key),
      ),
    [filteredLogs],
  );

  const summaryMetrics = useMemo(() => {
    const total = !filtersActive && usage ? usage.summary.total_requests : filteredLogs.length;
    const successRate = !filtersActive && usage
      ? usage.summary.success_rate
      : total > 0
        ? filteredLogs.filter((log) => log.success).length / total
        : 0;
    const latencyLogs = filteredLogs.filter((log) => log.latency_ms > 0);
    const averageLatency =
      latencyLogs.length > 0
        ? latencyLogs.reduce((sum, log) => sum + log.latency_ms, 0) / latencyLogs.length
        : 0;
    const totalTokens = !filtersActive && usage
      ? (usage.summary.total_input_tokens ?? 0) + (usage.summary.total_output_tokens ?? 0)
      : filteredLogs.reduce((sum, log) => sum + getRequestTokens(log), 0);

    return {
      total,
      successRate,
      averageLatency,
      totalTokens,
    };
  }, [filteredLogs, filtersActive, usage]);

  if (loading && !usage) {
    return (
      <div className="space-y-8">
        <Skeleton className="h-28 rounded-[2rem]" />
        <Skeleton className="h-28 rounded-[1.8rem]" />
        <div className="grid gap-6 xl:grid-cols-2">
          <Skeleton className="h-72 rounded-[1.8rem]" />
          <Skeleton className="h-72 rounded-[1.8rem]" />
        </div>
        <Skeleton className="h-96 rounded-[1.8rem]" />
      </div>
    );
  }

  if (error) {
    return <ErrorState message={error} onRetry={load} />;
  }

  return (
    <div className="space-y-8">
      <PageHeader
        title="Activity"
        description="查看最近请求与系统运行状态。"
        actions={
          <>
            <div className="flex items-center gap-2 rounded-full border border-moon-200/60 bg-white/62 p-1">
              {RANGE_OPTIONS.map((option) => (
                <button
                  key={option.value}
                  type="button"
                  onClick={() => setRange(option.value)}
                  className={cn(
                    "rounded-full px-3 py-1.5 text-sm transition-colors",
                    range === option.value
                      ? "bg-lunar-100/92 text-moon-800"
                      : "text-moon-500 hover:text-moon-700",
                  )}
                >
                  {option.label}
                </button>
              ))}
            </div>
            <Button variant="outline" onClick={load}>
              <RefreshCw className="size-4" />
              刷新
            </Button>
          </>
        }
        meta={
          lastUpdated ? <span>最后更新 {relativeTime(lastUpdated)}</span> : null
        }
      />

      <section className="surface-section overflow-hidden px-4 py-4 sm:px-5 sm:py-5">
        <div className="grid gap-5 sm:grid-cols-2 xl:grid-cols-4">
          <div className="xl:border-r xl:border-moon-200/45 xl:pr-5">
            <SummaryMetric
              label="今日请求"
              value={compact(overview?.requests_today ?? 0)}
              hint="来自全局今日概览"
            />
          </div>
          <div className="xl:border-r xl:border-moon-200/45 xl:px-5">
            <SummaryMetric
              label="成功率"
              value={pct(summaryMetrics.successRate)}
              hint="当前筛选下的最近请求"
            />
          </div>
          <div className="xl:border-r xl:border-moon-200/45 xl:px-5">
            <SummaryMetric
              label="平均延迟"
              value={latency(summaryMetrics.averageLatency)}
              hint="仅统计已记录延迟的请求"
            />
          </div>
          <div className="xl:pl-5">
            <SummaryMetric
              label="Token 用量"
              value={tokenCount(summaryMetrics.totalTokens)}
              hint="输入与输出合计"
            />
          </div>
        </div>
      </section>

      <section className="grid gap-6 xl:grid-cols-2">
        <TrendBars
          title="请求趋势"
          description="一眼看有没有流量，峰值是否异常。"
          buckets={trendBuckets}
        />
        <SuccessRateLine buckets={trendBuckets} />
      </section>

      <section className="grid gap-6 xl:grid-cols-2">
        <DistributionPanel
          title="Pool 分布"
          description="当前请求主要打到了哪些 Pool。"
          rows={poolDistribution}
        />
        <DistributionPanel
          title="Model 分布"
          description="当前请求主要集中在哪些模型。"
          rows={modelDistribution}
        />
      </section>

      <section className="surface-section px-5 py-5">
        <SectionHeading
          title="Request Logs"
          description="顺着一条请求往下查问题。"
        />

        <div className="mt-5 flex flex-wrap gap-3">
          <select
            value={filterPool}
            onChange={(event) => setFilterPool(event.target.value)}
            className="rounded-full border border-moon-200/70 bg-white/82 px-3 py-2 text-sm text-moon-600"
          >
            <option value="all">全部 Pool</option>
            {pools.map((pool) => (
              <option key={pool.id} value={String(pool.id)}>
                {pool.label}
              </option>
            ))}
          </select>
          <select
            value={filterStatus}
            onChange={(event) => setFilterStatus(event.target.value)}
            className="rounded-full border border-moon-200/70 bg-white/82 px-3 py-2 text-sm text-moon-600"
          >
            <option value="all">全部状态</option>
            <option value="success">成功</option>
            <option value="error">失败</option>
          </select>
          <select
            value={filterModel}
            onChange={(event) => setFilterModel(event.target.value)}
            className="rounded-full border border-moon-200/70 bg-white/82 px-3 py-2 text-sm text-moon-600"
          >
            <option value="all">全部模型</option>
            {modelOptions.map((model) => (
              <option key={model} value={model}>
                {model}
              </option>
            ))}
          </select>
        </div>

        <div className="mt-4 flex flex-wrap items-center justify-between gap-3 text-sm text-moon-400">
          <p>当前显示 {compact(summaryMetrics.total)} 条请求。</p>
          {usage?.truncated ? (
            <p>为保证加载稳定，当前聚合基于最近 {compact(usage.logs.length)} 条请求。</p>
          ) : null}
        </div>

        <div className="mt-5 overflow-x-auto rounded-[1.45rem] border border-moon-200/60">
          <table className="min-w-full text-left text-sm">
            <thead className="bg-moon-100/60 text-xs uppercase tracking-[0.16em] text-moon-400">
              <tr>
                <th className="px-4 py-3">Time</th>
                <th className="px-4 py-3">Request</th>
                <th className="px-4 py-3">Pool</th>
                <th className="px-4 py-3">Model</th>
                <th className="px-4 py-3">Status</th>
                <th className="px-4 py-3">Latency</th>
                <th className="px-4 py-3">Tokens</th>
                <th className="px-4 py-3">Request ID</th>
                <th className="px-4 py-3" />
              </tr>
            </thead>
            <tbody className="divide-y divide-moon-200/50 bg-white/66">
              {filteredLogs.length ? filteredLogs.map((item) => {
                const expanded = expandedRowId === item.id;
                const totalTokens = getRequestTokens(item);
                return (
                  <Fragment key={item.id}>
                    <tr
                      className={cn(
                        "cursor-pointer transition-colors hover:bg-white/55",
                        expanded ? "bg-white/82" : "",
                      )}
                      onClick={() => setExpandedRowId((current) => (current === item.id ? null : item.id))}
                    >
                      <td className="px-4 py-3 text-moon-400">{shortDate(item.created_at)}</td>
                      <td className="px-4 py-3">
                        <div className="space-y-1">
                          <p className="text-moon-700">{getRequestSummary(item)}</p>
                          <p className="text-xs text-moon-400">{item.source_kind || "gateway"}</p>
                        </div>
                      </td>
                      <td className="px-4 py-3 text-moon-500">
                        {poolMap.get(item.pool_id) ?? `#${item.pool_id}`}
                      </td>
                      <td className="px-4 py-3 text-moon-500">
                        {item.model_actual || item.model_requested}
                      </td>
                      <td className="px-4 py-3">
                        <span
                          className={cn(
                            "inline-flex rounded-full px-2 py-1 text-xs",
                            item.success
                              ? "bg-status-green/12 text-status-green"
                              : "bg-status-red/12 text-status-red",
                          )}
                        >
                          {item.success ? "OK" : item.status_code}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-moon-500">{latency(item.latency_ms)}</td>
                      <td className="px-4 py-3 text-moon-500">{compact(totalTokens)}</td>
                      <td className="px-4 py-3 text-moon-400">{item.request_id}</td>
                      <td className="px-4 py-3 text-moon-300">
                        <ChevronDown className={cn("size-4 transition-transform", expanded ? "rotate-180" : "")} />
                      </td>
                    </tr>
                    {expanded ? (
                      <tr className="bg-white/80">
                        <td colSpan={9} className="px-4 py-4">
                          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
                            <div className="space-y-1">
                              <p className="text-[11px] uppercase tracking-[0.16em] text-moon-400">Request ID</p>
                              <p className="break-all text-sm text-moon-700">{item.request_id}</p>
                            </div>
                            <div className="space-y-1">
                              <p className="text-[11px] uppercase tracking-[0.16em] text-moon-400">Requested Model</p>
                              <p className="text-sm text-moon-700">{item.model_requested || "--"}</p>
                            </div>
                            <div className="space-y-1">
                              <p className="text-[11px] uppercase tracking-[0.16em] text-moon-400">Actual Model</p>
                              <p className="text-sm text-moon-700">{item.model_actual || "--"}</p>
                            </div>
                            <div className="space-y-1">
                              <p className="text-[11px] uppercase tracking-[0.16em] text-moon-400">Pool</p>
                              <p className="text-sm text-moon-700">{poolMap.get(item.pool_id) ?? `Pool #${item.pool_id}`}</p>
                            </div>
                            <div className="space-y-1">
                              <p className="text-[11px] uppercase tracking-[0.16em] text-moon-400">Account</p>
                              <p className="text-sm text-moon-700">{item.account_label || `#${item.account_id}`}</p>
                            </div>
                            <div className="space-y-1">
                              <p className="text-[11px] uppercase tracking-[0.16em] text-moon-400">Status Code</p>
                              <p className="text-sm text-moon-700">{item.status_code}</p>
                            </div>
                            <div className="space-y-1">
                              <p className="text-[11px] uppercase tracking-[0.16em] text-moon-400">Latency</p>
                              <p className="text-sm text-moon-700">{latency(item.latency_ms)}</p>
                            </div>
                            <div className="space-y-1">
                              <p className="text-[11px] uppercase tracking-[0.16em] text-moon-400">Tokens</p>
                              <p className="text-sm text-moon-700">
                                输入 {compact(item.input_tokens ?? 0)} / 输出 {compact(item.output_tokens ?? 0)}
                              </p>
                            </div>
                          </div>
                          {item.error_message ? (
                            <div className="mt-4 rounded-[1.1rem] border border-status-red/15 bg-red-50/75 px-4 py-3">
                              <p className="text-[11px] uppercase tracking-[0.16em] text-status-red/75">Error Message</p>
                              <p className="mt-1 text-sm text-status-red">{item.error_message}</p>
                            </div>
                          ) : null}
                        </td>
                      </tr>
                    ) : null}
                  </Fragment>
                );
              }) : (
                <tr>
                  <td colSpan={9} className="px-4 py-12 text-center text-sm text-moon-400">
                    当前筛选下没有可显示的请求。
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </section>

      <section className="surface-section px-5 py-5">
        <SectionHeading
          title="Notifications"
          description="查看每次渠道投递是成功、失败还是被人工测试触发。"
        />

        <div className="mt-5 flex flex-wrap gap-3">
          <select
            value={notificationFilterChannel}
            onChange={(event) => setNotificationFilterChannel(event.target.value)}
            className="rounded-full border border-moon-200/70 bg-white/82 px-3 py-2 text-sm text-moon-600"
          >
            <option value="all">全部渠道</option>
            {notificationChannels.map((channel) => (
              <option key={channel.id} value={String(channel.id)}>
                {channel.name}
              </option>
            ))}
          </select>
          <select
            value={notificationFilterEvent}
            onChange={(event) => setNotificationFilterEvent(event.target.value)}
            className="rounded-full border border-moon-200/70 bg-white/82 px-3 py-2 text-sm text-moon-600"
          >
            <option value="all">全部事件</option>
            {notificationEvents.map((event) => (
              <option key={event} value={event}>
                {event}
              </option>
            ))}
          </select>
          <select
            value={notificationFilterStatus}
            onChange={(event) =>
              setNotificationFilterStatus(event.target.value as NotificationStatusFilter)
            }
            className="rounded-full border border-moon-200/70 bg-white/82 px-3 py-2 text-sm text-moon-600"
          >
            <option value="all">全部状态</option>
            <option value="success">success</option>
            <option value="failed">failed</option>
            <option value="dropped">dropped</option>
            <option value="test">test</option>
          </select>
        </div>

        <div className="mt-4 overflow-x-auto rounded-[1.45rem] border border-moon-200/60">
          <table className="min-w-full text-left text-sm">
            <thead className="bg-moon-100/60 text-xs uppercase tracking-[0.16em] text-moon-400">
              <tr>
                <th className="px-4 py-3">Time</th>
                <th className="px-4 py-3">Channel</th>
                <th className="px-4 py-3">Event</th>
                <th className="px-4 py-3">Severity</th>
                <th className="px-4 py-3">Status</th>
                <th className="px-4 py-3">Upstream</th>
                <th className="px-4 py-3">Latency</th>
                <th className="px-4 py-3">Payload</th>
                <th className="px-4 py-3" />
              </tr>
            </thead>
            <tbody className="divide-y divide-moon-200/50 bg-white/66">
              {notificationDeliveries.length ? notificationDeliveries.map((item) => {
                const expanded = notificationExpandedId === item.id;
                return (
                  <Fragment key={item.id}>
                    <tr
                      className={cn(
                        "cursor-pointer transition-colors hover:bg-white/55",
                        expanded ? "bg-white/82" : "",
                      )}
                      onClick={() =>
                        setNotificationExpandedId((current) =>
                          current === item.id ? null : item.id,
                        )
                      }
                    >
                      <td className="px-4 py-3 text-moon-400">{shortDate(item.created_at)}</td>
                      <td className="px-4 py-3">
                        <div className="space-y-1">
                          <p className="text-moon-700">{item.channel_name}</p>
                          <p className="text-xs text-moon-400">{item.channel_type}</p>
                        </div>
                      </td>
                      <td className="px-4 py-3 text-moon-500">{item.event}</td>
                      <td className="px-4 py-3 text-moon-500">{item.severity}</td>
                      <td className="px-4 py-3">
                        <span
                          className={cn(
                            "inline-flex rounded-full px-2 py-1 text-xs",
                            item.status === "success"
                              ? "bg-status-green/12 text-status-green"
                              : item.status === "failed"
                                ? "bg-status-red/12 text-status-red"
                                : "bg-moon-100/90 text-moon-500",
                          )}
                        >
                          {item.triggered_by === "test" ? "test" : item.status}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-moon-500">{item.upstream_code || "--"}</td>
                      <td className="px-4 py-3 text-moon-500">{latency(item.latency_ms)}</td>
                      <td className="max-w-xs px-4 py-3 text-moon-500">
                        <div className="truncate">{item.payload_summary || "--"}</div>
                      </td>
                      <td className="px-4 py-3 text-moon-300">
                        <ChevronDown className={cn("size-4 transition-transform", expanded ? "rotate-180" : "")} />
                      </td>
                    </tr>
                    {expanded ? (
                      <tr className="bg-white/80">
                        <td colSpan={9} className="px-4 py-4">
                          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
                            <div className="space-y-1">
                              <p className="text-[11px] uppercase tracking-[0.16em] text-moon-400">Title</p>
                              <p className="text-sm text-moon-700">{item.title || "--"}</p>
                            </div>
                            <div className="space-y-1">
                              <p className="text-[11px] uppercase tracking-[0.16em] text-moon-400">Attempt</p>
                              <p className="text-sm text-moon-700">#{item.attempt}</p>
                            </div>
                            <div className="space-y-1">
                              <p className="text-[11px] uppercase tracking-[0.16em] text-moon-400">Triggered By</p>
                              <p className="text-sm text-moon-700">{item.triggered_by}</p>
                            </div>
                            <div className="space-y-1">
                              <p className="text-[11px] uppercase tracking-[0.16em] text-moon-400">Upstream Message</p>
                              <p className="text-sm text-moon-700">{item.upstream_message || "--"}</p>
                            </div>
                          </div>
                          <div className="mt-4 rounded-[1.1rem] border border-moon-200/45 bg-moon-50/80 px-4 py-3">
                            <p className="text-[11px] uppercase tracking-[0.16em] text-moon-400">Payload Summary</p>
                            <p className="mt-2 whitespace-pre-wrap text-sm leading-6 text-moon-600">
                              {item.payload_summary || "没有可展示的摘要。"}
                            </p>
                          </div>
                        </td>
                      </tr>
                    ) : null}
                  </Fragment>
                );
              }) : (
                <tr>
                  <td colSpan={9} className="px-4 py-12 text-center text-sm text-moon-400">
                    当前筛选下没有可显示的通知投递记录。
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>

        {notificationHasMore ? (
          <div className="mt-4 flex justify-center">
            <Button
              variant="outline"
              onClick={() => loadNotificationDeliveries(false)}
              disabled={notificationLoadingMore}
            >
              {notificationLoadingMore ? (
                <RefreshCw className="size-4 animate-spin" />
              ) : null}
              Load More
            </Button>
          </div>
        ) : null}
      </section>
    </div>
  );
}
