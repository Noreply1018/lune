import { useEffect, useMemo, useRef, useState } from "react";
import {
  ChevronDown,
  MessageSquareText,
  MoreHorizontal,
  Power,
  RefreshCw,
  Trash2,
} from "lucide-react";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Button } from "@/components/ui/button";
import StatusBadge from "@/components/StatusBadge";
import MiniChat from "@/components/MiniChat";
import { compact, relativeTime } from "@/lib/fmt";
import type { PoolMember } from "@/lib/types";
import {
  ensureArray,
  getAccessLabel,
  getAccountHealth,
  getExpiryMeta,
  parseQuotaDisplay,
} from "@/lib/lune";
import { cn } from "@/lib/utils";

export default function AccountCard({
  member,
  requests,
  resolveToken,
  dragging = false,
  compactLayout = false,
  onToggleEnabled,
  onDelete,
  onRefreshModels,
}: {
  member: PoolMember;
  requests: number;
  resolveToken: () => Promise<string>;
  dragging?: boolean;
  compactLayout?: boolean;
  onToggleEnabled: () => void;
  onDelete: () => void;
  onRefreshModels: () => void;
}) {
  const [detailsOpen, setDetailsOpen] = useState(false);
  const [chatOpen, setChatOpen] = useState(false);
  const panelRef = useRef<HTMLDivElement | null>(null);
  const account = member.account;
  const health = account ? getAccountHealth(account) : "error";
  const expiry = getExpiryMeta(account?.cpa_expired_at ?? null);
  const quota = parseQuotaDisplay(account?.quota_display ?? "");
  const models = ensureArray(account?.models);

  const toneClass = useMemo(() => {
    if (!member.enabled) return "border-moon-200/60 bg-moon-100/55";
    if (!account || health === "error") return "border-status-red/20 bg-red-50/60";
    if (health === "degraded") return "border-status-yellow/20 bg-amber-50/60";
    return "border-white/78 bg-white/88";
  }, [account, health, member.enabled]);

  useEffect(() => {
    if (!detailsOpen) {
      return;
    }

    function handlePointerDown(event: MouseEvent) {
      if (panelRef.current?.contains(event.target as Node)) {
        return;
      }
      setDetailsOpen(false);
    }

    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        setDetailsOpen(false);
      }
    }

    window.addEventListener("mousedown", handlePointerDown);
    window.addEventListener("keydown", handleKeyDown);
    return () => {
      window.removeEventListener("mousedown", handlePointerDown);
      window.removeEventListener("keydown", handleKeyDown);
    };
  }, [detailsOpen]);

  if (!account) {
    return (
      <article className="rounded-[1.6rem] border border-status-red/15 bg-red-50/60 p-4 text-sm text-status-red">
        账号数据缺失，当前卡片无法渲染。请刷新页面或检查后端返回。
      </article>
    );
  }

  return (
    <div
      ref={panelRef}
      className={cn(
        "relative min-w-0",
        compactLayout && (detailsOpen || chatOpen) ? "pb-[15.5rem]" : "",
      )}
    >
      <article
          className={cn(
            "relative overflow-hidden rounded-[1.6rem] border shadow-[0_24px_50px_-42px_rgba(33,40,63,0.26)] transition-all duration-200",
          "min-h-[14.6rem] px-4 py-4 sm:px-4.5 sm:py-4.5",
          toneClass,
          dragging ? "scale-[1.015] shadow-[0_32px_90px_-48px_rgba(33,40,63,0.36)]" : "",
        )}
      >
        <div className="pointer-events-none absolute inset-x-0 top-0 h-16 bg-[linear-gradient(180deg,rgba(255,255,255,0.18),transparent)]" />
        <div className="relative flex h-full flex-col">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0 space-y-3">
              <div className="space-y-1.5">
                <p className="eyebrow-label">{getAccessLabel(account)}</p>
                <div className="flex flex-wrap items-center gap-2">
                  <h3
                    className={cn(
                      "truncate font-semibold tracking-[-0.03em] text-moon-800",
                      "text-[1.08rem]",
                    )}
                  >
                    {account.label}
                  </h3>
                  <StatusBadge status={health === "unknown" ? "degraded" : health} />
                </div>
              </div>
              <div className="flex flex-wrap gap-2 text-xs text-moon-500">
                <span className="rounded-full bg-moon-100/85 px-2.5 py-1">{quota}</span>
                <span className="rounded-full bg-moon-100/85 px-2.5 py-1">
                  今日 {compact(requests)} 请求
                </span>
                {expiry ? (
                  <span
                    className={cn(
                      "rounded-full px-2.5 py-1",
                      expiry.tone === "danger"
                        ? "bg-status-red/10 text-status-red"
                        : expiry.tone === "warning"
                          ? "bg-status-yellow/12 text-status-yellow"
                          : "bg-moon-100/80 text-moon-500",
                    )}
                  >
                    {expiry.label}
                  </span>
                ) : null}
              </div>
            </div>

            <div className="flex shrink-0 items-center gap-1.5 self-start">
              <Button
                variant="ghost"
                size="icon"
                className="size-8 rounded-full text-moon-500"
                onClick={() => {
                  setChatOpen(false);
                  setDetailsOpen((current) => !current);
                }}
              >
                <ChevronDown
                  className={cn(
                    "size-4 transition-transform",
                    detailsOpen ? "rotate-180" : "",
                  )}
                />
              </Button>
              <DropdownMenu>
                <DropdownMenuTrigger
                  render={
                    <Button variant="ghost" size="icon" className="size-8 rounded-full text-moon-500" />
                  }
                >
                  <MoreHorizontal className="size-4" />
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-52">
                  <DropdownMenuItem onClick={onRefreshModels}>
                    <RefreshCw className="size-4" />
                    刷新模型
                  </DropdownMenuItem>
                  <DropdownMenuItem onClick={onToggleEnabled}>
                    <Power className="size-4" />
                    {member.enabled ? "移入禁用区" : "重新启用"}
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

          <div className="mt-auto flex flex-wrap items-center justify-between gap-2.5 pt-4">
            <p className="text-xs text-moon-400">
              最后检查 {relativeTime(account.last_checked_at ?? null)}
            </p>
            <Button
              size="sm"
              onClick={() => {
                setDetailsOpen(false);
                setChatOpen((current) => !current);
              }}
            >
              <MessageSquareText className="size-4" />
              {chatOpen ? "收起测试" : "测试"}
            </Button>
          </div>
        </div>

        {chatOpen && !compactLayout ? (
          <div className="mt-3">
            <MiniChat
              accountId={account.id}
              model={models[0]}
              resolveToken={resolveToken}
              disabled={!member.enabled}
            />
          </div>
        ) : null}
      </article>

      {detailsOpen ? (
        <div
          className={cn(
            "absolute z-30 mt-2 overflow-hidden rounded-[1.4rem] border border-white/80 bg-white/96 shadow-[0_30px_70px_-44px_rgba(33,40,63,0.4)] backdrop-blur-xl",
            "left-0 right-0 top-[calc(100%+0.65rem)]",
          )}
        >
          <div className="grid gap-3 px-4 py-4 text-sm text-moon-500 sm:grid-cols-2">
            <DetailItem label="Runtime" value={account.runtime?.base_url || account.base_url || "--"} breakAll />
            <DetailItem label="Models" value={models.join(", ") || "--"} />
            <DetailItem label="Notes" value={account.notes || "--"} />
            <DetailItem label="Error" value={account.last_error || "--"} tone={account.last_error ? "danger" : "default"} />
          </div>
        </div>
      ) : null}

      {chatOpen && compactLayout ? (
        <div className="absolute left-0 right-0 top-[calc(100%+0.65rem)] z-30 rounded-[1.4rem] border border-white/80 bg-white/96 p-3 shadow-[0_30px_70px_-44px_rgba(33,40,63,0.4)] backdrop-blur-xl">
          <MiniChat
            accountId={account.id}
            model={models[0]}
            resolveToken={resolveToken}
            disabled={!member.enabled}
          />
        </div>
      ) : null}
    </div>
  );
}

function DetailItem({
  label,
  value,
  tone = "default",
  breakAll = false,
}: {
  label: string;
  value: string;
  tone?: "default" | "danger";
  breakAll?: boolean;
}) {
  return (
    <div>
      <p className="text-[11px] uppercase tracking-[0.18em] text-moon-350">{label}</p>
      <p
        className={cn(
          "mt-1 text-sm leading-6",
          tone === "danger" ? "text-status-red" : "text-moon-600",
          breakAll ? "break-all" : "",
        )}
      >
        {value}
      </p>
    </div>
  );
}
