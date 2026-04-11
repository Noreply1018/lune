import { useEffect, useState } from "react";
import { isAuthenticated } from "./lib/auth";
import { luneGet } from "./lib/api";
import Feedback from "./components/Feedback";
import Shell from "./components/Shell";
import LoginPage from "./pages/LoginPage";
import DashboardPage from "./pages/DashboardPage";
import ChannelsPage from "./pages/ChannelsPage";
import UsagePage from "./pages/UsagePage";
import TokensPage from "./pages/TokensPage";
import AccountsPage from "./pages/AccountsPage";
import PoolsPage from "./pages/PoolsPage";
import SettingsPage from "./pages/SettingsPage";
import SetupWizard from "./pages/SetupWizard";

function PageRouter() {
  const path = window.location.pathname.replace(/\/$/, "");
  switch (path) {
    case "/admin/channels":
      return <ChannelsPage />;
    case "/admin/usage":
      return <UsagePage />;
    case "/admin/tokens":
      return <TokensPage />;
    case "/admin/accounts":
      return <AccountsPage />;
    case "/admin/pools":
      return <PoolsPage />;
    case "/admin/settings":
      return <SettingsPage />;
    default:
      return <DashboardPage />;
  }
}

export default function App() {
  const [bootstrapNeeded, setBootstrapNeeded] = useState<boolean | null>(null);

  useEffect(() => {
    if (isAuthenticated()) {
      luneGet<{ overview: { needs_bootstrap: boolean } }>("/admin/api/overview")
        .then((d) => setBootstrapNeeded(d.overview.needs_bootstrap))
        .catch(() => setBootstrapNeeded(false));
    }
  }, []);

  if (!isAuthenticated()) {
    return (
      <>
        <Feedback />
        <LoginPage />
      </>
    );
  }

  if (bootstrapNeeded === null) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-paper-50">
        <p className="text-sm text-paper-300">加载中...</p>
      </div>
    );
  }

  if (bootstrapNeeded) {
    return (
      <>
        <Feedback />
        <SetupWizard onComplete={() => setBootstrapNeeded(false)} />
      </>
    );
  }

  return (
    <>
      <Feedback />
      <Shell>
        <PageRouter />
      </Shell>
    </>
  );
}
