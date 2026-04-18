import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Check, Copy, KeyRound, QrCode, ShieldCheck } from "lucide-react";
import AccountCard from "@/components/AccountCard";
import AccountDetailSheet from "@/components/AccountDetailSheet";
import ConfirmDialog from "@/components/ConfirmDialog";
import DragSortArea from "@/components/DragSortArea";
import EmptyState from "@/components/EmptyState";
import EnvSnippetsDialog from "@/components/EnvSnippetsDialog";
import ErrorState from "@/components/ErrorState";
import PageHeader from "@/components/PageHeader";
import QrCodeDialog from "@/components/QrCodeDialog";
import { useAdminUI } from "@/components/AdminUI";
import { toast } from "@/components/Feedback";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { api } from "@/lib/api";
import { compact, pct } from "@/lib/fmt";
import { ensureArray, getApiBaseUrl, getPoolHealth } from "@/lib/lune";
import { matchPath, usePathname, useRouter } from "@/lib/router";
import type {
  Overview,
  PoolDetailResponse,
  PoolMember,
  RevealedAccessToken,
} from "@/lib/types";

const SELF_CHECK_MESSAGE = "你好，请用一句话回复我。";
const SELF_CHECK_TIMEOUT_MS = 25_000;
const FLASH_HOLD_MS = 2400;

type FlashState = "success" | "error" | null;
type AccountStatsEntry = {
  requests: number;
  successRate: number | null;
  inputTokens: number;
  outputTokens: number;
};

