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
        "relative flex flex-col gap-6 border-b border-moon-200/70 pb-7 lg:flex-row lg:items-end lg:justify-between",
        className,
      )}
    >
      <div className="absolute inset-x-0 bottom-0 h-px moon-divider" />
      <div className="min-w-0 space-y-3">
        {eyebrow && (
          <p className="eyebrow-label">
            {eyebrow}
          </p>
        )}
        <div className="space-y-2">
          <h1 className="font-editorial text-[2.2rem] font-semibold tracking-[-0.055em] text-moon-800 sm:text-[2.8rem]">
            {title}
          </h1>
          {description && (
            <div className="max-w-3xl text-sm leading-7 text-moon-500 sm:text-[15px]">
              {description}
            </div>
          )}
        </div>
        {meta && (
          <div className="flex flex-wrap gap-x-5 gap-y-2 text-sm text-moon-500">
            {meta}
          </div>
        )}
      </div>

      {actions && (
        <div className="flex shrink-0 flex-wrap items-center gap-3 lg:justify-end">
          {actions}
        </div>
      )}
    </header>
  );
}
