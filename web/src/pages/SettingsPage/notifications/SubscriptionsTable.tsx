import SubscriptionRow from "./SubscriptionRow";
import type {
  NotificationEventType,
  NotificationSubscription,
} from "./types";

type SubscriptionsTableProps = {
  subscriptions: NotificationSubscription[];
  eventTypes: NotificationEventType[];
  savingField: Record<string, string | null>;
  fieldErrors: Record<string, { body?: string | null }>;
  onCommit: (
    event: string,
    field: "subscribed" | "body",
    next: NotificationSubscription,
  ) => void;
  onClearFieldError: (event: string, field: "body") => void;
};

export default function SubscriptionsTable({
  subscriptions,
  eventTypes,
  savingField,
  fieldErrors,
  onCommit,
  onClearFieldError,
}: SubscriptionsTableProps) {
  const byEvent = new Map(subscriptions.map((item) => [item.event, item]));

  return (
    <section className="space-y-3">
      <header className="flex items-center justify-between gap-3">
        <div>
          <p className="text-sm font-medium text-moon-800">订阅事件</p>
          <p className="text-xs leading-5 text-moon-400">
            每行一个事件；订阅开关独立，正文保存即生效。标题固定为「Lune 通知：事件中文名」，展开后可查看具体标题。
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
              bodyError={fieldErrors[eventType.event]?.body ?? null}
              onCommit={(field, next) => onCommit(eventType.event, field, next)}
              onClearFieldError={(field) =>
                onClearFieldError(eventType.event, field)
              }
            />
          );
        })}
      </div>
    </section>
  );
}
