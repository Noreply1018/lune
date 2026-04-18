import { Database } from "lucide-react";

import { formatBytes, shortDate } from "@/lib/fmt";
import type { DataRetentionSummary } from "@/lib/types";

type CurrentUsageCardProps = {
  summary: DataRetentionSummary | null;
};

export default function CurrentUsageCard({ summary }: CurrentUsageCardProps) {
  const totalLogs = Number(summary?.total_logs ?? 0);
  const totalDeliveries = Number(summary?.total_notification_deliveries ?? 0);
  const outboxPending = Number(summary?.outbox_pending_count ?? 0);
  const outboxDropped = Number(summary?.outbox_dropped_count ?? 0);
  const outboxTotal = outboxPending + outboxDropped;

  const logsRange = formatRange(summary?.oldest_log_at, summary?.newest_log_at);
  const deliveriesRange = formatRange(
    summary?.notification_deliveries_oldest_at,
    summary?.notification_deliveries_newest_at,
  );

  return (
    <div className="flex h-full flex-col rounded-[1.4rem] border border-moon-200/55 bg-[linear-gradient(180deg,rgba(255,255,255,0.94),rgba(244,241,250,0.78))] px-5 py-4 shadow-[0_24px_60px_-50px_rgba(74,68,108,0.32)]">
      <div className="flex items-center gap-2">
        <span className="flex size-7 items-center justify-center rounded-full bg-lunar-100/70 text-lunar-600">
          <Database className="size-3.5" />
        </span>
        <div>
          <p className="text-sm font-semibold text-moon-800">当前占用</p>
          <p className="text-[11px] tracking-[0.14em] text-moon-350">
            DATABASE SNAPSHOT
          </p>
        </div>
      </div>

      <div className="mt-4 space-y-1">
        <p className="text-[11px] tracking-[0.16em] text-moon-300">
          数据库文件
        </p>
        <p className="text-2xl font-semibold tabular-nums text-moon-800">
          {formatBytes(Number(summary?.database_size_bytes ?? 0))}
        </p>
      </div>

      <div className="mt-4 space-y-2.5">
        <UsageRow
          label="请求日志"
          primary={`${totalLogs.toLocaleString()} 条 · ${formatBytes(Number(summary?.logs_size_bytes ?? 0))}`}
          secondary={logsRange}
        />
        <UsageRow
          label="通知历史"
          primary={`${totalDeliveries.toLocaleString()} 条`}
          secondary={deliveriesRange}
        />
        <UsageRow
          label="通知队列"
          primary={`${outboxTotal.toLocaleString()} 条`}
          secondary={
            outboxTotal > 0
              ? `等待 ${outboxPending.toLocaleString()} · 已丢弃 ${outboxDropped.toLocaleString()}`
              : "无积压"
          }
        />
      </div>
    </div>
  );
}

function UsageRow({
  label,
  primary,
  secondary,
}: {
  label: string;
  primary: string;
  secondary: string;
}) {
  return (
    <div className="flex flex-col gap-0.5 rounded-[1.05rem] border border-moon-200/45 bg-white/55 px-3.5 py-2.5">
      <div className="flex items-center justify-between gap-3">
        <span className="text-xs font-medium text-moon-500">{label}</span>
        <span className="text-sm font-medium tabular-nums text-moon-800">
          {primary}
        </span>
      </div>
      <span className="text-[11px] text-moon-400">{secondary}</span>
    </div>
  );
}

function formatRange(
  oldest: string | null | undefined,
  newest: string | null | undefined,
): string {
  if (!oldest && !newest) return "暂无记录";
  if (oldest && newest) {
    if (oldest === newest) return shortDate(oldest);
    return `${shortDate(oldest)} → ${shortDate(newest)}`;
  }
  return shortDate((oldest ?? newest) as string);
}
