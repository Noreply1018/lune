import { RefreshCw, Send } from "lucide-react";

import { Button } from "@/components/ui/button";

export type TestResult = {
  ok: boolean;
  latency_ms: number;
  upstream_code: string;
  upstream_message: string;
};

export default function TestPanel({
  loading,
  result,
  onRun,
}: {
  loading: boolean;
  result: TestResult | null;
  onRun: () => void;
}) {
  return (
    <section className="space-y-3 rounded-[1.2rem] border border-amber-200/55 bg-[linear-gradient(180deg,rgba(255,251,235,0.88),rgba(255,247,237,0.72))] px-4 py-4">
      <div className="space-y-1">
        <p className="text-sm font-medium text-moon-800">测试发送</p>
        <p className="text-xs leading-5 text-moon-400">
          会真实发送一条测试消息到当前 channel。按钮样式刻意更重，避免和无副作用的预览混淆。
        </p>
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <Button
          className="rounded-full bg-amber-600 text-white hover:bg-amber-700 focus-visible:ring-2 focus-visible:ring-amber-900/70 focus-visible:ring-offset-2 focus-visible:ring-offset-amber-50"
          onClick={onRun}
          disabled={loading}
        >
          {loading ? (
            <RefreshCw className="size-4 animate-spin" />
          ) : (
            <Send className="size-4" />
          )}
          Send Test
        </Button>
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
