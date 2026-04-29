import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  AlertCircle,
  Check,
  CheckCircle2,
  Clock3,
  Copy,
  ExternalLink,
  Loader2,
  ShieldCheck,
  Sparkles,
  X,
} from "lucide-react";
import {
  CPA_PROVIDERS,
  DIRECT_PROVIDER_TEMPLATES,
  PROVIDER_POOL_RECOMMENDATION,
} from "@/copy/admin";
import { toast } from "@/components/Feedback";
import { useAdminUI } from "@/components/AdminUI";
import { useRouter } from "@/lib/router";
import { ApiError, api } from "@/lib/api";
import type { Account, CpaService, LoginSession, Pool } from "@/lib/types";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";

const NEW_POOL_VALUE = "__new_pool__";
const CPA_SESSION_STORAGE_KEY = "lune:add-account:cpa-session";

type SourceKind = "cpa" | "openai_compat" | null;

type Step = 1 | 2 | 3 | 4;

type DirectForm = {
  label: string;
  provider: string;
  base_url: string;
  api_key: string;
  notes: string;
};

type ResultState = {
  poolId: number;
  poolLabel: string;
  label: string;
  source: string;
};

type PendingCpaFlow = {
  poolId: number;
  poolLabel: string;
  providerLabel: string;
};

type StoredCpaSession = PendingCpaFlow & {
  sessionId: string;
  provider: string;
  serviceId: number;
};

const EMPTY_DIRECT_FORM: DirectForm = {
  label: "",
  provider: "",
  base_url: "",
  api_key: "",
  notes: "",
};

const STEP_META: Record<Step, { title: string; description: string }> = {
  1: {
    title: "接入方式",
    description: "先选路径，再继续。",
  },
  2: {
    title: "确认 Pool",
    description: "确认归属位置。",
  },
  3: {
    title: "填写信息",
    description: "完成输入或授权。",
  },
  4: {
    title: "完成",
    description: "账号已经可以使用。",
  },
};

function isActiveLoginSession(session: LoginSession | null | undefined) {
  return Boolean(session && (session.status === "pending" || session.status === "authorized"));
}

function getLoginStatusLabel(status: LoginSession["status"]) {
  switch (status) {
    case "failed":
      return "授权失败";
    case "expired":
      return "授权已过期";
    case "authorized":
      return "授权已确认，正在完成接入";
    case "cancelled":
      return "授权已取消";
    case "succeeded":
      return "账号已接入";
    default:
      return "等待授权";
  }
}

function getLoginStatusHint(status: LoginSession["status"]) {
  switch (status) {
    case "authorized":
      return "浏览器端已经确认，Lune 正在继续收口。";
    case "failed":
      return "这次授权没有完成，可以重新开始。";
    case "expired":
      return "授权窗口已经结束，需要重新获取新的授权码。";
    case "cancelled":
      return "这次授权已经终止，不会继续轮询。";
    case "succeeded":
      return "账号已经创建完成。";
    default:
      return "保持此页打开，或稍后继续回来完成。";
  }
}

function formatCountdown(ms: number) {
  const totalSeconds = Math.max(0, Math.ceil(ms / 1000));
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
}

function formatExpiryTime(expiresAt?: string) {
  if (!expiresAt) return "--:--";
  const date = new Date(expiresAt);
  if (Number.isNaN(date.getTime())) return "--:--";
  return date.toLocaleTimeString("zh-CN", {
    hour: "2-digit",
    minute: "2-digit",
  });
}

function readStoredCpaSession(): StoredCpaSession | null {
  try {
    const raw = window.localStorage.getItem(CPA_SESSION_STORAGE_KEY);
    if (!raw) return null;
    return JSON.parse(raw) as StoredCpaSession;
  } catch {
    return null;
  }
}

function writeStoredCpaSession(session: StoredCpaSession) {
  try {
    window.localStorage.setItem(CPA_SESSION_STORAGE_KEY, JSON.stringify(session));
  } catch {
    // ignore storage failures
  }
}

function clearStoredCpaSession() {
  try {
    window.localStorage.removeItem(CPA_SESSION_STORAGE_KEY);
  } catch {
    // ignore storage failures
  }
}

function InlineCopyAction({
  value,
  idleLabel = "复制",
  copiedLabel = "已复制",
  className,
}: {
  value: string;
  idleLabel?: string;
  copiedLabel?: string;
  className?: string;
}) {
  const [copied, setCopied] = useState(false);

  async function handleCopy() {
    await navigator.clipboard.writeText(value);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1600);
  }

  return (
    <button
      type="button"
      onClick={handleCopy}
      className={cn(
        "inline-flex items-center gap-1.5 text-xs text-moon-400 transition-colors hover:text-moon-700",
        className,
      )}
    >
      {copied ? <Check className="size-3.5 text-status-green" /> : <Copy className="size-3.5" />}
      <span>{copied ? copiedLabel : idleLabel}</span>
    </button>
  );
}

