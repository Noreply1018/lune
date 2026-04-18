import type { KeyboardEvent } from "react";
import { Activity, RefreshCw } from "lucide-react";

import SectionHeading from "@/components/SectionHeading";
import { Input } from "@/components/ui/input";

type SystemSectionProps = {
  healthCheckInterval: number;
  saving: boolean;
  onChange: (value: number) => void;
  onCommit: () => void;
};

export default function SystemSection({
  healthCheckInterval,
  saving,
  onChange,
  onCommit,
}: SystemSectionProps) {
  function handleKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "Enter") {
      event.preventDefault();
      event.currentTarget.blur();
    }
  }

  return (
    <section className="surface-section px-5 py-5 sm:px-6">
      <SectionHeading
        title="System"
        description="后台守护任务的运行节奏。"
      />
      <div className="mt-5 rounded-[1.4rem] border border-moon-200/55 bg-[linear-gradient(180deg,rgba(255,255,255,0.94),rgba(244,241,250,0.78))] px-5 py-4 shadow-[0_24px_60px_-50px_rgba(74,68,108,0.32)]">
        <div className="flex flex-wrap items-center justify-between gap-4">
          <div className="flex items-start gap-3">
            <span className="mt-0.5 flex size-8 items-center justify-center rounded-full bg-lunar-100/70 text-lunar-600">
              <Activity className="size-4" />
            </span>
            <div className="space-y-1">
              <p className="text-sm font-semibold text-moon-800">
                Health Check Interval
              </p>
              <p className="text-xs leading-5 text-moon-500">
                健康检查跳动周期。越短越能快速发现账号故障，也越频繁触发自动清理与通知派发。
              </p>
              <p className="text-[11px] tracking-[0.14em] text-moon-350">
                修改立即生效
              </p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Input
              type="number"
              value={healthCheckInterval}
              min={1}
              className="h-10 w-24 text-right text-base font-medium tabular-nums"
              onChange={(event) => onChange(Number(event.target.value))}
              onBlur={onCommit}
              onKeyDown={handleKeyDown}
            />
            <span className="text-sm text-moon-500">秒</span>
            {saving ? (
              <RefreshCw className="size-4 animate-spin text-moon-350" />
            ) : (
              <span className="size-4" />
            )}
          </div>
        </div>
      </div>
    </section>
  );
}
