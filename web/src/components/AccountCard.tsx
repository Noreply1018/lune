import { useMemo } from "react";
import { GripVertical, Info, MoreHorizontal, Power, RefreshCw, Trash2 } from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Button } from "@/components/ui/button";
import StatusBadge from "@/components/StatusBadge";
import {
  CodexQuotaBarsCompact,
  CodexQuotaBarsPendingCompact,
} from "@/components/CodexQuotaBars";
import DirectAccountSignal from "@/components/DirectAccountSignal";
import { isQuotaStale, parseCodexQuota } from "@/lib/codexQuota";
import { compact, relativeTime } from "@/lib/fmt";
import type { PoolMember } from "@/lib/types";
import {
  getAccessLabel,
  getAccountHealth,
  getCpaCredentialMeta,
  getCpaQuotaErrorMeta,
  getExpiryMeta,
  getRouteHealth,
  hasRuntimeBindingIssue,
} from "@/lib/lune";
import { cn } from "@/lib/utils";

type Variant = "active" | "disabled";
type FlashState = "success" | "error" | null;
type CardChip = {
  label: string;
  detail?: string;
  tone?: "default" | "warning" | "danger" | "processing" | "binding";
};

function chipClass(tone: CardChip["tone"]) {
  switch (tone) {
    case "danger":
      return "bg-status-red/10 text-status-red";
    case "warning":
      return "bg-status-yellow/12 text-status-yellow";
    case "processing":
      return "bg-fog-100 text-fog-500";
    case "binding":
      return "bg-lunar-100/85 text-lunar-700";
    default:
      return "bg-moon-100/70 text-moon-500";
  }
}

function compactCredentialLabel(label: string) {
  return label === "需要重新登录" ? "需要重登" : label;
}

function getSubscriptionChip(
  account: PoolMember["account"],
  expiry: ReturnType<typeof getExpiryMeta>,
  isCodexCpa: boolean,
): CardChip | null {
  if (!account || !isCodexCpa) return null;
  switch (account.cpa_subscription_status || "unknown") {
    case "active":
      return expiry ? { label: expiry.label, tone: expiry.tone } : null;
    case "expired":
      return { label: "已过期", detail: account.cpa_subscription_last_error, tone: "danger" };
    case "free":
      return { label: "Free", detail: account.cpa_subscription_last_error, tone: "danger" };
    case "pending":
      return { label: "订阅刷新中", detail: account.cpa_subscription_last_error, tone: "processing" };
    case "error":
      return { label: "订阅获取失败", detail: account.cpa_subscription_last_error, tone: "warning" };
    default:
      return { label: "订阅未知", detail: account.cpa_subscription_last_error, tone: "warning" };
  }
}

function getMainIssueChip(
  account: PoolMember["account"],
  credential: ReturnType<typeof getCpaCredentialMeta>,
  quotaError: ReturnType<typeof getCpaQuotaErrorMeta>,
): CardChip | null {
  if (!account) return null;
  const credentialStatus = account.cpa_credential_status || "unknown";
  if (hasRuntimeBindingIssue(account)) {
    return {
      label: "Binding 未确认",
      detail: "CPA HTTP provider 当前无法确认 per-request auth pinning，普通流量会 fail closed。",
      tone: "binding",
    };
  }
  if (
    account.source_kind === "cpa" &&
    ["needs_login", "refresh_failed", "runtime_error", "runtime_pending", "unknown", ""].includes(
      credentialStatus,
    ) &&
    credential
  ) {
    return {
      label: compactCredentialLabel(credential.label),
      detail: credential.detail,
      tone: credentialStatus === "runtime_pending" ? "processing" : credential.tone,
    };
  }
  if (quotaError?.tone === "danger") {
    return quotaError;
  }
  if (account.serving_status === "cooldown") {
    const quotaEvidence = account.cpa_provider?.toLowerCase() === "codex" ? getCpaQuotaErrorMeta(account) : null;
    if (quotaEvidence?.tone === "warning") {
      return quotaEvidence;
    }
    return {
      label: "服务冷却中",
      detail: account.last_error || account.cooldown_until || "",
      tone: "processing",
    };
  }
  if (account.serving_status === "error") {
    return { label: "服务异常", detail: account.last_error || undefined, tone: "danger" };
  }
  if (quotaError?.tone === "warning") {
    return quotaError;
  }
  if (account.source_kind === "cpa" && credentialStatus === "auth_suspect" && credential) {
    return {
      label: compactCredentialLabel(credential.label),
      detail: credential.detail,
      tone: "warning",
    };
  }
  return null;
}

