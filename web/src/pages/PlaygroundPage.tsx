import { useEffect, useRef, useState, type KeyboardEvent } from "react";
import CopyButton from "@/components/CopyButton";
import PageHeader from "@/components/PageHeader";
import StatusBadge from "@/components/StatusBadge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Loader2, Send, Trash2 } from "lucide-react";

interface Message {
  role: "user" | "assistant";
  content: string;
}

interface Metrics {
  ttfb_ms: number;
  total_ms: number;
  input_tokens: number | null;
  output_tokens: number | null;
}

export default function PlaygroundPage() {
  const [models, setModels] = useState<string[]>([]);
  const [model, setModel] = useState("");
  const [token, setToken] = useState("");
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState("");
  const [streaming, setStreaming] = useState(false);
  const [metrics, setMetrics] = useState<Metrics | null>(null);
  const chatRef = useRef<HTMLDivElement>(null);
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    fetch("/v1/models")
      .then((r) => r.json())
      .then((body) => {
        const ids: string[] = (body.data ?? []).map((m: { id: string }) => m.id);
        ids.sort();
        setModels(ids);
        if (ids.length > 0 && !model) setModel(ids[0]);
      })
      .catch(() => {});
  }, [model]);

  useEffect(() => {
    if (chatRef.current) {
      chatRef.current.scrollTop = chatRef.current.scrollHeight;
    }
  }, [messages]);

  async function handleSend() {
    const text = input.trim();
    if (!text || !model || !token || streaming) return;

    const userMsg: Message = { role: "user", content: text };
    const history = [...messages, userMsg];
    setMessages([...history, { role: "assistant", content: "" }]);
    setInput("");
    setStreaming(true);
    setMetrics(null);

    const controller = new AbortController();
    abortRef.current = controller;
    const start = performance.now();
    let ttfb = 0;
    let assistantContent = "";
    let usage: { prompt_tokens?: number; completion_tokens?: number } | null = null;

    try {
      const resp = await fetch("/v1/chat/completions", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({
          model,
          messages: history.map((m) => ({ role: m.role, content: m.content })),
          stream: true,
          stream_options: { include_usage: true },
        }),
        signal: controller.signal,
      });

      if (!resp.ok) {
        const err = await resp.text();
        let errorMsg: string;
        try {
          const parsed = JSON.parse(err);
          errorMsg = parsed.error?.message || parsed.message || `HTTP ${resp.status}`;
        } catch {
          errorMsg = `HTTP ${resp.status}: ${err.slice(0, 200)}`;
        }
        setMessages([...history, { role: "assistant", content: `Error: ${errorMsg}` }]);
        setStreaming(false);
        return;
      }

      const reader = resp.body?.getReader();
      if (!reader) {
        setStreaming(false);
        return;
      }

      const decoder = new TextDecoder();
      let buffer = "";

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split("\n");
        buffer = lines.pop() ?? "";

        for (const line of lines) {
          if (!line.startsWith("data: ")) continue;
          const data = line.slice(6).trim();
          if (data === "[DONE]") continue;

          try {
            const chunk = JSON.parse(data);
            if (chunk.usage) usage = chunk.usage;
            const delta = chunk.choices?.[0]?.delta?.content;
            if (delta) {
              if (!ttfb) ttfb = performance.now() - start;
              assistantContent += delta;
              setMessages([...history, { role: "assistant", content: assistantContent }]);
            }
          } catch {
            // Ignore malformed chunks.
          }
        }
      }

      const totalMs = performance.now() - start;
      setMetrics({
        ttfb_ms: Math.round(ttfb),
        total_ms: Math.round(totalMs),
        input_tokens: usage?.prompt_tokens ?? null,
        output_tokens: usage?.completion_tokens ?? null,
      });
    } catch (err) {
      if ((err as Error).name !== "AbortError") {
        setMessages([
          ...history,
          { role: "assistant", content: `Error: ${(err as Error).message}` },
        ]);
      }
    } finally {
      setStreaming(false);
      abortRef.current = null;
    }
  }

  function handleClear() {
    if (abortRef.current) abortRef.current.abort();
    setMessages([]);
    setMetrics(null);
  }

  function handleKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  }

  return (
    <div className="space-y-10">
      <PageHeader
        eyebrow="Playground / Workspace"
        title="调试台"
        description="直接发起真实请求，观察响应、耗时与 Token。"
        meta={
          <>
            <span>模型 {model || "未选择"}</span>
            <span>消息 {messages.length}</span>
            <span>{streaming ? "会话进行中" : "空闲"}</span>
          </>
        }
      />

      <section className="grid gap-6 xl:grid-cols-[minmax(0,1.45fr)_minmax(300px,0.55fr)]">
        <div className="surface-section px-5 py-5 sm:px-6 sm:py-6">
          <div className="flex flex-wrap items-end justify-between gap-4 border-b border-moon-200/60 pb-4">
            <div>
              <p className="eyebrow-label">会话配置</p>
              <h2 className="mt-1 text-[1.1rem] font-semibold tracking-[-0.03em] text-moon-800">
                设置模型与访问令牌
              </h2>
            </div>
            <Button
              variant="outline"
              size="sm"
              onClick={handleClear}
              disabled={messages.length === 0 && !streaming}
            >
              <Trash2 className="size-4" />
              清空会话
            </Button>
          </div>

          <div className="mt-5 grid gap-4 lg:grid-cols-[minmax(220px,0.8fr)_minmax(280px,1.2fr)]">
            <div className="space-y-2">
              <Label className="text-sm font-medium text-moon-600">模型</Label>
              <Select value={model} onValueChange={(v) => v && setModel(v)}>
                <SelectTrigger className="h-11 rounded-xl border-white/75 bg-white/84">
                  <SelectValue placeholder="选择模型" />
                </SelectTrigger>
                <SelectContent>
                  {models.map((m) => (
                    <SelectItem key={m} value={m}>
                      {m}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <Label className="text-sm font-medium text-moon-600">Access Token</Label>
              <Input
                type="password"
                value={token}
                onChange={(e) => setToken(e.target.value)}
                placeholder="sk-lune-..."
                className="h-11 rounded-xl border-white/75 bg-white/84"
              />
            </div>
          </div>

          <div className="mt-5 grid gap-3 md:grid-cols-3">
            <div className="rounded-[1.15rem] border border-white/72 bg-white/70 px-4 py-4">
              <p className="kicker">状态</p>
              <div className="mt-3">
                <StatusBadge
                  status={streaming ? "degraded" : "healthy"}
                  label={streaming ? "响应中" : "待命"}
                />
              </div>
            </div>
            <div className="rounded-[1.15rem] border border-white/72 bg-white/70 px-4 py-4">
              <p className="kicker">消息数</p>
              <p className="mt-3 text-[1.45rem] font-semibold tracking-[-0.04em] text-moon-800">
                {messages.length}
              </p>
            </div>
            <div className="rounded-[1.15rem] border border-white/72 bg-white/70 px-4 py-4">
              <p className="kicker">选择模型</p>
              <p className="mt-3 truncate text-sm font-medium text-moon-700">
                {model || "未选择"}
              </p>
            </div>
          </div>
        </div>

        <aside className="surface-card px-5 py-5">
          <div className="border-b border-moon-200/60 pb-4">
            <p className="eyebrow-label">会话指标</p>
            <p className="mt-1 text-sm text-moon-500">
              每次请求完成后更新。用于判断网关真实响应表现。
            </p>
          </div>

          <div className="space-y-4 pt-4">
            <div className="flex items-center justify-between gap-4">
              <span className="text-sm text-moon-500">状态</span>
              <span className="font-medium text-moon-700">{streaming ? "Streaming" : "Idle"}</span>
            </div>
            <div className="flex items-center justify-between gap-4">
              <span className="text-sm text-moon-500">TTFB</span>
              <span className="font-semibold text-moon-800">
                {metrics ? `${metrics.ttfb_ms}ms` : "-"}
              </span>
            </div>
            <div className="flex items-center justify-between gap-4">
              <span className="text-sm text-moon-500">总耗时</span>
              <span className="font-semibold text-moon-800">
                {metrics
                  ? metrics.total_ms < 1000
                    ? `${metrics.total_ms}ms`
                    : `${(metrics.total_ms / 1000).toFixed(1)}s`
                  : "-"}
              </span>
            </div>
            <div className="flex items-center justify-between gap-4">
              <span className="text-sm text-moon-500">输入 / 输出</span>
              <span className="font-medium text-moon-700">
                {metrics ? `${metrics.input_tokens ?? 0} / ${metrics.output_tokens ?? 0}` : "-"}
              </span>
            </div>
          </div>
        </aside>
      </section>

      <section className="surface-section overflow-hidden">
        <div className="grid min-h-[34rem] xl:grid-cols-[minmax(0,1fr)_320px]">
          <div className="flex min-h-[34rem] flex-col">
            <div className="border-b border-moon-200/60 px-5 py-4 sm:px-6">
              <p className="eyebrow-label">消息流</p>
              <p className="mt-1 text-sm text-moon-500">
                用户输入与模型输出按真实顺序显示。
              </p>
            </div>

            <div ref={chatRef} className="flex-1 space-y-4 overflow-y-auto px-5 py-5 sm:px-6">
              {messages.length === 0 && (
                <div className="panel-muted flex h-full min-h-[20rem] flex-col items-center justify-center rounded-[1.5rem] border border-dashed border-moon-200/80 text-center">
                  <p className="text-sm font-medium text-moon-600">发送第一条消息开始测试</p>
                  <p className="mt-1 text-sm text-moon-400">这里会显示完整对话流与响应内容。</p>
                </div>
              )}

              {messages.map((msg, index) => (
                <div
                  key={index}
                  className={`flex ${msg.role === "user" ? "justify-end" : "justify-start"}`}
                >
                  <div
                    className={`group relative max-w-[85%] rounded-[1.35rem] px-4 py-3 text-sm leading-7 ${
                      msg.role === "user"
                        ? "bg-[linear-gradient(180deg,rgba(134,125,193,0.16),rgba(134,125,193,0.07))] text-moon-800"
                        : "bg-[rgba(243,241,236,0.88)] text-moon-700"
                    }`}
                  >
                    <div className="mb-1 text-[11px] tracking-[0.16em] text-moon-400">
                      {msg.role === "user" ? "你" : "模型"}
                    </div>
                    <div className="whitespace-pre-wrap">
                      {msg.content || (streaming && index === messages.length - 1 ? "..." : "")}
                    </div>
                    {msg.content && (
                      <div className="absolute -top-2 right-1 hidden group-hover:block">
                        <CopyButton value={msg.content} />
                      </div>
                    )}
                  </div>
                </div>
              ))}
            </div>

            <div className="border-t border-moon-200/60 px-5 py-4 sm:px-6">
              <div className="flex gap-2">
                <textarea
                  className="min-h-[92px] flex-1 resize-none rounded-[1.15rem] border border-white/75 bg-white/86 px-4 py-3 text-sm leading-6 placeholder:text-moon-400 focus:outline-none focus:ring-2 focus:ring-lunar-300"
                  value={input}
                  onChange={(e) => setInput(e.target.value)}
                  onKeyDown={handleKeyDown}
                  placeholder="输入消息。Enter 发送，Shift + Enter 换行。"
                  disabled={streaming}
                />
                <Button
                  onClick={handleSend}
                  disabled={streaming || !token || !model || !input.trim()}
                  className="self-end rounded-[1rem]"
                >
                  {streaming ? (
                    <Loader2 className="size-4 animate-spin" />
                  ) : (
                    <Send className="size-4" />
                  )}
                </Button>
              </div>
            </div>
          </div>

          <aside className="border-t border-moon-200/60 bg-[rgba(244,245,248,0.72)] px-5 py-5 xl:border-l xl:border-t-0">
            <p className="eyebrow-label">工作侧栏</p>
            <div className="mt-4 space-y-4">
              <div className="rounded-[1.1rem] border border-white/70 bg-white/72 px-4 py-4">
                <p className="kicker">当前上下文</p>
                <div className="mt-3 space-y-3 text-sm text-moon-500">
                  <div className="flex items-center justify-between gap-4">
                    <span>模型</span>
                    <span className="font-medium text-moon-700">{model || "未选择"}</span>
                  </div>
                  <div className="flex items-center justify-between gap-4">
                    <span>Token</span>
                    <span className="font-medium text-moon-700">
                      {token ? "已填入" : "未填入"}
                    </span>
                  </div>
                  <div className="flex items-center justify-between gap-4">
                    <span>消息</span>
                    <span className="font-medium text-moon-700">{messages.length}</span>
                  </div>
                </div>
              </div>

              <div className="rounded-[1.1rem] border border-white/70 bg-white/72 px-4 py-4">
                <p className="kicker">使用建议</p>
                <ul className="mt-3 space-y-2 text-sm leading-6 text-moon-500">
                  <li>先确认模型别名是否按预期命中。</li>
                  <li>观察首字节时间是否明显异常。</li>
                  <li>输出完成后再比对 Token 与日志页。</li>
                </ul>
              </div>
            </div>
          </aside>
        </div>
      </section>
    </div>
  );
}
