import { Fragment, useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  Link2,
  RefreshCw,
  X,
} from "lucide-react";
import ErrorState from "@/components/ErrorState";
import PageHeader from "@/components/PageHeader";
import SectionHeading from "@/components/SectionHeading";
import SideTOC, { type TOCSection } from "@/components/SideTOC";
import { toast } from "@/components/Feedback";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { api } from "@/lib/api";
import { compact, latency, pct, relativeTime, shortDate } from "@/lib/fmt";
import type {
  Pool,
  RequestLog,
  UsageLogPage,
  UsageStats,
} from "@/lib/types";
import { cn } from "@/lib/utils";

type UsageResponse = UsageStats & {
  logs: UsageLogPage;
};

type UsageBundle = {
  summary: UsageStats;
  logs: RequestLog[];
  total: number;
  truncated: boolean;
};

const PAGE_SIZE = 250;
const MAX_PAGES = 12;
const AUTO_REFRESH_MS = 30_000;
// Rows per in-UI page of the Request Logs table. Deliberately small so an
// expanded row + its neighbours fit one viewport without scrolling.
const LOGS_PER_PAGE = 5;

const DASH = "—";

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

function within(log: RequestLog, hoursAgo: number): boolean {
  const ts = parseAdminDate(log.created_at).getTime();
  if (!Number.isFinite(ts)) return false;
  return ts >= Date.now() - hoursAgo * 3600_000;
}

