import { compact } from "@/lib/fmt";
import { cn } from "@/lib/utils";

export default function UsageBar({
  used,
  total,
  className,
}: {
  used: number;
  total: number;
  className?: string;
}) {
  if (total === 0) {
    return (
      <span className={cn("text-xs text-moon-400", className)}>
        {compact(used)} / unlimited
      </span>
    );
  }

  const pct = Math.min(100, (used / total) * 100);
  const color =
    pct > 90
      ? "bg-status-red"
      : pct > 70
        ? "bg-status-yellow"
        : "bg-status-green";

  return (
    <div className={cn("flex items-center gap-2", className)}>
      <div className="h-1.5 flex-1 rounded-full bg-moon-200">
        <div
          className={cn("h-full rounded-full transition-all", color)}
          style={{ width: `${pct}%` }}
        />
      </div>
      <span className="shrink-0 text-xs text-moon-500">
        {compact(used)} / {compact(total)}
      </span>
    </div>
  );
}
