import type { Account, Pool, PoolDetailResponse } from "@/lib/types";

export type RouteHealth = "unknown" | "healthy" | "degraded" | "error" | "disabled";

export type RouteSummary = {
  status: RouteHealth;
  label: string;
  reason: string;
  impact: string;
  actions: string[];
};

const ROUTE_LABELS: Record<RouteHealth, string> = {
  unknown: "待检查",
  healthy: "正常",
  degraded: "降级",
  error: "异常",
  disabled: "已停用",
};

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
  const availableMembers = activeMembers.filter((member) =>
    member.account ? isAccountRoutable(member.account) : false,
  );
  const healthyMembers = availableMembers.filter((member) => member.account?.status === "healthy");
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

export function hasRuntimeBindingIssue(account: Account | null | undefined): boolean {
  return (
    account?.source_kind === "cpa" &&
    isCpaOrDirectOtherwiseRoutable(account) &&
    !account.runtime?.provider_pinning_supported
  );
}

export function isAccountRoutable(account: Account): boolean {
  if (!isCpaOrDirectOtherwiseRoutable(account)) return false;
  if (account.source_kind === "cpa") {
    if (hasRuntimeBindingIssue(account)) return false;
  }
  return true;
}

function isCpaOrDirectOtherwiseRoutable(account: Account): boolean {
  if (!account.enabled) return false;
  if (account.status !== "healthy" && account.status !== "degraded") return false;
  if (account.serving_status === "cooldown") {
    const until = account.cooldown_until ? new Date(account.cooldown_until).getTime() : Number.NaN;
    if (Number.isNaN(until) || until > Date.now()) return false;
  }
  if (account.serving_status === "error") return false;
  if (account.source_kind === "cpa") {
    return isCpaOtherwiseRoutable(account);
  }
  return true;
}

function isCpaOtherwiseRoutable(account: Account): boolean {
  if (account.source_kind !== "cpa") return false;
  const provider = String(account.cpa_provider || "").toLowerCase();
  if (
    ["needs_login", "refresh_failed", "runtime_pending", "runtime_error", "unknown", ""].includes(
      account.cpa_credential_status || "",
    )
  ) {
    return false;
  }
  if (account.cpa_quota_status === "blocked") return false;
  if (
    provider === "codex" &&
    account.cpa_quota_status === "error" &&
    account.cpa_quota_last_error?.startsWith("HTTP 429 from model request")
  ) {
    return false;
  }
  if (provider === "codex") {
    if (account.cpa_access_status === "eligible") return true;
    if (account.cpa_access_status === "ineligible" || account.cpa_access_status === "pending") return false;
    return account.cpa_subscription_status === "active";
  }
  return true;
}

export function accountHasRoutePenalty(account: Account | null | undefined): boolean {
  if (!account || account.source_kind !== "cpa") return false;
  return (
    account.cpa_credential_status === "auth_suspect" ||
    account.cpa_quota_status === "error" ||
    account.cpa_quota_status === "unknown"
  );
}

export function getRouteHealth(
  account: Account | null | undefined,
  memberEnabled: boolean,
  discoveryHealth?: string,
): RouteHealth {
  return getRouteSummary(account, memberEnabled, discoveryHealth).status;
}