async function loadUsage30d(): Promise<UsageBundle> {
  const firstPage = await api.get<UsageResponse>(
    `/usage?range=30d&page=1&page_size=${PAGE_SIZE}`,
  );
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
      api.get<UsageResponse>(
        `/usage?range=30d&page=${index + 2}&page_size=${PAGE_SIZE}`,
      ),
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

const TOC_SECTIONS: TOCSection[] = [
  { id: "digest", label: "日报" },
  { id: "trends", label: "趋势" },
  { id: "flow", label: "流向" },
  { id: "health", label: "健康" },
  { id: "logs", label: "Logs" },
];

function DigestLine({ children }: { children: React.ReactNode }) {
  return <p className="text-[0.97rem] leading-7 text-moon-700">{children}</p>;
}

// --- Daily Digest ----------------------------------------------------------
// Digest window = rolling 24h ending now; comparison window = the 24h before that.
// Everything below treats `logs30d` as the source of truth so we never mix
// calendar-day metrics with rolling-window ones (the old overview/logs24h
// split did exactly that and produced off-by-several numbers on some days).

const DIGEST_MIN_SAMPLES = 5;
const SUCCESS_GREEN = 0.98;
const SUCCESS_YELLOW = 0.9;
const CONCENTRATION_THRESHOLD = 0.6;
const BUSY_HOUR_THRESHOLD = 0.4;
const LATENCY_SLOW_MS = 8000;
const LATENCY_IMPROVE_RATIO = 0.8;

type DigestFacts = {
  total: number;
  success: number;
  failed: number;
  successRate: number; // NaN when total === 0
  avgLatencyMs: number;
  p95LatencyMs: number;
  streamCount: number;
  retriesSaved: number;
  routeRejected: number;
  busiestHour: { hour: number; count: number } | null;
  quietHours: number;
  topError: { statusCode: number; message: string; count: number } | null;
  topPool: { id: number; label: string; count: number; share: number } | null;
  topModel: { name: string; count: number; share: number } | null;
  prevTotal: number;
  prevSuccessRate: number; // NaN when prevTotal < MIN
  prevAvgLatencyMs: number;
  prevP95LatencyMs: number;
};

type DigestLineSpec = {
  key: string;
  priority: number;
  node: React.ReactNode;
};

// djb2 hash — picks a variant deterministically from a data-derived seed so
// the digest stops reshuffling its wording on every 30s auto-refresh when the
// underlying numbers didn't budge.
function pickVariant<T>(seed: string, variants: T[]): T {
  let h = 5381;
  for (let i = 0; i < seed.length; i++) {
    h = ((h << 5) + h) ^ seed.charCodeAt(i);
  }
  return variants[Math.abs(h) % variants.length];
}

function computeDigestFacts(
  logs30d: RequestLog[],
  poolMap: Map<number, string>,
): DigestFacts {
  const nowMs = Date.now();
  const todayStartMs = nowMs - 24 * 3600_000;
  const yesterdayStartMs = nowMs - 48 * 3600_000;

  const today: RequestLog[] = [];
  const yesterday: RequestLog[] = [];
  for (const log of logs30d) {
    const ts = parseAdminDate(log.created_at).getTime();
    if (!Number.isFinite(ts)) continue;
    if (ts >= todayStartMs && ts <= nowMs) today.push(log);
    else if (ts >= yesterdayStartMs && ts < todayStartMs) yesterday.push(log);
  }

  const total = today.length;
  const success = today.filter((l) => l.success).length;
  const failed = total - success;
  const successRate = total > 0 ? success / total : NaN;

  const successLatencies = today
    .filter((l) => l.success && l.latency_ms > 0)
    .map((l) => l.latency_ms);
  const avgLatencyMs =
    successLatencies.length > 0
      ? successLatencies.reduce((s, v) => s + v, 0) / successLatencies.length
      : 0;
  const sortedLatencies = [...successLatencies].sort((a, b) => a - b);
  const p95LatencyMs = percentile(sortedLatencies, 0.95);

  const streamCount = today.filter((l) => l.stream).length;
  const retriesSaved = today.filter(
    (l) => l.success && l.attempt_count > 1,
  ).length;
  const routeRejected = today.filter(
    (l) => !l.success && l.attempt_count === 0,
  ).length;

  // Reuse buildBuckets for hour-granular stats. Its window is calendar-hour
  // aligned (≈23-24h) rather than exact rolling 24h, but the shape we need
  // (busiest single hour, number of quiet hours) is robust to that drift.
  const buckets = buildBuckets(today, "24h");
  let busiestHour: DigestFacts["busiestHour"] = null;
  let quietHours = 0;
  for (const b of buckets) {
    if (b.total === 0) quietHours += 1;
    if (busiestHour === null || b.total > busiestHour.count) {
      busiestHour = { hour: parseInt(b.key, 10), count: b.total };
    }
  }
  if (!busiestHour || busiestHour.count === 0) busiestHour = null;

  // Top error bucketing — mirror the TopErrors component so the digest
  // "失败里最多的一类是 X" and the Top Errors list agree on the #1 row.
  const errorBuckets = new Map<
    string,
    { statusCode: number; message: string; count: number }
  >();
  today.forEach((log) => {
    if (log.success) return;
    const msg = log.error_message || `HTTP ${log.status_code}`;
    const key = `${log.status_code}:${msg.slice(0, 120)}`;
    const existing = errorBuckets.get(key);
    if (existing) existing.count += 1;
    else
      errorBuckets.set(key, {
        statusCode: log.status_code,
        message: msg,
        count: 1,
      });
  });
  const topError =
    errorBuckets.size > 0
      ? Array.from(errorBuckets.values()).sort((a, b) => b.count - a.count)[0]
      : null;

  // Concentration: only worth calling out when one pool/model is dominant.
  // Below CONCENTRATION_THRESHOLD we return null rather than produce a
  // noisy "pool X has 42%" line that reads like filler on a normal day.
  const poolCounts = new Map<number, number>();
  today.forEach((log) => {
    poolCounts.set(log.pool_id, (poolCounts.get(log.pool_id) ?? 0) + 1);
  });
  let topPool: DigestFacts["topPool"] = null;
  if (poolCounts.size > 0 && total > 0) {
    const sorted = Array.from(poolCounts.entries()).sort(
      (a, b) => b[1] - a[1],
    );
    const [id, count] = sorted[0];
    const share = count / total;
    if (share >= CONCENTRATION_THRESHOLD) {
      topPool = {
        id,
        label: poolMap.get(id) ?? `Pool #${id}`,
        count,
        share,
      };
    }
  }

  const modelCounts = new Map<string, number>();
  today.forEach((log) => {
    const name = log.model_actual || log.model_requested || "unknown";
    modelCounts.set(name, (modelCounts.get(name) ?? 0) + 1);
  });
  let topModel: DigestFacts["topModel"] = null;
  if (modelCounts.size > 0 && total > 0) {
    const sorted = Array.from(modelCounts.entries()).sort(
      (a, b) => b[1] - a[1],
    );
    const [name, count] = sorted[0];
    const share = count / total;
    if (share >= CONCENTRATION_THRESHOLD) {
      topModel = { name, count, share };
    }
  }

  const prevTotal = yesterday.length;
  const prevSuccess = yesterday.filter((l) => l.success).length;
  const prevSuccessRate =
    prevTotal >= DIGEST_MIN_SAMPLES ? prevSuccess / prevTotal : NaN;
  const prevSuccessLatencies = yesterday
    .filter((l) => l.success && l.latency_ms > 0)
    .map((l) => l.latency_ms);
  const prevAvgLatencyMs =
    prevSuccessLatencies.length > 0
      ? prevSuccessLatencies.reduce((s, v) => s + v, 0) /
        prevSuccessLatencies.length
      : 0;
  const prevSortedLatencies = [...prevSuccessLatencies].sort((a, b) => a - b);
  const prevP95LatencyMs = percentile(prevSortedLatencies, 0.95);

  return {
    total,
    success,
    failed,
    successRate,
    avgLatencyMs,
    p95LatencyMs,
    streamCount,
    retriesSaved,
    routeRejected,
    busiestHour,
    quietHours,
    topError,
    topPool,
    topModel,
    prevTotal,
    prevSuccessRate,
    prevAvgLatencyMs,
    prevP95LatencyMs,
  };
}

function buildGreeting(): string {
  const h = new Date().getHours();
  if (h < 5) return "深夜好";
  if (h < 11) return "早上好";
  if (h < 14) return "午后";
  if (h < 18) return "下午好";
  if (h < 22) return "晚上好";
  return "夜深了";
}

function buildVolumeNode(
  facts: DigestFacts,
  greeting: string,
  seed: string,
): React.ReactNode {
  const { total, streamCount, prevTotal, avgLatencyMs } = facts;

  const deltaNote = (() => {
    if (prevTotal < DIGEST_MIN_SAMPLES) return null;
    const diff = total - prevTotal;
    const ratio = total / Math.max(prevTotal, 1);
    if (Math.abs(diff) < 3) return null; // too noisy, skip
    if (ratio >= 1.5) {
      return pickVariant(seed + "vol-up", [
        `比昨天这会儿热闹（昨天 ${compact(prevTotal)}）`,
        `比昨天多出一截（昨天 ${compact(prevTotal)}）`,
      ]);
    }
    if (ratio <= 0.6) {
      // Variants must be ratio-agnostic — the <=0.6 band covers both modest
      // dips and near-empty days, so quantitative wording like "差不多一半"
      // would mislead on the heavy-drop end.
      return pickVariant(seed + "vol-down", [
        `比昨天冷清（昨天 ${compact(prevTotal)}）`,
        `比昨天少了不少（昨天 ${compact(prevTotal)}）`,
      ]);
    }
    return null;
  })();

  return (
    <>
      {greeting}，过去 24 小时已经跑过{" "}
      <strong className="font-semibold text-moon-800">{compact(total)}</strong>{" "}
      次请求
      {avgLatencyMs > 0 ? (
        <>
          ，平均延迟{" "}
          <strong className="font-semibold text-moon-800">
            {latency(avgLatencyMs)}
          </strong>
        </>
      ) : null}
      {streamCount > 0 ? (
        <>，其中 {compact(streamCount)} 条走了 stream</>
      ) : null}
      {deltaNote ? <>，{deltaNote}</> : null}。
    </>
  );
}

function buildRouteRejectedLine(
  facts: DigestFacts,
  seed: string,
): DigestLineSpec | null {
  if (facts.routeRejected === 0) return null;
  const msg = pickVariant(seed + "route", [
    `有 ${compact(facts.routeRejected)} 次请求连上游都没打到，被路由直接拒了`,
    `${compact(facts.routeRejected)} 次请求卡在路由层就失败了`,
    `看到 ${compact(facts.routeRejected)} 次路由拒绝，可能是模型没配或 pool 不健康`,
  ]);
  return {
    key: "route-rejected",
    priority: 100,
    node: (
      <>
        <strong className="font-semibold text-status-red">{msg}</strong>
        ，值得查一下路由配置。
      </>
    ),
  };
}

function buildSuccessRateLine(
  facts: DigestFacts,
  seed: string,
): DigestLineSpec | null {
  if (facts.total < DIGEST_MIN_SAMPLES) return null;
  const rate = pct(facts.successRate);
  const diffNote = (() => {
    if (!Number.isFinite(facts.prevSuccessRate)) return null;
    const diff = facts.successRate - facts.prevSuccessRate;
    if (diff >= 0.05) {
      return pickVariant(seed + "rate-up", [
        "比昨天更稳了",
        "比昨天好一截",
      ]);
    }
    if (diff <= -0.05) {
      return pickVariant(seed + "rate-down", [
        "比昨天明显差了一些",
        "比昨天下滑了",
      ]);
    }
    return null;
  })();

  if (facts.successRate >= SUCCESS_GREEN) {
    const tail = pickVariant(seed + "rate-green", [
      "几乎一路绿灯",
      "挺省心的",
      "今天状态不错",
    ]);
    return {
      key: "success-rate",
      priority: 40,
      node: (
        <>
          成功率{" "}
          <strong className="font-semibold text-status-green">{rate}</strong>
          ，{tail}
          {diffNote ? `，${diffNote}` : ""}。
        </>
      ),
    };
  }
  if (facts.successRate >= SUCCESS_YELLOW) {
    return {
      key: "success-rate",
      priority: 60,
      node: (
        <>
          成功率{" "}
          <strong className="font-semibold text-status-yellow">{rate}</strong>
          {facts.failed > 0 ? `，有 ${compact(facts.failed)} 次最终失败` : ""}
          {diffNote ? `，${diffNote}` : ""}，翻一下 Top Errors 看看。
        </>
      ),
    };
  }
  return {
    key: "success-rate",
    priority: 95,
    node: (
      <>
        成功率跌到{" "}
        <strong className="font-semibold text-status-red">{rate}</strong>
        {facts.failed > 0
          ? `，${compact(facts.failed)} 次请求直接掉给了客户端`
          : ""}
        {diffNote ? `，${diffNote}` : ""}，Top Errors 是第一优先级。
      </>
    ),
  };
}

function buildTopErrorLine(
  facts: DigestFacts,
  seed: string,
): DigestLineSpec | null {
  // Threshold aligned with SuccessRate's yellow/red branches, which both
  // invite the user to "翻一下 Top Errors 看看". If we gated higher, that
  // prompt could hang with no follow-up row when failed ∈ {1, 2}.
  if (!facts.topError || facts.failed < 2) return null;
  const { statusCode, message, count } = facts.topError;
  const label = statusCode === 0 ? "网络故障" : `HTTP ${statusCode}`;
  const trimmed = message.length > 52 ? message.slice(0, 52) + "…" : message;
  const frame = pickVariant(seed + "top-err", [
    `失败里出镜最多的是 ${label}`,
    `最频繁的失败是 ${label}`,
    `今天撞得最多的错是 ${label}`,
  ]);
  return {
    key: "top-error",
    priority: 70,
    node: (
      <>
        {frame}（{compact(count)} 次），
        <span className="text-moon-500">「{trimmed}」</span>
        。
      </>
    ),
  };
}

function buildLatencyLine(
  facts: DigestFacts,
  seed: string,
): DigestLineSpec | null {
  if (facts.p95LatencyMs > 0 && facts.p95LatencyMs >= LATENCY_SLOW_MS) {
    const frame = pickVariant(seed + "lat-slow", [
      "尾部延迟偏慢",
      "慢请求比较明显",
    ]);
    return {
      key: "latency",
      priority: 50,
      node: (
        <>
          {frame}，p95 已经到{" "}
          <strong className="font-semibold text-status-yellow">
            {latency(facts.p95LatencyMs)}
          </strong>
          ，看看是不是某个 account 拖后腿。
        </>
      ),
    };
  }
  if (
    facts.p95LatencyMs > 0 &&
    facts.prevP95LatencyMs > 0 &&
    // Gate on prevTotal so a single slow request yesterday can't masquerade
    // as a full "p95" and trip the improvement line on a drive-by basis.
    facts.prevTotal >= DIGEST_MIN_SAMPLES &&
    facts.p95LatencyMs < facts.prevP95LatencyMs * LATENCY_IMPROVE_RATIO
  ) {
    const frame = pickVariant(seed + "lat-fast", [
      "长尾延迟明显收敛",
      "慢请求少了很多",
    ]);
    return {
      key: "latency",
      priority: 30,
      node: (
        <>
          {frame}——p95 从 {latency(facts.prevP95LatencyMs)} 降到{" "}
          <strong className="font-semibold text-status-green">
            {latency(facts.p95LatencyMs)}
          </strong>
          。
        </>
      ),
    };
  }
  return null;
}

function buildConcentrationLine(
  facts: DigestFacts,
  seed: string,
): DigestLineSpec | null {
  if (facts.topPool) {
    const frame = pickVariant(seed + "conc-pool", [
      `流量压在了 ${facts.topPool.label} 一个 pool 上，占 ${pct(facts.topPool.share)}`,
      `${facts.topPool.label} 扛了 ${pct(facts.topPool.share)} 的量`,
    ]);
    return {
      key: "concentration",
      priority: 25,
      node: (
        <>
          {frame}
          {facts.topPool.share >= 0.85 ? "，别的 pool 基本在睡" : ""}。
        </>
      ),
    };
  }
  if (facts.topModel) {
    const frame = pickVariant(seed + "conc-model", [
      `${facts.topModel.name} 一个模型就吃掉 ${pct(facts.topModel.share)}`,
      `${pct(facts.topModel.share)} 的请求都打在 ${facts.topModel.name} 上`,
    ]);
    return {
      key: "concentration",
      priority: 20,
      node: <>{frame}。</>,
    };
  }
  return null;
}

function buildBusyHourLine(
  facts: DigestFacts,
  seed: string,
): DigestLineSpec | null {
  const { busiestHour, total } = facts;
  if (!busiestHour || total === 0) return null;
  const share = busiestHour.count / total;
  if (share < BUSY_HOUR_THRESHOLD) return null;
  const hh = `${busiestHour.hour}:00`;
  const frame = pickVariant(seed + "busy", [
    `${hh} 那小时占了今天 ${pct(share)}`,
    `有个明显的尖峰在 ${hh}，占了 ${pct(share)}`,
  ]);
  return {
    key: "busy-hour",
    priority: 15,
    node: <>{frame}。</>,
  };
}

function buildRetriesLine(
  facts: DigestFacts,
  seed: string,
): DigestLineSpec | null {
  if (facts.retriesSaved === 0) return null;
  const frame = pickVariant(seed + "retry", [
    `重试救回了 ${compact(facts.retriesSaved)} 次请求`,
    `${compact(facts.retriesSaved)} 次请求是靠重试才成的`,
    `今天有 ${compact(facts.retriesSaved)} 次请求踩了坑但被重试捞回`,
  ]);
  return {
    key: "retries",
    priority: 10,
    node: (
      <>
        <strong className="font-semibold text-lunar-700">{frame}</strong>
        ——用户侧看到的是成功，背后确实抖了一下。
      </>
    ),
  };
}

function DailyDigest({
  logs30d,
  poolMap,
}: {
  logs30d: RequestLog[];
  poolMap: Map<number, string>;
}) {
  const facts = useMemo(
    () => computeDigestFacts(logs30d, poolMap),
    [logs30d, poolMap],
  );
  const greeting = buildGreeting();
  // Seed is a fingerprint of the numbers a variant could reasonably depend on.
  // Same numbers → same wording across refreshes; different numbers → maybe
  // a different phrasing. Keep it stable (don't mix in Date.now()).
  const seed = `${facts.total}:${facts.success}:${facts.failed}:${Math.round(
    (Number.isFinite(facts.successRate) ? facts.successRate : 0) * 1000,
  )}`;

  const lines: React.ReactNode[] = [];

  if (facts.total === 0) {
    lines.push(
      <DigestLine key="idle">
        {greeting}，过去 24 小时还没有请求经过 Lune，先把 token 或客户端配好试一条吧。
      </DigestLine>,
    );
  } else {
    lines.push(
      <DigestLine key="volume">
        {buildVolumeNode(facts, greeting, seed)}
      </DigestLine>,
    );

    // Priority-ordered extras. We include at most two below the volume line
    // so the digest stays 1-3 lines total — enough to read in one glance
    // without turning into a bullet list.
    const candidates: DigestLineSpec[] = [
      buildRouteRejectedLine(facts, seed),
      buildSuccessRateLine(facts, seed),
      buildTopErrorLine(facts, seed),
      buildLatencyLine(facts, seed),
      buildConcentrationLine(facts, seed),
      buildBusyHourLine(facts, seed),
      buildRetriesLine(facts, seed),
    ].filter((x): x is DigestLineSpec => x !== null);

    candidates.sort((a, b) => b.priority - a.priority);
    for (const spec of candidates.slice(0, 2)) {
      lines.push(<DigestLine key={spec.key}>{spec.node}</DigestLine>);
    }
  }

  return (
    <section
      id="digest"
      className="surface-section scroll-mt-6 px-5 py-5 sm:px-6 sm:py-6"
    >
      <SectionHeading
        title="Daily Digest"
        description="过去 24 小时的网关状态，一段话看完。"
      />
      <div className="mt-4 space-y-2">{lines}</div>
    </section>
  );
}

type TrendBucket = {
  key: string;
  label: string;
  total: number;
  success: number;
  fail: number;
};

function buildBuckets(
  logs: RequestLog[],
  range: "24h" | "7d" | "30d",
): TrendBucket[] {
  const now = new Date();
  const count = range === "24h" ? 24 : range === "7d" ? 7 : 30;
  const start = new Date(now);
  if (range === "24h") {
    start.setMinutes(0, 0, 0);
    start.setHours(start.getHours() - 23);
  } else {
    start.setHours(0, 0, 0, 0);
    start.setDate(start.getDate() - (range === "7d" ? 6 : 29));
  }

  const buckets = Array.from({ length: count }, (_, index) => {
    const date = new Date(start);
    if (range === "24h") {
      date.setHours(start.getHours() + index, 0, 0, 0);
      return {
        key: `${date.getHours()}`,
        label: `${String(date.getHours()).padStart(2, "0")}:00`,
        total: 0,
        success: 0,
        fail: 0,
      } as TrendBucket;
    }
    date.setDate(start.getDate() + index);
    return {
      key: `${date.getMonth() + 1}-${date.getDate()}`,
      label: `${date.getMonth() + 1}/${date.getDate()}`,
      total: 0,
      success: 0,
      fail: 0,
    } as TrendBucket;
  });

  const index = (d: Date) => {
    if (range === "24h") {
      return Math.floor((d.getTime() - start.getTime()) / 3600_000);
    }
    const clone = new Date(d);
    clone.setHours(0, 0, 0, 0);
    return Math.floor((clone.getTime() - start.getTime()) / (24 * 3600_000));
  };

  logs.forEach((log) => {
    const d = parseAdminDate(log.created_at);
    if (!Number.isFinite(d.getTime())) return;
    if (d < start || d > now) return;
    const i = index(d);
    if (i < 0 || i >= count) return;
    buckets[i].total += 1;
    if (log.success) buckets[i].success += 1;
    else buckets[i].fail += 1;
  });

  return buckets;
}

function StackedTrendBars({
  title,
  description,
  buckets,
}: {
  title: string;
  description: string;
  buckets: TrendBucket[];
}) {
  const max = Math.max(...buckets.map((b) => b.total), 1);
  const mid = Math.floor((buckets.length - 1) / 2);
  const totalAll = buckets.reduce((sum, b) => sum + b.total, 0);

  return (
    <section className="surface-section flex h-full flex-col px-5 py-5">
      <SectionHeading title={title} description={description} />
      <div className="mt-5 flex flex-1 flex-col">
        <div className="flex h-44 items-end gap-1.5">
          {buckets.map((bucket, idx) => {
            const heightPct =
              bucket.total > 0
                ? Math.max((bucket.total / max) * 100, 6)
                : 2;
            const successPct =
              bucket.total > 0 ? (bucket.success / bucket.total) * 100 : 0;
            const title =
              bucket.total > 0
                ? `${bucket.label} · ${bucket.success} 成功 / ${bucket.fail} 失败 (${pct(
                    bucket.success / bucket.total,
                  )})`
                : `${bucket.label} · 无请求`;
            return (
              <div
                key={`${bucket.key}-${idx}`}
                title={title}
                // h-full + flex-col + justify-end: the parent flex row uses
                // items-end, which would collapse each cell to intrinsic height
                // and break the bar's percentage height (no definite parent).
                // Forcing h-full gives the inner bar a concrete base to scale
                // against, then justify-end pins the bar to the baseline.
                className="group flex h-full min-w-0 flex-1 flex-col justify-end"
              >
                <div
                  className="w-full overflow-hidden rounded-t-[0.6rem] bg-moon-100/60 transition-opacity duration-200 group-hover:opacity-90"
                  style={{ height: `${heightPct}%` }}
                >
                  {bucket.total > 0 ? (
                    <div className="flex h-full w-full flex-col">
                      <div
                        className="w-full bg-[linear-gradient(180deg,rgba(190,116,118,0.78),rgba(190,116,118,0.55))]"
                        style={{ height: `${100 - successPct}%` }}
                      />
                      <div
                        className="w-full bg-[linear-gradient(180deg,rgba(93,159,135,0.78),rgba(93,159,135,0.45))]"
                        style={{ height: `${successPct}%` }}
                      />
                    </div>
                  ) : null}
                </div>
              </div>
            );
          })}
        </div>
        <div className="mt-3 flex items-center justify-between text-xs text-moon-400">
          <span>{buckets[0]?.label}</span>
          <span>{buckets[mid]?.label}</span>
          <span>{buckets[buckets.length - 1]?.label}</span>
        </div>
        <p className="mt-4 text-xs text-moon-400">
          {totalAll > 0 ? (
            <>共 {compact(totalAll)} 次请求</>
          ) : (
            <>{DASH}</>
          )}
        </p>
      </div>
    </section>
  );
}

function MoonDial({ buckets }: { buckets: TrendBucket[] }) {
  const max = Math.max(...buckets.map((b) => b.total), 1);
  const radiusOuter = 96;
  const radiusInner = 42;
  const center = 110;
  const size = 220;
  const totalAll = buckets.reduce((sum, b) => sum + b.total, 0);

  return (
    <section className="surface-section flex h-full flex-col px-5 py-5">
      <SectionHeading
        title="24h 月相"
        description="逆时针绕着圆盘走一天，亮的那半代表活跃时段。"
      />
      <div className="mt-5 flex flex-1 flex-col items-center justify-center">
        <svg
          width={size}
          height={size}
          viewBox={`0 0 ${size} ${size}`}
          className="overflow-visible"
          aria-label="24 hour activity dial"
        >
          <defs>
            <radialGradient id="moondial-bg" cx="50%" cy="50%" r="50%">
              <stop offset="0%" stopColor="rgba(244,241,250,0.75)" />
              <stop offset="100%" stopColor="rgba(223,218,240,0.3)" />
            </radialGradient>
          </defs>
          <circle
            cx={center}
            cy={center}
            r={radiusOuter + 6}
            fill="url(#moondial-bg)"
          />
          <circle
            cx={center}
            cy={center}
            r={radiusInner}
            fill="rgba(255,255,255,0.55)"
            stroke="rgba(200,194,226,0.5)"
            strokeWidth={0.8}
          />
          {/* 6/12/18/24 tick marks */}
          {[0, 6, 12, 18].map((h) => {
            const angle = (-90 + h * 15) * (Math.PI / 180);
            const x1 = center + Math.cos(angle) * (radiusOuter + 3);
            const y1 = center + Math.sin(angle) * (radiusOuter + 3);
            const xL = center + Math.cos(angle) * (radiusOuter + 16);
            const yL = center + Math.sin(angle) * (radiusOuter + 16);
            return (
              <g key={h}>
                <circle cx={x1} cy={y1} r={1.2} fill="rgba(140,134,173,0.55)" />
                <text
                  x={xL}
                  y={yL + 3}
                  textAnchor="middle"
                  className="fill-moon-400"
                  style={{ fontSize: "10px" }}
                >
                  {h === 0 ? "0" : h}
                </text>
              </g>
            );
          })}
          {buckets.map((bucket, i) => {
            if (bucket.total === 0) {
              return null;
            }
            const angle = (-90 + i * 15) * (Math.PI / 180);
            const length = (bucket.total / max) * (radiusOuter - radiusInner - 4);
            const rInner = radiusInner + 2;
            const rOuter = rInner + length;
            const x1 = center + Math.cos(angle) * rInner;
            const y1 = center + Math.sin(angle) * rInner;
            const x2 = center + Math.cos(angle) * rOuter;
            const y2 = center + Math.sin(angle) * rOuter;
            const successRate =
              bucket.total > 0 ? bucket.success / bucket.total : 1;
            const color =
              successRate >= 0.98
                ? "rgba(93,159,135,0.85)"
                : successRate >= 0.9
                  ? "rgba(192,154,85,0.85)"
                  : "rgba(190,116,118,0.85)";
            const tooltip = `${bucket.label} · ${bucket.success} 成功 / ${bucket.fail} 失败 (${pct(
              successRate,
            )})`;
            return (
              <line
                key={i}
                x1={x1}
                y1={y1}
                x2={x2}
                y2={y2}
                stroke={color}
                strokeWidth={4.2}
                strokeLinecap="round"
              >
                <title>{tooltip}</title>
              </line>
            );
          })}
          <text
            x={center}
            y={center - 4}
            textAnchor="middle"
            className="fill-moon-700"
            style={{ fontSize: "16px", fontWeight: 600 }}
          >
            {totalAll > 0 ? compact(totalAll) : DASH}
          </text>
          <text
            x={center}
            y={center + 14}
            textAnchor="middle"
            className="fill-moon-400"
            style={{ fontSize: "10px", letterSpacing: "0.14em" }}
          >
            24H
          </text>
        </svg>
        <p className="mt-3 text-xs text-moon-400">
          {totalAll > 0 ? (
            <>悬停查看每小时的成功/失败明细</>
          ) : (
            <>过去 24 小时无请求</>
          )}
        </p>
      </div>
    </section>
  );
}

type SankeyNode = { id: string; label: string; value: number };
type SankeyLink = { source: string; target: string; value: number };

function SankeyDiagram({
  title,
  description,
  logs,
  poolMap,
}: {
  title: string;
  description: string;
  logs: RequestLog[];
  poolMap: Map<number, string>;
}) {
  const { pools, models, links } = useMemo(() => {
    const poolCounts = new Map<string, number>();
    const modelCounts = new Map<string, number>();
    const linkCounts = new Map<string, number>();
    logs.forEach((log) => {
      const poolKey = `pool:${log.pool_id}`;
      const modelKey = `model:${log.model_actual || log.model_requested || "unknown"}`;
      poolCounts.set(poolKey, (poolCounts.get(poolKey) ?? 0) + 1);
      modelCounts.set(modelKey, (modelCounts.get(modelKey) ?? 0) + 1);
      const linkKey = `${poolKey}>>${modelKey}`;
      linkCounts.set(linkKey, (linkCounts.get(linkKey) ?? 0) + 1);
    });

    const pools: SankeyNode[] = Array.from(poolCounts.entries())
      .map(([id, value]) => {
        const poolId = Number(id.slice(5));
        return {
          id,
          label: poolMap.get(poolId) ?? `Pool #${poolId}`,
          value,
        };
      })
      .sort((a, b) => b.value - a.value);

    const models: SankeyNode[] = Array.from(modelCounts.entries())
      .map(([id, value]) => ({ id, label: id.slice(6), value }))
      .sort((a, b) => b.value - a.value);

    const links: SankeyLink[] = Array.from(linkCounts.entries()).map(
      ([key, value]) => {
        const [source, target] = key.split(">>");
        return { source, target, value };
      },
    );
    return { pools, models, links };
  }, [logs, poolMap]);

  const totalAll = pools.reduce((sum, node) => sum + node.value, 0);

  const layout = useMemo(() => {
    const width = 600;
    const height = Math.max(
      220,
      Math.max(pools.length, models.length) * 34 + 40,
    );
    const paddingY = 10;
    const nodeWidth = 10;
    const nodeGap = 10;
    const innerHeight = height - paddingY * 2;

    const totalPool = pools.reduce((sum, n) => sum + n.value, 0) || 1;
    const totalModel = models.reduce((sum, n) => sum + n.value, 0) || 1;

    const computeColumn = (nodes: SankeyNode[], x: number, total: number) => {
      const available = innerHeight - (nodes.length - 1) * nodeGap;
      const positions: Record<
        string,
        { x: number; y: number; h: number; label: string; value: number }
      > = {};
      let y = paddingY;
      nodes.forEach((node) => {
        const h = Math.max((node.value / total) * available, 6);
        positions[node.id] = {
          x,
          y,
          h,
          label: node.label,
          value: node.value,
        };
        y += h + nodeGap;
      });
      return positions;
    };

    const poolPositions = computeColumn(pools, 40, totalPool);
    const modelPositions = computeColumn(
      models,
      width - 40 - nodeWidth,
      totalModel,
    );

    // Link endpoints: we walk each source/target, peeling off height
    // proportional to the link value (Sankey ribbon layout).
    const sourceCursor: Record<string, number> = {};
    const targetCursor: Record<string, number> = {};
    const paths = links
      .sort((a, b) => b.value - a.value)
      .map((link) => {
        const s = poolPositions[link.source];
        const t = modelPositions[link.target];
        if (!s || !t) return null;
        const sourceH = (link.value / (pools.find((p) => p.id === link.source)?.value || 1)) * s.h;
        const targetH = (link.value / (models.find((m) => m.id === link.target)?.value || 1)) * t.h;
        const sOffset = sourceCursor[link.source] ?? 0;
        const tOffset = targetCursor[link.target] ?? 0;
        sourceCursor[link.source] = sOffset + sourceH;
        targetCursor[link.target] = tOffset + targetH;
        const x0 = s.x + nodeWidth;
        const x1 = t.x;
        const y0Top = s.y + sOffset;
        const y0Bot = y0Top + sourceH;
        const y1Top = t.y + tOffset;
        const y1Bot = y1Top + targetH;
        const curvature = 0.55;
        const xi0 = x0 + (x1 - x0) * curvature;
        const xi1 = x1 - (x1 - x0) * curvature;
        const d = `M${x0},${y0Top} C${xi0},${y0Top} ${xi1},${y1Top} ${x1},${y1Top} L${x1},${y1Bot} C${xi1},${y1Bot} ${xi0},${y0Bot} ${x0},${y0Bot} Z`;
        return { d, link };
      })
      .filter(Boolean) as Array<{ d: string; link: SankeyLink }>;

    return { width, height, nodeWidth, poolPositions, modelPositions, paths };
  }, [pools, models, links]);

  if (totalAll === 0) {
    return (
      <section id="flow" className="surface-section scroll-mt-6 px-5 py-5">
        <SectionHeading title={title} description={description} />
        <p className="mt-6 text-sm text-moon-400">{DASH} 过去 24 小时没有可视化的流向。</p>
      </section>
    );
  }

  return (
    <section id="flow" className="surface-section scroll-mt-6 px-5 py-5">
      <SectionHeading title={title} description={description} />
      <div className="mt-5 overflow-x-auto">
        <svg
          viewBox={`0 0 ${layout.width} ${layout.height}`}
          width="100%"
          height={layout.height}
          className="overflow-visible"
        >
          {layout.paths.map((item, idx) => (
            <path
              key={idx}
              d={item.d}
              fill="rgba(134,125,193,0.22)"
              stroke="rgba(134,125,193,0.3)"
              strokeWidth={0.4}
            >
              <title>
                {`${poolLabel(item.link.source, layout.poolPositions)} → ${modelLabel(
                  item.link.target,
                  layout.modelPositions,
                )} · ${item.link.value} 次`}
              </title>
            </path>
          ))}
          {Object.entries(layout.poolPositions).map(([id, node]) => (
            <g key={id}>
              <rect
                x={node.x}
                y={node.y}
                width={layout.nodeWidth}
                height={node.h}
                rx={3}
                fill="rgba(134,125,193,0.75)"
              />
              <text
                x={node.x - 6}
                y={node.y + node.h / 2 + 3}
                textAnchor="end"
                className="fill-moon-700"
                style={{ fontSize: "11px" }}
              >
                {node.label}
              </text>
              <text
                x={node.x - 6}
                y={node.y + node.h / 2 + 15}
                textAnchor="end"
                className="fill-moon-400"
                style={{ fontSize: "10px" }}
              >
                {compact(node.value)}
              </text>
            </g>
          ))}
          {Object.entries(layout.modelPositions).map(([id, node]) => (
            <g key={id}>
              <rect
                x={node.x}
                y={node.y}
                width={layout.nodeWidth}
                height={node.h}
                rx={3}
                fill="rgba(93,159,135,0.78)"
              />
              <text
                x={node.x + layout.nodeWidth + 6}
                y={node.y + node.h / 2 + 3}
                textAnchor="start"
                className="fill-moon-700"
                style={{ fontSize: "11px" }}
              >
                {node.label}
              </text>
              <text
                x={node.x + layout.nodeWidth + 6}
                y={node.y + node.h / 2 + 15}
                textAnchor="start"
                className="fill-moon-400"
                style={{ fontSize: "10px" }}
              >
                {compact(node.value)}
              </text>
            </g>
          ))}
        </svg>
      </div>
      <p className="mt-4 text-xs text-moon-400">
        左侧 Pool → 右侧 Model，带状宽度 ≈ 请求数，悬停查看每条路径详情。
      </p>
    </section>
  );
}

function poolLabel(
  id: string,
  map: Record<string, { label: string; value: number }>,
) {
  return map[id]?.label ?? id;
}
function modelLabel(
  id: string,
  map: Record<string, { label: string; value: number }>,
) {
  return map[id]?.label ?? id;
}

function TopErrors({
  logs,
  onJumpToLog,
}: {
  logs: RequestLog[];
  onJumpToLog: (requestId: string) => void;
}) {
  const rows = useMemo(() => {
    const buckets = new Map<
      string,
      { statusCode: number; message: string; count: number; sample: RequestLog }
    >();
    logs.forEach((log) => {
      if (log.success) return;
      const msg = log.error_message || `HTTP ${log.status_code}`;
      const key = `${log.status_code}:${msg.slice(0, 120)}`;
      const existing = buckets.get(key);
      if (existing) {
        existing.count += 1;
      } else {
        buckets.set(key, {
          statusCode: log.status_code,
          message: msg,
          count: 1,
          sample: log,
        });
      }
    });
    return Array.from(buckets.values())
      .sort((a, b) => b.count - a.count)
      .slice(0, 6);
  }, [logs]);

  return (
    <section className="surface-section flex h-full flex-col px-5 py-5">
      <SectionHeading
        title="Top Errors"
        description="最近 24 小时出现最多的几类失败。"
      />
      <div className="mt-5 flex-1">
        {rows.length === 0 ? (
          <p className="text-sm text-moon-400">{DASH} 过去 24 小时没有失败。</p>
        ) : (
          <ul className="space-y-3">
            {rows.map((row) => (
              <li
                key={`${row.statusCode}:${row.message}`}
                className="rounded-[1.1rem] border border-moon-200/45 bg-white/70 px-3 py-2.5"
              >
                <div className="flex items-center justify-between gap-3">
                  <span
                    className={cn(
                      "inline-flex rounded-full px-2 py-0.5 text-xs",
                      row.statusCode === 0 || row.statusCode >= 500
                        ? "bg-status-red/12 text-status-red"
                        : "bg-status-yellow/12 text-status-yellow",
                    )}
                  >
                    {row.statusCode === 0 ? "网络故障" : `HTTP ${row.statusCode}`}
                  </span>
                  <span className="text-sm font-semibold text-moon-700">
                    × {compact(row.count)}
                  </span>
                </div>
                <p className="mt-1.5 line-clamp-2 text-sm text-moon-600">
                  {row.message}
                </p>
                <a
                  href={`#logs-${row.sample.request_id}`}
                  className="mt-1.5 inline-flex items-center gap-1 text-[11px] text-lunar-600 hover:text-lunar-700"
                  onClick={(event) => {
                    event.preventDefault();
                    onJumpToLog(row.sample.request_id);
                  }}
                >
                  <Link2 className="size-3" />
                  跳到一次样本
                </a>
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}

function percentile(sorted: number[], p: number): number {
  if (sorted.length === 0) return 0;
  const idx = Math.min(
    sorted.length - 1,
    Math.max(0, Math.floor((sorted.length - 1) * p)),
  );
  return sorted[idx];
}

function LatencyPercentiles({ logs }: { logs: RequestLog[] }) {
  const { p50, p95, p99, count } = useMemo(() => {
    const values = logs
      .filter((l) => l.success && l.latency_ms > 0)
      .map((l) => l.latency_ms)
      .sort((a, b) => a - b);
    return {
      count: values.length,
      p50: percentile(values, 0.5),
      p95: percentile(values, 0.95),
      p99: percentile(values, 0.99),
    };
  }, [logs]);

  return (
    <section className="surface-section flex h-full flex-col px-5 py-5">
      <SectionHeading
        title="延迟分位线"
        description="看典型和长尾，光看均值会掩盖个别慢请求。"
      />
      <div className="mt-5 grid grid-cols-3 gap-3">
        <LatencyCell
          label="p50"
          caption="一半请求快于这"
          value={count > 0 ? latency(p50) : DASH}
        />
        <LatencyCell
          label="p95"
          caption="日常尾部"
          value={count > 0 ? latency(p95) : DASH}
        />
        <LatencyCell
          label="p99"
          caption="最慢的 1%"
          value={count > 0 ? latency(p99) : DASH}
          emphasized
        />
      </div>
      <p className="mt-4 text-xs text-moon-400">
        {count > 0 ? (
          <>基于最近 24h 的 {compact(count)} 条成功请求</>
        ) : (
          <>过去 24 小时无成功请求</>
        )}
      </p>
    </section>
  );
}

function LatencyCell({
  label,
  caption,
  value,
  emphasized,
}: {
  label: string;
  caption: string;
  value: string;
  emphasized?: boolean;
}) {
  return (
    <div
      className={cn(
        "rounded-[1.1rem] border border-moon-200/50 bg-white/70 px-3 py-3",
        emphasized && "bg-lunar-50/70",
      )}
    >
      <p className="text-[11px] uppercase tracking-[0.18em] text-moon-400">
        {label}
      </p>
      <p className="mt-1 text-[1.2rem] font-semibold tracking-[-0.03em] text-moon-800">
        {value}
      </p>
      <p className="mt-0.5 text-[11px] text-moon-400">{caption}</p>
    </div>
  );
}

export default function ActivityPage() {
  const [pools, setPools] = useState<Pool[]>([]);
  const [usage, setUsage] = useState<UsageBundle | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [lastUpdated, setLastUpdated] = useState<string | null>(null);
  const [filterPool, setFilterPool] = useState("all");
  const [filterStatus, setFilterStatus] = useState("all");
  const [filterModel, setFilterModel] = useState("all");
  const [searchRequestId, setSearchRequestId] = useState("");
  const [expandedRowId, setExpandedRowId] = useState<number | null>(null);
  const [highlightedRequestId, setHighlightedRequestId] = useState<string | null>(null);
  const [currentPage, setCurrentPage] = useState(1);
  const loadRequestIdRef = useRef(0);
  const highlightTimerRef = useRef<number | null>(null);
  const autoRefreshTimerRef = useRef<number | null>(null);

  const load = useCallback(() => {
    const requestId = ++loadRequestIdRef.current;
    setLoading(true);
    setError(null);
    Promise.all([api.get<Pool[]>("/pools"), loadUsage30d()])
      .then(([poolData, usageData]) => {
        if (requestId !== loadRequestIdRef.current) return;
        setPools(poolData ?? []);
        setUsage(usageData);
        setLastUpdated(new Date().toISOString());
      })
      .catch((err) => {
        if (requestId !== loadRequestIdRef.current) return;
        setError(err instanceof Error ? err.message : "Activity 加载失败");
      })
      .finally(() => {
        if (requestId === loadRequestIdRef.current) setLoading(false);
      });
  }, []);

  // Single auto-refresh timer. Resetting it on manual refresh avoids the race
  // where the user taps "刷新" at T=29s and the 30s tick fires at T=30s with
  // a duplicate 3-way fetch (3 endpoints × up to 12 usage pages each).
  const resetAutoRefresh = useCallback(() => {
    if (autoRefreshTimerRef.current != null) {
      window.clearInterval(autoRefreshTimerRef.current);
    }
    autoRefreshTimerRef.current = window.setInterval(() => {
      load();
    }, AUTO_REFRESH_MS);
  }, [load]);

  useEffect(() => {
    load();
    resetAutoRefresh();
    return () => {
      if (autoRefreshTimerRef.current != null) {
        window.clearInterval(autoRefreshTimerRef.current);
        autoRefreshTimerRef.current = null;
      }
    };
  }, [load, resetAutoRefresh]);

  const handleManualRefresh = useCallback(() => {
    load();
    resetAutoRefresh();
  }, [load, resetAutoRefresh]);

  const poolMap = useMemo(
    () => new Map(pools.map((pool) => [pool.id, pool.label])),
    [pools],
  );

  const logs30d = usage?.logs ?? [];
  const logs24h = useMemo(() => logs30d.filter((l) => within(l, 24)), [logs30d]);
  const logs7d = useMemo(() => logs30d.filter((l) => within(l, 24 * 7)), [logs30d]);

  const modelOptions = useMemo(() => {
    const values = new Set<string>();
    logs30d.forEach((log) => {
      const model = log.model_actual || log.model_requested;
      if (model) values.add(model);
    });
    return Array.from(values).sort();
  }, [logs30d]);

  const filteredLogs = useMemo(() => {
    const trimmed = searchRequestId.trim().toLowerCase();
    return logs30d.filter((log) => {
      if (filterPool !== "all" && String(log.pool_id) !== filterPool) return false;
      if (filterStatus === "success" && !log.success) return false;
      if (filterStatus === "error" && log.success) return false;
      if (
        filterModel !== "all" &&
        log.model_actual !== filterModel &&
        log.model_requested !== filterModel
      )
        return false;
      if (trimmed && !log.request_id.toLowerCase().includes(trimmed)) return false;
      return true;
    });
  }, [filterModel, filterPool, filterStatus, logs30d, searchRequestId]);

  const buckets24h = useMemo(() => buildBuckets(logs24h, "24h"), [logs24h]);
  const buckets7d = useMemo(() => buildBuckets(logs7d, "7d"), [logs7d]);
  const buckets30d = useMemo(() => buildBuckets(logs30d, "30d"), [logs30d]);

  const totalPages = Math.max(
    1,
    Math.ceil(filteredLogs.length / LOGS_PER_PAGE),
  );
  // Clamp at read time so an auto-refresh that shrinks filteredLogs can't
  // leave the table rendering page 5 of 3 (empty view, confusing).
  const clampedPage = Math.min(Math.max(1, currentPage), totalPages);
  const pagedLogs = useMemo(
    () =>
      filteredLogs.slice(
        (clampedPage - 1) * LOGS_PER_PAGE,
        clampedPage * LOGS_PER_PAGE,
      ),
    [filteredLogs, clampedPage],
  );

  // Any filter change or request-id deeplink lands the user on page 1.
  // Without this, changing the pool filter while on page 3 keeps you on
  // page 3 of the narrower result set — often empty.
  useEffect(() => {
    setCurrentPage(1);
  }, [filterPool, filterStatus, filterModel, searchRequestId]);

  // Scroll + highlight a single log row. Used by #logs-<rid> deep-link and
  // the "跳到一次样本" action in TopErrors. Clears filters so a filtered
  // view can't hide the target and strand the user.
  const jumpToLog = useCallback(
    (rid: string) => {
      const target = logs30d.find((l) => l.request_id === rid);
      if (!target) {
        if (usage?.truncated) {
          toast("这条请求不在当前聚合窗口里，可能已经被截断。", "error");
        } else {
          toast("没有找到这条 request_id。", "error");
        }
        return;
      }
      setFilterPool("all");
      setFilterStatus("all");
      setFilterModel("all");
      setSearchRequestId(rid);
      setExpandedRowId(target.id);
      setHighlightedRequestId(rid);
      window.history.replaceState(null, "", `#logs-${rid}`);
      if (highlightTimerRef.current) {
        window.clearTimeout(highlightTimerRef.current);
      }
      highlightTimerRef.current = window.setTimeout(() => {
        setHighlightedRequestId(null);
        highlightTimerRef.current = null;
      }, 2600);
      let attempts = 0;
      const tryScroll = () => {
        const el = document.getElementById(`logs-${rid}`);
        if (el) {
          el.scrollIntoView({ behavior: "smooth", block: "center" });
          return;
        }
        if (attempts < 20) {
          attempts += 1;
          window.setTimeout(tryScroll, 80);
        }
      };
      tryScroll();
    },
    [logs30d, usage],
  );

  // Hash-based deep-link: #logs-<request_id> scrolls to and highlights the row.
  const hashHandledRef = useRef<string | null>(null);
  useEffect(() => {
    if (loading || !usage) return;
    const match = window.location.hash.match(/^#logs-([A-Za-z0-9_-]+)$/);
    if (!match) return;
    const rid = match[1];
    if (hashHandledRef.current === rid) return;
    hashHandledRef.current = rid;
    jumpToLog(rid);
    return () => {
      if (highlightTimerRef.current) window.clearTimeout(highlightTimerRef.current);
    };
  }, [loading, usage, jumpToLog]);

  async function copyPermalink(log: RequestLog) {
    const hash = `#logs-${log.request_id}`;
    const url = `${window.location.origin}${window.location.pathname}${hash}`;
    try {
      await navigator.clipboard.writeText(url);
      window.history.replaceState(null, "", hash);
      toast("链接已复制");
    } catch {
      toast("复制链接失败", "error");
    }
  }

  if (loading && !usage) {
    return (
      <div className="space-y-8">
        <Skeleton className="h-28 rounded-[2rem]" />
        <div className="grid gap-6 xl:grid-cols-3">
          <Skeleton className="h-72 rounded-[1.8rem]" />
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
      <SideTOC sections={TOC_SECTIONS} ready={Boolean(usage)} />
      <PageHeader
        title="Activity"
        description="查看最近请求与系统运行状态。"
        actions={
          <Button
            variant="outline"
            onClick={handleManualRefresh}
            disabled={loading}
          >
            <RefreshCw className={cn("size-4", loading && "animate-spin")} />
            刷新
          </Button>
        }
        meta={
          lastUpdated ? <span>最后更新 {relativeTime(lastUpdated)}</span> : null
        }
      />

      <DailyDigest logs30d={logs30d} poolMap={poolMap} />

      <section id="trends" className="scroll-mt-6">
        <div className="grid gap-6 xl:grid-cols-3">
          <MoonDial buckets={buckets24h} />
          <StackedTrendBars
            title="7 天趋势"
            description="最近一周的每日请求量，绿色成功 / 红色失败。"
            buckets={buckets7d}
          />
          <StackedTrendBars
            title="30 天趋势"
            description="月度节奏，用来看有没有趋势性变化。"
            buckets={buckets30d}
          />
        </div>
      </section>

      <SankeyDiagram
        title="Pool → Model 流向"
        description="过去 24 小时，请求是怎么分散到各 Pool、最后打到了哪些模型。"
        logs={logs24h}
        poolMap={poolMap}
      />

      <section id="health" className="scroll-mt-6">
        <div className="grid gap-6 xl:grid-cols-2">
          <TopErrors logs={logs24h} onJumpToLog={jumpToLog} />
          <LatencyPercentiles logs={logs24h} />
        </div>
      </section>

      <section
        id="logs"
        className="surface-section scroll-mt-6 px-5 py-5"
      >
        <SectionHeading
          title="Request Logs"
          description="顺着一条请求往下查问题。"
        />

        <div className="mt-5 flex flex-wrap items-center gap-3">
          <Select
            value={filterPool}
            onValueChange={(value) => setFilterPool(value ?? "all")}
          >
            <SelectTrigger className="h-9 rounded-full border-moon-200/70 bg-white/82 px-3 text-sm text-moon-600">
              <SelectValue placeholder="全部 Pool" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部 Pool</SelectItem>
              {pools.map((pool) => (
                <SelectItem key={pool.id} value={String(pool.id)}>
                  {pool.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select
            value={filterStatus}
            onValueChange={(value) => setFilterStatus(value ?? "all")}
          >
            <SelectTrigger className="h-9 rounded-full border-moon-200/70 bg-white/82 px-3 text-sm text-moon-600">
              <SelectValue placeholder="全部状态" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部状态</SelectItem>
              <SelectItem value="success">成功</SelectItem>
              <SelectItem value="error">失败</SelectItem>
            </SelectContent>
          </Select>
          <Select
            value={filterModel}
            onValueChange={(value) => setFilterModel(value ?? "all")}
          >
            <SelectTrigger className="h-9 rounded-full border-moon-200/70 bg-white/82 px-3 text-sm text-moon-600">
              <SelectValue placeholder="全部模型" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部模型</SelectItem>
              {modelOptions.map((model) => (
                <SelectItem key={model} value={model}>
                  {model}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {/* Active-request chip — shows only when the list is narrowed down
              to a single request via TopErrors "跳到一次样本" or a
              #logs-<rid> hash link. Gives the user a way to back out of
              that single-row view without needing to wiggle another filter. */}
          {searchRequestId ? (
            <button
              type="button"
              onClick={() => {
                setSearchRequestId("");
                window.history.replaceState(
                  null,
                  "",
                  window.location.pathname + window.location.search,
                );
              }}
              className="inline-flex items-center gap-1.5 rounded-full bg-lunar-100/80 px-3 py-1.5 text-xs text-lunar-700 transition-colors hover:bg-lunar-200/80"
              title="清除单条请求过滤"
            >
              <span>
                单条请求{" "}
                <span className="font-mono">
                  {searchRequestId.slice(0, 8)}
                  {searchRequestId.length > 8 ? "…" : ""}
                </span>
              </span>
              <X className="size-3" />
            </button>
          ) : null}
        </div>

        <div className="mt-4 flex flex-wrap items-center justify-between gap-3 text-sm text-moon-400">
          <p>
            匹配到{" "}
            {filteredLogs.length > 0 ? compact(filteredLogs.length) : DASH} 条请求
            {filteredLogs.length > LOGS_PER_PAGE
              ? `，每页 ${LOGS_PER_PAGE} 条`
              : ""}
            。
          </p>
          {usage?.truncated ? (
            <p>
              为保证加载稳定，当前聚合基于最近 {compact(usage.logs.length)}{" "}
              条请求。
            </p>
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
              {pagedLogs.length ? (
                pagedLogs.map((item) => {
                  const expanded = expandedRowId === item.id;
                  const totalTokens = getRequestTokens(item);
                  const highlighted = highlightedRequestId === item.request_id;
                  return (
                    <Fragment key={item.id}>
                      <tr
                        id={`logs-${item.request_id}`}
                        tabIndex={0}
                        role="button"
                        aria-expanded={expanded}
                        className={cn(
                          "cursor-pointer transition-colors hover:bg-white/55 focus:outline-none focus-visible:bg-white/70 focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-lunar-400",
                          expanded ? "bg-white/82" : "",
                          highlighted &&
                            "bg-lunar-100/55 ring-2 ring-inset ring-lunar-300/70",
                        )}
                        onClick={() =>
                          setExpandedRowId((current) =>
                            current === item.id ? null : item.id,
                          )
                        }
                        onKeyDown={(event) => {
                          if (event.key === "Enter" || event.key === " ") {
                            event.preventDefault();
                            setExpandedRowId((current) =>
                              current === item.id ? null : item.id,
                            );
                          }
                        }}
                      >
                        <td className="px-4 py-3 text-moon-400">
                          {shortDate(item.created_at)}
                        </td>
                        <td className="px-4 py-3">
                          <div className="space-y-1">
                            <p className="text-moon-700">{getRequestSummary(item)}</p>
                            <p className="text-xs text-moon-400">
                              {item.source_kind || "gateway"}
                              {item.attempt_count > 1 ? (
                                <span className="ml-2 rounded-full bg-lunar-100/80 px-2 py-0.5 text-[10px] text-lunar-700">
                                  重试 ×{item.attempt_count}
                                </span>
                              ) : null}
                            </p>
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
                        <td className="px-4 py-3 text-moon-500">
                          {item.latency_ms > 0 ? latency(item.latency_ms) : DASH}
                        </td>
                        <td className="px-4 py-3 text-moon-500">
                          {totalTokens > 0 ? compact(totalTokens) : DASH}
                        </td>
                        <td className="px-4 py-3 text-moon-400 font-mono text-[12px]">
                          {item.request_id}
                        </td>
                        <td className="px-4 py-3 text-moon-300">
                          <ChevronDown
                            className={cn(
                              "size-4 transition-transform",
                              expanded ? "rotate-180" : "",
                            )}
                          />
                        </td>
                      </tr>
                      {expanded ? (
                        <tr className="bg-white/80">
                          <td colSpan={9} className="px-4 py-4">
                            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
                              <div className="space-y-1">
                                <p className="text-[11px] uppercase tracking-[0.16em] text-moon-400">
                                  Request ID
                                </p>
                                <p className="break-all text-sm text-moon-700">
                                  {item.request_id}
                                </p>
                              </div>
                              <div className="space-y-1">
                                <p className="text-[11px] uppercase tracking-[0.16em] text-moon-400">
                                  Requested Model
                                </p>
                                <p className="text-sm text-moon-700">
                                  {item.model_requested || "--"}
                                </p>
                              </div>
                              <div className="space-y-1">
                                <p className="text-[11px] uppercase tracking-[0.16em] text-moon-400">
                                  Actual Model
                                </p>
                                <p className="text-sm text-moon-700">
                                  {item.model_actual || "--"}
                                </p>
                              </div>
                              <div className="space-y-1">
                                <p className="text-[11px] uppercase tracking-[0.16em] text-moon-400">
                                  Pool
                                </p>
                                <p className="text-sm text-moon-700">
                                  {poolMap.get(item.pool_id) ??
                                    `Pool #${item.pool_id}`}
                                </p>
                              </div>
                              <div className="space-y-1">
                                <p className="text-[11px] uppercase tracking-[0.16em] text-moon-400">
                                  Account
                                </p>
                                <p className="text-sm text-moon-700">
                                  {item.account_label || `#${item.account_id}`}
                                </p>
                              </div>
                              <div className="space-y-1">
                                <p className="text-[11px] uppercase tracking-[0.16em] text-moon-400">
                                  Status Code
                                </p>
                                <p className="text-sm text-moon-700">
                                  {item.status_code}
                                </p>
                              </div>
                              <div className="space-y-1">
                                <p className="text-[11px] uppercase tracking-[0.16em] text-moon-400">
                                  Latency
                                </p>
                                <p className="text-sm text-moon-700">
                                  {latency(item.latency_ms)}
                                </p>
                              </div>
                              <div className="space-y-1">
                                <p className="text-[11px] uppercase tracking-[0.16em] text-moon-400">
                                  Tokens
                                </p>
                                <p className="text-sm text-moon-700">
                                  输入 {compact(item.input_tokens ?? 0)} / 输出{" "}
                                  {compact(item.output_tokens ?? 0)}
                                </p>
                              </div>
                              <div className="space-y-1">
                                <p className="text-[11px] uppercase tracking-[0.16em] text-moon-400">
                                  Attempts
                                </p>
                                <p className="text-sm text-moon-700">
                                  {item.attempt_count === 0
                                    ? "路由拒绝（未发起上游请求）"
                                    : item.attempt_count > 1
                                      ? item.success
                                        ? `#${item.attempt_count}（重试救回）`
                                        : `#${item.attempt_count}（重试仍失败）`
                                      : `#${item.attempt_count}`}
                                </p>
                              </div>
                            </div>
                            {item.error_message ? (
                              <div className="mt-4 rounded-[1.1rem] border border-status-red/15 bg-red-50/75 px-4 py-3">
                                <p className="text-[11px] uppercase tracking-[0.16em] text-status-red/75">
                                  Error Message
                                </p>
                                <p className="mt-1 text-sm text-status-red">
                                  {item.error_message}
                                </p>
                              </div>
                            ) : null}
                            <div className="mt-3 flex justify-end">
                              <Button
                                variant="outline"
                                size="sm"
                                className="rounded-full"
                                onClick={(event) => {
                                  event.stopPropagation();
                                  void copyPermalink(item);
                                }}
                              >
                                <Link2 className="size-3.5" />
                                复制链接
                              </Button>
                            </div>
                          </td>
                        </tr>
                      ) : null}
                    </Fragment>
                  );
                })
              ) : (
                <tr>
                  <td
                    colSpan={9}
                    className="px-4 py-12 text-center text-sm text-moon-400"
                  >
                    当前筛选下没有可显示的请求。
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>

        {totalPages > 1 ? (
          <div className="mt-4 flex items-center justify-between gap-3 text-sm text-moon-500">
            <span className="text-xs text-moon-400">
              第 {clampedPage} / {totalPages} 页
            </span>
            <div className="flex gap-2">
              <Button
                variant="outline"
                size="sm"
                className="rounded-full"
                disabled={clampedPage <= 1}
                onClick={() =>
                  // Base off clampedPage, not the raw currentPage. When
                  // auto-refresh shrinks totalPages while the user is on a
                  // late page, currentPage can sit above clampedPage; using
                  // the raw value would make the first few Prev clicks
                  // visually no-op until currentPage catches up.
                  setCurrentPage(Math.max(1, clampedPage - 1))
                }
              >
                <ChevronLeft className="size-4" />
                上一页
              </Button>
              <Button
                variant="outline"
                size="sm"
                className="rounded-full"
                disabled={clampedPage >= totalPages}
                onClick={() =>
                  setCurrentPage(Math.min(totalPages, clampedPage + 1))
                }
              >
                下一页
                <ChevronRight className="size-4" />
              </Button>
            </div>
          </div>
        ) : null}
      </section>
    </div>
  );
}
