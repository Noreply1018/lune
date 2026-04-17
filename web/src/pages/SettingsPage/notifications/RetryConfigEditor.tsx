import { useEffect, useMemo, useState } from "react";
import { RefreshCw } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

const RETRY_PRESETS = [
  { label: "Fast", schedule: [30, 120, 600] },
  { label: "Balanced", schedule: [30, 120, 600, 1800, 7200] },
  { label: "Escalating", schedule: [60, 300, 1800, 7200, 21600] },
];

export default function RetryConfigEditor({
  maxAttempts,
  scheduleInput,
  saving,
  onCommit,
}: {
  maxAttempts: number;
  scheduleInput: string;
  saving?: boolean;
  onCommit: (nextAttempts: number, nextSchedule: string) => void;
}) {
  const [attemptDraft, setAttemptDraft] = useState(String(maxAttempts));
  const [scheduleDraft, setScheduleDraft] = useState(scheduleInput);
  const [touched, setTouched] = useState(false);

  useEffect(() => {
    if (!touched) {
      setAttemptDraft(String(maxAttempts));
      setScheduleDraft(scheduleInput);
    }
  }, [maxAttempts, scheduleInput, touched]);

  const currentPreset = useMemo(
    () =>
      RETRY_PRESETS.find((preset) => preset.schedule.join(", ") === scheduleDraft) ??
      null,
    [scheduleDraft],
  );

  function commitIfNeeded() {
    const nextAttempts = Number(attemptDraft);
    if (
      !touched ||
      (!Number.isFinite(nextAttempts) && scheduleDraft === scheduleInput) ||
      (Number(nextAttempts) === maxAttempts && scheduleDraft === scheduleInput)
    ) {
      setTouched(false);
      return;
    }
    onCommit(Number.isFinite(nextAttempts) ? nextAttempts : maxAttempts, scheduleDraft);
    setTouched(false);
  }

  return (
    <section
      className="space-y-3"
      onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget as Node | null)) {
          commitIfNeeded();
        }
      }}
    >
      <div className="space-y-1">
        <p className="text-sm font-medium text-moon-800">重试策略</p>
        <p className="text-xs leading-5 text-moon-400">
          以整体 schedule 为主编辑，再决定最多执行前几轮。离开本区域时只提交一次。
        </p>
      </div>
      <div className="flex flex-wrap gap-2">
        {RETRY_PRESETS.map((preset) => (
          <Button
            key={preset.label}
            type="button"
            variant="outline"
            className="rounded-full"
            data-active={currentPreset?.label === preset.label}
            onClick={() => {
              setScheduleDraft(preset.schedule.join(", "));
              setAttemptDraft(String(preset.schedule.length));
              setTouched(true);
            }}
          >
            {preset.label}
          </Button>
        ))}
      </div>
      <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_10rem]">
        <label className="space-y-2">
          <span className="text-xs font-medium tracking-[0.16em] text-moon-350">
            RETRY SCHEDULE (SECONDS)
          </span>
          <Input
            value={scheduleDraft}
            onChange={(event) => {
              setScheduleDraft(event.target.value);
              setTouched(true);
            }}
            placeholder="30, 120, 600, 1800, 7200"
          />
        </label>
        <label className="space-y-2">
          <span className="text-xs font-medium tracking-[0.16em] text-moon-350">
            MAX ATTEMPTS
          </span>
          <Input
            type="number"
            min={1}
            value={attemptDraft}
            onChange={(event) => {
              setAttemptDraft(event.target.value);
              setTouched(true);
            }}
          />
        </label>
      </div>
      <div className="flex items-center gap-2 text-xs text-moon-400">
        <div className="flex items-center gap-2">
          {saving ? <RefreshCw className="size-3.5 animate-spin" /> : null}
          <span>示例：`1m, 5m, 30m, 2h, 6h` 对应 `60, 300, 1800, 7200, 21600`。</span>
        </div>
      </div>
    </section>
  );
}