function PathCard({
  active,
  title,
  description,
  onClick,
}: {
  active: boolean;
  title: string;
  description: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "rounded-[1.45rem] px-4 py-4 text-left transition-colors",
        active
          ? "bg-lunar-100/72 text-moon-800"
          : "bg-white/62 text-moon-700 hover:bg-white/82",
      )}
    >
      <div className="flex items-start justify-between gap-3">
        <div>
          <p className="text-sm font-semibold">{title}</p>
          <p className="mt-1 text-sm text-moon-500">{description}</p>
        </div>
        <span
          className={cn(
            "mt-0.5 inline-flex size-5 items-center justify-center rounded-full",
            active ? "bg-white/90 text-lunar-700" : "bg-moon-100/80 text-transparent",
          )}
        >
          <Check className="size-3.5" />
        </span>
      </div>
    </button>
  );
}

function SelectPill({
  active,
  label,
  onClick,
}: {
  active: boolean;
  label: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "rounded-full px-3 py-1.5 text-sm transition-colors",
        active ? "bg-lunar-100/90 text-moon-800" : "text-moon-500 hover:bg-white/70 hover:text-moon-700",
      )}
    >
      {label}
    </button>
  );
}

function PoolRow({
  active,
  title,
  description,
  onClick,
}: {
  active: boolean;
  title: string;
  description?: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "flex w-full items-center justify-between gap-3 border-b border-moon-200/50 py-3 text-left transition-colors last:border-b-0",
        active ? "text-moon-800" : "text-moon-600 hover:text-moon-800",
      )}
    >
      <div>
        <p className="text-sm font-medium">{title}</p>
        {description ? <p className="mt-1 text-sm text-moon-500">{description}</p> : null}
      </div>
      <span
        className={cn(
          "inline-flex size-5 items-center justify-center rounded-full",
          active ? "bg-lunar-100 text-lunar-700" : "bg-moon-100/80 text-transparent",
        )}
      >
        <Check className="size-3.5" />
      </span>
    </button>
  );
}

