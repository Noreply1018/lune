import { useMemo } from "react";
import { Boxes, Radar } from "lucide-react";
import type { Account } from "@/lib/types";
import { parseSqliteUTC } from "@/lib/codexQuota";
import { relativeTime } from "@/lib/fmt";
import { cn } from "@/lib/utils";

const DOT_COUNT = 10;

// Two-row signal strip for direct (non-Codex) account cards. Both rows share a
// `icon + flex-1 bar/dots + right label` layout so the component height is
// stable regardless of text length — that's what keeps direct cards aligned
// with Codex cards on the same row. Text labels are deliberately replaced with
// small icons: spelling "模型" / "自检" caused the narrow card to wrap and
// gave direct cards a taller shape than CPA cards.
export default function DirectAccountSignal({ account }: { account: Account }) {
  const modelCount = account.models?.length ?? 0;
  const filledDots = Math.min(modelCount, DOT_COUNT);

  const probe = useMemo(() => describeProbe(account), [
    account.last_probe_status,
    account.last_probe_at,
    account.last_probe_error,
    account.last_checked_at,
    account.last_error,
  ]);

  return (
    <div className="space-y-1">
      <div className="flex items-center gap-2 text-[11px] tabular-nums" title={`已发现 ${modelCount} 个模型`}>
        <span className="flex w-5 shrink-0 justify-center text-moon-500">
          <Boxes className="size-3" aria-hidden />
        </span>
        <div className="flex flex-1 items-center gap-[3px]">
          {Array.from({ length: DOT_COUNT }, (_, i) => (
            <span
              key={i}
              className={cn(
                "size-1.5 rounded-full",
                i < filledDots ? "bg-lunar-500/80" : "bg-moon-200/80",
              )}
            />
          ))}
        </div>
        <span className={cn("w-9 shrink-0 text-right", modelCount > 0 ? "text-moon-600" : "text-moon-400")}>
          {modelCount}
        </span>
      </div>

      <div
        className="flex items-center gap-2 text-[11px] tabular-nums"
        title={probe.title}
      >
        <span className="flex w-5 shrink-0 justify-center text-moon-500">
          <Radar className="size-3" aria-hidden />
        </span>
        <div
          className={cn(
            "relative h-1.5 flex-1 overflow-hidden rounded-full",
            probe.trackClass,
          )}
        >
          <div
            className={cn(
              "absolute inset-y-0 left-0 rounded-full transition-[width]",
              probe.fillClass,
            )}
            style={{ width: `${probe.percent}%` }}
          />
        </div>
        <span className={cn("w-9 shrink-0 text-right", probe.textClass)}>
          {probe.label}
        </span>
      </div>
    </div>
  );
}

interface ProbeSignal {
  percent: number;
  label: string;
  title: string;
  trackClass: string;
  fillClass: string;
  textClass: string;
}

// describeProbe derives the row-2 signal from (in priority order):
//   1. last_probe_status — user-triggered self-check on this account
//   2. last_checked_at + last_error — background health-loop outcome
// The user's self-check verdict wins so the bar reflects "what I just tested"
// instead of a stale models endpoint hit.
function describeProbe(account: Account): ProbeSignal {
  const probeStatus = account.last_probe_status;
  if (probeStatus === "healthy" || probeStatus === "degraded" || probeStatus === "error") {
    return probeFromStatus(probeStatus, account.last_probe_at ?? null, account.last_probe_error ?? "");
  }
  return probeFromHealthLoop(account.last_checked_at, account.last_error);
}

function probeFromStatus(
  status: "healthy" | "degraded" | "error",
  atRaw: string | null,
  err: string,
): ProbeSignal {
  const ms = parseSqliteUTC(atRaw);
  const rel = ms != null ? relativeTime(new Date(ms).toISOString()) || "刚刚" : "刚刚";
  if (status === "healthy") {
    return {
      percent: 100,
      label: rel,
      title: `自检通过 · ${rel}`,
      trackClass: "bg-moon-200/70",
      fillClass: "bg-status-green",
      textClass: "text-moon-500",
    };
  }
  if (status === "degraded") {
    return {
      percent: 55,
      label: rel,
      title: err ? `自检降级 · ${rel} · ${err}` : `自检降级 · ${rel}`,
      trackClass: "bg-status-yellow/20",
      fillClass: "bg-status-yellow",
      textClass: "text-status-yellow",
    };
  }
  return {
    percent: 0,
    label: rel,
    title: err ? `自检失败 · ${rel} · ${err}` : `自检失败 · ${rel}`,
    trackClass: "bg-status-red/15",
    fillClass: "bg-status-red",
    textClass: "text-status-red",
  };
}

// Freshness decays from full (<1 min) to empty (>10 min) linearly. This mirrors
// the default 60 s health-check cadence so a healthy account sits near full and
// a neglected one drains visibly before users need to dig for exact timestamps.
const FRESH_FULL_SECONDS = 60;
const FRESH_EMPTY_SECONDS = 10 * 60;

function probeFromHealthLoop(lastCheckedAt: string | null, lastError: string | null): ProbeSignal {
  const hasError = Boolean(lastError);
  const ms = parseSqliteUTC(lastCheckedAt);
  if (ms == null) {
    return {
      percent: 0,
      label: "从未",
      title: "尚未进行过健康检查",
      trackClass: "bg-moon-200/70",
      fillClass: hasError ? "bg-status-red" : "bg-lunar-500",
      textClass: hasError ? "text-status-red" : "text-moon-500",
    };
  }
  const ageSeconds = Math.max(0, (Date.now() - ms) / 1000);
  const percent = hasError ? 0 : freshnessFromAge(ageSeconds);
  const label = relativeTime(new Date(ms).toISOString()) || "刚刚";
  const title = hasError && lastError
    ? `上次探活 ${label} · 错误：${lastError}`
    : `上次探活 ${label}`;
  return {
    percent,
    label,
    title,
    trackClass: hasError ? "bg-status-red/15" : "bg-moon-200/70",
    fillClass: hasError ? "bg-status-red" : "bg-lunar-500",
    textClass: hasError ? "text-status-red" : "text-moon-500",
  };
}

function freshnessFromAge(ageSeconds: number): number {
  if (ageSeconds <= FRESH_FULL_SECONDS) return 100;
  if (ageSeconds >= FRESH_EMPTY_SECONDS) return 0;
  const span = FRESH_EMPTY_SECONDS - FRESH_FULL_SECONDS;
  const drained = ageSeconds - FRESH_FULL_SECONDS;
  return Math.round(100 - (drained / span) * 100);
}