export function getRouteSummary(
  account: Account | null | undefined,
  memberEnabled: boolean,
  discoveryHealth?: string,
): RouteSummary {
  if (!account) {
    return routeSummary("unknown", "账号数据缺失", "无法判断路由能力。", ["刷新页面"]);
  }
  if (!memberEnabled || !account.enabled) {
    return routeSummary("disabled", "账号已停用", "普通路由不会选择这个账号。", ["启用账号或 Pool 成员"]);
  }
  if (hasRuntimeBindingIssue(account)) {
    return routeSummary(
      "error",
      "Binding 未确认",
      "请求量、额度归因和健康修复暂不可信，普通流量会在网关侧 fail closed。",
      ["检查 CPA runtime", "查看 Activity 中的 Runtime Binding", "等待 provider pinning 支持后再接流量"],
    );
  }

  const health = discoveryHealth ?? getAccountHealth(account);
  if (account.status === "unknown" || health === "unknown") {
    return routeSummary("unknown", "状态待检查", "缺少足够的健康检查结果，普通路由暂不选择。", ["刷新账号状态", "运行自检"]);
  }

  if (account.source_kind === "cpa") {
    const credentialStatus = String(account.cpa_credential_status || "unknown");
    switch (credentialStatus) {
      case "needs_login":
        return routeSummary("error", "需要重登", "已确认上游凭据失效，普通路由会跳过。", ["重新登录", "刷新账号状态"]);
      case "refresh_failed":
        return routeSummary("error", "凭据刷新失败", "系统刷新登录态失败，普通路由会跳过。", ["重新登录", "检查 CPA runtime"]);
      case "runtime_error":
        return routeSummary("error", "CPA Runtime 异常", "运行环境不可用或配置异常，普通路由会跳过。", ["检查 CPA runtime", "查看服务日志"]);
      case "runtime_pending":
        return routeSummary("error", "凭据同步中", "auth file 与 runtime 索引仍在同步，普通路由暂不选择。", ["稍后刷新状态"]);
      case "unknown":
      case "":
        return routeSummary("error", "凭据状态未知", "缺少可信凭据状态，普通路由会跳过。", ["刷新账号状态", "检查 CPA runtime"]);
      default:
        break;
    }

    const provider = String(account.cpa_provider || "").toLowerCase();
    if (provider === "codex") {
      const accessStatus = account.cpa_access_status || "unknown";
      if (accessStatus !== "eligible") {
        const reason =
          accessStatus === "ineligible"
            ? "Access 不可用"
            : accessStatus === "pending"
              ? "Access 待确认"
              : accessStatus === "error"
                ? "Access 探测失败"
                : "Access 未确认";
        return routeSummary("error", reason, "Codex 使用资格尚未确认可用，普通路由会跳过。", ["刷新账号状态", "检查 Access 详情"]);
      }
    }

    if (account.cpa_quota_status === "blocked") {
      return routeSummary("error", "额度已用尽", "额度接口明确拒绝继续使用，普通路由会跳过。", ["刷新额度", "更换账号"]);
    }
    const quotaErrorMeta = getCpaQuotaErrorMeta(account);
    if (quotaErrorMeta?.reason === "model_request_429") {
      return routeSummary(
        account.serving_status === "cooldown" ? "error" : "degraded",
        quotaErrorMeta.label,
        quotaErrorMeta.detail,
        ["刷新额度", "查看 Activity", "等待冷却结束"],
      );
    }
  }

  if (account.serving_status === "cooldown") {
    return routeSummary("error", "服务冷却中", "最近真实请求失败，冷却结束前普通路由会跳过。", ["等待冷却结束", "运行自检"]);
  }
  if (account.serving_status === "error") {
    return routeSummary("error", "服务异常", "真实模型请求持续失败，普通路由会跳过。", ["运行自检", "查看最近错误"]);
  }
  if (!isAccountRoutable(account)) {
    return routeSummary("error", "不可路由", "当前组合状态不满足普通路由条件。", ["刷新账号状态", "查看诊断"]);
  }

  if (account.source_kind === "cpa") {
    if (account.cpa_quota_status === "error") {
      const quotaErrorMeta = getCpaQuotaErrorMeta(account);
      return routeSummary(
        "degraded",
        quotaErrorMeta?.label || "额度查询失败",
        quotaErrorMeta?.detail || "最近模型调用可用性未被单独否定，但额度接口暂不可用，路由会降权。",
        ["刷新额度", "查看 Activity"],
      );
    }
    if (account.cpa_quota_status === "unknown") {
      return routeSummary("degraded", "额度未知", "没有可用额度快照，账号可路由但会降权。", ["刷新额度"]);
    }
    if (account.cpa_credential_status === "auth_suspect") {
      return routeSummary(
        "degraded",
        "鉴权待确认",
        "辅助接口疑似鉴权异常，但真实模型调用尚未确认失败，路由会降权。",
        ["运行直测", "查看 Activity"],
      );
    }
  }

  return routeSummary("healthy", "可接流量", "账号满足普通路由条件。", ["保持监控"]);
}

function routeSummary(
  status: RouteHealth,
  reason: string,
  impact: string,
  actions: string[],
): RouteSummary {
  return {
    status,
    label: ROUTE_LABELS[status],
    reason,
    impact,
    actions,
  };
}

