import { useEffect, useMemo, useState } from "react";

import ConfirmDialog from "@/components/ConfirmDialog";
import { toast } from "@/components/Feedback";
import { api, ApiError } from "@/lib/api";

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

function defaultPreviewSelection(eventTypes: NotificationEventType[]) {
  const event = eventTypes[0]?.event ?? "account_error";
  const severity =
    (eventTypes[0]?.default_severity as NotificationSeverity | undefined) ??
    "critical";
  return { event, severity };
}

export default function ChannelDetail({
  draft,
  eventTypes,
  expiringDays,
  expiringDaysError,
  savingField,
  saveError,
  deleteError,
  deleting,
  onDraftChange,
  onCommit,
  onSaveExpiringDays,
  onDelete,
  onRefresh,
}: {
  draft: NotificationChannelDraft;
  eventTypes: NotificationEventType[];
  expiringDays: number;
  expiringDaysError: string | null;
  savingField: string | null;
  saveError: string | null;
  deleteError: string | null;
  deleting?: boolean;
  onDraftChange: (next: NotificationChannelDraft) => void;
  onCommit: (field: string, next?: NotificationChannelDraft) => void;
  onSaveExpiringDays: (value: number) => void;
  onDelete: () => void;
  onRefresh: () => Promise<void>;
}) {
  const previewDefaults = useMemo(
    () => defaultPreviewSelection(eventTypes),
    [eventTypes],
  );
  const [dangerOpen, setDangerOpen] = useState(false);
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false);
  const [testConfirmOpen, setTestConfirmOpen] = useState(false);
  const [testLoading, setTestLoading] = useState(false);
  const [testResult, setTestResult] = useState<TestResult | null>(null);
  const [previewEvent, setPreviewEvent] = useState(previewDefaults.event);
  const [previewSeverity, setPreviewSeverity] =
    useState<NotificationSeverity>(previewDefaults.severity);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [previewResult, setPreviewResult] = useState<PreviewResult | null>(null);
  const [channelTitleMode, setChannelTitleMode] = useState<"default" | "custom">(
    draft.title_template.trim() ? "custom" : "default",
  );
  const [channelBodyMode, setChannelBodyMode] = useState<"default" | "custom">(
    draft.body_template.trim() ? "custom" : "default",
  );

  useEffect(() => {
    setChannelTitleMode(draft.title_template.trim() ? "custom" : "default");
  }, [draft.title_template]);

  useEffect(() => {
    setChannelBodyMode(draft.body_template.trim() ? "custom" : "default");
  }, [draft.body_template]);

  useEffect(() => {
    if (!eventTypes.some((item) => item.event === previewEvent)) {
      setPreviewEvent(previewDefaults.event);
      setPreviewSeverity(previewDefaults.severity);
    }
  }, [eventTypes, previewDefaults.event, previewDefaults.severity, previewEvent]);

  function commitChannelTemplates() {
    onCommit("channel-templates", {
      ...draft,
      title_template:
        channelTitleMode === "custom" ? draft.title_template.trim() : "",
      body_template: channelBodyMode === "custom" ? draft.body_template.trim() : "",
    });
  }

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
        expiringDaysError={expiringDaysError}
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
            只有订阅级模板为空时才回退到这里；两层都为空时才使用内置默认模板。
          </p>
        </div>
        <div
          className="grid gap-4 xl:grid-cols-2"
          onBlur={(event) => {
            if (!event.currentTarget.contains(event.relatedTarget as Node | null)) {
              commitChannelTemplates();
            }
          }}
        >
          <TemplateOverrideEditor
            label="标题模板"
            mode={channelTitleMode}
            value={draft.title_template}
            defaultValue="Lune · {{ .Title }}"
            defaultLabel="内置标题模板"
            onModeChange={setChannelTitleMode}
            onValueChange={(value) =>
              onDraftChange({ ...draft, title_template: value })
            }
          />
          <TemplateOverrideEditor
            label="正文模板"
            mode={channelBodyMode}
            value={draft.body_template}
            defaultValue="{{ .Message }}"
            defaultLabel="内置正文模板"
            onModeChange={setChannelBodyMode}
            onValueChange={(value) =>
              onDraftChange({ ...draft, body_template: value })
            }
          />
        </div>
      </section>

      <RetryConfigEditor
        maxAttempts={draft.retry_max_attempts}
        scheduleInput={retryInputValue(draft.retry_schedule_seconds)}
        saving={savingField === "retry"}
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
          onDraftChange(next);
          onCommit("retry", next);
        }}
      />

      <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
        <TestPanel
          loading={testLoading}
          result={testResult}
          onRun={() => setTestConfirmOpen(true)}
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
            setPreviewSeverity(
              (matched?.default_severity as NotificationSeverity | undefined) ??
                "critical",
            );
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

      {saveError ? (
        <div className="rounded-[1rem] border border-status-red/18 bg-status-red/6 px-4 py-3 text-sm text-status-red">
          {saveError}
        </div>
      ) : null}

      <DangerZone
        open={dangerOpen}
        error={deleteError}
        deleting={deleting}
        onOpenChange={setDangerOpen}
        onDeleteRequest={() => setDeleteConfirmOpen(true)}
      />

      <ConfirmDialog
        open={testConfirmOpen}
        onOpenChange={setTestConfirmOpen}
        title="发送测试消息"
        description="这会向当前 webhook 真实发送一条测试通知，用于确认渠道是否可达。"
        confirmLabel="发送测试"
        variant="default"
        onConfirm={() => {
          void (async () => {
            setTestLoading(true);
            setTestConfirmOpen(false);
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
          })();
        }}
      />
      <ConfirmDialog
        open={deleteConfirmOpen}
        onOpenChange={setDeleteConfirmOpen}
        title="删除通知渠道"
        description="删除后不会再投递到这个 channel，待投递队列会被清空，历史记录仍保留在 Activity。"
        confirmLabel="确认删除"
        onConfirm={() => {
          setDeleteConfirmOpen(false);
          onDelete();
        }}
      />
    </div>
  );
}
