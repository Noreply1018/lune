import type { ReactNode } from "react";
import { logout } from "../lib/auth";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
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
  Radio,
  BarChart3,
  Key,
  Settings,
  LogOut,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";

const navItems = [
  { label: "总览", href: "/admin", icon: LayoutDashboard },
  { label: "账号", href: "/admin/accounts", icon: Users },
  { label: "号池", href: "/admin/pools", icon: Layers },
  { label: "渠道", href: "/admin/channels", icon: Radio },
  { label: "用量", href: "/admin/usage", icon: BarChart3 },
  { label: "令牌", href: "/admin/tokens", icon: Key },
  { label: "设置", href: "/admin/settings", icon: Settings },
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

        <SidebarFooter>
          <Button
            variant="ghost"
            size="sm"
            className="w-full justify-start text-muted-foreground hover:text-destructive"
            onClick={logout}
          >
            <LogOut className="size-4" />
            <span>退出登录</span>
          </Button>
        </SidebarFooter>
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
