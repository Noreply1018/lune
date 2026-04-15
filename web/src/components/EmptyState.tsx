import type { ReactNode } from "react";

export default function EmptyState({
  eyebrow,
  title,
  description,
  action,
  secondary,
}: {
  eyebrow?: string;
  title: string;
  description: string;
  action?: ReactNode;
  secondary?: ReactNode;
}) {
  return (
    <section className="surface-section hero-glow relative overflow-hidden px-6 py-8 sm:px-8 sm:py-10">
      <div className="absolute inset-y-0 right-0 w-[16rem] bg-[radial-gradient(circle_at_70%_35%,rgba(255,255,255,0.62),rgba(255,255,255,0)_46%),radial-gradient(circle_at_62%_40%,rgba(134,125,193,0.18),rgba(134,125,193,0)_34%)]" />
      <div className="relative max-w-2xl space-y-4">
        {eyebrow ? <p className="eyebrow-label">{eyebrow}</p> : null}
        <h2 className="font-editorial text-[2rem] font-semibold tracking-[-0.05em] text-moon-800 sm:text-[2.6rem]">
          {title}
        </h2>
        <p className="max-w-xl text-sm leading-7 text-moon-500 sm:text-[15px]">
          {description}
        </p>
        <div className="flex flex-wrap gap-3">
          {action}
          {secondary}
        </div>
      </div>
    </section>
  );
}
