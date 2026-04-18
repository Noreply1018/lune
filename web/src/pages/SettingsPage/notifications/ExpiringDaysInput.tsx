import { useEffect, useRef, useState } from "react";

import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";

type ExpiringDaysInputProps = {
  value: number;
  error?: string | null;
  onCommit: (next: number) => void;
  onClearError: () => void;
};

// ExpiringDaysInput is a small self-contained input for the
// account_expiring threshold. It owns a draft string so the user can type
// freely, and only reconciles with the server value on Enter/blur (commit)
// or Esc (rollback). Invalid drafts (NaN, <= 0, non-integer) are discarded
// on blur by restoring the last server-accepted value.
export default function ExpiringDaysInput({
  value,
  error,
  onCommit,
  onClearError,
}: ExpiringDaysInputProps) {
  const [draft, setDraft] = useState(() => String(value));
  const committedRef = useRef(value);

  useEffect(() => {
    // Sync draft whenever an external change to the committed server value
    // arrives (e.g. initial load, parent refetch, or error rollback). We
    // skip the sync when value already equals committedRef.current — that
    // means the change came from our own commit() below, in which case the
    // parent is optimistically mirroring what we already track locally and
    // we must not overwrite the user's in-progress typing.
    if (value === committedRef.current) {
      return;
    }
    committedRef.current = value;
    setDraft(String(value));
  }, [value]);

  function parseDraft(s: string): number | null {
    const trimmed = s.trim();
    if (!/^\d+$/.test(trimmed)) {
      return null;
    }
    const n = Number(trimmed);
    if (!Number.isFinite(n) || n <= 0) {
      return null;
    }
    return n;
  }

  function commit() {
    const parsed = parseDraft(draft);
    if (parsed === null) {
      // Invalid draft: roll back to the last committed server value so the
      // visible number always reflects persisted state.
      setDraft(String(committedRef.current));
      return;
    }
    if (parsed === committedRef.current) {
      return;
    }
    // Advance committedRef locally so the upcoming value-prop change from
    // the parent's optimistic update is recognized as a no-op and doesn't
    // stomp the user's draft mid-edit.
    committedRef.current = parsed;
    onCommit(parsed);
  }

  return (
    <div className="space-y-1.5">
      <label className="text-xs font-medium uppercase tracking-[0.18em] text-moon-450">
        过期提醒阈值
      </label>
      <div className="flex items-center gap-2">
        <Input
          type="number"
          inputMode="numeric"
          min={1}
          step={1}
          value={draft}
          onChange={(event) => {
            setDraft(event.target.value);
            if (error) {
              onClearError();
            }
          }}
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              event.preventDefault();
              commit();
              (event.target as HTMLInputElement).blur();
              return;
            }
            if (event.key === "Escape") {
              event.preventDefault();
              setDraft(String(committedRef.current));
              onClearError();
              (event.target as HTMLInputElement).blur();
              return;
            }
            // Block signs and the exponent notation that `type=number`
            // would otherwise accept; the setting only makes sense as a
            // positive integer.
            if (event.key === "-" || event.key === "+" || event.key === "e" || event.key === "E" || event.key === ".") {
              event.preventDefault();
            }
          }}
          onBlur={commit}
          className={cn("w-20", error ? "border-status-red/60" : "")}
        />
        <span className="text-xs text-moon-400">天</span>
      </div>
      <p className="text-[11px] leading-4 text-moon-400">
        账号凭据剩余时间小于该天数时触发本事件。按 Enter 保存，Esc 取消。
      </p>
      {error ? <p className="text-xs text-status-red">{error}</p> : null}
    </div>
  );
}
