import { useEffect, useMemo, useRef, useState } from "react";
import { Check, Copy, Download } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

function poolSlug(poolLabel: string) {
  const parts: string[] = [];
  let pendingSeparator = false;

  for (const char of poolLabel.trim().normalize("NFKD")) {
    if (/[\u0300-\u036f]/.test(char)) continue;

    const codePoint = char.codePointAt(0);
    if (!codePoint) continue;

    if (/[a-z0-9]/i.test(char)) {
      if (pendingSeparator && parts.length > 0) parts.push("-");
      parts.push(char.toLowerCase());
      pendingSeparator = false;
      continue;
    }

    if (/[\p{Letter}\p{Number}]/u.test(char)) {
      if (pendingSeparator && parts.length > 0) parts.push("-");
      parts.push(`u${codePoint.toString(16)}`);
      pendingSeparator = true;
      continue;
    }

    pendingSeparator = parts.length > 0;
  }

  return parts.join("").replace(/-+/g, "-").replace(/^-+|-+$/g, "") || "pool";
}

function providerId(poolLabel: string) {
  return `lune-${poolSlug(poolLabel)}`;
}

function envKey(poolLabel: string) {
  const suffix = poolSlug(poolLabel).replace(/-/g, "_").toUpperCase();
  return `LUNE_POOL_${suffix}_API_KEY`;
}

