import type { Account, Pool } from "@/lib/types";

export function getApiBaseUrl(externalUrl?: string | null): string {
  const fallback = `${window.location.origin}/v1`;
  if (!externalUrl) {
    return fallback;
  }

  const normalized = externalUrl.trim().replace(/\/$/, "");
  if (!normalized) {
    return fallback;
  }
  if (normalized.endsWith("/v1")) {
    return normalized;
  }
  return `${normalized}/v1`;
}

export function maskToken(token: string): string {
  if (!token) return "";
  if (token.length <= 12) return token;
  return `${token.slice(0, 12)}...${token.slice(-4)}`;
}

export function getPoolHealth(pool: Pool): "healthy" | "degraded" | "error" | "disabled" {
  if (!pool.enabled) return "disabled";
  if (pool.account_count === 0) return "degraded";
  if (pool.healthy_account_count === 0) return "error";
  if (pool.healthy_account_count < pool.account_count) return "degraded";
  return "healthy";
}

export function getAccountHealth(
  account: Account,
): "unknown" | "healthy" | "degraded" | "error" | "disabled" {
  if (!account.enabled) return "disabled";
  if (account.status === "healthy" || account.status === "error" || account.status === "unknown") {
    return account.status;
  }
  if (account.status === "degraded") {
    return "degraded";
  }
  return "unknown";
}

export function getProviderLabel(account: Account): string {
  if (account.source_kind === "cpa") {
    return account.cpa_provider || "CPA";
  }
  return account.provider || "Direct";
}

export function getAccessLabel(account: Account): string {
  if (account.source_kind === "cpa") {
    return `CPA · ${getProviderLabel(account)}`;
  }
  return `直连 · ${getProviderLabel(account)}`;
}

export function getExpiryMeta(iso: string | null): {
  label: string;
  tone: "default" | "warning" | "danger";
  daysLeft: number | null;
} | null {
  if (!iso) return null;
  const expiry = new Date(iso).getTime();
  if (Number.isNaN(expiry)) return null;
  const diff = expiry - Date.now();
  const daysLeft = Math.ceil(diff / (24 * 60 * 60 * 1000));
  if (diff <= 0) {
    return { label: "已过期", tone: "danger", daysLeft: 0 };
  }
  if (daysLeft <= 7) {
    return { label: `${daysLeft} 天内到期`, tone: "warning", daysLeft };
  }
  return { label: `${daysLeft} 天后到期`, tone: "default", daysLeft };
}

export function parseQuotaDisplay(raw: string): string {
  if (!raw) return "--";

  try {
    const data = JSON.parse(raw) as Record<string, string | number | undefined>;
    const used = data.used ?? data.current ?? data.value;
    const total = data.total ?? data.limit ?? data.max;
    const unit = data.unit ? ` ${String(data.unit)}` : "";
    if (used != null && total != null) {
      return `${used} / ${total}${unit}`;
    }
    if (used != null) {
      return `${used}${unit}`;
    }
  } catch {
    // fall through
  }

  return raw;
}
