import { useMemo, useState } from "react";
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  pointerWithin,
  rectIntersection,
  useDroppable,
  useSensor,
  useSensors,
  type CollisionDetection,
  type DragEndEvent,
  type DragStartEvent,
} from "@dnd-kit/core";
import {
  SortableContext,
  arrayMove,
  rectSortingStrategy,
  verticalListSortingStrategy,
  useSortable,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import type { PoolMember } from "@/lib/types";

type Zone = "enabled" | "disabled";

type RenderOptions = {
  dragging: boolean;
  priorityIndex?: number;
  dragHandleProps: React.HTMLAttributes<HTMLDivElement>;
};

function DropZone({
  id,
  title,
  description,
  tone = "active",
  children,
  childrenWrapperClassName,
}: {
  id: string;
  title: string;
  description: string;
  tone?: "active" | "disabled";
  children: React.ReactNode;
  childrenWrapperClassName?: string;
}) {
  const { setNodeRef, isOver } = useDroppable({ id });
  const isActive = tone === "active";

  return (
    <section
      ref={setNodeRef}
      className={`relative flex h-full min-h-[28rem] flex-col overflow-visible rounded-[1.6rem] border transition-colors ${
        isActive
          ? isOver
            ? "border-lunar-300/70 bg-[linear-gradient(180deg,rgba(250,247,253,0.94),rgba(243,238,250,0.9))]"
            : "border-moon-200/60 bg-[linear-gradient(180deg,rgba(255,255,255,0.88),rgba(244,241,250,0.82))]"
          : isOver
            ? "border-lunar-300/65 border-dashed bg-[linear-gradient(180deg,rgba(247,244,250,0.92),rgba(240,235,247,0.85))]"
            : "border-moon-200/55 border-dashed bg-[linear-gradient(180deg,rgba(250,248,253,0.78),rgba(243,240,249,0.72))]"
      }`}
    >
      {isActive ? (
        <div className="pointer-events-none absolute inset-0 rounded-[1.6rem] bg-[radial-gradient(circle_at_top_right,rgba(154,147,201,0.14),transparent_44%)]" />
      ) : null}
      <div className="relative flex h-full flex-col px-4 py-4 sm:px-5 sm:py-5">
        <div>
          <p className="eyebrow-label">{title}</p>
          <p className="mt-1 text-sm text-moon-500">{description}</p>
        </div>
        <div className={`mt-4 flex-1 ${childrenWrapperClassName ?? ""}`}>{children}</div>
      </div>
    </section>
  );
}

function SortableMember({
  member,
  priorityIndex,
  children,
}: {
  member: PoolMember;
  priorityIndex?: number;
  children: (options: RenderOptions) => React.ReactNode;
}) {
  const { listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: String(member.id),
  });

  // Only spread the pointer/keyboard listeners onto the card; we deliberately drop
  // dnd-kit's `attributes` (role="button", tabIndex) so the card doesn't masquerade
  // as a single button and swallow inner Info/Menu buttons in the accessibility tree.
  const handleProps = (listeners ?? {}) as React.HTMLAttributes<HTMLDivElement>;

  return (
    <div
      ref={setNodeRef}
      style={{
        transform: isDragging ? undefined : CSS.Transform.toString(transform),
        transition,
        opacity: isDragging ? 0 : 1,
        touchAction: "none",
      }}
    >
      {children({ dragging: isDragging, priorityIndex, dragHandleProps: handleProps })}
    </div>
  );
}

