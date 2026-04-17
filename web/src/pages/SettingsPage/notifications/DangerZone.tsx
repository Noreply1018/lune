import { Trash2 } from "lucide-react";

import { Button } from "@/components/ui/button";

export default function DangerZone({
  open,
  error,
  onToggle,
  onDelete,
}: {
  open: boolean;
  error?: string | null;
  onToggle: () => void;
  onDelete: () => void;
}) {
  return (
    <section className="rounded-[1.2rem] border border-status-red/20 bg-status-red/5 px-4 py-4">
      <div className="flex items-center justify-between gap-3">
        <div>
          <p className="text-sm font-medium text-status-red">危险操作</p>
          <p className="mt-1 text-xs leading-5 text-status-red/80">
            删除后该 channel 的待投递队列会被清空，历史记录保留在 Activity。
          </p>
        </div>
        <Button
          variant="outline"
          className="rounded-full border-status-red/25 text-status-red"
          onClick={onToggle}
        >
          {open ? "收起" : "展开"}
        </Button>
      </div>
      {open ? (
        <div className="mt-4 flex flex-wrap items-center justify-between gap-3 border-t border-status-red/15 pt-4">
          <Button
            variant="outline"
            className="rounded-full border-status-red/30 text-status-red"
            onClick={onDelete}
          >
            <Trash2 className="size-4" />
            Delete Channel
          </Button>
          {error ? <p className="text-sm text-status-red">{error}</p> : null}
        </div>
      ) : null}
    </section>
  );
}
