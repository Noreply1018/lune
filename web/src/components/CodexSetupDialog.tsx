import { useEffect, useMemo, useRef, useState } from "react";
import { Check, Copy, Download, FileText } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

function providerId(poolId: number) {
  return `lune-pool-${poolId}`;
}

function envKey(poolId: number) {
  return `LUNE_POOL_${poolId}_API_KEY`;
}

function escapeTomlString(value: string) {
  return value.replace(/\\/g, "\\\\").replace(/"/g, '\\"');
}

function buildCodexConfig({
  poolId,
  poolLabel,
  baseUrl,
  model,
}: {
  poolId: number;
  poolLabel: string;
  baseUrl: string;
  model: string;
}) {
  const id = providerId(poolId);
  return `model = "${escapeTomlString(model)}"
model_provider = "${id}"

[model_providers.${id}]
name = "Lune / ${escapeTomlString(poolLabel)}"
base_url = "${escapeTomlString(baseUrl)}"
env_key = "${envKey(poolId)}"
wire_api = "responses"
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
  poolId,
  poolLabel,
  baseUrl,
  token,
  model,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  poolId: number;
  poolLabel: string;
  baseUrl: string;
  token: string;
  model: string;
}) {
  const envLine = useMemo(
    () => `export ${envKey(poolId)}="${token}"`,
    [poolId, token],
  );
  const config = useMemo(
    () => buildCodexConfig({ poolId, poolLabel, baseUrl, model }),
    [baseUrl, model, poolId, poolLabel],
  );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-[58rem] rounded-[1.9rem] border border-white/75 bg-[linear-gradient(180deg,rgba(251,250,247,0.96),rgba(246,244,240,0.97))] p-0 shadow-[0_36px_90px_-52px_rgba(33,40,63,0.34)] sm:max-w-[58rem]">
        <DialogHeader className="border-b border-moon-200/55 px-7 py-6">
          <DialogTitle>{poolLabel} · Codex CLI Setup</DialogTitle>
          <DialogDescription>
            使用当前 Pool 的访问凭证配置 Codex CLI；已有配置请合并到 ~/.codex/config.toml。
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-6 px-7 py-7">
          <section className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_16rem]">
            <div className="space-y-3 rounded-[1.25rem] border border-moon-200/55 bg-white/60 px-4 py-4">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <p className="text-[11px] uppercase tracking-[0.18em] text-moon-400">
                    1. Environment
                  </p>
                  <p className="mt-1 text-sm text-moon-600">
                    在运行 Codex CLI 的 shell 中导出 Pool token。
                  </p>
                </div>
                <InlineCopyAction value={envLine} />
              </div>
              <pre className="overflow-x-auto rounded-[1rem] bg-moon-100/70 px-3 py-3 text-[12px] leading-6 text-moon-700">
                <code>{envLine}</code>
              </pre>
            </div>

            <div className="space-y-2 rounded-[1.25rem] border border-moon-200/55 bg-white/54 px-4 py-4">
              <p className="text-[11px] uppercase tracking-[0.18em] text-moon-400">默认模型</p>
              <p className="break-all font-mono text-[13px] leading-5 text-moon-700">{model}</p>
              <p className="text-xs leading-5 text-moon-400">
                默认选择当前 Pool 发现到的最新模型，可在 config.toml 中手动替换。
              </p>
            </div>
          </section>

          <section className="space-y-3 rounded-[1.25rem] border border-moon-200/55 bg-white/60 px-4 py-4">
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
            <pre className="max-h-[21rem] overflow-auto rounded-[1rem] bg-[linear-gradient(180deg,rgba(246,244,250,0.78),rgba(241,239,245,0.72))] px-4 py-4 text-[12px] leading-6 text-moon-700">
              <code>{config}</code>
            </pre>
          </section>

          <section className="grid gap-3 sm:grid-cols-2">
            <div className="rounded-[1.15rem] border border-moon-200/50 bg-white/46 px-4 py-3">
              <p className="text-sm font-medium text-moon-800">3. Start Codex CLI</p>
              <p className="mt-1 text-sm leading-6 text-moon-500">
                配置合并完成后，重新打开终端或确认环境变量已生效，然后启动 Codex CLI。
              </p>
            </div>
            <div className="rounded-[1.15rem] border border-dashed border-moon-200/70 bg-white/34 px-4 py-3">
              <p className="inline-flex items-center gap-2 text-sm font-medium text-moon-650">
                <FileText className="size-4" />
                VSCode
              </p>
              <p className="mt-1 text-sm leading-6 text-moon-400">
                插件接入说明将在后续版本补齐；v0.1.3 仅提供 Codex CLI 配置。
              </p>
            </div>
          </section>
        </div>
      </DialogContent>
    </Dialog>
  );
}
