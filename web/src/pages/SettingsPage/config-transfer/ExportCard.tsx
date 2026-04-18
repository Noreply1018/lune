import { useState } from "react";
import { Download, ShieldCheck, Unlock } from "lucide-react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

type ExportMode = "masked" | "full";

export default function ExportCard() {
  const [mode, setMode] = useState<ExportMode>("masked");

  function triggerDownload() {
    const params = new URLSearchParams();
    if (mode === "full") {
      params.set("include_secrets", "true");
    }
    const url = params.toString()
      ? `/admin/api/export?${params.toString()}`
      : "/admin/api/export";

    // Use an anchor with `download` so Safari/Firefox both honor the
    // Content-Disposition filename consistently (window.location.href is
    // flakier across browsers). The anchor is appended + clicked + removed
    // synchronously so it never enters the live DOM the user sees.
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.rel = "noopener";
    anchor.setAttribute("download", "");
    document.body.appendChild(anchor);
    anchor.click();
    document.body.removeChild(anchor);
  }

  return (
    <div className="flex h-full flex-col rounded-[1.4rem] border border-moon-200/55 bg-[linear-gradient(180deg,rgba(255,255,255,0.94),rgba(244,241,250,0.78))] px-5 py-4 shadow-[0_24px_60px_-50px_rgba(74,68,108,0.32)]">
      <div className="flex items-center gap-2">
        <span className="flex size-7 items-center justify-center rounded-full bg-lunar-100/70 text-lunar-600">
          <Download className="size-3.5" />
        </span>
        <div>
          <p className="text-sm font-semibold text-moon-800">Export</p>
          <p className="text-[11px] tracking-[0.14em] text-moon-350">
            SNAPSHOT TO FILE
          </p>
        </div>
      </div>

      <div className="mt-4 space-y-2.5">
        <ModeOption
          active={mode === "masked"}
          onClick={() => setMode("masked")}
          icon={<ShieldCheck className="size-4" />}
          title="脱敏导出"
          hint="默认推荐 · 所有密钥被打码，可直接分享"
        />
        <ModeOption
          active={mode === "full"}
          onClick={() => setMode("full")}
          icon={<Unlock className="size-4" />}
          title="完整导出"
          hint="含明文 Token / API Key / Management Key"
          warning
        />
      </div>

      <div className="mt-auto pt-4">
        <Button
          className="w-full rounded-full"
          onClick={triggerDownload}
        >
          <Download className="size-4" />
          下载配置文件
        </Button>
        <p className="mt-2 text-[11px] leading-4 text-moon-400">
          文件名：lune-export-[时间戳]-[主机名].json · 包含 schema_version 便于校验
        </p>
      </div>
    </div>
  );
}

function ModeOption({
  active,
  onClick,
  icon,
  title,
  hint,
  warning,
}: {
  active: boolean;
  onClick: () => void;
  icon: React.ReactNode;
  title: string;
  hint: string;
  warning?: boolean;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "flex w-full items-start gap-3 rounded-[1.1rem] border px-3.5 py-3 text-left transition-colors",
        active
          ? "border-lunar-300/65 bg-lunar-100/55 shadow-[inset_0_1px_0_rgba(255,255,255,0.55)]"
          : "border-moon-200/55 bg-white/55 hover:border-moon-250/75 hover:bg-white/72",
      )}
    >
      <span
        className={cn(
          "mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full",
          active
            ? warning
              ? "bg-status-yellow/15 text-status-yellow"
              : "bg-lunar-200/70 text-lunar-700"
            : "bg-moon-100/60 text-moon-500",
        )}
      >
        {icon}
      </span>
      <span className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="flex items-center gap-2 text-sm font-medium text-moon-800">
          {title}
          <span
            className={cn(
              "flex size-3.5 items-center justify-center rounded-full border",
              active
                ? "border-lunar-400 bg-lunar-500"
                : "border-moon-300/70 bg-white",
            )}
            aria-hidden
          >
            {active ? (
              <span className="size-1.5 rounded-full bg-white" />
            ) : null}
          </span>
        </span>
        <span className="text-xs leading-5 text-moon-500">{hint}</span>
      </span>
    </button>
  );
}
