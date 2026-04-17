import { useEffect, useMemo, useRef, useState } from "react";
import { ChevronDown, Trash2 } from "lucide-react";

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

import TemplateOverrideEditor from "./TemplateOverrideEditor";
import {
  DEFAULT_SUBSCRIPTION,
  eventLabel,
  SEVERITY_OPTIONS,
  type NotificationEventType,
  type NotificationSubscription,
} from "./types";

export default function SubscriptionChip({
  subscription,
  eventTypes,
  usedEvents,
  expiringDays,
  onSave,
  onRemove,
  onSaveExpiringDays,
}: {
  subscription: NotificationSubscription;
  eventTypes: NotificationEventType[];
  usedEvents: string[];
  expiringDays: number;
  onSave: (value: NotificationSubscription) => void;
  onRemove: () => void;
  onSaveExpiringDays: (value: number) => void;
}) {
  const ref = useRef<HTMLDivElement | null>(null);
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState<NotificationSubscription>(subscription);
  const [daysDraft, setDaysDraft] = useState(expiringDays);

  useEffect(() => {
    setDraft(subscription);
  }, [subscription]);

  useEffect(() => {
    setDaysDraft(expiringDays);
  }, [expiringDays]);

  useEffect(() => {
    if (!open) {
      return;
    }
    function handlePointerDown(event: MouseEvent) {
      if (!ref.current?.contains(event.target as Node)) {
        closeAndSave();
      }
    }
    document.addEventListener("mousedown", handlePointerDown);
    return () => document.removeEventListener("mousedown", handlePointerDown);
  }, [daysDraft, draft, onSave, onSaveExpiringDays, open]);

  const eventOptions = useMemo(() => {
    const allowed = new Set(usedEvents.filter((item) => item !== subscription.event));
    return eventTypes.filter((item) => !allowed.has(item.event));
  }, [eventTypes, subscription.event, usedEvents]);

  const matchedEvent = eventTypes.find((item) => item.event === draft.event);
  const titleMode =
    draft.title_template && draft.title_template.trim() ? "custom" : "default";
  const bodyMode =
    draft.body_template && draft.body_template.trim() ? "custom" : "default";

  function closeAndSave() {
    onSave(normalizeSubscription(draft));
    if (draft.event === "account_expiring" && daysDraft > 0) {
      onSaveExpiringDays(daysDraft);
    }
    setOpen(false);
  }

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        className={cn(
          "inline-flex items-center gap-2 rounded-full border px-3 py-2 text-xs transition",
          open
            ? "border-lunar-300/55 bg-lunar-100/70 text-moon-700"
            : "border-moon-200/60 bg-white/82 text-moon-500 hover:border-moon-250/80 hover:bg-white",
        )}
        onClick={() => {
          if (open) {
            closeAndSave();
            return;
          }
          setOpen(true);
        }}
      >
        <span>
          {eventLabel(eventTypes, subscription.event)} ≥
          {subscription.min_severity || "info"} ·{" "}
          {subscription.title_template || subscription.body_template
            ? "模板覆盖"
            : "默认"}
        </span>
        <ChevronDown className="size-3.5" />
      </button>

      {open ? (
        <div className="absolute z-20 mt-3 w-[min(32rem,calc(100vw-3rem))] rounded-[1.35rem] border border-white/75 bg-white/96 p-4 shadow-[0_28px_70px_-48px_rgba(33,40,63,0.42)]">
          <div className="grid gap-3 sm:grid-cols-2">
            <label className="space-y-2">
              <span className="text-xs font-medium tracking-[0.16em] text-moon-350">
                EVENT
              </span>
              <Select
                value={draft.event || DEFAULT_SUBSCRIPTION.event}
                onValueChange={(value) =>
                  setDraft((current: NotificationSubscription) => ({
                    ...current,
                    event: value ?? DEFAULT_SUBSCRIPTION.event,
                  }))
                }
              >
                <SelectTrigger className="h-10 rounded-xl border-moon-200/65 bg-white/82">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {eventOptions.map((item) => (
                    <SelectItem key={item.event} value={item.event}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </label>
            <label className="space-y-2">
              <span className="text-xs font-medium tracking-[0.16em] text-moon-350">
                MIN SEVERITY
              </span>
              <Select
                value={draft.min_severity || "info"}
                onValueChange={(value) =>
                  setDraft((current: NotificationSubscription) => ({
                    ...current,
                    min_severity: (value ??
                      "info") as NotificationSubscription["min_severity"],
                  }))
                }
              >
                <SelectTrigger className="h-10 rounded-xl border-moon-200/65 bg-white/82">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {SEVERITY_OPTIONS.map((item) => (
                    <SelectItem key={item} value={item}>
                      {item}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </label>
          </div>

          {draft.event === "account_expiring" ? (
            <label className="mt-4 block space-y-2">
              <span className="text-xs font-medium tracking-[0.16em] text-moon-350">
                全局阈值天数
              </span>
              <input
                type="number"
                min={1}
                value={daysDraft}
                onChange={(event) => setDaysDraft(Number(event.target.value))}
                onBlur={() => {
                  if (daysDraft > 0) {
                    onSaveExpiringDays(daysDraft);
                  }
                }}
                className="h-10 w-full rounded-xl border border-moon-200/65 bg-white/82 px-3 text-sm text-moon-700 outline-none"
              />
              <p className="text-xs text-moon-400">全局共用，只在这里 inline 调整。</p>
            </label>
          ) : null}

          <div className="mt-4 space-y-4">
            <TemplateOverrideEditor
              label="标题模板"
              mode={titleMode}
              value={draft.title_template || ""}
              defaultValue={matchedEvent?.default_title_template || ""}
              onModeChange={(value) =>
                setDraft((current: NotificationSubscription) => ({
                  ...current,
                  title_template: value === "custom" ? current.title_template || "" : "",
                }))
              }
              onValueChange={(value) =>
                setDraft((current: NotificationSubscription) => ({
                  ...current,
                  title_template: value,
                }))
              }
            />
            <TemplateOverrideEditor
              label="正文模板"
              mode={bodyMode}
              value={draft.body_template || ""}
              defaultValue={matchedEvent?.default_body_template || ""}
              onModeChange={(value) =>
                setDraft((current: NotificationSubscription) => ({
                  ...current,
                  body_template: value === "custom" ? current.body_template || "" : "",
                }))
              }
              onValueChange={(value) =>
                setDraft((current: NotificationSubscription) => ({
                  ...current,
                  body_template: value,
                }))
              }
            />
          </div>

          <div className="mt-4 flex items-center justify-between border-t border-moon-200/45 pt-4">
            <Button
              variant="outline"
              className="rounded-full text-status-red"
              onClick={() => {
                onRemove();
                setOpen(false);
              }}
            >
              <Trash2 className="size-4" />
              Remove
            </Button>
            <p className="text-xs text-moon-400">关闭编辑层时自动保存。</p>
          </div>
        </div>
      ) : null}
    </div>
  );
}

function normalizeSubscription(value: NotificationSubscription) {
  return {
    ...value,
    event: value.event?.trim() || DEFAULT_SUBSCRIPTION.event,
    min_severity: value.min_severity || "info",
    title_template: value.title_template?.trim() || "",
    body_template: value.body_template?.trim() || "",
  };
}
