import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

const styles: Record<string, string> = {
  ok: "bg-sage-500/15 text-sage-600 border-sage-500/30",
  error: "bg-clay-500/15 text-clay-600 border-clay-500/30",
  disabled: "bg-muted text-muted-foreground border-border",
};

export default function StatusBadge({
  status,
  label,
}: {
  status: "ok" | "error" | "disabled";
  label?: string;
}) {
  return (
    <Badge
      variant="outline"
      className={cn("text-xs font-medium", styles[status] ?? styles.disabled)}
    >
      {label ?? status}
    </Badge>
  );
}
