import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { SidebarProvider } from "@/components/ui/sidebar";
import { RouterProvider, usePathname } from "@/lib/router";
import Shell from "./components/Shell";
import OverviewPage from "./pages/OverviewPage";
import AccountsPage from "./pages/AccountsPage";
import PoolsPage from "./pages/PoolsPage";
import RoutesPage from "./pages/RoutesPage";
import TokensPage from "./pages/TokensPage";
import UsagePage from "./pages/UsagePage";
import CpaServicePage from "./pages/CpaServicePage";
import PlaygroundPage from "./pages/PlaygroundPage";

function PageRouter() {
  const path = usePathname();
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
