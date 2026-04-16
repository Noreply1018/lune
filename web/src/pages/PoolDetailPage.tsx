import { useEffect, useMemo, useState } from "react";
import { KeyRound, Plus, QrCode } from "lucide-react";
import AccountCard from "@/components/AccountCard";
import ConfirmDialog from "@/components/ConfirmDialog";
import DragSortArea from "@/components/DragSortArea";
import EmptyState from "@/components/EmptyState";
import EnvSnippetsDialog from "@/components/EnvSnippetsDialog";
import ErrorState from "@/components/ErrorState";
import PageHeader from "@/components/PageHeader";
import QrCodeDialog from "@/components/QrCodeDialog";
import SectionHeading from "@/components/SectionHeading";
import { useAdminUI } from "@/components/AdminUI";
import { toast } from "@/components/Feedback";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Skeleton } from "@/components/ui/skeleton";
import { api } from "@/lib/api";
import { compact, pct, relativeTime, tokenCount } from "@/lib/fmt";
import { ensureArray, getApiBaseUrl, getPoolHealth, maskToken } from "@/lib/lune";
import { matchPath, usePathname, useRouter } from "@/lib/router";
import type {
  Account,
  Overview,
  PoolDetailResponse,
  PoolMember,
  RevealedAccessToken,
} from "@/lib/types";

