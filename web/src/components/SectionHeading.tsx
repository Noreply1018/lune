import type { ReactNode } from "react";

export default function SectionHeading({
  title,
  description,
  action,
}: {
  title: string;
  description?: ReactNode;
  action?: ReactNode;
}) {
  return (
    <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
      <div className="space-y-1">
        <h2 className="text-[11px] font-semibold uppercase tracking-[0.24em] text-moon-400">
          {title}
        </h2>
        {description && (
          <div className="text-sm leading-6 text-moon-500">{description}</div>
        )}
      </div>
      {action && <div className="shrink-0">{action}</div>}
    </div>
  );
}
