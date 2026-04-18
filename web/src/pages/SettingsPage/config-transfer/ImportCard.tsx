import { useRef, useState, type ChangeEvent } from "react";
import {
  AlertTriangle,
  CheckCheck,
  FileUp,
  RefreshCw,
  Upload,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import { toast } from "@/components/Feedback";
import { api } from "@/lib/api";
import { shortDate } from "@/lib/fmt";
import type {
  ConfigImportPreview,
  ConfigImportResult,
} from "@/lib/types";

type ImportCardProps = {
  onImported: () => Promise<void> | void;
};

type RawPayload = Record<string, unknown>;

export default function ImportCard({ onImported }: ImportCardProps) {
  const [fileName, setFileName] = useState<string | null>(null);
  const [rawPayload, setRawPayload] = useState<RawPayload | null>(null);
  const [preview, setPreview] = useState<ConfigImportPreview | null>(null);
  const [previewing, setPreviewing] = useState(false);
  const [importing, setImporting] = useState(false);
  const fileInputRef = useRef<HTMLInputElement | null>(null);

  function reset() {
    setFileName(null);
    setRawPayload(null);
    setPreview(null);
  }

  async function handleFileChange(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return;

    reset();
    setFileName(file.name);
    setPreviewing(true);
    try {
      const parsed = JSON.parse(await file.text()) as RawPayload;
      if (typeof parsed !== "object" || parsed === null) {
        throw new Error("导入文件格式不正确");
      }
      setRawPayload(parsed);
      const previewData = await api.post<ConfigImportPreview>(
        "/import/preview",
        parsed,
      );
      setPreview(previewData);
    } catch (err) {
      reset();
      toast(err instanceof Error ? err.message : "读取导入文件失败", "error");
    } finally {
      setPreviewing(false);
    }
  }

  async function confirmImport() {
    if (!rawPayload) return;
    setImporting(true);
    try {
      const result = await api.post<ConfigImportResult>(
        "/import",
        rawPayload,
      );
      toast(
        `导入完成：新建 ${result.created_pools} Pool / ${result.created_tokens} Token，跳过 ${result.skipped_tokens} 项`,
      );
      reset();
      await onImported();
    } catch (err) {
      toast(err instanceof Error ? err.message : "导入失败", "error");
    } finally {
      setImporting(false);
    }
  }

  const writePlan = preview
    ? buildWritePlan(preview)
    : [];

  return (
    <div className="flex h-full flex-col rounded-[1.4rem] border border-moon-200/55 bg-[linear-gradient(180deg,rgba(255,255,255,0.94),rgba(244,241,250,0.78))] px-5 py-4 shadow-[0_24px_60px_-50px_rgba(74,68,108,0.32)]">
      <div className="flex items-center gap-2">
        <span className="flex size-7 items-center justify-center rounded-full bg-lunar-100/70 text-lunar-600">
          <Upload className="size-3.5" />
        </span>
        <div>
          <p className="text-sm font-semibold text-moon-800">Import</p>
          <p className="text-[11px] tracking-[0.14em] text-moon-350">
            RESTORE FROM FILE
          </p>
        </div>
      </div>

      {!preview ? (
        <div className="mt-4 flex flex-1 flex-col justify-between gap-4">
          <div className="rounded-[1.1rem] border border-dashed border-moon-200/65 bg-white/55 px-4 py-6 text-center">
            <FileUp className="mx-auto size-7 text-moon-400" />
            <p className="mt-2 text-sm font-medium text-moon-700">
              选择导出的 JSON 文件
            </p>
            <p className="mt-1 text-xs text-moon-450">
              选好后会先预览再导入，不会立即写入数据。
            </p>
          </div>
          <Button
            variant="outline"
            className="w-full rounded-full"
            onClick={() => fileInputRef.current?.click()}
            disabled={previewing}
          >
            {previewing ? (
              <RefreshCw className="size-4 animate-spin" />
            ) : (
              <FileUp className="size-4" />
            )}
            {previewing ? "解析文件中…" : "选择文件"}
          </Button>
          <input
            ref={fileInputRef}
            type="file"
            accept=".json,application/json"
            className="hidden"
            onChange={handleFileChange}
          />
        </div>
      ) : (
        <div className="mt-4 flex flex-1 flex-col gap-3">
          <div className="rounded-[1.1rem] border border-moon-200/45 bg-white/55 px-3.5 py-3">
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium text-moon-800">
                  {fileName ?? "import.json"}
                </p>
                <p className="mt-0.5 text-[11px] text-moon-450">
                  {preview.source_host ? `来自 ${preview.source_host}` : "来源未知"}
                  {preview.exported_at
                    ? ` · 导出于 ${shortDate(preview.exported_at)}`
                    : ""}
                </p>
              </div>
              <span className="inline-flex shrink-0 items-center gap-1 rounded-full bg-lunar-100/70 px-2 py-0.5 text-[10px] font-medium tracking-[0.1em] text-lunar-600">
                schema {preview.schema_version || "?"}
              </span>
            </div>
            {preview.include_secrets ? (
              <div className="mt-3 flex items-start gap-2 rounded-[0.85rem] border border-status-yellow/30 bg-status-yellow/10 px-3 py-2 text-xs text-status-yellow">
                <AlertTriangle className="mt-0.5 size-3.5 shrink-0" />
                <p className="leading-5">
                  文件含完整密钥。即便如此，Token 仍会由本机重新生成。
                </p>
              </div>
            ) : null}
          </div>

          <div className="rounded-[1.1rem] border border-moon-200/45 bg-white/55 px-3.5 py-3">
            <p className="text-[11px] tracking-[0.16em] text-moon-300">
              将执行的写入
            </p>
            <ul className="mt-2 space-y-1 text-xs text-moon-600">
              {writePlan.map((item) => (
                <li
                  key={item.label}
                  className="flex items-center justify-between gap-3"
                >
                  <span className="flex items-center gap-2">
                    <CheckCheck className="size-3.5 text-status-green" />
                    {item.label}
                  </span>
                  <span className="font-medium tabular-nums text-moon-700">
                    {item.value}
                  </span>
                </li>
              ))}
            </ul>
            {preview.ignored_accounts + preview.ignored_services > 0 ? (
              <p className="mt-2 text-[11px] leading-4 text-moon-400">
                文件中的 {preview.ignored_accounts} 个账号 /{" "}
                {preview.ignored_services} 个 CPA Service 不会被导入（需手动在
                Overview / Settings 重新接入）。
              </p>
            ) : null}
          </div>

          <div className="mt-auto flex items-center justify-end gap-2">
            <Button
              variant="outline"
              size="sm"
              className="rounded-full"
              onClick={reset}
              disabled={importing}
            >
              取消
            </Button>
            <Button
              size="sm"
              className="rounded-full"
              onClick={() => void confirmImport()}
              disabled={importing}
            >
              {importing ? (
                <RefreshCw className="size-4 animate-spin" />
              ) : (
                <Upload className="size-4" />
              )}
              确认导入
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}

function buildWritePlan(preview: ConfigImportPreview) {
  const plan: Array<{ label: string; value: string }> = [];
  plan.push({
    label: "Pools（新建 / 更新）",
    value: `${preview.created_pools} / ${preview.updated_pools}`,
  });
  plan.push({
    label: "Tokens（新建 / 跳过）",
    value: `${preview.created_tokens} / ${preview.skipped_tokens}`,
  });
  plan.push({
    label: "Settings 覆盖",
    value: `${preview.updated_settings} 项`,
  });
  return plan;
}
