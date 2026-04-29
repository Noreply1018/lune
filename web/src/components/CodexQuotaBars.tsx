import { AlertTriangle, Clock3 } from "lucide-react";
import {
  formatResetAt,
  formatResetIn,
  isQuotaStale,
  parseSqliteUTC,
  windowTone,
  type CodexQuota,
  type QuotaWindow,
  type WindowTone,
} from "@/lib/codexQuota";
import { relativeTime } from "@/lib/fmt";
import { cn } from "@/lib/utils";

// Bars encode *remaining* quota: a full green bar means lots of headroom,
// shrinking and reddening as the window approaches its ceiling. The fill
// stays as the semantic signal (green → yellow → red); the track is a muted
// rail so a nearly-empty bar is still visible against the card background.
const toneFill: Record<WindowTone, string> = {
  ok: "bg-status-green",
  warning: "bg-status-yellow",
  danger: "bg-status-red",
};
const toneTrack: Record<WindowTone, string> = {
  ok: "bg-moon-200/70",
  warning: "bg-status-yellow/20",
  danger: "bg-status-red/15",
};
const toneText: Record<WindowTone, string> = {
  ok: "text-status-green",
  warning: "text-status-yellow",
  danger: "text-status-red",
};

export function CodexQuotaBarsCompact({
  quota,
  stale,
}: {
  quota: CodexQuota;
  stale: boolean;
}) {
  return (
    <div className={cn("space-y-1", stale && "opacity-60")}>
      <CompactRow label="5h" window={quota.primary} stale={stale} />
      <CompactRow label="7d" window={quota.secondary} stale={stale} />
    </div>
  );
}

export function CodexQuotaBarsPendingCompact() {
  return (
    <div className="space-y-1 rounded-[0.9rem] border border-dashed border-moon-200/70 bg-white/42 px-3 py-2">
      <div className="flex items-center gap-2 text-[11px] text-moon-500">
        <Clock3 className="size-3.5 text-moon-400" />
        <span className="font-medium">额度抓取中</span>
      </div>
      <div className="grid grid-cols-[1.25rem_minmax(0,1fr)_2.25rem] items-center gap-2 text-[11px] tabular-nums">
        <span className="text-moon-400">5h</span>
        <div className="h-1.5 rounded-full bg-moon-200/55" />
        <span className="text-right text-moon-350">--</span>
        <span className="text-moon-400">7d</span>
        <div className="h-1.5 rounded-full bg-moon-200/55" />
        <span className="text-right text-moon-350">--</span>
      </div>
    </div>
  );
}

function CompactRow({
  label,
  window: w,
  stale,
}: {
  label: string;
  window: QuotaWindow;
  stale: boolean;
}) {
  const remain = clampPercent(100 - w.usedPercent);
  const used = clampPercent(w.usedPercent);
  const tone = stale ? "ok" : windowTone(remain);
  const title = `${label} 窗口 · 已用 ${used.toFixed(1)}% · ${formatResetIn(w.resetAfterSeconds)} 后重置`;
  return (
    <div className="flex items-center gap-2 text-[11px] tabular-nums" title={title}>
      <span className="w-5 shrink-0 text-moon-500">{label}</span>
      <div
        className={cn(
          "relative h-1.5 flex-1 overflow-hidden rounded-full",
          stale ? "bg-moon-200/50" : toneTrack[tone],
        )}
      >
        <div
          className={cn(
            "absolute inset-y-0 left-0 rounded-full transition-[width]",
            stale ? "bg-moon-300" : toneFill[tone],
          )}
          style={{ width: `${remain}%` }}
        />
      </div>
      <span className={cn("w-9 shrink-0 text-right", stale ? "text-moon-400" : toneText[tone])}>
        {formatPercent(remain)}
      </span>
    </div>
  );
}