export default function PoolDetailPage() {
  const pathname = usePathname();
  const { navigate } = useRouter();
  const { dataVersion, refreshData } = useAdminUI();
  const params = matchPath("/admin/pools/:id", pathname);
  const poolId = Number(params?.id);
  const hasValidPoolId = Number.isInteger(poolId) && poolId > 0;
  const [detail, setDetail] = useState<PoolDetailResponse | null>(null);
  const [overview, setOverview] = useState<Overview | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<PoolMember | null>(null);
  const [detailMemberId, setDetailMemberId] = useState<number | null>(null);
  const [connectTokenValue, setConnectTokenValue] = useState<string | null>(null);
  const [revealedTokenCache, setRevealedTokenCache] = useState<Record<number, string>>({});
  const [revealedGlobalToken, setRevealedGlobalToken] = useState<string | null>(null);
  const [snippetsOpen, setSnippetsOpen] = useState(false);
  const [qrOpen, setQrOpen] = useState(false);
  const [dialogToken, setDialogToken] = useState<string>("");
  const [selfChecking, setSelfChecking] = useState(false);
  const [flashMap, setFlashMap] = useState<Record<number, FlashState>>({});
  const loadSeqRef = useRef(0);
  const hasLoadedRef = useRef(false);
  const flashTimersRef = useRef<Map<number, number>>(new Map());
  const selfCheckingRef = useRef(false);
  const revealInFlightRef = useRef<Set<number>>(new Set());

  const load = useCallback(() => {
    if (!hasValidPoolId) {
      setLoading(false);
      setError("无效的 Pool 路径");
      return;
    }
    const seq = ++loadSeqRef.current;
    if (!hasLoadedRef.current) setLoading(true);
    setError(null);
    Promise.all([
      api.get<PoolDetailResponse>(`/pools/${poolId}`),
      api.get<Overview>("/overview"),
    ])
      .then(([detailData, overviewData]) => {
        if (seq !== loadSeqRef.current) return;
        hasLoadedRef.current = true;
        setDetail(detailData);
        setOverview(overviewData);
      })
      .catch((err) => {
        if (seq !== loadSeqRef.current) return;
        hasLoadedRef.current = true;
        setError(err instanceof Error ? err.message : "Pool 详情加载失败");
      })
      .finally(() => {
        if (seq !== loadSeqRef.current) return;
        setLoading(false);
      });
  }, [hasValidPoolId, poolId]);

  useEffect(() => {
    hasLoadedRef.current = false;
  }, [hasValidPoolId, poolId]);

  useEffect(() => {
    load();
  }, [load, dataVersion]);

  useEffect(() => {
    return () => {
      flashTimersRef.current.forEach((id) => window.clearTimeout(id));
      flashTimersRef.current.clear();
    };
  }, []);

  const pool = detail?.pool ?? null;
  const members = ensureArray(detail?.members);
  const stats = detail?.stats;
  const poolTokens = ensureArray(detail?.tokens);
  const statsByAccount = ensureArray(stats?.by_account);
  // Prefer the first enabled pool-scoped token; if every token is disabled we
  // still surface the first one so the user sees the masked value plus a
  // warning, rather than an empty slot that looks like "nothing configured".
  const primaryPoolToken = useMemo(
    () => poolTokens.find((t) => t.enabled) ?? poolTokens[0] ?? null,
    [poolTokens],
  );
  const primaryPoolTokenId = primaryPoolToken?.id ?? null;
  const poolTokenCacheKey = useMemo(
    () => poolTokens.map((token) => `${token.id}:${token.enabled ? 1 : 0}`).join("|"),
    [poolTokens],
  );
  const accountStatsMap = useMemo(() => {
    const map = new Map<number, AccountStatsEntry>();
    statsByAccount.forEach((row) => {
      map.set(row.account_id, {
        requests: row.requests ?? 0,
        successRate:
          row.requests > 0 && typeof row.success_rate === "number" ? row.success_rate : null,
        inputTokens: row.input_tokens ?? 0,
        outputTokens: row.output_tokens ?? 0,
      });
    });
    return map;
  }, [statsByAccount]);
  const enabledMembers = useMemo(
    () => members.filter((m) => m.enabled),
    [members],
  );
  const priorityIndexMap = useMemo(() => {
    const map = new Map<number, number>();
    enabledMembers.forEach((m, idx) => map.set(m.id, idx + 1));
    return map;
  }, [enabledMembers]);
  const health = pool ? getPoolHealth(pool) : "degraded";
  const detailMember = useMemo(
    () => (detailMemberId == null ? null : members.find((m) => m.id === detailMemberId) ?? null),
    [detailMemberId, members],
  );
  const detailMemberStats: AccountStatsEntry = useMemo(() => {
    if (!detailMember) {
      return { requests: 0, successRate: null, inputTokens: 0, outputTokens: 0 };
    }
    return (
      accountStatsMap.get(detailMember.account_id) ?? {
        requests: 0,
        successRate: null,
        inputTokens: 0,
        outputTokens: 0,
      }
    );
  }, [detailMember, accountStatsMap]);
  const detailPriorityIndex = detailMember ? priorityIndexMap.get(detailMember.id) : undefined;

  const baseUrl = getApiBaseUrl();
  const hasConnectToken = Boolean(primaryPoolTokenId) || Boolean(overview?.global_token_id);
  const primaryTokenRevealed = primaryPoolTokenId
    ? revealedTokenCache[primaryPoolTokenId] ?? null
    : null;
  const primaryTokenDisplay = primaryPoolToken
    ? primaryTokenRevealed ?? primaryPoolToken.token_masked
    : overview?.global_token_masked ?? "未配置全局 Token";
  const firstPoolModel = useMemo(() => {
    for (const member of enabledMembers) {
      const models = ensureArray(member.account?.models);
      if (models.length > 0) return models[0];
    }
    return ensureArray(pool?.models)[0];
  }, [enabledMembers, pool]);

  // When the set of tokens or their enabled flags change, drop the "connect"
  // choice so the next Env Snippets / QR open re-picks a still-enabled token.
  // We do NOT wipe revealedTokenCache — entries are keyed by token id and
  // remain valid across enabled-toggles (toggling doesn't change the value).
  // Wiping would create a cross-effect stale-state trap: the reveal effect
  // below, reading its own render's closure, would see the pre-wipe cache
  // and skip re-fetch while the UI falls back to masked.
  useEffect(() => {
    setConnectTokenValue(null);
  }, [poolTokenCacheKey]);

  // Auto-reveal the primary token so the meta row shows the full value without
  // an extra gesture. Fires when primaryPoolTokenId changes (including initial
  // mount). revealInFlightRef de-duplicates the React.StrictMode double-mount.
  useEffect(() => {
    if (!primaryPoolTokenId) return;
    if (revealedTokenCache[primaryPoolTokenId]) return;
    if (revealInFlightRef.current.has(primaryPoolTokenId)) return;
    const tokenId = primaryPoolTokenId;
    revealInFlightRef.current.add(tokenId);
    let cancelled = false;
    revealPoolToken(tokenId)
      .catch((err) => {
        if (cancelled) return;
        toast(err instanceof Error ? err.message : "读取 Token 失败", "error");
      })
      .finally(() => {
        revealInFlightRef.current.delete(tokenId);
      });
    return () => {
      cancelled = true;
    };
    // revealPoolToken is a new closure each render but the in-flight ref and
    // cache checks above make re-invocation safe.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [primaryPoolTokenId]);

  useEffect(() => {
    setRevealedGlobalToken(null);
  }, [overview?.global_token_id]);

  async function revealPoolToken(tokenId: number): Promise<string> {
    const cached = revealedTokenCache[tokenId];
    if (cached) {
      return cached;
    }
    const revealed = await api.post<RevealedAccessToken>(`/tokens/${tokenId}/reveal`);
    setRevealedTokenCache((current) => ({ ...current, [tokenId]: revealed.token }));
    return revealed.token;
  }

  async function revealGlobalToken(): Promise<string> {
    if (revealedGlobalToken) return revealedGlobalToken;
    if (!overview?.global_token_id) return "";
    const revealed = await api.post<RevealedAccessToken>("/tokens/global/reveal");
    setRevealedGlobalToken(revealed.token);
    return revealed.token;
  }

  async function getTokenForConnect(): Promise<string> {
    if (connectTokenValue) {
      return connectTokenValue;
    }
    if (primaryPoolTokenId) {
      const token = await revealPoolToken(primaryPoolTokenId);
      setConnectTokenValue(token);
      return token;
    }
    return revealGlobalToken();
  }

  // For per-account direct requests we need a token that leaves tokenPoolID = nil on
  // the gateway, otherwise X-Lune-Account-Id is silently dropped (gateway/handler.go).
  // Pool-scoped tokens always route through pool weights — use global whenever possible.
  async function getTokenForAccountProbe(): Promise<string> {
    const global = await revealGlobalToken();
    if (global) return global;
    return getTokenForConnect();
  }

  async function reorderMembers(memberIds: number[]) {
    try {
      await api.put(`/pools/${poolId}/members/reorder`, { member_ids: memberIds });
      toast("优先级已更新");
    } catch (err) {
      toast(err instanceof Error ? err.message : "优先级更新失败", "error");
    } finally {
      refreshData();
    }
  }

  async function toggleMember(member: PoolMember, enabled: boolean) {
    try {
      await api.put(`/pools/${poolId}/members/${member.id}`, { enabled });
      toast(enabled ? "账号已启用" : "账号已移入禁用区");
    } catch (err) {
      toast(err instanceof Error ? err.message : "账号状态更新失败", "error");
    } finally {
      refreshData();
    }
  }

  async function refreshModels(member: PoolMember) {
    try {
      await api.post(`/accounts/${member.account_id}/discover-models`);
      toast("模型刷新任务已触发");
      refreshData();
    } catch (err) {
      toast(err instanceof Error ? err.message : "刷新模型失败", "error");
    }
  }

  async function deleteAccount() {
    if (!deleteTarget) return;
    try {
      await api.delete(`/accounts/${deleteTarget.account_id}`);
      toast("账号已删除");
      setDeleteTarget(null);
      refreshData();
    } catch (err) {
      toast(err instanceof Error ? err.message : "删除账号失败", "error");
    }
  }

  function setFlash(memberId: number, state: FlashState) {
    setFlashMap((prev) => ({ ...prev, [memberId]: state }));
    const prevTimer = flashTimersRef.current.get(memberId);
    if (prevTimer) window.clearTimeout(prevTimer);
    if (state != null) {
      const timer = window.setTimeout(() => {
        setFlashMap((prev) => {
          const next = { ...prev };
          delete next[memberId];
          return next;
        });
        flashTimersRef.current.delete(memberId);
      }, FLASH_HOLD_MS);
      flashTimersRef.current.set(memberId, timer);
    } else {
      flashTimersRef.current.delete(memberId);
    }
  }

  function jumpToTokenInSettings(tokenId: number) {
    // Custom router stores pathname without hash; replaceState afterwards so
    // the URL is shareable without confusing pathname matching in App.tsx.
    // Scroll + highlight are owned by SettingsPage's hash effect — this
    // component unmounts as soon as the route changes, so any scroll retry
    // scheduled here would get cleaned up before the token DOM exists.
    navigate("/admin/settings");
    window.history.replaceState(null, "", `/admin/settings#access-token-${tokenId}`);
  }

  async function runSelfCheck() {
    if (selfCheckingRef.current || enabledMembers.length === 0) return;
    selfCheckingRef.current = true;
    setSelfChecking(true);
    try {
      const token = await getTokenForAccountProbe().catch(() => "");
      if (!token) {
        toast("未找到可用的 Token，无法自检", "error");
        return;
      }
      if (!overview?.global_token_id) {
        toast("没有全局 Token，自检改用 Pool Token，结果反映整个 Pool 而非单账号", "error");
      }

      // Each target can probe multiple models in parallel and passes when any
      // single model succeeds. Codex accounts ship with `gpt-5-codex` as the
      // first model but that endpoint is rejected by CPA for ChatGPT auth;
      // letting the user override via probe_models (or falling back to the
      // last discovered model, which is usually the plain chat model) avoids
      // spurious failures. Accounts with no usable model at all are skipped.
      const targets = enabledMembers
        .map((member) => {
          const accountModels = ensureArray(member.account?.models);
          const configured = ensureArray(member.account?.probe_models);
          const fallback = accountModels[accountModels.length - 1] ?? firstPoolModel;
          const models = configured.length > 0 ? configured : fallback ? [fallback] : [];
          return { member, models };
        })
        .filter((t) => t.models.length > 0);
      if (targets.length === 0) {
        toast("没有可测模型", "error");
        return;
      }

      setFlashMap({});
      const initial: Record<number, FlashState> = {};
      targets.forEach(({ member }) => {
        initial[member.id] = null;
      });
      setFlashMap(initial);

      const results = await Promise.all(
        targets.map(async ({ member, models }) => {
          const attempts = await Promise.all(
            models.map(async (model) => {
              const controller = new AbortController();
              const timer = window.setTimeout(() => controller.abort(), SELF_CHECK_TIMEOUT_MS);
              try {
                const res = await fetch("/v1/chat/completions", {
                  method: "POST",
                  signal: controller.signal,
                  headers: {
                    "Content-Type": "application/json",
                    Authorization: `Bearer ${token}`,
                    "X-Lune-Account-Id": String(member.account_id),
                  },
                  body: JSON.stringify({
                    model,
                    messages: [{ role: "user", content: SELF_CHECK_MESSAGE }],
                    stream: false,
                  }),
                });
                if (res.ok) return { model, ok: true as const };
                // Consume the error body best-effort so we can surface
                // a meaningful reason in the detail drawer.
                let msg = `HTTP ${res.status}`;
                try {
                  const txt = await res.text();
                  if (txt) msg = txt.slice(0, 240);
                } catch { /* ignore */ }
                return { model, ok: false as const, err: msg };
              } catch (err) {
                const msg = err instanceof Error ? err.message : "request failed";
                return { model, ok: false as const, err: msg };
              } finally {
                window.clearTimeout(timer);
              }
            }),
          );
          const passed = attempts.find((a) => a.ok);
          const ok = Boolean(passed);
          setFlash(member.id, ok ? "success" : "error");
          // Persist so the card badge (getAccountHealth) reflects the verdict.
          // Any individual model failure is aggregated into a compact message;
          // success leaves last_probe_error empty. Awaited so the refreshData
          // below observes the new probe state instead of racing past it.
          const status: "healthy" | "error" = ok ? "healthy" : "error";
          const last_error = ok
            ? ""
            : attempts
                .filter((a) => !a.ok)
                .map((a) => `${a.model}: ${("err" in a && a.err) || "failed"}`)
                .join(" | ");
          try {
            await api.post(`/accounts/${member.account_id}/probe-result`, { status, last_error });
          } catch { /* best-effort — flash + toast still reflect the outcome */ }
          return { memberId: member.id, ok };
        }),
      );
      const successCount = results.filter((r) => r.ok).length;
      const failCount = results.length - successCount;
      // Refresh account data so new last_probe_* propagates to cards+drawer.
      refreshData();
      if (failCount === 0) {
        toast(`自检全部通过（${successCount}/${results.length}）`);
      } else {
        toast(
          `自检完成：${successCount}/${results.length} 成功，${failCount} 失败`,
          "error",
        );
      }
    } finally {
      selfCheckingRef.current = false;
      setSelfChecking(false);
    }
  }

  async function openSnippetsWithToken() {
    try {
      const token = await getTokenForConnect();
      if (!token) {
        toast("未找到可用的 Token", "error");
        return;
      }
      setDialogToken(token);
      setSnippetsOpen(true);
    } catch (err) {
      toast(err instanceof Error ? err.message : "读取 Token 失败", "error");
    }
  }

  async function openQrWithToken() {
    try {
      const token = await getTokenForConnect();
      if (!token) {
        toast("未找到可用的 Token", "error");
        return;
      }
      setDialogToken(token);
      setQrOpen(true);
    } catch (err) {
      toast(err instanceof Error ? err.message : "读取 Token 失败", "error");
    }
  }

  if (loading) {
    return (
      <div className="space-y-8">
        <Skeleton className="h-36 rounded-[2rem]" />
        <Skeleton className="h-[34rem] rounded-[1.8rem]" />
        <Skeleton className="h-56 rounded-[1.8rem]" />
      </div>
    );
  }

  if (error) {
    return <ErrorState message={error} onRetry={load} />;
  }

  if (!pool || !detail) {
    return <ErrorState message="Pool 不存在或已被删除。" onRetry={load} />;
  }

  const isEmpty = members.length === 0;

  return (
    <div className="space-y-7 pb-6">
      <PageHeader
        title={pool.label}
        description="左边是 Active Pool，拖动卡片即可调整路由优先级；右侧停泊区收纳临时停用的账号。"
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={runSelfCheck}
              disabled={selfChecking || enabledMembers.length === 0}
              className="rounded-full border-status-green/55 bg-status-green/10 text-status-green hover:bg-status-green/18 hover:text-status-green"
            >
              <ShieldCheck className="size-3.5" />
              {selfChecking ? "自检中" : "自检 Pool"}
            </Button>
          </div>
        }
        meta={
          <>
            <span className="inline-flex items-center gap-2">
              <span
                className={`size-2 rounded-full ${
                  health === "healthy"
                    ? "bg-status-green"
                    : health === "degraded"
                      ? "bg-status-yellow"
                      : health === "error"
                        ? "bg-status-red"
                        : "bg-moon-400"
                }`}
              />
              <span className="capitalize">{health}</span>
            </span>
            <span>{pool.account_count} 账号</span>
            <span>{pool.routable_account_count} 可用</span>
            <span>24h 请求 {compact(stats?.total_requests ?? 0)}</span>
            <span>成功率 {pct(stats?.success_rate ?? 0)}</span>
          </>
        }
        metaEnd={
          <>
            {primaryPoolToken ? (
              // inline-flex flex-wrap + basis-full on the disabled link lets the
              // "已禁用 · ..." warning drop onto its own visual row tucked under
              // the token, instead of pushing Env/QR off-line or forcing the
              // whole metaEnd group to wrap as a single orphaned block.
              <span className="inline-flex flex-wrap items-center gap-x-1.5 gap-y-0.5">
                <span
                  className={`size-2 rounded-full ${
                    primaryPoolToken.enabled ? "bg-status-green" : "bg-moon-400"
                  }`}
                  aria-hidden
                />
                <code className="font-mono text-[12.5px] text-moon-700">
                  {primaryTokenDisplay}
                </code>
                {primaryTokenRevealed ? (
                  <InlineCopyIcon value={primaryTokenRevealed} />
                ) : null}
                {!primaryPoolToken.enabled ? (
                  <button
                    type="button"
                    onClick={() => jumpToTokenInSettings(primaryPoolToken.id)}
                    className="basis-full text-status-yellow hover:underline"
                  >
                    已禁用 · 前往 Settings 启用 →
                  </button>
                ) : null}
              </span>
            ) : (
              <span className="truncate text-moon-400" title={primaryTokenDisplay}>
                Token {primaryTokenDisplay}
              </span>
            )}
            <Button
              variant="outline"
              size="icon-xs"
              onClick={openSnippetsWithToken}
              disabled={!hasConnectToken}
              className="rounded-full border-moon-200/55 bg-white/45"
              aria-label="Env Snippets"
              title="Env Snippets"
            >
              <KeyRound className="size-3" />
            </Button>
            <Button
              variant="outline"
              size="icon-xs"
              onClick={openQrWithToken}
              disabled={!hasConnectToken}
              className="rounded-full border-moon-200/55 bg-white/45"
              aria-label="Token QR"
              title="Token QR"
            >
              <QrCode className="size-3" />
            </Button>
          </>
        }
      />

      {isEmpty ? (
        <EmptyState
          title={`${pool.label} 还没有账号。`}
          description="当前页面已经收敛为账号编排面。先从全局入口添加账号，再回来排序、停用和调整优先级。"
        />
      ) : (
      <DragSortArea
        members={members}
        onReorder={reorderMembers}
        onToggleEnabled={toggleMember}
        renderMember={(member, options) => {
          const spotlightActive = detailMemberId != null;
          const isSelected = detailMemberId === member.id;
          return (
            <AccountCard
              member={member}
              variant={member.enabled ? "active" : "disabled"}
              priorityIndex={options.priorityIndex}
              dragging={options.dragging}
              dragHandleProps={options.dragHandleProps}
              requests={accountStatsMap.get(member.account_id)?.requests ?? 0}
              successRate={accountStatsMap.get(member.account_id)?.successRate ?? null}
              selected={isSelected}
              dimmed={spotlightActive && !isSelected}
              flashState={flashMap[member.id] ?? null}
              onOpenDetails={() => setDetailMemberId(member.id)}
              onToggleEnabled={() => toggleMember(member, !member.enabled)}
              onDelete={() => setDeleteTarget(member)}
              onRefreshModels={() => refreshModels(member)}
            />
          );
        }}
      />
      )}

      <AccountDetailSheet
        member={detailMember}
        stats={detailMemberStats}
        priorityIndex={detailPriorityIndex}
        poolId={poolId}
        resolveToken={getTokenForAccountProbe}
        hasGlobalToken={Boolean(overview?.global_token_id)}
        onOpenChange={(open) => {
          if (!open) setDetailMemberId(null);
        }}
      />

      <EnvSnippetsDialog
        open={snippetsOpen}
        onOpenChange={setSnippetsOpen}
        title={`${pool.label} · Env Snippets`}
        baseUrl={baseUrl}
        token={dialogToken}
        model={firstPoolModel}
      />
      <QrCodeDialog
        open={qrOpen}
        onOpenChange={setQrOpen}
        title={`${pool.label} · Token QR`}
        baseUrl={baseUrl}
        token={dialogToken}
      />

      <ConfirmDialog
        open={Boolean(deleteTarget)}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        title="删除账号"
        description={`将删除 ${deleteTarget?.account?.label ?? "该账号"}，并从当前 Pool 移除。`}
        onConfirm={deleteAccount}
      />
    </div>
  );
}

// Frameless copy affordance sized to sit on the meta-row text baseline; the
// framed CopyButton (size-7) made the token chip visibly taller than sibling
// text spans, throwing the whole "healthy · 3 账号 · token" row out of line.
function InlineCopyIcon({ value }: { value: string }) {
  const [copied, setCopied] = useState(false);
  async function handle() {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      toast("已复制");
      window.setTimeout(() => setCopied(false), 1600);
    } catch {
      toast("复制失败", "error");
    }
  }
  return (
    <button
      type="button"
      onClick={handle}
      className="text-moon-400 transition-colors hover:text-moon-700"
      aria-label="复制 Token"
      title="复制 Token"
    >
      {copied ? <Check className="size-3 text-status-green" /> : <Copy className="size-3" />}
    </button>
  );
}
