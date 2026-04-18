import { useCallback, useEffect, useState } from "react";

import SectionHeading from "@/components/SectionHeading";
import { toast } from "@/components/Feedback";
import { api } from "@/lib/api";
import type {
  DataRetentionPreview,
  DataRetentionSummary,
} from "@/lib/types";

import CurrentUsageCard from "./CurrentUsageCard";
import AutoPruneCard from "./AutoPruneCard";
import ManualPruneCard from "./ManualPruneCard";

type DataRetentionSectionProps = {
  retentionDays: number;
  savingRetention: boolean;
  onRetentionDaysChange: (value: number) => void;
  onRetentionDaysCommit: () => void;
  summary: DataRetentionSummary | null;
  onReloadSummary: () => Promise<void> | void;
};

export default function DataRetentionSection({
  retentionDays,
  savingRetention,
  onRetentionDaysChange,
  onRetentionDaysCommit,
  summary,
  onReloadSummary,
}: DataRetentionSectionProps) {
  const [preview, setPreview] = useState<DataRetentionPreview | null>(null);
  const [previewOpen, setPreviewOpen] = useState(false);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [pruning, setPruning] = useState(false);

  const openPreview = useCallback(async () => {
    setPreviewLoading(true);
    try {
      const data = await api.get<DataRetentionPreview>(
        "/settings/data-retention/preview",
      );
      setPreview(data);
      setPreviewOpen(true);
    } catch (err) {
      toast(err instanceof Error ? err.message : "预览清理结果失败", "error");
    } finally {
      setPreviewLoading(false);
    }
  }, []);

  const cancelPreview = useCallback(() => {
    setPreviewOpen(false);
    setPreview(null);
  }, []);

  const confirmPrune = useCallback(async () => {
    setPruning(true);
    try {
      const result = await api.post<{
        deleted_logs: number;
        deleted_notification_deliveries: number;
        deleted_notification_outbox: number;
      }>("/settings/data-retention/prune", {});
      await onReloadSummary();
      const total =
        Number(result.deleted_logs ?? 0) +
        Number(result.deleted_notification_deliveries ?? 0) +
        Number(result.deleted_notification_outbox ?? 0);
      toast(total > 0 ? `已清理 ${total} 条` : "已清理完成");
      setPreviewOpen(false);
      setPreview(null);
    } catch (err) {
      toast(err instanceof Error ? err.message : "清理失败", "error");
    } finally {
      setPruning(false);
    }
  }, [onReloadSummary]);

  // Close the preview automatically when retention drops to 0 — the preview
  // numbers are stale the moment auto-prune is disabled.
  useEffect(() => {
    if (retentionDays <= 0 && previewOpen) {
      setPreviewOpen(false);
      setPreview(null);
    }
  }, [retentionDays, previewOpen]);

  return (
    <section className="surface-section px-5 py-5 sm:px-6">
      <SectionHeading
        title="Data Retention"
        description="掌握当前库内占用，设定自动清理节奏，必要时立即腾空间。"
      />
      <div className="mt-5 grid gap-4 lg:grid-cols-3 lg:items-stretch">
        <CurrentUsageCard summary={summary} />
        <AutoPruneCard
          retentionDays={retentionDays}
          savingRetention={savingRetention}
          onRetentionDaysChange={onRetentionDaysChange}
          onRetentionDaysCommit={onRetentionDaysCommit}
          summary={summary}
        />
        <ManualPruneCard
          retentionDays={retentionDays}
          preview={preview}
          previewOpen={previewOpen}
          previewLoading={previewLoading}
          pruning={pruning}
          onOpenPreview={openPreview}
          onCancelPreview={cancelPreview}
          onConfirm={confirmPrune}
        />
      </div>
    </section>
  );
}
