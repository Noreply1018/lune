import { useEffect, useRef, useState } from "react";
import PageHeader from "@/components/PageHeader";
import SectionHeading from "@/components/SectionHeading";
import StatCard from "@/components/StatCard";
import CopyButton from "@/components/CopyButton";
import { api } from "@/lib/api";
import { compact, pct } from "@/lib/fmt";
import type { Overview } from "@/lib/types";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import {
  Activity,
  AlertTriangle,
  CheckCircle2,
  Clock,
  Key,
  Layers,
  RefreshCw,
  Users,
  Zap,
} from "lucide-react";

export default function OverviewPage() {
  const [data, setData] = useState<Overview | null>(null);
  const [initialLoading, setInitialLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [refreshError, setRefreshError] = useState<string | null>(null);
  const dataRef = useRef<Overview | null>(null);
  const mountedRef = useRef(true);
  const requestIdRef = useRef(0);

  function load(options?: { background?: boolean }) {
    const background = options?.background ?? false;
    const hasData = dataRef.current !== null;
    const requestId = requestIdRef.current + 1;
    requestIdRef.current = requestId;

    if (background && hasData) {
      setRefreshing(true);
    } else if (!hasData) {
      setInitialLoading(true);
    }

    api
      .get<Overview>("/overview")
      .then((next) => {
        if (!mountedRef.current || requestId !== requestIdRef.current) return;
        dataRef.current = next;
        setData(next);
        setRefreshError(null);
      })
      .catch((err) => {
        if (!mountedRef.current || requestId !== requestIdRef.current) return;
        setRefreshError(err instanceof Error ? err.message : "刷新失败");
      })
      .finally(() => {
        if (!mountedRef.current || requestId !== requestIdRef.current) return;
        if (background && hasData) {
          setRefreshing(false);
        } else if (!hasData) {
          setInitialLoading(false);
        }
      });
  }

  useEffect(() => {
    mountedRef.current = true;
    load();
    const interval = setInterval(() => load({ background: true }), 10_000);
    return () => {
      mountedRef.current = false;
      clearInterval(interval);
    };
  }, []);

  if (initialLoading) {
    return (
      <div className="space-y-8">
        <Skeleton className="h-28 rounded-[1.5rem]" />
        <div className="grid gap-5 sm:grid-cols-2 xl:grid-cols-5">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-32 rounded-[1.45rem]" />
          ))}
        </div>
        <Skeleton className="h-24 rounded-[1.5rem]" />
        <Skeleton className="h-48 rounded-[1.5rem]" />
      </div>
    );
  }

  const overview = data;
  const alerts = overview?.alerts ?? [];

  return (
    <div className="space-y-10">
      <PageHeader
        eyebrow="Overview / Console"
        title="总览"
        description="先看状态，再看流量。"
        meta={
          <>
            <span className="inline-flex items-center gap-2 rounded-full border border-white/70 bg-white/56 px-3 py-1.5">
              池 {overview?.pools_total ?? 0}
            </span>
            <span className="inline-flex items-center gap-2 rounded-full border border-white/70 bg-white/56 px-3 py-1.5">
              账号 {overview?.accounts_total ?? 0}
            </span>
            <span className="inline-flex items-center gap-2 rounded-full border border-white/70 bg-white/56 px-3 py-1.5">
              模型 {overview?.models_total ?? 0}
            </span>
            <span className="inline-flex items-center gap-2 text-moon-400">
              {refreshing ? "数据更新中" : "自动刷新"}
            </span>
            <Button
              size="sm"
              variant="outline"
              onClick={() => load({ background: true })}
              disabled={refreshing}
              className="h-7 rounded-full border-white/72 bg-white/62 px-3.5 text-[12px] shadow-[0_10px_24px_-20px_rgba(33,40,63,0.2)] disabled:opacity-100"
            >
              <RefreshCw className={refreshing ? "size-3.5 animate-spin" : "size-3.5"} />
              {refreshing ? "更新中" : "刷新"}
            </Button>
          </>
        }
        ornament={
          <>
            <div className="absolute inset-0 rounded-[999px] bg-[radial-gradient(circle_at_76%_28%,rgba(255,255,255,0.34),rgba(255,255,255,0)_24%),radial-gradient(circle_at_82%_38%,rgba(134,125,193,0.1),rgba(134,125,193,0)_36%)]" />
            <div className="absolute right-2 top-1 size-[7.5rem] rounded-full border border-moon-300/15" />
            <div className="absolute right-7 top-4 size-[5.4rem] rounded-full border border-lunar-200/20" />
            <div className="absolute right-[4.9rem] top-[1.45rem] size-[2.6rem] rounded-full border border-white/22" />
            <div className="absolute right-[8.75rem] top-[1.2rem] h-[4.6rem] w-[11rem] rounded-[999px] border border-moon-300/8" />
            <div className="absolute right-[3.8rem] top-[2.2rem] h-px w-[10rem] bg-[linear-gradient(90deg,rgba(152,160,183,0),rgba(152,160,183,0.24),rgba(152,160,183,0.02),rgba(152,160,183,0))]" />
            <div className="absolute right-[7.4rem] top-[4.2rem] h-px w-[6.4rem] bg-[linear-gradient(90deg,rgba(152,160,183,0),rgba(152,160,183,0.16),rgba(152,160,183,0))]" />
            <div className="absolute right-[6.8rem] top-[1.6rem] size-[3px] rounded-full bg-white/40 shadow-[0_0_10px_rgba(255,255,255,0.24)]" />
            <div className="absolute right-[10.4rem] top-[3.45rem] size-[3px] rounded-full bg-lunar-200/50 shadow-[0_0_12px_rgba(197,192,236,0.22)]" />
            <div className="absolute right-[2.7rem] top-[5rem] size-[2px] rounded-full bg-moon-300/60" />
            <svg
              viewBox="0 0 320 126"
              className="absolute right-0 top-0 h-full w-full text-moon-300/32"
              fill="none"
            >
              <path
                d="M54 84c28-35 72-53 137-53 39 0 77 8 115 23"
                stroke="currentColor"
                strokeWidth="1"
                strokeLinecap="round"
                strokeDasharray="2 8"
              />
              <path
                d="M122 100c17-18 42-28 74-29 25-1 53 3 85 14"
                stroke="currentColor"
                strokeWidth="1"
                strokeLinecap="round"
              />
              <path
                d="M188 22c32 12 58 32 78 60"
                stroke="currentColor"
                strokeWidth="0.9"
                strokeLinecap="round"
                strokeOpacity="0.75"
              />
              <path
                d="M90 52c26-12 56-18 91-18"
                stroke="currentColor"
                strokeWidth="0.9"
                strokeLinecap="round"
                strokeOpacity="0.6"
              />
              <circle cx="268" cy="56" r="2.2" fill="currentColor" fillOpacity="0.42" />
              <circle cx="221" cy="34" r="1.6" fill="currentColor" fillOpacity="0.32" />
            </svg>
          </>
        }
      />

      {refreshError && (
        <section className="surface-card border-amber-200/70 bg-amber-50/80 px-4 py-4">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex items-start gap-3">
              <AlertTriangle className="mt-0.5 size-4 text-amber-700" />
              <div>
                <p className="text-sm font-medium text-amber-900">总览刷新失败</p>
                <p className="mt-1 text-sm text-amber-800/80">{refreshError}</p>
              </div>
            </div>
            <Button size="sm" variant="outline" onClick={() => load({ background: true })}>
              重试
            </Button>
          </div>
        </section>
      )}

      {/* Stat Cards */}
      <section
        className="grid gap-5 transition-[opacity,filter] duration-300 sm:grid-cols-2 xl:grid-cols-5"
        style={{
          opacity: refreshing ? 0.985 : 1,
          filter: refreshing ? "saturate(0.97)" : "none",
        }}
        aria-busy={refreshing}
      >
        <StatCard
          label="池"
          value={`${overview?.pools_healthy ?? 0} / ${overview?.pools_total ?? 0}`}
          sub="健康 / 总数"
          icon={Layers}
        />
        <StatCard
          label="账号"
          value={`${overview?.accounts_healthy ?? 0} / ${overview?.accounts_total ?? 0}`}
          sub="健康 / 总数"
          icon={Users}
        />
        <StatCard
          label="模型"
          value={compact(overview?.models_total ?? 0)}
          sub="可用模型总数"
          icon={Zap}
        />
        <StatCard
          label="今日请求"
          value={compact(overview?.requests_today ?? 0)}
          sub="当天累计"
          icon={Activity}
        />
        <StatCard
          label="今日成功率"
          value={pct(overview?.success_rate_today ?? 0)}
          sub="请求成功占比"
          icon={CheckCircle2}
        />
      </section>

      {/* Global Token */}
      <section className="space-y-4">
        <SectionHeading
          title="全局令牌"
          description="可用于所有池的默认访问令牌。"
        />
        <div className="surface-card flex items-center justify-between gap-4 px-5 py-4">
          <div className="flex items-center gap-3 overflow-hidden">
            <div className="flex size-9 shrink-0 items-center justify-center rounded-full border border-white/70 bg-white/72">
              <Key className="size-4 text-lunar-600" />
            </div>
            <code className="truncate text-sm text-moon-700">
              {overview?.global_token || "-"}
            </code>
          </div>
          {overview?.global_token && (
            <CopyButton value={overview.global_token} label="复制" />
          )}
        </div>
      </section>

      {/* Alerts */}
      {alerts.length > 0 && (
        <section className="space-y-4">
          <SectionHeading
            title="告警"
            description="需要关注的账号状态变更。"
          />
          <div className="surface-card divide-y divide-moon-200/60 overflow-hidden">
            {alerts.map((alert, index) => (
              <div
                key={index}
                className="flex items-start gap-3 px-5 py-4"
              >
                {alert.type === "error" ? (
                  <AlertTriangle className="mt-0.5 size-4 shrink-0 text-status-red" />
                ) : (
                  <Clock className="mt-0.5 size-4 shrink-0 text-status-yellow" />
                )}
                <div className="min-w-0">
                  <p
                    className={
                      alert.type === "error"
                        ? "text-sm font-medium text-status-red"
                        : "text-sm font-medium text-status-yellow"
                    }
                  >
                    {alert.type === "error" ? "异常" : "即将到期"}
                  </p>
                  <p className="mt-1 text-sm text-moon-600">{alert.message}</p>
                </div>
              </div>
            ))}
          </div>
        </section>
      )}
    </div>
  );
}
