/** Format a number as percentage string, e.g. 0.956 -> "95.6%" */
export function pct(value: number): string {
  return `${(value * 100).toFixed(1)}%`;
}

/** Format milliseconds as a human-readable latency, e.g. 1234 -> "1.23s" */
export function latency(ms: number): string {
  if (ms < 1) return "< 1ms";
  if (ms < 1000) return `${Math.round(ms)}ms`;
  return `${(ms / 1000).toFixed(2)}s`;
}

/** Format a quota number: 0 means unlimited. */
export function quota(n: number): string {
  if (n === 0) return "unlimited";
  return n.toLocaleString();
}

/** Normalize a SQLite datetime string so JS parses it as UTC.
 *  SQLite `datetime('now')` returns "2026-04-14 04:00:00" (UTC, no tz indicator).
 *  Without the 'Z' suffix, `new Date()` treats it as local time.
 */
function parseUTC(iso: string): Date {
  const s = iso.includes("T") || iso.includes("Z") || iso.includes("+")
    ? iso
    : iso.replace(" ", "T") + "Z";
  return new Date(s);
}

/** Format an ISO date string to a compact local representation. */
export function shortDate(iso: string): string {
  const d = parseUTC(iso);
  if (isNaN(d.getTime())) return iso;
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getMonth() + 1}/${d.getDate()} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/** Format a large number with k/M suffix. */
export function compact(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return String(n);
}

/** Format an ISO date string as relative time, e.g. "2 分钟前" */
export function relativeTime(iso: string | null): string {
  if (!iso) return "从未";
  const d = parseUTC(iso);
  if (isNaN(d.getTime())) return iso;
  const seconds = Math.floor((Date.now() - d.getTime()) / 1000);
  if (seconds < 5) return "刚刚";
  if (seconds < 60) return `${seconds}s 前`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes} 分钟前`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours} 小时前`;
  const days = Math.floor(hours / 24);
  return `${days} 天前`;
}

/** Format token count with unit, e.g. 456000 -> "456K tokens" */
export function tokenCount(n: number): string {
  return `${compact(n)} tokens`;
}
