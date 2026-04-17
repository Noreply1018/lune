import { Plus, RefreshCw } from "lucide-react";

import { Button } from "@/components/ui/button";

import SubscriptionChip from "./SubscriptionChip";
import { DEFAULT_SUBSCRIPTION, eventLabel } from "./types";
import type {
  NotificationEventType,
  NotificationSubscription,
} from "./types";

export default function SubscriptionList({
  subscriptions,
  eventTypes,
  expiringDays,
  saving,
  onChange,
  onSaveExpiringDays,
}: {
  subscriptions: NotificationSubscription[];
  eventTypes: NotificationEventType[];
  expiringDays: number;
  saving?: boolean;
  onChange: (value: NotificationSubscription[]) => void;
  onSaveExpiringDays: (value: number) => void;
}) {
  const currentEvents = subscriptions.map((item) => item.event);
  const nextAddEvent =
    eventTypes.find((item) => !currentEvents.includes(item.event))?.event ?? null;

  return (
    <section className="space-y-3">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <p className="text-sm font-medium text-moon-800">订阅事件</p>
          <p className="text-xs leading-5 text-moon-400">
            以 chip 管理事件、最低严重级别和模板覆盖；关闭编辑层时自动保存。
          </p>
        </div>
        <Button
          variant="outline"
          className="rounded-full"
          onClick={() =>
            nextAddEvent
              ? onChange([
                  ...subscriptions,
                  {
                    ...DEFAULT_SUBSCRIPTION,
                    event: nextAddEvent,
                  },
                ])
              : undefined
          }
          disabled={!nextAddEvent}
        >
          <Plus className="size-4" />
          Add Event
        </Button>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        {subscriptions.map((item, index) => (
          <SubscriptionChip
            key={`${item.event}-${index}`}
            subscription={item}
            eventTypes={eventTypes}
            usedEvents={subscriptions.map((entry) => entry.event)}
            expiringDays={expiringDays}
            onSave={(value) =>
              onChange(
                subscriptions.map((entry, entryIndex) =>
                  entryIndex === index ? value : entry,
                ),
              )
            }
            onRemove={() =>
              onChange(
                subscriptions.length === 1
                  ? [DEFAULT_SUBSCRIPTION]
                  : subscriptions.filter((_, entryIndex) => entryIndex !== index),
              )
            }
            onSaveExpiringDays={onSaveExpiringDays}
          />
        ))}
        {saving ? <RefreshCw className="size-4 animate-spin text-moon-350" /> : null}
      </div>
      <div className="text-xs leading-5 text-moon-400">
        {subscriptions.map((item) => eventLabel(eventTypes, item.event)).join(" / ")}
      </div>
    </section>
  );
}
