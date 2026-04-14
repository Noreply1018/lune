import { lazy, Suspense } from "react";
import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { SidebarProvider } from "@/components/ui/sidebar";
import { RouterProvider, usePathname } from "@/lib/router";
import Shell from "./components/Shell";

const OverviewPage = lazy(() => import("./pages/OverviewPage"));
const AccountsPage = lazy(() => import("./pages/AccountsPage"));
const PoolsPage = lazy(() => import("./pages/PoolsPage"));
const RoutesPage = lazy(() => import("./pages/RoutesPage"));
const TokensPage = lazy(() => import("./pages/TokensPage"));
const UsagePage = lazy(() => import("./pages/UsagePage"));
const CpaServicePage = lazy(() => import("./pages/CpaServicePage"));
const PlaygroundPage = lazy(() => import("./pages/PlaygroundPage"));

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
  return (
    <Suspense fallback={<PageSkeleton />}>
      {(() => {
        switch (path) {
          case "/admin/accounts":
            return <AccountsPage />;
          case "/admin/pools":
            return <PoolsPage />;
          case "/admin/routes":
            return <RoutesPage />;
          case "/admin/tokens":
            return <TokensPage />;
          case "/admin/usage":
            return <UsagePage />;
          case "/admin/cpa-service":
            return <CpaServicePage />;
          case "/admin/playground":
            return <PlaygroundPage />;
          default:
            return <OverviewPage />;
        }
      })()}
    </Suspense>
  );
}

export default function App() {
  return (
    <TooltipProvider>
      <Toaster richColors position="top-right" />
      <RouterProvider>
        <SidebarProvider>
          <Shell>
            <PageRouter />
          </Shell>
        </SidebarProvider>
      </RouterProvider>
    </TooltipProvider>
  );
}
