import type { LucideIcon } from "lucide-react";
import {
  BarChart3,
  Key,
  Layers,
  LayoutDashboard,
  MessageSquare,
  Route,
  Server,
  Users,
} from "lucide-react";

export type AdminStatus = "healthy" | "degraded" | "error" | "disabled";

export const STATUS_LABELS_ZH: Record<AdminStatus, string> = {
  healthy: "正常",
  degraded: "降级",
  error: "异常",
  disabled: "已停用",
};

export type NavItem = {
  label: string;
  href: string;
  icon: LucideIcon;
};

export type NavGroup = {
  label: string;
  items: NavItem[];
};

export const ADMIN_NAV_GROUPS: NavGroup[] = [
  {
    label: "观测",
    items: [
      { label: "总览", href: "/admin", icon: LayoutDashboard },
      { label: "用量", href: "/admin/usage", icon: BarChart3 },
      { label: "调试台", href: "/admin/playground", icon: MessageSquare },
    ],
  },
  {
    label: "配置",
    items: [
      { label: "账号", href: "/admin/accounts", icon: Users },
      { label: "池", href: "/admin/pools", icon: Layers },
      { label: "路由", href: "/admin/routes", icon: Route },
      { label: "令牌", href: "/admin/tokens", icon: Key },
      { label: "CPA 服务", href: "/admin/cpa-service", icon: Server },
    ],
  },
];

export function sourceKindLabelZh(kind: "openai_compat" | "cpa"): string {
  return kind === "cpa" ? "CPA" : "OpenAI 兼容";
}
