import { ChevronDown, ChevronUp, RefreshCw } from "lucide-react";

import { Switch } from "@/components/ui/switch";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { relativeTime } from "@/lib/fmt";
import { cn } from "@/lib/utils";

import {
  CHANNEL_TYPE_META,
  deliveryTone,
  formatDeliverySummary,
} from "./types";
import type { NotificationChannelDraft } from "./types";

export default function ChannelRow({
  channel,
  expanded,
  toggling,
  onToggleExpand,
  onToggleEnabled,
}: {
  channel: NotificationChannelDraft;
  expanded: boolean;
  toggling?: boolean;
  onToggleExpand: () => void;
  onToggleEnabled: (enabled: boolean) => void;
}) {
  return (
    <div className="flex min-w-0 items-center gap-3 px-4 py-4 sm:px-5">
      <button
        type="button"
        className="flex min-w-0 flex-1 items-center gap-3 text-left"
        aria-label={`${channel.name}，${expanded ? "收起通知详情" : "展开通知详情"}`}
        onClick={onToggleExpand}
      >
        <span
          aria-hidden="true"
          className={cn(
            "size-2.5 shrink-0 rounded-full shadow-[0_0_0_4px_rgba(255,255,255,0.7)]",
            deliveryTone(channel.last_delivery),
          )}
        />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <p className="truncate text-sm font-medium text-moon-800">
              {channel.name}
            </p>
            <span
              className={cn(
                "rounded-full px-2.5 py-1 text-[11px] tracking-[0.14em]",
                CHANNEL_TYPE_META[channel.type].tone,
              )}
            >
              {CHANNEL_TYPE_META[channel.type].label}
            </span>
          </div>
          <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-moon-450">
            <span aria-hidden="true">
              {channel.last_delivery
                ? `最近 ${relativeTime(channel.last_delivery.created_at)} · ${formatDeliverySummary(channel.last_delivery)}`
                : "尚未投递"}
            </span>
          </div>
          {channel.recent_deliveries?.length ? (
            <div className="mt-2 flex flex-wrap items-center gap-1.5">
              {channel.recent_deliveries.slice(0, 5).map((item, index) => (
                <Tooltip key={`${item.created_at}-${index}`}>
                  <TooltipTrigger
                    render={
                      <span
                        aria-hidden="true"
                        className={cn(
                          "size-2.5 rounded-full border border-white/80 shadow-[0_0_0_4px_rgba(255,255,255,0.62)]",
                          deliveryTone(item),
                        )}
                      />
                    }
                  />
                  <TooltipContent>
                    {formatDeliverySummary(item)} · {relativeTime(item.created_at)}
                  </TooltipContent>
                </Tooltip>
              ))}
            </div>
          ) : null}
        </div>
        {expanded ? (
          <ChevronUp className="size-4 shrink-0 text-moon-400" aria-hidden="true" />
        ) : (
          <ChevronDown className="size-4 shrink-0 text-moon-400" aria-hidden="true" />
        )}
      </button>

      <div className="flex shrink-0 items-center gap-2">
        {toggling ? (
          <RefreshCw className="size-3.5 animate-spin text-moon-400" />
        ) : null}
        <span
          aria-hidden="true"
          className={cn(
            "text-[11px] font-medium tracking-[0.14em]",
            channel.enabled ? "text-emerald-600" : "text-moon-450",
          )}
        >
          {channel.enabled ? "ON" : "OFF"}
        </span>
        <Switch
          checked={channel.enabled}
          onCheckedChange={onToggleEnabled}
          disabled={toggling}
          aria-label={channel.enabled ? "停用通知渠道" : "启用通知渠道"}
        />
      </div>
    </div>
  );
}
