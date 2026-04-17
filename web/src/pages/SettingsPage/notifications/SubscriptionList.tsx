import { Plus, RefreshCw } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";

import SubscriptionChip from "./SubscriptionChip";
import { DEFAULT_SUBSCRIPTION, eventLabel, normalizeSubscriptions } from "./types";
import type {
  NotificationEventType,
  NotificationSubscription,
} from "./types";

export default function SubscriptionList({
  subscriptions,
  eventTypes,
  expiringDays,
  expiringDaysError,
  saving,
  onChange,
  onSaveExpiringDays,
}: {
  subscriptions: NotificationSubscription[];
  eventTypes: NotificationEventType[];
  expiringDays: number;
  expiringDaysError?: string | null;
  saving?: boolean;
  onChange: (value: NotificationSubscription[]) => void;
  onSaveExpiringDays: (value: number) => void;
}) {
  const currentEvents = subscriptions.map((item) => item.event);
  const nextAddEvent =
    eventTypes.find((item) => !currentEvents.includes(item.event))?.event ??
    (!currentEvents.includes(DEFAULT_SUBSCRIPTION.event)
      ? DEFAULT_SUBSCRIPTION.event
      : null);

  return (
    <section className="space-y-3">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <p className="text-sm font-medium text-moon-800">订阅事件</p>
          <p className="text-xs leading-5 text-moon-400">
            以 chip 管理事件、最低严重级别和模板覆盖。允许留空；留空时该 channel 只保留配置，不接收任何事件。
          </p>
        </div>
        {nextAddEvent ? (
          <Button
            variant="outline"
            className="rounded-full"
            onClick={() =>
              onChange([
                ...subscriptions,
                {
                  ...DEFAULT_SUBSCRIPTION,
                  event: nextAddEvent,
                },
              ])
            }
          >
            <Plus className="size-4" />
            Add Event
          </Button>
        ) : (
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  variant="outline"
                  className="rounded-full"
                  disabled
                  aria-label="已为所有事件添加订阅"
                >
                  <Plus className="size-4" />
                  Add Event
                </Button>
              }
            />
            <TooltipContent>已为所有事件添加订阅</TooltipContent>
          </Tooltip>
        )}
      </div>
      <div className="flex flex-wrap items-center gap-2">
        {subscriptions.length ? subscriptions.map((item, index) => (
          <SubscriptionChip
            key={`${item.event}-${index}`}
            subscription={item}
            eventTypes={eventTypes}
            usedEvents={subscriptions.map((entry) => entry.event)}
            expiringDays={expiringDays}
            onSave={(value) =>
              onChange(
                normalizeSubscriptions(
                  subscriptions.map((entry, entryIndex) =>
                    entryIndex === index ? value : entry,
                  ),
                ),
              )
            }
            onRemove={() =>
              onChange(
                subscriptions.filter((_, entryIndex) => entryIndex !== index),
              )
            }
            onSaveExpiringDays={onSaveExpiringDays}
          />
        )) : (
          <div className="rounded-[1rem] border border-dashed border-moon-200/60 px-3 py-3 text-sm text-moon-450">
            当前没有事件订阅。这个 channel 不会接收任何通知，直到你添加一条订阅。
          </div>
        )}
        {saving ? <RefreshCw className="size-4 animate-spin text-moon-350" /> : null}
      </div>
      <div className="text-xs leading-5 text-moon-400">
        {subscriptions.length
          ? subscriptions.map((item) => eventLabel(eventTypes, item.event)).join(" / ")
          : "无订阅"}
      </div>
      {expiringDaysError ? (
        <div className="rounded-[0.95rem] border border-status-red/18 bg-status-red/6 px-3 py-2 text-xs text-status-red">
          {expiringDaysError}
        </div>
      ) : null}
    </section>
  );
}
