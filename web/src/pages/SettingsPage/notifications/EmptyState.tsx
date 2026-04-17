import { Plus } from "lucide-react";

import { Button } from "@/components/ui/button";

import { CHANNEL_TYPE_META, type ChannelType } from "./types";

export default function EmptyState({
  onCreate,
}: {
  onCreate: (type: ChannelType) => void;
}) {
  return (
    <div className="surface-outline hero-glow overflow-hidden px-5 py-6 sm:px-6">
      <div className="max-w-xl space-y-2">
        <p className="text-sm font-medium text-moon-800">还没有通知渠道</p>
        <p className="text-sm leading-6 text-moon-500">
          直接选择目标类型并创建，随后在展开态里补齐 webhook、订阅和模板覆盖。
        </p>
      </div>
      <div className="mt-5 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        {Object.entries(CHANNEL_TYPE_META).map(([type, meta]) => (
          <button
            key={type}
            type="button"
            className="group rounded-[1.35rem] border border-white/75 bg-white/78 px-4 py-4 text-left shadow-[0_20px_50px_-44px_rgba(33,40,63,0.28)] transition hover:-translate-y-0.5 hover:border-lunar-300/45"
            onClick={() => onCreate(type as ChannelType)}
          >
            <div className="flex items-center justify-between gap-3">
              <span
                className={`rounded-full px-2.5 py-1 text-[11px] tracking-[0.14em] ${meta.tone}`}
              >
                {meta.label}
              </span>
              <Plus className="size-4 text-moon-350 transition group-hover:text-moon-600" />
            </div>
            <p className="mt-3 text-sm leading-6 text-moon-500">
              {meta.description}
            </p>
          </button>
        ))}
      </div>
      <div className="mt-5">
        <Button variant="outline" className="rounded-full" disabled>
          <Plus className="size-4" />
          选择一种类型开始
        </Button>
      </div>
    </div>
  );
}
