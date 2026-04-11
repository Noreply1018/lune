import { Card, CardContent } from "@/components/ui/card";

export default function StatCard({
  label,
  value,
  sub,
}: {
  label: string;
  value: string;
  sub?: string;
}) {
  return (
    <Card>
      <CardContent className="px-5 py-4">
        <p className="text-xs text-muted-foreground tracking-wide">{label}</p>
        <p className="mt-1 text-2xl font-semibold">{value}</p>
        {sub && (
          <p className="mt-0.5 text-xs text-muted-foreground/60">{sub}</p>
        )}
      </CardContent>
    </Card>
  );
}
