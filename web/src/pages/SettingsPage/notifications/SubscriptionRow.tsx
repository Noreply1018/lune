import { useEffect, useMemo, useState } from "react";
import { ChevronDown } from "lucide-react";

import { Switch } from "@/components/ui/switch";
import { cn } from "@/lib/utils";

import TemplateEditor from "./TemplateEditor";
import {
  EVENT_TRIGGER_DESCRIPTION,
  placeholdersForEvent,
  severityTone,
  toDisplay,
  toStorage,
  type NotificationEventType,
  type NotificationSubscription,
} from "./types";

type SubscriptionRowProps = {
  subscription: NotificationSubscription;
  eventType: NotificationEventType;
  savingField: string | null;
  titleError?: string | null;
  bodyError?: string | null;
  onCommit: (
    field: "subscribed" | "title" | "body",
    next: NotificationSubscription,
  ) => void;
  onClearFieldError: (field: "title" | "body") => void;
};

export default function SubscriptionRow({
  subscription,
  eventType,
  savingField,
  titleError,
  bodyError,
  onCommit,
  onClearFieldError,
}: SubscriptionRowProps) {
  const placeholders = useMemo(
    () => placeholdersForEvent(eventType),
    [eventType],
  );

  const [expanded, setExpanded] = useState(false);
  const [titleDisplay, setTitleDisplay] = useState(() =>
    toDisplay(subscription.title_template, placeholders),
  );
  const [bodyDisplay, setBodyDisplay] = useState(() =>
    toDisplay(subscription.body_template, placeholders),
  );

  useEffect(() => {
    setTitleDisplay(toDisplay(subscription.title_template, placeholders));
    setBodyDisplay(toDisplay(subscription.body_template, placeholders));
  }, [subscription.title_template, subscription.body_template, placeholders]);

  const defaultTitleDisplay = useMemo(
    () => toDisplay(eventType.default_title_template, placeholders),
    [eventType.default_title_template, placeholders],
  );
  const defaultBodyDisplay = useMemo(
    () => toDisplay(eventType.default_body_template, placeholders),
    [eventType.default_body_template, placeholders],
  );

  function commitTitle() {
    const stored = toStorage(titleDisplay, placeholders);
    if (stored === subscription.title_template) {
      return;
    }
    onCommit("title", { ...subscription, title_template: stored });
  }

  function commitBody() {
    const stored = toStorage(bodyDisplay, placeholders);
    if (stored === subscription.body_template) {
      return;
    }
    onCommit("body", { ...subscription, body_template: stored });
  }

  const triggerDescription =
    EVENT_TRIGGER_DESCRIPTION[eventType.event] ?? "内置事件";

  return (
    <div
      className={cn(
        "overflow-hidden rounded-[1.1rem] border border-white/75 bg-white/75",
        expanded ? "shadow-[0_20px_48px_-36px_rgba(33,40,63,0.24)]" : "",
      )}
    >
      <button
        type="button"
        className="flex w-full items-center gap-3 px-4 py-3 text-left transition hover:bg-moon-50/70"
        onClick={() => setExpanded((current) => !current)}
      >
        <span
          className={cn(
            "inline-block size-2 rounded-full",
            subscription.subscribed ? "bg-status-green" : "bg-moon-300",
          )}
          aria-hidden
        />
        <div className="flex flex-1 flex-col">
          <span className="text-sm font-medium text-moon-800">
            {eventType.label}
          </span>
          <span className="text-xs text-moon-400">{eventType.event}</span>
        </div>
        <span
          className={cn(
            "inline-flex rounded-full px-2 py-1 text-[11px]",
            severityTone(eventType.default_severity),
          )}
        >
          {eventType.default_severity}
        </span>
        <Switch
          checked={subscription.subscribed}
          disabled={savingField === "subscribed"}
          onClick={(event) => event.stopPropagation()}
          onCheckedChange={(checked) =>
            onCommit("subscribed", { ...subscription, subscribed: checked })
          }
        />
        <ChevronDown
          className={cn(
            "size-4 text-moon-400 transition-transform",
            expanded ? "rotate-180" : "",
          )}
        />
      </button>

      {expanded ? (
        <div className="space-y-4 border-t border-moon-200/40 bg-moon-50/40 px-4 py-4">
          <p className="text-xs leading-5 text-moon-400">
            触发时机：{triggerDescription}。严重级别由后端生成，无法在前端修改。
          </p>
          {!subscription.subscribed ? (
            <div className="rounded-[0.75rem] border border-amber-200/70 bg-amber-50/85 px-3 py-1.5 text-xs text-amber-800">
              当前未订阅：保存后该事件不会发送，但配置仍会保留。
            </div>
          ) : null}
          <TemplateEditor
            label="标题"
            value={titleDisplay}
            defaultValue={defaultTitleDisplay}
            placeholders={placeholders}
            disabled={savingField === "title"}
            minRows={2}
            onChange={setTitleDisplay}
            onCommit={commitTitle}
            onReset={() => onClearFieldError("title")}
            error={titleError ?? null}
          />
          <TemplateEditor
            label="正文"
            value={bodyDisplay}
            defaultValue={defaultBodyDisplay}
            placeholders={placeholders}
            disabled={savingField === "body"}
            minRows={3}
            onChange={setBodyDisplay}
            onCommit={commitBody}
            onReset={() => onClearFieldError("body")}
            error={bodyError ?? null}
          />
        </div>
      ) : null}
    </div>
  );
}