export default function DragSortArea({
  members,
  renderMember,
  onReorder,
  onToggleEnabled,
}: {
  members: PoolMember[];
  renderMember: (member: PoolMember, options: RenderOptions) => React.ReactNode;
  onReorder: (memberIds: number[]) => void;
  onToggleEnabled: (member: PoolMember, enabled: boolean) => void;
}) {
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 6 } }));
  const enabledMembers = useMemo(() => members.filter((member) => member.enabled), [members]);
  const disabledMembers = useMemo(() => members.filter((member) => !member.enabled), [members]);
  const [activeId, setActiveId] = useState<string | null>(null);
  const activeMember = activeId ? members.find((m) => String(m.id) === activeId) ?? null : null;
  const activePriorityIndex = (() => {
    if (!activeMember?.enabled) return undefined;
    const idx = enabledMembers.findIndex((m) => m.id === activeMember.id);
    return idx >= 0 ? idx + 1 : undefined;
  })();

  // Custom collision detection: SortableContext synthesizes oversized rects for items so its
  // own sort strategy works, which makes the last active card "claim" the empty Disabled zone
  // when measured purely by item rects. We instead anchor on the section's droppable id, then
  // only return items that actually belong to that zone — guaranteeing cross-zone drops fire.
  const collisionDetection: CollisionDetection = (args) => {
    const pointerHits = pointerWithin(args);
    const zoneHit = pointerHits.find(
      (hit) => hit.id === "enabled-drop" || hit.id === "disabled-drop",
    );
    if (zoneHit) {
      const zoneMembers = zoneHit.id === "enabled-drop" ? enabledMembers : disabledMembers;
      const memberIds = new Set(zoneMembers.map((member) => String(member.id)));
      const itemHits = pointerHits.filter((hit) => memberIds.has(String(hit.id)));
      if (itemHits.length > 0) return itemHits;
      return [zoneHit];
    }
    if (pointerHits.length > 0) return pointerHits;
    return rectIntersection(args);
  };

  function findZone(id: string): Zone | null {
    if (enabledMembers.some((member) => String(member.id) === id)) return "enabled";
    if (disabledMembers.some((member) => String(member.id) === id)) return "disabled";
    if (id === "enabled-drop") return "enabled";
    if (id === "disabled-drop") return "disabled";
    return null;
  }

  function handleDragStart(event: DragStartEvent) {
    setActiveId(String(event.active.id));
  }

  function handleDragCancel() {
    setActiveId(null);
  }

  function handleDragEnd(event: DragEndEvent) {
    setActiveId(null);
    const activeId = String(event.active.id);
    const overId = event.over ? String(event.over.id) : "";
    if (!overId) return;

    const sourceZone = findZone(activeId);
    const targetZone = findZone(overId);
    if (!sourceZone || !targetZone) return;

    const sourceMember = members.find((member) => String(member.id) === activeId);
    if (!sourceMember) return;

    if (sourceZone !== targetZone) {
      const remainingSource = (sourceZone === "enabled" ? enabledMembers : disabledMembers).filter(
        (member) => member.id !== sourceMember.id,
      );
      const targetList = targetZone === "enabled" ? enabledMembers : disabledMembers;
      const overIndex = targetList.findIndex((member) => String(member.id) === overId);
      const insertAt = overIndex >= 0 ? overIndex : targetList.length;
      const nextTarget = [
        ...targetList.slice(0, insertAt),
        sourceMember,
        ...targetList.slice(insertAt),
      ];
      const nextEnabled = targetZone === "enabled" ? nextTarget : remainingSource;
      const nextDisabled = targetZone === "disabled" ? nextTarget : remainingSource;
      onToggleEnabled(sourceMember, targetZone === "enabled");
      onReorder([...nextEnabled, ...nextDisabled].map((member) => member.id));
      return;
    }

    const zoneMembers = sourceZone === "enabled" ? enabledMembers : disabledMembers;
    const oldIndex = zoneMembers.findIndex((member) => String(member.id) === activeId);
    const newIndex = zoneMembers.findIndex((member) => String(member.id) === overId);
    if (oldIndex < 0 || newIndex < 0 || oldIndex === newIndex) return;

    const reorderedZone = arrayMove(zoneMembers, oldIndex, newIndex);
    const nextEnabled = sourceZone === "enabled" ? reorderedZone : enabledMembers;
    const nextDisabled = sourceZone === "disabled" ? reorderedZone : disabledMembers;
    onReorder([...nextEnabled, ...nextDisabled].map((member) => member.id));
  }

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={collisionDetection}
      onDragStart={handleDragStart}
      onDragCancel={handleDragCancel}
      onDragEnd={handleDragEnd}
    >
      <div className="grid grid-cols-1 items-stretch gap-5 md:grid-cols-[minmax(0,3fr)_minmax(0,1fr)]">
        <DropZone
          id="enabled-drop"
          title="Active Pool"
          description="左右拖拽即调整路由优先级，越靠前的账号优先承接请求。"
        >
          <SortableContext
            items={enabledMembers.map((member) => String(member.id))}
            strategy={rectSortingStrategy}
          >
            {enabledMembers.length === 0 ? (
              <EmptyDropHint label="把账号拖进来以启用" />
            ) : (
              <div
                className="grid gap-3"
                style={{ gridTemplateColumns: "repeat(auto-fill, minmax(15.5rem, 1fr))" }}
              >
                {enabledMembers.map((member, index) => (
                  <SortableMember key={member.id} member={member} priorityIndex={index + 1}>
                    {(options) => renderMember(member, options)}
                  </SortableMember>
                ))}
              </div>
            )}
          </SortableContext>
        </DropZone>

        <DropZone
          id="disabled-drop"
          title="Disabled Dock"
          description="拖到这里即停用，仍保留账号信息以便随时启用。"
          tone="disabled"
        >
          <SortableContext
            items={disabledMembers.map((member) => String(member.id))}
            strategy={verticalListSortingStrategy}
          >
            {disabledMembers.length === 0 ? (
              <EmptyDropHint label="拖账号到这里即停用" tone="disabled" />
            ) : (
              <div className="flex flex-col gap-2.5">
                {disabledMembers.map((member) => (
                  <SortableMember key={member.id} member={member}>
                    {(options) => renderMember(member, options)}
                  </SortableMember>
                ))}
              </div>
            )}
          </SortableContext>
        </DropZone>
      </div>
      <DragOverlay dropAnimation={null}>
        {activeMember
          ? renderMember(activeMember, {
              dragging: true,
              priorityIndex: activePriorityIndex,
              dragHandleProps: {},
            })
          : null}
      </DragOverlay>
    </DndContext>
  );
}

function EmptyDropHint({
  label,
  tone = "active",
}: {
  label: string;
  tone?: "active" | "disabled";
}) {
  return (
    <div
      className={`flex h-full min-h-[10rem] items-center justify-center rounded-[1.2rem] border border-dashed px-4 py-6 text-center text-sm ${
        tone === "active"
          ? "border-moon-300/60 bg-white/40 text-moon-400"
          : "border-moon-300/55 bg-white/30 text-moon-400"
      }`}
    >
      {label}
    </div>
  );
}
