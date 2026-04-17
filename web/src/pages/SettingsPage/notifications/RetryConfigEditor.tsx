import { RefreshCw } from "lucide-react";

import { Input } from "@/components/ui/input";

export default function RetryConfigEditor({
  maxAttempts,
  scheduleInput,
  saving,
  onMaxAttemptsChange,
  onScheduleInputChange,
  onCommit,
}: {
  maxAttempts: number;
  scheduleInput: string;
  saving?: boolean;
  onMaxAttemptsChange: (value: number) => void;
  onScheduleInputChange: (value: string) => void;
  onCommit: (nextAttempts: number, nextSchedule: string) => void;
}) {
  return (
    <section className="space-y-3">
      <div className="space-y-1">
        <p className="text-sm font-medium text-moon-800">重试策略</p>
        <p className="text-xs leading-5 text-moon-400">
          调整最大重试次数和每一轮的间隔秒数，长度必须覆盖最大重试次数。
        </p>
      </div>
      <div className="grid gap-3 sm:grid-cols-[9rem_minmax(0,1fr)]">
        <label className="space-y-2">
          <span className="text-xs font-medium tracking-[0.16em] text-moon-350">
            MAX ATTEMPTS
          </span>
          <Input
            type="number"
            min={1}
            value={maxAttempts}
            onChange={(event) => onMaxAttemptsChange(Number(event.target.value))}
            onBlur={(event) =>
              onCommit(Number(event.currentTarget.value), scheduleInput)
            }
          />
        </label>
        <label className="space-y-2">
          <span className="text-xs font-medium tracking-[0.16em] text-moon-350">
            INTERVALS (SECONDS)
          </span>
          <Input
            value={scheduleInput}
            onChange={(event) => onScheduleInputChange(event.target.value)}
            onBlur={(event) => onCommit(maxAttempts, event.currentTarget.value)}
            placeholder="30, 120, 600, 1800, 7200"
          />
        </label>
      </div>
      <div className="flex items-center gap-2 text-xs text-moon-400">
        {saving ? <RefreshCw className="size-3.5 animate-spin" /> : null}
        <span>示例：`30, 120, 600` 代表第 1/2/3 轮间隔。</span>
      </div>
    </section>
  );
}
