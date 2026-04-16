import { useMemo } from "react";
import {
  DndContext,
  PointerSensor,
  closestCenter,
  useDroppable,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import {
  SortableContext,
  arrayMove,
  rectSortingStrategy,
  useSortable,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import type { PoolMember } from "@/lib/types";

type Zone = "enabled" | "disabled";

function DropZone({
  id,
  title,
  description,
  compact = false,
  children,
}: {
  id: string;
  title: string;
  description: string;
  compact?: boolean;
  children: React.ReactNode;
}) {
  const { setNodeRef, isOver } = useDroppable({ id });

  return (
    <section
      ref={setNodeRef}
      className={`relative overflow-visible rounded-[1.95rem] border transition-colors ${
        compact
          ? isOver
            ? "border-lunar-300/65 bg-[linear-gradient(180deg,rgba(248,245,252,0.92),rgba(243,239,249,0.86))]"
            : "border-moon-200/60 bg-[linear-gradient(180deg,rgba(255,255,255,0.82),rgba(244,241,249,0.76))]"
          : isOver
            ? "border-lunar-300/65 bg-[linear-gradient(180deg,rgba(250,247,253,0.94),rgba(243,238,250,0.9))]"
            : "border-moon-200/60 bg-[linear-gradient(180deg,rgba(255,255,255,0.88),rgba(244,241,250,0.82))]"
      }`}
    >
      <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_top_right,rgba(154,147,201,0.14),transparent_44%)]" />
      <div className={compact ? "relative px-4 py-4" : "relative px-5 py-5 sm:px-6 sm:py-6"}>
        <div>
          <p className="eyebrow-label">{title}</p>
          <p className="mt-1 text-sm text-moon-500">{description}</p>
        </div>
        <div className={compact ? "mt-4" : "mt-5"}>{children}</div>
      </div>
    </section>
  );
}

function SortableMember({
  member,
  children,
}: {
  member: PoolMember;
  children: (options: { dragging: boolean }) => React.ReactNode;
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: String(member.id),
  });

  return (
    <div
      ref={setNodeRef}
      style={{
        transform: CSS.Transform.toString(transform),
        transition,
      }}
      {...attributes}
      {...listeners}
    >
      {children({ dragging: isDragging })}
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
  renderMember: (member: PoolMember, options: { dragging: boolean }) => React.ReactNode;
  onReorder: (memberIds: number[]) => void;
  onToggleEnabled: (member: PoolMember, enabled: boolean) => void;
}) {
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 6 } }));
  const enabledMembers = useMemo(() => members.filter((member) => member.enabled), [members]);
  const disabledMembers = useMemo(() => members.filter((member) => !member.enabled), [members]);

  function findZone(id: string): Zone | null {
    if (enabledMembers.some((member) => String(member.id) === id)) return "enabled";
    if (disabledMembers.some((member) => String(member.id) === id)) return "disabled";
    if (id === "enabled-drop") return "enabled";
    if (id === "disabled-drop") return "disabled";
    return null;
  }

  function handleDragEnd(event: DragEndEvent) {
    const activeId = String(event.active.id);
    const overId = event.over ? String(event.over.id) : "";
    if (!overId) return;

    const sourceZone = findZone(activeId);
    const targetZone = findZone(overId);
    if (!sourceZone || !targetZone) return;

    const sourceMember = members.find((member) => String(member.id) === activeId);
    if (!sourceMember) return;

    if (sourceZone !== targetZone) {
      onToggleEnabled(sourceMember, targetZone === "enabled");
      return;
    }

    const zoneMembers = sourceZone === "enabled" ? enabledMembers : disabledMembers;
    const oldIndex = zoneMembers.findIndex((member) => String(member.id) === activeId);
    const newIndex = zoneMembers.findIndex((member) => String(member.id) === overId);
    if (oldIndex < 0 || newIndex < 0 || oldIndex === newIndex) return;

    const reorderedZone = arrayMove(zoneMembers, oldIndex, newIndex);
    const untouched = sourceZone === "enabled" ? disabledMembers : enabledMembers;
    onReorder([...reorderedZone, ...untouched].map((member) => member.id));
  }

  return (
    <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
      <div className="space-y-6">
        <DropZone
          id="enabled-drop"
          title="Active Order"
          description="拖拽排列顺序即路由优先级。"
        >
          <SortableContext items={enabledMembers.map((member) => String(member.id))} strategy={rectSortingStrategy}>
            <div className="flex min-h-[25rem] flex-col gap-4">
              {enabledMembers.map((member) => (
                <SortableMember key={member.id} member={member}>
                  {(options) => renderMember(member, options)}
                </SortableMember>
              ))}
            </div>
          </SortableContext>
        </DropZone>

        <DropZone
          id="disabled-drop"
          title="Disabled Dock"
          description="拖入这里可停用账号，仍保留可见性和测试入口。"
          compact
        >
          <SortableContext items={disabledMembers.map((member) => String(member.id))} strategy={rectSortingStrategy}>
            <div className="flex flex-col gap-4">
              {disabledMembers.length === 0 ? (
                <div className="rounded-[1.3rem] border border-dashed border-moon-200/70 px-4 py-6 text-sm text-moon-400">
                  这里还没有停用账号。
                </div>
              ) : (
                disabledMembers.map((member) => (
                  <SortableMember key={member.id} member={member}>
                    {(options) => renderMember(member, options)}
                  </SortableMember>
                ))
              )}
            </div>
          </SortableContext>
        </DropZone>
      </div>
    </DndContext>
  );
}
