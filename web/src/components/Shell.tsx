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
  SidebarTrigger,
} from "@/components/ui/sidebar";
import {
  LayoutDashboard,
  Users,
  Layers,
  BarChart3,
  Key,
  Route,
} from "lucide-react";
import { Separator } from "@/components/ui/separator";

const navItems = [
  { label: "Overview", href: "/admin", icon: LayoutDashboard },
  { label: "Accounts", href: "/admin/accounts", icon: Users },
  { label: "Pools", href: "/admin/pools", icon: Layers },
  { label: "Tokens", href: "/admin/tokens", icon: Key },
  { label: "Usage", href: "/admin/usage", icon: BarChart3 },
  { label: "Routes", href: "/admin/routes", icon: Route },
];

export default function Shell({ children }: { children: ReactNode }) {
  const path = window.location.pathname.replace(/\/$/, "") || "/admin";

  return (
    <>
      <Sidebar collapsible="icon">
        <SidebarHeader className="px-4 py-4">
          <a
            href="/admin"
            className="text-lg font-semibold tracking-wide text-sidebar-foreground"
            style={{
              fontFamily:
                '"Iowan Old Style","Palatino Linotype","Noto Serif SC",Georgia,serif',
            }}
          >
            Lune
          </a>
        </SidebarHeader>

        <SidebarContent>
          <SidebarGroup>
            <SidebarGroupContent>
              <SidebarMenu>
                {navItems.map((item) => (
                  <SidebarMenuItem key={item.href}>
                    <SidebarMenuButton
                      isActive={path === item.href}
                      tooltip={item.label}
                      render={<a href={item.href} />}
                    >
                      <item.icon />
                      <span>{item.label}</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                ))}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>
      </Sidebar>

      <SidebarInset>
        <header className="flex h-12 items-center gap-2 border-b px-4 md:hidden">
          <SidebarTrigger />
          <Separator orientation="vertical" className="h-4" />
          <span className="text-sm font-medium">Lune</span>
        </header>
        <main className="flex-1 overflow-y-auto px-6 py-6 lg:px-8">
          {children}
        </main>
      </SidebarInset>
    </>
  );
}
