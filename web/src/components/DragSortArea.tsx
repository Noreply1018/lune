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
  children,
}: {
  id: string;
  title: string;
  description: string;
  children: React.ReactNode;
}) {
  const { setNodeRef, isOver } = useDroppable({ id });

  return (
    <section
      ref={setNodeRef}
      className={`rounded-[1.8rem] border px-4 py-4 transition-colors ${
        isOver ? "border-lunar-300 bg-lunar-100/35" : "border-moon-200/60 bg-white/38"
      }`}
    >
      <div className="mb-4 flex items-center justify-between gap-3">
        <div>
          <p className="eyebrow-label">{title}</p>
          <p className="mt-1 text-sm text-moon-500">{description}</p>
        </div>
      </div>
      {children}
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
      <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_20rem]">
        <DropZone
          id="enabled-drop"
          title="Active Order"
          description="拖拽排列顺序即路由优先级。"
        >
          <SortableContext items={enabledMembers.map((member) => String(member.id))} strategy={rectSortingStrategy}>
            <div className="grid gap-4 md:grid-cols-2 2xl:grid-cols-3">
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
        >
          <SortableContext items={disabledMembers.map((member) => String(member.id))} strategy={rectSortingStrategy}>
            <div className="grid gap-4">
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
