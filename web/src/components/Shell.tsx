import { useEffect, useRef, type MouseEvent, type ReactNode } from "react";
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarInset,
  SidebarFooter,
} from "@/components/ui/sidebar";
import { cn } from "@/lib/utils";
import { usePathname, useRouter } from "@/lib/router";
import { Separator } from "@/components/ui/separator";
import { ADMIN_NAV_GROUPS } from "@/copy/admin";

const FOOTER_QUOTES = [
  "月色落在路由之上",
  "留一点空白给夜色",
  "光不喧哗，流量自明",
  "让复杂藏在月背之后",
  "此处宜静观，不宜喧说",
  "看见波动，也保留平静",
];

function getFooterQuote(): string {
  const idx = new Date().getDate() % FOOTER_QUOTES.length;
  return FOOTER_QUOTES[idx];
}

function CrescentIcon({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" fill="none" className={className}>
      <defs>
        <linearGradient id="lune-moon-fill" x1="6.2" x2="18.9" y1="4.1" y2="19.8">
          <stop offset="0%" stopColor="#ffffff" stopOpacity="1" />
          <stop offset="26%" stopColor="#f5f2fe" stopOpacity="1" />
          <stop offset="100%" stopColor="#7369b0" stopOpacity="0.99" />
        </linearGradient>
        <linearGradient id="lune-moon-edge" x1="7.7" x2="18.6" y1="4.5" y2="19.2">
          <stop offset="0%" stopColor="#ffffff" stopOpacity="0.98" />
          <stop offset="56%" stopColor="#ddd6f6" stopOpacity="0.96" />
          <stop offset="100%" stopColor="#8d82c8" stopOpacity="0.86" />
        </linearGradient>
        <radialGradient id="lune-moon-glow" cx="0" cy="0" r="1" gradientTransform="translate(9.9 8) rotate(34) scale(9.8 11.2)">
          <stop offset="0%" stopColor="#ffffff" stopOpacity="0.94" />
          <stop offset="100%" stopColor="#ffffff" stopOpacity="0" />
        </radialGradient>
        <filter id="lune-moon-shadow" x="0" y="0" width="24" height="24" colorInterpolationFilters="sRGB">
          <feDropShadow dx="0" dy="1.85" stdDeviation="1.18" floodColor="#746bb0" floodOpacity="0.3" />
        </filter>
      </defs>
      <path
        d="M14.78 2.82a9.92 9.92 0 1 0 6.18 14.7 8.06 8.06 0 0 1-6.18-14.7Z"
        fill="url(#lune-moon-fill)"
        filter="url(#lune-moon-shadow)"
      />
      <path
        d="M14.78 2.82a9.92 9.92 0 1 0 6.18 14.7 8.06 8.06 0 0 1-6.18-14.7Z"
        fill="url(#lune-moon-glow)"
      />
      <path
        d="M15.1 3.98a9 9 0 0 1-5.38 15.05"
        stroke="url(#lune-moon-edge)"
        strokeWidth="1.72"
        strokeLinecap="round"
      />
      <path
        d="M13.24 4.98c1.34-.62 2.66-.94 4-.95"
        stroke="#ffffff"
        strokeOpacity="0.9"
        strokeWidth="1.12"
        strokeLinecap="round"
      />
    </svg>
  );
}

