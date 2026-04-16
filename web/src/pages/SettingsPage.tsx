import { useEffect, useMemo, useState, type KeyboardEvent, type ReactNode } from "react";
import {
  ChevronDown,
  ChevronRight,
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
  Download,
} from "lucide-react";
import ConfirmDialog from "@/components/ConfirmDialog";
import ErrorState from "@/components/ErrorState";
import PageHeader from "@/components/PageHeader";
import SectionHeading from "@/components/SectionHeading";
import { toast } from "@/components/Feedback";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { api } from "@/lib/api";
import { relativeTime, shortDate } from "@/lib/fmt";
import type {
  AccessToken,
  CpaService,
  DataRetentionSummary,
  Pool,
  RevealedAccessToken,
  SystemNotification,
  SystemSettings,
} from "@/lib/types";
import { cn } from "@/lib/utils";

type EditableSettingField =
  | "request_timeout"
  | "max_retry_attempts"
  | "health_check_interval"
  | "notification_error_enabled"
  | "notification_expiring_enabled"
  | "notification_expiring_days"
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

export default function SettingsPage() {
  const [service, setService] = useState<CpaService | null>(null);
  const [settings, setSettings] = useState<SystemSettings | null>(null);
  const [notifications, setNotifications] = useState<SystemNotification[]>([]);
  const [retentionSummary, setRetentionSummary] = useState<DataRetentionSummary | null>(null);
  const [tokens, setTokens] = useState<AccessToken[]>([]);
  const [pools, setPools] = useState<Pool[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [gatewayForm, setGatewayForm] = useState({ request_timeout: 120, max_retry_attempts: 3 });
  const [notificationForm, setNotificationForm] = useState({
    notification_error_enabled: true,
    notification_expiring_enabled: true,
    notification_expiring_days: 7,
  });
  const [retentionForm, setRetentionForm] = useState({ data_retention_days: 30 });
  const [systemForm, setSystemForm] = useState({ health_check_interval: 60 });
  const [savingField, setSavingField] = useState<EditableSettingField | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [createDraft, setCreateDraft] = useState<TokenDraft>(INITIAL_TOKEN_DRAFT);
  const [editingToken, setEditingToken] = useState<AccessToken | null>(null);
  const [editingName, setEditingName] = useState("");
  const [deleteToken, setDeleteToken] = useState<AccessToken | null>(null);
  const [regenerateToken, setRegenerateToken] = useState<AccessToken | null>(null);
  const [revealedTokens, setRevealedTokens] = useState<Record<number, string>>({});
  const [visibleTokenIds, setVisibleTokenIds] = useState<number[]>([]);
  const [testingService, setTestingService] = useState(false);
  const [pruningRetention, setPruningRetention] = useState(false);
  const [systemOpen, setSystemOpen] = useState(false);

  function load() {
    setLoading(true);
    setError(null);
    Promise.all([
      api.get<CpaService | null>("/cpa/service"),
      api.get<SystemSettings>("/settings"),
      api.get<SystemNotification[]>("/settings/notifications"),
      api.get<DataRetentionSummary>("/settings/data-retention"),
      api.get<AccessToken[]>("/tokens"),
      api.get<Pool[]>("/pools"),
    ])
      .then(([serviceData, settingsData, notificationData, retentionData, tokenData, poolData]) => {
        setService(serviceData);
        setSettings(settingsData);
        setNotifications(notificationData ?? []);
        setRetentionSummary(retentionData);
        setTokens(tokenData ?? []);
        setPools(poolData ?? []);
        setGatewayForm({
          request_timeout: settingsData.request_timeout,
          max_retry_attempts: settingsData.max_retry_attempts,
        });
        setNotificationForm({
          notification_error_enabled: settingsData.notification_error_enabled,
          notification_expiring_enabled: settingsData.notification_expiring_enabled,
          notification_expiring_days: settingsData.notification_expiring_days,
        });
        setRetentionForm({
          data_retention_days: settingsData.data_retention_days,
        });
        setSystemForm({
          health_check_interval: settingsData.health_check_interval,
        });
      })
      .catch((err) => setError(err instanceof Error ? err.message : "Settings 加载失败"))
      .finally(() => setLoading(false));
  }

  useEffect(() => {
    load();
  }, []);

  const poolNameMap = useMemo(
    () => Object.fromEntries(pools.map((pool) => [pool.id, pool.label])),
    [pools],
  );

  const globalTokens = useMemo(
    () => tokens.filter((token) => token.pool_id == null),
    [tokens],
  );

  const poolGroups = useMemo(
    () => pools
      .map((pool) => ({
        pool,
        tokens: tokens.filter((token) => token.pool_id === pool.id),
      }))
      .filter((group) => group.tokens.length > 0),
    [pools, tokens],
  );

  async function saveSetting(field: EditableSettingField, value: number | boolean) {
    const normalized = typeof value === "number" ? (Number.isFinite(value) ? value : 0) : value;
    setSavingField(field);
    try {
      await api.put("/settings", { [field]: normalized });
      setSettings((current) => (current ? { ...current, [field]: normalized } : current));
      if (field.startsWith("notification_")) {
        const latestNotifications = await api.get<SystemNotification[]>("/settings/notifications");
        setNotifications(latestNotifications ?? []);
        toast("Notifications 已更新");
      } else if (field === "data_retention_days") {
        const latestSummary = await api.get<DataRetentionSummary>("/settings/data-retention");
        setRetentionSummary(latestSummary);
        toast("Data Retention 已更新");
      } else {
        toast(field === "health_check_interval" ? "维护项已更新" : "Gateway Behavior 已更新");
      }
    } catch (err) {
      toast(err instanceof Error ? err.message : "保存设置失败", "error");
      if (settings) {
        setGatewayForm({
          request_timeout: settings.request_timeout,
          max_retry_attempts: settings.max_retry_attempts,
        });
        setNotificationForm({
          notification_error_enabled: settings.notification_error_enabled,
          notification_expiring_enabled: settings.notification_expiring_enabled,
          notification_expiring_days: settings.notification_expiring_days,
        });
        setRetentionForm({
          data_retention_days: settings.data_retention_days,
        });
        setSystemForm({ health_check_interval: settings.health_check_interval });
      }
    } finally {
      setSavingField(null);
    }
  }

  async function pruneRetentionNow() {
    setPruningRetention(true);
    try {
      const result = await api.post<{
        deleted_logs: number;
        total_logs: number;
        oldest_log_at: string | null;
        newest_log_at: string | null;
        retention_days: number;
      }>("/settings/data-retention/prune", {});
      setRetentionSummary({
        retention_days: result.retention_days,
        total_logs: result.total_logs,
        oldest_log_at: result.oldest_log_at,
        newest_log_at: result.newest_log_at,
      });
      toast(result.deleted_logs > 0 ? `已清理 ${result.deleted_logs} 条请求日志` : "没有需要清理的旧日志");
    } catch (err) {
      toast(err instanceof Error ? err.message : "执行数据清理失败", "error");
    } finally {
      setPruningRetention(false);
    }
  }

  function handleSettingKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "Enter") {
      event.preventDefault();
      event.currentTarget.blur();
    }
  }

  async function revealTokenValue(token: AccessToken): Promise<string> {
    const cached = revealedTokens[token.id];
    if (cached) {
      return cached;
    }
    const revealed = await api.post<RevealedAccessToken>(`/tokens/${token.id}/reveal`);
    setRevealedTokens((current) => ({ ...current, [token.id]: revealed.token }));
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
    try {
      await api.post(`/tokens/${token.id}/${token.enabled ? "disable" : "enable"}`);
      toast(token.enabled ? "Token 已停用" : "Token 已启用");
      load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "更新 Token 状态失败", "error");
    }
  }

  async function submitCreateToken() {
    if (!createDraft.name.trim()) {
      toast("请先填写 Token 名称", "error");
      return;
    }
    const poolId = createDraft.scope === "pool" ? Number(createDraft.poolId) : null;
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
      setCreateOpen(false);
      setCreateDraft(INITIAL_TOKEN_DRAFT);
      setRevealedTokens((current) => ({ ...current, [created.id]: created.token }));
      setVisibleTokenIds((current) => [...current, created.id]);
      toast("Token 已创建");
      load();
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
    try {
      await api.put(`/tokens/${editingToken.id}`, {
        name,
        pool_id: editingToken.pool_id,
        enabled: editingToken.enabled,
      });
      toast("名称已更新");
      setEditingToken(null);
      setEditingName("");
      load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "更新名称失败", "error");
    }
  }

  async function confirmRegenerateToken() {
    if (!regenerateToken) {
      return;
    }
    try {
      const revealed = await api.post<RevealedAccessToken>(`/tokens/${regenerateToken.id}/regenerate`, {});
      setRevealedTokens((current) => ({ ...current, [revealed.id]: revealed.token }));
      setVisibleTokenIds((current) => Array.from(new Set([...current, revealed.id])));
      toast("Token 已重新生成");
      setRegenerateToken(null);
      load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "重新生成失败", "error");
    }
  }

  async function confirmDeleteToken() {
    if (!deleteToken) {
      return;
    }
    try {
      await api.delete(`/tokens/${deleteToken.id}`);
      setVisibleTokenIds((current) => current.filter((id) => id !== deleteToken.id));
      setRevealedTokens((current) => {
        const next = { ...current };
        delete next[deleteToken.id];
        return next;
      });
      toast("Token 已删除");
      setDeleteToken(null);
      load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "删除 Token 失败", "error");
    }
  }

  async function testService() {
    setTestingService(true);
    try {
      const result = await api.post<{ reachable: boolean; latency_ms: number; error: string }>("/cpa/service/test", {});
      toast(result.reachable ? `连接正常 ${result.latency_ms}ms` : result.error || "连接失败", result.reachable ? "success" : "error");
      load();
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
        <Skeleton className="h-36 rounded-[1.8rem]" />
        <Skeleton className="h-[28rem] rounded-[1.8rem]" />
        <Skeleton className="h-44 rounded-[1.8rem]" />
      </div>
    );
  }

  if (error) {
    return <ErrorState message={error} onRetry={load} />;
  }

  return (
    <div className="space-y-12 pb-8">
      <PageHeader
        eyebrow="Settings"
        title="Settings"
        description="把运行行为与 Token 控制收束到同一条安静工作面里，低频维护项折叠到后面。"
        actions={
          <Button onClick={() => setCreateOpen(true)}>
            <Plus className="size-4" />
            Create Token
          </Button>
        }
      />

      <section className="surface-section px-5 py-5 sm:px-6">
        <SectionHeading title="Gateway Behavior" description="轻量 runtime settings。" />
        <div className="mt-6 divide-y divide-moon-200/45">
          <SettingsNumericRow
            label="Request Timeout"
            value={gatewayForm.request_timeout}
            suffix="秒"
            min={1}
            saving={savingField === "request_timeout"}
            onChange={(value) => setGatewayForm((current) => ({ ...current, request_timeout: value }))}
            onBlur={() => void saveSetting("request_timeout", gatewayForm.request_timeout)}
            onKeyDown={handleSettingKeyDown}
          />
          <SettingsNumericRow
            label="Max Retry Attempts"
            value={gatewayForm.max_retry_attempts}
            min={1}
            saving={savingField === "max_retry_attempts"}
            onChange={(value) => setGatewayForm((current) => ({ ...current, max_retry_attempts: value }))}
            onBlur={() => void saveSetting("max_retry_attempts", gatewayForm.max_retry_attempts)}
            onKeyDown={handleSettingKeyDown}
          />
        </div>
      </section>

      <section className="surface-section px-5 py-5 sm:px-6">
        <SectionHeading
          title="Token Management"
          description="统一管理 Global Tokens 与 Pool Tokens。危险操作收进菜单里，列表只保留必要控制。"
        />
        <div className="mt-7 space-y-8">
          <TokenGroup
            title="Global Tokens"
            tokens={globalTokens}
            emptyText="还没有 Global Token。"
            poolNameMap={poolNameMap}
            revealedTokens={revealedTokens}
            visibleTokenIds={visibleTokenIds}
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

          <div className="space-y-6">
            <div className="border-t border-moon-200/45 pt-6">
              <p className="text-[11px] uppercase tracking-[0.18em] text-moon-400">Pool Tokens</p>
            </div>
            {poolGroups.length === 0 ? (
              <p className="text-sm text-moon-400">还没有 Pool Token。</p>
            ) : (
              poolGroups.map((group) => (
                <TokenGroup
                  key={group.pool.id}
                  title={group.pool.label}
                  tokens={group.tokens}
                  poolNameMap={poolNameMap}
                  revealedTokens={revealedTokens}
                  visibleTokenIds={visibleTokenIds}
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
              ))
            )}
          </div>
        </div>
      </section>

      <section className="surface-section px-5 py-5 sm:px-6">
        <SectionHeading
          title="Notifications"
          description="只做站内运维提醒，聚焦 CPA 到期和健康检查失败。"
        />
        <div className="mt-6 space-y-5">
          <div className="divide-y divide-moon-200/45">
            <SwitchRow
              label="Account / Service Errors"
              helper="当账号或 CPA Service 进入 error 状态时出现在这里。"
              checked={notificationForm.notification_error_enabled}
              saving={savingField === "notification_error_enabled"}
              onCheckedChange={(checked) => {
                setNotificationForm((current) => ({ ...current, notification_error_enabled: checked }));
                void saveSetting("notification_error_enabled", checked);
              }}
            />
            <SwitchRow
              label="Expiring CPA Accounts"
              helper="按到期窗口筛出即将过期的 CPA 账号。"
              checked={notificationForm.notification_expiring_enabled}
              saving={savingField === "notification_expiring_enabled"}
              onCheckedChange={(checked) => {
                setNotificationForm((current) => ({ ...current, notification_expiring_enabled: checked }));
                void saveSetting("notification_expiring_enabled", checked);
              }}
            />
            <SettingsNumericRow
              label="Expiring Window"
              helper="多少天内到期算作提醒。"
              value={notificationForm.notification_expiring_days}
              suffix="天"
              min={1}
              saving={savingField === "notification_expiring_days"}
              onChange={(value) => setNotificationForm((current) => ({ ...current, notification_expiring_days: value }))}
              onBlur={() => void saveSetting("notification_expiring_days", notificationForm.notification_expiring_days)}
              onKeyDown={handleSettingKeyDown}
            />
          </div>

          <div className="rounded-[1.4rem] border border-moon-200/55 bg-white/66">
            <div className="flex items-center justify-between gap-3 border-b border-moon-200/45 px-4 py-3">
              <p className="text-sm font-medium text-moon-800">Current Notifications</p>
              <p className="text-xs text-moon-400">{notifications.length} items</p>
            </div>
            {notifications.length === 0 ? (
              <p className="px-4 py-4 text-sm text-moon-400">当前没有需要处理的通知。</p>
            ) : (
              <div className="divide-y divide-moon-200/40">
                {notifications.map((notification, index) => (
                  <NotificationRow key={`${notification.type}-${notification.account_id ?? notification.service_id ?? index}-${notification.expires_at ?? ""}`} notification={notification} />
                ))}
              </div>
            )}
          </div>
        </div>
      </section>

      <section className="surface-section px-5 py-5 sm:px-6">
        <SectionHeading
          title="Data Retention"
          description="控制 `request_logs` 的保留周期，自动清理和手动清理都只作用于请求日志。"
        />
        <div className="mt-6 space-y-5">
          <SettingsNumericRow
            label="Request Log Retention"
            helper="0 表示禁用自动清理。"
            value={retentionForm.data_retention_days}
            suffix="天"
            min={0}
            saving={savingField === "data_retention_days"}
            onChange={(value) => setRetentionForm({ data_retention_days: value })}
            onBlur={() => void saveSetting("data_retention_days", retentionForm.data_retention_days)}
            onKeyDown={handleSettingKeyDown}
          />

          <div className="grid gap-4 rounded-[1.4rem] border border-moon-200/55 bg-white/66 px-4 py-4 sm:grid-cols-3">
            <InfoBlock label="Stored Logs" value={String(retentionSummary?.total_logs ?? 0)} />
            <InfoBlock label="Oldest Log" value={retentionSummary?.oldest_log_at ? shortDate(retentionSummary.oldest_log_at) : "--"} />
            <InfoBlock label="Newest Log" value={retentionSummary?.newest_log_at ? shortDate(retentionSummary.newest_log_at) : "--"} />
          </div>

          <div className="flex flex-wrap items-center justify-between gap-3 rounded-[1.4rem] border border-dashed border-moon-200/65 px-4 py-4">
            <p className="text-sm text-moon-500">
              {retentionForm.data_retention_days > 0
                ? `后台健康检查周期会自动清理 ${retentionForm.data_retention_days} 天前的请求日志。`
                : "当前已禁用自动清理。"}
            </p>
            <Button variant="outline" onClick={pruneRetentionNow} disabled={pruningRetention}>
              {pruningRetention ? <RefreshCw className="size-4 animate-spin" /> : <Trash2 className="size-4" />}
              Prune Now
            </Button>
          </div>
        </div>
      </section>

      <section className="surface-section px-5 py-5 sm:px-6">
        <SectionHeading title="CPA Service" description="只读连接状态，用于判断 CPA 通道当前是否可用。" />
        <div className="mt-6 space-y-4">
          {service ? (
            <>
              <div className="grid gap-4 border-b border-moon-200/45 pb-4 sm:grid-cols-2 xl:grid-cols-4">
                <InfoBlock label="Status" value={<StatusBadge ok={service.status === "healthy"}>{service.status === "healthy" ? "Healthy" : "Error"}</StatusBadge>} />
                <InfoBlock label="Label" value={service.label || "--"} />
                <InfoBlock label="Base URL" value={service.base_url || "--"} />
                <InfoBlock label="Last Checked" value={service.last_checked_at ? shortDate(service.last_checked_at) : "尚未检查"} />
              </div>
              <div className="flex justify-start">
                <Button variant="outline" onClick={testService} disabled={testingService}>
                  {testingService ? <RefreshCw className="size-4 animate-spin" /> : <CircleDot className="size-4" />}
                  Test Connection
                </Button>
              </div>
            </>
          ) : (
            <div className="flex flex-col gap-4 rounded-[1.4rem] border border-dashed border-moon-200/65 px-4 py-5 text-sm text-moon-500 sm:flex-row sm:items-center sm:justify-between">
              <p>请通过环境变量完成 CPA 配置</p>
              <Button variant="outline" onClick={testService} disabled={testingService}>
                {testingService ? <RefreshCw className="size-4 animate-spin" /> : <CircleDot className="size-4" />}
                Test Connection
              </Button>
            </div>
          )}
        </div>
      </section>

      <section className="surface-section px-5 py-5 sm:px-6">
        <button
          type="button"
          className="flex w-full items-center justify-between gap-4 text-left"
          onClick={() => setSystemOpen((current) => !current)}
        >
          <SectionHeading
            title="System Administration"
            description="低频高级维护项。默认收起，不主动打扰日常操作。"
          />
          <span className="flex size-8 items-center justify-center rounded-full border border-moon-200/65 bg-white/60 text-moon-500">
            {systemOpen ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" />}
          </span>
        </button>
        {systemOpen ? (
          <div className="mt-6 space-y-5 border-t border-moon-200/45 pt-5">
            <SettingsNumericRow
              label="Health Check Interval"
              helper="修改后需重启生效"
              value={systemForm.health_check_interval}
              suffix="秒"
              min={1}
              saving={savingField === "health_check_interval"}
              onChange={(value) => setSystemForm({ health_check_interval: value })}
              onBlur={() => void saveSetting("health_check_interval", systemForm.health_check_interval)}
              onKeyDown={handleSettingKeyDown}
            />
            <div className="flex flex-wrap gap-3">
              <Button variant="outline" onClick={() => window.open("/admin/api/export", "_blank") }>
                <Download className="size-4" />
                Export Configuration
              </Button>
            </div>
          </div>
        ) : null}
      </section>

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="max-w-md rounded-[1.6rem] border border-white/75 bg-white/95 p-0">
          <DialogHeader className="border-b border-moon-200/55 px-6 py-5">
            <DialogTitle>Create Token</DialogTitle>
            <DialogDescription>在这里选择归属并创建新的访问凭证。</DialogDescription>
          </DialogHeader>
          <div className="space-y-5 px-6 py-6">
            <div className="space-y-2">
              <label className="text-sm font-medium text-moon-700">Name</label>
              <Input
                value={createDraft.name}
                onChange={(event) => setCreateDraft((current) => ({ ...current, name: event.target.value }))}
                placeholder="例如：global-cli"
              />
            </div>
            <div className="space-y-3">
              <p className="text-sm font-medium text-moon-700">Ownership</p>
              <div className="flex flex-wrap gap-2">
                <button
                  type="button"
                  className={cn(
                    "rounded-full border px-3 py-1.5 text-sm transition-colors",
                    createDraft.scope === "global"
                      ? "border-lunar-300/60 bg-lunar-100/55 text-moon-800"
                      : "border-moon-200/65 bg-white/55 text-moon-500",
                  )}
                  onClick={() => setCreateDraft((current) => ({ ...current, scope: "global", poolId: "" }))}
                >
                  Global
                </button>
                <button
                  type="button"
                  className={cn(
                    "rounded-full border px-3 py-1.5 text-sm transition-colors",
                    createDraft.scope === "pool"
                      ? "border-lunar-300/60 bg-lunar-100/55 text-moon-800"
                      : "border-moon-200/65 bg-white/55 text-moon-500",
                  )}
                  onClick={() => setCreateDraft((current) => ({ ...current, scope: "pool" }))}
                >
                  指定 Pool
                </button>
              </div>
            </div>
            {createDraft.scope === "pool" ? (
              <div className="space-y-2">
                <label className="text-sm font-medium text-moon-700">Pool</label>
                <select
                  className="flex h-9 w-full rounded-lg border border-moon-200/65 bg-white/72 px-3 text-sm text-moon-700 outline-none focus:border-lunar-300/70"
                  value={createDraft.poolId}
                  onChange={(event) => setCreateDraft((current) => ({ ...current, poolId: event.target.value }))}
                >
                  <option value="">选择 Pool</option>
                  {pools.map((pool) => (
                    <option key={pool.id} value={pool.id}>
                      {pool.label}
                    </option>
                  ))}
                </select>
              </div>
            ) : null}
          </div>
          <DialogFooter className="border-t border-moon-200/55 bg-white/76 px-6 py-4">
            <Button variant="outline" onClick={() => setCreateOpen(false)}>取消</Button>
            <Button onClick={() => void submitCreateToken()}>Create Token</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={Boolean(editingToken)} onOpenChange={(open) => !open && setEditingToken(null)}>
        <DialogContent className="max-w-md rounded-[1.6rem] border border-white/75 bg-white/95 p-0">
          <DialogHeader className="border-b border-moon-200/55 px-6 py-5">
            <DialogTitle>Edit Token Name</DialogTitle>
            <DialogDescription>只更新名称，不改变归属与状态。</DialogDescription>
          </DialogHeader>
          <div className="px-6 py-6">
            <Input value={editingName} onChange={(event) => setEditingName(event.target.value)} />
          </div>
          <DialogFooter className="border-t border-moon-200/55 bg-white/76 px-6 py-4">
            <Button variant="outline" onClick={() => setEditingToken(null)}>取消</Button>
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
  onChange,
  onBlur,
  onKeyDown,
  suffix,
  helper = "实时生效",
  saving,
  min = 0,
}: {
  label: string;
  value: number;
  onChange: (value: number) => void;
  onBlur: () => void;
  onKeyDown: (event: KeyboardEvent<HTMLInputElement>) => void;
  suffix?: string;
  helper?: string;
  saving?: boolean;
  min?: number;
}) {
  return (
    <div className="flex flex-col gap-4 py-4 sm:flex-row sm:items-center sm:justify-between">
      <div className="space-y-1">
        <p className="text-sm font-medium text-moon-800">{label}</p>
        <p className="text-sm text-moon-400">{helper}</p>
      </div>
      <div className="flex items-center gap-2 self-start sm:self-auto">
        <Input
          type="number"
          value={value}
          min={min}
          className="h-9 w-24 text-right"
          onChange={(event) => onChange(Number(event.target.value))}
          onBlur={onBlur}
          onKeyDown={onKeyDown}
        />
        {suffix ? <span className="text-sm text-moon-400">{suffix}</span> : null}
        {saving ? <RefreshCw className="size-4 animate-spin text-moon-400" /> : null}
      </div>
    </div>
  );
}

