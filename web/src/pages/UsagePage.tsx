import { useEffect, useState } from "react";
import { luneGet } from "../lib/api";
import { compact } from "../lib/fmt";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";

type UsageSummary = {
  total_entries: number;
  successful: number;
  failed: number;
  by_account: Record<string, number>;
  by_token: Record<string, number>;
};

export default function UsagePage() {
  const [usage, setUsage] = useState<UsageSummary | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    luneGet<UsageSummary>("/admin/api/usage")
      .then(setUsage)
      .finally(() => setLoading(false));
  }, []);

  if (loading) {
    return (
      <div className="space-y-8">
        <Skeleton className="h-7 w-20" />
        <Skeleton className="h-48" />
      </div>
    );
  }

  const byAccountEntries = usage ? Object.entries(usage.by_account) : [];
  const byTokenEntries = usage ? Object.entries(usage.by_token) : [];

  return (
    <div className="space-y-8">
      <h2 className="text-xl font-semibold">Usage</h2>

      <div className="grid gap-6 md:grid-cols-2">
        <section>
          <h3 className="mb-3 text-sm font-medium text-muted-foreground">
            By Account
          </h3>
          <Card>
            <CardContent className="p-4 space-y-2">
              {byAccountEntries.length === 0 ? (
                <p className="text-sm text-muted-foreground">No data</p>
              ) : (
                byAccountEntries.map(([name, count]) => (
                  <div
                    key={name}
                    className="flex items-center justify-between text-sm"
                  >
                    <span className="font-medium">{name}</span>
                    <span className="text-muted-foreground">
                      {compact(count)}
                    </span>
                  </div>
                ))
              )}
            </CardContent>
          </Card>
        </section>

        <section>
          <h3 className="mb-3 text-sm font-medium text-muted-foreground">
            By Token
          </h3>
          <Card>
            <CardContent className="p-4 space-y-2">
              {byTokenEntries.length === 0 ? (
                <p className="text-sm text-muted-foreground">No data</p>
              ) : (
                byTokenEntries.map(([name, count]) => (
                  <div
                    key={name}
                    className="flex items-center justify-between text-sm"
                  >
                    <span className="font-medium">{name}</span>
                    <span className="text-muted-foreground">
                      {compact(count)}
                    </span>
                  </div>
                ))
              )}
            </CardContent>
          </Card>
        </section>
      </div>
    </div>
  );
}
