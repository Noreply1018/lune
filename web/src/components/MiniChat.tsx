import { useState } from "react";
import { Loader2, SendHorizonal } from "lucide-react";
import { toast } from "@/components/Feedback";
import { latency } from "@/lib/fmt";
import { Button } from "@/components/ui/button";

type ChatUsage = {
  prompt_tokens?: number;
  completion_tokens?: number;
  total_tokens?: number;
};

type ChatResponse = {
  choices?: Array<{
    message?: {
      content?: string;
    };
  }>;
  usage?: ChatUsage;
};

export default function MiniChat({
  accountId,
  model,
  globalToken,
  disabled,
}: {
  accountId: number;
  model?: string;
  globalToken: string;
  disabled?: boolean;
}) {
  const [message, setMessage] = useState("Hi, reply with one word.");
  const [reply, setReply] = useState("");
  const [usage, setUsage] = useState<ChatUsage | null>(null);
  const [durationMs, setDurationMs] = useState<number | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function runTest() {
    if (!model || !globalToken || loading || disabled) return;

    setLoading(true);
    setError(null);
    setReply("");
    setUsage(null);

    const started = performance.now();

    try {
      const res = await fetch("/v1/chat/completions", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${globalToken}`,
          "X-Lune-Account-Id": String(accountId),
        },
        body: JSON.stringify({
          model,
          messages: [{ role: "user", content: message }],
          stream: false,
        }),
      });

      const data = (await res.json().catch(() => null)) as ChatResponse | null;
      if (!res.ok) {
        throw new Error(
          (data as { error?: { message?: string } } | null)?.error?.message ??
            `请求失败 (${res.status})`,
        );
      }

      setReply(data?.choices?.[0]?.message?.content ?? "");
      setUsage(data?.usage ?? null);
      setDurationMs(performance.now() - started);
    } catch (err) {
      const message = err instanceof Error ? err.message : "测试失败";
      setError(message);
      toast(message, "error");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="surface-outline space-y-3 px-4 py-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <p className="text-xs uppercase tracking-[0.18em] text-moon-400">Mini Test</p>
          <p className="mt-1 text-sm text-moon-500">
            使用当前账号的首个模型发起一次直测。
          </p>
        </div>
        <Button onClick={runTest} disabled={loading || !model || !globalToken || disabled}>
          {loading ? <Loader2 className="size-4 animate-spin" /> : <SendHorizonal className="size-4" />}
          {loading ? "测试中" : "发送"}
        </Button>
      </div>
      <textarea
        value={message}
        onChange={(event) => setMessage(event.target.value)}
        className="min-h-24 w-full rounded-[1rem] border border-moon-200/60 bg-white/78 px-3 py-3 text-sm text-moon-700 outline-none ring-0 transition focus:border-lunar-300"
      />
      <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_12rem]">
        <div className="rounded-[1rem] bg-white/76 px-3 py-3 text-sm leading-7 text-moon-700">
          {error ? (
            <span className="text-status-red">{error}</span>
          ) : reply ? (
            reply
          ) : (
            <span className="text-moon-400">等待响应</span>
          )}
        </div>
        <div className="rounded-[1rem] bg-moon-100/55 px-3 py-3 text-xs leading-6 text-moon-500">
          <p>Model: {model ?? "--"}</p>
          <p>Latency: {durationMs != null ? latency(durationMs) : "--"}</p>
          <p>Total: {usage?.total_tokens ?? "--"} tokens</p>
        </div>
      </div>
    </div>
  );
}
