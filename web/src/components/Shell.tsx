import type { ReactNode } from "react";
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
import {
  LayoutDashboard,
  Users,
  Layers,
  Route,
  Key,
  BarChart3,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { Separator } from "@/components/ui/separator";

const navGroups = [
  {
    label: "Observe",
    items: [
      { label: "Overview", href: "/admin", icon: LayoutDashboard },
      { label: "Usage", href: "/admin/usage", icon: BarChart3 },
    ],
  },
  {
    label: "Configure",
    items: [
      { label: "Accounts", href: "/admin/accounts", icon: Users },
      { label: "Pools", href: "/admin/pools", icon: Layers },
      { label: "Routes", href: "/admin/routes", icon: Route },
      { label: "Tokens", href: "/admin/tokens", icon: Key },
    ],
  },
];

function CrescentIcon({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="currentColor"
      className={className}
    >
      <path d="M12 2C6.477 2 2 6.477 2 12s4.477 10 10 10c1.292 0 2.528-.245 3.665-.69A8.5 8.5 0 0 1 9.5 12a8.5 8.5 0 0 1 6.165-8.18A9.963 9.963 0 0 0 12 2Z" />
    </svg>
  );
}

export default function Shell({ children }: { children: ReactNode }) {
  const path = window.location.pathname.replace(/\/$/, "") || "/admin";

  return (
    <>
      <Sidebar
        collapsible="icon"
        className="border-r border-moon-200/60 bg-[linear-gradient(180deg,rgba(240,242,248,0.98),rgba(248,249,252,0.96))]"
      >
        <SidebarHeader className="px-3 pb-3 pt-4">
          <a
            href="/admin"
            className="group/brand flex items-center gap-3 overflow-hidden rounded-[1.35rem] border border-white/65 bg-[linear-gradient(180deg,rgba(255,255,255,0.88),rgba(240,242,248,0.82))] px-3 py-3 text-sidebar-foreground shadow-[0_18px_40px_-34px_rgba(36,43,74,0.55)] transition-all duration-200 hover:border-lunar-500/30 hover:shadow-[0_18px_45px_-32px_rgba(36,43,74,0.62)] group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:rounded-[1rem] group-data-[collapsible=icon]:px-0"
          >
            <span className="relative flex size-10 shrink-0 items-center justify-center rounded-full border border-lunar-500/25 bg-[radial-gradient(circle_at_35%_30%,rgba(255,255,255,0.96),rgba(226,230,240,0.88)_55%,rgba(201,207,223,0.72))] text-lunar-600 shadow-[inset_0_1px_0_rgba(255,255,255,0.8),0_10px_25px_-18px_rgba(36,43,74,0.8)] transition-transform duration-200 group-hover/brand:scale-[1.03]">
              <span className="absolute inset-[4px] rounded-full border border-white/60" />
              <CrescentIcon className="relative size-[18px]" />
            </span>
            <span className="min-w-0 space-y-0.5 transition-all duration-200 group-data-[collapsible=icon]:hidden">
              <span
                className="block text-lg font-semibold tracking-[0.05em] text-moon-800"
                style={{
                  fontFamily:
                    '"Iowan Old Style","Palatino Linotype","Noto Serif SC",Georgia,serif',
                }}
              >
                Lune
              </span>
              <span className="block text-[10px] font-medium uppercase tracking-[0.24em] text-moon-400">
                Moonlight Admin
              </span>
            </span>
          </a>
        </SidebarHeader>

        <Separator className="mx-3 mb-2 w-auto bg-moon-200/70" />

        <SidebarContent className="gap-3 px-2 pb-3 pt-1">
          {navGroups.map((group) => (
            <SidebarGroup key={group.label} className="gap-1.5 px-0 py-0">
              <SidebarGroupLabel className="px-3 text-[10px] font-semibold uppercase tracking-[0.24em] text-moon-400/90">
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
                          render={<a href={item.href} />}
                          className={cn(
                            "relative h-10 rounded-[1rem] px-3 text-[13px] font-medium tracking-[0.01em] transition-all duration-200 before:absolute before:inset-x-2 before:top-0 before:h-px before:rounded-full before:bg-white/70 before:opacity-0 before:transition-opacity before:duration-200 group-data-[collapsible=icon]:size-10! group-data-[collapsible=icon]:rounded-[0.95rem] group-data-[collapsible=icon]:px-0!",
                            active
                              ? "bg-[linear-gradient(180deg,rgba(124,134,184,0.18),rgba(124,134,184,0.08))] text-moon-800 shadow-[inset_0_1px_0_rgba(255,255,255,0.75),0_12px_30px_-26px_rgba(36,43,74,0.55)] before:opacity-100"
                              : "text-moon-500 hover:bg-white/65 hover:text-moon-700 hover:shadow-[inset_0_1px_0_rgba(255,255,255,0.7)]",
                          )}
                        >
                          <span
                            className={cn(
                              "flex size-7 shrink-0 items-center justify-center rounded-full transition-all duration-200 group-data-[collapsible=icon]:size-8",
                              active
                                ? "bg-white/65 text-lunar-600 shadow-[0_10px_22px_-18px_rgba(36,43,74,0.5)]"
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
          <div className="rounded-[1rem] border border-white/60 bg-white/55 px-3 py-2.5 shadow-[inset_0_1px_0_rgba(255,255,255,0.68)] group-data-[collapsible=icon]:hidden">
            <span className="block text-[10px] font-medium uppercase tracking-[0.22em] text-moon-400">
              Moonlight Admin
            </span>
            <span className="mt-1 block text-xs text-moon-500">
              v1.0
            </span>
          </div>
        </SidebarFooter>
      </Sidebar>

      <SidebarInset>
        <main className="min-h-screen flex-1 overflow-y-auto px-5 py-5 sm:px-6 sm:py-6 lg:px-8 lg:py-8 2xl:px-10">
          {children}
        </main>
      </SidebarInset>
    </>
  );
}
