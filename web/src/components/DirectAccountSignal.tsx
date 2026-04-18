import { Activity, ShieldCheck } from "lucide-react";
import { compact } from "@/lib/fmt";
import { cn } from "@/lib/utils";

const DOT_COUNT = 10;

// Log-scale thresholds for the request-volume dot strip: dot N lights up once
// requests cross the N-th threshold. Anchors at 1/10/100/1K/10K/100K — between
// those, two intermediate stops give smoother growth so a busy account drifts
// visibly before topping out, instead of jumping two dots per decade.
const REQUEST_DOT_THRESHOLDS = [
  1, 10, 50, 100, 500, 1_000, 5_000, 10_000, 50_000, 100_000,
];

// Two-row signal strip shared by direct accounts and non-Codex CPA accounts
// (Claude etc.). Row 1 shows request volume on a log-scale dot strip; row 2
// shows 24h success rate as a color-coded bar. "—" placeholders keep the
// layout stable when a card has no traffic yet.
export default function DirectAccountSignal({
  requests,
  successRate,
}: {
  requests: number;
  successRate: number | null;
}) {
  const filledDots = requestsToFilledDots(requests);
  const success = describeSuccess(requests, successRate);

  return (
    <div className="space-y-1">
      <div
        className="flex items-center gap-2 text-[11px] tabular-nums"
        title={`24h 请求 ${requests} 次`}
      >
        <span className="flex w-5 shrink-0 justify-center text-moon-500">
          <Activity className="size-3" aria-hidden />
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
        <span
          className={cn(
            "w-10 shrink-0 text-right",
            requests > 0 ? "text-moon-600" : "text-moon-400",
          )}
        >
          {requests > 0 ? compact(requests) : "—"}
        </span>
      </div>

      <div
        className="flex items-center gap-2 text-[11px] tabular-nums"
        title={success.title}
      >
        <span className="flex w-5 shrink-0 justify-center text-moon-500">
          <ShieldCheck className="size-3" aria-hidden />
        </span>
        <div
          className={cn(
            "relative h-1.5 flex-1 overflow-hidden rounded-full",
            success.trackClass,
          )}
        >
          <div
            className={cn(
              "absolute inset-y-0 left-0 rounded-full transition-[width]",
              success.fillClass,
            )}
            style={{ width: `${success.percent}%` }}
          />
        </div>
        <span className={cn("w-10 shrink-0 text-right", success.textClass)}>
          {success.label}
        </span>
      </div>
    </div>
  );
}

function requestsToFilledDots(requests: number): number {
  if (requests <= 0) return 0;
  let filled = 0;
  for (const threshold of REQUEST_DOT_THRESHOLDS) {
    if (requests >= threshold) filled += 1;
    else break;
  }
  return Math.min(filled, DOT_COUNT);
}

interface SuccessSignal {
  percent: number;
  label: string;
  title: string;
  trackClass: string;
  fillClass: string;
  textClass: string;
}

// Tone thresholds intentionally generous on the top end: 99% and above reads
// as "healthy", 95-99 as "fine", 80-95 as "warn", below 80 as "broken". The
// bar fill always reflects the raw percentage so the eye can still rank two
// degraded accounts against each other even though they share a color.
function describeSuccess(requests: number, rate: number | null): SuccessSignal {
  if (requests <= 0 || rate == null) {
    return {
      percent: 0,
      label: "—",
      title: "24h 无请求样本",
      trackClass: "bg-moon-200/70",
      fillClass: "bg-moon-300/60",
      textClass: "text-moon-400",
    };
  }
  const percent = Math.max(0, Math.min(1, rate)) * 100;
  const label = formatSuccess(percent);
  const title = `24h 成功率 ${label}（${requests} 次请求）`;
  if (percent >= 99) {
    return {
      percent,
      label,
      title,
      trackClass: "bg-moon-200/70",
      fillClass: "bg-status-green",
      textClass: "text-moon-500",
    };
  }
  if (percent >= 95) {
    return {
      percent,
      label,
      title,
      trackClass: "bg-moon-200/70",
      fillClass: "bg-status-green/70",
      textClass: "text-moon-500",
    };
  }
  if (percent >= 80) {
    return {
      percent,
      label,
      title,
      trackClass: "bg-status-yellow/20",
      fillClass: "bg-status-yellow",
      textClass: "text-status-yellow",
    };
  }
  return {
    percent,
    label,
    title,
    trackClass: "bg-status-red/15",
    fillClass: "bg-status-red",
    textClass: "text-status-red",
  };
}

function formatSuccess(percent: number): string {
  if (percent >= 99.95) return "100%";
  if (percent >= 10) return `${percent.toFixed(0)}%`;
  return `${percent.toFixed(1).replace(/\.0$/, "")}%`;
}
