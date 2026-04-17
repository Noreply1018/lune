import SubscriptionRow from "./SubscriptionRow";
import type {
  NotificationEventType,
  NotificationSubscription,
} from "./types";

type SubscriptionsTableProps = {
  subscriptions: NotificationSubscription[];
  eventTypes: NotificationEventType[];
  savingEvent: string | null;
  savingField: Record<string, string | null>;
  fieldErrors: Record<string, { title?: string | null; body?: string | null }>;
  onCommit: (
    event: string,
    field: "subscribed" | "title" | "body",
    next: NotificationSubscription,
  ) => void;
};

export default function SubscriptionsTable({
  subscriptions,
  eventTypes,
  savingField,
  fieldErrors,
  onCommit,
}: SubscriptionsTableProps) {
  const byEvent = new Map(subscriptions.map((item) => [item.event, item]));

  return (
    <section className="space-y-3">
      <header className="flex items-center justify-between gap-3">
        <div>
          <p className="text-sm font-medium text-moon-800">订阅事件</p>
          <p className="text-xs leading-5 text-moon-400">
            每行一个事件；订阅开关独立，标题/正文保存即生效。严重级别由后端决定。
          </p>
        </div>
      </header>
      <div className="space-y-3">
        {eventTypes.map((eventType) => {
          const sub = byEvent.get(eventType.event);
          if (!sub) {
            return null;
          }
          return (
            <SubscriptionRow
              key={eventType.event}
              subscription={sub}
              eventType={eventType}
              savingField={savingField[eventType.event] ?? null}
              titleError={fieldErrors[eventType.event]?.title ?? null}
              bodyError={fieldErrors[eventType.event]?.body ?? null}
              onCommit={(field, next) => onCommit(eventType.event, field, next)}
            />
          );
        })}
      </div>
    </section>
  );
}
