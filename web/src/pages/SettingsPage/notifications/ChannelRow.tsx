import { ChevronDown, ChevronUp, RefreshCw } from "lucide-react";

import { Switch } from "@/components/ui/switch";
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
        onClick={onToggleExpand}
      >
        <span
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
            <span>
              {channel.last_delivery
                ? `最近 ${relativeTime(channel.last_delivery.created_at)} ${formatDeliverySummary(channel.last_delivery)}`
                : "尚未投递"}
            </span>
            <span>{channel.enabled ? "Enabled" : "Disabled"}</span>
          </div>
          {channel.recent_deliveries?.length ? (
            <div className="mt-2 flex flex-wrap items-center gap-1.5">
              {channel.recent_deliveries.slice(0, 5).map((item, index) => (
                <span
                  key={`${item.created_at}-${index}`}
                  className="rounded-full border border-moon-200/55 bg-white/82 px-2.5 py-1 text-[11px] text-moon-500"
                >
                  {formatDeliverySummary(item)} · {relativeTime(item.created_at)}
                </span>
              ))}
            </div>
          ) : null}
        </div>
        {expanded ? (
          <ChevronUp className="size-4 shrink-0 text-moon-400" />
        ) : (
          <ChevronDown className="size-4 shrink-0 text-moon-400" />
        )}
      </button>

      <div className="flex shrink-0 items-center gap-2 rounded-full border border-moon-200/60 bg-white/88 px-3 py-1.5">
        {toggling ? (
          <RefreshCw className="size-3.5 animate-spin text-moon-400" />
        ) : null}
        <span className="text-[11px] tracking-[0.14em] text-moon-400">
          Enabled
        </span>
        <Switch
          checked={channel.enabled}
          onCheckedChange={onToggleEnabled}
          disabled={toggling}
        />
      </div>
    </div>
  );
}
