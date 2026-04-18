import { useId, type ReactNode } from "react";
import { cn } from "@/lib/utils";

export type TabKey = string;

export type TabItem<K extends TabKey> = {
  key: K;
  label: string;
};

type TabsProps<K extends TabKey> = {
  tabs: TabItem<K>[];
  active: K;
  onChange: (next: K) => void;
  // Accessible name for the tablist (screen readers). Required so switching
  // tabs is announced with context, not just "tab 1 of 2".
  ariaLabel: string;
  // Panels keyed by tab. All panels are kept mounted and toggled via `hidden`
  // so transient state inside a tab (e.g. unsaved subscription body drafts)
  // survives tab switches — conditional rendering would lose it.
  panels: Record<K, ReactNode>;
};

// Minimal tab component. Keyboard: ArrowLeft/ArrowRight/Home/End move focus
// and activation (automatic activation — matches the rest of the app's
// pattern where filter dropdowns commit on change).
export default function Tabs<K extends TabKey>({
  tabs,
  active,
  onChange,
  ariaLabel,
  panels,
}: TabsProps<K>) {
  const baseId = useId();

  function handleKeyDown(event: React.KeyboardEvent<HTMLDivElement>) {
    const idx = tabs.findIndex((t) => t.key === active);
    if (idx < 0) return;
    let nextIdx: number | null = null;
    if (event.key === "ArrowRight") nextIdx = (idx + 1) % tabs.length;
    else if (event.key === "ArrowLeft") nextIdx = (idx - 1 + tabs.length) % tabs.length;
    else if (event.key === "Home") nextIdx = 0;
    else if (event.key === "End") nextIdx = tabs.length - 1;
    if (nextIdx === null) return;
    event.preventDefault();
    const nextKey = tabs[nextIdx]!.key;
    onChange(nextKey);
    const el = document.getElementById(`${baseId}-tab-${nextKey}`);
    el?.focus();
  }

  return (
    <div>
      <div
        role="tablist"
        aria-label={ariaLabel}
        onKeyDown={handleKeyDown}
        className="flex gap-1 border-b border-moon-200/55"
      >
        {tabs.map((tab) => {
          const selected = tab.key === active;
          return (
            <button
              key={tab.key}
              id={`${baseId}-tab-${tab.key}`}
              role="tab"
              type="button"
              aria-selected={selected}
              aria-controls={`${baseId}-panel-${tab.key}`}
              tabIndex={selected ? 0 : -1}
              onClick={() => onChange(tab.key)}
              className={cn(
                "-mb-px cursor-pointer border-b-2 px-4 py-2 text-sm font-medium transition-colors",
                "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-lunar-400/60 focus-visible:ring-offset-2 focus-visible:ring-offset-white rounded-t-md",
                selected
                  ? "border-lunar-500 text-moon-800"
                  : "border-transparent text-moon-400 hover:text-moon-600",
              )}
            >
              {tab.label}
            </button>
          );
        })}
      </div>
      {tabs.map((tab) => {
        const selected = tab.key === active;
        return (
          <div
            key={tab.key}
            id={`${baseId}-panel-${tab.key}`}
            role="tabpanel"
            aria-labelledby={`${baseId}-tab-${tab.key}`}
            hidden={!selected}
            className={selected ? "pt-5" : undefined}
          >
            {panels[tab.key]}
          </div>
        );
      })}
    </div>
  );
}
