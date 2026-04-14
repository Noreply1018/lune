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
        <linearGradient id="lune-moon" x1="5" x2="19" y1="4" y2="20">
          <stop offset="0%" stopColor="rgba(255,255,255,0.95)" />
          <stop offset="100%" stopColor="rgba(134,125,193,0.95)" />
        </linearGradient>
      </defs>
      <path
        d="M14.71 2.72a9.67 9.67 0 1 0 6.57 13.86A8.22 8.22 0 0 1 14.71 2.72Z"
        fill="url(#lune-moon)"
      />
      <path
        d="M15.1 4.05a8.47 8.47 0 0 1-4.65 14.44"
        stroke="rgba(255,255,255,0.5)"
        strokeWidth="1.1"
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
            className="group/brand relative flex items-center gap-3 overflow-hidden rounded-[1.55rem] border border-white/75 bg-[linear-gradient(180deg,rgba(255,255,255,0.88),rgba(243,240,250,0.62))] px-3 py-3.5 text-sidebar-foreground shadow-[0_24px_55px_-42px_rgba(33,40,63,0.24)] transition-all duration-300 hover:border-lunar-300/80 hover:shadow-[0_26px_60px_-38px_rgba(33,40,63,0.28)] group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:rounded-[1.15rem] group-data-[collapsible=icon]:px-0"
          >
            <span className="absolute inset-x-4 top-0 h-px moon-divider" />
            <span className="relative flex size-10 shrink-0 items-center justify-center rounded-full border border-white/80 bg-[radial-gradient(circle_at_32%_30%,rgba(255,255,255,0.96),rgba(229,226,245,0.88)_55%,rgba(211,208,232,0.8))] shadow-[inset_0_1px_0_rgba(255,255,255,0.9),0_12px_28px_-18px_rgba(61,68,105,0.38)]">
              <CrescentIcon className="size-[18px]" />
            </span>
            <span className="min-w-0 space-y-1 transition-all duration-200 group-data-[collapsible=icon]:hidden">
              <span
                className="block font-editorial text-[1.42rem] font-semibold tracking-[0.02em] text-moon-800"
                style={{
                  fontFamily:
                    '"Iowan Old Style","Palatino Linotype","Noto Serif SC",Georgia,serif',
                }}
              >
                Lune
              </span>
              <span className="block text-[12px] text-moon-500">
                你的 LLM 网关控制台
              </span>
              <span className="block text-[10px] tracking-[0.14em] text-moon-400">
                moonlight console
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
