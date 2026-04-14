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
      <div className="space-y-2">
        <h2 className="text-[1.05rem] font-semibold tracking-[-0.02em] text-moon-800 sm:text-[1.12rem]">
          {title}
        </h2>
        {description && (
          <div className="text-sm leading-6 text-moon-500">
            {description}
          </div>
        )}
      </div>
      {action && <div className="shrink-0">{action}</div>}
    </div>
  );
}
