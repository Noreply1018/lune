import {
  useRef,
  useEffect,
  useMemo,
  useState,
  type ChangeEvent,
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
  Download,
  Upload,
} from "lucide-react";
import ConfirmDialog from "@/components/ConfirmDialog";
import ErrorState from "@/components/ErrorState";
import PageHeader from "@/components/PageHeader";
import SectionHeading from "@/components/SectionHeading";
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
import { Switch } from "@/components/ui/switch";
import { api } from "@/lib/api";
import { shortDate } from "@/lib/fmt";
import { maskToken } from "@/lib/lune";
import type {
  AccessToken,
  ConfigImportResult,
  CpaService,
  DataRetentionSummary,
  Pool,
  RevealedAccessToken,
  SystemSettings,
} from "@/lib/types";
import { cn } from "@/lib/utils";

type EditableSettingField =
  | "request_timeout"
  | "max_retry_attempts"
  | "health_check_interval"
  | "data_retention_days"
  | "notification_expiring_days"
  | "webhook_url";

type ToggleSettingField =
  | "notification_error_enabled"
  | "notification_expiring_enabled"
  | "webhook_enabled";

type TokenDraft = {
  name: string;
  scope: "global" | "pool";
  poolId: string;
};

type ParsedImportConfig = {
  pools?: Array<{ label?: string }>;
  access_tokens?: Array<{ name?: string }>;
  settings?: Record<string, unknown>;
};

type ParsedImportEnvelope = ParsedImportConfig & {
  data?: ParsedImportConfig;
};

const IMPORTABLE_SETTING_KEYS = new Set([
  "health_check_interval",
  "request_timeout",
  "max_retry_attempts",
  "notification_error_enabled",
  "notification_expiring_enabled",
  "notification_expiring_days",
  "webhook_url",
  "webhook_enabled",
  "data_retention_days",
]);

const INITIAL_TOKEN_DRAFT: TokenDraft = {
  name: "",
  scope: "global",
  poolId: "",
};

const TOKEN_GRID_COLUMNS =
  "xl:grid-cols-[minmax(10rem,1fr)_minmax(0,4.8fr)_9.5rem_6.25rem_6.25rem_11.5rem]";
