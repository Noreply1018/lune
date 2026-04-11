import { isAuthenticated } from "./lib/auth";
import Feedback from "./components/Feedback";
import Shell from "./components/Shell";
import LoginPage from "./pages/LoginPage";
import DashboardPage from "./pages/DashboardPage";
import ChannelsPage from "./pages/ChannelsPage";
import UsagePage from "./pages/UsagePage";
import TokensPage from "./pages/TokensPage";

function PageRouter() {
  const path = window.location.pathname.replace(/\/$/, "");
  switch (path) {
    case "/admin/channels":
      return <ChannelsPage />;
    case "/admin/usage":
      return <UsagePage />;
    case "/admin/tokens":
      return <TokensPage />;
    default:
      return <DashboardPage />;
  }
}

export default function App() {
  if (!isAuthenticated()) {
    return (
      <>
        <Feedback />
        <LoginPage />
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