export default function Shell({ children }: { children: ReactNode }) {
  const path = usePathname();
  const { onLinkClick } = useRouter();
  const mainRef = useRef<HTMLElement | null>(null);

  useEffect(() => {
    mainRef.current?.scrollTo({ top: 0, behavior: "instant" as ScrollBehavior });
  }, [path]);

  function handleNavClick(event: MouseEvent<HTMLAnchorElement>, href: string) {
    onLinkClick(event, href);
  }

  return (
    <>
      <Sidebar
        collapsible="icon"
        className="border-r border-moon-200/60 bg-[linear-gradient(180deg,rgba(247,246,243,0.92),rgba(243,241,236,0.82))]"
      >
        <SidebarHeader className="px-3 pb-4 pt-4">
          <a
            href="/admin"
            onClick={(event) => handleNavClick(event, "/admin")}
            className="group/brand relative flex items-center gap-3 overflow-hidden rounded-[1.55rem] border border-white/78 bg-[linear-gradient(180deg,rgba(255,255,255,0.9),rgba(244,241,250,0.7))] px-3 py-3.5 text-sidebar-foreground shadow-[0_24px_55px_-42px_rgba(33,40,63,0.24)] transition-all duration-300 hover:border-lunar-300/80 hover:shadow-[0_26px_60px_-38px_rgba(33,40,63,0.28)] group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:rounded-[1.15rem] group-data-[collapsible=icon]:px-0"
          >
            <span className="absolute inset-x-4 top-0 h-px moon-divider" />
            <span className="relative flex size-10 shrink-0 items-center justify-center rounded-full border border-white/92 bg-[radial-gradient(circle_at_34%_28%,rgba(255,255,255,1),rgba(248,244,253,0.99)_36%,rgba(225,219,241,0.96)_68%,rgba(191,183,221,0.92))] shadow-[inset_0_1px_0_rgba(255,255,255,1),inset_0_-12px_20px_rgba(114,102,174,0.14),0_18px_34px_-18px_rgba(61,68,105,0.42)] before:absolute before:inset-[2px] before:rounded-full before:border before:border-white/58 before:content-[''] after:absolute after:inset-[6px] after:rounded-full after:bg-[radial-gradient(circle_at_30%_26%,rgba(255,255,255,0.42),rgba(255,255,255,0)_72%)] after:content-['']">
              <CrescentIcon className="relative z-[1] size-[19px] drop-shadow-[0_1px_1px_rgba(255,255,255,0.24)]" />
            </span>
            <span className="min-w-0 space-y-0 transition-all duration-200 group-data-[collapsible=icon]:hidden">
              <span
                className="block font-editorial text-[1.42rem] font-semibold tracking-[0.015em] text-moon-800"
                style={{
                  fontFamily:
                    '"Iowan Old Style","Palatino Linotype","Noto Serif SC",Georgia,serif',
                }}
              >
                Lune
              </span>
              <span className="block pt-0.5 text-[9px] tracking-[0.2em] text-moon-400/76">
                moonlit gateway
              </span>
            </span>
          </a>
        </SidebarHeader>

        <Separator className="mx-3 mb-2 w-auto bg-moon-200/70" />

        <SidebarContent className="gap-4 px-2 pb-3 pt-1">
          {ADMIN_NAV_GROUPS.map((group) => (
            <SidebarGroup key={group.label} className="gap-1.5 px-0 py-0">
              <SidebarGroupLabel className="px-3 text-[10px] font-semibold tracking-[0.22em] text-moon-400/90">
                {group.label}
              </SidebarGroupLabel>
              <SidebarGroupContent>
                <SidebarMenu className="gap-1">
                  {group.items.map((item) => {
                    const active = path === item.href;
                    return (
                      <SidebarMenuItem key={item.href}>
                        <SidebarMenuButton
                          isActive={active}
                          tooltip={item.label}
                          render={
                            <a
                              href={item.href}
                              onClick={(event) => handleNavClick(event, item.href)}
                            />
                          }
                          className={cn(
                            "relative h-11 rounded-[1rem] px-3 text-[13px] font-medium transition-all duration-200 group-data-[collapsible=icon]:size-10! group-data-[collapsible=icon]:rounded-[0.95rem] group-data-[collapsible=icon]:px-0!",
                            active
                              ? "bg-[linear-gradient(180deg,rgba(134,125,193,0.16),rgba(134,125,193,0.07))] text-moon-800 shadow-[inset_0_1px_0_rgba(255,255,255,0.82)]"
                              : "text-moon-500 hover:bg-white/68 hover:text-moon-700",
                          )}
                        >
                          <span
                            className={cn(
                              "flex size-7 shrink-0 items-center justify-center rounded-full transition-all duration-200 group-data-[collapsible=icon]:size-8",
                              active
                                ? "bg-white/80 text-lunar-600 shadow-[0_12px_24px_-18px_rgba(61,68,105,0.36)]"
                                : "text-moon-400 group-hover/menu-button:text-moon-600",
                            )}
                          >
                            <item.icon className="size-4" />
                          </span>
                          <span>{item.label}</span>
                        </SidebarMenuButton>
                      </SidebarMenuItem>
                    );
                  })}
                </SidebarMenu>
              </SidebarGroupContent>
            </SidebarGroup>
          ))}
        </SidebarContent>

        <SidebarFooter className="px-3 pb-4 pt-2">
          <div className="rounded-[1.15rem] border border-white/70 bg-white/56 px-3 py-3 shadow-[inset_0_1px_0_rgba(255,255,255,0.82)] group-data-[collapsible=icon]:hidden">
            <span className="block text-[10px] tracking-[0.2em] text-moon-400">
              月光注记
            </span>
            <p
              className="mt-2 text-[12px] leading-6 text-moon-500"
              style={{ textWrap: "balance" }}
            >
              {getFooterQuote()}
            </p>
          </div>
        </SidebarFooter>
      </Sidebar>

      <SidebarInset>
        <main
          ref={mainRef}
          className="min-h-screen flex-1 overflow-y-auto px-5 py-6 sm:px-7 sm:py-8 lg:px-10 lg:py-10 2xl:px-12"
        >
          {children}
        </main>
      </SidebarInset>
    </>
  );
}
