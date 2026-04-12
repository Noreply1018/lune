import type { LucideIcon } from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";

export default function StatCard({
  label,
  value,
  sub,
  icon: Icon,
}: {
  label: string;
  value: string;
  sub?: string;
  icon?: LucideIcon;
}) {
  return (
    <Card className="ring-1 ring-moon-200/60">
      <CardContent className="relative px-6 py-5">
        {Icon && (
          <Icon className="absolute right-5 top-5 size-5 text-moon-300" />
        )}
        <p className="text-xs font-medium uppercase tracking-wider text-moon-400">
          {label}
        </p>
        <p
          className="mt-1 text-2xl font-semibold text-moon-800"
          style={{
            fontFamily:
              '"Iowan Old Style","Palatino Linotype","Noto Serif SC",Georgia,serif',
          }}
        >
          {value}
        </p>
        {sub && <p className="mt-0.5 text-xs text-moon-400">{sub}</p>}
      </CardContent>
    </Card>
  );
}