export default function PoolDetailPage() {
  const pathname = usePathname();
  const { navigate } = useRouter();
  const { openAddAccount, dataVersion, refreshData } = useAdminUI();
  const params = matchPath("/admin/pools/:id", pathname);
  const poolId = Number(params?.id);
  const hasValidPoolId = Number.isInteger(poolId) && poolId > 0;
  const [detail, setDetail] = useState<PoolDetailResponse | null>(null);
  const [overview, setOverview] = useState<Overview | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [snippetsOpen, setSnippetsOpen] = useState(false);
  const [qrOpen, setQrOpen] = useState(false);
  const [modelsAccount, setModelsAccount] = useState<Account | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<PoolMember | null>(null);
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
    setConnectTokenValue(null);
    setRevealedTokenCache({});
    setRevealedGlobalToken(null);
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
  const baseUrl = getApiBaseUrl();
  const poolTokens = ensureArray(detail?.tokens);
  const poolModels = ensureArray(detail?.models);
  const statsByAccount = ensureArray(stats?.by_account);
  const primaryPoolTokenId = poolTokens.find((token) => token.enabled)?.id ?? null;
  const accountRequestMap = useMemo(() => {
    const map = new Map<number, number>();
    statsByAccount.forEach((row) => {
      map.set(row.account_id, row.requests);
    });
    return map;
  }, [statsByAccount]);
  const poolToken = connectTokenValue || revealedGlobalToken || "";
  const health = pool ? getPoolHealth(pool) : "degraded";

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

  async function openSnippets() {
    try {
      await getTokenForConnect();
      setSnippetsOpen(true);
    } catch (err) {
      toast(err instanceof Error ? err.message : "读取 Pool Token 失败", "error");
    }
  }

  async function openQr() {
    try {
      await getTokenForConnect();
      setQrOpen(true);
    } catch (err) {
      toast(err instanceof Error ? err.message : "读取 Pool Token 失败", "error");
    }
  }

  if (loading) {
    return (
      <div className="space-y-8">
        <Skeleton className="h-36 rounded-[2rem]" />
        <Skeleton className="h-80 rounded-[1.8rem]" />
        <Skeleton className="h-64 rounded-[1.8rem]" />
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
        eyebrow={`Pool / ${pool.label}`}
        title={`${pool.label} 还没有账号。`}
        description="先往这个 Pool 添加账号，之后它会自动参与模型发现、优先级排序和内嵌测试。"
        action={<Button onClick={() => openAddAccount(pool.id)}><Plus className="size-4" />添加账号</Button>}
      />
    );
  }

  return (
    <div className="space-y-8">
      <PageHeader
        eyebrow={`Pool / ${pool.label}`}
        title={pool.label}
        description="账号卡片顺序即路由优先级。把常用账号拖到前面，把暂时不用的拖进禁用区。"
        actions={
          <>
            <Button variant="outline" onClick={openSnippets}>
              <KeyRound className="size-4" />
              Env Snippets
            </Button>
            <Button variant="outline" onClick={openQr}>
              <QrCode className="size-4" />
              QR 码
            </Button>
            <Button onClick={() => openAddAccount(pool.id)}>
              <Plus className="size-4" />
              Add Account
            </Button>
          </>
        }
        meta={
          <>
            <span>状态 {health}</span>
            <span>{pool.account_count} 账号</span>
            <span>{pool.routable_account_count} 可用</span>
            <span>{poolModels.length} 个模型</span>
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
            key={member.id}
            member={member}
            dragging={options.dragging}
            requests={accountRequestMap.get(member.account_id) ?? 0}
            resolveToken={getTokenForConnect}
            onToggleEnabled={() => toggleMember(member, !member.enabled)}
            onDelete={() => setDeleteTarget(member)}
            onRefreshModels={() => refreshModels(member)}
            onViewModels={() => setModelsAccount(member.account ?? null)}
          />
        )}
      />

      <section className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_minmax(18rem,22rem)]">
        <div className="surface-section px-5 py-5">
          <SectionHeading
            title="Pool Tokens"
            description="这里只保留归属感知，完整管理已收拢到 Settings。"
            action={
              <Button variant="outline" onClick={() => navigate("/admin/settings")}>
                在 Settings 中统一管理
              </Button>
            }
          />
          <div className="mt-5 space-y-3">
            {poolTokens.length === 0 ? (
              <div className="rounded-[1.25rem] border border-dashed border-moon-200/70 px-4 py-5 text-sm text-moon-400">
                还没有 Pool Token。创建与启停请前往 Settings 统一管理。
              </div>
            ) : (
              poolTokens.map((token) => (
                <div key={token.id} className="surface-outline flex items-center justify-between gap-4 px-4 py-4">
                  <div className="min-w-0">
                    <p className="text-sm font-semibold text-moon-800">{token.name}</p>
                    <p className="mt-1 break-all text-sm text-moon-500">
                      {maskToken(token.token || token.token_masked)}
                    </p>
                    <p className="mt-1 text-xs text-moon-400">
                      最后使用 {relativeTime(token.last_used_at)} · {token.enabled ? "启用中" : "已停用"}
                    </p>
                  </div>
                  <span className="text-xs text-moon-400">只读快捷入口</span>
                </div>
              ))
            )}
          </div>
        </div>

        <div className="surface-section px-5 py-5">
          <SectionHeading title="Pool Snapshot" description="最近 24 小时的请求与模型视图。" />
          <div className="mt-5 space-y-4 text-sm text-moon-500">
            <div className="surface-outline px-4 py-4">
              <p className="text-xs uppercase tracking-[0.18em] text-moon-400">Models</p>
              <p className="mt-2 leading-7 text-moon-700">
                {poolModels.join(", ") || "等待模型发现"}
              </p>
            </div>
            <div className="surface-outline px-4 py-4">
              <p className="text-xs uppercase tracking-[0.18em] text-moon-400">Top Account</p>
              <div className="mt-2 space-y-2">
                {statsByAccount.slice(0, 3).map((item) => (
                  <div key={item.account_id} className="flex items-center justify-between">
                    <span>{item.account_label}</span>
                    <span>{compact(item.requests)} req</span>
                  </div>
                ))}
              </div>
            </div>
            <div className="surface-outline px-4 py-4">
              <p className="text-xs uppercase tracking-[0.18em] text-moon-400">Usage</p>
              <p className="mt-2">输入 {tokenCount(stats?.total_input_tokens ?? 0)}</p>
              <p>输出 {tokenCount(stats?.total_output_tokens ?? 0)}</p>
            </div>
          </div>
        </div>
      </section>

      <EnvSnippetsDialog
        open={snippetsOpen}
        onOpenChange={setSnippetsOpen}
        title={`${pool.label} Env Snippets`}
        baseUrl={baseUrl}
        token={poolToken}
        model={poolModels[0]}
      />
      <QrCodeDialog
        open={qrOpen}
        onOpenChange={setQrOpen}
        title={`${pool.label} QR`}
        baseUrl={baseUrl}
        token={poolToken}
      />
      <Dialog open={Boolean(modelsAccount)} onOpenChange={(open) => !open && setModelsAccount(null)}>
        <DialogContent className="max-w-2xl rounded-[1.6rem] border border-white/75 bg-white/95">
          <DialogHeader>
            <DialogTitle>{modelsAccount?.label} 模型列表</DialogTitle>
          </DialogHeader>
          <div className="flex flex-wrap gap-2">
            {ensureArray(modelsAccount?.models).map((model) => (
              <span key={model} className="rounded-full bg-moon-100/80 px-3 py-1 text-sm text-moon-600">
                {model}
              </span>
            ))}
          </div>
        </DialogContent>
      </Dialog>
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
