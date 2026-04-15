import { useMemo, useState } from "react";
import {
  ChevronDown,
  ChevronsUpDown,
  Eye,
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
import { getAccessLabel, getAccountHealth, getExpiryMeta, parseQuotaDisplay } from "@/lib/lune";
import { cn } from "@/lib/utils";

export default function AccountCard({
  member,
  requests,
  globalToken,
  dragging = false,
  onToggleEnabled,
  onDelete,
  onRefreshModels,
  onViewModels,
}: {
  member: PoolMember;
  requests: number;
  globalToken: string;
  dragging?: boolean;
  onToggleEnabled: () => void;
  onDelete: () => void;
  onRefreshModels: () => void;
  onViewModels: () => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const [chatOpen, setChatOpen] = useState(false);
  const account = member.account;

  const health = getAccountHealth(account!);
  const expiry = getExpiryMeta(account?.cpa_expired_at ?? null);
  const quota = parseQuotaDisplay(account?.quota_display ?? "");
  const models = account?.models ?? [];

  const toneClass = useMemo(() => {
    if (!member.enabled) return "border-moon-200/60 bg-moon-100/55";
    if (health === "error") return "border-status-red/20 bg-red-50/60";
    if (health === "degraded") return "border-status-yellow/20 bg-amber-50/60";
    return "border-white/78 bg-white/88";
  }, [health, member.enabled]);

  return (
    <article
      className={cn(
        "rounded-[1.6rem] border p-4 shadow-[0_24px_50px_-42px_rgba(33,40,63,0.26)] transition-all duration-200",
        toneClass,
        dragging ? "scale-[1.02] shadow-[0_32px_90px_-48px_rgba(33,40,63,0.36)]" : "",
      )}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="space-y-3">
          <div className="space-y-1">
            <p className="eyebrow-label">{getAccessLabel(account!)}</p>
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="text-lg font-semibold tracking-[-0.03em] text-moon-800">
                {account?.label}
              </h3>
              <StatusBadge status={health === "unknown" ? "degraded" : health} />
            </div>
          </div>
          <div className="flex flex-wrap gap-2 text-xs text-moon-500">
            <span className="rounded-full bg-moon-100/80 px-2.5 py-1">{quota}</span>
            <span className="rounded-full bg-moon-100/80 px-2.5 py-1">
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

        <div className="flex items-center gap-2">
          <Button
            variant="ghost"
            size="icon"
            className="size-8 rounded-full"
            onClick={() => setExpanded((current) => !current)}
          >
            {expanded ? <ChevronDown className="size-4 rotate-180" /> : <ChevronsUpDown className="size-4" />}
          </Button>
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <Button variant="ghost" size="icon" className="size-8 rounded-full" />
              }
            >
              <MoreHorizontal className="size-4" />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-52">
              <DropdownMenuItem onClick={onViewModels}>
                <Eye className="size-4" />
                查看模型列表
              </DropdownMenuItem>
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

      <div className="mt-4 flex flex-wrap items-center gap-2">
        <Button size="sm" onClick={() => setChatOpen((current) => !current)}>
          <MessageSquareText className="size-4" />
          {chatOpen ? "收起测试" : "测试"}
        </Button>
        <p className="text-xs text-moon-400">
          最后检查 {relativeTime(account?.last_checked_at ?? null)}
        </p>
      </div>

      {expanded ? (
        <div className="mt-4 grid gap-3 border-t border-moon-200/50 pt-4 text-sm text-moon-500 sm:grid-cols-2">
          <div>
            <p className="text-xs uppercase tracking-[0.18em] text-moon-400">Runtime</p>
            <p className="mt-1 break-all">{account?.runtime?.base_url || account?.base_url || "--"}</p>
          </div>
          <div>
            <p className="text-xs uppercase tracking-[0.18em] text-moon-400">Models</p>
            <p className="mt-1">{models.slice(0, 4).join(", ") || "--"}</p>
          </div>
          <div>
            <p className="text-xs uppercase tracking-[0.18em] text-moon-400">Notes</p>
            <p className="mt-1">{account?.notes || "--"}</p>
          </div>
          <div>
            <p className="text-xs uppercase tracking-[0.18em] text-moon-400">Error</p>
            <p className="mt-1">{account?.last_error || "--"}</p>
          </div>
        </div>
      ) : null}

      {chatOpen ? (
        <div className="mt-4">
          <MiniChat
            accountId={account!.id}
            model={models[0]}
            globalToken={globalToken}
            disabled={!member.enabled}
          />
        </div>
      ) : null}
    </article>
  );
}
