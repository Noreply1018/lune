import {
  useRef,
  useEffect,
  useMemo,
  useState,
  type KeyboardEvent,
  type ReactNode,
} from "react";
import {
  CircleDot,
  Copy,
  Eye,
  EyeOff,
  MoreHorizontal,
  PencilLine,
  Plus,
  RefreshCw,
  Trash2,
  WandSparkles,
} from "lucide-react";
import ConfirmDialog from "@/components/ConfirmDialog";
import ErrorState from "@/components/ErrorState";
import PageHeader from "@/components/PageHeader";
import SectionHeading from "@/components/SectionHeading";
import SideTOC, { type TOCSection } from "@/components/SideTOC";
import { toast } from "@/components/Feedback";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { api } from "@/lib/api";
import { shortDate } from "@/lib/fmt";
import { maskToken } from "@/lib/lune";
import type {
  AccessToken,
  CpaService,
  DataRetentionSummary,
  Pool,
  RevealedAccessToken,
  SystemSettings,
} from "@/lib/types";
import { cn } from "@/lib/utils";
import NotificationsSection from "./SettingsPage/notifications/NotificationsSection";
import NotificationHistorySection from "./SettingsPage/notifications/NotificationHistorySection";
import DataRetentionSection from "./SettingsPage/data-retention/DataRetentionSection";
import ConfigTransferSection from "./SettingsPage/config-transfer/ConfigTransferSection";
import SystemSection from "./SettingsPage/system/SystemSection";

type EditableSettingField =
  | "request_timeout"
  | "max_retry_attempts"
  | "health_check_interval"
  | "data_retention_days";

type TokenDraft = {
  name: string;
  scope: "global" | "pool";
  poolId: string;
};

const INITIAL_TOKEN_DRAFT: TokenDraft = {
  name: "",
  scope: "global",
  poolId: "",
};

const TOKEN_GRID_COLUMNS =
  "xl:grid-cols-[minmax(10rem,1fr)_minmax(0,4.8fr)_9.5rem_6.25rem_6.25rem_11.5rem]";
const TOKEN_COLUMN_LABELS = ["名称", "Token", "创建时间", "状态", "归属"];

const SETTINGS_SECTIONS: TOCSection[] = [
  { id: "settings-gateway", label: "Gateway" },
  { id: "notifications", label: "Notifications" },
  { id: "token-management", label: "Tokens" },
  { id: "data-retention", label: "Retention" },
  { id: "system", label: "System" },
  { id: "config-transfer", label: "Transfer" },
  { id: "notification-history", label: "History" },
];

