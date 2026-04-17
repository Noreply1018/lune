import { ChevronDown, Trash2 } from "lucide-react";

import { Button } from "@/components/ui/button";

export default function DangerZone({
  open,
  error,
  deleting,
  onOpenChange,
  onDeleteRequest,
}: {
  open: boolean;
  error?: string | null;
  deleting?: boolean;
  onOpenChange: (next: boolean) => void;
  onDeleteRequest: () => void;
}) {
  return (
    <section className="rounded-[1.2rem] border border-status-red/20 bg-[linear-gradient(180deg,rgba(255,241,242,0.84),rgba(255,247,247,0.72))] px-4 py-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <p className="text-sm font-medium text-status-red">危险操作</p>
          <p className="text-xs leading-5 text-status-red/80">
            删除后该 channel 的待投递队列会被清空，历史记录仍保留在 Activity。
          </p>
        </div>
        <Button
          variant="outline"
          className="rounded-full border-status-red/30 text-status-red"
          onClick={() => onOpenChange(!open)}
        >
          <ChevronDown className={`size-4 transition-transform ${open ? "rotate-180" : ""}`} />
          {open ? "收起" : "展开"}
        </Button>
      </div>
      {open ? (
        <div className="mt-4 flex flex-wrap items-center justify-between gap-3 border-t border-status-red/15 pt-4">
          <Button
            variant="outline"
            className="rounded-full border-status-red/30 text-status-red"
            onClick={onDeleteRequest}
            disabled={deleting}
          >
            <Trash2 className="size-4" />
            {deleting ? "Deleting..." : "Delete Channel"}
          </Button>
          {error ? <p className="text-sm text-status-red">{error}</p> : null}
        </div>
      ) : null}
    </section>
  );
}
