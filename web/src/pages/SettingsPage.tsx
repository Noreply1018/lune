import { type FormEvent, useEffect, useMemo, useState } from "react";
import { Download, KeyRound, RefreshCw, Server, Sparkles } from "lucide-react";
import ConfirmDialog from "@/components/ConfirmDialog";
import CopyButton from "@/components/CopyButton";
import EnvSnippetsDialog from "@/components/EnvSnippetsDialog";
import ErrorState from "@/components/ErrorState";
import PageHeader from "@/components/PageHeader";
import SectionHeading from "@/components/SectionHeading";
import { toast } from "@/components/Feedback";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { api } from "@/lib/api";
import { compact, latency, pct, relativeTime } from "@/lib/fmt";
import { getApiBaseUrl, maskToken } from "@/lib/lune";
import type {
  AccessToken,
  CpaService,
  Overview,
  Pool,
  SystemSettings,
  UsageLogPage,
  UsageStats,
} from "@/lib/types";

type UsageResponse = UsageStats & {
  logs: UsageLogPage;
};

export default function SettingsPage() {
  const [service, setService] = useState<CpaService | null>(null);
  const [settings, setSettings] = useState<SystemSettings | null>(null);
  const [overview, setOverview] = useState<Overview | null>(null);
  const [tokens, setTokens] = useState<AccessToken[]>([]);
  const [pools, setPools] = useState<Pool[]>([]);
  const [logs, setLogs] = useState<UsageLogPage | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [serviceForm, setServiceForm] = useState({ label: "", base_url: "", api_key: "" });
  const [settingsForm, setSettingsForm] = useState({
    external_url: "",
    health_check_interval: 60,
    request_timeout: 30,
    max_retry_attempts: 1,
  });
  const [globalSnippetOpen, setGlobalSnippetOpen] = useState(false);
  const [deleteServiceOpen, setDeleteServiceOpen] = useState(false);
  const [logFilterPool, setLogFilterPool] = useState("all");
  const [logFilterStatus, setLogFilterStatus] = useState("all");

  function load() {
    setLoading(true);
    setError(null);
    Promise.all([
      api.get<CpaService | null>("/cpa/service"),
      api.get<SystemSettings>("/settings"),
      api.get<Overview>("/overview"),
      api.get<AccessToken[]>("/tokens"),
      api.get<Pool[]>("/pools"),
      api.get<UsageResponse>("/usage?range=24h&page_size=30"),
    ])
      .then(([serviceData, settingsData, overviewData, tokenData, poolData, usageData]) => {
        setService(serviceData);
        setSettings(settingsData);
        setOverview(overviewData);
        setTokens(tokenData ?? []);
        setPools(poolData ?? []);
        setLogs(usageData.logs);
        setServiceForm({
          label: serviceData?.label ?? "",
          base_url: serviceData?.base_url ?? "",
          api_key: "",
        });
        setSettingsForm({
          external_url: settingsData.external_url ?? "",
          health_check_interval: settingsData.health_check_interval,
          request_timeout: settingsData.request_timeout,
          max_retry_attempts: settingsData.max_retry_attempts,
        });
      })
      .catch((err) => setError(err instanceof Error ? err.message : "Settings 加载失败"))
      .finally(() => setLoading(false));
  }

  useEffect(() => {
    load();
  }, []);

  const baseUrl = getApiBaseUrl(settingsForm.external_url || settings?.external_url);
  const globalToken = overview?.global_token ?? "";
  const filteredLogs = useMemo(() => {
    const items = logs?.items ?? [];
    return items.filter((item) => {
      if (logFilterPool !== "all" && String(item.pool_id) !== logFilterPool) {
        return false;
      }
      if (logFilterStatus === "success" && !item.success) {
        return false;
      }
      if (logFilterStatus === "error" && item.success) {
        return false;
      }
      return true;
    });
  }, [logFilterPool, logFilterStatus, logs]);

  async function saveService(event: FormEvent) {
    event.preventDefault();
    try {
      await api.put("/cpa/service", {
        label: serviceForm.label,
        base_url: serviceForm.base_url,
        api_key: serviceForm.api_key || undefined,
        enabled: true,
      });
      toast("CPA Service 已保存");
      load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "保存 CPA Service 失败", "error");
    }
  }

  async function saveSettings(event: FormEvent) {
    event.preventDefault();
    try {
      await api.put("/settings", settingsForm);
      toast("全局配置已保存");
      load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "保存配置失败", "error");
    }
  }

  async function createGlobalToken() {
    try {
      await api.post("/tokens", {
        name: `global-${Date.now()}`,
        pool_id: null,
        enabled: true,
      });
      toast("新的全局 Token 已创建");
      load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "创建全局 Token 失败", "error");
    }
  }

  async function testService() {
    try {
      const result = await api.post<{ reachable: boolean; latency_ms: number; error: string }>("/cpa/service/test", {});
      toast(result.reachable ? `连接正常 ${result.latency_ms}ms` : result.error || "连接失败", result.reachable ? "success" : "error");
    } catch (err) {
      toast(err instanceof Error ? err.message : "测试失败", "error");
    }
  }

  async function deleteService() {
    try {
      await api.delete("/cpa/service");
      toast("CPA Service 已删除");
      setDeleteServiceOpen(false);
      load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "删除失败", "error");
    }
  }

  if (loading) {
    return (
      <div className="space-y-8">
        <Skeleton className="h-40 rounded-[2rem]" />
        <Skeleton className="h-72 rounded-[1.8rem]" />
        <Skeleton className="h-72 rounded-[1.8rem]" />
      </div>
    );
  }

  if (error) {
    return <ErrorState message={error} onRetry={load} />;
  }

  return (
    <div className="space-y-8">
      <PageHeader
        eyebrow="Settings"
        title="系统配置"
        description="把连接、全局地址、Token 和请求日志收拢到一个页面，作为高级操作区。"
        actions={
          <>
            <Button variant="outline" onClick={load}>
              <RefreshCw className="size-4" />
              刷新
            </Button>
            <Button variant="outline" onClick={() => window.open("/admin/api/export", "_blank")}>
              <Download className="size-4" />
              导出配置
            </Button>
          </>
        }
      />

      <section className="surface-section grid gap-5 px-5 py-5 lg:grid-cols-2">
        <div className="space-y-4">
          <SectionHeading
            title="CPA Service"
            description="保留 v2 的服务配置能力，供 CPA 登录流程使用。"
          />
          <form className="space-y-3" onSubmit={saveService}>
            <div className="space-y-2">
              <label className="text-sm font-medium text-moon-700">Label</label>
              <Input
                value={serviceForm.label}
                onChange={(event) => setServiceForm((current) => ({ ...current, label: event.target.value }))}
              />
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium text-moon-700">Base URL</label>
              <Input
                value={serviceForm.base_url}
                onChange={(event) => setServiceForm((current) => ({ ...current, base_url: event.target.value }))}
              />
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium text-moon-700">API Key</label>
              <Input
                type="password"
                value={serviceForm.api_key}
                placeholder={service?.api_key_set ? "已保存，留空则不改" : ""}
                onChange={(event) => setServiceForm((current) => ({ ...current, api_key: event.target.value }))}
              />
            </div>
            <div className="flex flex-wrap gap-3">
              <Button type="submit">
                <Server className="size-4" />
                保存
              </Button>
              <Button type="button" variant="outline" onClick={testService}>
                测试连接
              </Button>
              {service ? (
                <Button type="button" variant="outline" onClick={() => setDeleteServiceOpen(true)}>
                  删除
                </Button>
              ) : null}
            </div>
          </form>
        </div>

        <div className="space-y-4">
          <SectionHeading
            title="Global Config"
            description="外部地址优先用于 Snippets 和 QR。"
          />
          <form className="space-y-3" onSubmit={saveSettings}>
            <div className="space-y-2">
              <label className="text-sm font-medium text-moon-700">External URL</label>
              <Input
                value={settingsForm.external_url}
                onChange={(event) => setSettingsForm((current) => ({ ...current, external_url: event.target.value }))}
                placeholder="https://your-domain.example"
              />
            </div>
            <div className="grid gap-3 sm:grid-cols-3">
              <div className="space-y-2">
                <label className="text-sm font-medium text-moon-700">Health Check</label>
                <Input
                  type="number"
                  value={settingsForm.health_check_interval}
                  onChange={(event) => setSettingsForm((current) => ({ ...current, health_check_interval: Number(event.target.value) }))}
                />
              </div>
              <div className="space-y-2">
                <label className="text-sm font-medium text-moon-700">Timeout</label>
                <Input
                  type="number"
                  value={settingsForm.request_timeout}
                  onChange={(event) => setSettingsForm((current) => ({ ...current, request_timeout: Number(event.target.value) }))}
                />
              </div>
              <div className="space-y-2">
                <label className="text-sm font-medium text-moon-700">Retries</label>
                <Input
                  type="number"
                  value={settingsForm.max_retry_attempts}
                  onChange={(event) => setSettingsForm((current) => ({ ...current, max_retry_attempts: Number(event.target.value) }))}
                />
              </div>
            </div>
            <Button type="submit">保存配置</Button>
          </form>
        </div>
      </section>

      <section className="surface-section grid gap-5 px-5 py-5 lg:grid-cols-[minmax(0,1fr)_20rem]">
        <div className="space-y-4">
          <SectionHeading
            title="Global Token"
            description="全局 Token 默认可访问全部 Pool。"
            action={
              <div className="flex gap-3">
                <Button variant="outline" onClick={() => setGlobalSnippetOpen(true)}>
                  <KeyRound className="size-4" />
                  Env Snippets
                </Button>
                <Button onClick={createGlobalToken}>
                  <Sparkles className="size-4" />
                  新建全局 Token
                </Button>
              </div>
            }
          />
          <div className="surface-outline px-4 py-4">
            <p className="text-xs uppercase tracking-[0.18em] text-moon-400">Current Token</p>
            <p className="mt-2 break-all text-sm text-moon-700">{maskToken(globalToken)}</p>
            <div className="mt-3 flex flex-wrap gap-3">
              <CopyButton value={globalToken} label="复制" className="px-0" />
              <CopyButton value={baseUrl} label="复制 API 地址" className="px-0" />
            </div>
          </div>
          <div className="space-y-2">
            {tokens.filter((item) => item.is_global).map((token) => (
              <div key={token.id} className="surface-outline flex items-center justify-between gap-3 px-4 py-3">
                <div>
                  <p className="text-sm font-semibold text-moon-800">{token.name}</p>
                  <p className="text-xs text-moon-400">最后使用 {relativeTime(token.last_used_at)}</p>
                </div>
                <span className="text-xs text-moon-500">{token.enabled ? "启用中" : "已停用"}</span>
              </div>
            ))}
          </div>
        </div>

        <div className="space-y-4">
          <SectionHeading title="System Info" description="便于快速检查当前实例状态。" />
          <div className="surface-outline space-y-3 px-4 py-4 text-sm text-moon-500">
            <div className="flex items-center justify-between gap-3">
              <span>API Base</span>
              <span className="text-right text-moon-700">{baseUrl}</span>
            </div>
            <div className="flex items-center justify-between gap-3">
              <span>Pools</span>
              <span className="text-moon-700">{pools.length}</span>
            </div>
            <div className="flex items-center justify-between gap-3">
              <span>今日请求</span>
              <span className="text-moon-700">{compact(overview?.requests_today ?? 0)}</span>
            </div>
            <div className="flex items-center justify-between gap-3">
              <span>成功率</span>
              <span className="text-moon-700">{pct(overview?.success_rate_today ?? 0)}</span>
            </div>
          </div>
        </div>
      </section>

      <section className="surface-section px-5 py-5">
        <SectionHeading title="Request Logs" description="作为高级调试工具，默认展示最近 24 小时请求。" />
        <div className="mt-5 flex flex-wrap gap-3">
          <select
            value={logFilterPool}
            onChange={(event) => setLogFilterPool(event.target.value)}
            className="rounded-full border border-moon-200/70 bg-white/82 px-3 py-2 text-sm text-moon-600"
          >
            <option value="all">全部 Pool</option>
            {pools.map((pool) => (
              <option key={pool.id} value={String(pool.id)}>
                {pool.label}
              </option>
            ))}
          </select>
          <select
            value={logFilterStatus}
            onChange={(event) => setLogFilterStatus(event.target.value)}
            className="rounded-full border border-moon-200/70 bg-white/82 px-3 py-2 text-sm text-moon-600"
          >
            <option value="all">全部状态</option>
            <option value="success">成功</option>
            <option value="error">失败</option>
          </select>
        </div>
        <div className="mt-5 overflow-x-auto rounded-[1.4rem] border border-moon-200/60">
          <table className="min-w-full text-left text-sm">
            <thead className="bg-moon-100/60 text-xs uppercase tracking-[0.16em] text-moon-400">
              <tr>
                <th className="px-4 py-3">Request</th>
                <th className="px-4 py-3">Pool</th>
                <th className="px-4 py-3">Model</th>
                <th className="px-4 py-3">Status</th>
                <th className="px-4 py-3">Latency</th>
                <th className="px-4 py-3">Tokens</th>
                <th className="px-4 py-3">Time</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-moon-200/50 bg-white/66">
              {filteredLogs.map((item) => (
                <tr key={item.id}>
                  <td className="px-4 py-3 text-moon-700">{item.request_id.slice(0, 8)}</td>
                  <td className="px-4 py-3 text-moon-500">
                    {pools.find((pool) => pool.id === item.pool_id)?.label ?? `#${item.pool_id}`}
                  </td>
                  <td className="px-4 py-3 text-moon-500">{item.model_actual || item.model_requested}</td>
                  <td className="px-4 py-3 text-moon-500">{item.success ? "OK" : item.status_code}</td>
                  <td className="px-4 py-3 text-moon-500">{latency(item.latency_ms)}</td>
                  <td className="px-4 py-3 text-moon-500">
                    {(item.input_tokens ?? 0) + (item.output_tokens ?? 0)}
                  </td>
                  <td className="px-4 py-3 text-moon-400">{relativeTime(item.created_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>

      <EnvSnippetsDialog
        open={globalSnippetOpen}
        onOpenChange={setGlobalSnippetOpen}
        title="Global Env Snippets"
        baseUrl={baseUrl}
        token={globalToken}
        model={pools[0]?.models?.[0]}
      />
      <ConfirmDialog
        open={deleteServiceOpen}
        onOpenChange={setDeleteServiceOpen}
        title="删除 CPA Service"
        description="删除后将无法继续通过 CPA 进行新登录。现有账号不会被删除。"
        onConfirm={deleteService}
      />
    </div>
  );
}
