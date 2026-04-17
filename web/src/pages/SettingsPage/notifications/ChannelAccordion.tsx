import { useEffect, useRef } from "react";

import ChannelDetail from "./ChannelDetail";
import ChannelRow from "./ChannelRow";
import type { NotificationChannelDraft, NotificationEventType } from "./types";

export default function ChannelAccordion({
  channels,
  expandedId,
  draft,
  eventTypes,
  expiringDays,
  togglingId,
  savingField,
  error,
  onExpand,
  onToggleEnabled,
  onDraftChange,
  onCommit,
  onSaveExpiringDays,
  onDelete,
  onRefresh,
}: {
  channels: NotificationChannelDraft[];
  expandedId: number | null;
  draft: NotificationChannelDraft | null;
  eventTypes: NotificationEventType[];
  expiringDays: number;
  togglingId: number | null;
  savingField: string | null;
  error: string | null;
  onExpand: (id: number | null) => void;
  onToggleEnabled: (id: number, enabled: boolean) => void;
  onDraftChange: (next: NotificationChannelDraft) => void;
  onCommit: (field: string, next?: NotificationChannelDraft) => void;
  onSaveExpiringDays: (value: number) => void;
  onDelete: (id: number) => void;
  onRefresh: () => Promise<void>;
}) {
  const rowRefs = useRef<Record<number, HTMLDivElement | null>>({});

  useEffect(() => {
    if (expandedId == null) {
      return;
    }
    rowRefs.current[expandedId]?.scrollIntoView({
      block: "start",
      behavior: "smooth",
    });
  }, [expandedId]);

  return (
    <div className="space-y-3">
      {channels.map((channel) => {
        const expanded = channel.id === expandedId;
        const display = expanded && draft ? draft : channel;

        return (
          <div
            key={channel.id}
            ref={(node) => {
              rowRefs.current[channel.id] = node;
            }}
            className="overflow-hidden rounded-[1.5rem] border border-white/75 bg-[linear-gradient(180deg,rgba(255,255,255,0.86),rgba(247,245,250,0.78))] shadow-[0_24px_60px_-48px_rgba(33,40,63,0.3)]"
          >
            <ChannelRow
              channel={display}
              expanded={expanded}
              toggling={togglingId === channel.id}
              onToggleExpand={() => onExpand(expanded ? null : channel.id)}
              onToggleEnabled={(enabled) => onToggleEnabled(channel.id, enabled)}
            />

            <div
              className="grid transition-[grid-template-rows] duration-300 ease-out"
              style={{ gridTemplateRows: expanded ? "1fr" : "0fr" }}
            >
              <div className="overflow-hidden">
                <div
                  className={`border-t border-moon-200/35 pt-4 transition-[opacity,transform] duration-200 ease-out ${
                    expanded
                      ? "translate-y-0 opacity-100 delay-75"
                      : "translate-y-1 opacity-0"
                  }`}
                >
                  {expanded && draft ? (
                    <ChannelDetail
                      draft={draft}
                      eventTypes={eventTypes}
                      expiringDays={expiringDays}
                      savingField={savingField}
                      error={error}
                      onDraftChange={onDraftChange}
                      onCommit={onCommit}
                      onSaveExpiringDays={onSaveExpiringDays}
                      onDelete={() => onDelete(channel.id)}
                      onRefresh={onRefresh}
                    />
                  ) : null}
                </div>
              </div>
            </div>
          </div>
        );
      })}
    </div>
  );
}
