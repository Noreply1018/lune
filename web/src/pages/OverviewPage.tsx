import { useEffect, useMemo, useState } from "react";
import { ArrowRight, KeyRound, QrCode, RefreshCw, Sparkles } from "lucide-react";
import CopyButton from "@/components/CopyButton";
import EmptyState from "@/components/EmptyState";
import EnvSnippetsDialog from "@/components/EnvSnippetsDialog";
import ErrorState from "@/components/ErrorState";
import PageHeader from "@/components/PageHeader";
import QrCodeDialog from "@/components/QrCodeDialog";
import SectionHeading from "@/components/SectionHeading";
import { useAdminUI } from "@/components/AdminUI";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { api } from "@/lib/api";
import { compact, pct } from "@/lib/fmt";
import { getApiBaseUrl, getPoolHealth, maskToken } from "@/lib/lune";
import { useRouter } from "@/lib/router";
import type { Overview, Pool, SystemSettings } from "@/lib/types";

export default function OverviewPage() {
  const { openAddAccount, dataVersion } = useAdminUI();
  const { navigate } = useRouter();
  const [overview, setOverview] = useState<Overview | null>(null);
  const [pools, setPools] = useState<Pool[]>([]);
  const [settings, setSettings] = useState<SystemSettings | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [snippetsOpen, setSnippetsOpen] = useState(false);
  const [qrOpen, setQrOpen] = useState(false);

  function load() {
    setLoading(true);
    setError(null);

    Promise.all([
      api.get<Overview>("/overview"),
      api.get<Pool[]>("/pools"),
      api.get<SystemSettings>("/settings"),
    ])
      .then(([overviewData, poolData, settingsData]) => {
        setOverview(overviewData);
        setPools(poolData ?? []);
        setSettings(settingsData);
      })
      .catch((err) => {
        setError(err instanceof Error ? err.message : "总览加载失败");
      })
      .finally(() => setLoading(false));
  }

  useEffect(() => {
    load();
  }, [dataVersion]);

  const baseUrl = getApiBaseUrl(settings?.external_url);
  const statusLine = useMemo(() => {
    if (!overview) return "";
    const expiring = overview.alerts.filter((item) => item.type === "expiring").length;
    const broken = overview.alerts.filter((item) => item.type === "error").length;
    return `${overview.pools_healthy} 个 Pool 正常运行 · ${expiring} 个账号临近到期 · 今日 ${compact(overview.requests_today)} 次请求${broken ? ` · ${broken} 个异常` : ""}`;
  }, [overview]);

  if (loading) {
    return (
      <div className="space-y-8">
        <Skeleton className="h-40 rounded-[2rem]" />
        <Skeleton className="h-36 rounded-[1.8rem]" />
        <div className="grid gap-5 lg:grid-cols-2 xl:grid-cols-3">
          {Array.from({ length: 3 }).map((_, index) => (
            <Skeleton key={index} className="h-64 rounded-[1.8rem]" />
          ))}
        </div>
      </div>
    );
  }

  if (error) {
    return <ErrorState message={error} onRetry={load} />;
  }

  if (!pools.length) {
    return (
      <EmptyState
        eyebrow="First Run"
        title="开始之前，先接入第一个账号。"
        description="v3 的管理面板以 Pool 为中心。添加账号后，Lune 会自动生成可用的 API 地址与 Token。"
        action={<Button onClick={() => openAddAccount()}><Sparkles className="size-4" />添加账号</Button>}
      />
    );
  }

  return (
    <div className="space-y-8">
      <PageHeader
        eyebrow="Overview"
        title="Pool-first 控制台"
        description="把状态、连接方式和 Pool 工作面收拢到同一个首页。"
        actions={
          <>
            <Button variant="outline" onClick={load}>
              <RefreshCw className="size-4" />
              刷新
            </Button>
            <Button onClick={() => openAddAccount()}>
              <Sparkles className="size-4" />
              Add Account
            </Button>
          </>
        }
        meta={<span>{statusLine}</span>}
      />

      <section className="surface-section hero-glow relative overflow-hidden px-6 py-6 sm:px-8">
        <div className="absolute inset-y-0 right-0 w-[20rem] bg-[radial-gradient(circle_at_70%_35%,rgba(255,255,255,0.78),rgba(255,255,255,0)_48%),radial-gradient(circle_at_62%_40%,rgba(134,125,193,0.18),rgba(134,125,193,0)_34%)]" />
        <div className="relative grid gap-6 lg:grid-cols-[minmax(0,1fr)_19rem]">
          <div className="space-y-4">
            <div className="space-y-2">
              <p className="eyebrow-label">Global Token</p>
              <h2 className="font-editorial text-[2rem] font-semibold tracking-[-0.05em] text-moon-800">
                API 已准备就绪
              </h2>
              <p className="max-w-2xl text-sm leading-7 text-moon-500">
                全局 Token 可访问所有 Pool。用于 SDK、Cursor、CLI 和调试请求。
              </p>
            </div>
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="surface-outline px-4 py-4">
                <p className="text-xs uppercase tracking-[0.18em] text-moon-400">API 地址</p>
                <p className="mt-2 break-all text-sm text-moon-700">{baseUrl}</p>
                <CopyButton value={baseUrl} label="复制" className="mt-3 px-0" />
              </div>
              <div className="surface-outline px-4 py-4">
                <p className="text-xs uppercase tracking-[0.18em] text-moon-400">API Key</p>
                <p className="mt-2 break-all text-sm text-moon-700">
                  {maskToken(overview?.global_token ?? "")}
                </p>
                <CopyButton value={overview?.global_token ?? ""} label="复制" className="mt-3 px-0" />
              </div>
            </div>
            <div className="flex flex-wrap gap-3">
              <Button variant="outline" onClick={() => setSnippetsOpen(true)}>
                <KeyRound className="size-4" />
                Env Snippets
              </Button>
              <Button variant="outline" onClick={() => setQrOpen(true)}>
                <QrCode className="size-4" />
                QR 码
              </Button>
            </div>
          </div>

          <div className="grid gap-3 sm:grid-cols-3 lg:grid-cols-1">
            <div className="surface-outline px-4 py-4">
              <p className="text-xs uppercase tracking-[0.18em] text-moon-400">Pools</p>
              <p className="mt-2 text-2xl font-semibold tracking-[-0.04em] text-moon-800">
                {overview?.pools_total ?? 0}
              </p>
              <p className="mt-1 text-sm text-moon-500">{overview?.pools_healthy ?? 0} 个健康</p>
            </div>
            <div className="surface-outline px-4 py-4">
              <p className="text-xs uppercase tracking-[0.18em] text-moon-400">Requests</p>
              <p className="mt-2 text-2xl font-semibold tracking-[-0.04em] text-moon-800">
                {compact(overview?.requests_today ?? 0)}
              </p>
              <p className="mt-1 text-sm text-moon-500">今日累计</p>
            </div>
            <div className="surface-outline px-4 py-4">
              <p className="text-xs uppercase tracking-[0.18em] text-moon-400">Success Rate</p>
              <p className="mt-2 text-2xl font-semibold tracking-[-0.04em] text-moon-800">
                {pct(overview?.success_rate_today ?? 0)}
              </p>
              <p className="mt-1 text-sm text-moon-500">{overview?.models_total ?? 0} 个模型可用</p>
            </div>
          </div>
        </div>
      </section>

      <section className="space-y-4">
        <SectionHeading
          title="Pools"
          description="每个 Pool 是一条独立工作线。先看健康，再看模型与今日流量。"
        />
        <div className="grid gap-5 lg:grid-cols-2 xl:grid-cols-3">
          {pools.map((pool) => {
            const health = getPoolHealth(pool);
            return (
              <button
                key={pool.id}
                type="button"
                onClick={() => navigate(`/admin/pools/${pool.id}`)}
                className="surface-section text-left transition-transform duration-200 hover:-translate-y-0.5"
              >
                <div className="flex items-start justify-between gap-4 px-5 py-5">
                  <div className="space-y-3">
                    <div className="flex items-center gap-2">
                      <span
                        className={`size-2 rounded-full ${
                          health === "healthy"
                            ? "bg-status-green"
                            : health === "degraded"
                              ? "bg-status-yellow"
                              : health === "error"
                                ? "bg-status-red"
                                : "bg-moon-300"
                        }`}
                      />
                      <p className="eyebrow-label">Pool</p>
                    </div>
                    <h3 className="text-[1.35rem] font-semibold tracking-[-0.03em] text-moon-800">
                      {pool.label}
                    </h3>
                    <p className="text-sm text-moon-500">
                      {pool.account_count} 账号 · {pool.healthy_account_count} 可用
                    </p>
                  </div>
                  <ArrowRight className="mt-1 size-4 text-moon-400" />
                </div>
                <div className="border-t border-moon-200/60 px-5 py-4">
                  <p className="text-xs uppercase tracking-[0.18em] text-moon-400">Models</p>
                  <p className="mt-2 min-h-12 text-sm leading-7 text-moon-600">
                    {pool.models.slice(0, 3).join(", ") || "等待模型发现"}
                    {pool.models.length > 3 ? ` 等 ${pool.models.length} 个` : ""}
                  </p>
                </div>
              </button>
            );
          })}
        </div>
      </section>

      <EnvSnippetsDialog
        open={snippetsOpen}
        onOpenChange={setSnippetsOpen}
        title="Global Env Snippets"
        baseUrl={baseUrl}
        token={overview?.global_token ?? ""}
        model={pools[0]?.models[0]}
      />
      <QrCodeDialog
        open={qrOpen}
        onOpenChange={setQrOpen}
        title="Global Token QR"
        baseUrl={baseUrl}
        token={overview?.global_token ?? ""}
      />
    </div>
  );
}
