import { useEffect, useMemo, useState } from "react";
import AccountCard from "@/components/AccountCard";
import AccountDetailSheet from "@/components/AccountDetailSheet";
import ConfirmDialog from "@/components/ConfirmDialog";
import DragSortArea from "@/components/DragSortArea";
import EmptyState from "@/components/EmptyState";
import ErrorState from "@/components/ErrorState";
import PageHeader from "@/components/PageHeader";
import { useAdminUI } from "@/components/AdminUI";
import { toast } from "@/components/Feedback";
import { Skeleton } from "@/components/ui/skeleton";
import { api } from "@/lib/api";
import { compact, pct } from "@/lib/fmt";
import { ensureArray, getPoolHealth } from "@/lib/lune";
import { matchPath, usePathname } from "@/lib/router";
import type {
  Overview,
  PoolDetailResponse,
  PoolMember,
  RevealedAccessToken,
} from "@/lib/types";

export default function PoolDetailPage() {
  const pathname = usePathname();
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

  function load() {
    if (!hasValidPoolId) {
      setLoading(false);
      setError("无效的 Pool 路径");
      return;
    }
    setLoading(true);
    setError(null);
    Promise.all([
      api.get<PoolDetailResponse>(`/pools/${poolId}`),
      api.get<Overview>("/overview"),
    ])
      .then(([detailData, overviewData]) => {
        setDetail(detailData);
        setOverview(overviewData);
      })
      .catch((err) => {
        setError(err instanceof Error ? err.message : "Pool 详情加载失败");
      })
      .finally(() => setLoading(false));
  }

  useEffect(() => {
    load();
  }, [hasValidPoolId, poolId, dataVersion]);

  const pool = detail?.pool ?? null;
  const members = ensureArray(detail?.members);
  const stats = detail?.stats;
  const poolTokens = ensureArray(detail?.tokens);
  const statsByAccount = ensureArray(stats?.by_account);
  const primaryPoolTokenId = poolTokens.find((token) => token.enabled)?.id ?? null;
  const poolTokenCacheKey = useMemo(
    () => poolTokens.map((token) => `${token.id}:${token.enabled ? 1 : 0}`).join("|"),
    [poolTokens],
  );
  const accountRequestMap = useMemo(() => {
    const map = new Map<number, number>();
    statsByAccount.forEach((row) => {
      map.set(row.account_id, row.requests);
    });
    return map;
  }, [statsByAccount]);
  const health = pool ? getPoolHealth(pool) : "degraded";
  const detailMember = useMemo(
    () => (detailMemberId == null ? null : members.find((m) => m.id === detailMemberId) ?? null),
    [detailMemberId, members],
  );

  useEffect(() => {
    setConnectTokenValue(null);
    setRevealedTokenCache({});
  }, [poolTokenCacheKey]);

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

  async function getTokenForConnect(): Promise<string> {
    if (connectTokenValue) {
      return connectTokenValue;
    }
    if (primaryPoolTokenId) {
      const token = await revealPoolToken(primaryPoolTokenId);
      setConnectTokenValue(token);
      return token;
    }
    if (revealedGlobalToken) {
      return revealedGlobalToken;
    }
    if (overview?.global_token_id) {
      const revealed = await api.post<RevealedAccessToken>("/tokens/global/reveal");
      setRevealedGlobalToken(revealed.token);
      return revealed.token;
    }
    return "";
  }

  async function reorderMembers(memberIds: number[]) {
    await api.put(`/pools/${poolId}/members/reorder`, { member_ids: memberIds });
    toast("优先级已更新");
    refreshData();
  }

  async function toggleMember(member: PoolMember, enabled: boolean) {
    await api.put(`/pools/${poolId}/members/${member.id}`, { enabled });
    toast(enabled ? "账号已启用" : "账号已移入禁用区");
    refreshData();
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

  if (!members.length) {
    return (
      <EmptyState
        title={`${pool.label} 还没有账号。`}
        description="当前页面已经收敛为账号编排面。先从全局入口添加账号，再回来排序、停用和调整优先级。"
      />
    );
  }

  return (
    <div className="space-y-7 pb-6">
      <PageHeader
        title={pool.label}
        description="左边是 Active Pool，拖动卡片即可调整路由优先级；右侧停泊区收纳临时停用的账号。"
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
      />

      <DragSortArea
        members={members}
        onReorder={reorderMembers}
        onToggleEnabled={toggleMember}
        renderMember={(member, options) => (
          <AccountCard
            member={member}
            variant={member.enabled ? "active" : "disabled"}
            priorityIndex={options.priorityIndex}
            dragging={options.dragging}
            dragHandleProps={options.dragHandleProps}
            requests={accountRequestMap.get(member.account_id) ?? 0}
            onOpenDetails={() => setDetailMemberId(member.id)}
            onToggleEnabled={() => toggleMember(member, !member.enabled)}
            onDelete={() => setDeleteTarget(member)}
            onRefreshModels={() => refreshModels(member)}
          />
        )}
      />
      <AccountDetailSheet
        member={detailMember}
        requests={detailMember ? accountRequestMap.get(detailMember.account_id) ?? 0 : 0}
        resolveToken={getTokenForConnect}
        onOpenChange={(open) => {
          if (!open) setDetailMemberId(null);
        }}
        onToggleEnabled={() => {
          if (!detailMember) return;
          toggleMember(detailMember, !detailMember.enabled);
        }}
        onDelete={() => {
          if (!detailMember) return;
          setDeleteTarget(detailMember);
          setDetailMemberId(null);
        }}
        onRefreshModels={() => detailMember && refreshModels(detailMember)}
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
