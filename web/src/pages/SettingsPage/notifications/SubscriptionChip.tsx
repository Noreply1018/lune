import { useEffect, useMemo, useRef, useState } from "react";
import { ChevronDown, Trash2, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
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
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement | null>(null);
  const [draft, setDraft] = useState<NotificationSubscription>(subscription);
  const [daysDraft, setDaysDraft] = useState(expiringDays);
  const [titleMode, setTitleMode] = useState<"default" | "custom">(
    subscription.title_template?.trim() ? "custom" : "default",
  );
  const [bodyMode, setBodyMode] = useState<"default" | "custom">(
    subscription.body_template?.trim() ? "custom" : "default",
  );

  useEffect(() => {
    setDraft(subscription);
    setTitleMode(subscription.title_template?.trim() ? "custom" : "default");
    setBodyMode(subscription.body_template?.trim() ? "custom" : "default");
  }, [subscription]);

  useEffect(() => {
    setDaysDraft(expiringDays);
  }, [expiringDays]);

  useEffect(() => {
    if (!open) {
      return;
    }
    function handlePointerDown(event: MouseEvent) {
      const path = event.composedPath();
      if (rootRef.current && path.includes(rootRef.current)) {
        return;
      }
      if (
        path.some(
          (item) =>
            item instanceof HTMLElement &&
            typeof item.dataset.slot === "string" &&
            item.dataset.slot.startsWith("select-"),
        )
      ) {
        return;
      }
      applyDraft();
    }
    document.addEventListener("mousedown", handlePointerDown);
    return () => document.removeEventListener("mousedown", handlePointerDown);
  }, [open, draft, daysDraft, titleMode, bodyMode]);

  const eventOptions = useMemo(() => {
    const allowed = new Set(
      usedEvents.filter(
        (item) => item !== subscription.event && item !== DEFAULT_SUBSCRIPTION.event,
      ),
    );
    const options = [
      { event: DEFAULT_SUBSCRIPTION.event, label: "全部事件" },
      ...eventTypes.map((item) => ({ event: item.event, label: item.label })),
    ];
    return options.filter((item) => !allowed.has(item.event));
  }, [eventTypes, subscription.event, usedEvents]);

  const matchedEvent = eventTypes.find((item) => item.event === draft.event);
  const fallbackLabel =
    titleMode === "custom" || bodyMode === "custom"
      ? "渠道默认 / 内置默认"
      : "当前默认";
  const chipTone =
    draft.title_template?.trim() || draft.body_template?.trim()
      ? "border-lunar-300/55 bg-lunar-100/70 text-moon-700"
      : "border-moon-200/60 bg-white/82 text-moon-500 hover:border-moon-250/80 hover:bg-white";

  function applyDraft() {
    const next = normalizeSubscription(draft, titleMode, bodyMode);
    onSave(next);
    if (next.event === "account_expiring" && daysDraft > 0) {
      onSaveExpiringDays(daysDraft);
    }
    setOpen(false);
  }

  return (
    <div ref={rootRef} className="relative">
      <button
        type="button"
        className={cn(
          "inline-flex items-center gap-2 rounded-full border px-3 py-2 text-xs transition",
          open
            ? "border-lunar-300/55 bg-lunar-100/72 text-moon-700"
            : chipTone,
        )}
        onClick={() => {
          if (open) {
            applyDraft();
            return;
          }
          setOpen(true);
        }}
      >
        <span>
          {eventLabel(eventTypes, subscription.event)} ≥
          {subscription.min_severity || "info"} ·{" "}
          {subscription.title_template || subscription.body_template
            ? "已覆盖模板"
            : "使用默认"}
        </span>
        <ChevronDown className="size-3.5" />
      </button>

      {open ? (
        <div className="absolute z-20 mt-3 w-[min(34rem,calc(100vw-3rem))] rounded-[1.45rem] border border-white/80 bg-[linear-gradient(180deg,rgba(255,255,255,0.97),rgba(246,244,249,0.94))] p-4 shadow-[0_32px_80px_-46px_rgba(33,40,63,0.38)]">
          <div className="flex items-start justify-between gap-3">
            <div className="space-y-1">
              <p className="text-sm font-medium text-moon-800">
                {eventLabel(eventTypes, subscription.event)}
              </p>
              <p className="text-xs leading-5 text-moon-400">
                在这里调整事件、严重级别和模板覆盖。点外部、再点 chip 或右上角关闭时会自动保存。
              </p>
            </div>
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              className="rounded-full text-moon-450"
              onClick={applyDraft}
            >
              <X className="size-4" />
            </Button>
          </div>

          <div className="mt-4 grid gap-3 sm:grid-cols-2">
            <label className="space-y-2">
              <span className="text-xs font-medium tracking-[0.16em] text-moon-350">
                EVENT
              </span>
              <Select
                value={draft.event || DEFAULT_SUBSCRIPTION.event}
                onValueChange={(value) =>
                  setDraft((current) => ({
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
                  setDraft((current) => ({
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
            <label className="mt-4 block space-y-2 rounded-[1.1rem] border border-moon-200/55 bg-white/74 px-3 py-3">
              <span className="text-xs font-medium tracking-[0.16em] text-moon-350">
                全局阈值天数
              </span>
              <input
                type="number"
                min={1}
                value={daysDraft}
                onChange={(event) => setDaysDraft(Number(event.target.value))}
                className="h-10 w-full rounded-xl border border-moon-200/65 bg-white px-3 text-sm text-moon-700 outline-none transition focus:border-lunar-300/70"
              />
              <p className="text-xs text-moon-400">
                这是全局参数，只是借这个事件入口集中调整。
              </p>
            </label>
          ) : null}

          <div className="mt-4 space-y-4">
            <TemplateOverrideEditor
              label="标题模板"
              mode={titleMode}
              value={draft.title_template || ""}
              defaultValue={matchedEvent?.default_title_template || ""}
              defaultLabel={fallbackLabel}
              onModeChange={setTitleMode}
              onValueChange={(value) =>
                setDraft((current) => ({
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
              defaultLabel={fallbackLabel}
              onModeChange={setBodyMode}
              onValueChange={(value) =>
                setDraft((current) => ({
                  ...current,
                  body_template: value,
                }))
              }
            />
          </div>

          <div className="mt-4 rounded-[1rem] border border-moon-200/45 bg-moon-50/70 px-3 py-3 text-xs leading-5 text-moon-500">
            当前生效路径：
            {titleMode === "custom" || bodyMode === "custom"
              ? " 订阅覆盖优先；未填写的部分继续回退到渠道默认，再回退到内置默认。"
              : " 当前没有订阅级覆盖，会直接使用渠道默认；渠道默认为空时再回退到内置默认。"}
          </div>

          <div className="mt-4 flex flex-wrap items-center justify-between gap-3 border-t border-moon-200/45 pt-4">
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
            <p className="text-xs text-moon-400">点外部、再点 chip 或右上角关闭时自动保存。</p>
          </div>
        </div>
      ) : null}
    </div>
  );
}

function normalizeSubscription(
  value: NotificationSubscription,
  titleMode: "default" | "custom",
  bodyMode: "default" | "custom",
) {
  return {
    ...value,
    event: value.event?.trim() || DEFAULT_SUBSCRIPTION.event,
    min_severity: value.min_severity || "info",
    title_template:
      titleMode === "custom" ? value.title_template?.trim() || "" : "",
    body_template:
      bodyMode === "custom" ? value.body_template?.trim() || "" : "",
  };
}
