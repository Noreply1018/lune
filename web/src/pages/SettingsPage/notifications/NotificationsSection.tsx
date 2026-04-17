import { useEffect, useState } from "react";
import { Plus, RefreshCw } from "lucide-react";

import { toast } from "@/components/Feedback";
import SectionHeading from "@/components/SectionHeading";
import { Button } from "@/components/ui/button";
import { api } from "@/lib/api";

import ChannelAccordion from "./ChannelAccordion";
import EmptyState from "./EmptyState";
import {
  CHANNEL_TYPE_META,
  defaultChannelName,
  DEFAULT_RETRY_SCHEDULE,
  DEFAULT_SUBSCRIPTION,
  makeChannelDraft,
  buildChannelPayload,
  type ChannelType,
  type NotificationChannelDraft,
  type NotificationEventType,
} from "./types";
import type { NotificationChannel } from "@/lib/types";

export default function NotificationsSection({
  initialExpiringDays,
  onExpiringDaysChange,
}: {
  initialExpiringDays: number;
  onExpiringDaysChange: (value: number) => void;
}) {
  const [channels, setChannels] = useState<NotificationChannel[]>([]);
  const [eventTypes, setEventTypes] = useState<NotificationEventType[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [expandedId, setExpandedId] = useState<number | null>(null);
  const [draft, setDraft] = useState<NotificationChannelDraft | null>(null);
  const [savingField, setSavingField] = useState<string | null>(null);
  const [inlineError, setInlineError] = useState<string | null>(null);
  const [togglingId, setTogglingId] = useState<number | null>(null);
  const [creatorOpen, setCreatorOpen] = useState(false);
  const [expiringDays, setExpiringDays] = useState(initialExpiringDays);

  useEffect(() => {
    setExpiringDays(initialExpiringDays);
  }, [initialExpiringDays]);

  useEffect(() => {
    void reload();
  }, []);

  useEffect(() => {
    if (!inlineError) {
      return;
    }
    const timer = window.setTimeout(() => setInlineError(null), 4000);
    return () => window.clearTimeout(timer);
  }, [inlineError]);

  async function reload(silent = false) {
    if (!silent) {
      setLoading(true);
    }
    try {
      const [channelData, eventTypeData] = await Promise.all([
        api.get<NotificationChannel[]>("/notifications/channels"),
        api.get<NotificationEventType[]>("/notifications/event-types"),
      ]);
      setChannels(channelData ?? []);
      setEventTypes(eventTypeData ?? []);
      setError(null);
      if (expandedId != null) {
        const current = (channelData ?? []).find((item) => item.id === expandedId);
        setDraft(current ? makeChannelDraft(current) : null);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "通知渠道加载失败");
    } finally {
      if (!silent) {
        setLoading(false);
      }
    }
  }

  async function createChannel(type: ChannelType) {
    try {
      const created = await api.post<NotificationChannel>("/notifications/channels", {
        name: defaultChannelName(type, channels),
        type,
        enabled: true,
        config: buildCreateConfig(type),
        subscriptions: [DEFAULT_SUBSCRIPTION],
        title_template: "",
        body_template: "",
        retry_max_attempts: 5,
        retry_schedule_seconds: DEFAULT_RETRY_SCHEDULE,
      });
      const nextChannels = [...channels, created];
      setChannels(nextChannels);
      setExpandedId(created.id);
      setDraft(makeChannelDraft(created));
      setCreatorOpen(false);
      toast("渠道已创建");
    } catch (err) {
      toast(err instanceof Error ? err.message : "创建渠道失败", "error");
    }
  }

  async function saveDraft(field: string, nextDraft = draft) {
    if (!nextDraft) {
      return;
    }
    if (!nextDraft.name.trim()) {
      setInlineError("渠道名称不能为空");
      return;
    }
    setSavingField(field);
    try {
      const updated = await api.put<NotificationChannel>(
        `/notifications/channels/${nextDraft.id}`,
        buildChannelPayload(nextDraft),
      );
      setChannels((current) =>
        current.map((item) => (item.id === updated.id ? updated : item)),
      );
      setDraft(makeChannelDraft(updated));
      setInlineError(null);
    } catch (err) {
      setInlineError(err instanceof Error ? err.message : "保存失败");
    } finally {
      setSavingField(null);
    }
  }

  async function saveExpiringDays(value: number) {
    if (!Number.isFinite(value) || value <= 0) {
      return;
    }
    setExpiringDays(value);
    try {
      await api.put("/settings", { notification_expiring_days: value });
      onExpiringDaysChange(value);
    } catch (err) {
      toast(err instanceof Error ? err.message : "保存阈值失败", "error");
    }
  }

  async function toggleEnabled(id: number, enabled: boolean) {
    const previous = channels;
    setTogglingId(id);
    setChannels((current) =>
      current.map((item) => (item.id === id ? { ...item, enabled } : item)),
    );
    if (expandedId === id && draft) {
      setDraft({ ...draft, enabled });
    }
    try {
      await api.post(`/notifications/channels/${id}/enabled`, { enabled });
      await reload(true);
    } catch (err) {
      setChannels(previous);
      if (expandedId === id && draft) {
        const old = previous.find((item) => item.id === id);
        if (old) {
          setDraft(makeChannelDraft(old));
        }
      }
      toast(err instanceof Error ? err.message : "更新渠道状态失败", "error");
    } finally {
      setTogglingId(null);
    }
  }

  async function deleteChannel(id: number) {
    if (!window.confirm("删除后不会再投递，确认继续？")) {
      return;
    }
    try {
      await api.delete(`/notifications/channels/${id}`);
      const nextChannels = channels.filter((item) => item.id !== id);
      setChannels(nextChannels);
      if (expandedId === id) {
        setExpandedId(null);
        setDraft(null);
      }
      toast("渠道已删除");
    } catch (err) {
      toast(err instanceof Error ? err.message : "删除渠道失败", "error");
      setInlineError(err instanceof Error ? err.message : "删除渠道失败");
    }
  }

  const channelDrafts = channels.map((item) =>
    expandedId === item.id && draft ? draft : makeChannelDraft(item),
  );

  return (
    <section className="surface-section px-5 py-5 sm:px-6">
      <SectionHeading
        title="Notifications"
        description="围绕 channel 管理订阅、模板、重试和投递反馈。"
        action={
          <div className="flex flex-wrap items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              className="rounded-full"
              onClick={() => void reload()}
              disabled={loading}
            >
              <RefreshCw className={`size-4 ${loading ? "animate-spin" : ""}`} />
              Refresh
            </Button>
            <Button
              className="rounded-full"
              onClick={() => setCreatorOpen((current) => !current)}
            >
              <Plus className="size-4" />
              Add Channel
            </Button>
          </div>
        }
      />

      <div className="mt-6 space-y-5">
        {creatorOpen ? (
          <div className="surface-outline flex flex-wrap items-center gap-2 px-4 py-4">
            {Object.entries(CHANNEL_TYPE_META).map(([type, meta]) => (
              <Button
                key={type}
                variant="outline"
                className="rounded-full"
                onClick={() => void createChannel(type as ChannelType)}
              >
                <Plus className="size-4" />
                {meta.label}
              </Button>
            ))}
          </div>
        ) : null}

        {error ? (
          <div className="rounded-[1.2rem] border border-status-red/18 bg-status-red/6 px-4 py-4 text-sm text-status-red">
            {error}
          </div>
        ) : null}

        {loading && !channels.length ? (
          <div className="rounded-[1.35rem] border border-dashed border-moon-200/55 px-5 py-6 text-sm text-moon-450">
            正在加载通知渠道…
          </div>
        ) : null}

        {!loading && !channels.length ? (
          <EmptyState onCreate={(type) => void createChannel(type)} />
        ) : null}

        {channels.length ? (
          <ChannelAccordion
            channels={channelDrafts}
            expandedId={expandedId}
            draft={draft}
            eventTypes={eventTypes}
            expiringDays={expiringDays}
            togglingId={togglingId}
            savingField={savingField}
            error={inlineError}
            onExpand={(id) => {
              setExpandedId(id);
              const matched =
                id == null
                  ? null
                  : channels.find((item) => item.id === id) ?? null;
              setDraft(matched ? makeChannelDraft(matched) : null);
            }}
            onToggleEnabled={toggleEnabled}
            onDraftChange={setDraft}
            onCommit={saveDraft}
            onSaveExpiringDays={(value) => void saveExpiringDays(value)}
            onDelete={(id) => void deleteChannel(id)}
            onRefresh={() => reload(true)}
          />
        ) : null}
      </div>
    </section>
  );
}

function buildCreateConfig(type: ChannelType) {
  const defaults = CHANNEL_TYPE_META[type].defaults;
  switch (type) {
    case "generic_webhook":
      return { schema: 1, url: defaults.url };
    case "wechat_work_bot":
      return {
        schema: 1,
        webhook_url: defaults.webhook_url,
        format: defaults.format,
        mention_list: [],
        mention_mobile_list: [],
      };
    case "feishu_bot":
      return {
        schema: 1,
        webhook_url: defaults.webhook_url,
        secret: defaults.secret,
        format: defaults.format,
      };
    case "email_smtp":
      return {
        schema: 1,
        host: defaults.host,
        port: Number(defaults.port || "587"),
        username: defaults.username,
        password: defaults.password,
        from: defaults.from,
        to: [],
        tls_mode: defaults.tls_mode,
      };
  }
}
