import { useEffect, useRef, useState } from "react";
import { cn } from "@/lib/utils";
import type { OverviewAlert } from "@/lib/types";

type AlertConstellationProps = {
  alerts: OverviewAlert[];
  tone: "calm" | "warning" | "critical";
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

export default function AlertConstellation({ alerts, tone, onAlertClick }: AlertConstellationProps) {
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

  if (alerts.length === 0) return null;

  const palette = TONE_COLOR[tone];
  const count = alerts.length;

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
        <div className="flex max-h-[52vh] w-[min(24rem,80vw)] flex-col rounded-[1.2rem] border border-moon-200/55 bg-white/92 p-3 shadow-[0_30px_70px_-40px_rgba(33,40,63,0.5)] backdrop-blur-md">
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
            {alerts.map((alert, i) => (
              <li key={`${alert.type}-${i}`}>
                <button
                  type="button"
                  onClick={() => {
                    onAlertClick(alert);
                    setOpen(false);
                  }}
                  className="w-full rounded-lg border border-moon-200/45 bg-white/55 px-3 py-2 text-left text-sm text-moon-700 transition-colors hover:bg-white/85"
                >
                  {alert.message}
                </button>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </div>
  );
}
