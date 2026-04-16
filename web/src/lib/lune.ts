import type { Account, Pool, PoolDetailResponse } from "@/lib/types";

export type PoolSnapshot = {
  id: number;
  label: string;
  enabled: boolean;
  activeAccountIds: number[];
  activeAccountCount: number | null;
  availableAccountCount: number | null;
  memberStatusCounts: {
    healthy: number;
    degraded: number;
    error: number;
    disabled: number;
    unknown: number;
    total: number;
  } | null;
  models: string[];
  health: "healthy" | "degraded" | "error" | "disabled" | "unknown";
};

export function ensureArray<T>(value: T[] | null | undefined): T[] {
  return Array.isArray(value) ? value : [];
}

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
  if (pool.routable_account_count === 0) return "error";
  if (pool.healthy_account_count === pool.account_count) return "healthy";
  return "degraded";
}

export function derivePoolSnapshot(pool: Pool, detail?: PoolDetailResponse): PoolSnapshot {
  if (!detail) {
    return {
      id: pool.id,
      label: pool.label,
      enabled: pool.enabled,
      activeAccountIds: [],
      activeAccountCount: null,
      availableAccountCount: null,
      memberStatusCounts: null,
      models: [],
      health: pool.enabled ? "unknown" : "disabled",
    };
  }

  const activeMembers = ensureArray(detail.members).filter(
    (member) => member.enabled && member.account?.enabled,
  );
  const healthyMembers = activeMembers.filter((member) => member.account?.status === "healthy");
  const availableMembers = activeMembers.filter(
    (member) => member.account?.status === "healthy" || member.account?.status === "degraded",
  );
  const memberStatusCounts = ensureArray(detail.members).reduce<NonNullable<PoolSnapshot["memberStatusCounts"]>>(
    (counts, member) => {
      counts.total += 1;
      if (!member.enabled || !member.account?.enabled || member.account?.status === "disabled") {
        counts.disabled += 1;
        return counts;
      }
      if (member.account.status === "healthy") {
        counts.healthy += 1;
        return counts;
      }
      if (member.account.status === "degraded") {
        counts.degraded += 1;
        return counts;
      }
      if (member.account.status === "error") {
        counts.error += 1;
        return counts;
      }
      counts.unknown += 1;
      return counts;
    },
    {
      healthy: 0,
      degraded: 0,
      error: 0,
      disabled: 0,
      unknown: 0,
      total: 0,
    },
  );
  const modelSet = new Set<string>();

  availableMembers.forEach((member) => {
    ensureArray(member.account?.models).forEach((model) => {
      modelSet.add(model);
    });
  });

  let health: PoolSnapshot["health"] = "disabled";
  if (!pool.enabled) {
    health = "disabled";
  } else if (activeMembers.length === 0) {
    health = "degraded";
  } else if (availableMembers.length === 0) {
    health = "error";
  } else if (healthyMembers.length === activeMembers.length) {
    health = "healthy";
  } else {
    health = "degraded";
  }

  return {
    id: pool.id,
    label: pool.label,
    enabled: pool.enabled,
    activeAccountIds: activeMembers.map((member) => member.account_id),
    activeAccountCount: activeMembers.length,
    availableAccountCount: availableMembers.length,
    memberStatusCounts,
    models: pool.enabled ? Array.from(modelSet).sort() : [],
    health,
  };
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
