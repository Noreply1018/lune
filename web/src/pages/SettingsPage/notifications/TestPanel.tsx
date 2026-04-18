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

// TestPanel renders an inline row meant to sit inside the channel-config card;
// it owns only the action button plus the latest result strip.
export default function TestPanel({
  loading,
  result,
  disabled,
  disabledReason,
  onRun,
}: TestPanelProps) {
  return (
    <div className="flex flex-wrap items-center gap-3 rounded-[0.9rem] border border-moon-200/45 bg-moon-50/40 px-3 py-2.5">
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
