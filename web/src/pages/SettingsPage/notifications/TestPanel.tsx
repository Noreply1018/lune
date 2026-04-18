import { RefreshCw, Send } from "lucide-react";

import { Button } from "@/components/ui/button";

export type TestResult = {
  ok: boolean;
  latency_ms: number;
  upstream_code: string;
  upstream_message: string;
};

type TestPanelProps = {
  loading: boolean;
  result: TestResult | null;
  disabled: boolean;
  disabledReason?: string;
  onRun: () => void;
};

// TestPanel renders a self-contained card meant to sit in the top two-column
// strip next to the "启用通知" switch; it owns the action button plus the
// latest result line.
export default function TestPanel({
  loading,
  result,
  disabled,
  disabledReason,
  onRun,
}: TestPanelProps) {
  return (
    <div className="flex flex-wrap items-center gap-3 rounded-[1rem] border border-white/75 bg-white/75 px-4 py-3">
      <Button
        className="rounded-full bg-lunar-500 text-white hover:bg-lunar-600 focus-visible:ring-2 focus-visible:ring-lunar-400/60 focus-visible:ring-offset-2 focus-visible:ring-offset-white"
        onClick={onRun}
        disabled={disabled || loading}
        title={disabled ? disabledReason : undefined}
      >
        {loading ? (
          <RefreshCw className="size-4 animate-spin" />
        ) : (
          <Send className="size-4" />
        )}
        Send Test
      </Button>
      {disabled && disabledReason ? (
        <p className="text-xs text-moon-450">{disabledReason}</p>
      ) : null}
      {result ? (
        <p
          className={
            result.ok ? "text-sm text-status-green" : "text-sm text-status-red"
          }
        >
          {result.ok ? "✓" : "✗"} {result.upstream_code || "unknown"} ·{" "}
          {(result.latency_ms / 1000).toFixed(1)}s
          {result.upstream_message ? ` · ${result.upstream_message}` : ""}
        </p>
      ) : (
        <p className="text-xs text-moon-400">
          向当前 webhook 真实发送一条 test 事件；结果会显示在按钮右侧。
        </p>
      )}
    </div>
  );
}
