import { Power, RefreshCw, Trash2 } from "lucide-react";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Button } from "@/components/ui/button";
import StatusBadge from "@/components/StatusBadge";
import MiniChat from "@/components/MiniChat";
import { compact, relativeTime } from "@/lib/fmt";
import {
  ensureArray,
  getAccessLabel,
  getAccountHealth,
  getExpiryMeta,
  parseQuotaDisplay,
} from "@/lib/lune";
import type { PoolMember } from "@/lib/types";
import { cn } from "@/lib/utils";

export default function AccountDetailSheet({
  member,
  requests,
  resolveToken,
  onOpenChange,
  onToggleEnabled,
  onDelete,
  onRefreshModels,
}: {
  member: PoolMember | null;
  requests: number;
  resolveToken: () => Promise<string>;
  onOpenChange: (open: boolean) => void;
  onToggleEnabled: () => void;
  onDelete: () => void;
  onRefreshModels: () => void;
}) {
  const account = member?.account ?? null;
  const open = Boolean(member && account);
  const health = account ? getAccountHealth(account) : "unknown";
  const expiry = getExpiryMeta(account?.cpa_expired_at ?? null);
  const quota = parseQuotaDisplay(account?.quota_display ?? "");
  const models = ensureArray(account?.models);

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        className="w-full max-w-[28rem] overflow-y-auto border-l border-moon-200/60 bg-[linear-gradient(180deg,rgba(255,255,255,0.96),rgba(244,241,250,0.92))] sm:max-w-[28rem]"
      >
        {member && account ? (
          <>
            <SheetHeader className="gap-2 px-5 pt-5">
              <p className="eyebrow-label">{getAccessLabel(account)}</p>
              <SheetTitle className="text-[1.2rem] font-semibold tracking-[-0.02em] text-moon-800">
                {account.label}
              </SheetTitle>
              <SheetDescription className="text-moon-500">
                这里是账号的运行时与连通性细节，可直接发起一次测试请求。
              </SheetDescription>
              <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-moon-500">
                <StatusBadge status={health === "unknown" ? "degraded" : health} />
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
                <span className="text-moon-400">
                  最后检查 {relativeTime(account.last_checked_at ?? null)}
                </span>
              </div>
            </SheetHeader>

            <div className="space-y-5 px-5 pb-2">
              <div className="grid gap-3 rounded-[1.2rem] border border-moon-200/55 bg-white/72 p-4 sm:grid-cols-2">
                <DetailItem
                  label="Runtime"
                  value={account.runtime?.base_url || account.base_url || "--"}
                  breakAll
                />
                <DetailItem label="Models" value={models.join(", ") || "--"} />
                <DetailItem label="Notes" value={account.notes || "--"} />
                <DetailItem
                  label="Error"
                  value={account.last_error || "--"}
                  tone={account.last_error ? "danger" : "default"}
                />
              </div>

              <div>
                <MiniChat
                  key={account.id}
                  accountId={account.id}
                  model={models[0]}
                  resolveToken={resolveToken}
                  disabled={!member.enabled}
                />
              </div>

              <div className="flex flex-wrap items-center justify-between gap-2 rounded-[1.2rem] border border-moon-200/55 bg-white/64 px-4 py-3">
                <div className="flex items-center gap-2">
                  <Button variant="outline" size="sm" onClick={onRefreshModels}>
                    <RefreshCw className="size-4" />
                    刷新模型
                  </Button>
                  <Button variant="outline" size="sm" onClick={onToggleEnabled}>
                    <Power className="size-4" />
                    {member.enabled ? "移入禁用区" : "重新启用"}
                  </Button>
                </div>
                <Button
                  variant="ghost"
                  size="sm"
                  className="text-status-red hover:text-status-red"
                  onClick={onDelete}
                >
                  <Trash2 className="size-4" />
                  删除账号
                </Button>
              </div>
            </div>
          </>
        ) : null}
      </SheetContent>
    </Sheet>
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
