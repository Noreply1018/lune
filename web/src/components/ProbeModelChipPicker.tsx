import { useMemo, useState } from "react";
import { Plus, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { cn } from "@/lib/utils";

type ProbeModelChipPickerProps = {
  value: string[];
  available: string[];
  onChange: (next: string[]) => void;
  disabled?: boolean;
  placeholder?: string;
};

// ProbeModelChipPicker is the detail-drawer control that lets the user pick
// which models the Pool-detail self-check should probe. It mirrors the chip
// style of `MobileChipInput` (same rounded pill look) but replaces free-text
// entry with a "+ 添加" popover listing models the account actually advertises
// via the /models endpoint. Models already selected are filtered out of the
// popover so the same value can never appear twice.
export default function ProbeModelChipPicker({
  value,
  available,
  onChange,
  disabled,
  placeholder = "未配置，默认使用最后发现的模型",
}: ProbeModelChipPickerProps) {
  const [open, setOpen] = useState(false);

  const remaining = useMemo(() => {
    const selected = new Set(value);
    return available.filter((m) => !selected.has(m));
  }, [value, available]);

  function add(model: string) {
    if (!model || value.includes(model)) return;
    onChange([...value, model]);
    setOpen(false);
  }

  function remove(index: number) {
    onChange(value.filter((_, i) => i !== index));
  }

  return (
    <div
      className={cn(
        "flex min-h-[2.5rem] flex-wrap items-center gap-1.5 rounded-[0.9rem] border border-moon-200/70 bg-white/86 px-2 py-1.5 shadow-inner",
        disabled ? "cursor-not-allowed opacity-60" : "",
      )}
    >
      {value.length === 0 ? (
        <span className="px-1 text-xs text-moon-400">{placeholder}</span>
      ) : null}
      {value.map((item, index) => (
        <span
          key={`${item}-${index}`}
          className="inline-flex items-center gap-1 rounded-full bg-moon-100/95 px-2 py-1 text-xs text-moon-700"
        >
          <span className="max-w-[14rem] truncate">{item}</span>
          <button
            type="button"
            className="text-moon-400 hover:text-status-red disabled:opacity-50"
            onClick={() => remove(index)}
            disabled={disabled}
            aria-label={`移除 ${item}`}
          >
            <X className="size-3.5" />
          </button>
        </span>
      ))}
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger
          render={
            <Button
              type="button"
              variant="ghost"
              size="sm"
              disabled={disabled || remaining.length === 0}
              className="h-7 rounded-full border border-dashed border-moon-300/70 px-2.5 text-xs text-moon-600 hover:bg-moon-100/60"
            />
          }
        >
          <Plus className="mr-0.5 size-3.5" />
          {remaining.length === 0 ? "无可添加模型" : "添加"}
        </PopoverTrigger>
        <PopoverContent align="start" className="w-[16rem] p-1">
          <div className="max-h-[14rem] overflow-y-auto">
            {remaining.length === 0 ? (
              <p className="px-2 py-1.5 text-xs text-moon-400">没有更多可选模型</p>
            ) : (
              remaining.map((model) => (
                <button
                  key={model}
                  type="button"
                  onClick={() => add(model)}
                  className="flex w-full items-center rounded-md px-2 py-1.5 text-left text-xs text-moon-700 hover:bg-moon-100/80"
                >
                  <span className="truncate">{model}</span>
                </button>
              ))
            )}
          </div>
        </PopoverContent>
      </Popover>
    </div>
  );
}