export function CodexQuotaBarsFull({
  quota,
  fetchedAt,
  planType,
}: {
  quota: CodexQuota;
  fetchedAt: string | undefined;
  planType: string;
}) {
  const stale = isQuotaStale(fetchedAt);
  const fetchedRel = fetchedAt ? relativeTime(sqliteToISO(fetchedAt)) : "从未";

  return (
    <section className="space-y-3 rounded-[1.2rem] border border-moon-200/55 bg-white/60 px-4 py-4">
      <div className="flex items-center justify-between gap-2">
        <p className="text-[11px] uppercase tracking-[0.18em] text-moon-400">Quota</p>
        {stale ? (
          <span className="inline-flex items-center gap-1 rounded-full bg-moon-100/80 px-2 py-0.5 text-[10.5px] text-moon-500">
            <Clock3 className="size-3" />
            抓取滞后
          </span>
        ) : null}
      </div>

      {!quota.allowed || quota.limitReached ? (
        <div className="flex items-start gap-2 rounded-[0.9rem] border border-status-red/25 bg-status-red/8 px-3 py-2 text-[12.5px] text-status-red">
          <AlertTriangle className="mt-0.5 size-3.5 shrink-0" />
          <p>限额已触发，CPA 当前会拒绝此账号的请求。</p>
        </div>
      ) : null}

      <WindowRow label="5h 窗口" window={quota.primary} stale={stale} />
      <WindowRow label="7 天窗口" window={quota.secondary} stale={stale} />

      {quota.credits?.hasCredits ? (
        <div className="flex items-center justify-between gap-2 border-t border-moon-200/50 pt-2.5 text-[12px] text-moon-500">
          <span className="text-moon-500">Credits</span>
          <span className="text-moon-700 tabular-nums">
            balance {quota.credits.balance}
            {quota.credits.unlimited ? " · 无限" : ""}
            {quota.credits.overageLimitReached ? " · overage 已触发" : ""}
          </span>
        </div>
      ) : null}

      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 border-t border-moon-200/50 pt-2.5 text-[11px] text-moon-400">
        <span>最后抓取 · {fetchedRel}</span>
        {planType ? <span>plan: {planType}</span> : null}
      </div>
    </section>
  );
}

function WindowRow({
  label,
  window: w,
  stale,
}: {
  label: string;
  window: QuotaWindow;
  stale: boolean;
}) {
  const remain = clampPercent(100 - w.usedPercent);
  const used = clampPercent(w.usedPercent);
  const tone = stale ? "ok" : windowTone(remain);
  const resetAbs = formatResetAt(w.resetAtMs);

  return (
    <div className="space-y-1.5">
      <div className="flex items-baseline justify-between gap-2">
        <p className="text-[12px] font-medium text-moon-700">{label}</p>
        <p className={cn("text-[12px] font-semibold tabular-nums", stale ? "text-moon-500" : toneText[tone])}>
          {formatPercent(remain)} 剩余
        </p>
      </div>
      <div
        className={cn(
          "relative h-2 overflow-hidden rounded-full",
          stale ? "bg-moon-200/50" : toneTrack[tone],
        )}
      >
        <div
          className={cn(
            "absolute inset-y-0 left-0 rounded-full transition-[width]",
            stale ? "bg-moon-300" : toneFill[tone],
          )}
          style={{ width: `${remain}%` }}
        />
      </div>
      <div className="flex flex-wrap items-center justify-between gap-x-2 gap-y-0.5 text-[11px] text-moon-400">
        <span>已用 {formatPercent(used)} · {formatResetIn(w.resetAfterSeconds)} 后重置</span>
        {resetAbs ? <span className="tabular-nums">{resetAbs}</span> : null}
      </div>
    </div>
  );
}

function clampPercent(v: number): number {
  if (!Number.isFinite(v)) return 0;
  if (v < 0) return 0;
  if (v > 100) return 100;
  return v;
}

// Render "42%", but keep one decimal when the value is small and non-zero so
// ">0 but rounded to 0" doesn't read as fully idle.
function formatPercent(v: number): string {
  if (v === 0) return "0%";
  if (v < 1) return `${v.toFixed(1)}%`;
  return `${Math.round(v)}%`;
}

function sqliteToISO(s: string): string {
  // Reuse parseSqliteUTC's normalization; relativeTime accepts either form.
  const ms = parseSqliteUTC(s);
  if (ms == null) return s;
  return new Date(ms).toISOString();
}
