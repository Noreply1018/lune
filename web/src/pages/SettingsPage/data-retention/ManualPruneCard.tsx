import { BrushCleaning, RefreshCw, Sparkles, Trash2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { formatBytes } from "@/lib/fmt";
import type { DataRetentionPreview } from "@/lib/types";

type ManualPruneCardProps = {
  retentionDays: number;
  preview: DataRetentionPreview | null;
  previewOpen: boolean;
  previewLoading: boolean;
  pruning: boolean;
  onOpenPreview: () => void;
  onCancelPreview: () => void;
  onConfirm: () => void;
};

export default function ManualPruneCard({
  retentionDays,
  preview,
  previewOpen,
  previewLoading,
  pruning,
  onOpenPreview,
  onCancelPreview,
  onConfirm,
}: ManualPruneCardProps) {
  const enabled = retentionDays > 0;
  const totalToDelete =
    Number(preview?.logs_to_delete ?? 0) +
    Number(preview?.deliveries_to_delete ?? 0) +
    Number(preview?.outbox_to_delete ?? 0);

  return (
    <div className="flex h-full flex-col rounded-[1.4rem] border border-moon-200/55 bg-[linear-gradient(180deg,rgba(255,255,255,0.94),rgba(244,241,250,0.78))] px-5 py-4 shadow-[0_24px_60px_-50px_rgba(74,68,108,0.32)]">
      <div className="flex items-center gap-2">
        <span className="flex size-7 items-center justify-center rounded-full bg-lunar-100/70 text-lunar-600">
          <BrushCleaning className="size-3.5" />
        </span>
        <div>
          <p className="text-sm font-semibold text-moon-800">立即清理</p>
          <p className="text-[11px] tracking-[0.14em] text-moon-350">
            RUN PRUNE NOW
          </p>
        </div>
      </div>

      <div className="mt-4 flex-1">
        {!previewOpen ? (
          <div className="flex h-full flex-col justify-between gap-4">
            <p className="text-xs leading-5 text-moon-500">
              {enabled
                ? "按当前保留规则立即跑一次清理。预览会先告诉你会删掉什么，再决定是否执行。"
                : "保留天数为 0 时无清理目标，请先在左侧设置一个保留天数。"}
            </p>
            <Button
              variant="outline"
              size="sm"
              className="rounded-full"
              onClick={onOpenPreview}
              disabled={!enabled || previewLoading}
            >
              {previewLoading ? (
                <RefreshCw className="size-4 animate-spin" />
              ) : (
                <Sparkles className="size-4" />
              )}
              预览本次清理
            </Button>
          </div>
        ) : (
          <div className="flex h-full flex-col gap-3">
            <div className="rounded-[1.1rem] border border-moon-200/45 bg-white/55 px-3.5 py-3">
              <p className="text-[11px] tracking-[0.16em] text-moon-300">
                将会删除
              </p>
              <p className="mt-1 text-xl font-semibold tabular-nums text-moon-800">
                {totalToDelete.toLocaleString()} 条
              </p>
              <div className="mt-2 space-y-1 text-xs text-moon-500">
                <PreviewRow
                  label={`请求日志（> ${preview?.retention_days ?? 0} 天）`}
                  value={`${Number(preview?.logs_to_delete ?? 0).toLocaleString()} 条 · ${formatBytes(Number(preview?.logs_to_delete_size_bytes ?? 0))}`}
                />
                <PreviewRow
                  label={`通知历史（> ${preview?.retention_days ?? 0} 天）`}
                  value={`${Number(preview?.deliveries_to_delete ?? 0).toLocaleString()} 条`}
                />
                <PreviewRow
                  label={`通知队列 · dropped（> ${preview?.outbox_safety_days ?? 0} 天）`}
                  value={`${Number(preview?.outbox_to_delete ?? 0).toLocaleString()} 条`}
                />
              </div>
              <p className="mt-2 text-[11px] leading-4 text-moon-400">
                队列仅删除 status = dropped 的项；pending/retrying 会被保留。
              </p>
            </div>
            <div className="mt-auto flex items-center justify-end gap-2">
              <Button
                variant="outline"
                size="sm"
                className="rounded-full"
                onClick={onCancelPreview}
                disabled={pruning}
              >
                取消
              </Button>
              <Button
                size="sm"
                className="rounded-full"
                onClick={onConfirm}
                disabled={pruning || totalToDelete === 0}
              >
                {pruning ? (
                  <RefreshCw className="size-4 animate-spin" />
                ) : (
                  <Trash2 className="size-4" />
                )}
                {totalToDelete === 0 ? "无可清理" : "确认清理"}
              </Button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

function PreviewRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-3">
      <span className="text-moon-500">{label}</span>
      <span className="font-medium tabular-nums text-moon-700">{value}</span>
    </div>
  );
}
