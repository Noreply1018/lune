import { cn } from "@/lib/utils";

type Status = "healthy" | "degraded" | "error" | "disabled";

const dotColor: Record<Status, string> = {
  healthy: "bg-status-green",
  degraded: "bg-status-yellow animate-pulse",
  error: "bg-status-red",
  disabled: "bg-moon-400",
};

const badgeStyle: Record<Status, string> = {
  healthy: "bg-status-green/10 text-status-green border-status-green/20",
  degraded: "bg-status-yellow/10 text-status-yellow border-status-yellow/20",
  error: "bg-status-red/10 text-status-red border-status-red/20",
  disabled: "bg-moon-200/50 text-moon-400 border-moon-200",
};

export default function StatusBadge({
  status,
  label,
}: {
  status: Status;
  label?: string;
}) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-xs font-medium",
        badgeStyle[status] ?? badgeStyle.disabled,
      )}
    >
      <span
        className={cn(
          "size-1.5 rounded-full",
          dotColor[status] ?? dotColor.disabled,
        )}
      />
      {label ?? status}
    </span>
  );
}
