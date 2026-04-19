import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type MouseEvent,
} from "react";
import { ArrowUpRight } from "lucide-react";

import { toast } from "@/components/Feedback";
import SectionHeading from "@/components/SectionHeading";
import { api, ApiError } from "@/lib/api";
import { Switch } from "@/components/ui/switch";

import ExpiringDaysInput from "./ExpiringDaysInput";
import SettingsForm from "./SettingsForm";
import SubscriptionsTable from "./SubscriptionsTable";
import Tabs from "./Tabs";
import TestPanel, { type TestResult } from "./TestPanel";
import type {
  NotificationEventType,
  NotificationSettings,
  NotificationSubscription,
} from "./types";

type OverviewResponse = {
  settings: NotificationSettings;
  subscriptions: NotificationSubscription[];
  event_types: NotificationEventType[];
};

type NotificationsSectionProps = {
  initialExpiringDays: number;
  onExpiringDaysChange: (value: number) => void;
};

type TabKey = "channel" | "subscriptions";

const TABS = [
  { key: "channel" as const, label: "渠道" },
  { key: "subscriptions" as const, label: "订阅事件" },
];

export default function NotificationsSection({
  initialExpiringDays,
  onExpiringDaysChange,
}: NotificationsSectionProps) {
  const [settings, setSettings] = useState<NotificationSettings | null>(null);
  // Mirror of the webhook URL input. Lives here so the enable switch (in
  // SettingsForm) and the Send Test button (in TestPanel) both see the same
  // value the user is currently typing — even before they blur the input.
  const [webhookUrlDraft, setWebhookUrlDraft] = useState("");
  const [subscriptions, setSubscriptions] = useState<NotificationSubscription[]>(
    [],
  );
  const [eventTypes, setEventTypes] = useState<NotificationEventType[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [banner, setBanner] = useState<string | null>(null);
  const [settingsSaving, setSettingsSaving] = useState(false);
  const [settingsUrlError, setSettingsUrlError] = useState<string | null>(null);
  const [rowSavingField, setRowSavingField] = useState<
    Record<string, string | null>
  >({});
  const [rowErrors, setRowErrors] = useState<
    Record<string, { body?: string | null }>
  >({});
  const [testLoading, setTestLoading] = useState(false);
  const [testResult, setTestResult] = useState<TestResult | null>(null);
  const [expiringDays, setExpiringDays] = useState(initialExpiringDays);
  const [expiringDaysError, setExpiringDaysError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<TabKey>("channel");

  // Sequence guards: rapid PUT bursts can return out of order, and we must
  // not let an older response stomp newer state. Each commit path owns its
  // own monotonic counter; only the latest seq writes back into React state.
  const settingsSeqRef = useRef(0);
  const subSeqRef = useRef<Record<string, number>>({});
  const expiringDaysSeqRef = useRef(0);
  // Synchronous inflight latch for Send Test. Relying on the testLoading
  // state would work for distinct user clicks (they arrive in separate
  // Tasks after a re-render) but can't defend against two runTest() calls
  // within the same microtask — a ref updates immediately, state doesn't.
  const testInFlightRef = useRef(false);

  useEffect(() => {
    setExpiringDays(initialExpiringDays);
  }, [initialExpiringDays]);

  const reload = useCallback(async () => {
    try {
      const data = await api.get<OverviewResponse>("/notifications");
      setSettings(data.settings);
      setWebhookUrlDraft(data.settings.webhook_url);
      setSubscriptions(data.subscriptions ?? []);
      setEventTypes(data.event_types ?? []);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "通知设置加载失败");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void reload();
  }, [reload]);

  async function commitSettings(next: NotificationSettings) {
    const seq = ++settingsSeqRef.current;
    // Snapshot pre-optimistic state so failure can roll back locally. The
    // old path called reload() here, which had two problems: (a) it wiped
    // webhookUrlDraft the user might still be typing, and (b) a slow GET
    // could land AFTER a newer successful commit and stomp its result.
    // The seq guard below still handles the "newer commit in flight" case.
    const prevSnapshot = settings;
    setSettingsSaving(true);
    setSettingsUrlError(null);
    try {
      const updated = await api.put<NotificationSettings>(
        "/notifications/settings",
        {
          enabled: next.enabled,
          webhook_url: next.webhook_url.trim(),
          mention_mobile_list: next.mention_mobile_list,
        },
      );
      // Drop stale responses so a slow earlier PUT can't roll back the
      // newer values the user just committed.
      if (seq !== settingsSeqRef.current) return;
      setSettings(updated);
      setWebhookUrlDraft(updated.webhook_url);
    } catch (err) {
      if (seq !== settingsSeqRef.current) return;
      const message = err instanceof Error ? err.message : "保存通知设置失败";
      // Webhook URL errors surface inline under the input; everything else
      // only goes through the toast so we don't stack three redundant
      // indicators on screen.
      if (err instanceof ApiError && err.status === 400) {
        if (
          message.toLowerCase().includes("webhook_url") ||
          message.toLowerCase().includes("webhook url")
        ) {
          setSettingsUrlError(message);
        }
      }
      toast(message, "error");
      // Revert to the pre-optimistic snapshot. Leave webhookUrlDraft
      // alone so the user's in-progress typing survives — they can fix
      // the bad URL and blur to retry.
      if (prevSnapshot) {
        setSettings(prevSnapshot);
      }
      // Re-throw so callers that await commitSettings (e.g. runTest pre-save)
      // can short-circuit on failure instead of racing ahead with a URL the
      // server rejected.
      throw err;
    } finally {
      if (seq === settingsSeqRef.current) {
        setSettingsSaving(false);
      }
    }
  }

  function setFieldSaving(event: string, field: string | null) {
    setRowSavingField((current) => ({ ...current, [event]: field }));
  }

  function setFieldError(
    event: string,
    patch: { body?: string | null },
  ) {
    setRowErrors((current) => {
      const next = { ...current, [event]: { ...current[event], ...patch } };
      return next;
    });
  }

  function clearFieldErrors(event: string) {
    setRowErrors((current) => {
      if (!current[event]) {
        return current;
      }
      const next = { ...current };
      delete next[event];
      return next;
    });
  }

  function clearSingleFieldError(event: string, field: "body") {
    setRowErrors((current) => {
      const existing = current[event];
      if (!existing || existing[field] == null) {
        return current;
      }
      const updated = { ...existing, [field]: null };
      return { ...current, [event]: updated };
    });
  }

  async function commitSubscription(
    event: string,
    field: "subscribed" | "body",
    next: NotificationSubscription,
  ) {
    const body = next.body_template.trim();
    if (field === "body" && body === "") {
      setFieldError(event, { body: "正文模板不能为空" });
      return;
    }
    if (body === "" && field === "subscribed") {
      setBanner("请先补全正文模板再切换订阅状态");
      return;
    }
    clearFieldErrors(event);
    // Seq is keyed per row, not per (row, field). Body and subscribed both
    // hit the same PUT endpoint, so two in-flight requests against the
    // same row can land responses out of order — we must serialize ack
    // handling row-wide, not column-wide.
    const seq = (subSeqRef.current[event] ?? 0) + 1;
    subSeqRef.current[event] = seq;
    setFieldSaving(event, field);
    try {
      const updated = await api.put<NotificationSubscription>(
        `/notifications/subscriptions/${encodeURIComponent(event)}`,
        {
          subscribed: next.subscribed,
          body_template: next.body_template,
        },
      );
      if (seq !== subSeqRef.current[event]) return;
      setSubscriptions((current) =>
        current.map((item) => (item.event === event ? updated : item)),
      );
      setBanner(null);
    } catch (err) {
      if (seq !== subSeqRef.current[event]) return;
      const message =
        err instanceof Error ? err.message : "保存订阅设置失败";
      if (err instanceof ApiError && err.status === 400) {
        if (field === "body") {
          setFieldError(event, { body: message });
        } else {
          setBanner(message);
        }
      } else {
        setBanner(message);
      }
      toast(message, "error");
      // No reload() here: subscriptions state was never optimistically
      // mutated on the client side (callers hand us `next` but we only
      // write to state on success). A blanket reload would race newer
      // commits and could stomp their fresh writes.
    } finally {
      if (seq === subSeqRef.current[event]) {
        setFieldSaving(event, null);
      }
    }
  }

  async function runTest() {
    // The button is disabled={disabled || loading}, but rapid double-
    // clicks can still fire two handlers before React re-renders with
    // the new disabled state. A state-based guard (testLoading) lags one
    // render; a ref latch flips synchronously so only the first caller
    // reaches the POST.
    if (testInFlightRef.current) return;
    testInFlightRef.current = true;
    setTestLoading(true);
    try {
      // If the user typed a new URL but hasn't blurred yet, persist the draft
      // first so the test runs against the URL they can actually see — not
      // the one still on the server. If the pre-save fails (e.g. server
      // rejects the URL as 400), commitSettings already surfaced the toast —
      // bail out instead of testing a URL we know isn't live.
      if (settings && webhookUrlDraft.trim() !== settings.webhook_url) {
        try {
          await commitSettings({
            ...settings,
            webhook_url: webhookUrlDraft.trim(),
          });
        } catch {
          return;
        }
      }
      const result = await api.post<TestResult>("/notifications/test", {});
      setTestResult(result);
      if (!result.ok) {
        toast(result.upstream_message || "测试失败", "error");
      }
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        toast("通知未启用或 Webhook URL 为空", "error");
        setTestResult({
          ok: false,
          latency_ms: 0,
          upstream_code: "disabled",
          upstream_message: "通知未启用或 Webhook URL 为空",
        });
      } else {
        const message = err instanceof Error ? err.message : "测试失败";
        toast(message, "error");
        setTestResult({
          ok: false,
          latency_ms: 0,
          upstream_code:
            err instanceof ApiError ? `http ${err.status}` : "network_error",
          upstream_message: message,
        });
      }
    } finally {
      testInFlightRef.current = false;
      setTestLoading(false);
    }
  }

  async function saveExpiringDays(value: number) {
    const previous = expiringDays;
    setExpiringDays(value);
    setExpiringDaysError(null);
    const seq = ++expiringDaysSeqRef.current;
    try {
      await api.put("/settings", { notification_expiring_days: value });
      if (seq !== expiringDaysSeqRef.current) return;
      onExpiringDaysChange(value);
    } catch (err) {
      if (seq !== expiringDaysSeqRef.current) return;
      const message = err instanceof Error ? err.message : "保存阈值失败";
      setExpiringDaysError(message);
      setExpiringDays(previous);
      toast(message, "error");
    }
  }

  function jumpToDeliveries(event: MouseEvent<HTMLAnchorElement>) {
    event.preventDefault();
    // Notification history lives in an independent section on this same page.
    // Setting window.location.hash both updates the URL and emits
    // `hashchange`, which SettingsPage.handleHash already handles (retry loop
    // + cleanup). Doing it there keeps scroll logic single-sourced.
    if (window.location.hash === "#notification-history") {
      // Same hash — no event fires. Manually kick the listener.
      window.dispatchEvent(new HashChangeEvent("hashchange"));
    } else {
      window.location.hash = "notification-history";
    }
  }

  // Use the live draft, not the persisted value, so users see the test
  // button enable as soon as they finish typing — no need to blur first.
  const trimmedDraftUrl = webhookUrlDraft.trim();
  const canTest = Boolean(settings?.enabled && trimmedDraftUrl.length > 0);
  const testDisabledReason = !settings?.enabled
    ? "开启顶部总开关后才能发送"
    : trimmedDraftUrl.length === 0
      ? "请先填写 Webhook URL"
      : undefined;

  return (
    <section className="surface-section overflow-hidden px-5 py-5 sm:px-6">
      <div className="rounded-[1.7rem] border border-white/75 bg-[linear-gradient(180deg,rgba(255,255,255,0.92),rgba(243,240,248,0.76))] px-4 py-4 shadow-[0_30px_80px_-54px_rgba(33,40,63,0.32)] sm:px-5">
        <SectionHeading
          title="Notifications"
          description="配置企业微信机器人，接收账号、CPA 告警。只支持企微一种渠道。"
        />

        <div className="mt-6 space-y-6">
          {error ? (
            <div className="rounded-[1.1rem] border border-status-red/18 bg-status-red/6 px-4 py-3 text-sm text-status-red">
              {error}
            </div>
          ) : null}

          {banner ? (
            <div
              role="alert"
              aria-live="polite"
              className="rounded-[1.1rem] border border-status-red/18 bg-status-red/6 px-4 py-3 text-sm text-status-red"
            >
              {banner}
            </div>
          ) : null}

          {loading && !settings ? (
            <div className="rounded-[1.25rem] border border-dashed border-moon-200/55 px-5 py-6 text-sm text-moon-450">
              正在加载通知设置…
            </div>
          ) : null}

          {settings ? (
            <>
              {/* Top strip: global enable switch (stays outside Tabs because
                  it applies to both tabs) + the "view deliveries" shortcut.
                  Mirroring the pre-tab layout's 2-col grid keeps the switch
                  visually anchored where users expect it. */}
              <div className="grid gap-4 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center">
                <div className="flex items-center justify-between gap-4 rounded-[1rem] border border-white/75 bg-white/75 px-4 py-3">
                  <div>
                    <p className="text-sm font-medium text-moon-800">启用通知</p>
                    <p className="text-xs leading-5 text-moon-400">
                      关闭后所有事件都不再投递，已配置的企微信息保留。
                    </p>
                  </div>
                  <Switch
                    checked={settings.enabled}
                    disabled={settingsSaving}
                    onCheckedChange={(checked) => {
                      // Fold any pending URL draft into this commit so toggling
                      // the switch never silently re-validates against a stale
                      // URL.
                      const trimmedUrl = webhookUrlDraft.trim();
                      const next = {
                        ...settings,
                        enabled: checked,
                        webhook_url: trimmedUrl,
                      };
                      setSettings(next);
                      commitSettings(next).catch(() => {});
                    }}
                  />
                </div>
                <a
                  href="/admin/settings#notification-history"
                  onClick={jumpToDeliveries}
                  className="inline-flex items-center justify-end gap-1.5 text-sm font-medium text-lunar-600 hover:text-lunar-700 sm:justify-start"
                >
                  查看最近投递
                  <ArrowUpRight className="size-3.5" />
                </a>
              </div>

              {!settings.enabled ? (
                <div className="rounded-[0.9rem] border border-amber-200/70 bg-amber-50/85 px-3 py-2 text-xs text-amber-800">
                  通知已关闭。订阅和模板仍可编辑，开启后立刻生效。
                </div>
              ) : null}

              <Tabs<TabKey>
                tabs={TABS}
                active={activeTab}
                onChange={setActiveTab}
                ariaLabel="通知配置"
                panels={{
                  channel: (
                    <div className="space-y-5">
                      <SettingsForm
                        settings={settings}
                        webhookUrlDraft={webhookUrlDraft}
                        onWebhookUrlDraftChange={setWebhookUrlDraft}
                        saving={settingsSaving}
                        urlError={settingsUrlError}
                        onChange={setSettings}
                        onCommit={(next) => {
                          // Swallow here — commitSettings already surfaced the
                          // toast and reloaded server truth.
                          commitSettings(next).catch(() => {});
                        }}
                      />
                      <div className="space-y-3">
                        <div className="flex items-center gap-3">
                          <span className="text-[11px] font-medium uppercase tracking-[0.18em] text-moon-400">
                            测试
                          </span>
                          <div className="h-px flex-1 bg-moon-200/55" />
                        </div>
                        <TestPanel
                          loading={testLoading}
                          result={testResult}
                          disabled={!canTest}
                          disabledReason={testDisabledReason}
                          onRun={() => void runTest()}
                        />
                      </div>
                    </div>
                  ),
                  subscriptions: eventTypes.length ? (
                    <SubscriptionsTable
                      subscriptions={subscriptions}
                      eventTypes={eventTypes}
                      savingField={rowSavingField}
                      fieldErrors={rowErrors}
                      onCommit={(event, field, next) =>
                        void commitSubscription(event, field, next)
                      }
                      onClearFieldError={clearSingleFieldError}
                      renderExtra={(event) =>
                        event === "account_expiring" ? (
                          <ExpiringDaysInput
                            value={expiringDays}
                            error={expiringDaysError}
                            onCommit={(next) => void saveExpiringDays(next)}
                            onClearError={() => setExpiringDaysError(null)}
                          />
                        ) : null
                      }
                    />
                  ) : (
                    <div className="rounded-[1.25rem] border border-dashed border-moon-200/55 px-5 py-6 text-sm text-moon-450">
                      还没有可订阅的事件类型。
                    </div>
                  ),
                }}
              />
            </>
          ) : null}
        </div>
      </div>
    </section>
  );
}