function SwitchRow({
  label,
  helper,
  checked,
  saving,
  onCheckedChange,
}: {
  label: string;
  helper: string;
  checked: boolean;
  saving?: boolean;
  onCheckedChange: (checked: boolean) => void;
}) {
  return (
    <div className="flex items-center justify-between gap-4 py-4">
      <div className="space-y-1">
        <p className="text-sm font-medium text-moon-800">{label}</p>
        <p className="text-sm text-moon-400">{helper}</p>
      </div>
      <div className="flex items-center gap-2">
        <Switch checked={checked} onCheckedChange={onCheckedChange} />
        {saving ? <RefreshCw className="size-4 animate-spin text-moon-400" /> : null}
      </div>
    </div>
  );
}

function NotificationRow({ notification }: { notification: SystemNotification }) {
  return (
    <div className="flex flex-col gap-3 px-4 py-4 sm:flex-row sm:items-start sm:justify-between">
      <div className="space-y-1.5">
        <div className="flex flex-wrap items-center gap-2">
          <StatusBadge ok={notification.severity !== "critical"}>
            {notification.severity === "critical" ? "Critical" : "Warning"}
          </StatusBadge>
          <p className="text-sm font-medium text-moon-800">{notification.title}</p>
        </div>
        <p className="text-sm text-moon-500">{notification.message}</p>
      </div>
      {notification.expires_at ? (
        <p className="text-xs text-moon-400">Expires {shortDate(notification.expires_at)}</p>
      ) : null}
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
        <p className="text-xs text-moon-400">{tokens.length} tokens</p>
      </div>
      {tokens.length === 0 ? (
        <p className="text-sm text-moon-400">{emptyText ?? "当前分组为空。"}</p>
      ) : (
        <div className="border-y border-moon-200/45">
          <div className="hidden xl:grid xl:grid-cols-[minmax(0,1.25fr)_minmax(0,1.4fr)_8rem_8rem_7rem_8rem_auto] xl:gap-4 xl:border-b xl:border-moon-200/35 xl:py-3">
            {['Name', 'Token', 'Created', 'Last Used', 'Status', 'Ownership'].map((label) => (
              <p key={label} className="text-[11px] uppercase tracking-[0.18em] text-moon-400">
                {label}
              </p>
            ))}
            <p className="text-right text-[11px] uppercase tracking-[0.18em] text-moon-400">Actions</p>
          </div>
          {tokens.map((token) => (
            <TokenRow
              key={token.id}
              token={token}
              ownerLabel={token.pool_id ? poolNameMap[token.pool_id] ?? token.pool_label ?? `Pool #${token.pool_id}` : "Global"}
              visible={visibleTokenIds.includes(token.id)}
              revealedValue={revealedTokens[token.id]}
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
  onCopy: () => void;
  onReveal: () => void;
  onEdit: () => void;
  onToggleEnabled: () => void;
  onRegenerate: () => void;
  onDelete: () => void;
}) {
  const displayToken = visible ? revealedValue ?? token.token_masked : token.token_masked;

  return (
    <div className="grid gap-4 border-b border-moon-200/35 py-4 last:border-b-0 xl:grid-cols-[minmax(0,1.25fr)_minmax(0,1.4fr)_8rem_8rem_7rem_8rem_auto] xl:items-center">
      <InlineMeta label="Name" value={token.name} strong />
      <InlineMeta label="Token" value={displayToken || "--"} mono muted={!visible} />
      <InlineMeta label="Created" value={shortDate(token.created_at)} />
      <InlineMeta label="Last Used" value={relativeTime(token.last_used_at)} />
      <InlineMeta label="Status" value={<StatusBadge ok={token.enabled}>{token.enabled ? "Enabled" : "Disabled"}</StatusBadge>} />
      <InlineMeta label="Ownership" value={ownerLabel} />
      <div className="flex flex-wrap items-center justify-start gap-1.5 xl:justify-end">
        <Button variant="ghost" size="sm" className="rounded-full text-moon-500" onClick={onCopy}>
          <Copy className="size-3.5" />
          Copy
        </Button>
        <Button variant="ghost" size="sm" className="rounded-full text-moon-500" onClick={onReveal}>
          {visible ? <EyeOff className="size-3.5" /> : <Eye className="size-3.5" />}
          Reveal
        </Button>
        <Button variant="ghost" size="sm" className="rounded-full text-moon-500" onClick={onEdit}>
          <PencilLine className="size-3.5" />
          Edit name
        </Button>
        <Button variant="ghost" size="sm" className="rounded-full text-moon-500" onClick={onToggleEnabled}>
          {token.enabled ? "Disable" : "Enable"}
        </Button>
        <DropdownMenu>
          <DropdownMenuTrigger render={<Button variant="ghost" size="icon-sm" className="rounded-full text-moon-500" />}>
            <MoreHorizontal className="size-4" />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-48">
            <DropdownMenuItem onClick={onRegenerate}>
              <WandSparkles className="size-4" />
              Regenerate
            </DropdownMenuItem>
            <DropdownMenuItem onClick={onDelete} className="text-status-red focus:text-status-red">
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
}: {
  label: string;
  value: string | ReactNode;
  strong?: boolean;
  mono?: boolean;
  muted?: boolean;
}) {
  return (
    <div className="space-y-1.5 xl:space-y-1">
      <p className="text-[11px] uppercase tracking-[0.18em] text-moon-400 xl:hidden">{label}</p>
      <div
        className={cn(
          "text-sm text-moon-500",
          strong && "font-medium text-moon-800",
          mono && "font-mono text-[13px]",
          muted && "text-moon-400",
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
      <p className="text-[11px] uppercase tracking-[0.18em] text-moon-400">{label}</p>
      <div className="text-sm text-moon-700">{value}</div>
    </div>
  );
}

function StatusBadge({ ok, children }: { ok: boolean; children: string }) {
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-full px-2.5 py-1 text-xs font-medium",
        ok ? "bg-status-green/12 text-status-green" : "bg-status-red/10 text-status-red",
      )}
    >
      {children}
    </span>
  );
}
