import { Plus } from "lucide-react";

import { Button } from "@/components/ui/button";

import { CHANNEL_TYPE_META, type ChannelType } from "./types";

export default function EmptyState({
  onCreate,
}: {
  onCreate: (type: ChannelType) => void;
}) {
  return (
    <div className="rounded-[1.4rem] border border-dashed border-moon-200/60 bg-white/72 px-4 py-5 sm:px-5">
      <div className="space-y-1">
        <p className="text-sm font-medium text-moon-800">还没有通知渠道</p>
        <p className="text-xs leading-5 text-moon-400">
          直接选择目标类型并创建，随后在展开态里补齐 webhook、订阅和模板覆盖。
        </p>
      </div>
      <div className="mt-4 flex flex-wrap gap-2">
        {Object.entries(CHANNEL_TYPE_META).map(([type, meta]) => (
          <Button
            key={type}
            variant="outline"
            className="rounded-full"
            onClick={() => onCreate(type as ChannelType)}
          >
            <Plus className="size-4" />
            {meta.label}
          </Button>
        ))}
      </div>
    </div>
  );
}
