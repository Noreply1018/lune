import { lazy, Suspense } from "react";
import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { SidebarProvider } from "@/components/ui/sidebar";
import { RouterProvider, matchPath, usePathname } from "@/lib/router";
import Shell from "./components/Shell";
import AppErrorBoundary from "./components/AppErrorBoundary";
import { AdminUIProvider } from "./components/AdminUI";

const OverviewPage = lazy(() => import("./pages/OverviewPage"));
const PoolDetailPage = lazy(() => import("./pages/PoolDetailPage"));
const SettingsPage = lazy(() => import("./pages/SettingsPage"));
const ActivityPage = lazy(() => import("./pages/ActivityPage"));

function PageSkeleton() {
  return (
    <div className="flex flex-col gap-6 p-6">
      <div className="h-8 w-48 animate-pulse rounded-md bg-muted" />
      <div className="h-64 w-full animate-pulse rounded-md bg-muted" />
    </div>
  );
}

function PageRouter() {
  const path = usePathname();

  const poolMatch = matchPath("/admin/pools/:id", path);

  return (
    <Suspense fallback={<PageSkeleton />}>
      <AppErrorBoundary>
        {poolMatch ? (
          <PoolDetailPage />
        ) : path === "/admin/activity" ? (
          <ActivityPage />
        ) : path === "/admin/settings" ? (
          <SettingsPage />
        ) : (
          <OverviewPage />
        )}
      </AppErrorBoundary>
    </Suspense>
  );
}

export default function App() {
  return (
    <TooltipProvider>
      <Toaster richColors position="top-right" />
      <RouterProvider>
        <AdminUIProvider>
          <SidebarProvider>
            <AppErrorBoundary>
              <Shell>
                <PageRouter />
              </Shell>
            </AppErrorBoundary>
          </SidebarProvider>
        </AdminUIProvider>
      </RouterProvider>
    </TooltipProvider>
  );
}
