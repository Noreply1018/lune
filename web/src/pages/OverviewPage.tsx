import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import EmptyState from "@/components/EmptyState";
import EnvSnippetsDialog from "@/components/EnvSnippetsDialog";
import ErrorState from "@/components/ErrorState";
import QrCodeDialog from "@/components/QrCodeDialog";
import { useAdminUI } from "@/components/AdminUI";
import { toast } from "@/components/Feedback";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { api } from "@/lib/api";
import { ensureArray, getAccountHealth, getApiBaseUrl } from "@/lib/lune";
import { useRouter } from "@/lib/router";
import type {
  Overview,
  OverviewAlert,
  Pool,
  PoolDetailResponse,
  RevealedAccessToken,
} from "@/lib/types";
import OrbitCanvas, { type OrbitPool } from "./OverviewPage/OrbitCanvas";
import StardustGlobalAccess from "./OverviewPage/StardustGlobalAccess";
import MoonInscription from "./OverviewPage/MoonInscription";

type ZoomLevel = "far" | "mid" | "near";

function getMoonTone(alerts: OverviewAlert[]): "calm" | "warning" | "critical" {
  if (!alerts?.length) return "calm";
  const hasCritical = alerts.some(
    (a) =>
      a.type === "account_error" ||
      a.type === "error" ||
      a.type === "pool_unhealthy",
  );
  if (hasCritical) return "critical";
  return "warning";
}

function poolsToOrbit(pools: Pool[], details: Record<number, PoolDetailResponse>): OrbitPool[] {
  return pools.map((pool) => {
    const detail = details[pool.id];
    const members = ensureArray(detail?.members);
    const accounts = members
      .filter((m) => m.enabled && m.account)
      .map((m) => {
        const acc = m.account!;
        return { id: acc.id, label: acc.label, health: getAccountHealth(acc) };
      });

    const activeHealths = accounts.filter((a) => a.health !== "disabled");
    let poolHealth: OrbitPool["health"] = "unknown";
    if (!pool.enabled) poolHealth = "disabled";
    else if (activeHealths.length === 0) poolHealth = "degraded";
    else if (activeHealths.every((a) => a.health === "healthy")) poolHealth = "healthy";
    else if (activeHealths.every((a) => a.health === "error")) poolHealth = "error";
    else poolHealth = "degraded";

    return {
      id: pool.id,
      label: pool.label,
      enabled: pool.enabled,
      health: poolHealth,
      accounts,
    };
  });
}

