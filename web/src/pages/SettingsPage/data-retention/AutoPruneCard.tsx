import { useEffect, useRef, useState, type KeyboardEvent } from "react";
import { Clock3, RefreshCw } from "lucide-react";

import { Input } from "@/components/ui/input";
import { relativeTime } from "@/lib/fmt";
import { cn } from "@/lib/utils";
import type { DataRetentionSummary } from "@/lib/types";

type AutoPruneCardProps = {
  retentionDays: number;
  savingRetention: boolean;
  onRetentionDaysCommit: (value: number) => void;
  summary: DataRetentionSummary | null;
};

export default function AutoPruneCard({
  retentionDays,
  savingRetention,
  onRetentionDaysCommit,
  summary,
}: AutoPruneCardProps) {
  const enabled = retentionDays > 0;
  const lastPruneAt = summary?.last_prune_at ?? null;
  const lastDeletedLogs = Number(summary?.last_prune_deleted_logs ?? 0);
  const lastDeletedDeliveries = Number(
    summary?.last_prune_deleted_deliveries ?? 0,
  );
  const lastDeletedOutbox = Number(summary?.last_prune_deleted_outbox ?? 0);
  const lastDeleted =
    lastDeletedLogs + lastDeletedDeliveries + lastDeletedOutbox;

  const [draft, setDraft] = useState(`${retentionDays}`);
  // Track the last server-committed value locally so the useEffect below
  // only resyncs when the prop drifts from what we already know — e.g.
  // parent rolled back after a PUT failure. A naive setDraft on every
  // prop bump would race the user's next keystroke.
  const committedRef = useRef(retentionDays);

  useEffect(() => {
    if (retentionDays === committedRef.current) return;
    committedRef.current = retentionDays;
    setDraft(`${retentionDays}`);
  }, [retentionDays]);

  function handleKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "Enter") {
      event.preventDefault();
      event.currentTarget.blur();
    }
  }

  function commit() {
    const trimmed = draft.trim();
    const parsed = Number(trimmed);
    if (trimmed === "" || !Number.isFinite(parsed) || parsed < 0) {
      // Empty / NaN / negative: roll back display rather than silently
      // disabling auto-prune (0) or sending a bad value to the server.
      setDraft(`${committedRef.current}`);
      return;
    }
    const normalized = Math.floor(parsed);
    setDraft(`${normalized}`);
    if (normalized === committedRef.current) return;
    committedRef.current = normalized;
    onRetentionDaysCommit(normalized);
  }

  return (
    <div className="flex h-full flex-col rounded-[1.4rem] border border-moon-200/55 bg-[linear-gradient(180deg,rgba(255,255,255,0.94),rgba(244,241,250,0.78))] px-5 py-4 shadow-[0_24px_60px_-50px_rgba(74,68,108,0.32)]">
      <div className="flex items-center gap-2">
        <span className="flex size-7 items-center justify-center rounded-full bg-lunar-100/70 text-lunar-600">
          <Clock3 className="size-3.5" />
        </span>
        <div>
          <p className="text-sm font-semibold text-moon-800">自动清理</p>
          <p className="text-[11px] tracking-[0.14em] text-moon-350">
            RETENTION WINDOW
          </p>
        </div>
      </div>

      <div className="mt-4 space-y-3">
        <label
          className="text-[11px] tracking-[0.16em] text-moon-300"
          htmlFor="data-retention-days"
        >
          保留天数
        </label>
        <div className="flex items-center gap-2">
          <Input
            id="data-retention-days"
            type="number"
            value={draft}
            min={0}
            className="h-10 w-28 text-right text-base font-medium tabular-nums"
            onChange={(event) => setDraft(event.target.value)}
            onBlur={commit}
            onKeyDown={handleKeyDown}
          />
          <span className="text-sm text-moon-500">天</span>
          {savingRetention ? (
            <RefreshCw className="size-4 animate-spin text-moon-350" />
          ) : (
            <span className="size-4" />
          )}
        </div>
        <p className="text-xs text-moon-450">
          设为 0 将停止自动清理，所有历史日志与通知会无限期保留。
        </p>
      </div>

      <div className="mt-4 flex-1 rounded-[1.1rem] border border-moon-200/45 bg-white/55 px-3.5 py-3">
        <div className="flex items-center gap-2">
          <span
            className={cn(
              "flex size-2 rounded-full",
              enabled
                ? "animate-pulse bg-status-green shadow-[0_0_0_3px_rgba(63,170,117,0.15)]"
                : "bg-moon-300",
            )}
          />
          <p className="text-sm font-medium text-moon-800">
            {enabled ? "自动清理运行中" : "自动清理已停用"}
          </p>
        </div>
        <p className="mt-1.5 text-xs leading-5 text-moon-500">
          {enabled
            ? `保留最近 ${retentionDays} 天数据，每次健康检查会删掉超出窗口的记录。`
            : "保留天数归零后不会再自动清理任何数据。"}
        </p>
        <div className="mt-3 space-y-1 text-xs text-moon-500">
          <p>
            上次执行：
            <span className="font-medium text-moon-700">
              {lastPruneAt ? relativeTime(lastPruneAt) : "尚未执行"}
            </span>
          </p>
          <p>
            上次清掉：
            <span className="font-medium text-moon-700">
              {lastDeleted > 0
                ? `${lastDeleted.toLocaleString()} 条`
                : "0 条"}
            </span>
            {lastDeleted > 0 ? (
              <span className="ml-1 text-moon-400">
                （日志 {lastDeletedLogs.toLocaleString()} · 通知{" "}
                {lastDeletedDeliveries.toLocaleString()} · 队列{" "}
                {lastDeletedOutbox.toLocaleString()}）
              </span>
            ) : null}
          </p>
        </div>
      </div>
    </div>
  );
}
