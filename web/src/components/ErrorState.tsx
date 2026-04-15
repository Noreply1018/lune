import { AlertTriangle } from "lucide-react";
import { Button } from "@/components/ui/button";

export default function ErrorState({
  title = "加载失败",
  message,
  onRetry,
}: {
  title?: string;
  message: string;
  onRetry?: () => void;
}) {
  return (
    <section className="surface-card border-status-red/20 px-5 py-5">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div className="flex items-start gap-3">
          <span className="mt-0.5 inline-flex size-8 items-center justify-center rounded-full bg-status-red/10 text-status-red">
            <AlertTriangle className="size-4" />
          </span>
          <div className="space-y-1">
            <p className="text-sm font-semibold text-moon-800">{title}</p>
            <p className="text-sm text-moon-500">{message}</p>
          </div>
        </div>
        {onRetry ? (
          <Button variant="outline" onClick={onRetry}>
            重试
          </Button>
        ) : null}
      </div>
    </section>
  );
}
