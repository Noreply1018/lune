import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { SidebarProvider } from "@/components/ui/sidebar";
import Shell from "./components/Shell";
import DashboardPage from "./pages/DashboardPage";
import AccountsPage from "./pages/AccountsPage";
import PoolsPage from "./pages/PoolsPage";
import TokensPage from "./pages/TokensPage";
import UsagePage from "./pages/UsagePage";

function PageRouter() {
  const path = window.location.pathname.replace(/\/$/, "");
  switch (path) {
    case "/admin/accounts":
      return <AccountsPage />;
    case "/admin/pools":
      return <PoolsPage />;
    case "/admin/tokens":
      return <TokensPage />;
    case "/admin/usage":
      return <UsagePage />;
    default:
      return <DashboardPage />;
  }
}

export default function App() {
  return (
    <TooltipProvider>
      <Toaster richColors position="top-right" />
      <SidebarProvider>
        <Shell>
          <PageRouter />
        </Shell>
      </SidebarProvider>
    </TooltipProvider>
  );
}
