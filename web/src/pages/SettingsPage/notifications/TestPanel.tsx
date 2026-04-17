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

export default function TestPanel({
  loading,
  result,
  disabled,
  disabledReason,
  onRun,
}: TestPanelProps) {
  return (
    <section className="space-y-3 rounded-[1.1rem] border border-amber-200/55 bg-[linear-gradient(180deg,rgba(255,251,235,0.88),rgba(255,247,237,0.72))] px-4 py-4">
      <div className="space-y-1">
        <p className="text-sm font-medium text-moon-800">测试发送</p>
        <p className="text-xs leading-5 text-moon-400">
          直接向配置的企微 webhook 真实发送一条「test」事件消息。结果会立即显示在按钮右侧。
        </p>
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <Button
          className="rounded-full bg-amber-600 text-white hover:bg-amber-700 focus-visible:ring-2 focus-visible:ring-amber-900/70 focus-visible:ring-offset-2 focus-visible:ring-offset-amber-50"
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
              result.ok
                ? "text-sm text-status-green"
                : "text-sm text-status-red"
            }
          >
            {result.ok ? "✓" : "✗"} {result.upstream_code || "unknown"} ·{" "}
            {(result.latency_ms / 1000).toFixed(1)}s
            {result.upstream_message ? ` · ${result.upstream_message}` : ""}
          </p>
        ) : null}
      </div>
    </section>
  );
}
