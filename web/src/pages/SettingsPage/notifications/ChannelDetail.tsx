import { useEffect, useState } from "react";

import { api, ApiError } from "@/lib/api";
import { toast } from "@/components/Feedback";

import BasicConfigForm from "./BasicConfigForm";
import DangerZone from "./DangerZone";
import PreviewPanel, { type PreviewResult } from "./PreviewPanel";
import RetryConfigEditor from "./RetryConfigEditor";
import SubscriptionList from "./SubscriptionList";
import TemplateOverrideEditor from "./TemplateOverrideEditor";
import TestPanel, { type TestResult } from "./TestPanel";
import { parseRetryInput, retryInputValue } from "./types";
import type {
  NotificationChannelDraft,
  NotificationEventType,
  NotificationSeverity,
} from "./types";

export default function ChannelDetail({
  draft,
  eventTypes,
  expiringDays,
  savingField,
  error,
  onDraftChange,
  onCommit,
  onSaveExpiringDays,
  onDelete,
  onRefresh,
}: {
  draft: NotificationChannelDraft;
  eventTypes: NotificationEventType[];
  expiringDays: number;
  savingField: string | null;
  error: string | null;
  onDraftChange: (next: NotificationChannelDraft) => void;
  onCommit: (field: string, next?: NotificationChannelDraft) => void;
  onSaveExpiringDays: (value: number) => void;
  onDelete: () => void;
  onRefresh: () => Promise<void>;
}) {
  const [dangerOpen, setDangerOpen] = useState(false);
  const [testLoading, setTestLoading] = useState(false);
  const [testResult, setTestResult] = useState<TestResult | null>(null);
  const [previewEvent, setPreviewEvent] = useState(
    eventTypes[0]?.event ?? "account_error",
  );
  const [previewSeverity, setPreviewSeverity] =
    useState<NotificationSeverity>("critical");
  const [previewLoading, setPreviewLoading] = useState(false);
  const [previewResult, setPreviewResult] = useState<PreviewResult | null>(null);
  const [retryInput, setRetryInput] = useState(
    retryInputValue(draft.retry_schedule_seconds),
  );

  useEffect(() => {
    setRetryInput(retryInputValue(draft.retry_schedule_seconds));
  }, [draft.retry_schedule_seconds]);

  useEffect(() => {
    const matched = eventTypes.find((item) => item.event === previewEvent);
    if (!matched && eventTypes[0]) {
      setPreviewEvent(eventTypes[0].event);
      setPreviewSeverity(
        (eventTypes[0].default_severity as NotificationSeverity) || "critical",
      );
      return;
    }
    if (matched && previewSeverity === "critical") {
      return;
    }
    if (matched?.default_severity) {
      setPreviewSeverity(matched.default_severity as NotificationSeverity);
    }
  }, [eventTypes, previewEvent, previewSeverity]);

  return (
    <div className="space-y-6 px-4 pt-1 pb-5 sm:px-5">
      <BasicConfigForm
        draft={draft}
        savingField={savingField}
        onDraftChange={onDraftChange}
        onCommit={onCommit}
      />

      <SubscriptionList
        subscriptions={draft.subscriptions}
        eventTypes={eventTypes}
        expiringDays={expiringDays}
        saving={savingField === "subscriptions"}
        onChange={(value) => {
          const next = { ...draft, subscriptions: value };
          onDraftChange(next);
          onCommit("subscriptions", next);
        }}
        onSaveExpiringDays={onSaveExpiringDays}
      />

      <section className="space-y-3">
        <div className="space-y-1">
          <p className="text-sm font-medium text-moon-800">渠道默认模板</p>
          <p className="text-xs leading-5 text-moon-400">
            当订阅自身没有覆盖模板时，回退到这里；再为空才会使用内置默认模板。
          </p>
        </div>
        <div className="grid gap-4 xl:grid-cols-2">
          <TemplateOverrideEditor
            label="标题模板"
            mode={draft.title_template.trim() ? "custom" : "default"}
            value={draft.title_template}
            defaultValue="Lune · {{ .Title }}"
            onModeChange={(value) => {
              onDraftChange({
                ...draft,
                title_template: value === "custom" ? draft.title_template : "",
              });
            }}
            onValueChange={(value) =>
              onDraftChange({ ...draft, title_template: value })
            }
            onBlur={() =>
              onCommit("channel-title-template", {
                ...draft,
                title_template: draft.title_template,
              })
            }
          />
          <TemplateOverrideEditor
            label="正文模板"
            mode={draft.body_template.trim() ? "custom" : "default"}
            value={draft.body_template}
            defaultValue="{{ .Message }}"
            onModeChange={(value) => {
              onDraftChange({
                ...draft,
                body_template: value === "custom" ? draft.body_template : "",
              });
            }}
            onValueChange={(value) =>
              onDraftChange({ ...draft, body_template: value })
            }
            onBlur={() =>
              onCommit("channel-body-template", {
                ...draft,
                body_template: draft.body_template,
              })
            }
          />
        </div>
      </section>

      <RetryConfigEditor
        maxAttempts={draft.retry_max_attempts}
        scheduleInput={retryInput}
        saving={savingField === "retry"}
        onMaxAttemptsChange={(value) =>
          onDraftChange({
            ...draft,
            retry_max_attempts: Number.isFinite(value) && value > 0 ? value : 1,
          })
        }
        onScheduleInputChange={setRetryInput}
        onCommit={(nextAttempts, nextSchedule) => {
          const parsed = parseRetryInput(nextSchedule);
          const next = {
            ...draft,
            retry_max_attempts:
              Number.isFinite(nextAttempts) && nextAttempts > 0
                ? nextAttempts
                : 1,
            retry_schedule_seconds: parsed,
          };
          setRetryInput(nextSchedule);
          onDraftChange(next);
          onCommit("retry", next);
        }}
      />

      <div className="grid gap-6 xl:grid-cols-2">
        <TestPanel
          loading={testLoading}
          result={testResult}
          onRun={async () => {
            if (
              !window.confirm("会真的发送测试消息到 webhook，确认？")
            ) {
              return;
            }
            setTestLoading(true);
            try {
              const result = await api.post<TestResult>(
                `/notifications/channels/${draft.id}/test`,
                {},
              );
              setTestResult(result);
              await onRefresh();
            } catch (err) {
              setTestResult({
                ok: false,
                latency_ms: 0,
                upstream_code:
                  err instanceof ApiError ? `http ${err.status}` : "network_error",
                upstream_message:
                  err instanceof Error ? err.message : "测试失败",
              });
              toast(err instanceof Error ? err.message : "测试失败", "error");
            } finally {
              setTestLoading(false);
            }
          }}
        />

        <PreviewPanel
          eventTypes={eventTypes}
          event={previewEvent}
          severity={previewSeverity}
          result={previewResult}
          loading={previewLoading}
          onEventChange={(value) => {
            setPreviewEvent(value);
            const matched = eventTypes.find((item) => item.event === value);
            if (matched?.default_severity) {
              setPreviewSeverity(
                matched.default_severity as NotificationSeverity,
              );
            }
          }}
          onSeverityChange={setPreviewSeverity}
          onRun={async () => {
            setPreviewLoading(true);
            try {
              const items = await api.post<
                Array<{
                  channel_id: number;
                  rendered_title: string;
                  rendered_body: string;
                }>
              >("/notifications/preview", {
                event: previewEvent,
                severity: previewSeverity,
              });
              const matched = items.find((item) => item.channel_id === draft.id);
              setPreviewResult(
                matched
                  ? {
                      rendered_title: matched.rendered_title,
                      rendered_body: matched.rendered_body,
                    }
                  : null,
              );
            } catch (err) {
              toast(err instanceof Error ? err.message : "预览失败", "error");
            } finally {
              setPreviewLoading(false);
            }
          }}
        />
      </div>

      <DangerZone
        open={dangerOpen}
        error={error}
        onToggle={() => setDangerOpen((current) => !current)}
        onDelete={onDelete}
      />
    </div>
  );
}