export default function AccountCard({
  member,
  requests,
  successRate,
  variant,
  priorityIndex,
  dragging = false,
  selected = false,
  dimmed = false,
  flashState = null,
  refreshing = false,
  onOpenDetails,
  onToggleEnabled,
  onDelete,
  onRefresh,
  dragHandleProps,
}: {
  member: PoolMember;
  requests: number;
  successRate: number | null;
  variant: Variant;
  priorityIndex?: number;
  dragging?: boolean;
  selected?: boolean;
  dimmed?: boolean;
  flashState?: FlashState;
  refreshing?: boolean;
  onOpenDetails: () => void;
  onToggleEnabled: () => void;
  onDelete: () => void;
  onRefresh: () => void;
  dragHandleProps?: React.HTMLAttributes<HTMLDivElement>;
}) {
  const account = member.account;
  const enabled = member.enabled;
  const health = account ? getAccountHealth(account) : "error";
  const routeHealth = getRouteHealth(account, enabled, health);
  // Card re-renders on every drag tick; parsing the raw JSON each time is
  // wasted work even though the payload is small.
  const codexQuota = useMemo(
    () => (account ? parseCodexQuota(account) : null),
    [account?.source_kind, account?.cpa_provider, account?.codex_quota_json],
  );
  const isCodexCpa =
    account?.source_kind === "cpa" && account.cpa_provider.toLowerCase() === "codex";
  const expiry = getExpiryMeta(
    isCodexCpa
      ? account?.cpa_subscription_expires_at ?? null
      : account?.cpa_expired_at ?? null,
  );
  const credential = account ? getCpaCredentialMeta(account) : null;
  const quotaError = account ? getCpaQuotaErrorMeta(account) : null;
  const requestChip: CardChip = { label: `今日 ${compact(requests)}` };
  const subscriptionChip = getSubscriptionChip(account, expiry, isCodexCpa);
  const mainIssueChip = getMainIssueChip(account, credential, quotaError);
  const cardChips = [requestChip, subscriptionChip, mainIssueChip].filter(
    (chip): chip is CardChip => Boolean(chip),
  );
  const codexQuotaStale = codexQuota ? isQuotaStale(account?.codex_quota_fetched_at) : false;
  // Every non-Codex account — direct as well as non-Codex CPA (e.g. Claude) —
  // gets the dual-row signal strip so both card variants share the same height.
  const showDirectSignal = !isCodexCpa;

  const toneClass = !enabled
    ? "border-moon-200/60 bg-moon-100/55"
    : !account || health === "error"
      ? "border-status-red/20 bg-red-50/55"
      : health === "degraded"
        ? "border-status-yellow/20 bg-amber-50/55"
        : "border-white/78 bg-white/88";

  if (!account) {
    return (
      <article className="rounded-[1.2rem] border border-status-red/20 bg-red-50/60 p-3 text-xs text-status-red">
        账号数据缺失。
      </article>
    );
  }

  const isActive = variant === "active";

  const flashRingClass =
    flashState === "success"
      ? "ring-2 ring-status-green/55 animate-pulse"
      : flashState === "error"
        ? "ring-2 ring-status-red/55 animate-pulse"
        : "";
  const selectedClass = selected
    ? "ring-2 ring-lunar-300/70 scale-[1.01] shadow-[0_26px_52px_-30px_rgba(134,125,193,0.45)]"
    : "";
  const dimmedClass = dimmed && !selected ? "opacity-60 saturate-75" : "";
  const refreshingClass = refreshing
    ? "ring-2 ring-lunar-300/55 shadow-[0_24px_50px_-30px_rgba(134,125,193,0.42)]"
    : "";

  return (
    <article
      {...dragHandleProps}
      className={cn(
        "group relative cursor-grab overflow-hidden rounded-[1.3rem] border transition-all duration-200",
        "shadow-[0_18px_38px_-30px_rgba(33,40,63,0.22)]",
        toneClass,
        dragging
          ? "z-10 scale-[1.025] cursor-grabbing shadow-[0_28px_60px_-32px_rgba(33,40,63,0.4)]"
          : "hover:shadow-[0_22px_44px_-28px_rgba(33,40,63,0.3)]",
        isActive ? "min-h-[9.4rem] px-3.5 py-3" : "px-3 py-2.5",
        selectedClass,
        dimmedClass,
        refreshingClass,
        flashRingClass,
      )}
    >
      <div className="pointer-events-none absolute inset-x-0 top-0 h-10 bg-[linear-gradient(180deg,rgba(255,255,255,0.18),transparent)]" />

      <div
        className={cn(
          "pointer-events-none absolute bottom-0 left-0 z-0 flex w-5 items-center justify-center text-moon-300 opacity-0 transition-opacity",
          isActive && priorityIndex != null ? "top-6" : "top-0",
          "group-hover:opacity-100",
          dragging ? "opacity-100" : "",
        )}
        aria-hidden
      >
        <GripVertical className="size-3.5" />
      </div>

      {isActive && priorityIndex != null ? (
        <span
          className={cn(
            "absolute left-0 top-0 z-10 flex size-5 items-center justify-center rounded-br-[0.9rem] rounded-tl-[1.3rem] bg-lunar-200/70 text-[10px] font-semibold text-lunar-700 shadow-[inset_0_-1px_0_rgba(154,147,201,0.25)]",
            !enabled ? "opacity-60" : "",
          )}
          title={`当前优先级序号 ${priorityIndex}`}
          aria-hidden
        >
          {priorityIndex}
        </span>
      ) : null}

      <div className="relative flex h-full flex-col gap-2.5 pl-2">
        <div className="flex items-start justify-between gap-2">
          <div className="min-w-0 space-y-1">
            <p className="eyebrow-label text-[10px]">{getAccessLabel(account)}</p>
            <div className="flex items-center gap-1.5">
              <h3 className="truncate text-[0.95rem] font-semibold tracking-[-0.02em] text-moon-800">
                {account.label}
              </h3>
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-0.5">
            <StatusBadge status={routeHealth} />
          </div>
        </div>

        {codexQuota ? (
          <CodexQuotaBarsCompact quota={codexQuota} stale={codexQuotaStale} />
        ) : isCodexCpa ? (
          <CodexQuotaBarsPendingCompact />
        ) : showDirectSignal ? (
          <DirectAccountSignal requests={requests} successRate={successRate} />
        ) : null}

        <div className="flex flex-wrap items-center gap-1.5 text-[11px] text-moon-500">
          {cardChips.map((chip, index) => (
            <span
              key={`${chip.label}-${index}`}
              className={cn(
                "max-w-full truncate rounded-full px-2 py-0.5",
                index === 0 ? "bg-moon-100/80 text-moon-500" : chipClass(chip.tone),
              )}
              title={chip.detail}
            >
              {chip.label}
            </span>
          ))}
        </div>

        <div className="mt-auto flex items-center justify-between gap-2 pt-1">
          <p className="truncate text-[10.5px] text-moon-400">
            {refreshing ? "刷新中…" : showDirectSignal ? "\u00a0" : relativeTime(account.last_checked_at ?? null)}
          </p>
          <div className="flex items-center gap-0.5">
            <Button
              variant="ghost"
              size="icon"
              className="size-7 rounded-full text-moon-500"
              onPointerDown={(event) => event.stopPropagation()}
              onClick={(event) => {
                event.stopPropagation();
                onOpenDetails();
              }}
              title="查看详情"
            >
              <Info className="size-3.5" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              className="size-7 rounded-full text-moon-500"
              disabled={refreshing}
              onPointerDown={(event) => event.stopPropagation()}
              onClick={(event) => {
                event.stopPropagation();
                if (refreshing) return;
                onRefresh();
              }}
              title={refreshing ? "刷新中" : "刷新"}
            >
              <RefreshCw className={cn("size-3.5", refreshing ? "animate-spin" : "")} />
            </Button>
            <DropdownMenu>
              <DropdownMenuTrigger
                render={
                  <Button
                    variant="ghost"
                    size="icon"
                    className="size-7 rounded-full text-moon-500"
                    onPointerDown={(event) => event.stopPropagation()}
                  />
                }
              >
                <MoreHorizontal className="size-3.5" />
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="w-48">
                <DropdownMenuItem onClick={onToggleEnabled}>
                  <Power className="size-4" />
                  {enabled ? "移入禁用区" : "重新启用"}
                </DropdownMenuItem>
                <DropdownMenuItem
                  onClick={onDelete}
                  className="text-status-red focus:text-status-red"
                >
                  <Trash2 className="size-4" />
                  删除账号
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </div>
      </div>
    </article>
  );
}
