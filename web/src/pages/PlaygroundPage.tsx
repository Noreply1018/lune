import { useEffect, useRef, useState } from "react";
import PageHeader from "@/components/PageHeader";
import CopyButton from "@/components/CopyButton";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Send, Loader2, Trash2 } from "lucide-react";

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
  }, []);

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
            if (chunk.usage) {
              usage = chunk.usage;
            }
            const delta = chunk.choices?.[0]?.delta?.content;
            if (delta) {
              if (!ttfb) ttfb = performance.now() - start;
              assistantContent += delta;
              setMessages([...history, { role: "assistant", content: assistantContent }]);
            }
          } catch {
            // skip malformed chunks
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

  function handleKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        eyebrow="Lune 控制台"
        title="调试台"
        description="通过交互式对话直接测试网关的端到端响应。"
      />

      {/* Controls */}
      <section className="rounded-[1.6rem] border border-moon-200/70 bg-[linear-gradient(135deg,rgba(255,255,255,0.95),rgba(249,249,252,0.9))] p-4 sm:p-5">
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-[1fr_1fr_auto]">
          <div className="space-y-1.5">
            <Label className="text-xs text-moon-500">模型</Label>
            <Select value={model} onValueChange={(v) => v && setModel(v)}>
              <SelectTrigger>
                <SelectValue placeholder="选择模型..." />
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

          <div className="space-y-1.5">
            <Label className="text-xs text-moon-500">访问令牌</Label>
            <Input
              type="password"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              placeholder="sk-lune-..."
            />
          </div>

          <div className="flex items-end">
            <Button
              variant="outline"
              size="sm"
              onClick={handleClear}
              disabled={messages.length === 0 && !streaming}
            >
              <Trash2 className="size-4" />
              清空
            </Button>
          </div>
        </div>

        {metrics && (
          <div className="mt-3 flex flex-wrap gap-4 border-t border-moon-200/50 pt-3 text-xs text-moon-500">
            <span>
              TTFB <strong className="text-moon-700">{metrics.ttfb_ms}ms</strong>
            </span>
            <span>
              Total{" "}
              <strong className="text-moon-700">
                {metrics.total_ms < 1000
                  ? `${metrics.total_ms}ms`
                  : `${(metrics.total_ms / 1000).toFixed(1)}s`}
              </strong>
            </span>
            {metrics.input_tokens != null && (
              <span>
                In <strong className="text-moon-700">{metrics.input_tokens}</strong>
              </span>
            )}
            {metrics.output_tokens != null && (
              <span>
                Out <strong className="text-moon-700">{metrics.output_tokens}</strong>
              </span>
            )}
          </div>
        )}
      </section>

      {/* Chat */}
      <section className="flex min-h-[420px] flex-col overflow-hidden rounded-[1.6rem] border border-moon-200/70 bg-white/85">
        <div
          ref={chatRef}
          className="flex-1 space-y-4 overflow-y-auto p-5"
        >
          {messages.length === 0 && (
            <div className="flex h-full items-center justify-center text-sm text-moon-400">
              发送一条消息开始测试
            </div>
          )}
          {messages.map((msg, i) => (
            <div
              key={i}
              className={`flex ${msg.role === "user" ? "justify-end" : "justify-start"}`}
            >
              <div
                className={`group relative max-w-[80%] rounded-2xl px-4 py-3 text-sm leading-relaxed ${
                  msg.role === "user"
                    ? "bg-lunar-100/60 text-moon-800"
                    : "bg-moon-100/60 text-moon-700"
                }`}
              >
                <div className="whitespace-pre-wrap">{msg.content || (streaming && i === messages.length - 1 ? "..." : "")}</div>
                {msg.content && (
                  <div className="absolute -top-2 right-1 hidden group-hover:block">
                    <CopyButton value={msg.content} />
                  </div>
                )}
              </div>
            </div>
          ))}
        </div>

        <div className="border-t border-moon-200/60 p-4">
          <div className="flex gap-2">
            <textarea
              className="flex-1 resize-none rounded-xl border border-moon-200 bg-white px-4 py-2.5 text-sm placeholder:text-moon-400 focus:outline-none focus:ring-2 focus:ring-lunar-300"
              rows={1}
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="输入消息...（Enter 发送，Shift+Enter 换行）"
              disabled={streaming}
            />
            <Button
              onClick={handleSend}
              disabled={streaming || !token || !model || !input.trim()}
              className="self-end"
            >
              {streaming ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Send className="size-4" />
              )}
            </Button>
          </div>
        </div>
      </section>
    </div>
  );
}
