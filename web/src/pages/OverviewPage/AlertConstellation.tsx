import { useEffect, useMemo, useRef, useState } from "react";
import { X } from "lucide-react";
import { cn } from "@/lib/utils";
import type { OverviewAlert } from "@/lib/types";
import {
  fingerprint,
  filterVisible,
  formatCn,
  type ParsedAlert,
} from "./alertUtils";

type AlertConstellationProps = {
  alerts: OverviewAlert[];
  tone: "calm" | "warning" | "critical";
  dismissed: Set<string>;
  onDismiss: (fp: string) => void;
  onAlertClick: (alert: OverviewAlert) => void;
};

const TONE_COLOR: Record<AlertConstellationProps["tone"], { dot: string; ring: string; text: string }> = {
  calm: {
    dot: "rgba(120,160,140,0.8)",
    ring: "rgba(120,160,140,0.35)",
    text: "text-status-green",
  },
  warning: {
    dot: "rgba(192,154,85,0.92)",
    ring: "rgba(192,154,85,0.35)",
    text: "text-status-yellow",
  },
  critical: {
    dot: "rgba(190,116,118,0.95)",
    ring: "rgba(190,116,118,0.4)",
    text: "text-status-red",
  },
};

export default function AlertConstellation({
  alerts,
  tone,
  dismissed,
  onDismiss,
  onAlertClick,
}: AlertConstellationProps) {
  const [open, setOpen] = useState(false);
  const [hover, setHover] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    function onDocPointer(e: PointerEvent) {
      if (!containerRef.current) return;
      if (containerRef.current.contains(e.target as Node)) return;
      setOpen(false);
    }
    document.addEventListener("pointerdown", onDocPointer);
    return () => document.removeEventListener("pointerdown", onDocPointer);
  }, [open]);

  const visible = useMemo(() => filterVisible(alerts, dismissed), [alerts, dismissed]);

  if (visible.length === 0) return null;

  const palette = TONE_COLOR[tone];
  const count = visible.length;

  function handleDismiss(alert: OverviewAlert, parsed: ParsedAlert) {
    onDismiss(fingerprint(alert, parsed));
  }

  return (
    <div ref={containerRef} className="pointer-events-auto flex flex-col items-end gap-2">
      <style>{`
        @keyframes lune-alert-pulse {
          0%   { transform: scale(1);   opacity: 0.55; }
          70%  { transform: scale(2.2); opacity: 0;    }
          100% { transform: scale(2.2); opacity: 0;    }
        }
      `}</style>
      <button
        type="button"
        onMouseEnter={() => setHover(true)}
        onMouseLeave={() => setHover(false)}
        onClick={() => setOpen((v) => !v)}
        className="relative inline-flex items-center gap-2 rounded-full px-2 py-1 transition-colors hover:bg-white/40"
        aria-label={`${count} 条提醒`}
      >
        <span className="relative inline-flex size-2.5 items-center justify-center">
          <span
            className="absolute inset-0 rounded-full"
            style={{
              background: palette.ring,
              animation: "lune-alert-pulse 2.4s ease-out infinite",
              transformOrigin: "center",
            }}
          />
          <span
            className="relative inline-block size-2.5 rounded-full"
            style={{ background: palette.dot, boxShadow: `0 0 8px ${palette.ring}` }}
          />
        </span>
        <span
          className={cn(
            "text-[10px] font-medium uppercase tracking-[0.3em] transition-opacity duration-300",
            hover || open ? "opacity-100" : "opacity-0",
          )}
        >
          <span className={palette.text}>{count}</span>
          <span className="ml-1 text-moon-500">提醒</span>
        </span>
      </button>

      {open ? (
        <div className="flex max-h-[52vh] w-[min(26rem,82vw)] flex-col rounded-[1.2rem] border border-moon-200/55 bg-white/90 p-3 shadow-[0_30px_70px_-40px_rgba(33,40,63,0.5)] backdrop-blur-md">
          <div className="mb-2 flex items-center justify-between">
            <p className="text-[10px] font-semibold uppercase tracking-[0.28em] text-moon-400">
              提醒
            </p>
            <button
              type="button"
              onClick={() => setOpen(false)}
              className="text-[11px] text-moon-500 hover:text-moon-800"
            >
              收起
            </button>
          </div>
          <ul className="space-y-1.5 overflow-y-auto pr-1">
            {visible.map(({ alert, parsed }, i) => {
              const canDismiss = parsed.kind === "account_expiring";
              return (
                <li key={`${alert.type}-${i}`}>
                  <div className="group flex items-stretch gap-1 rounded-lg border border-moon-200/45 bg-white/55 text-moon-700 transition-colors hover:bg-white/85">
                    <button
                      type="button"
                      onClick={() => {
                        onAlertClick(alert);
                        setOpen(false);
                      }}
                      className="flex-1 px-3 py-2 text-left text-sm leading-relaxed"
                      title={parsed.detail ? parsed.detail : undefined}
                    >
                      {formatCn(alert, parsed)}
                    </button>
                    {canDismiss ? (
                      <button
                        type="button"
                        onClick={(e) => {
                          e.stopPropagation();
                          handleDismiss(alert, parsed);
                        }}
                        aria-label="标记为已读"
                        title="标记为已读（下次仍出现相同提醒会再显示）"
                        className="flex w-8 items-center justify-center rounded-r-lg text-moon-400 opacity-0 transition-opacity hover:text-moon-700 group-hover:opacity-100"
                      >
                        <X className="size-3.5" />
                      </button>
                    ) : null}
                  </div>
                </li>
              );
            })}
          </ul>
        </div>
      ) : null}
    </div>
  );
}
