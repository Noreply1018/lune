import { useMemo } from "react";
import type { Account } from "@/lib/types";
import { parseSqliteUTC } from "@/lib/codexQuota";
import { relativeTime } from "@/lib/fmt";
import { cn } from "@/lib/utils";

const DOT_COUNT = 10;

// Fill `DOT_COUNT` dots proportional to min(total, DOT_COUNT); the right-side
// number always carries the true count so the cluster is a visual cue, not a
// precise value. 0 models still renders an empty rail so the row keeps its
// height and the two-card layout stays aligned.
export default function DirectAccountSignal({ account }: { account: Account }) {
  const modelCount = account.models?.length ?? 0;
  const filledDots = Math.min(modelCount, DOT_COUNT);

  const probe = useMemo(
    () => describeProbe(account.last_checked_at, account.last_error),
    [account.last_checked_at, account.last_error],
  );

  return (
    <div className="space-y-1">
      <div className="flex items-center gap-2 text-[11px] tabular-nums">
        <span className="w-5 shrink-0 text-moon-500">模型</span>
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
        <span className="w-5 shrink-0 text-moon-500">探活</span>
        <div className="relative h-1.5 flex-1 overflow-hidden rounded-full bg-moon-200/70">
          <div
            className={cn(
              "absolute inset-y-0 left-0 rounded-full transition-[width]",
              probe.hasError ? "bg-status-red" : "bg-lunar-500",
            )}
            style={{ width: `${probe.freshnessPercent}%` }}
          />
        </div>
        <span
          className={cn(
            "flex w-9 shrink-0 items-center justify-end gap-1 text-right",
            probe.hasError ? "text-status-red" : "text-moon-500",
          )}
        >
          {probe.hasError ? <span className="size-1.5 rounded-full bg-status-red" /> : null}
          <span className="truncate">{probe.label}</span>
        </span>
      </div>
    </div>
  );
}

interface ProbeSignal {
  freshnessPercent: number;
  label: string;
  title: string;
  hasError: boolean;
}

// Freshness decays from full (<1 min) to empty (>10 min) linearly. This mirrors
// the default 60 s health-check cadence so a healthy account sits near full and
// a neglected one drains visibly before users need to dig for exact timestamps.
const FRESH_FULL_SECONDS = 60;
const FRESH_EMPTY_SECONDS = 10 * 60;

function describeProbe(lastCheckedAt: string | null, lastError: string | null): ProbeSignal {
  const hasError = Boolean(lastError);
  const ms = parseSqliteUTC(lastCheckedAt);
  if (ms == null) {
    return {
      freshnessPercent: 0,
      label: "从未",
      title: "尚未进行过健康检查",
      hasError,
    };
  }
  const ageSeconds = Math.max(0, (Date.now() - ms) / 1000);
  // When the last probe failed we drain the bar regardless of age — a fresh
  // red stripe would otherwise read like a healthy account because the bar is
  // still full. Users should see "the probe is not healthy right now" first,
  // and the relative time + tooltip still carry the freshness detail.
  const freshnessPercent = hasError ? 0 : freshnessFromAge(ageSeconds);
  const label = relativeTime(new Date(ms).toISOString()) || "刚刚";
  const title = hasError && lastError
    ? `上次探活 ${label} · 错误：${lastError}`
    : `上次探活 ${label}`;
  return { freshnessPercent, label, title, hasError };
}

function freshnessFromAge(ageSeconds: number): number {
  if (ageSeconds <= FRESH_FULL_SECONDS) return 100;
  if (ageSeconds >= FRESH_EMPTY_SECONDS) return 0;
  const span = FRESH_EMPTY_SECONDS - FRESH_FULL_SECONDS;
  const drained = ageSeconds - FRESH_FULL_SECONDS;
  return Math.round(100 - (drained / span) * 100);
}
