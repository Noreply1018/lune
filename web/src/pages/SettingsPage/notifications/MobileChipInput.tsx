import { useRef, useState, type KeyboardEvent } from "react";
import { X } from "lucide-react";

import { cn } from "@/lib/utils";

import { MENTION_ALL, MOBILE_PATTERN } from "./types";

type MobileChipInputProps = {
  value: string[];
  onChange: (next: string[]) => void;
  onCommit: (next: string[]) => void;
  disabled?: boolean;
};

export default function MobileChipInput({
  value,
  onChange,
  onCommit,
  disabled,
}: MobileChipInputProps) {
  const inputRef = useRef<HTMLInputElement | null>(null);
  const [draft, setDraft] = useState("");
  const [draftError, setDraftError] = useState<string | null>(null);

  function isValid(candidate: string): boolean {
    return candidate === MENTION_ALL || MOBILE_PATTERN.test(candidate);
  }

  function commitDraft(): string[] | null {
    const candidate = draft.trim();
    if (!candidate) {
      setDraftError(null);
      return null;
    }
    if (!isValid(candidate)) {
      setDraftError("必须是 11 位手机号或 @all");
      return null;
    }
    if (value.includes(candidate)) {
      setDraft("");
      setDraftError(null);
      return null;
    }
    const next = [...value, candidate];
    onChange(next);
    setDraft("");
    setDraftError(null);
    return next;
  }

  function removeAt(index: number) {
    const next = value.filter((_, idx) => idx !== index);
    onChange(next);
    onCommit(next);
  }

  function handleKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "Enter" || event.key === ",") {
      event.preventDefault();
      const next = commitDraft();
      if (next) {
        onCommit(next);
      }
      return;
    }
    if (event.key === "Backspace" && draft === "" && value.length > 0) {
      event.preventDefault();
      removeAt(value.length - 1);
    }
  }

  return (
    <div className="space-y-1.5">
      <div
        className={cn(
          "flex min-h-[2.5rem] flex-wrap items-center gap-1.5 rounded-[0.9rem] border border-moon-200/70 bg-white/86 px-2 py-1.5 shadow-inner focus-within:ring-2 focus-within:ring-moon-500/40",
          draftError ? "border-status-red/60 focus-within:ring-status-red/40" : "",
          disabled ? "cursor-not-allowed opacity-60" : "",
        )}
        onClick={() => inputRef.current?.focus()}
      >
        {value.map((item, index) => (
          <span
            key={`${item}-${index}`}
            className="inline-flex items-center gap-1 rounded-full bg-moon-100/95 px-2 py-1 text-xs text-moon-700"
          >
            <span>{item}</span>
            <button
              type="button"
              className="text-moon-400 hover:text-status-red"
              onClick={(event) => {
                event.stopPropagation();
                removeAt(index);
              }}
              disabled={disabled}
              aria-label={`移除 ${item}`}
            >
              <X className="size-3.5" />
            </button>
          </span>
        ))}
        <input
          ref={inputRef}
          value={draft}
          onChange={(event) => {
            setDraft(event.target.value);
            if (draftError) {
              setDraftError(null);
            }
          }}
          onKeyDown={handleKeyDown}
          onBlur={() => {
            const next = commitDraft();
            if (next) {
              onCommit(next);
            }
          }}
          disabled={disabled}
          placeholder={
            value.length ? "" : "输入 11 位手机号或 @all，回车确认"
          }
          className="min-w-[12rem] flex-1 border-none bg-transparent px-1 py-0.5 text-sm text-moon-700 outline-none"
        />
      </div>
      {draftError ? (
        <p className="text-xs text-status-red">{draftError}</p>
      ) : null}
    </div>
  );
}
