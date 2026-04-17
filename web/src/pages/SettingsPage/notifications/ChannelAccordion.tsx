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
  expiringDaysError,
  togglingId,
  savingField,
  saveError,
  deleteError,
  deletingId,
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
  expiringDaysError: string | null;
  togglingId: number | null;
  savingField: string | null;
  saveError: string | null;
  deleteError: string | null;
  deletingId: number | null;
  onExpand: (id: number | null) => void;
  onToggleEnabled: (id: number, enabled: boolean) => void;
  onDraftChange: (next: NotificationChannelDraft) => void;
  onCommit: (field: string, next?: NotificationChannelDraft) => void;
  onSaveExpiringDays: (value: number) => void;
  onDelete: (id: number) => void;
  onRefresh: () => Promise<void>;
}) {
  const rowRefs = useRef<Map<number, HTMLDivElement>>(new Map());
  const previousExpandedId = useRef<number | null>(null);

  useEffect(() => {
    if (expandedId == null) {
      previousExpandedId.current = expandedId;
      return;
    }
    if (previousExpandedId.current == null) {
      previousExpandedId.current = expandedId;
      return;
    }
    // Use instant scroll so the page doesn't visibly fight the expand animation.
    rowRefs.current.get(expandedId)?.scrollIntoView({
      block: "start",
      behavior: "auto",
    });
    previousExpandedId.current = expandedId;
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
              if (node) {
                rowRefs.current.set(channel.id, node);
              } else {
                rowRefs.current.delete(channel.id);
              }
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
              className="grid transition-[grid-template-rows] duration-200 ease-out"
              style={{ gridTemplateRows: expanded ? "1fr" : "0fr" }}
            >
              <div className="overflow-hidden">
                <div className="border-t border-moon-200/35 pt-4">
                  {expanded && draft ? (
                    <ChannelDetail
                      draft={draft}
                      eventTypes={eventTypes}
                      expiringDays={expiringDays}
                      expiringDaysError={expiringDaysError}
                      savingField={savingField}
                      saveError={saveError}
                      deleteError={deleteError}
                      deleting={deletingId === channel.id}
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
