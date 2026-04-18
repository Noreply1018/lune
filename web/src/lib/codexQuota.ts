import type { Account } from "@/lib/types";

export type WindowTone = "ok" | "warning" | "danger";

export interface QuotaWindow {
  usedPercent: number;
  limitWindowSeconds: number;
  resetAfterSeconds: number;
  resetAtMs: number;
}

export interface QuotaCredits {
  hasCredits: boolean;
  unlimited: boolean;
  overageLimitReached: boolean;
  balance: string;
}

export interface CodexQuota {
  primary: QuotaWindow;
  secondary: QuotaWindow;
  allowed: boolean;
  limitReached: boolean;
  credits: QuotaCredits | null;
  spendControlReached: boolean;
}

// Parse the raw `accounts.codex_quota_json` payload. Only populated for
// CPA-codex accounts and only after at least one successful quota fetch.
export function parseCodexQuota(account: Account): CodexQuota | null {
  if (account.cpa_provider !== "codex") return null;
  const raw = account.codex_quota_json;
  if (!raw) return null;
  let parsed: Record<string, unknown>;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return null;
  }
  const rl = parsed.rate_limit as Record<string, unknown> | undefined;
  if (!rl) return null;
  const primary = parseWindow(rl.primary_window);
  const secondary = parseWindow(rl.secondary_window);
  if (!primary || !secondary) return null;

  const creditsRaw = parsed.credits as Record<string, unknown> | undefined;
  const credits: QuotaCredits | null = creditsRaw
    ? {
        hasCredits: Boolean(creditsRaw.has_credits),
        unlimited: Boolean(creditsRaw.unlimited),
        overageLimitReached: Boolean(creditsRaw.overage_limit_reached),
        balance: String(creditsRaw.balance ?? ""),
      }
    : null;

  const spendCtrl = parsed.spend_control as Record<string, unknown> | undefined;

  return {
    primary,
    secondary,
    allowed: rl.allowed !== false,
    limitReached: Boolean(rl.limit_reached),
    credits,
    spendControlReached: Boolean(spendCtrl?.reached),
  };
}

function parseWindow(raw: unknown): QuotaWindow | null {
  if (!raw || typeof raw !== "object") return null;
  const w = raw as Record<string, unknown>;
  const usedPercent = toNumber(w.used_percent);
  const limit = toNumber(w.limit_window_seconds);
  const resetAfter = toNumber(w.reset_after_seconds);
  const resetAt = toNumber(w.reset_at);
  if (limit == null || resetAfter == null || resetAt == null) return null;
  return {
    usedPercent: usedPercent ?? 0,
    limitWindowSeconds: limit,
    resetAfterSeconds: resetAfter,
    resetAtMs: resetAt * 1000,
  };
}

function toNumber(v: unknown): number | null {
  if (typeof v === "number" && Number.isFinite(v)) return v;
  return null;
}

// Tone reflects how much headroom is left: a full green bar means plenty of
// quota remaining; red means the window is almost exhausted. Callers pass the
// "remaining" percentage (100 - used).
export function windowTone(remainPercent: number): WindowTone {
  if (remainPercent < 20) return "danger";
  if (remainPercent < 50) return "warning";
  return "ok";
}

// Format a duration in seconds as a short countdown like "4h 59m" / "5d 18h" / "12m".
export function formatResetIn(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return "已重置";
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (days > 0) return `${days}d ${hours}h`;
  if (hours > 0) return `${hours}h ${minutes}m`;
  if (minutes > 0) return `${minutes}m`;
  return `${Math.max(1, Math.floor(seconds))}s`;
}

// Format an absolute epoch-ms timestamp for the local timezone.
export function formatResetAt(ms: number): string {
  if (!Number.isFinite(ms) || ms <= 0) return "";
  const d = new Date(ms);
  if (isNaN(d.getTime())) return "";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

// Quota snapshot is considered stale when older than this threshold.
// The backend fetches every `codex_quota_fetch_interval` seconds (default 600).
// A 900s (15min) threshold gives one full interval of slack before we warn.
const STALE_THRESHOLD_MS = 15 * 60 * 1000;

export function isQuotaStale(fetchedAt: string | undefined): boolean {
  if (!fetchedAt) return true;
  const ms = parseSqliteUTC(fetchedAt);
  if (ms == null) return true;
  return Date.now() - ms > STALE_THRESHOLD_MS;
}

// Parse a timestamp written by the backend in SQLite "YYYY-MM-DD HH:MM:SS" (UTC) form,
// or any ISO-8601 string. Returns epoch-ms, or null if unparseable.
export function parseSqliteUTC(s: string | null | undefined): number | null {
  if (!s) return null;
  const normalized = s.includes("T") || s.includes("Z") || s.includes("+")
    ? s
    : s.replace(" ", "T") + "Z";
  const ms = Date.parse(normalized);
  return Number.isFinite(ms) ? ms : null;
}