export default function AddAccountDrawer() {
  const { addAccountOpen, closeAddAccount, preferredPoolId, refreshData } = useAdminUI();
  const { navigate } = useRouter();
  const pollRef = useRef<number | null>(null);

  const [step, setStep] = useState<Step>(1);
  const [sourceKind, setSourceKind] = useState<SourceKind>(null);
  const [provider, setProvider] = useState("");
  const [pools, setPools] = useState<Pool[]>([]);
  const [poolLoadFailed, setPoolLoadFailed] = useState(false);
  const [cpaService, setCpaService] = useState<CpaService | null>(null);
  const [selectedPoolId, setSelectedPoolId] = useState("");
  const [newPoolLabel, setNewPoolLabel] = useState("");
  const [directForm, setDirectForm] = useState<DirectForm>(EMPTY_DIRECT_FORM);
  const [loading, setLoading] = useState(false);
  const [loginSession, setLoginSession] = useState<LoginSession | null>(null);
  const [result, setResult] = useState<ResultState | null>(null);
  const [pendingCpaFlow, setPendingCpaFlow] = useState<PendingCpaFlow | null>(null);
  const [restoredSession, setRestoredSession] = useState(false);
  const [nowTick, setNowTick] = useState(Date.now());

  const currentStepMeta = STEP_META[step];

  const providerOptions = sourceKind === "cpa" ? CPA_PROVIDERS : DIRECT_PROVIDER_TEMPLATES;
  const recommendedPoolLabel = useMemo(
    () => (provider ? PROVIDER_POOL_RECOMMENDATION[provider] ?? "" : ""),
    [provider],
  );

  const refreshAfterAccountCreate = useCallback(
    async (accountId?: number | null) => {
      if (accountId) {
        try {
          await api.post(`/accounts/${accountId}/discover-models`);
        } catch {
          // The account has already been created. Keep the drawer flow moving;
          // the Pool reload below will render the persisted backend state.
        }
      }
      refreshData();
    },
    [refreshData],
  );

  useEffect(() => {
    if (!addAccountOpen) return;

    setLoading(true);
    setPoolLoadFailed(false);
    Promise.allSettled([
      api.get<Pool[]>("/pools"),
      api.get<CpaService | null>("/cpa/service"),
    ])
      .then(async ([poolResult, serviceResult]) => {
        const safePools = poolResult.status === "fulfilled" ? poolResult.value ?? [] : [];
        if (poolResult.status === "fulfilled") {
          setPools(safePools);
          setPoolLoadFailed(false);
        } else {
          setPools([]);
          setPoolLoadFailed(true);
        }

        if (serviceResult.status === "fulfilled") {
          setCpaService(serviceResult.value ?? null);
        } else {
          setCpaService(null);
        }

        if (serviceResult.status === "fulfilled" && serviceResult.value) {
          const stored = readStoredCpaSession();
          const restoreBySession = async (sessionId: string, fallback?: StoredCpaSession) => {
            try {
              const restored = await api.get<LoginSession>(`/accounts/cpa/login-sessions/${sessionId}`);
              const restoredPoolId = restored.pool_id ?? fallback?.poolId ?? 0;
              const restoredProvider = restored.provider ?? fallback?.provider ?? "codex";
              const restoredProviderLabel = getProviderLabel(CPA_PROVIDERS, restoredProvider);
              setSourceKind("cpa");
              setProvider(restoredProvider);
              setSelectedPoolId(String(restoredPoolId));
              setPendingCpaFlow({
                poolId: restoredPoolId,
                poolLabel: safePools.find((pool) => pool.id === restoredPoolId)?.label || fallback?.poolLabel || "未命名 Pool",
                providerLabel: restoredProviderLabel,
              });
              setLoginSession(restored);
              setStep(3);
              setRestoredSession(isActiveLoginSession(restored));
              if (restored.status === "succeeded" && restored.account_id) {
                await refreshAfterAccountCreate(restored.account_id);
                setResult({
                  poolId: restoredPoolId,
                  poolLabel: safePools.find((pool) => pool.id === restoredPoolId)?.label || fallback?.poolLabel || "未命名 Pool",
                  label: restored.account?.label ?? "CPA account",
                  source: `CPA · ${restoredProviderLabel}`,
                });
                setStep(4);
                clearStoredCpaSession();
              }
              if (!isActiveLoginSession(restored)) {
                clearStoredCpaSession();
              }
              if (isActiveLoginSession(restored) && serviceResult.value) {
                writeStoredCpaSession({
                  sessionId: restored.id,
                  provider: restoredProvider,
                  providerLabel: restoredProviderLabel,
                  poolId: restoredPoolId,
                  poolLabel: safePools.find((pool) => pool.id === restoredPoolId)?.label || fallback?.poolLabel || "未命名 Pool",
                  serviceId: serviceResult.value.id,
                });
              }
              return true;
            } catch {
              return false;
            }
          };

          if (stored && stored.serviceId === serviceResult.value.id) {
            const restored = await restoreBySession(stored.sessionId, stored);
            if (!restored) {
              clearStoredCpaSession();
            }
          } else {
            try {
              const active = await api.get<LoginSession>(`/accounts/cpa/login-sessions/active?service_id=${serviceResult.value.id}`);
              await restoreBySession(active.id);
            } catch {
              // no active session to restore
            }
          }
        }
      })
      .finally(() => setLoading(false));
  }, [addAccountOpen, refreshAfterAccountCreate]);

  useEffect(() => {
    if (!addAccountOpen) {
      setStep(1);
      setSourceKind(null);
      setProvider("");
      setDirectForm(EMPTY_DIRECT_FORM);
      setLoginSession(null);
      setResult(null);
      setPendingCpaFlow(null);
      setRestoredSession(false);
      setSelectedPoolId("");
      setNewPoolLabel("");
      setPoolLoadFailed(false);
      if (pollRef.current) {
        window.clearTimeout(pollRef.current);
        pollRef.current = null;
      }
      return;
    }

    const preferred = preferredPoolId ? String(preferredPoolId) : "";
    if (preferred) {
      setSelectedPoolId(preferred);
    }
  }, [addAccountOpen, preferredPoolId]);

  useEffect(() => {
    if (!provider) return;
    if (poolLoadFailed) {
      setSelectedPoolId("");
      setNewPoolLabel("");
      return;
    }
    const recommended = PROVIDER_POOL_RECOMMENDATION[provider] ?? "";
    const preferred = preferredPoolId ? String(preferredPoolId) : "";
    const matched = pools.find((pool) => pool.label === recommended);

    if (preferred) {
      setSelectedPoolId(preferred);
      return;
    }
    setSelectedPoolId(matched ? String(matched.id) : recommended ? NEW_POOL_VALUE : "");
    setNewPoolLabel(recommended);
  }, [pools, preferredPoolId, provider]);

  useEffect(() => {
    if (sourceKind !== "openai_compat" || !provider) return;
    const template = DIRECT_PROVIDER_TEMPLATES.find((item) => item.value === provider);
    if (!template) return;

    setDirectForm((current) => ({
      ...current,
      provider,
      base_url: template.base_url,
      label: current.label || (template.label === "自定义" ? "" : `My ${template.label}`),
    }));
  }, [provider, sourceKind]);

  useEffect(() => {
    if (!isActiveLoginSession(loginSession)) {
      return;
    }
    const currentSession = loginSession as LoginSession;
    let disposed = false;

    if (pollRef.current) {
      window.clearTimeout(pollRef.current);
    }

    const delayMs = Math.max(1000, (currentSession.poll_interval_seconds ?? 3) * 1000);
    const scheduleRetry = () => {
      pollRef.current = window.setTimeout(() => {
        if (disposed) {
          return;
        }
        setLoginSession((session) =>
          session && session.id === currentSession.id ? { ...session } : session,
        );
      }, delayMs);
    };

    pollRef.current = window.setTimeout(async () => {
      try {
        const session = await api.get<LoginSession>(`/accounts/cpa/login-sessions/${currentSession.id}`);
        setLoginSession(session);
        if (isActiveLoginSession(session) && cpaService && pendingCpaFlow) {
          writeStoredCpaSession({
            sessionId: session.id,
            provider,
            providerLabel: pendingCpaFlow.providerLabel,
            poolId: pendingCpaFlow.poolId,
            poolLabel: pendingCpaFlow.poolLabel,
            serviceId: cpaService.id,
          });
        }
        if (session.status === "succeeded" && session.account_id) {
          const poolId = pendingCpaFlow?.poolId ?? preferredPoolId ?? 0;
          if (pollRef.current) {
            window.clearTimeout(pollRef.current);
            pollRef.current = null;
          }
          setRestoredSession(false);
          clearStoredCpaSession();
          await refreshAfterAccountCreate(session.account_id);
          if (disposed) return;
          setResult({
            poolId,
            poolLabel: pendingCpaFlow?.poolLabel || getPoolLabel(poolId),
            label: session.account?.label ?? "CPA account",
            source: `CPA · ${pendingCpaFlow?.providerLabel || getProviderLabel(providerOptions, provider)}`,
          });
          setStep(4);
        }
        if (session.status === "failed" || session.status === "expired" || session.status === "cancelled") {
          setRestoredSession(false);
          clearStoredCpaSession();
          if (pollRef.current) {
            window.clearTimeout(pollRef.current);
            pollRef.current = null;
          }
        }
      } catch {
        if (!disposed && isActiveLoginSession(currentSession)) {
          scheduleRetry();
        } else if (pollRef.current) {
          window.clearTimeout(pollRef.current);
          pollRef.current = null;
        }
      }
    }, delayMs);

    return () => {
      disposed = true;
      if (pollRef.current) {
        window.clearTimeout(pollRef.current);
        pollRef.current = null;
      }
    };
  }, [cpaService, loginSession, pendingCpaFlow, preferredPoolId, provider, providerOptions, refreshAfterAccountCreate]);

  useEffect(() => {
    if (!isActiveLoginSession(loginSession)) return;

    setNowTick(Date.now());
    const timer = window.setInterval(() => setNowTick(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [loginSession]);

  function getPoolLabel(poolId: number) {
    return pools.find((pool) => pool.id === poolId)?.label || newPoolLabel.trim() || "未命名 Pool";
  }

  function getProviderLabel(
    options: Array<{ value: string; label: string; base_url?: string }>,
    value: string,
  ) {
    return options.find((item) => item.value === value)?.label || value;
  }

  function chooseSourceKind(kind: Exclude<SourceKind, null>) {
    setSourceKind(kind);
    setProvider("");
    if (kind === "cpa") {
      setDirectForm(EMPTY_DIRECT_FORM);
      setRestoredSession(false);
    } else {
      setLoginSession(null);
      setPendingCpaFlow(null);
      setRestoredSession(false);
    }
  }

  async function ensurePool(): Promise<number> {
    if (selectedPoolId && selectedPoolId !== NEW_POOL_VALUE) {
      return Number(selectedPoolId);
    }

    if (!newPoolLabel.trim()) {
      throw new Error("请输入 Pool 名称");
    }

    const created = await api.post<Pool>("/pools", {
      label: newPoolLabel.trim(),
      priority: pools.length + 1,
    });
    setPools((current) => [...current, created]);
    setSelectedPoolId(String(created.id));
    return created.id;
  }

  async function submitDirect() {
    setLoading(true);
    try {
      const poolId = await ensurePool();
      const created = await api.post<Account>("/accounts", {
        label: directForm.label.trim(),
        source_kind: "openai_compat",
        provider: directForm.provider,
        base_url: directForm.base_url.trim(),
        api_key: directForm.api_key.trim(),
        notes: directForm.notes.trim(),
        enabled: true,
        pool_id: poolId,
      });
      await refreshAfterAccountCreate(created.id);
      setResult({
        poolId,
        poolLabel: getPoolLabel(poolId),
        label: directForm.label.trim(),
        source: `直连 · ${getProviderLabel(DIRECT_PROVIDER_TEMPLATES, directForm.provider)}`,
      });
      setStep(4);
    } catch (err) {
      toast(err instanceof Error ? err.message : "创建账号失败", "error");
    } finally {
      setLoading(false);
    }
  }

  async function startCpaLogin() {
    if (!cpaService) {
      toast("内置 CPA 服务未就绪，请稍后重试或检查容器日志。", "error");
      return;
    }
    if (!provider) {
      toast("请先选择 Provider", "error");
      return;
    }

    setLoading(true);
    try {
      const active = await api
        .get<LoginSession>(`/accounts/cpa/login-sessions/active?service_id=${cpaService.id}`)
        .catch((err) => {
          if (err instanceof ApiError && err.status === 404) {
            return null;
          }
          throw err;
        });
      if (active) {
        const activeProvider = active.provider ?? provider;
        const activeProviderLabel = getProviderLabel(CPA_PROVIDERS, activeProvider);
        const activePoolId = active.pool_id ?? 0;
        const activePoolLabel = activePoolId ? getPoolLabel(activePoolId) : "未命名 Pool";
        setSourceKind("cpa");
        setProvider(activeProvider);
        if (activePoolId) {
          setSelectedPoolId(String(activePoolId));
        }
        setPendingCpaFlow({
          poolId: activePoolId,
          poolLabel: activePoolLabel,
          providerLabel: activeProviderLabel,
        });
        setLoginSession(active);
        setStep(3);
        setRestoredSession(true);
        writeStoredCpaSession({
          sessionId: active.id,
          provider: activeProvider,
          providerLabel: activeProviderLabel,
          poolId: activePoolId,
          poolLabel: activePoolLabel,
          serviceId: cpaService.id,
        });
        return;
      }

      const poolId = await ensurePool();
      const providerLabel = getProviderLabel(CPA_PROVIDERS, provider);
      const poolLabel = getPoolLabel(poolId);
      setPendingCpaFlow({
        poolId,
        poolLabel,
        providerLabel,
      });
      const session = await api.post<LoginSession>("/accounts/cpa/login-sessions", {
        service_id: cpaService.id,
        pool_id: poolId,
        provider,
      });
      const effectivePoolId = session.pool_id ?? poolId;
      const effectivePoolLabel = getPoolLabel(effectivePoolId);
      const effectiveProvider = session.provider ?? provider;
      const effectiveProviderLabel = getProviderLabel(CPA_PROVIDERS, effectiveProvider);
      setPendingCpaFlow({
        poolId: effectivePoolId,
        poolLabel: effectivePoolLabel,
        providerLabel: effectiveProviderLabel,
      });
      setSelectedPoolId(String(effectivePoolId));
      setProvider(effectiveProvider);
      setLoginSession(session);
      setStep(3);
      setRestoredSession(false);
      if (isActiveLoginSession(session)) {
        writeStoredCpaSession({
          sessionId: session.id,
          provider: effectiveProvider,
          providerLabel: effectiveProviderLabel,
          poolId: effectivePoolId,
          poolLabel: effectivePoolLabel,
          serviceId: cpaService.id,
        });
      }
    } catch (err) {
      setPendingCpaFlow(null);
      if (err instanceof ApiError) {
        toast(err.message, "error");
      } else {
        toast(err instanceof Error ? err.message : "启动登录失败", "error");
      }
    } finally {
      setLoading(false);
    }
  }

  async function cancelCpaLogin() {
    if (!loginSession) return;
    setLoading(true);
    try {
      await api.post(`/accounts/cpa/login-sessions/${loginSession.id}/cancel`);
      if (pollRef.current) {
        window.clearTimeout(pollRef.current);
        pollRef.current = null;
      }
      clearStoredCpaSession();
      setLoginSession({
        ...loginSession,
        status: "cancelled",
      });
      setRestoredSession(false);
      toast("授权已取消");
    } catch (err) {
      toast(err instanceof Error ? err.message : "取消授权失败", "error");
    } finally {
      setLoading(false);
    }
  }

  function resetForNextAccount() {
    setStep(1);
    setSourceKind(null);
    setProvider("");
    setDirectForm(EMPTY_DIRECT_FORM);
    setLoginSession(null);
    setResult(null);
    setPendingCpaFlow(null);
    setRestoredSession(false);
    setSelectedPoolId("");
    setNewPoolLabel("");
    clearStoredCpaSession();
  }

  function complete() {
    if (!result) {
      closeAddAccount();
      return;
    }
    closeAddAccount();
    navigate(`/admin/pools/${result.poolId}`);
  }

  function openVerificationUri() {
    if (!loginSession?.verification_uri) return;
    window.open(loginSession.verification_uri, "_blank", "noopener,noreferrer");
  }

  const canContinueStepOne = Boolean(sourceKind && provider);
  const canContinueStepTwo = Boolean(
    !poolLoadFailed && ((selectedPoolId && selectedPoolId !== NEW_POOL_VALUE) || newPoolLabel.trim()),
  );
  const canSubmitDirect = Boolean(
    directForm.label.trim() && directForm.base_url.trim() && directForm.api_key.trim(),
  );
  const cpaAuthorizationInProgress = isActiveLoginSession(loginSession);
  const canCancelCpaAuthorization = loginSession?.status === "pending";
  const remainingMs = loginSession?.expires_at ? Math.max(0, new Date(loginSession.expires_at).getTime() - nowTick) : 0;
  const countdownLabel = formatCountdown(remainingMs);
  const expiryTimeLabel = formatExpiryTime(loginSession?.expires_at);

  return (
    <Sheet open={addAccountOpen} onOpenChange={(open) => !open && closeAddAccount()}>
      <SheetContent
        side="right"
        className="w-full max-w-[42rem] border-l border-white/75 bg-[linear-gradient(180deg,rgba(251,250,247,0.98),rgba(246,244,240,0.98))] p-0 data-[side=right]:sm:max-w-[42rem] sm:max-w-[42rem]"
      >
        <SheetHeader className="border-b border-moon-200/60 px-7 py-6">
          <SheetTitle>Add Account</SheetTitle>
          <SheetDescription>先选接入方式，再确认归属 Pool，完成后即可使用。</SheetDescription>
        </SheetHeader>

        <div className="flex min-h-0 flex-1 flex-col">
          <div className="border-b border-moon-200/55 px-7 py-5">
            <p className="text-sm font-semibold text-moon-800">
              Step {step} · {currentStepMeta.title}
            </p>
            <p className="mt-1 text-sm text-moon-500">{currentStepMeta.description}</p>
            <div className="mt-4 flex items-center gap-2">
              {[1, 2, 3, 4].map((index) => (
                <span
                  key={index}
                  className={cn(
                    "inline-flex size-7 items-center justify-center rounded-full text-xs transition-colors",
                    step === index
                      ? "bg-lunar-100 text-lunar-700"
                      : step > index
                        ? "bg-moon-100 text-moon-600"
                        : "bg-white/70 text-moon-400",
                  )}
                >
                  {index}
                </span>
              ))}
            </div>
          </div>

          <div className="min-h-0 flex-1 overflow-y-auto px-7 py-7">
            {step === 1 ? (
              <section className="space-y-7">
                <div>
                  <h3 className="text-[1.08rem] font-semibold tracking-[-0.02em] text-moon-800">
                    选择接入方式
                  </h3>
                  <p className="mt-2 text-sm text-moon-500">
                    告诉 Lune 你准备从哪里接入账号。
                  </p>
                </div>

                <div className="grid gap-3 sm:grid-cols-2">
                  <PathCard
                    active={sourceKind === "cpa"}
                    title="CPA"
                    description="通过 CPA 登录接入账号"
                    onClick={() => chooseSourceKind("cpa")}
                  />
                  <PathCard
                    active={sourceKind === "openai_compat"}
                    title="直连"
                    description="连接 OpenAI 兼容上游"
                    onClick={() => chooseSourceKind("openai_compat")}
                  />
                </div>

                {sourceKind ? (
                  <div className="space-y-4">
                    <div>
                      <p className="text-[11px] uppercase tracking-[0.18em] text-moon-400">Provider</p>
                      <p className="mt-2 text-sm text-moon-500">选择服务提供方。</p>
                    </div>
                    <div className="flex flex-wrap gap-2">
                      {providerOptions.map((item) => (
                        <SelectPill
                          key={item.value}
                          active={provider === item.value}
                          label={item.label}
                          onClick={() => setProvider(item.value)}
                        />
                      ))}
                    </div>
                    {sourceKind === "cpa" ? (
                      <p className="text-sm text-moon-400">当前 Device Code 授权流程会跟随所选 Provider 记录来源。</p>
                    ) : null}
                  </div>
                ) : null}
              </section>
            ) : null}

            {step === 2 ? (
              <section className="space-y-7">
                <div>
                  <h3 className="text-[1.08rem] font-semibold tracking-[-0.02em] text-moon-800">
                    确认归属 Pool
                  </h3>
                  <p className="mt-2 text-sm text-moon-500">
                    Lune 已根据接入方式与 Provider 自动推荐归属。
                  </p>
                </div>

                <div className="rounded-[1.35rem] bg-lunar-100/45 px-4 py-4">
                  <p className="text-sm font-medium text-moon-800">
                    推荐归属：{poolLoadFailed ? "待重新加载" : recommendedPoolLabel || "手动选择"}
                  </p>
                  <p className="mt-1 text-sm text-moon-500">
                    {poolLoadFailed
                      ? "Pool 列表加载失败，请先重试，再确认归属。"
                      : "你仍然可以改到其他 Pool，或新建一个。"}
                  </p>
                </div>

                {poolLoadFailed ? (
                  <div className="rounded-[1.35rem] bg-white/70 px-4 py-4">
                    <p className="text-sm text-status-red">Pool 列表加载失败，当前不能安全判断归属。</p>
                    <Button variant="ghost" size="sm" onClick={() => void (async () => {
                      setLoading(true);
                      try {
                        const poolData = await api.get<Pool[]>("/pools");
                        setPools(poolData ?? []);
                        setPoolLoadFailed(false);
                      } catch {
                        setPoolLoadFailed(true);
                      } finally {
                        setLoading(false);
                      }
                    })()} className="mt-2 px-0">
                      重试加载
                    </Button>
                  </div>
                ) : (
                  <div>
                  <p className="text-[11px] uppercase tracking-[0.18em] text-moon-400">Pool</p>
                  <div className="mt-3 divide-y divide-moon-200/50 rounded-[1.4rem] bg-white/62 px-4">
                    {pools.map((pool) => (
                      <PoolRow
                        key={pool.id}
                        active={selectedPoolId === String(pool.id)}
                        title={pool.label}
                        onClick={() => setSelectedPoolId(String(pool.id))}
                      />
                    ))}
                    <PoolRow
                      active={selectedPoolId === NEW_POOL_VALUE || (!selectedPoolId && !pools.length)}
                      title="新建 Pool"
                      description="为当前接入新建归属位置"
                      onClick={() => setSelectedPoolId(NEW_POOL_VALUE)}
                    />
                  </div>
                  </div>
                )}

                {!poolLoadFailed && (selectedPoolId === NEW_POOL_VALUE || (!selectedPoolId && !pools.length)) ? (
                  <div className="space-y-3">
                    <Label>新 Pool 名称</Label>
                    <Input
                      value={newPoolLabel}
                      onChange={(event) => setNewPoolLabel(event.target.value)}
                      placeholder="例如 OpenAI / Claude / Gemini"
                    />
                  </div>
                ) : null}
              </section>
            ) : null}

            {step === 3 && sourceKind === "openai_compat" ? (
              <section className="space-y-7">
                <div>
                  <h3 className="text-[1.08rem] font-semibold tracking-[-0.02em] text-moon-800">
                    填写连接信息
                  </h3>
                  <p className="mt-2 text-sm text-moon-500">
                    使用最少字段完成直连接入。
                  </p>
                </div>

                <div className="space-y-5">
                  <div className="space-y-3">
                    <Label>Label</Label>
                    <Input
                      value={directForm.label}
                      onChange={(event) =>
                        setDirectForm((current) => ({ ...current, label: event.target.value }))
                      }
                      placeholder="例如 My OpenAI"
                    />
                  </div>
                  <div className="space-y-3">
                    <div className="flex items-center justify-between gap-3">
                      <Label>Base URL</Label>
                      <span className="text-xs text-moon-400">已根据 Provider 预填</span>
                    </div>
                    <Input
                      value={directForm.base_url}
                      onChange={(event) =>
                        setDirectForm((current) => ({ ...current, base_url: event.target.value }))
                      }
                      placeholder="https://api.openai.com/v1"
                    />
                  </div>
                  <div className="space-y-3">
                    <Label>API Key</Label>
                    <Input
                      value={directForm.api_key}
                      type="password"
                      onChange={(event) =>
                        setDirectForm((current) => ({ ...current, api_key: event.target.value }))
                      }
                      placeholder="粘贴你的 API Key"
                    />
                  </div>
                </div>
              </section>
            ) : null}

            {step === 3 && sourceKind === "cpa" ? (
              <section className="space-y-7">
                <div>
                  <h3 className="text-[1.08rem] font-semibold tracking-[-0.02em] text-moon-800">
                    在浏览器中完成授权
                  </h3>
                  <p className="mt-2 text-sm text-moon-500">
                    打开授权地址并输入授权码，完成后此页会继续自动处理。
                  </p>
                </div>

                <div className="relative overflow-hidden rounded-[1.65rem] border border-white/75 bg-[linear-gradient(180deg,rgba(255,255,255,0.9),rgba(243,240,249,0.64))] px-6 py-6 shadow-[0_26px_58px_-46px_rgba(33,40,63,0.26)]">
                  <div className="absolute inset-x-6 top-0 h-px moon-divider" />
                  {!loginSession ? (
                    <div className="space-y-5">
                      <div>
                        <p className="text-sm font-medium text-moon-800">准备启动 Device Code 授权</p>
                        <p className="mt-2 text-sm leading-7 text-moon-500">
                          账号会在授权完成后自动加入已确认的 Pool。
                        </p>
                      </div>
                      {!cpaService ? (
                        <p className="text-sm text-status-red">内置 CPA 服务未就绪，请稍后重试或检查容器日志。</p>
                      ) : (
                        <p className="text-sm text-moon-400">
                          授权完成后无需手动刷新，此页会自动继续。
                        </p>
                      )}
                    </div>
                  ) : (
                    <div className="space-y-6">
                      <div className="flex flex-col gap-5 border-b border-moon-200/55 pb-5 sm:flex-row sm:items-start sm:justify-between">
                        <div className="min-w-0 flex-1 space-y-2.5">
                          <div className="flex flex-wrap items-center gap-2">
                            <span className="eyebrow-label">Authorization Window</span>
                            {restoredSession ? (
                              <span className="rounded-full bg-lunar-100/85 px-2.5 py-1 text-[11px] tracking-[0.12em] text-lunar-700">
                                已恢复上次进度
                              </span>
                            ) : null}
                          </div>
                          <div className="flex items-center gap-2 text-moon-800">
                            {loginSession.status === "failed" || loginSession.status === "expired" || loginSession.status === "cancelled" ? (
                              <AlertCircle className="size-4.5 text-status-red" />
                            ) : (
                              <ShieldCheck className="size-4.5 text-lunar-700" />
                            )}
                            <p className="text-[1.05rem] font-semibold tracking-[-0.02em]">
                              {getLoginStatusLabel(loginSession.status)}
                            </p>
                          </div>
                          <p className="max-w-md text-sm leading-6 text-moon-500">
                            {getLoginStatusHint(loginSession.status)}
                          </p>
                        </div>

                        <div className="w-full rounded-[1.25rem] bg-white/72 px-4 py-3 shadow-[inset_0_1px_0_rgba(255,255,255,0.8)] sm:w-[11.25rem] sm:shrink-0">
                          <div className="flex items-center gap-2 text-[11px] uppercase tracking-[0.18em] text-moon-400">
                            <Clock3 className="size-3.5" />
                            <span>剩余时间</span>
                          </div>
                          <p className="mt-2 font-editorial text-[2rem] font-semibold leading-none tracking-[0.04em] text-moon-800 tabular-nums">
                            {countdownLabel}
                          </p>
                          <p className="mt-2 text-xs text-moon-400">
                            窗口将于 {expiryTimeLabel} 结束
                          </p>
                        </div>
                      </div>

                      <div className="space-y-5">
                        <div className="space-y-3 border-b border-moon-200/50 pb-5">
                          <div className="flex items-center justify-between gap-3">
                            <p className="text-sm font-medium text-moon-800">1. 打开授权地址</p>
                            <div className="flex items-center gap-3">
                              {loginSession.verification_uri ? (
                                <InlineCopyAction value={loginSession.verification_uri} />
                              ) : null}
                              <button
                                type="button"
                                onClick={openVerificationUri}
                                className="inline-flex items-center gap-1.5 text-xs text-moon-400 transition-colors hover:text-moon-700"
                              >
                                <ExternalLink className="size-3.5" />
                                <span>在浏览器打开</span>
                              </button>
                            </div>
                          </div>
                          <p className="break-all text-sm leading-7 text-moon-600">{loginSession.verification_uri}</p>
                        </div>

                        <div className="space-y-3">
                          <div className="flex items-center justify-between gap-3">
                            <p className="text-sm font-medium text-moon-800">2. 输入授权码</p>
                            {loginSession.user_code ? (
                              <InlineCopyAction value={loginSession.user_code} />
                            ) : null}
                          </div>
                          <div className="overflow-x-auto rounded-[1.45rem] bg-[linear-gradient(180deg,rgba(234,229,248,0.68),rgba(248,246,252,0.82))] px-5 py-6">
                            <p className="min-w-fit whitespace-nowrap text-[2rem] font-semibold tracking-[0.26em] text-moon-800 tabular-nums sm:text-[2.25rem]">
                              {loginSession.user_code || "等待生成"}
                            </p>
                          </div>
                        </div>
                      </div>

                      {loginSession.error_message ? (
                        <div className="rounded-[1.2rem] border border-status-red/15 bg-red-50/75 px-4 py-3 text-sm text-status-red">
                          {loginSession.error_message}
                        </div>
                      ) : null}
                    </div>
                  )}
                </div>
              </section>
            ) : null}

            {step === 4 && result ? (
              <section className="space-y-7">
                <div>
                  <h3 className="text-[1.08rem] font-semibold tracking-[-0.02em] text-moon-800">
                    账号已添加
                  </h3>
                  <p className="mt-2 text-sm text-moon-500">
                    这次接入已经完成，可以继续添加，或直接进入对应 Pool。
                  </p>
                </div>

                <div className="rounded-[1.55rem] bg-white/70 px-6 py-6">
                  <div className="flex items-start gap-3">
                    <span className="inline-flex size-10 items-center justify-center rounded-full bg-lunar-100/88 text-lunar-700">
                      <CheckCircle2 className="size-5" />
                    </span>
                    <div className="space-y-3">
                      <div>
                        <p className="text-sm font-medium text-moon-800">账号已添加</p>
                        <p className="mt-1 text-sm text-moon-500">已完成接入，可以立即使用。</p>
                      </div>
                      <div className="space-y-2 text-sm text-moon-600">
                        <p>接入方式：{result.source}</p>
                        <p>归属 Pool：{result.poolLabel}</p>
                        <p>账号标识：{result.label}</p>
                        <p>状态：正常</p>
                      </div>
                    </div>
                  </div>
                </div>
              </section>
            ) : null}
          </div>

          <SheetFooter className="border-t border-moon-200/60 bg-white/72 px-7 py-5 sm:flex-row sm:items-center sm:justify-between">
            <div>
              {step === 3 && sourceKind === "cpa" && cpaAuthorizationInProgress ? (
                <div className="flex flex-wrap gap-2">
                  <Button variant="ghost" onClick={closeAddAccount}>
                    稍后继续
                  </Button>
                  <Button variant="outline" onClick={cancelCpaLogin} disabled={loading || !canCancelCpaAuthorization}>
                    <X className="size-4" />
                    取消授权
                  </Button>
                </div>
              ) : step > 1 && step < 4 && !cpaAuthorizationInProgress ? (
                <Button variant="ghost" onClick={() => setStep((current) => (current - 1) as Step)}>
                  返回
                </Button>
              ) : step === 4 ? (
                <Button variant="ghost" onClick={resetForNextAccount}>
                  继续添加
                </Button>
              ) : (
                <span />
              )}
            </div>

            <div>
              {step === 1 ? (
                <Button onClick={() => setStep(2)} disabled={!canContinueStepOne}>
                  继续
                </Button>
              ) : null}

              {step === 2 ? (
                <Button onClick={() => setStep(3)} disabled={!canContinueStepTwo}>
                  确认 Pool
                </Button>
              ) : null}

              {step === 3 && sourceKind === "openai_compat" ? (
                <Button onClick={submitDirect} disabled={loading || !canSubmitDirect}>
                  {loading ? <Loader2 className="size-4 animate-spin" /> : null}
                  确认添加
                </Button>
              ) : null}

              {step === 3 && sourceKind === "cpa" && !loginSession ? (
                <Button onClick={startCpaLogin} disabled={loading || !cpaService}>
                  {loading ? <Loader2 className="size-4 animate-spin" /> : null}
                  开始授权
                </Button>
              ) : null}

              {step === 3 && sourceKind === "cpa" && loginSession && (loginSession.status === "failed" || loginSession.status === "expired") ? (
                <Button onClick={startCpaLogin} disabled={loading || !cpaService}>
                  {loading ? <Loader2 className="size-4 animate-spin" /> : null}
                  重新开始
                </Button>
              ) : null}

              {step === 3 && sourceKind === "cpa" && loginSession?.status === "cancelled" ? (
                <Button onClick={startCpaLogin} disabled={loading || !cpaService}>
                  {loading ? <Loader2 className="size-4 animate-spin" /> : null}
                  重新开始
                </Button>
              ) : null}

              {step === 4 ? (
                <Button onClick={complete}>
                  <Sparkles className="size-4" />
                  前往 Pool
                </Button>
              ) : null}
            </div>
          </SheetFooter>
        </div>
      </SheetContent>
    </Sheet>
  );
}