export function getAccountHealth(
  account: Account,
): "unknown" | "healthy" | "degraded" | "error" | "disabled" {
  if (!account.enabled) return "disabled";
  // Direct accounts surface the user's self-check verdict when one exists, so
  // the badge on the card matches whatever the Pool-detail self-check reported.
  // Fall back to the health-loop status (models endpoint probe) when there's
  // been no self-check yet. CPA accounts stay on `status` — their health is
  // measured at the CPA service layer, not per-account.
  if (account.source_kind !== "cpa" && account.last_probe_status) {
    const probe = account.last_probe_status;
    if (probe === "healthy" || probe === "degraded" || probe === "error") {
      return probe;
    }
  }
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

export function getCpaCredentialMeta(account: Account): {
  label: string;
  detail: string;
  tone: "default" | "danger";
} | null {
  if (account.source_kind !== "cpa") return null;
  const status = account.cpa_credential_status || "unknown";
  if (
    ![
      "needs_login",
      "refresh_failed",
      "auth_suspect",
      "runtime_pending",
      "runtime_error",
      "unknown",
    ].includes(status)
  )
    return null;
  const reason = account.cpa_credential_reason || "";
  const reasonLabel = cpaCredentialReasonLabel(reason);
  const pending = status === "runtime_pending";
  return {
    label: pending
      ? "凭据同步中"
      : status === "runtime_error"
        ? "CPA Runtime 异常"
        : status === "auth_suspect"
          ? "鉴权待确认"
          : status === "unknown"
            ? "凭据状态未知"
          : status === "refresh_failed"
            ? "凭据刷新失败"
            : "需要重新登录",
    detail: pending ? reasonLabel : account.cpa_credential_last_error || reasonLabel,
    tone: pending ? "default" : "danger",
  };
}

export function getCpaQuotaErrorMeta(account: Account): {
  label: string;
  detail: string;
  tone: "warning" | "danger";
  source: "model_request" | "wham_usage" | "cpa_management" | "store";
  reason:
    | "quota_blocked"
    | "model_request_429"
    | "quota_fetch_auth_failed"
    | "runtime_api_call_failed"
    | "quota_fetch_failed"
    | "quota_unknown";
} | null {
  if (account.source_kind !== "cpa") return null;
  if (account.cpa_provider.toLowerCase() !== "codex") return null;
  const status = account.cpa_quota_status || "";
  if (status === "blocked") {
    return {
      label: "额度已用尽",
      detail: account.cpa_quota_last_error || "额度已耗尽或上游拒绝使用",
      tone: "danger",
      source: "store",
      reason: "quota_blocked",
    };
  }
  if (status === "error") {
    const lastError = account.cpa_quota_last_error || "";
    if (lastError.startsWith("HTTP 429 from model request")) {
      return {
        label: "模型请求被限流",
        detail: lastError || "最近一次真实模型请求返回了 HTTP 429",
        tone: "warning",
        source: "model_request",
        reason: "model_request_429",
      };
    }
    const lower = lastError.toLowerCase();
    if (lower.includes("management") || lower.includes("api-call")) {
      return {
        label: "额度查询失败",
        detail: lastError || "CPA management 无法代理额度查询。",
        tone: "warning",
        source: "cpa_management",
        reason: "runtime_api_call_failed",
      };
    }
    if (lastError.includes("HTTP 401") || lastError.includes("HTTP 403")) {
      return {
        label: "额度查询失败",
        detail: lastError || "额度辅助接口鉴权失败，不代表模型请求被限流。",
        tone: "warning",
        source: "wham_usage",
        reason: "quota_fetch_auth_failed",
      };
    }
    return {
      label: "额度查询失败",
      detail: lastError || "额度辅助接口暂不可用。",
      tone: "warning",
      source: "wham_usage",
      reason: "quota_fetch_failed",
    };
  }
  if (status === "unknown") {
    return {
      label: "额度未知",
      detail: account.cpa_quota_last_error || "没有可用额度快照。",
      tone: "warning",
      source: "store",
      reason: "quota_unknown",
    };
  }
  return null;
}

export function getCpaSubscriptionErrorMeta(account: Account): {
  label: string;
  detail: string;
  tone: "warning";
} | null {
  if (account.source_kind !== "cpa") return null;
  if (account.cpa_provider.toLowerCase() !== "codex") return null;
  if (account.cpa_subscription_expires_at) return null;
  const detail = account.cpa_subscription_last_error || "";
  if (!detail) return null;
  return {
    label: "订阅到期获取失败",
    detail,
    tone: "warning",
  };
}

export function cpaCredentialReasonLabel(reason: string): string {
  switch (reason) {
    case "disabled":
      return "CPA 凭证已被禁用";
    case "file_missing":
      return "CPA 凭证文件缺失";
    case "file_corrupt":
      return "CPA 凭证文件损坏";
    case "auth_index_pending":
      return "正在同步账号凭据";
    case "runtime_unreachable":
      return "CPA runtime 不可用";
    case "service_missing":
      return "CPA 服务缺失";
    case "management_key_missing":
      return "CPA 管理密钥缺失";
    case "auth_dir_missing":
      return "CPA 凭证目录未配置";
    case "refresh_failed":
      return "CPA 自动刷新失败";
    case "auth_suspect":
      return "鉴权状态待确认";
    case "auth_failed":
      return "CPA 授权失败";
    default:
      return "CPA 登录态失效";
  }
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
