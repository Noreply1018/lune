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
  RefreshCw,
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
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
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

type EditableSettingField =
  | "request_timeout"
  | "max_retry_attempts"
  | "gateway_max_body_mb"
  | "gateway_memory_body_mb"
  | "health_check_interval"
  | "data_retention_days";

const TOKEN_GRID_COLUMNS =
  "xl:grid-cols-[minmax(10rem,1.2fr)_minmax(10rem,1.1fr)_minmax(0,4.8fr)_9.5rem_11.5rem]";
// `align` shifts header label within its column without moving the data rows.
// 操作 走完全居中，状态 用 pl-5 落在“左对齐和完全居中”之间——列本身窄，硬 center
// 会让表头离值显得远；半偏一点既收紧又不贴边。
const TOKEN_COLUMNS: { label: string; align?: string }[] = [
  { label: "Pool" },
  { label: "名称" },
  { label: "Token" },
  { label: "Last Used" },
  { label: "操作", align: "text-center" },
];

const SETTINGS_SECTIONS: TOCSection[] = [
  { id: "settings-gateway", label: "Runtime" },
  { id: "notifications", label: "Notifications" },
  { id: "token-management", label: "Tokens" },
  { id: "data-retention", label: "Retention" },
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
    gateway_max_body_mb: 100,
    gateway_memory_body_mb: 8,
    health_check_interval: 60,
  });
  const [savingField, setSavingField] = useState<EditableSettingField | null>(
    null,
  );
  const settingsRef = useRef(settings);
  const [editingToken, setEditingToken] = useState<AccessToken | null>(null);
  const [editingName, setEditingName] = useState("");
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
          gateway_max_body_mb: settingsData.gateway_max_body_mb,
          gateway_memory_body_mb: settingsData.gateway_memory_body_mb,
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

  const poolCredentials = useMemo(
    () =>
      pools.map((pool) => ({
        pool,
        token: tokens.find((token) => token.pool_id === pool.id) ?? null,
      })),
    [pools, tokens],
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
        if (
          field === "request_timeout" ||
          field === "max_retry_attempts" ||
          field === "gateway_max_body_mb" ||
          field === "gateway_memory_body_mb" ||
          field === "health_check_interval"
        ) {
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
        description="管理网关行为、访问凭证与运行状态。"
      />

      <section
        id="settings-gateway"
        className="surface-section scroll-mt-6 overflow-hidden px-0 py-0"
      >
        <div className="border-b border-moon-200/45 px-5 py-5 sm:px-6">
          <SectionHeading
            title="System Runtime"
            description="网关执行、请求重放、健康检查与内置 CPA runtime 状态。"
            action={
              <Button
                variant="outline"
                size="sm"
                className="rounded-full"
                onClick={testService}
                disabled={testingService || !service}
              >
                {testingService ? (
                  <RefreshCw className="size-4 animate-spin" />
                ) : (
                  <CircleDot className="size-4" />
                )}
                Check Runtime
              </Button>
            }
          />
        </div>

        <div className="px-5 py-5 sm:px-6">
          {service ? (
            <div className="space-y-5">
              <div className="grid gap-x-6 gap-y-4 sm:grid-cols-2 xl:grid-cols-5">
                <InfoBlock
                  label="Status"
                  value={
                    <StatusBadge
                      ok={service.status === "healthy"}
                      pending={service.status === "unknown"}
                    >
                      {service.status === "healthy"
                        ? "Healthy"
                        : service.status === "unknown"
                          ? "Pending"
                          : "Error"}
                    </StatusBadge>
                  }
                />
                <InfoBlock label="Mode" value={service.runtime_mode || "embedded"} />
                <InfoBlock
                  label="CPA Version"
                  value={service.current_version || "Unknown"}
                />
                <InfoBlock
                  label="Latest Version"
                  value={
                    service.latest_version
                      ? service.latest_version
                      : "随 Lune 镜像更新"
                  }
                />
                <InfoBlock
                  label="Last Checked"
                  value={
                    service.last_checked_at
                      ? shortDate(service.last_checked_at)
                      : "尚未检查"
                  }
                />
              </div>

              <div className="grid gap-4 border-y border-moon-200/35 py-4 lg:grid-cols-[minmax(0,1.2fr)_minmax(0,1fr)]">
                <InfoBlock label="Auth Dir" value={service.auth_dir || "--"} />
                <InfoBlock label="Last Error" value={service.last_error || "None"} />
              </div>
            </div>
          ) : (
            <div className="border-y border-dashed border-moon-200/55 py-4 text-sm text-moon-500">
              内置 CPA 服务未就绪，请稍后重试或检查容器日志。
            </div>
          )}
        </div>

        <div className="grid border-t border-moon-200/45 lg:grid-cols-2">
          <div className="px-5 py-5 sm:px-6 lg:border-r lg:border-moon-200/35">
            <p className="mb-1 text-[11px] font-medium uppercase tracking-[0.16em] text-moon-300">
              Execution
            </p>
            <div className="divide-y divide-moon-200/30">
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
              <SettingsNumericRow
                label="Health Check Interval"
                value={gatewayForm.health_check_interval}
                suffix="秒"
                min={1}
                helper="健康检查跳动周期；越短越快发现故障。"
                saving={savingField === "health_check_interval"}
                onCommit={(value) => {
                  setGatewayForm((current) => ({
                    ...current,
                    health_check_interval: value,
                  }));
                  void saveSetting("health_check_interval", value);
                }}
              />
            </div>
          </div>

          <div className="border-t border-moon-200/45 px-5 py-5 sm:px-6 lg:border-t-0">
            <p className="mb-1 text-[11px] font-medium uppercase tracking-[0.16em] text-moon-300">
              Payload
            </p>
            <div className="divide-y divide-moon-200/30">
              <SettingsNumericRow
                label="Max Request Body"
                value={gatewayForm.gateway_max_body_mb}
                suffix="MB"
                min={1}
                helper="超过上限的请求会被拒绝并记录到 Activity。"
                saving={savingField === "gateway_max_body_mb"}
                onCommit={(value) => {
                  setGatewayForm((current) => ({
                    ...current,
                    gateway_max_body_mb: value,
                    gateway_memory_body_mb: Math.min(
                      current.gateway_memory_body_mb,
                      value,
                    ),
                  }));
                  void saveSetting("gateway_max_body_mb", value);
                }}
              />
              <SettingsNumericRow
                label="Memory Body Threshold"
                value={gatewayForm.gateway_memory_body_mb}
                suffix="MB"
                min={1}
                helper="超过阈值的请求写入磁盘重放，用于重试。"
                saving={savingField === "gateway_memory_body_mb"}
                onCommit={(value) => {
                  const next = Math.min(value, gatewayForm.gateway_max_body_mb);
                  setGatewayForm((current) => ({
                    ...current,
                    gateway_memory_body_mb: next,
                  }));
                  void saveSetting("gateway_memory_body_mb", next);
                }}
              />
            </div>
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

      <section id="token-management" className="surface-section scroll-mt-6 px-5 py-5 sm:px-6">
        <SectionHeading
          title="Pool Credentials"
          description="每个 Pool 自动拥有一条访问凭证；可复制、查看、重命名或重新生成。"
        />
        <div className="mt-7">
          <PoolCredentialsTable
            credentials={poolCredentials}
            revealedTokens={revealedTokens}
            visibleTokenIds={visibleTokenIds}
            highlightedTokenId={highlightedTokenId}
            onCopy={copyToken}
            onReveal={toggleReveal}
            onEdit={(token) => {
              setEditingToken(token);
              setEditingName(token.name);
            }}
            onRegenerate={setRegenerateToken}
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

      <div id="config-transfer" className="scroll-mt-6">
        <ConfigTransferSection onImported={() => load(true)} />
      </div>

      <NotificationHistorySection />

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
  // Track the last server-committed value locally. The naive
  // useEffect([value]) path setDraft on every prop bump — including the
  // optimistic bump from our own commit() — which races the user's next
  // keystroke. We only resync when the prop drifts from what we locally
  // committed (e.g. parent rolled back after a PUT failure).
  const committedRef = useRef(value);
  useEffect(() => {
    if (value === committedRef.current) return;
    committedRef.current = value;
    setDraft(`${value}`);
  }, [value]);

  function commit() {
    const trimmed = draft.trim();
    const parsed = Number(trimmed);
    if (trimmed === "" || !Number.isFinite(parsed) || parsed < min) {
      // Empty / NaN / below floor: roll back display rather than committing
      // a sentinel value that the server will just bounce.
      setDraft(`${committedRef.current}`);
      return;
    }
    const normalized = Math.floor(parsed);
    setDraft(`${normalized}`);
    if (normalized === committedRef.current) return;
    committedRef.current = normalized;
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
        <span className="w-8 text-sm text-moon-350">{suffix ?? ""}</span>
        {saving ? (
          <RefreshCw className="size-4 animate-spin text-moon-350" />
        ) : (
          <span className="size-4" />
        )}
      </div>
    </div>
  );
}

function PoolCredentialsTable({
  credentials,
  revealedTokens,
  visibleTokenIds,
  highlightedTokenId,
  onCopy,
  onReveal,
  onEdit,
  onRegenerate,
}: {
  credentials: { pool: Pool; token: AccessToken | null }[];
  revealedTokens: Record<number, string>;
  visibleTokenIds: number[];
  highlightedTokenId: number | null;
  onCopy: (token: AccessToken) => void;
  onReveal: (token: AccessToken) => void;
  onEdit: (token: AccessToken) => void;
  onRegenerate: (token: AccessToken) => void;
}) {
  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-3">
        <p className="text-sm font-medium text-moon-800">Pool access</p>
        <p className="text-xs text-moon-350">{credentials.length} pools</p>
      </div>
      {credentials.length === 0 ? (
        <div className="border-y border-moon-200/30 py-5">
          <p className="text-sm text-moon-400">还没有 Pool，创建 Pool 后会自动生成访问凭证。</p>
        </div>
      ) : (
        <div className="border-y border-moon-200/30">
          <div
            className={cn(
              "hidden border-b border-moon-200/20 py-2.5 xl:grid xl:gap-x-4",
              TOKEN_GRID_COLUMNS,
            )}
          >
            {TOKEN_COLUMNS.map(({ label, align }) => (
              <p
                key={label}
                className={cn(
                  "text-[11px] font-medium tracking-[0.16em] text-moon-300",
                  align,
                )}
              >
                {label}
              </p>
            ))}
          </div>
          {credentials.map(({ pool, token }) => (
            <PoolCredentialRow
              key={pool.id}
              pool={pool}
              token={token}
              visible={token ? visibleTokenIds.includes(token.id) : false}
              revealedValue={token ? revealedTokens[token.id] : undefined}
              highlighted={token ? highlightedTokenId === token.id : false}
              onCopy={token ? () => onCopy(token) : undefined}
              onReveal={token ? () => onReveal(token) : undefined}
              onEdit={token ? () => onEdit(token) : undefined}
              onRegenerate={token ? () => onRegenerate(token) : undefined}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function PoolCredentialRow({
  pool,
  token,
  visible,
  revealedValue,
  highlighted = false,
  onCopy,
  onReveal,
  onEdit,
  onRegenerate,
}: {
  pool: Pool;
  token: AccessToken | null;
  visible: boolean;
  revealedValue?: string;
  highlighted?: boolean;
  onCopy?: () => void;
  onReveal?: () => void;
  onEdit?: () => void;
  onRegenerate?: () => void;
}) {
  const displayToken = token
    ? visible
      ? (revealedValue ?? token.token_masked)
      : token.token_masked
    : "凭证未就绪，刷新后重试";

  return (
    <div
      id={token ? `access-token-${token.id}` : undefined}
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
      <InlineMeta label="Pool" value={pool.label} strong />
      <InlineMeta label="名称" value={token?.name ?? "自动创建中"} />
      <InlineMeta
        label="Token"
        value={displayToken}
        mono
        muted={!visible}
        nowrap
      />
      <InlineMeta
        label="Last Used"
        value={token?.last_used_at ? shortDate(token.last_used_at) : "Never"}
      />
      <div className="flex min-w-0 flex-nowrap items-center justify-start gap-1.5 overflow-hidden xl:self-start">
        <Button
          variant="ghost"
          size="sm"
          className="shrink-0 rounded-full px-2.5 text-moon-500"
          onClick={onCopy}
          disabled={!token}
        >
          <Copy className="size-3.5" />
          Copy
        </Button>
        <Button
          variant="ghost"
          size="sm"
          className="shrink-0 rounded-full px-2.5 text-moon-500"
          onClick={onReveal}
          disabled={!token}
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
            <DropdownMenuItem onClick={onEdit} disabled={!token}>
              <PencilLine className="size-4" />
              Edit name
            </DropdownMenuItem>
            <DropdownMenuItem onClick={onRegenerate} disabled={!token}>
              <WandSparkles className="size-4" />
              Regenerate
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

function StatusBadge({
  ok,
  pending,
  children,
}: {
  ok: boolean;
  pending?: boolean;
  children: string;
}) {
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-full px-2.5 py-1 text-xs font-medium",
        ok
          ? "bg-status-green/12 text-status-green"
          : pending
            ? "bg-moon-100/80 text-moon-500"
            : "bg-status-red/10 text-status-red",
      )}
    >
      {children}
    </span>
  );
}