export default function SettingsPage() {
  const [service, setService] = useState<CpaService | null>(null);
  const [settings, setSettings] = useState<SystemSettings | null>(null);
  const [tokens, setTokens] = useState<AccessToken[]>([]);
  const [pools, setPools] = useState<Pool[]>([]);
  const [retentionSummary, setRetentionSummary] =
    useState<DataRetentionSummary | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [gatewayForm, setGatewayForm] = useState({
    request_timeout: 120,
    max_retry_attempts: 3,
  });
  const [systemForm, setSystemForm] = useState({ health_check_interval: 60 });
  const [savingField, setSavingField] = useState<EditableSettingField | null>(
    null,
  );
  const settingsRef = useRef(settings);
  const [createOpen, setCreateOpen] = useState(false);
  const [createDraft, setCreateDraft] =
    useState<TokenDraft>(INITIAL_TOKEN_DRAFT);
  const [editingToken, setEditingToken] = useState<AccessToken | null>(null);
  const [editingName, setEditingName] = useState("");
  const [deleteToken, setDeleteToken] = useState<AccessToken | null>(null);
  const [regenerateToken, setRegenerateToken] = useState<AccessToken | null>(
    null,
  );
  const [revealedTokens, setRevealedTokens] = useState<Record<number, string>>(
    {},
  );
  const [visibleTokenIds, setVisibleTokenIds] = useState<number[]>([]);
  const [highlightedTokenId, setHighlightedTokenId] = useState<number | null>(null);
  const [testingService, setTestingService] = useState(false);

  async function loadRetentionSummary() {
    const summary = await api.get<DataRetentionSummary>(
      "/settings/data-retention",
    );
    setRetentionSummary(summary);
    return summary;
  }

  function load(silent = false) {
    if (!silent) setLoading(true);
    setError(null);
    return Promise.all([
      api.get<CpaService | null>("/cpa/service"),
      api.get<SystemSettings>("/settings"),
      api.get<AccessToken[]>("/tokens"),
      api.get<Pool[]>("/pools"),
      loadRetentionSummary(),
    ])
      .then(([serviceData, settingsData, tokenData, poolData]) => {
        setService(serviceData);
        setSettings(settingsData);
        setTokens(tokenData ?? []);
        setPools(poolData ?? []);
        setGatewayForm({
          request_timeout: settingsData.request_timeout,
          max_retry_attempts: settingsData.max_retry_attempts,
        });
        setSystemForm({
          health_check_interval: settingsData.health_check_interval,
        });
      })
      .catch((err) =>
        setError(err instanceof Error ? err.message : "Settings 加载失败"),
      )
      .finally(() => setLoading(false));
  }

  useEffect(() => {
    load();
  }, []);

  useEffect(() => {
    settingsRef.current = settings;
  }, [settings]);

  // Honor `#access-token-<id>` and `#token-management` deep links so other
  // pages (PoolDetailPage's disabled-token jump) can land us on the right row.
  // Scroll is owned here because the sender unmounts as soon as the route
  // changes — a retry scheduled in the sender would get cleaned up before
  // the token DOM even exists. Scroll target is the Token Management section
  // itself so the section heading stays visible above the highlighted row.
  const highlightTimerRef = useRef<number | null>(null);
  const scrollRetryRef = useRef<number | null>(null);
  // Per-field monotonic sequence so a slow PUT response can never overwrite
  // state from a newer save against the same field.
  const saveSeqRef = useRef<Record<string, number>>({});
  useEffect(() => {
    function handleHash() {
      const hash = window.location.hash;
      const tokenMatch = hash.match(/^#access-token-(\d+)$/);
      const tokenId = tokenMatch ? Number(tokenMatch[1]) : null;
      const wantsTokens = hash === "#token-management" || tokenId != null;
      const wantsHistory = hash === "#notification-history";
      if (!wantsTokens && !wantsHistory) return;

      if (tokenId != null) {
        setHighlightedTokenId(tokenId);
        if (highlightTimerRef.current) window.clearTimeout(highlightTimerRef.current);
        highlightTimerRef.current = window.setTimeout(() => {
          setHighlightedTokenId(null);
          highlightTimerRef.current = null;
        }, 2600);
      }

      const targetId = wantsHistory ? "notification-history" : "token-management";
      if (scrollRetryRef.current) window.clearTimeout(scrollRetryRef.current);
      let attempts = 0;
      const tryScroll = () => {
        scrollRetryRef.current = null;
        const section = document.getElementById(targetId);
        if (section) {
          section.scrollIntoView({ behavior: "smooth", block: "start" });
          return;
        }
        if (attempts < 20) {
          attempts += 1;
          scrollRetryRef.current = window.setTimeout(tryScroll, 60);
        }
      };
      tryScroll();
    }
    handleHash();
    window.addEventListener("hashchange", handleHash);
    return () => {
      window.removeEventListener("hashchange", handleHash);
      if (highlightTimerRef.current) window.clearTimeout(highlightTimerRef.current);
      if (scrollRetryRef.current) window.clearTimeout(scrollRetryRef.current);
    };
    // Re-run once loading flips to false: the #token-management section is
    // gated on !loading, so a cold mount with a hash attempts scrolling
    // against a skeleton-only DOM and the 1.2s retry window can lapse before
    // data arrives on slower networks. Re-triggering on loading transitions
    // guarantees the scroll fires against the real section.
  }, [loading]);

  const poolNameMap = useMemo(
    () => Object.fromEntries(pools.map((pool) => [pool.id, pool.label])),
    [pools],
  );
  const globalTokens = useMemo(
    () => tokens.filter((token) => token.pool_id == null),
    [tokens],
  );
  const poolTokens = useMemo(
    () => tokens.filter((token) => token.pool_id != null),
    [tokens],
  );

  async function saveSetting(
    field: EditableSettingField,
    value: number | string,
  ) {
    const payloadValue =
      typeof value === "number"
        ? Number.isFinite(value)
          ? value
          : 0
        : value.trim();
    const prevSettings = settingsRef.current;
    const seq = (saveSeqRef.current[field] ?? 0) + 1;
    saveSeqRef.current[field] = seq;
    setSavingField(field);
    try {
      await api.put("/settings", { [field]: payloadValue });
      // Drop the response if a newer save against the same field has
      // already started — its result must win.
      if (saveSeqRef.current[field] !== seq) return;
      setSettings((current) =>
        current ? { ...current, [field]: payloadValue } : current,
      );
      if (field === "data_retention_days") {
        await loadRetentionSummary();
      }
      toast("设置已更新");
    } catch (err) {
      if (saveSeqRef.current[field] !== seq) return;
      toast(err instanceof Error ? err.message : "保存设置失败", "error");
      if (prevSettings) {
        if (field === "health_check_interval") {
          setSystemForm((current) => ({
            ...current,
            health_check_interval: prevSettings.health_check_interval,
          }));
        }
        if (field === "request_timeout" || field === "max_retry_attempts") {
          setGatewayForm((current) => ({
            ...current,
            [field]: prevSettings[field],
          }));
        }
        if (field === "data_retention_days") {
          setSettings((current) =>
            current
              ? {
                  ...current,
                  data_retention_days: prevSettings.data_retention_days,
                }
              : current,
          );
        }
      }
    } finally {
      // Keep the spinner up while a newer save is still in flight.
      if (saveSeqRef.current[field] === seq) {
        setSavingField(null);
      }
    }
  }

  async function revealTokenValue(token: AccessToken): Promise<string> {
    const cached = revealedTokens[token.id];
    if (cached) {
      return cached;
    }
    const revealed = await api.post<RevealedAccessToken>(
      `/tokens/${token.id}/reveal`,
    );
    setRevealedTokens((current) => ({
      ...current,
      [token.id]: revealed.token,
    }));
    return revealed.token;
  }

  async function copyToken(token: AccessToken) {
    try {
      const value = await revealTokenValue(token);
      await navigator.clipboard.writeText(value);
      toast("已复制");
    } catch (err) {
      toast(err instanceof Error ? err.message : "复制 Token 失败", "error");
    }
  }

  async function toggleReveal(token: AccessToken) {
    const isVisible = visibleTokenIds.includes(token.id);
    if (isVisible) {
      setVisibleTokenIds((current) => current.filter((id) => id !== token.id));
      return;
    }
    try {
      await revealTokenValue(token);
      setVisibleTokenIds((current) => [...current, token.id]);
    } catch (err) {
      toast(err instanceof Error ? err.message : "读取 Token 失败", "error");
    }
  }

  async function toggleEnabled(token: AccessToken) {
    const nextEnabled = !token.enabled;
    setTokens((current) =>
      current.map((item) =>
        item.id === token.id ? { ...item, enabled: nextEnabled } : item,
      ),
    );
    try {
      await api.post(
        `/tokens/${token.id}/${token.enabled ? "disable" : "enable"}`,
      );
      toast(nextEnabled ? "Token 已启用" : "Token 已停用");
    } catch (err) {
      setTokens((current) =>
        current.map((item) =>
          item.id === token.id ? { ...item, enabled: token.enabled } : item,
        ),
      );
      toast(
        err instanceof Error ? err.message : "更新 Token 状态失败",
        "error",
      );
    }
  }

  async function submitCreateToken() {
    if (!createDraft.name.trim()) {
      toast("请先填写 Token 名称", "error");
      return;
    }
    const poolId =
      createDraft.scope === "pool" ? Number(createDraft.poolId) : null;
    if (createDraft.scope === "pool" && !poolId) {
      toast("请选择归属 Pool", "error");
      return;
    }
    try {
      const created = await api.post<RevealedAccessToken>("/tokens", {
        name: createDraft.name.trim(),
        pool_id: poolId,
        enabled: true,
      });
      const now = new Date().toISOString();
      const { token: revealedTokenValue, ...createdTokenData } = created;
      const createdToken: AccessToken = {
        ...createdTokenData,
        token_masked: maskToken(revealedTokenValue),
        pool_label:
          created.pool_id != null ? poolNameMap[created.pool_id] : undefined,
        is_global: created.pool_id == null,
        created_at: now,
        updated_at: now,
        last_used_at: null,
      };
      setTokens((current) => [createdToken, ...current]);
      setCreateOpen(false);
      setCreateDraft(INITIAL_TOKEN_DRAFT);
      setRevealedTokens((current) => ({
        ...current,
        [created.id]: revealedTokenValue,
      }));
      setVisibleTokenIds((current) =>
        Array.from(new Set([...current, created.id])),
      );
      toast("Token 已创建");
    } catch (err) {
      toast(err instanceof Error ? err.message : "创建 Token 失败", "error");
    }
  }

  async function submitRename() {
    if (!editingToken) {
      return;
    }
    const name = editingName.trim();
    if (!name) {
      toast("名称不能为空", "error");
      return;
    }

    const previousName = editingToken.name;
    setTokens((current) =>
      current.map((token) =>
        token.id === editingToken.id ? { ...token, name } : token,
      ),
    );

    try {
      await api.put(`/tokens/${editingToken.id}`, {
        name,
        pool_id: editingToken.pool_id,
        enabled: editingToken.enabled,
      });
      toast("名称已更新");
      setEditingToken(null);
      setEditingName("");
    } catch (err) {
      setTokens((current) =>
        current.map((token) =>
          token.id === editingToken.id
            ? { ...token, name: previousName }
            : token,
        ),
      );
      toast(err instanceof Error ? err.message : "更新名称失败", "error");
    }
  }

  async function confirmRegenerateToken() {
    if (!regenerateToken) {
      return;
    }
    try {
      const revealed = await api.post<RevealedAccessToken>(
        `/tokens/${regenerateToken.id}/regenerate`,
        {},
      );
      setRevealedTokens((current) => ({
        ...current,
        [revealed.id]: revealed.token,
      }));
      setVisibleTokenIds((current) =>
        Array.from(new Set([...current, revealed.id])),
      );
      setTokens((current) =>
        current.map((token) =>
          token.id === revealed.id
            ? {
                ...token,
                token_masked: maskToken(revealed.token),
                updated_at: new Date().toISOString(),
              }
            : token,
        ),
      );
      toast("Token 已重新生成");
      setRegenerateToken(null);
    } catch (err) {
      toast(err instanceof Error ? err.message : "重新生成失败", "error");
    }
  }

  async function confirmDeleteToken() {
    if (!deleteToken) {
      return;
    }
    const removed = deleteToken;
    const prevVisible = visibleTokenIds.includes(removed.id);
    const prevRevealed = revealedTokens[removed.id];
    setTokens((current) => current.filter((token) => token.id !== removed.id));
    setVisibleTokenIds((current) => current.filter((id) => id !== removed.id));
    setRevealedTokens((current) => {
      const next = { ...current };
      delete next[removed.id];
      return next;
    });
    try {
      await api.delete(`/tokens/${removed.id}`);
      toast("Token 已删除");
      setDeleteToken(null);
    } catch (err) {
      setTokens((current) => {
        const existingIndex = current.findIndex(
          (token) => token.id === removed.id,
        );
        if (existingIndex >= 0) {
          return current;
        }
        return [removed, ...current];
      });
      if (prevVisible) {
        setVisibleTokenIds((current) =>
          current.includes(removed.id) ? current : [...current, removed.id],
        );
      }
      if (prevRevealed !== undefined) {
        setRevealedTokens((current) => ({
          ...current,
          [removed.id]: prevRevealed,
        }));
      }
      setDeleteToken(null);
      toast(err instanceof Error ? err.message : "删除 Token 失败", "error");
    }
  }

  async function testService() {
    if (!service) {
      return;
    }
    setTestingService(true);
    try {
      const result = await api.post<{
        reachable: boolean;
        latency_ms: number;
        error: string;
      }>("/cpa/service/test", {});
      toast(
        result.reachable
          ? `连接正常 ${result.latency_ms}ms`
          : result.error || "连接失败",
        result.reachable ? "success" : "error",
      );
      const serviceData = await api.get<CpaService | null>("/cpa/service");
      setService(serviceData);
    } catch (err) {
      toast(err instanceof Error ? err.message : "测试失败", "error");
    } finally {
      setTestingService(false);
    }
  }

  if (loading) {
    return (
      <div className="space-y-10">
        <Skeleton className="h-32 rounded-[2rem]" />
        <div className="grid gap-6 lg:grid-cols-2">
          <Skeleton className="h-44 rounded-[1.8rem]" />
          <Skeleton className="h-44 rounded-[1.8rem]" />
        </div>
        <Skeleton className="h-[28rem] rounded-[1.8rem]" />
        <Skeleton className="h-40 rounded-[1.8rem]" />
      </div>
    );
  }

  if (error) {
    return <ErrorState message={error} onRetry={load} />;
  }

  return (
    <div className="space-y-12 pb-8">
      <SideTOC sections={SETTINGS_SECTIONS} ready={!loading} />
      <PageHeader
        title="Settings"
        description="管理网关行为、访问凭证与系统连接。"
      />

      <section
        id="settings-gateway"
        className="scroll-mt-6 grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)] lg:items-stretch"
      >
        <div className="surface-section flex h-full flex-col px-5 py-5 sm:px-6">
          <SectionHeading
            title="Gateway Behavior"
            description="控制请求超时与重试行为。"
          />
          <div className="mt-4 flex-1 divide-y divide-moon-200/30">
            <SettingsNumericRow
              label="Request Timeout"
              value={gatewayForm.request_timeout}
              suffix="秒"
              min={1}
              saving={savingField === "request_timeout"}
              onCommit={(value) => {
                setGatewayForm((current) => ({
                  ...current,
                  request_timeout: value,
                }));
                void saveSetting("request_timeout", value);
              }}
            />
            <SettingsNumericRow
              label="Max Retry Attempts"
              value={gatewayForm.max_retry_attempts}
              suffix="次"
              min={1}
              saving={savingField === "max_retry_attempts"}
              onCommit={(value) => {
                setGatewayForm((current) => ({
                  ...current,
                  max_retry_attempts: value,
                }));
                void saveSetting("max_retry_attempts", value);
              }}
            />
          </div>
        </div>

        <div className="surface-section flex h-full flex-col px-5 py-5 sm:px-6">
          <SectionHeading
            title="CPA Service"
            description="查看当前 CPA 通道状态。"
            action={
              service ? (
                <Button
                  variant="outline"
                  size="sm"
                  className="rounded-full"
                  onClick={testService}
                  disabled={testingService}
                >
                  {testingService ? (
                    <RefreshCw className="size-4 animate-spin" />
                  ) : (
                    <CircleDot className="size-4" />
                  )}
                  Test Connection
                </Button>
              ) : undefined
            }
          />
          <div className="mt-4 flex flex-1 flex-col gap-4">
            {service ? (
              <div className="grid gap-x-6 gap-y-4 sm:grid-cols-2">
                <InfoBlock
                  label="Status"
                  value={
                    <StatusBadge ok={service.status === "healthy"}>
                      {service.status === "healthy" ? "Healthy" : "Error"}
                    </StatusBadge>
                  }
                />
                <InfoBlock label="Label" value={service.label || "--"} />
                <InfoBlock label="Base URL" value={service.base_url || "--"} />
                <InfoBlock
                  label="Last Checked"
                  value={
                    service.last_checked_at
                      ? shortDate(service.last_checked_at)
                      : "尚未检查"
                  }
                />
              </div>
            ) : (
              <div className="flex h-full flex-col gap-3 rounded-[1.35rem] border border-dashed border-moon-200/55 px-4 py-4 text-sm text-moon-500">
                <p>请通过环境变量完成 CPA 配置</p>
              </div>
            )}
          </div>
        </div>
      </section>

      <div id="notifications" className="scroll-mt-6">
        <NotificationsSection
          initialExpiringDays={settings?.notification_expiring_days ?? 7}
          onExpiringDaysChange={(value) =>
            setSettings((current) =>
              current
                ? { ...current, notification_expiring_days: value }
                : current,
            )
          }
        />
      </div>

      <section id="token-management" className="surface-section px-5 py-5 sm:px-6 scroll-mt-6">
        <SectionHeading
          title="Token Management"
          description="集中管理全局与 Pool 访问凭证。"
          action={
            <Button onClick={() => setCreateOpen(true)}>
              <Plus className="size-4" />
              Create Token
            </Button>
          }
        />
        <div className="mt-7 space-y-8">
          <TokenGroup
            title="Global Tokens"
            tokens={globalTokens}
            emptyText="还没有 Global Token。"
            poolNameMap={poolNameMap}
            revealedTokens={revealedTokens}
            visibleTokenIds={visibleTokenIds}
            highlightedTokenId={highlightedTokenId}
            onCopy={copyToken}
            onReveal={toggleReveal}
            onEdit={(token) => {
              setEditingToken(token);
              setEditingName(token.name);
            }}
            onToggleEnabled={toggleEnabled}
            onRegenerate={setRegenerateToken}
            onDelete={setDeleteToken}
          />
          <TokenGroup
            title="Pool Tokens"
            tokens={poolTokens}
            emptyText="还没有 Pool Token。"
            poolNameMap={poolNameMap}
            revealedTokens={revealedTokens}
            visibleTokenIds={visibleTokenIds}
            highlightedTokenId={highlightedTokenId}
            onCopy={copyToken}
            onReveal={toggleReveal}
            onEdit={(token) => {
              setEditingToken(token);
              setEditingName(token.name);
            }}
            onToggleEnabled={toggleEnabled}
            onRegenerate={setRegenerateToken}
            onDelete={setDeleteToken}
          />
        </div>
      </section>

      <div id="data-retention" className="scroll-mt-6">
        <DataRetentionSection
          retentionDays={settings?.data_retention_days ?? 0}
          savingRetention={savingField === "data_retention_days"}
          onRetentionDaysCommit={(value) => {
            setSettings((current) =>
              current ? { ...current, data_retention_days: value } : current,
            );
            void saveSetting("data_retention_days", value);
          }}
          summary={retentionSummary}
          onReloadSummary={async () => {
            await loadRetentionSummary();
          }}
        />
      </div>

      <div id="system" className="scroll-mt-6">
        <SystemSection
          healthCheckInterval={systemForm.health_check_interval}
          saving={savingField === "health_check_interval"}
          onCommit={(value) => {
            setSystemForm({ health_check_interval: value });
            void saveSetting("health_check_interval", value);
          }}
        />
      </div>

      <div id="config-transfer" className="scroll-mt-6">
        <ConfigTransferSection onImported={() => load(true)} />
      </div>

      <NotificationHistorySection />

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="max-w-md overflow-hidden rounded-[1.6rem] border border-white/75 bg-white/95 p-0 shadow-[0_26px_70px_-38px_rgba(74,68,108,0.34)]">
          <DialogHeader className="border-b border-moon-200/55 px-6 py-5 pr-12">
            <DialogTitle>Create Token</DialogTitle>
            <DialogDescription>选择归属并创建新的访问凭证。</DialogDescription>
          </DialogHeader>
          <div className="space-y-5 px-6 py-6">
            <div className="space-y-2">
              <label className="text-sm font-medium text-moon-700">Name</label>
              <Input
                value={createDraft.name}
                onChange={(event) =>
                  setCreateDraft((current) => ({
                    ...current,
                    name: event.target.value,
                  }))
                }
                placeholder="例如：global-cli"
              />
            </div>
            <div className="space-y-2">
              <p className="text-sm font-medium text-moon-700">Ownership</p>
              <div className="flex flex-wrap gap-2">
                <button
                  type="button"
                  className={cn(
                    "rounded-full border px-3 py-1.5 text-sm transition-colors",
                    createDraft.scope === "global"
                      ? "border-lunar-300/65 bg-lunar-100/60 text-moon-800 shadow-[inset_0_1px_0_rgba(255,255,255,0.45)]"
                      : "border-moon-200/65 bg-white/60 text-moon-500 hover:border-moon-250/75 hover:bg-white/80",
                  )}
                  onClick={() =>
                    setCreateDraft((current) => ({
                      ...current,
                      scope: "global",
                      poolId: "",
                    }))
                  }
                >
                  Global
                </button>
                <button
                  type="button"
                  className={cn(
                    "rounded-full border px-3 py-1.5 text-sm transition-colors",
                    createDraft.scope === "pool"
                      ? "border-lunar-300/65 bg-lunar-100/60 text-moon-800 shadow-[inset_0_1px_0_rgba(255,255,255,0.45)]"
                      : "border-moon-200/65 bg-white/60 text-moon-500 hover:border-moon-250/75 hover:bg-white/80",
                  )}
                  onClick={() =>
                    setCreateDraft((current) => ({ ...current, scope: "pool" }))
                  }
                >
                  指定 Pool
                </button>
              </div>
            </div>
            {createDraft.scope === "pool" ? (
              <div className="space-y-2">
                <label className="text-sm font-medium text-moon-700">
                  Pool
                </label>
                <Select
                  value={createDraft.poolId}
                  onValueChange={(value) =>
                    setCreateDraft((current) => ({
                      ...current,
                      poolId: value ?? "",
                    }))
                  }
                >
                  <SelectTrigger className="h-9 w-full rounded-lg border-moon-200/65 bg-white/72 px-3 text-sm text-moon-700 shadow-[inset_0_1px_0_rgba(255,255,255,0.52)] hover:border-moon-250/80 hover:bg-white/82 focus-visible:border-lunar-300/70 focus-visible:ring-lunar-200/45">
                    <SelectValue placeholder="选择 Pool" />
                  </SelectTrigger>
                  <SelectContent
                    sideOffset={2}
                    align="start"
                    className="w-(--anchor-width) rounded-[1rem] border border-moon-200/70 bg-white/95 p-1 shadow-[0_20px_44px_-28px_rgba(74,68,108,0.34)]"
                  >
                    {pools.map((pool) => (
                      <SelectItem
                        key={pool.id}
                        value={String(pool.id)}
                        className="rounded-[0.8rem] px-3 py-2 text-sm text-moon-700 focus:bg-lunar-100/80 focus:text-moon-800 data-[selected]:bg-lunar-100/70 data-[selected]:text-moon-800"
                      >
                        {pool.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            ) : null}
          </div>
          <div className="flex items-center justify-end gap-3 border-t border-moon-200/55 bg-white/72 px-6 py-4">
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              取消
            </Button>
            <Button onClick={() => void submitCreateToken()}>
              Create Token
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      <Dialog
        open={Boolean(editingToken)}
        onOpenChange={(open) => !open && setEditingToken(null)}
      >
        <DialogContent className="max-w-md rounded-[1.6rem] border border-white/75 bg-white/95 p-0">
          <DialogHeader className="border-b border-moon-200/55 px-6 py-5">
            <DialogTitle>Edit Token Name</DialogTitle>
            <DialogDescription>只更新名称。</DialogDescription>
          </DialogHeader>
          <div className="px-6 py-6">
            <Input
              value={editingName}
              onChange={(event) => setEditingName(event.target.value)}
            />
          </div>
          <DialogFooter className="border-t border-moon-200/55 bg-white/76 px-6 py-4">
            <Button variant="outline" onClick={() => setEditingToken(null)}>
              取消
            </Button>
            <Button onClick={() => void submitRename()}>保存</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={Boolean(regenerateToken)}
        onOpenChange={(open) => !open && setRegenerateToken(null)}
        title="重新生成 Token"
        description={`重新生成后，${regenerateToken?.name ?? "该 Token"} 的旧值会立即失效。`}
        confirmLabel="重新生成"
        variant="default"
        onConfirm={confirmRegenerateToken}
      />

      <ConfirmDialog
        open={Boolean(deleteToken)}
        onOpenChange={(open) => !open && setDeleteToken(null)}
        title="删除 Token"
        description={`删除后，${deleteToken?.name ?? "该 Token"} 将不再可用。`}
        onConfirm={confirmDeleteToken}
      />

    </div>
  );
}

function SettingsNumericRow({
  label,
  value,
  onCommit,
  suffix,
  helper = "实时生效",
  saving,
  min = 0,
}: {
  label: string;
  value: number;
  onCommit: (value: number) => void;
  suffix?: string;
  helper?: string;
  saving?: boolean;
  min?: number;
}) {
  const [draft, setDraft] = useState(`${value}`);
  useEffect(() => {
    setDraft(`${value}`);
  }, [value]);

  function commit() {
    const trimmed = draft.trim();
    const parsed = Number(trimmed);
    if (trimmed === "" || !Number.isFinite(parsed) || parsed < min) {
      // Empty / NaN / below floor: roll back display rather than committing
      // a sentinel value that the server will just bounce.
      setDraft(`${value}`);
      return;
    }
    const normalized = Math.floor(parsed);
    setDraft(`${normalized}`);
    if (normalized === value) return;
    onCommit(normalized);
  }

  function handleKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "Enter") {
      event.preventDefault();
      event.currentTarget.blur();
    }
  }

  return (
    <div className="flex flex-col gap-3 py-3 sm:flex-row sm:items-center sm:justify-between">
      <div className="space-y-0.5">
        <p className="text-sm font-medium text-moon-800">{label}</p>
        <p className="text-xs text-moon-350">{helper}</p>
      </div>
      <div className="flex items-center gap-2 self-start sm:self-auto">
        <Input
          type="number"
          value={draft}
          min={min}
          className="h-9 w-24 text-right"
          onChange={(event) => setDraft(event.target.value)}
          onBlur={commit}
          onKeyDown={handleKeyDown}
        />
        <span className="w-5 text-sm text-moon-350">{suffix ?? ""}</span>
        {saving ? (
          <RefreshCw className="size-4 animate-spin text-moon-350" />
        ) : (
          <span className="size-4" />
        )}
      </div>
    </div>
  );
}

function TokenGroup({
  title,
  tokens,
  emptyText,
  poolNameMap,
  revealedTokens,
  visibleTokenIds,
  highlightedTokenId,
  onCopy,
  onReveal,
  onEdit,
  onToggleEnabled,
  onRegenerate,
  onDelete,
}: {
  title: string;
  tokens: AccessToken[];
  emptyText?: string;
  poolNameMap: Record<number, string>;
  revealedTokens: Record<number, string>;
  visibleTokenIds: number[];
  highlightedTokenId: number | null;
  onCopy: (token: AccessToken) => void;
  onReveal: (token: AccessToken) => void;
  onEdit: (token: AccessToken) => void;
  onToggleEnabled: (token: AccessToken) => void;
  onRegenerate: (token: AccessToken) => void;
  onDelete: (token: AccessToken) => void;
}) {
  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-3">
        <p className="text-sm font-medium text-moon-800">{title}</p>
        <p className="text-xs text-moon-350">{tokens.length} tokens</p>
      </div>
      {tokens.length === 0 ? (
        <div className="border-y border-moon-200/30 py-5">
          <p className="text-sm text-moon-400">
            {emptyText ?? "当前分组为空。"}
          </p>
        </div>
      ) : (
        <div className="border-y border-moon-200/30">
          <div
            className={cn(
              "hidden border-b border-moon-200/20 py-2.5 xl:grid xl:gap-x-4",
              TOKEN_GRID_COLUMNS,
            )}
          >
            {TOKEN_COLUMN_LABELS.map((label) => (
              <p
                key={label}
                className="text-[11px] font-medium tracking-[0.16em] text-moon-300"
              >
                {label}
              </p>
            ))}
            <p className="text-[11px] font-medium tracking-[0.16em] text-moon-300">
              操作
            </p>
          </div>
          {tokens.map((token) => (
            <TokenRow
              key={token.id}
              token={token}
              ownerLabel={
                token.pool_id
                  ? (poolNameMap[token.pool_id] ??
                    token.pool_label ??
                    `Pool #${token.pool_id}`)
                  : "Global"
              }
              visible={visibleTokenIds.includes(token.id)}
              revealedValue={revealedTokens[token.id]}
              highlighted={highlightedTokenId === token.id}
              onCopy={() => onCopy(token)}
              onReveal={() => onReveal(token)}
              onEdit={() => onEdit(token)}
              onToggleEnabled={() => onToggleEnabled(token)}
              onRegenerate={() => onRegenerate(token)}
              onDelete={() => onDelete(token)}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function TokenRow({
  token,
  ownerLabel,
  visible,
  revealedValue,
  highlighted = false,
  onCopy,
  onReveal,
  onEdit,
  onToggleEnabled,
  onRegenerate,
  onDelete,
}: {
  token: AccessToken;
  ownerLabel: string;
  visible: boolean;
  revealedValue?: string;
  highlighted?: boolean;
  onCopy: () => void;
  onReveal: () => void;
  onEdit: () => void;
  onToggleEnabled: () => void;
  onRegenerate: () => void;
  onDelete: () => void;
}) {
  const displayToken = visible
    ? (revealedValue ?? token.token_masked)
    : token.token_masked;

  return (
    <div
      id={`access-token-${token.id}`}
      className={cn(
        "grid gap-4 border-b border-moon-200/20 py-3.5 last:border-b-0 xl:items-start xl:gap-x-4",
        TOKEN_GRID_COLUMNS,
        // ring-inset keeps the ring inside the row's box so it does not shift
        // grid column alignment relative to sibling rows (which would happen
        // with outside ring + padding compensation).
        highlighted &&
          "rounded-[0.9rem] bg-lunar-100/45 ring-2 ring-inset ring-lunar-300/70 transition-colors",
      )}
    >
      <InlineMeta label="名称" value={token.name} strong />
      <InlineMeta
        label="Token"
        value={displayToken || "--"}
        mono
        muted={!visible}
        nowrap
      />
      <InlineMeta label="创建时间" value={shortDate(token.created_at)} />
      <InlineMeta
        label="状态"
        value={
          <StatusBadge ok={token.enabled}>
            {token.enabled ? "Enabled" : "Disabled"}
          </StatusBadge>
        }
      />
      <InlineMeta label="归属" value={ownerLabel} />
      <div className="flex min-w-0 flex-nowrap items-center justify-start gap-1.5 overflow-hidden xl:self-start">
        <Button
          variant="ghost"
          size="sm"
          className="shrink-0 rounded-full px-2.5 text-moon-500"
          onClick={onCopy}
        >
          <Copy className="size-3.5" />
          Copy
        </Button>
        <Button
          variant="ghost"
          size="sm"
          className="shrink-0 rounded-full px-2.5 text-moon-500"
          onClick={onReveal}
        >
          {visible ? (
            <EyeOff className="size-3.5" />
          ) : (
            <Eye className="size-3.5" />
          )}
          Reveal
        </Button>
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button
                variant="ghost"
                size="icon-sm"
                className="shrink-0 rounded-full text-moon-500"
              />
            }
          >
            <MoreHorizontal className="size-4" />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-48">
            <DropdownMenuItem onClick={onEdit}>
              <PencilLine className="size-4" />
              Edit name
            </DropdownMenuItem>
            <DropdownMenuItem onClick={onToggleEnabled}>
              <CircleDot className="size-4" />
              {token.enabled ? "Disable" : "Enable"}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={onRegenerate}>
              <WandSparkles className="size-4" />
              Regenerate
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={onDelete}
              className="text-status-red focus:text-status-red"
            >
              <Trash2 className="size-4" />
              Delete
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </div>
  );
}

function InlineMeta({
  label,
  value,
  strong,
  mono,
  muted,
  nowrap,
}: {
  label: string;
  value: string | ReactNode;
  strong?: boolean;
  mono?: boolean;
  muted?: boolean;
  nowrap?: boolean;
}) {
  const title = typeof value === "string" && value !== "--" ? value : undefined;

  return (
    <div className="min-w-0 space-y-1">
      <p className="text-[11px] tracking-[0.16em] text-moon-300 xl:hidden">
        {label}
      </p>
      <div
        title={title}
        className={cn(
          "text-sm text-moon-500",
          strong && "font-medium text-moon-800",
          mono && "font-mono text-[13px] leading-5",
          muted && "text-moon-400",
          nowrap && "overflow-hidden text-ellipsis whitespace-nowrap",
          !nowrap && "truncate",
        )}
      >
        {value}
      </div>
    </div>
  );
}

function InfoBlock({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="space-y-1.5">
      <p className="text-[11px] tracking-[0.16em] text-moon-300">{label}</p>
      <div className="text-sm text-moon-700">{value}</div>
    </div>
  );
}

function StatusBadge({ ok, children }: { ok: boolean; children: string }) {
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-full px-2.5 py-1 text-xs font-medium",
        ok
          ? "bg-status-green/12 text-status-green"
          : "bg-status-red/10 text-status-red",
      )}
    >
      {children}
    </span>
  );
}
