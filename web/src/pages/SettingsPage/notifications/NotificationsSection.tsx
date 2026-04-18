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
import { useRouter } from "@/lib/router";

import ExpiringDaysInput from "./ExpiringDaysInput";
import SettingsForm from "./SettingsForm";
import SubscriptionsTable from "./SubscriptionsTable";
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

  // Sequence guards: rapid PUT bursts can return out of order, and we must
  // not let an older response stomp newer state. Each commit path owns its
  // own monotonic counter; only the latest seq writes back into React state.
  const settingsSeqRef = useRef(0);
  const subSeqRef = useRef<Record<string, number>>({});
  const expiringDaysSeqRef = useRef(0);

  const { navigate } = useRouter();

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
      // Revert to the last-known server state so the UI reflects truth.
      void reload();
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
    const seqKey = `${event}:${field}`;
    const seq = (subSeqRef.current[seqKey] ?? 0) + 1;
    subSeqRef.current[seqKey] = seq;
    setFieldSaving(event, field);
    try {
      const updated = await api.put<NotificationSubscription>(
        `/notifications/subscriptions/${encodeURIComponent(event)}`,
        {
          subscribed: next.subscribed,
          body_template: next.body_template,
        },
      );
      if (seq !== subSeqRef.current[seqKey]) return;
      setSubscriptions((current) =>
        current.map((item) => (item.event === event ? updated : item)),
      );
      setBanner(null);
    } catch (err) {
      if (seq !== subSeqRef.current[seqKey]) return;
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
      void reload();
    } finally {
      if (seq === subSeqRef.current[seqKey]) {
        setFieldSaving(event, null);
      }
    }
  }

  async function runTest() {
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
    navigate("/admin/activity");
    // The custom router stores pathname without hash, so navigate() above
    // writes /admin/activity. We then add the hash via replaceState so the
    // URL is stateful (survives refresh / share) without confusing the
    // pathname match in App.tsx routing.
    window.history.replaceState(null, "", "/admin/activity#notifications");
    // ActivityPage is lazy-loaded and its Notifications section renders
    // after the first data fetch, so we retry scroll-into-view until the
    // anchor appears.
    let attempts = 0;
    const tryScroll = () => {
      const el = document.getElementById("notifications");
      if (el) {
        el.scrollIntoView({ behavior: "smooth", block: "start" });
        return;
      }
      if (attempts < 20) {
        attempts += 1;
        window.setTimeout(tryScroll, 60);
      }
    };
    tryScroll();
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
            <SettingsForm
              settings={settings}
              webhookUrlDraft={webhookUrlDraft}
              onWebhookUrlDraftChange={setWebhookUrlDraft}
              saving={settingsSaving}
              urlError={settingsUrlError}
              onChange={setSettings}
              onCommit={(next) => {
                // Swallow here — commitSettings already surfaced the toast and
                // reloaded server truth. We re-throw inside so `runTest` can
                // short-circuit, but SettingsForm doesn't need to know.
                commitSettings(next).catch(() => {});
              }}
              testSlot={
                <TestPanel
                  loading={testLoading}
                  result={testResult}
                  disabled={!canTest}
                  disabledReason={testDisabledReason}
                  onRun={() => void runTest()}
                />
              }
            />
          ) : null}

          {eventTypes.length ? (
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
          ) : null}

          <div className="flex justify-end">
            <a
              href="/admin/activity#notifications"
              onClick={jumpToDeliveries}
              className="inline-flex items-center gap-1.5 text-xs font-medium text-lunar-600 hover:text-lunar-700"
            >
              查看最近投递
              <ArrowUpRight className="size-3.5" />
            </a>
          </div>
        </div>
      </div>
    </section>
  );
}
