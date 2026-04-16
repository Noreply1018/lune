import type { LucideIcon } from "lucide-react";
import { Activity, Cog, LayoutDashboard, Plus, Waves } from "lucide-react";

export type AdminStatus = "unknown" | "healthy" | "degraded" | "error" | "disabled";

export const STATUS_LABELS_ZH: Record<AdminStatus, string> = {
  unknown: "待检查",
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

export const ADMIN_NAV_ITEMS: NavItem[] = [
  { label: "Overview", href: "/admin", icon: LayoutDashboard },
  { label: "Pools", href: "/admin/pools", icon: Waves },
  { label: "Settings", href: "/admin/settings", icon: Cog },
  { label: "Activity", href: "/admin/activity", icon: Activity },
];

export const ADD_ACCOUNT_ACTION = {
  label: "Add Account",
  icon: Plus,
};

export const PROVIDER_POOL_RECOMMENDATION: Record<string, string> = {
  codex: "OpenAI",
  openai: "OpenAI",
  claude: "Claude",
  gemini: "Gemini",
  "gemini-cli": "Gemini",
  vertex: "Gemini",
  aistudio: "Gemini",
};

export const DIRECT_PROVIDER_TEMPLATES = [
  { value: "openai", label: "OpenAI", base_url: "https://api.openai.com/v1" },
  { value: "anthropic", label: "Anthropic", base_url: "https://api.anthropic.com/v1" },
  { value: "google", label: "Google Gemini", base_url: "https://generativelanguage.googleapis.com/v1beta/openai/" },
  { value: "deepseek", label: "DeepSeek", base_url: "https://api.deepseek.com/v1" },
  { value: "openrouter", label: "OpenRouter", base_url: "https://openrouter.ai/api/v1" },
  { value: "custom", label: "自定义", base_url: "" },
];

export const CPA_PROVIDERS = [
  { value: "codex", label: "Codex" },
  { value: "claude", label: "Claude" },
  { value: "gemini", label: "Gemini" },
  { value: "gemini-cli", label: "Gemini CLI" },
  { value: "vertex", label: "Vertex AI" },
  { value: "aistudio", label: "AI Studio" },
];

export function sourceKindLabelZh(kind: "openai_compat" | "cpa"): string {
  return kind === "cpa" ? "CPA" : "OpenAI 兼容";
}