function escapeTomlString(value: string) {
  return value.replace(/\\/g, "\\\\").replace(/"/g, '\\"');
}

function shellQuote(value: string) {
  return `'${value.replace(/'/g, `'\\''`)}'`;
}

function buildCodexConfig({
  poolLabel,
  baseUrl,
  model,
}: {
  poolLabel: string;
  baseUrl: string;
  model: string;
}) {
  const id = providerId(poolLabel);
  return `model = "${escapeTomlString(model)}"
model_provider = "${id}"
profile = "full"

model_reasoning_effort = "medium"
model_reasoning_summary = "concise"
model_verbosity = "low"

approval_policy = "never"
web_search = "live"
personality = "pragmatic"
disable_response_storage = true
suppress_unstable_features_warning = true

project_root_markers = [".git"]

[history]
persistence = "save-all"

[tui]
notifications = true
notification_method = "auto"
show_tooltips = false

[shell_environment_policy]
inherit = "all"
ignore_default_excludes = false

[sandbox_workspace_write]
network_access = true

[tools]
view_image = true
web_search = true

[profiles.cli-auto]
approval_policy = "never"
sandbox_mode = "workspace-write"

[profiles.review]
approval_policy = "on-request"
sandbox_mode = "workspace-write"

[profiles.full]
approval_policy = "never"
sandbox_mode = "danger-full-access"

[model_providers.${id}]
name = "Lune / ${escapeTomlString(poolLabel)}"
base_url = "${escapeTomlString(baseUrl)}"
env_key = "${envKey(poolLabel)}"
wire_api = "responses"
request_max_retries = 4
stream_max_retries = 10
stream_idle_timeout_ms = 300000
`;
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

function downloadConfig(content: string) {
  const blob = new Blob([content], { type: "text/toml;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = "config.toml";
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

export default function CodexSetupDialog({
  open,
  onOpenChange,
  poolLabel,
  baseUrl,
  token,
  model,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  poolLabel: string;
  baseUrl: string;
  token: string;
  model: string;
}) {
  const key = useMemo(() => envKey(poolLabel), [poolLabel]);
  const envLine = useMemo(
    () => `export ${key}=${shellQuote(token)}`,
    [key, token],
  );
  const bashrcLine = useMemo(
    () =>
      `{ grep -v '^export ${key}=' ~/.bashrc 2>/dev/null; printf '\\nexport ${key}=%s\\n' ${shellQuote(token)}; } > ~/.bashrc.lune.tmp && mv ~/.bashrc.lune.tmp ~/.bashrc`,
    [key, token],
  );
  const config = useMemo(
    () => buildCodexConfig({ poolLabel, baseUrl, model }),
    [baseUrl, model, poolLabel],
  );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[min(44rem,calc(100vh-2rem))] max-w-[min(52rem,calc(100vw-1.5rem))] flex-col overflow-hidden rounded-[1.35rem] border border-white/75 bg-[linear-gradient(180deg,rgba(251,250,247,0.96),rgba(246,244,240,0.97))] p-0 shadow-[0_36px_90px_-52px_rgba(33,40,63,0.34)] sm:max-w-[min(52rem,calc(100vw-2.5rem))]">
        <DialogHeader className="shrink-0 border-b border-moon-200/55 px-5 py-4 sm:px-6">
          <DialogTitle>{poolLabel} · Codex CLI</DialogTitle>
          <DialogDescription>
            使用当前 Pool 的访问凭证配置 Codex CLI；已有配置请合并到 ~/.codex/config.toml。
          </DialogDescription>
        </DialogHeader>

        <div className="min-h-0 flex-1 space-y-4 overflow-y-auto px-5 py-5 sm:px-6">
          <section className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_13rem]">
            <div className="space-y-2.5 rounded-[1rem] border border-moon-200/55 bg-white/60 px-3.5 py-3.5">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <p className="text-[11px] uppercase tracking-[0.18em] text-moon-400">
                    1. Environment
                  </p>
                  <p className="mt-1 text-sm text-moon-600">
                    在当前 shell 中导出 Pool token，或写入 ~/.bashrc 长期生效。
                  </p>
                </div>
                <InlineCopyAction value={envLine} />
              </div>
              <pre className="overflow-x-auto rounded-[0.85rem] bg-moon-100/70 px-3 py-2.5 text-[12px] leading-5 text-moon-700">
                <code>{envLine}</code>
              </pre>
              <div className="flex items-start justify-between gap-3 pt-1">
                <p className="text-xs leading-5 text-moon-400">
                  写入 ~/.bashrc 时会先移除同名旧 export，再追加当前 token。
                </p>
                <InlineCopyAction
                  value={bashrcLine}
                  idleLabel="复制 bashrc 命令"
                  className="shrink-0"
                />
              </div>
              <pre className="overflow-x-auto rounded-[0.85rem] bg-moon-100/70 px-3 py-2.5 text-[12px] leading-5 text-moon-700">
                <code>{bashrcLine}</code>
              </pre>
            </div>

            <div className="space-y-2 rounded-[1rem] border border-moon-200/55 bg-white/54 px-3.5 py-3.5">
              <p className="text-[11px] uppercase tracking-[0.18em] text-moon-400">默认模型</p>
              <p className="break-all font-mono text-[13px] leading-5 text-moon-700">{model}</p>
              <p className="text-xs leading-5 text-moon-400">
                Codex CLI 配置默认写入 gpt-5.5，可在 config.toml 中手动替换。
              </p>
            </div>
          </section>

          <section className="space-y-3 rounded-[1rem] border border-moon-200/55 bg-white/60 px-3.5 py-3.5">
            <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
              <div>
                <p className="text-[11px] uppercase tracking-[0.18em] text-moon-400">
                  2. ~/.codex/config.toml
                </p>
                <p className="mt-1 text-sm text-moon-600">
                  将下面片段写入或合并到 Codex CLI 配置文件。
                </p>
              </div>
              <div className="flex shrink-0 flex-wrap items-center gap-2">
                <InlineCopyAction value={config} idleLabel="复制配置" />
                <Button
                  variant="outline"
                  size="sm"
                  className="rounded-full"
                  onClick={() => downloadConfig(config)}
                >
                  <Download className="size-3.5" />
                  下载 config.toml
                </Button>
              </div>
            </div>
            <pre className="max-h-[min(18rem,38vh)] overflow-auto rounded-[0.85rem] bg-[linear-gradient(180deg,rgba(246,244,250,0.78),rgba(241,239,245,0.72))] px-3 py-3 text-[12px] leading-5 text-moon-700">
              <code>{config}</code>
            </pre>
          </section>

          <section>
            <div className="rounded-[1rem] border border-moon-200/50 bg-white/46 px-3.5 py-3">
              <p className="text-sm font-medium text-moon-800">3. Start Codex CLI</p>
              <p className="mt-1 text-sm leading-6 text-moon-500">
                配置合并完成后，重新打开终端或确认环境变量已生效，然后启动 Codex CLI。
              </p>
            </div>
          </section>
        </div>
      </DialogContent>
    </Dialog>
  );
}
