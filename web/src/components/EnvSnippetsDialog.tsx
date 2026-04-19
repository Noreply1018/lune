import { useEffect, useMemo, useRef, useState } from "react";
import { Check, Copy } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

const MODE_OPTIONS = [
  {
    key: "python",
    label: "Python SDK",
    title: "用于 Python 项目或脚本",
    description: "直接在 Python 里完成最小可用接入。",
  },
  {
    key: "shell",
    label: "Shell Env",
    title: "先在终端中配置环境变量",
    description: "供后续命令或工具复用。",
  },
  {
    key: "cursor",
    label: "Cursor",
    title: "用于 Cursor 的自定义接入",
    description: "把连接信息写进 Cursor 配置即可使用。",
  },
  {
    key: "curl",
    label: "curl Test",
    title: "直接测试接口是否可用",
    description: "先用一条请求确认连通性和响应。",
  },
] as const;

type ModeKey = (typeof MODE_OPTIONS)[number]["key"];

function getSnippets(baseUrl: string, token: string, model?: string) {
  const safeModel = model || "gpt-4o";

  return {
    python: `from openai import OpenAI

client = OpenAI(
    api_key="${token}",
    base_url="${baseUrl}",
)

resp = client.chat.completions.create(
    model="${safeModel}",
    messages=[{"role": "user", "content": "Hello"}],
)`,
    shell: `export OPENAI_BASE_URL="${baseUrl}"
export OPENAI_API_KEY="${token}"`,
    cursor: `{
  "openai.baseUrl": "${baseUrl}",
  "openai.apiKey": "${token}"
}`,
    curl: `curl ${baseUrl}/chat/completions \\
  -H "Authorization: Bearer ${token}" \\
  -H "Content-Type: application/json" \\
  -d '{"model":"${safeModel}","messages":[{"role":"user","content":"Hello"}]}'`,
  } satisfies Record<ModeKey, string>;
}

function InlineCopyAction({
  value,
  idleLabel = "复制",
  copiedLabel = "已复制",
  className,
}: {
  value: string;
  idleLabel?: string;
  copiedLabel?: string;
  className?: string;
}) {
  const [copied, setCopied] = useState(false);
  // Cancel the pending idle-revert on unmount — otherwise closing the dialog
  // mid-copy would trigger setState on an unmounted component.
  const revertTimerRef = useRef<number | null>(null);
  useEffect(() => {
    return () => {
      if (revertTimerRef.current) window.clearTimeout(revertTimerRef.current);
    };
  }, []);

  async function handleCopy() {
    await navigator.clipboard.writeText(value);
    setCopied(true);
    if (revertTimerRef.current) window.clearTimeout(revertTimerRef.current);
    revertTimerRef.current = window.setTimeout(() => setCopied(false), 1600);
  }

  return (
    <button
      type="button"
      onClick={handleCopy}
      className={cn(
        "inline-flex items-center gap-1.5 text-xs text-moon-400 transition-colors hover:text-moon-700",
        className,
      )}
    >
      {copied ? <Check className="size-3.5 text-status-green" /> : <Copy className="size-3.5" />}
      <span>{copied ? copiedLabel : idleLabel}</span>
    </button>
  );
}

export default function EnvSnippetsDialog({
  open,
  onOpenChange,
  title,
  baseUrl,
  token,
  model,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  baseUrl: string;
  token: string;
  model?: string;
}) {
  const [mode, setMode] = useState<ModeKey>("shell");
  const snippets = useMemo(() => getSnippets(baseUrl, token, model), [baseUrl, token, model]);

  useEffect(() => {
    if (open) {
      setMode("shell");
    }
  }, [open]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="max-w-[60rem] rounded-[1.9rem] border border-white/75 bg-[linear-gradient(180deg,rgba(251,250,247,0.96),rgba(246,244,240,0.97))] p-0 shadow-[0_36px_90px_-52px_rgba(33,40,63,0.34)] sm:max-w-[60rem]"
      >
        <DialogHeader className="border-b border-moon-200/55 px-7 py-6">
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>选择一种方式完成接入。</DialogDescription>
        </DialogHeader>

        <div className="space-y-6 px-7 py-7">
          <section className="space-y-3">
            <div className="space-y-3 border-b border-moon-200/50 pb-3">
              <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
                <div className="min-w-0">
                  <p className="text-[11px] uppercase tracking-[0.18em] text-moon-400">Base URL</p>
                  <p className="mt-1 break-all text-sm text-moon-700">{baseUrl}</p>
                </div>
                <InlineCopyAction value={baseUrl} className="shrink-0 sm:mt-0.5" />
              </div>
            </div>
            <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
              <div className="min-w-0">
                <p className="text-[11px] uppercase tracking-[0.18em] text-moon-400">API Key</p>
                <p className="mt-1 break-all text-sm text-moon-700">{token}</p>
              </div>
              <InlineCopyAction value={token} className="shrink-0 sm:mt-0.5" />
            </div>
          </section>

          <section className="space-y-3">
            <p className="text-[11px] uppercase tracking-[0.18em] text-moon-400">Mode</p>
            <div className="flex flex-wrap gap-2">
              {MODE_OPTIONS.map((option) => (
                <button
                  key={option.key}
                  type="button"
                  onClick={() => setMode(option.key)}
                  className={cn(
                    "rounded-full px-3 py-1.5 text-sm transition-colors",
                    mode === option.key
                      ? "bg-lunar-100/90 text-moon-800"
                      : "text-moon-500 hover:bg-white/70 hover:text-moon-700",
                  )}
                >
                  {option.label}
                </button>
              ))}
            </div>
          </section>

          <section className="space-y-3">
            <div className="flex items-start justify-between gap-3">
              <div>
                <p className="text-sm font-semibold text-moon-800">
                  {MODE_OPTIONS.find((option) => option.key === mode)?.title}
                </p>
                <p className="mt-1 text-sm text-moon-500">
                  {MODE_OPTIONS.find((option) => option.key === mode)?.description}
                </p>
              </div>
              <InlineCopyAction value={snippets[mode]} idleLabel="复制示例" />
            </div>

            <div className="overflow-hidden rounded-[1.45rem] border border-moon-200/60 bg-[linear-gradient(180deg,rgba(246,244,250,0.72),rgba(241,239,245,0.66))]">
              <pre className="overflow-x-auto px-4 py-4 text-[12px] leading-6 text-moon-700">
                <code>{snippets[mode]}</code>
              </pre>
            </div>
          </section>

          <div className="flex items-center justify-between gap-3 border-t border-moon-200/50 pt-4">
            <p className="text-sm text-moon-500">
              优先使用全局 Token；需要固定某个 Pool 时再切换。
            </p>
            <Button variant="ghost" size="sm" onClick={() => onOpenChange(false)} className="shrink-0">
              关闭
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
