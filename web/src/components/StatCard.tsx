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
        "relative overflow-hidden rounded-[1.45rem] border border-white/72 bg-white/72 backdrop-blur-xl",
        hero
          ? "min-h-[210px] px-6 py-6 shadow-[0_30px_80px_-56px_rgba(61,68,105,0.38)]"
          : compact
            ? "px-4 py-4 shadow-[0_14px_32px_-26px_rgba(33,40,63,0.16)]"
            : "px-5 py-5 shadow-[0_18px_40px_-30px_rgba(33,40,63,0.18)]",
        className,
      )}
    >
      <div className="absolute inset-x-0 top-0 h-px moon-divider" />
      <div className="absolute inset-0 bg-[radial-gradient(circle_at_top_right,rgba(134,125,193,0.12),transparent_34%)]" />
      {Icon && (
        <div
          className={cn(
            "absolute rounded-full border border-white/70 bg-white/72 text-lunar-600",
            hero ? "right-5 top-5 p-2.5" : "right-4 top-4 p-2",
          )}
        >
          <Icon className={cn(hero ? "size-5" : "size-4")} />
        </div>
      )}

      <p className="kicker">{label}</p>
      <p
        className={cn(
          "mt-2 text-moon-800",
          hero
            ? "max-w-[12ch] text-4xl font-semibold tracking-[-0.06em]"
            : compact
              ? "text-[1.32rem] font-semibold tracking-[-0.045em]"
              : "text-[1.92rem] font-semibold tracking-[-0.05em]",
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
