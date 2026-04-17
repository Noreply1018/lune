import { useEffect, useState } from "react";
import { ChevronDown, Plus, RefreshCw } from "lucide-react";

import { toast } from "@/components/Feedback";
import SectionHeading from "@/components/SectionHeading";
import { Button } from "@/components/ui/button";
import { api } from "@/lib/api";
import { cn } from "@/lib/utils";

import ChannelAccordion from "./ChannelAccordion";
import CreateChannelPanel from "./CreateChannelPanel";
import EmptyState from "./EmptyState";
import {
  CHANNEL_TYPE_META,
  DEFAULT_RETRY_SCHEDULE,
  buildChannelPayload,
  defaultChannelName,
  makeChannelDraft,
  type ChannelType,
  type NotificationChannelDraft,
  type NotificationEventType,
} from "./types";
import type { NotificationChannel } from "@/lib/types";

type ReloadOptions = {
  silent?: boolean;
  preserveDraft?: boolean;
};

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
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [expandedId, setExpandedId] = useState<number | null>(null);
  const [draft, setDraft] = useState<NotificationChannelDraft | null>(null);
  const [draftDirty, setDraftDirty] = useState(false);
  const [savingField, setSavingField] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [deleteError, setDeleteError] = useState<string | null>(null);
  const [expiringDaysError, setExpiringDaysError] = useState<string | null>(null);
  const [togglingId, setTogglingId] = useState<number | null>(null);
  const [deletingId, setDeletingId] = useState<number | null>(null);
  const [creatingType, setCreatingType] = useState<ChannelType | null>(null);
  const [creatorOpen, setCreatorOpen] = useState(false);
  const [creatorDraft, setCreatorDraft] = useState<NotificationChannelDraft | null>(
    null,
  );
  const [creatorError, setCreatorError] = useState<string | null>(null);
  const [expiringDays, setExpiringDays] = useState(initialExpiringDays);

  useEffect(() => {
    setExpiringDays(initialExpiringDays);
  }, [initialExpiringDays]);

  useEffect(() => {
    void reload();
  }, []);

  useEffect(() => {
    if (!saveError) {
      return;
    }
    const timer = window.setTimeout(() => setSaveError(null), 4000);
    return () => window.clearTimeout(timer);
  }, [saveError]);

  async function reload(options: ReloadOptions = {}) {
    const { silent = false, preserveDraft = false } = options;
    if (silent) {
      setRefreshing(true);
    } else {
      setLoading(true);
    }
    try {
      const [channelData, eventTypeData] = await Promise.all([
        api.get<NotificationChannel[]>("/notifications/channels"),
        api.get<NotificationEventType[]>("/notifications/event-types"),
      ]);
      const nextChannels = channelData ?? [];
      setChannels(nextChannels);
      setEventTypes(eventTypeData ?? []);
      setError(null);
      if (expandedId != null) {
        const current = nextChannels.find((item) => item.id === expandedId) ?? null;
        if (!current) {
          setExpandedId(null);
          setDraft(null);
          setDraftDirty(false);
        } else if (!preserveDraft || !draftDirty) {
          setDraft(makeChannelDraft(current));
          setDraftDirty(false);
        }
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "通知渠道加载失败");
    } finally {
      if (silent) {
        setRefreshing(false);
      } else {
        setLoading(false);
      }
    }
  }

  function beginCreate(type: ChannelType) {
    setCreatorDraft(makeCreatorDraft(type, channels));
    setCreatorError(null);
    setCreatorOpen(false);
  }

  async function createChannel() {
    if (!creatorDraft) {
      return;
    }
    setCreatingType(creatorDraft.type);
    try {
      const created = await api.post<NotificationChannel>("/notifications/channels", {
        ...buildChannelPayload(creatorDraft),
        enabled: false,
      });
      setChannels((current) => [...current, created]);
      setExpandedId(created.id);
      setDraft(makeChannelDraft(created));
      setDraftDirty(false);
      setSaveError(null);
      setDeleteError(null);
      setCreatorDraft(null);
      setCreatorError(null);
      setCreatorOpen(false);
      toast("渠道已创建");
    } catch (err) {
      setCreatorError(err instanceof Error ? err.message : "创建渠道失败");
    } finally {
      setCreatingType(null);
    }
  }

  async function saveDraft(field: string, nextDraft = draft) {
    if (!nextDraft) {
      return;
    }
    if (!nextDraft.name.trim()) {
      setSaveError("渠道名称不能为空");
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
      setDraftDirty(false);
      setSaveError(null);
      setDeleteError(null);
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : "保存失败");
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
      setExpiringDaysError(null);
    } catch (err) {
      toast(err instanceof Error ? err.message : "保存阈值失败", "error");
      setExpiringDaysError(err instanceof Error ? err.message : "保存阈值失败");
    }
  }

  async function toggleEnabled(id: number, enabled: boolean) {
    const previousChannels = channels;
    const previousDraft = draft;
    setTogglingId(id);
    setChannels((current) =>
      current.map((item) => (item.id === id ? { ...item, enabled } : item)),
    );
    if (expandedId === id && draft) {
      setDraft({ ...draft, enabled });
    }
    try {
      await api.post(`/notifications/channels/${id}/enabled`, { enabled });
    } catch (err) {
      setChannels(previousChannels);
      setDraft(previousDraft);
      toast(err instanceof Error ? err.message : "更新渠道状态失败", "error");
    } finally {
      setTogglingId(null);
    }
  }

  async function deleteChannel(id: number) {
    setDeletingId(id);
    try {
      await api.delete(`/notifications/channels/${id}`);
      const nextChannels = channels.filter((item) => item.id !== id);
      setChannels(nextChannels);
      setDeleteError(null);
      if (expandedId === id) {
        setExpandedId(null);
        setDraft(null);
        setDraftDirty(false);
      }
      toast("渠道已删除");
    } catch (err) {
      const message = err instanceof Error ? err.message : "删除渠道失败";
      toast(message, "error");
      setDeleteError(message);
    } finally {
      setDeletingId(null);
    }
  }

  const busy = loading || refreshing || creatingType != null;
  const channelDrafts = channels.map((item) =>
    expandedId === item.id && draft ? draft : makeChannelDraft(item),
  );

  return (
    <section className="surface-section overflow-hidden px-5 py-5 sm:px-6">
      <div className="rounded-[1.7rem] border border-white/75 bg-[linear-gradient(180deg,rgba(255,255,255,0.92),rgba(243,240,248,0.76))] px-4 py-4 shadow-[0_30px_80px_-54px_rgba(33,40,63,0.32)] sm:px-5">
        <SectionHeading
          title="Notifications"
          description="围绕 channel 管理订阅、模板、重试和投递反馈。"
          action={
            <div className="flex flex-wrap items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                className="rounded-full"
                onClick={() => void reload({ silent: true, preserveDraft: true })}
                disabled={busy}
              >
                <RefreshCw
                  className={`size-4 ${(loading || refreshing) ? "animate-spin" : ""}`}
                />
                Refresh
              </Button>
              {channels.length ? (
                <Button
                  className="rounded-full"
                  onClick={() => setCreatorOpen((current) => !current)}
                  disabled={busy}
                >
                  <Plus className="size-4" />
                  {creatorOpen ? "Hide Types" : "Add Channel"}
                  <ChevronDown
                    className={cn(
                      "size-4 transition-transform",
                      creatorOpen ? "rotate-180" : "",
                    )}
                  />
                </Button>
              ) : null}
            </div>
          }
        />

        <div className="mt-6 space-y-5">
          {creatorOpen ? (
            <div className="rounded-[1.4rem] border border-white/75 bg-[linear-gradient(180deg,rgba(255,255,255,0.86),rgba(246,243,249,0.78))] px-4 py-4 shadow-[0_20px_56px_-48px_rgba(33,40,63,0.24)]">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="space-y-1">
                  <p className="text-sm font-medium text-moon-800">选择渠道类型</p>
                  <p className="text-xs leading-5 text-moon-400">
                    创建后会直接展开当前 channel，继续填写 webhook、订阅和模板。
                  </p>
                </div>
              </div>
              <div className="mt-4 flex flex-wrap gap-2">
                {Object.entries(CHANNEL_TYPE_META).map(([type, meta]) => (
                  <Button
                    key={type}
                    variant="outline"
                    className="rounded-full"
                    onClick={() => beginCreate(type as ChannelType)}
                    disabled={busy}
                  >
                    <Plus className="size-4" />
                    {meta.label}
                  </Button>
                ))}
              </div>
            </div>
          ) : null}

          {creatorDraft ? (
            <CreateChannelPanel
              draft={creatorDraft}
              creating={creatingType === creatorDraft.type}
              error={creatorError}
              onDraftChange={setCreatorDraft}
              onCreate={() => void createChannel()}
              onCancel={() => {
                setCreatorDraft(null);
                setCreatorError(null);
              }}
            />
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

          {!loading && !channels.length && !creatorDraft ? (
            <EmptyState onCreate={beginCreate} />
          ) : null}

          {channels.length ? (
            <ChannelAccordion
              channels={channelDrafts}
              expandedId={expandedId}
              draft={draft}
              eventTypes={eventTypes}
              expiringDays={expiringDays}
              expiringDaysError={expiringDaysError}
              togglingId={togglingId}
              deletingId={deletingId}
              savingField={savingField}
              saveError={saveError}
              deleteError={deleteError}
              onExpand={(id) => {
                setSaveError(null);
                setDeleteError(null);
                setExpandedId(id);
                const matched =
                  id == null ? null : channels.find((item) => item.id === id) ?? null;
                setDraft(matched ? makeChannelDraft(matched) : null);
                setDraftDirty(false);
              }}
              onToggleEnabled={toggleEnabled}
              onDraftChange={(next) => {
                setDraft(next);
                setDraftDirty(true);
              }}
              onCommit={saveDraft}
              onSaveExpiringDays={(value) => void saveExpiringDays(value)}
              onDelete={(id) => void deleteChannel(id)}
              onRefresh={() => reload({ silent: true, preserveDraft: true })}
            />
          ) : null}
        </div>
      </div>
    </section>
  );
}

function buildCreateConfig(type: ChannelType) {
  return { ...CHANNEL_TYPE_META[type].defaults };
}

function makeCreatorDraft(
  type: ChannelType,
  channels: NotificationChannel[],
): NotificationChannelDraft {
  return {
    id: 0,
    name: defaultChannelName(type, channels),
    type,
    enabled: false,
    config: buildCreateConfig(type),
    preservedSecrets: {},
    subscriptions: [],
    title_template: "",
    body_template: "",
    retry_max_attempts: DEFAULT_RETRY_SCHEDULE.length,
    retry_schedule_seconds: DEFAULT_RETRY_SCHEDULE,
    created_at: "",
    updated_at: "",
    last_delivery: null,
    recent_deliveries: [],
  };
}
