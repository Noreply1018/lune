import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Check, Copy } from "lucide-react";
import EmptyState from "@/components/EmptyState";
import ErrorState from "@/components/ErrorState";
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
} from "@/lib/types";
import OrbitCanvas, { type OrbitPool } from "./OverviewPage/OrbitCanvas";
import MoonInscription from "./OverviewPage/MoonInscription";
import AlertConstellation from "./OverviewPage/AlertConstellation";
import PhaseReveal from "./OverviewPage/PhaseReveal";
import { getMoonPhase, getMoonPhaseName } from "./OverviewPage/moonPhase";
import {
  filterVisible,
  fingerprint,
  loadDismissed,
  parseAlert,
  saveDismissed,
} from "./OverviewPage/alertUtils";

const ZOOM_MIN = 0.5;
const ZOOM_MAX = 2.4;
const ZOOM_SENSITIVITY = 0.0012;

function getMoonTone(alerts: OverviewAlert[]): "calm" | "warning" | "critical" {
  if (!alerts?.length) return "calm";
  const hasCritical = alerts.some((a) => {
    const kind = parseAlert(a).kind;
    return kind === "account_error" || kind === "pool_unhealthy";
  });
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
  const [zoomScale, setZoomScale] = useState(1);
  const [phaseRevealAt, setPhaseRevealAt] = useState<number | null>(null);
  const [dismissed, setDismissed] = useState<Set<string>>(() => loadDismissed());
  const canvasRef = useRef<HTMLDivElement>(null);

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
    if (loading || error || !pools.length) return;
    const el = canvasRef.current;
    if (!el) return;
    const handler = (e: WheelEvent) => {
      if (Math.abs(e.deltaY) < 1) return;
      e.preventDefault();
      setZoomScale((current) => {
        const next = current * Math.exp(-e.deltaY * ZOOM_SENSITIVITY);
        return Math.max(ZOOM_MIN, Math.min(ZOOM_MAX, next));
      });
    };
    el.addEventListener("wheel", handler, { passive: false });
    return () => el.removeEventListener("wheel", handler);
  }, [loading, error, pools.length]);

  const orbitPools = useMemo(() => poolsToOrbit(pools, poolDetails), [pools, poolDetails]);
  const alerts = overview?.alerts ?? [];
  const visibleAlerts = useMemo(
    () => filterVisible(alerts, dismissed).map((v) => v.alert),
    [alerts, dismissed],
  );
  const moonTone = useMemo(() => getMoonTone(visibleAlerts), [visibleAlerts]);
  const phaseName = useMemo(() => getMoonPhaseName(getMoonPhase()), []);
  const gatewayBaseUrl = getApiBaseUrl();
  const handlePhaseRevealDismiss = useCallback(() => setPhaseRevealAt(null), []);

  // Prune dismissed entries whose matching alerts no longer exist.
  // Read `dismissed` via ref to avoid looping the effect on its own setState.
  const dismissedRef = useRef(dismissed);
  useEffect(() => {
    dismissedRef.current = dismissed;
  }, [dismissed]);
  useEffect(() => {
    const current = dismissedRef.current;
    if (current.size === 0) return;
    const live = new Set(
      alerts
        .map((a) => ({ alert: a, parsed: parseAlert(a) }))
        .filter(({ parsed }) => parsed.kind === "account_expiring")
        .map(({ alert, parsed }) => fingerprint(alert, parsed)),
    );
    let changed = false;
    const next = new Set<string>();
    current.forEach((fp) => {
      if (live.has(fp)) next.add(fp);
      else changed = true;
    });
    if (changed) {
      setDismissed(next);
      saveDismissed(next);
    }
  }, [alerts]);

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
          zoomScale={zoomScale}
          onAccountClick={(poolId) => navigate(`/admin/pools/${poolId}`)}
          onMoonClick={() => setPhaseRevealAt(Date.now())}
        />
      </div>

      {/* phase name reveal — screen-fixed, independent of zoom.
          `key={phaseRevealAt}` is on the JSX element so React remounts
          PhaseReveal on every click, restarting the CSS animation. */}
      {phaseRevealAt !== null ? (
        <PhaseReveal
          key={phaseRevealAt}
          text={phaseName}
          onDismiss={handlePhaseRevealDismiss}
        />
      ) : null}

      {/* upper-left — MoonInscription (metrics in Chinese numerals, click to expand) */}
      {overview ? (
        <div className="absolute left-6 top-16 z-10">
          <MoonInscription
            requests={overview.requests_today}
            successRate={overview.success_rate_today}
            avgLatency={overview.avg_latency_today}
          />
        </div>
      ) : null}

      {/* upper-right — AlertConstellation (pulsing dot, click to expand) */}
      <div className="absolute right-6 top-16 z-10">
        <AlertConstellation
          alerts={alerts}
          tone={moonTone}
          dismissed={dismissed}
          onDismiss={(fp) => {
            const next = new Set(dismissed);
            next.add(fp);
            setDismissed(next);
            saveDismissed(next);
          }}
          onAlertClick={(alert) =>
            navigate(alert.pool_id ? `/admin/pools/${alert.pool_id}` : "/admin/settings")
          }
        />
      </div>

      <div className="absolute bottom-6 right-6 z-10">
        <GatewayBaseUrlInscription baseUrl={gatewayBaseUrl} />
      </div>

    </div>
  );
}

