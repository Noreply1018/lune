import { Trash2 } from "lucide-react";

import { Button } from "@/components/ui/button";

export default function DangerZone({
  error,
  deleting,
  onDeleteRequest,
}: {
  error?: string | null;
  deleting?: boolean;
  onDeleteRequest: () => void;
}) {
  return (
    <section className="rounded-[1.2rem] border border-moon-200/60 bg-white/82 px-4 py-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="space-y-1">
          <p className="text-sm font-medium text-moon-800">删除通道</p>
          <p className="text-xs leading-5 text-moon-400">
            删除后待投递队列会被清空，历史记录仍保留在 Activity。
          </p>
        </div>
        <Button
          variant="outline"
          className="rounded-full border-status-red/30 text-status-red"
          onClick={onDeleteRequest}
          disabled={deleting}
        >
          <Trash2 className="size-4" />
          {deleting ? "Deleting..." : "Delete Channel"}
        </Button>
      </div>
      {error ? (
        <p className="mt-3 text-sm text-status-red">{error}</p>
      ) : null}
    </section>
  );
}
