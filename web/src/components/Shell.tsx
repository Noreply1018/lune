import type { ReactNode } from "react";
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
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
import { Separator } from "@/components/ui/separator";

const navItems = [
  { label: "Overview", href: "/admin", icon: LayoutDashboard },
  { label: "Accounts", href: "/admin/accounts", icon: Users },
  { label: "Pools", href: "/admin/pools", icon: Layers },
  { label: "Routes", href: "/admin/routes", icon: Route },
  { label: "Tokens", href: "/admin/tokens", icon: Key },
  { label: "Usage", href: "/admin/usage", icon: BarChart3 },
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
      <Sidebar collapsible="icon">
        <SidebarHeader className="px-4 py-5">
          <a
            href="/admin"
            className="flex items-center gap-2.5 text-sidebar-foreground"
          >
            <CrescentIcon className="size-5 shrink-0 text-lunar-500" />
            <span
              className="text-lg font-semibold tracking-wide group-data-[collapsible=icon]:hidden"
              style={{
                fontFamily:
                  '"Iowan Old Style","Palatino Linotype","Noto Serif SC",Georgia,serif',
              }}
            >
              Lune
            </span>
          </a>
        </SidebarHeader>

        <Separator className="mx-3 mb-2 w-auto bg-moon-200" />

        <SidebarContent>
          <SidebarGroup>
            <SidebarGroupContent>
              <SidebarMenu>
                {navItems.map((item) => {
                  const active = path === item.href;
                  return (
                    <SidebarMenuItem key={item.href}>
                      <SidebarMenuButton
                        isActive={active}
                        tooltip={item.label}
                        render={<a href={item.href} />}
                        className={
                          active
                            ? "border-l-2 border-lunar-500 bg-lunar-500/10 font-medium text-moon-800"
                            : "text-moon-500 hover:bg-moon-200/50 hover:text-moon-700"
                        }
                      >
                        <item.icon
                          className={
                            active ? "text-lunar-600" : "text-moon-400"
                          }
                        />
                        <span>{item.label}</span>
                      </SidebarMenuButton>
                    </SidebarMenuItem>
                  );
                })}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>

        <SidebarFooter className="px-4 pb-4">
          <span className="text-[10px] text-moon-400 group-data-[collapsible=icon]:hidden">
            v1.0
          </span>
        </SidebarFooter>
      </Sidebar>

      <SidebarInset>
        <main className="mx-auto max-w-6xl flex-1 overflow-y-auto px-8 py-8 lg:px-10">
          {children}
        </main>
      </SidebarInset>
    </>
  );
}
