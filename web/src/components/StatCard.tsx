import type { LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";

export default function StatCard({
  label,
  value,
  sub,
  icon: Icon,
  variant = "default",
  className,
}: {
  label: string;
  value: string;
  sub?: string;
  icon?: LucideIcon;
  variant?: "hero" | "default" | "compact";
  className?: string;
}) {
  const hero = variant === "hero";
  const compact = variant === "compact";

  return (
    <article
      className={cn(
        "relative overflow-hidden rounded-[1.35rem] border border-moon-200/70 bg-white/88 backdrop-blur-sm",
        hero
          ? "min-h-[190px] px-6 py-6 shadow-[0_20px_60px_-40px_rgba(36,43,74,0.35)] sm:px-7 sm:py-7"
          : compact
            ? "px-4 py-4"
            : "px-5 py-5",
        className,
      )}
    >
      <div className="absolute inset-x-0 top-0 h-px bg-gradient-to-r from-transparent via-lunar-500/60 to-transparent" />
      {Icon && (
        <div
          className={cn(
            "absolute rounded-full border border-moon-200/80 bg-moon-50/90 text-moon-400",
            hero ? "right-5 top-5 p-2.5" : "right-4 top-4 p-2",
          )}
        >
          <Icon className={cn(hero ? "size-5" : "size-4")} />
        </div>
      )}

      <p className="text-[11px] font-semibold uppercase tracking-[0.22em] text-moon-400">
        {label}
      </p>
      <p
        className={cn(
          "mt-2 text-moon-800",
          hero
            ? "max-w-[12ch] text-4xl font-semibold tracking-tight sm:text-[3rem]"
            : compact
              ? "text-xl font-semibold"
              : "text-2xl font-semibold",
        )}
        style={{
          fontFamily:
            '"Iowan Old Style","Palatino Linotype","Noto Serif SC",Georgia,serif',
        }}
      >
        {value}
      </p>
      {sub && (
        <p
          className={cn(
            "mt-2 max-w-[32ch] text-moon-500",
            hero ? "text-sm leading-6" : "text-xs leading-5",
          )}
        >
          {sub}
        </p>
      )}
    </article>
  );
}
