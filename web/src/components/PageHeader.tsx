import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

export default function PageHeader({
  eyebrow,
  title,
  description,
  actions,
  meta,
  metaEnd,
  ornament,
  className,
}: {
  eyebrow?: string;
  title: string;
  description?: ReactNode;
  actions?: ReactNode;
  meta?: ReactNode;
  metaEnd?: ReactNode;
  ornament?: ReactNode;
  className?: string;
}) {
  return (
    <header
      className={cn(
        "relative flex flex-col gap-5 border-b border-moon-200/70 pb-6 lg:flex-row lg:items-end lg:justify-between",
        className,
      )}
    >
      <div className="absolute inset-x-0 bottom-0 h-px moon-divider" />
      {ornament && (
        <div
          className="pointer-events-none absolute right-[-0.25rem] top-[-0.15rem] hidden h-[7.9rem] w-[24rem] lg:block"
          aria-hidden="true"
        >
          {ornament}
        </div>
      )}
      <div className="min-w-0 space-y-3">
        {eyebrow && (
          <p className="eyebrow-label">
            {eyebrow}
          </p>
        )}
        <div className="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
          <div className="min-w-0 space-y-2">
            <h1 className="font-editorial text-[2.2rem] font-semibold tracking-[-0.055em] text-moon-800 sm:text-[2.8rem]">
              {title}
            </h1>
            {description && (
              <div className="max-w-2xl text-sm leading-7 text-moon-500 sm:text-[15px]">
                {description}
              </div>
            )}
          </div>
          {actions && (
            <div className="flex shrink-0 flex-wrap items-center gap-3 lg:justify-end">
              {actions}
            </div>
          )}
        </div>
        {(meta || metaEnd) && (
          <div className="flex flex-wrap items-center gap-x-4 gap-y-2 text-[13px] text-moon-500">
            {meta}
            {metaEnd && (
              <div className="ml-auto flex flex-wrap items-center gap-2">
                {metaEnd}
              </div>
            )}
          </div>
        )}
      </div>
    </header>
  );
}