function GatewayBaseUrlInscription({ baseUrl }: { baseUrl: string }) {
  const [awake, setAwake] = useState(false);
  const [copied, setCopied] = useState(false);
  const awakeTimerRef = useRef<number | null>(null);
  const copyTimerRef = useRef<number | null>(null);

  useEffect(() => {
    return () => {
      if (awakeTimerRef.current) window.clearTimeout(awakeTimerRef.current);
      if (copyTimerRef.current) window.clearTimeout(copyTimerRef.current);
    };
  }, []);

  function soften() {
    if (awakeTimerRef.current) window.clearTimeout(awakeTimerRef.current);
    awakeTimerRef.current = window.setTimeout(() => setAwake(false), 1200);
  }

  async function copyBaseUrl() {
    try {
      await navigator.clipboard.writeText(baseUrl);
      setCopied(true);
      toast("已复制");
      if (copyTimerRef.current) window.clearTimeout(copyTimerRef.current);
      copyTimerRef.current = window.setTimeout(() => setCopied(false), 1600);
    } catch {
      toast("复制失败", "error");
    }
  }

  return (
    <div
      onMouseEnter={() => {
        setAwake(true);
        if (awakeTimerRef.current) {
          window.clearTimeout(awakeTimerRef.current);
          awakeTimerRef.current = null;
        }
      }}
      onMouseLeave={soften}
      onFocus={() => setAwake(true)}
      onBlur={soften}
      className={[
        "pointer-events-auto w-[calc(100vw-3rem)] max-w-[30rem] select-none transition-opacity duration-700",
        awake || copied ? "opacity-100" : "opacity-40",
      ].join(" ")}
    >
      <div className="rounded-[1rem] border border-moon-200/55 bg-white/80 px-3.5 py-3 shadow-[0_18px_40px_-30px_rgba(33,40,63,0.4)] backdrop-blur-md">
        <div className="flex items-center justify-between gap-3">
          <div className="min-w-0 space-y-1">
            <p className="text-[10px] uppercase tracking-[0.28em] text-moon-400">
              Gateway Base URL
            </p>
            <code className="block truncate font-mono text-[12px] text-moon-700" title={baseUrl}>
              {baseUrl}
            </code>
          </div>
          <Button
            variant="ghost"
            size="icon"
            className="size-7 shrink-0 rounded-full text-moon-500"
            onClick={copyBaseUrl}
            aria-label="复制 Gateway Base URL"
            title="复制 Gateway Base URL"
          >
            {copied ? (
              <Check className="size-3.5 text-status-green" />
            ) : (
              <Copy className="size-3.5" />
            )}
          </Button>
        </div>
      </div>
    </div>
  );
}