const TOKEN_COLUMN_LABELS = ["名称", "Token", "创建时间", "状态", "归属"];

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
  const [notificationForm, setNotificationForm] = useState({
    webhook_url: "",
    notification_expiring_days: 7,
  });
  const [savingField, setSavingField] = useState<EditableSettingField | null>(
    null,
  );
  const [savingToggles, setSavingToggles] = useState<
    Partial<Record<ToggleSettingField, boolean>>
  >({});
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
  const [testingService, setTestingService] = useState(false);
  const [pruning, setPruning] = useState(false);
  const [importDraft, setImportDraft] = useState<ParsedImportEnvelope | null>(
    null,
  );
  const [importConfirmOpen, setImportConfirmOpen] = useState(false);
  const [importing, setImporting] = useState(false);
  const fileInputRef = useRef<HTMLInputElement | null>(null);

  async function loadRetentionSummary() {
    const summary = await api.get<DataRetentionSummary>(
      "/settings/data-retention",
    );
    setRetentionSummary(summary);
    return summary;
  }

  function load() {
    setLoading(true);
    setError(null);
    Promise.all([
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
        setNotificationForm({
          webhook_url: settingsData.webhook_url,
          notification_expiring_days: settingsData.notification_expiring_days,
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
  const importPreview = useMemo(() => {
    if (!importDraft) {
      return { pools: 0, tokens: 0, settings: 0 };
    }
    const source = importDraft.data ?? importDraft;
    return {
      pools: source.pools?.length ?? 0,
      tokens: source.access_tokens?.length ?? 0,
      settings: Object.keys(source.settings ?? {}).filter((key) =>
        IMPORTABLE_SETTING_KEYS.has(key),
      ).length,
    };
  }, [importDraft]);

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
    const prevSettings = settings;
    setSavingField(field);
    try {
      await api.put("/settings", { [field]: payloadValue });
      setSettings((current) =>
        current ? { ...current, [field]: payloadValue } : current,
      );
      if (field === "data_retention_days") {
        await loadRetentionSummary();
      }
      toast("设置已更新");
    } catch (err) {
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
        if (field === "webhook_url" || field === "notification_expiring_days") {
          setNotificationForm((current) => ({
            ...current,
            [field]: prevSettings[field],
          }));
        }
      }
    } finally {
      setSavingField(null);
    }
  }

  async function saveToggle(field: ToggleSettingField, value: boolean) {
    setSavingToggles((current) => ({ ...current, [field]: true }));
    setSettings((current) =>
      current ? { ...current, [field]: value } : current,
    );
    try {
      await api.put("/settings", { [field]: value });
      toast("设置已更新");
    } catch (err) {
      setSettings((current) =>
        current ? { ...current, [field]: !value } : current,
      );
      toast(err instanceof Error ? err.message : "保存设置失败", "error");
    } finally {
      setSavingToggles((current) => ({ ...current, [field]: false }));
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

  async function testWebhook() {
    const webhookURL = notificationForm.webhook_url.trim();
    if (!webhookURL) {
      return;
    }
    try {
      const result = await api.post<{ success: boolean; latency_ms: number }>(
        "/settings/webhook/test",
        { url: webhookURL },
      );
      toast(
        result.success
          ? `Webhook 测试成功 ${result.latency_ms}ms`
          : "Webhook 测试失败",
        result.success ? "success" : "error",
      );
    } catch (err) {
      toast(err instanceof Error ? err.message : "Webhook 测试失败", "error");
    }
  }

  async function pruneNow() {
    setPruning(true);
    try {
      const result = await api.post<{ deleted_logs: number }>(
        "/settings/data-retention/prune",
        {},
      );
      await loadRetentionSummary();
      toast(`已清理 ${result.deleted_logs} 条`);
    } catch (err) {
      toast(err instanceof Error ? err.message : "清理失败", "error");
    } finally {
      setPruning(false);
    }
  }

  function triggerImportPicker() {
    fileInputRef.current?.click();
  }

  async function handleImportFileChange(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) {
      return;
    }
    try {
      const parsed = JSON.parse(await file.text()) as ParsedImportEnvelope;
      if (typeof parsed !== "object" || parsed === null) {
        throw new Error("导入文件格式不正确");
      }
      setImportDraft(parsed);
      setImportConfirmOpen(true);
    } catch (err) {
      toast(err instanceof Error ? err.message : "读取导入文件失败", "error");
    }
  }

  async function confirmImport() {
    if (!importDraft) {
      return;
    }
    setImporting(true);
    try {
      const result = await api.post<ConfigImportResult>("/import", importDraft);
      setImportConfirmOpen(false);
      setImportDraft(null);
      await load();
      toast(
        `导入完成：${result.created_pools} 新建 Pool，${result.updated_pools} 更新 Pool，${result.created_tokens} 新建 Token，${result.skipped_tokens} 跳过 Token，${result.updated_settings} 项设置更新`,
      );
    } catch (err) {
      toast(err instanceof Error ? err.message : "导入失败", "error");
    } finally {
      setImporting(false);
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
      <PageHeader
        title="Settings"
        description="管理网关行为、访问凭证与系统连接。"
      />

      <section className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)] lg:items-stretch">
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
              onChange={(value) =>
                setGatewayForm((current) => ({
                  ...current,
                  request_timeout: value,
                }))
              }
              onBlur={() =>
                void saveSetting("request_timeout", gatewayForm.request_timeout)
              }
              onKeyDown={handleSettingKeyDown}
            />
            <SettingsNumericRow
              label="Max Retry Attempts"
              value={gatewayForm.max_retry_attempts}
              suffix="次"
              min={1}
              saving={savingField === "max_retry_attempts"}
              onChange={(value) =>
                setGatewayForm((current) => ({
                  ...current,
                  max_retry_attempts: value,
                }))
              }
              onBlur={() =>
                void saveSetting(
                  "max_retry_attempts",
                  gatewayForm.max_retry_attempts,
                )
              }
              onKeyDown={handleSettingKeyDown}
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

      <section className="surface-section px-5 py-5 sm:px-6">
        <SectionHeading
          title="Notifications"
          description="将关键告警推送到外部 Webhook。"
        />
        <div className="mt-4 flex flex-col gap-5">
          <div className="space-y-2.5">
            <div className="flex items-center justify-between gap-3">
              <label
                htmlFor="webhook-url"
                className="text-sm font-medium text-moon-800"
              >
                Webhook URL
              </label>
              <Button
                variant="outline"
                size="sm"
                className="rounded-full"
                onClick={() => void testWebhook()}
                disabled={!notificationForm.webhook_url.trim()}
              >
                <CircleDot className="size-4" />
                Test
              </Button>
            </div>
            <Input
              id="webhook-url"
              value={notificationForm.webhook_url}
              placeholder="https://example.com/webhook"
              onChange={(event) =>
                setNotificationForm((current) => ({
                  ...current,
                  webhook_url: event.target.value,
                }))
              }
              onBlur={() =>
                void saveSetting("webhook_url", notificationForm.webhook_url)
              }
              onKeyDown={handleSettingKeyDown}
            />
          </div>

          <div className="rounded-[1.35rem] border border-moon-200/35 bg-white/50 px-4 py-4">
            <div className="flex items-center justify-between gap-3 border-b border-moon-200/25 pb-4">
              <div className="space-y-0.5">
                <p className="text-sm font-medium text-moon-800">
                  Enable Webhook
                </p>
                <p className="text-xs text-moon-350">
                  关闭后保留 URL，但不会发送任何推送。
                </p>
              </div>
              <div className="flex items-center gap-2">
                {savingToggles.webhook_enabled ? (
                  <RefreshCw className="size-4 animate-spin text-moon-350" />
                ) : null}
                <Switch
                  checked={settings?.webhook_enabled ?? false}
                  disabled={savingToggles.webhook_enabled}
                  onCheckedChange={(checked) =>
                    void saveToggle("webhook_enabled", checked)
                  }
                />
              </div>
            </div>

            <div className="divide-y divide-moon-200/25">
              <SettingsToggleRow
                label="Account Health Failure"
                helper="账号健康检查进入 error 状态时触发。"
                checked={settings?.notification_error_enabled ?? false}
                saving={savingToggles.notification_error_enabled ?? false}
                disabled={savingToggles.notification_error_enabled ?? false}
                onCheckedChange={(checked) =>
                  void saveToggle("notification_error_enabled", checked)
                }
              />
              <SettingsToggleRow
                label="Account Expiring Soon"
                helper="CPA 账号即将过期或已过期时触发。"
                checked={settings?.notification_expiring_enabled ?? false}
                saving={savingToggles.notification_expiring_enabled ?? false}
                disabled={savingToggles.notification_expiring_enabled ?? false}
                onCheckedChange={(checked) =>
                  void saveToggle("notification_expiring_enabled", checked)
                }
              />
              <SettingsNumericRow
                label="Expiring Threshold"
                helper="到期前多少天开始告警"
                value={notificationForm.notification_expiring_days}
                suffix="天"
                min={1}
                saving={savingField === "notification_expiring_days"}
                onChange={(value) =>
                  setNotificationForm((current) => ({
                    ...current,
                    notification_expiring_days: value,
                  }))
                }
                onBlur={() =>
                  void saveSetting(
                    "notification_expiring_days",
                    notificationForm.notification_expiring_days,
                  )
                }
                onKeyDown={handleSettingKeyDown}
              />
            </div>
          </div>
        </div>
      </section>

      <section className="surface-section px-5 py-5 sm:px-6">
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

      <section className="surface-section px-5 py-5 sm:px-6">
        <SectionHeading
          title="Data Retention"
          description="控制日志清理规则，并查看当前库内日志的总体状态。"
        />
        <div className="mt-5 space-y-5">
          <div className="divide-y divide-moon-200/25 border-y border-moon-200/25">
            <SettingsNumericRow
              label="Log Retention Days"
              helper="实时生效"
              value={settings?.data_retention_days ?? 0}
              suffix="天"
              min={0}
              saving={savingField === "data_retention_days"}
              onChange={(value) =>
                setSettings((current) =>
                  current
                    ? { ...current, data_retention_days: value }
                    : current,
                )
              }
              onBlur={() =>
                void saveSetting(
                  "data_retention_days",
                  settings?.data_retention_days ?? 0,
                )
              }
              onKeyDown={handleSettingKeyDown}
            />
            <SettingsNumericRow
              label="Health Check Interval"
              helper="修改后需重启生效"
              value={systemForm.health_check_interval}
              suffix="秒"
              min={1}
              saving={savingField === "health_check_interval"}
              onChange={(value) =>
                setSystemForm({ health_check_interval: value })
              }
              onBlur={() =>
                void saveSetting(
                  "health_check_interval",
                  systemForm.health_check_interval,
                )
              }
              onKeyDown={handleSettingKeyDown}
            />
          </div>

          <div className="space-y-4">
            <p className="text-[11px] font-medium tracking-[0.16em] text-moon-300">
              当前状态
            </p>
            <div className="grid gap-4 border-y border-moon-200/25 py-4 sm:grid-cols-3">
              <MetricMeta
                label="总日志量"
                value={`${Number(retentionSummary?.total_logs ?? 0).toLocaleString()} 条`}
              />
              <MetricMeta
                label="最早记录"
                value={
                  retentionSummary?.oldest_log_at
                    ? shortDate(retentionSummary.oldest_log_at)
                    : "暂无"
                }
              />
              <MetricMeta
                label="最近记录"
                value={
                  retentionSummary?.newest_log_at
                    ? shortDate(retentionSummary.newest_log_at)
                    : "暂无"
                }
              />
            </div>
            <div className="flex flex-wrap items-center justify-between gap-3">
              <p className="text-sm text-moon-450">
                当前清理规则为 {retentionSummary?.retention_days ?? 0} 天。
              </p>
              <Button
                variant="outline"
                size="sm"
                className="rounded-full"
                onClick={() => void pruneNow()}
                disabled={pruning || (retentionSummary?.total_logs ?? 0) === 0}
              >
                {pruning ? (
                  <RefreshCw className="size-4 animate-spin" />
                ) : null}
                Clean Up Now
              </Button>
            </div>
          </div>
        </div>
      </section>

      <section className="surface-section px-5 py-5 sm:px-6">
        <SectionHeading
          title="Configuration Transfer"
          description="导出当前配置快照，并从导出文件恢复可导入的配置。"
        />
        <div className="mt-5 space-y-4">
          <div className="flex flex-wrap gap-3">
            <Button
              variant="outline"
              size="sm"
              className="rounded-full"
              onClick={() => window.open("/admin/api/export", "_blank")}
            >
              <Download className="size-4" />
              Export Configuration
            </Button>
            <Button
              variant="outline"
              size="sm"
              className="rounded-full"
              onClick={triggerImportPicker}
            >
              <Upload className="size-4" />
              Import Configuration
            </Button>
            <input
              ref={fileInputRef}
              type="file"
              accept=".json,application/json"
              className="hidden"
              onChange={handleImportFileChange}
            />
          </div>
          <div className="space-y-1.5 text-sm leading-6 text-moon-500">
            <p>导出文件会包含 Pool、Token、系统设置，以及账号与 CPA Service 的快照信息。</p>
            <p>导入只会恢复 Pool、Token 名称与可导入的设置项；Token 密钥会重新生成。</p>
          </div>
        </div>
      </section>

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

      <Dialog
        open={importConfirmOpen}
        onOpenChange={(open) => {
          setImportConfirmOpen(open);
          if (!open && !importing) {
            setImportDraft(null);
          }
        }}
      >
        <DialogContent className="max-w-lg overflow-hidden rounded-[1.6rem] border border-white/75 bg-white/95 p-0 shadow-[0_26px_70px_-38px_rgba(74,68,108,0.34)]">
          <DialogHeader className="border-b border-moon-200/55 px-6 py-5 pr-12">
            <DialogTitle>Import Configuration</DialogTitle>
            <DialogDescription>
              即将把导出文件中的 Pool、Token 与设置写入当前环境。
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-5 px-6 py-6">
            <div className="grid gap-4 sm:grid-cols-3">
              <MetricMeta label="Pools" value={`${importPreview.pools}`} />
              <MetricMeta label="Tokens" value={`${importPreview.tokens}`} />
              <MetricMeta
                label="Settings"
                value={`${importPreview.settings}`}
              />
            </div>
            <div className="space-y-3 rounded-[1.2rem] border border-moon-200/45 bg-moon-50/55 px-4 py-4 text-sm leading-6 text-moon-550">
              <p>Token 将自动生成新的密钥值。</p>
              <p>已存在的 Pool 会更新；已存在的同名 Token 会被跳过。</p>
              <p>账号、CPA Service 与 admin token 不会导入。</p>
            </div>
          </div>
          <DialogFooter className="border-t border-moon-200/55 bg-white/76 px-6 py-4">
            <Button
              variant="outline"
              onClick={() => {
                setImportConfirmOpen(false);
                setImportDraft(null);
              }}
              disabled={importing}
            >
              取消
            </Button>
            <Button onClick={() => void confirmImport()} disabled={importing}>
              {importing ? <RefreshCw className="size-4 animate-spin" /> : null}
              Confirm Import
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
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
    <div className="flex flex-col gap-3 py-3 sm:flex-row sm:items-center sm:justify-between">
      <div className="space-y-0.5">
        <p className="text-sm font-medium text-moon-800">{label}</p>
        <p className="text-xs text-moon-350">{helper}</p>
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

function SettingsToggleRow({
  label,
  helper,
  checked,
  saving,
  disabled,
  onCheckedChange,
}: {
  label: string;
  helper: string;
  checked: boolean;
  saving?: boolean;
  disabled?: boolean;
  onCheckedChange: (checked: boolean) => void;
}) {
  return (
    <div className="flex items-center justify-between gap-3 py-4">
      <div className="space-y-0.5">
        <p className="text-sm font-medium text-moon-800">{label}</p>
        <p className="text-xs text-moon-350">{helper}</p>
      </div>
      <div className="flex items-center gap-2">
        {saving ? (
          <RefreshCw className="size-4 animate-spin text-moon-350" />
        ) : (
          <span className="size-4" />
        )}
        <Switch
          checked={checked}
          disabled={disabled}
          onCheckedChange={onCheckedChange}
        />
      </div>
    </div>
  );
}

function MetricMeta({ label, value }: { label: string; value: string }) {
  return (
    <div className="space-y-1">
      <p className="text-[11px] tracking-[0.16em] text-moon-300">{label}</p>
      <p className="text-sm font-medium text-moon-800">{value}</p>
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
  const displayToken = visible
    ? (revealedValue ?? token.token_masked)
    : token.token_masked;

  return (
    <div
      className={cn(
        "grid gap-4 border-b border-moon-200/20 py-3.5 last:border-b-0 xl:items-start xl:gap-x-4",
        TOKEN_GRID_COLUMNS,
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
