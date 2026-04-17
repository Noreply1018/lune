export default function TemplateOverrideEditor({
  label,
  mode,
  value,
  defaultValue,
  onModeChange,
  onValueChange,
  onBlur,
}: {
  label: string;
  mode: "default" | "custom";
  value: string;
  defaultValue: string;
  onModeChange: (value: "default" | "custom") => void;
  onValueChange: (value: string) => void;
  onBlur?: () => void;
}) {
  return (
    <div
      className="space-y-3"
      onBlur={(event) => {
        if (
          onBlur &&
          !event.currentTarget.contains(event.relatedTarget as Node | null)
        ) {
          onBlur();
        }
      }}
    >
      <div className="flex flex-wrap items-center gap-3">
        <p className="text-sm font-medium text-moon-700">{label}</p>
        <label className="inline-flex items-center gap-2 text-xs text-moon-500">
          <input
            type="radio"
            checked={mode === "default"}
            onChange={() => onModeChange("default")}
          />
          使用默认
        </label>
        <label className="inline-flex items-center gap-2 text-xs text-moon-500">
          <input
            type="radio"
            checked={mode === "custom"}
            onChange={() => onModeChange("custom")}
          />
          自定义
        </label>
      </div>
      {mode === "custom" ? (
        <textarea
          value={value}
          onChange={(event) => onValueChange(event.target.value)}
          onBlur={onBlur}
          className="min-h-28 w-full rounded-[1rem] border border-moon-200/65 bg-white/82 px-3 py-3 text-sm text-moon-700 outline-none transition focus:border-lunar-300/70"
        />
      ) : null}
      <div className="rounded-[1rem] border border-moon-200/55 bg-moon-50/78 px-3 py-3">
        <p className="text-[11px] tracking-[0.16em] text-moon-350">DEFAULT</p>
        <pre className="mt-2 whitespace-pre-wrap text-[12px] leading-5 text-moon-500">
          {defaultValue || "--"}
        </pre>
      </div>
    </div>
  );
}