export default function OverviewPage() {
  const { openAddAccount, dataVersion } = useAdminUI();
  const { navigate } = useRouter();
  const [overview, setOverview] = useState<Overview | null>(null);
  const [pools, setPools] = useState<Pool[]>([]);
  const [poolDetails, setPoolDetails] = useState<Record<number, PoolDetailResponse>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [snippetsOpen, setSnippetsOpen] = useState(false);
  const [qrOpen, setQrOpen] = useState(false);
  const [revealedGlobalToken, setRevealedGlobalToken] = useState<string | null>(null);
  const [zoomLevel, setZoomLevel] = useState<ZoomLevel>("mid");
  const [alertsOpen, setAlertsOpen] = useState(false);
  const canvasRef = useRef<HTMLDivElement>(null);

  const baseUrl = getApiBaseUrl();
  const hasGlobalToken = Boolean(overview?.global_token_id);

  const load = useCallback(async (signal: { cancelled: boolean }) => {
    setLoading(true);
    setError(null);

    try {
      const [overviewData, poolData] = await Promise.all([
        api.get<Overview>("/overview"),
        api.get<Pool[]>("/pools"),
      ]);
      if (signal.cancelled) return;
      const safePools = ensureArray(poolData);
      setOverview(overviewData);
      setPools(safePools);

      const detailEntries = await Promise.all(
        safePools.map(async (pool) => {
          try {
            const detail = await api.get<PoolDetailResponse>(`/pools/${pool.id}`);
            return [pool.id, detail] as const;
          } catch (err) {
            console.warn(`[overview] pool ${pool.id} detail failed`, err);
            return [pool.id, null] as const;
          }
        }),
      );
      if (signal.cancelled) return;
      const details: Record<number, PoolDetailResponse> = {};
      detailEntries.forEach(([id, detail]) => {
        if (detail) details[id] = detail;
      });
      setPoolDetails(details);
    } catch (err) {
      if (signal.cancelled) return;
      setError(err instanceof Error ? err.message : "总览加载失败");
    } finally {
      if (!signal.cancelled) setLoading(false);
    }
  }, []);

  useEffect(() => {
    const signal = { cancelled: false };
    load(signal);
    return () => {
      signal.cancelled = true;
    };
  }, [dataVersion, load]);

  useEffect(() => {
    setRevealedGlobalToken(null);
  }, [overview?.global_token_id]);

  useEffect(() => {
    if (loading || error || !pools.length) return;
    const el = canvasRef.current;
    if (!el) return;
    const handler = (e: WheelEvent) => {
      if (Math.abs(e.deltaY) < 1) return;
      e.preventDefault();
      setZoomLevel((current) => {
        if (e.deltaY > 0) {
          return current === "near" ? "mid" : current === "mid" ? "far" : "far";
        }
        return current === "far" ? "mid" : current === "mid" ? "near" : "near";
      });
    };
    el.addEventListener("wheel", handler, { passive: false });
    return () => el.removeEventListener("wheel", handler);
  }, [loading, error, pools.length]);

  const firstPoolModel = useMemo(() => {
    for (const pool of pools) {
      if (pool.enabled && pool.models.length > 0) return pool.models[0];
    }
    return undefined;
  }, [pools]);

  const orbitPools = useMemo(() => poolsToOrbit(pools, poolDetails), [pools, poolDetails]);
  const moonTone = useMemo(() => getMoonTone(overview?.alerts ?? []), [overview?.alerts]);
  const alerts = overview?.alerts ?? [];

  async function revealGlobalTokenValue(): Promise<string> {
    if (revealedGlobalToken) return revealedGlobalToken;
    const revealed = await api.post<RevealedAccessToken>("/tokens/global/reveal");
    setRevealedGlobalToken(revealed.token);
    return revealed.token;
  }

  async function copyToClipboard(value: string) {
    await navigator.clipboard.writeText(value);
  }

  async function copyGlobalToken() {
    try {
      const token = await revealGlobalTokenValue();
      await copyToClipboard(token);
      toast("已复制 Token", "success");
    } catch (err) {
      toast(err instanceof Error ? err.message : "复制失败", "error");
    }
  }

  async function copyBaseUrl() {
    try {
      await copyToClipboard(baseUrl);
      toast("已复制 API 地址", "success");
    } catch (err) {
      toast(err instanceof Error ? err.message : "复制失败", "error");
    }
  }

  async function openSnippetsWithToken() {
    try {
      await revealGlobalTokenValue();
      setSnippetsOpen(true);
    } catch (err) {
      toast(err instanceof Error ? err.message : "读取全局 Token 失败", "error");
    }
  }

  async function openQrWithToken() {
    try {
      await revealGlobalTokenValue();
      setQrOpen(true);
    } catch (err) {
      toast(err instanceof Error ? err.message : "读取全局 Token 失败", "error");
    }
  }

  if (loading) {
    return (
      <div className="relative h-[calc(100vh-6rem)] w-full">
        <Skeleton className="absolute inset-0 rounded-[2rem]" />
      </div>
    );
  }

  if (error) {
    return <ErrorState message={error} onRetry={() => load({ cancelled: false })} />;
  }

  if (!pools.length) {
    return (
      <EmptyState
        eyebrow="First Run"
        title="开始之前，先接入第一个账号。"
        description="管理面板以 Pool 为中心。添加账号后，Lune 会自动生成可用的 API 地址与 Token。"
        action={<Button onClick={() => openAddAccount()}>添加账号</Button>}
      />
    );
  }

  return (
    <div className="relative h-[calc(100vh-6rem)] w-full overflow-hidden">
      {/* title — just "Lune" in serif */}
      <div className="pointer-events-none absolute left-1/2 top-4 z-10 -translate-x-1/2 text-center">
        <h1
          className="text-[28px] tracking-[0.22em] text-moon-700"
          style={{
            fontFamily: "'Iowan Old Style','Palatino Linotype','Noto Serif SC','Source Han Serif SC',Georgia,serif",
            fontWeight: 400,
          }}
        >
          Lune
        </h1>
      </div>

      {/* the canvas fills the whole page */}
      <div ref={canvasRef} className="absolute inset-0">
        <OrbitCanvas
          pools={orbitPools}
          moonTone={moonTone}
          zoomLevel={zoomLevel}
          moonFace={zoomLevel === "near" && overview ? (
            <MoonInscription
              requests={overview.requests_today}
              successRate={overview.success_rate_today}
              avgLatency={overview.avg_latency_today}
            />
          ) : null}
          onAccountClick={(poolId) => navigate(`/admin/pools/${poolId}`)}
          moonCursor={alerts.length > 0 ? "pointer" : "default"}
          onMoonClick={alerts.length > 0 ? () => setAlertsOpen((v) => !v) : undefined}
        />
      </div>

      {/* alert reveal panel — only when moon is clicked and alerts exist */}
      {alertsOpen && alerts.length > 0 ? (
        <div className="pointer-events-auto absolute left-1/2 top-1/2 z-20 flex max-h-[52vh] w-[min(28rem,90vw)] -translate-x-1/2 translate-y-[8rem] flex-col rounded-[1.4rem] border border-moon-200/55 bg-white/92 p-4 shadow-[0_30px_70px_-40px_rgba(33,40,63,0.5)] backdrop-blur-md">
          <div className="mb-3 flex items-center justify-between">
            <p className="text-[10px] font-semibold uppercase tracking-[0.28em] text-moon-400">
              提醒
            </p>
            <button
              type="button"
              onClick={() => setAlertsOpen(false)}
              className="text-[11px] text-moon-500 hover:text-moon-800"
            >
              收起
            </button>
          </div>
          <ul className="space-y-2 overflow-y-auto pr-1">
            {alerts.map((alert, i) => (
              <li key={`${alert.type}-${i}`}>
                <button
                  type="button"
                  onClick={() =>
                    navigate(alert.pool_id ? `/admin/pools/${alert.pool_id}` : "/admin/settings")
                  }
                  className="w-full rounded-lg border border-moon-200/45 bg-white/55 px-3 py-2 text-left text-sm text-moon-700 transition-colors hover:bg-white/85"
                >
                  {alert.message}
                </button>
              </li>
            ))}
          </ul>
        </div>
      ) : null}

      {/* zoom hint (only when default) */}
      <div className="pointer-events-none absolute left-5 bottom-5 text-[10px] tracking-[0.28em] text-moon-400/70">
        滚轮 · 缩放 {zoomLevel === "far" ? "远" : zoomLevel === "near" ? "近" : "·"}
      </div>

      {/* stardust global access at bottom right */}
      <div className="absolute bottom-5 right-6 z-10 text-right">
        <StardustGlobalAccess
          baseUrl={baseUrl}
          tokenMasked={overview?.global_token_masked ?? ""}
          hasToken={hasGlobalToken}
          onCopyToken={copyGlobalToken}
          onCopyBaseUrl={copyBaseUrl}
          onOpenSnippets={openSnippetsWithToken}
          onOpenQr={openQrWithToken}
        />
      </div>

      <EnvSnippetsDialog
        open={snippetsOpen}
        onOpenChange={setSnippetsOpen}
        title="Global Env Snippets"
        baseUrl={baseUrl}
        token={revealedGlobalToken ?? ""}
        model={firstPoolModel}
      />
      <QrCodeDialog
        open={qrOpen}
        onOpenChange={setQrOpen}
        title="Global Token QR"
        baseUrl={baseUrl}
        token={revealedGlobalToken ?? ""}
      />
    </div>
  );
}
