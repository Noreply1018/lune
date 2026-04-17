import { GripVertical, Info, MoreHorizontal, Power, RefreshCw, Trash2 } from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Button } from "@/components/ui/button";
import StatusBadge from "@/components/StatusBadge";
import { compact, relativeTime } from "@/lib/fmt";
import type { PoolMember } from "@/lib/types";
import {
  getAccessLabel,
  getAccountHealth,
  getExpiryMeta,
  parseQuotaDisplay,
} from "@/lib/lune";
import { cn } from "@/lib/utils";

type Variant = "active" | "disabled";

export default function AccountCard({
  member,
  requests,
  variant,
  priorityIndex,
  dragging = false,
  onOpenDetails,
  onToggleEnabled,
  onDelete,
  onRefreshModels,
  dragHandleProps,
}: {
  member: PoolMember;
  requests: number;
  variant: Variant;
  priorityIndex?: number;
  dragging?: boolean;
  onOpenDetails: () => void;
  onToggleEnabled: () => void;
  onDelete: () => void;
  onRefreshModels: () => void;
  dragHandleProps?: React.HTMLAttributes<HTMLDivElement>;
}) {
  const account = member.account;
  const health = account ? getAccountHealth(account) : "error";
  const expiry = getExpiryMeta(account?.cpa_expired_at ?? null);
  const quota = parseQuotaDisplay(account?.quota_display ?? "");
  const enabled = member.enabled;

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
      )}
    >
      <div className="pointer-events-none absolute inset-x-0 top-0 h-10 bg-[linear-gradient(180deg,rgba(255,255,255,0.18),transparent)]" />

      <div
        className={cn(
          "pointer-events-none absolute inset-y-0 left-0 z-0 flex w-5 items-center justify-center text-moon-300 opacity-0 transition-opacity",
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
            "absolute right-2 top-2 z-10 flex size-5 items-center justify-center rounded-full bg-moon-100/90 text-[10px] font-semibold text-moon-500",
            !enabled ? "opacity-60" : "",
          )}
          title={`当前优先级序号 ${priorityIndex}`}
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
            <StatusBadge status={health === "unknown" ? "degraded" : health} />
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-1.5 text-[11px] text-moon-500">
          <span className="rounded-full bg-moon-100/80 px-2 py-0.5">{quota}</span>
          <span className="rounded-full bg-moon-100/80 px-2 py-0.5">
            今日 {compact(requests)}
          </span>
          {expiry ? (
            <span
              className={cn(
                "rounded-full px-2 py-0.5",
                expiry.tone === "danger"
                  ? "bg-status-red/10 text-status-red"
                  : expiry.tone === "warning"
                    ? "bg-status-yellow/12 text-status-yellow"
                    : "bg-moon-100/70 text-moon-500",
              )}
            >
              {expiry.label}
            </span>
          ) : null}
        </div>

        <div className="mt-auto flex items-center justify-between gap-2 pt-1">
          <p className="truncate text-[10.5px] text-moon-400">
            {relativeTime(account.last_checked_at ?? null)}
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
                <DropdownMenuItem onClick={onOpenDetails}>
                  <Info className="size-4" />
                  查看详情
                </DropdownMenuItem>
                <DropdownMenuItem onClick={onRefreshModels}>
                  <RefreshCw className="size-4" />
                  刷新模型
                </DropdownMenuItem>
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
