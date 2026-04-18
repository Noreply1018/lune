import { useEffect, useRef, useState } from "react";

import { cn } from "@/lib/utils";

export type TOCSection = {
  id: string;
  label: string;
};

type SideTOCProps = {
  sections: TOCSection[];
  // Gate IntersectionObserver mounting behind a "ready" flag so the consumer
  // can delay scrollspy until target sections are actually rendered (post
  // skeleton / data fetch). Observers mounted against a skeleton-only DOM
  // observe nothing and never recover after the real sections appear.
  ready?: boolean;
};

export default function SideTOC({ sections, ready = true }: SideTOCProps) {
  const [active, setActive] = useState<string>(sections[0]?.id ?? "");
  // Mount-once guard. Re-running the effect would disconnect + re-observe
  // during every re-render from the parent, which flickers the active dot.
  // The ref also suppresses React StrictMode's double-mount in dev.
  const mountedRef = useRef(false);

  useEffect(() => {
    if (!ready || mountedRef.current) return;
    mountedRef.current = true;
    const observer = new IntersectionObserver(
      (entries) => {
        const visible = entries
          .filter((entry) => entry.isIntersecting)
          .sort((a, b) => b.intersectionRatio - a.intersectionRatio);
        if (visible[0]) {
          setActive(visible[0].target.id);
        }
      },
      { rootMargin: "-20% 0px -55% 0px", threshold: [0, 0.3, 0.7, 1] },
    );
    sections.forEach((section) => {
      const el = document.getElementById(section.id);
      if (el) observer.observe(el);
    });
    return () => {
      observer.disconnect();
      mountedRef.current = false;
    };
  }, [ready, sections]);

  return (
    <nav
      aria-label="Section navigation"
      className="fixed right-6 top-1/2 z-20 hidden -translate-y-1/2 flex-col gap-3 xl:flex"
    >
      {sections.map((section) => {
        const isActive = section.id === active;
        return (
          <a
            key={section.id}
            href={`#${section.id}`}
            title={section.label}
            className="group relative flex items-center justify-end"
            onClick={(event) => {
              event.preventDefault();
              const el = document.getElementById(section.id);
              if (el) {
                el.scrollIntoView({ behavior: "smooth", block: "start" });
                window.history.replaceState(null, "", `#${section.id}`);
              }
            }}
          >
            <span
              className={cn(
                "absolute right-6 whitespace-nowrap rounded-full border border-moon-200/70 bg-white/92 px-2 py-0.5 text-[11px] text-moon-500 opacity-0 shadow-sm transition-opacity duration-150 group-hover:opacity-100 group-focus-within:opacity-100",
              )}
            >
              {section.label}
            </span>
            <span
              className={cn(
                "size-2.5 rounded-full border transition-all duration-200",
                isActive
                  ? "border-lunar-500 bg-lunar-500 shadow-[0_0_0_4px_rgba(134,125,193,0.18)]"
                  : "border-moon-300 bg-white/80 group-hover:border-lunar-400",
              )}
            />
          </a>
        );
      })}
    </nav>
  );
}
