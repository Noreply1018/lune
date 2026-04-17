import { useCallback, useEffect, useState } from "react";
import { RefreshCw } from "lucide-react";

import { toast } from "@/components/Feedback";
import SectionHeading from "@/components/SectionHeading";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { api, ApiError } from "@/lib/api";

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
  const [subscriptions, setSubscriptions] = useState<NotificationSubscription[]>(
    [],
  );
  const [eventTypes, setEventTypes] = useState<NotificationEventType[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [banner, setBanner] = useState<string | null>(null);
  const [settingsSaving, setSettingsSaving] = useState(false);
  const [settingsUrlError, setSettingsUrlError] = useState<string | null>(null);
  const [rowSavingField, setRowSavingField] = useState<
    Record<string, string | null>
  >({});
  const [rowErrors, setRowErrors] = useState<
    Record<string, { title?: string | null; body?: string | null }>
  >({});
  const [testLoading, setTestLoading] = useState(false);
  const [testResult, setTestResult] = useState<TestResult | null>(null);
  const [expiringDays, setExpiringDays] = useState(initialExpiringDays);
  const [expiringDaysError, setExpiringDaysError] = useState<string | null>(null);

  useEffect(() => {
    setExpiringDays(initialExpiringDays);
  }, [initialExpiringDays]);

  const reload = useCallback(
    async (opts: { silent?: boolean } = {}) => {
      if (opts.silent) {
        setRefreshing(true);
      } else {
        setLoading(true);
      }
      try {
        const data = await api.get<OverviewResponse>("/notifications");
        setSettings(data.settings);
        setSubscriptions(data.subscriptions ?? []);
        setEventTypes(data.event_types ?? []);
        setError(null);
      } catch (err) {
        setError(err instanceof Error ? err.message : "通知设置加载失败");
      } finally {
        if (opts.silent) {
          setRefreshing(false);
        } else {
          setLoading(false);
        }
      }
    },
    [],
  );

  useEffect(() => {
    void reload();
  }, [reload]);

  async function commitSettings(next: NotificationSettings) {
    setSettingsSaving(true);
    setSettingsUrlError(null);
    try {
      const updated = await api.put<NotificationSettings>(
        "/notifications/settings",
        {
          enabled: next.enabled,
          webhook_url: next.webhook_url.trim(),
          format: next.format,
          mention_mobile_list: next.mention_mobile_list,
        },
      );
      setSettings(updated);
      setBanner(null);
    } catch (err) {
      const message = err instanceof Error ? err.message : "保存通知设置失败";
      if (err instanceof ApiError && err.status === 400) {
        if (
          message.toLowerCase().includes("webhook_url") ||
          message.toLowerCase().includes("webhook url")
        ) {
          setSettingsUrlError(message);
        }
      }
      setBanner(message);
      toast(message, "error");
      // Revert to the last-known server state so the UI reflects truth.
      void reload({ silent: true });
    } finally {
      setSettingsSaving(false);
    }
  }

  function setFieldSaving(event: string, field: string | null) {
    setRowSavingField((current) => ({ ...current, [event]: field }));
  }

  function setFieldError(
    event: string,
    patch: { title?: string | null; body?: string | null },
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

  async function commitSubscription(
    event: string,
    field: "subscribed" | "title" | "body",
    next: NotificationSubscription,
  ) {
    const title = next.title_template.trim();
    const body = next.body_template.trim();
    if (field === "title" && title === "") {
      setFieldError(event, { title: "标题模板不能为空" });
      return;
    }
    if (field === "body" && body === "") {
      setFieldError(event, { body: "正文模板不能为空" });
      return;
    }
    if (title === "" || body === "") {
      // A subscribed toggle shouldn't re-send an empty template body.
      if (field === "subscribed") {
        setBanner("请先补全标题/正文模板再切换订阅状态");
        return;
      }
    }
    clearFieldErrors(event);
    setFieldSaving(event, field);
    try {
      const updated = await api.put<NotificationSubscription>(
        `/notifications/subscriptions/${encodeURIComponent(event)}`,
        {
          subscribed: next.subscribed,
          title_template: next.title_template,
          body_template: next.body_template,
        },
      );
      setSubscriptions((current) =>
        current.map((item) => (item.event === event ? updated : item)),
      );
      setBanner(null);
    } catch (err) {
      const message =
        err instanceof Error ? err.message : "保存订阅设置失败";
      if (err instanceof ApiError && err.status === 400) {
        if (field === "title") {
          setFieldError(event, { title: message });
        } else if (field === "body") {
          setFieldError(event, { body: message });
        } else {
          setBanner(message);
        }
      } else {
        setBanner(message);
      }
      toast(message, "error");
      void reload({ silent: true });
    } finally {
      setFieldSaving(event, null);
    }
  }

  async function runTest() {
    setTestLoading(true);
    try {
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
    if (!Number.isFinite(value) || value <= 0) {
      setExpiringDaysError("请输入正整数");
      return;
    }
    setExpiringDays(value);
    try {
      await api.put("/settings", { notification_expiring_days: value });
      onExpiringDaysChange(value);
      setExpiringDaysError(null);
    } catch (err) {
      const message = err instanceof Error ? err.message : "保存阈值失败";
      setExpiringDaysError(message);
      toast(message, "error");
    }
  }

  const canTest = Boolean(
    settings?.enabled && settings.webhook_url.trim().length > 0,
  );
  const testDisabledReason = !settings?.enabled
    ? "开启顶部总开关后才能发送"
    : !settings?.webhook_url.trim()
      ? "请先填写 Webhook URL"
      : undefined;

  return (
    <section className="surface-section overflow-hidden px-5 py-5 sm:px-6">
      <div className="rounded-[1.7rem] border border-white/75 bg-[linear-gradient(180deg,rgba(255,255,255,0.92),rgba(243,240,248,0.76))] px-4 py-4 shadow-[0_30px_80px_-54px_rgba(33,40,63,0.32)] sm:px-5">
        <SectionHeading
          title="Notifications"
          description="配置企业微信机器人，接收账号、CPA 告警。只支持企微一种渠道。"
          action={
            <Button
              variant="outline"
              size="sm"
              className="rounded-full"
              onClick={() => void reload({ silent: true })}
              disabled={loading || refreshing}
            >
              <RefreshCw
                className={
                  loading || refreshing ? "size-4 animate-spin" : "size-4"
                }
              />
              Refresh
            </Button>
          }
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
              saving={settingsSaving}
              urlError={settingsUrlError}
              onChange={setSettings}
              onCommit={(next) => void commitSettings(next)}
            />
          ) : null}

          {settings ? (
            <div className="flex items-end gap-3 rounded-[1rem] border border-white/75 bg-white/75 px-4 py-3">
              <div className="space-y-1">
                <p className="text-sm font-medium text-moon-800">过期提醒阈值</p>
                <p className="text-xs leading-5 text-moon-400">
                  账号过期时间距今小于该天数就触发 account_expiring。
                </p>
              </div>
              <div className="ml-auto flex items-end gap-2">
                <div className="flex items-center gap-2">
                  <Input
                    type="number"
                    min={1}
                    value={expiringDays}
                    onChange={(event) =>
                      setExpiringDays(Number(event.target.value) || 0)
                    }
                    onBlur={() => {
                      if (expiringDays !== initialExpiringDays) {
                        void saveExpiringDays(expiringDays);
                      }
                    }}
                    className="w-20"
                  />
                  <span className="text-xs text-moon-400">天</span>
                </div>
              </div>
              {expiringDaysError ? (
                <p className="text-xs text-status-red">{expiringDaysError}</p>
              ) : null}
            </div>
          ) : null}

          {eventTypes.length ? (
            <SubscriptionsTable
              subscriptions={subscriptions}
              eventTypes={eventTypes}
              savingEvent={null}
              savingField={rowSavingField}
              fieldErrors={rowErrors}
              onCommit={(event, field, next) =>
                void commitSubscription(event, field, next)
              }
            />
          ) : null}

          {settings ? (
            <TestPanel
              loading={testLoading}
              result={testResult}
              disabled={!canTest}
              disabledReason={testDisabledReason}
              onRun={() => void runTest()}
            />
          ) : null}
        </div>
      </div>
    </section>
  );
}
