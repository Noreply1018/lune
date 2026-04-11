import type { ReactNode } from "react";
import { logout } from "../lib/auth";

const navItems = [
  { label: "总览", href: "/admin" },
  { label: "渠道", href: "/admin/channels" },
  { label: "用量", href: "/admin/usage" },
  { label: "令牌", href: "/admin/tokens" },
];

export default function Shell({ children }: { children: ReactNode }) {
  const path = window.location.pathname.replace(/\/$/, "") || "/admin";

  return (
    <div className="flex min-h-screen bg-paper-50">
      {/* ── sidebar ── */}
      <nav className="w-48 shrink-0 border-r border-paper-200 bg-paper-100 flex flex-col">
        <a
          href="/admin"
          className="px-5 py-5 text-lg font-semibold tracking-wide text-paper-700"
          style={{ fontFamily: '"Iowan Old Style","Palatino Linotype","Noto Serif SC",Georgia,serif' }}
        >
          Lune
        </a>

        <ul className="flex-1 space-y-0.5 px-2">
          {navItems.map((item) => {
            const active = path === item.href;
            return (
              <li key={item.href}>
                <a
                  href={item.href}
                  className={`block rounded-md px-3 py-2 text-sm transition-colors ${
                    active
                      ? "bg-paper-200 text-paper-800 font-medium"
                      : "text-paper-500 hover:bg-paper-200/60 hover:text-paper-700"
                  }`}
                >
                  {item.label}
                </a>
              </li>
            );
          })}
        </ul>

        <button
          onClick={logout}
          className="mx-4 mb-5 rounded-md px-3 py-2 text-xs text-paper-500 hover:bg-paper-200/60 hover:text-clay-500 transition-colors text-left"
        >
          退出登录
        </button>
      </nav>

      {/* ── main content ── */}
      <main className="flex-1 overflow-y-auto px-8 py-6">{children}</main>
    </div>
  );
}
