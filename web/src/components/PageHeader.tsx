import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

export default function PageHeader({
  eyebrow,
  title,
  description,
  actions,
  meta,
  className,
}: {
  eyebrow?: string;
  title: string;
  description?: ReactNode;
  actions?: ReactNode;
  meta?: ReactNode;
  className?: string;
}) {
  return (
    <header
      className={cn(
        "flex flex-col gap-5 border-b border-moon-200/70 pb-6 lg:flex-row lg:items-end lg:justify-between",
        className,
      )}
    >
      <div className="min-w-0 space-y-2">
        {eyebrow && (
          <p className="text-[11px] font-semibold uppercase tracking-[0.24em] text-lunar-600">
            {eyebrow}
          </p>
        )}
        <div className="space-y-2">
          <h1 className="text-3xl font-semibold tracking-tight text-moon-800 sm:text-[2.2rem]">
            {title}
          </h1>
          {description && (
            <div className="max-w-3xl text-sm leading-6 text-moon-500 sm:text-[15px]">
              {description}
            </div>
          )}
        </div>
        {meta && <div className="pt-1 text-sm text-moon-500">{meta}</div>}
      </div>

      {actions && (
        <div className="flex shrink-0 items-center gap-3 lg:justify-end">
          {actions}
        </div>
      )}
    </header>
  );
}
