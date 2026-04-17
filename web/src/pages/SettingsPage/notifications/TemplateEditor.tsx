import { useEffect, useMemo, useRef, useState } from "react";
import { ChevronDown, RotateCcw } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";

import { renderPreview, type PlaceholderMeta } from "./types";

type TemplateEditorProps = {
  label: string;
  value: string;
  defaultValue: string;
  placeholders: PlaceholderMeta[];
  disabled?: boolean;
  minRows?: number;
  onChange: (nextDisplay: string) => void;
  onCommit: () => void;
  error?: string | null;
};

export default function TemplateEditor({
  label,
  value,
  defaultValue,
  placeholders,
  disabled = false,
  minRows = 2,
  onChange,
  onCommit,
  error,
}: TemplateEditorProps) {
  const textareaRef = useRef<HTMLTextAreaElement | null>(null);
  const [preview, setPreview] = useState(() => renderPreview(value, placeholders));
  const debounceRef = useRef<number | null>(null);

  useEffect(() => {
    if (debounceRef.current != null) {
      window.clearTimeout(debounceRef.current);
    }
    debounceRef.current = window.setTimeout(() => {
      setPreview(renderPreview(value, placeholders));
    }, 150);
    return () => {
      if (debounceRef.current != null) {
        window.clearTimeout(debounceRef.current);
      }
    };
  }, [value, placeholders]);

  const canReset = useMemo(
    () => value !== defaultValue,
    [value, defaultValue],
  );

  function insertPlaceholder(meta: PlaceholderMeta) {
    const el = textareaRef.current;
    if (!el) {
      return;
    }
    const start = el.selectionStart ?? value.length;
    const end = el.selectionEnd ?? value.length;
    const next = value.slice(0, start) + meta.display + value.slice(end);
    onChange(next);
    requestAnimationFrame(() => {
      const node = textareaRef.current;
      if (!node) {
        return;
      }
      const cursor = start + meta.display.length;
      node.focus();
      node.setSelectionRange(cursor, cursor);
    });
  }

  const trimmedEmpty = value.trim() === "";
  const previewDisplay = trimmedEmpty ? "（内容为空）" : preview;

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between gap-2">
        <label className="text-xs font-medium uppercase tracking-[0.18em] text-moon-450">
          {label}
        </label>
        <div className="flex items-center gap-1.5">
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <Button
                  size="sm"
                  variant="outline"
                  className="h-8 rounded-full px-3 text-xs"
                  disabled={disabled}
                />
              }
            >
              插入字段
              <ChevronDown className="size-3.5" />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-56">
              {placeholders.map((meta) => (
                <DropdownMenuItem
                  key={meta.template}
                  onSelect={(event) => {
                    event.preventDefault();
                    insertPlaceholder(meta);
                  }}
                >
                  <div className="flex w-full flex-col gap-0.5">
                    <span className="font-mono text-xs text-moon-700">
                      {meta.display}
                    </span>
                    {meta.description ? (
                      <span className="text-[11px] text-moon-400">
                        {meta.description}
                      </span>
                    ) : null}
                  </div>
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
          <Button
            size="sm"
            variant="ghost"
            className="h-8 rounded-full px-2 text-xs text-moon-450 hover:text-moon-700"
            onClick={() => {
              onChange(defaultValue);
              requestAnimationFrame(() => onCommit());
            }}
            disabled={disabled || !canReset}
          >
            <RotateCcw className="size-3.5" />
            恢复默认
          </Button>
        </div>
      </div>
      <textarea
        ref={textareaRef}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        onBlur={() => onCommit()}
        disabled={disabled}
        rows={minRows}
        className={cn(
          "w-full resize-y rounded-[0.9rem] border border-moon-200/70 bg-white/86 px-3 py-2 text-sm text-moon-700 shadow-inner focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-moon-500/40",
          error
            ? "border-status-red/60 focus-visible:ring-status-red/40"
            : "border-moon-200/70",
          disabled ? "cursor-not-allowed opacity-60" : "",
        )}
      />
      {error ? (
        <p className="text-xs text-status-red">{error}</p>
      ) : null}
      <div className="rounded-[0.75rem] border border-dashed border-moon-200/70 bg-moon-50/70 px-3 py-1.5">
        <p className="text-[10px] uppercase tracking-[0.18em] text-moon-400">
          预览
        </p>
        <p className="line-clamp-2 text-xs leading-5 text-moon-600">
          {previewDisplay}
        </p>
      </div>
    </div>
  );
}
