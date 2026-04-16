import { useEffect, useMemo, useRef, useState, type MouseEvent, type ReactNode } from "react";
import {
  Activity,
  ChevronDown,
  CirclePlus,
  Cog,
  LayoutDashboard,
  MoonStar,
  Waves,
} from "lucide-react";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarSeparator,
  SidebarTrigger,
} from "@/components/ui/sidebar";
import { usePathname, useRouter } from "@/lib/router";
import { api } from "@/lib/api";
import { cn } from "@/lib/utils";
import type { Pool, PoolDetailResponse } from "@/lib/types";
import { derivePoolSnapshot, type PoolSnapshot } from "@/lib/lune";
import { useAdminUI } from "@/components/AdminUI";
import AddAccountDrawer from "@/components/AddAccountDrawer";

function PoolDot({ snapshot }: { snapshot: PoolSnapshot }) {
  const tone = snapshot.health;
  const className =
    tone === "healthy"
      ? "bg-status-green"
      : tone === "degraded"
        ? "bg-status-yellow"
        : tone === "error"
          ? "bg-status-red"
          : "bg-moon-300";

  return <span className={cn("size-1.5 rounded-full", className)} />;
}

export default function Shell({ children }: { children: ReactNode }) {
  const path = usePathname();
  const { onLinkClick, navigate } = useRouter();
  const { openAddAccount, dataVersion, poolSnapshots, setPoolSnapshots } = useAdminUI();
  const mainRef = useRef<HTMLElement | null>(null);
  const [pools, setPools] = useState<Pool[]>([]);
  const [poolsExpanded, setPoolsExpanded] = useState(true);

  useEffect(() => {
    mainRef.current?.scrollTo({ top: 0, behavior: "instant" as ScrollBehavior });
  }, [path]);

  useEffect(() => {
    let cancelled = false;
    api
      .get<Pool[]>("/pools")
      .then(async (data) => {
        if (!cancelled) {
          const safePools = data ?? [];
          setPools(safePools);
          const detailEntries = await Promise.all(
            safePools.map(async (pool) => {
              try {
                const detail = await api.get<PoolDetailResponse>(`/pools/${pool.id}`);
                return [pool.id, derivePoolSnapshot(pool, detail)] as const;
              } catch {
                return [pool.id, derivePoolSnapshot(pool)] as const;
              }
            }),
          );
          if (!cancelled) {
            setPoolSnapshots(Object.fromEntries(detailEntries));
          }
        }
      })
      .catch(() => {
        if (!cancelled) {
          setPools([]);
          setPoolSnapshots({});
        }
      });

    return () => {
      cancelled = true;
    };
  }, [dataVersion]);

  const currentPoolId = useMemo(() => {
    const match = path.match(/^\/admin\/pools\/(\d+)$/);
    return match ? Number(match[1]) : null;
  }, [path]);

  function handleNavClick(event: MouseEvent<HTMLAnchorElement>, href: string) {
    onLinkClick(event, href);
  }

  return (
    <>
      <Sidebar
        collapsible="icon"
        className="border-r border-moon-200/60 bg-[linear-gradient(180deg,rgba(247,246,243,0.94),rgba(242,240,236,0.86))]"
      >
        <SidebarHeader className="px-3 pb-4 pt-4">
          <a
            href="/admin"
            onClick={(event) => handleNavClick(event, "/admin")}
            className="group/brand relative flex items-center gap-3 overflow-hidden rounded-[1.65rem] border border-white/80 bg-[linear-gradient(180deg,rgba(255,255,255,0.94),rgba(244,241,250,0.72))] px-3 py-3.5 shadow-[0_24px_55px_-42px_rgba(33,40,63,0.26)] transition-all duration-300 hover:border-lunar-300/70 group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:rounded-[1.15rem] group-data-[collapsible=icon]:px-0"
          >
            <span className="absolute inset-x-4 top-0 h-px moon-divider" />
            <span className="relative flex size-10 shrink-0 items-center justify-center rounded-full border border-white/92 bg-[radial-gradient(circle_at_34%_28%,rgba(255,255,255,1),rgba(248,244,253,0.99)_36%,rgba(225,219,241,0.96)_68%,rgba(191,183,221,0.92))] shadow-[inset_0_1px_0_rgba(255,255,255,1),inset_0_-12px_20px_rgba(114,102,174,0.14),0_18px_34px_-18px_rgba(61,68,105,0.42)]">
              <MoonStar className="size-4.5 text-lunar-600" />
            </span>
            <span className="min-w-0 space-y-0 transition-all duration-200 group-data-[collapsible=icon]:hidden">
              <span className="block font-editorial text-[1.46rem] font-semibold tracking-[0.015em] text-moon-800">
                Lune
              </span>
              <span className="block pt-0.5 text-[9px] tracking-[0.2em] text-moon-400/76">
                pool-centered admin
              </span>
            </span>
          </a>
        </SidebarHeader>

        <SidebarSeparator className="mx-3 w-auto bg-moon-200/70" />

        <SidebarContent className="gap-4 px-2 py-4">
          <SidebarMenu className="gap-1">
            <SidebarMenuItem>
              <SidebarMenuButton
                isActive={path === "/admin"}
                render={<a href="/admin" onClick={(event) => handleNavClick(event, "/admin")} />}
                className={cn(
                  "h-11 rounded-[1rem] px-3 text-[13px] font-medium",
                  path === "/admin"
                    ? "bg-[linear-gradient(180deg,rgba(134,125,193,0.16),rgba(134,125,193,0.07))] text-moon-800"
                    : "text-moon-500 hover:bg-white/70 hover:text-moon-700",
                )}
              >
                <LayoutDashboard className="size-4" />
                <span>Overview</span>
              </SidebarMenuButton>
            </SidebarMenuItem>

            <SidebarMenuItem>
              <SidebarMenuButton
                isActive={path.startsWith("/admin/pools/")}
                onClick={() => {
                  setPoolsExpanded((current) => !current);
                  if (!currentPoolId && pools[0]) {
                    navigate(`/admin/pools/${pools[0].id}`);
                  }
                }}
                className={cn(
                  "h-11 rounded-[1rem] px-3 text-[13px] font-medium",
                  path.startsWith("/admin/pools/")
                    ? "bg-[linear-gradient(180deg,rgba(134,125,193,0.16),rgba(134,125,193,0.07))] text-moon-800"
                    : "text-moon-500 hover:bg-white/70 hover:text-moon-700",
                )}
              >
                <Waves className="size-4" />
                <span className="flex-1">Pools</span>
                <ChevronDown
                  className={cn(
                    "size-4 transition-transform",
                    poolsExpanded ? "rotate-180" : "",
                  )}
                />
              </SidebarMenuButton>
            </SidebarMenuItem>

            {poolsExpanded ? (
              <div className="space-y-1 pl-2">
                {pools.map((pool) => {
                  const active = currentPoolId === pool.id;
                  const snapshot = poolSnapshots[pool.id] ?? derivePoolSnapshot(pool);
                  return (
                    <SidebarMenuItem key={pool.id}>
                      <SidebarMenuButton
                        isActive={active}
                        render={
                          <a
                            href={`/admin/pools/${pool.id}`}
                            onClick={(event) => handleNavClick(event, `/admin/pools/${pool.id}`)}
                          />
                        }
                        className={cn(
                          "h-10 rounded-[0.95rem] px-3 text-[13px]",
                          active
                            ? "bg-white/82 text-moon-800 shadow-[0_16px_34px_-28px_rgba(33,40,63,0.28)]"
                            : "text-moon-500 hover:bg-white/58 hover:text-moon-700",
                        )}
                      >
                        <PoolDot snapshot={snapshot} />
                        <span className="truncate">{pool.label}</span>
                      </SidebarMenuButton>
                    </SidebarMenuItem>
                  );
                })}
              </div>
            ) : null}

            <SidebarMenuItem>
              <SidebarMenuButton
                isActive={path === "/admin/settings"}
                render={
                  <a
                    href="/admin/settings"
                    onClick={(event) => handleNavClick(event, "/admin/settings")}
                  />
                }
                className={cn(
                  "h-11 rounded-[1rem] px-3 text-[13px] font-medium",
                  path === "/admin/settings"
                    ? "bg-[linear-gradient(180deg,rgba(134,125,193,0.16),rgba(134,125,193,0.07))] text-moon-800"
                    : "text-moon-500 hover:bg-white/70 hover:text-moon-700",
                )}
              >
                <Cog className="size-4" />
                <span>Settings</span>
              </SidebarMenuButton>
            </SidebarMenuItem>

            <SidebarMenuItem>
              <SidebarMenuButton
                isActive={path === "/admin/activity"}
                render={
                  <a
                    href="/admin/activity"
                    onClick={(event) => handleNavClick(event, "/admin/activity")}
                  />
                }
                className={cn(
                  "h-11 rounded-[1rem] px-3 text-[13px] font-medium",
                  path === "/admin/activity"
                    ? "bg-[linear-gradient(180deg,rgba(134,125,193,0.16),rgba(134,125,193,0.07))] text-moon-800"
                    : "text-moon-500 hover:bg-white/70 hover:text-moon-700",
                )}
              >
                <Activity className="size-4" />
                <span>Activity</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarContent>

        <SidebarFooter className="px-3 pb-4">
          <div className="space-y-2.5">
            <button
              type="button"
              onClick={() => openAddAccount(currentPoolId)}
              className="group flex w-full items-center gap-3 rounded-[1.25rem] border border-white/42 bg-[linear-gradient(180deg,rgba(255,255,255,0.74),rgba(244,241,250,0.5))] px-3 py-3 text-left shadow-[0_12px_24px_-28px_rgba(33,40,63,0.16)] transition-all hover:border-lunar-300/28 hover:shadow-[0_16px_28px_-28px_rgba(33,40,63,0.18)] group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0"
            >
              <span className="inline-flex size-9 shrink-0 items-center justify-center rounded-full bg-lunar-100/34 text-lunar-700/64">
                <CirclePlus className="size-4.5" />
              </span>
              <span className="space-y-0.5 group-data-[collapsible=icon]:hidden">
                <span className="block text-sm font-medium text-moon-700">Add Account</span>
                <span className="block text-xs text-moon-400/64">新账号会直接加入 Pool</span>
              </span>
            </button>
            <p className="px-1 text-[11px] tracking-[0.08em] text-moon-400/46 group-data-[collapsible=icon]:hidden">
              月亮升起以后，工作会自己安静下来。
            </p>
          </div>
        </SidebarFooter>
      </Sidebar>

      <SidebarInset>
        <main
          ref={mainRef}
          className="min-h-screen flex-1 overflow-y-auto px-4 py-4 sm:px-7 sm:py-8 lg:px-10 lg:py-10 2xl:px-12"
        >
          <div className="mb-4 flex items-center gap-3 md:hidden">
            <SidebarTrigger variant="outline" size="icon" className="rounded-full bg-white/75" />
            <span className="text-sm text-moon-400">Lune Admin</span>
          </div>
          {children}
        </main>
      </SidebarInset>

      <AddAccountDrawer />
    </>
  );
}
