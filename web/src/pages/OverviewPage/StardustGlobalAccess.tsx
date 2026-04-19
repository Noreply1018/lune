import { useEffect, useRef, useState } from "react";
import { Check, Copy, KeyRound, QrCode } from "lucide-react";
import { cn } from "@/lib/utils";

type StardustGlobalAccessProps = {
  baseUrl: string;
  tokenMasked: string;
  hasToken: boolean;
  onCopyToken: () => Promise<void>;
  onCopyBaseUrl: () => Promise<void>;
  onOpenSnippets: () => void;
  onOpenQr: () => void;
};

export default function StardustGlobalAccess({
  baseUrl,
  tokenMasked,
  hasToken,
  onCopyToken,
  onCopyBaseUrl,
  onOpenSnippets,
  onOpenQr,
}: StardustGlobalAccessProps) {
  const [awake, setAwake] = useState(false);
  const awakeTimerRef = useRef<number | null>(null);

  function touch() {
    setAwake(true);
    if (awakeTimerRef.current) {
      window.clearTimeout(awakeTimerRef.current);
    }
    awakeTimerRef.current = window.setTimeout(() => setAwake(false), 2200);
  }

  useEffect(() => {
    return () => {
      if (awakeTimerRef.current) window.clearTimeout(awakeTimerRef.current);
    };
  }, []);

  return (
    <div
      onMouseEnter={() => {
        setAwake(true);
        if (awakeTimerRef.current) {
          window.clearTimeout(awakeTimerRef.current);
          awakeTimerRef.current = null;
        }
      }}
      onMouseLeave={() => touch()}
      className={cn(
        "pointer-events-auto space-y-1.5 transition-opacity duration-700",
        awake ? "opacity-100" : "opacity-40",
      )}
    >
      <StardustRow
        label="API"
        value={baseUrl}
        onCopy={onCopyBaseUrl}
        awake={awake}
      />
      <StardustRow
        label="KEY"
        value={hasToken ? tokenMasked : "未配置全局 Token"}
        onCopy={hasToken ? onCopyToken : undefined}
        awake={awake}
        muted={!hasToken}
      />
      <div
        className={cn(
          "flex items-center justify-end gap-2 pt-1 transition-all duration-500",
          awake ? "translate-y-0 opacity-100" : "pointer-events-none -translate-y-1 opacity-0",
        )}
      >
        <StardustAction
          icon={<KeyRound className="size-3" />}
          label="Env"
          onClick={onOpenSnippets}
          disabled={!hasToken}
        />
        <StardustAction
          icon={<QrCode className="size-3" />}
          label="QR"
          onClick={onOpenQr}
          disabled={!hasToken}
        />
      </div>
    </div>
  );
}

function StardustRow({
  label,
  value,
  onCopy,
  awake,
  muted,
}: {
  label: string;
  value: string;
  onCopy?: () => Promise<void>;
  awake: boolean;
  muted?: boolean;
}) {
  const [copied, setCopied] = useState(false);
  const timerRef = useRef<number | null>(null);

  useEffect(() => {
    return () => {
      if (timerRef.current) window.clearTimeout(timerRef.current);
    };
  }, []);

  async function handleCopy() {
    if (!onCopy) return;
    try {
      await onCopy();
      setCopied(true);
      if (timerRef.current) window.clearTimeout(timerRef.current);
      timerRef.current = window.setTimeout(() => setCopied(false), 1400);
    } catch {
      /* caller toasts */
    }
  }

  return (
    <div className="flex items-center justify-end gap-2.5">
      <span className="text-[9px] font-medium uppercase tracking-[0.32em] text-moon-400">
        {label}
      </span>
      <span
        className={cn(
          "max-w-[22rem] truncate text-right font-mono text-[11px] transition-colors duration-500",
          muted ? "text-moon-400" : awake ? "text-moon-700" : "text-moon-500",
        )}
      >
        {value}
      </span>
      {onCopy ? (
        <button
          type="button"
          onClick={handleCopy}
          aria-label={`复制 ${label}`}
          className={cn(
            "inline-flex size-5 items-center justify-center rounded-full transition-all duration-400",
            awake ? "text-moon-500 hover:text-moon-800" : "pointer-events-none text-transparent",
          )}
        >
          {copied ? (
            <Check className="size-3 text-status-green" />
          ) : (
            <Copy className="size-3" />
          )}
        </button>
      ) : (
        <span className="inline-block size-5" />
      )}
    </div>
  );
}

function StardustAction({
  icon,
  label,
  onClick,
  disabled,
}: {
  icon: React.ReactNode;
  label: string;
  onClick: () => void;
  disabled?: boolean;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className={cn(
        "inline-flex items-center gap-1 rounded-full border border-moon-200/55 bg-white/60 px-2.5 py-1 text-[10px] font-medium text-moon-600 backdrop-blur-sm transition-colors",
        "hover:border-moon-300 hover:bg-white/85 hover:text-moon-800",
        "disabled:cursor-not-allowed disabled:opacity-45",
      )}
    >
      {icon}
      {label}
    </button>
  );
}
