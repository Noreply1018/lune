import { type FormEvent, useEffect, useState } from "react";
import StatusBadge from "@/components/StatusBadge";
import DataTable, { type Column } from "@/components/DataTable";
import PageHeader from "@/components/PageHeader";
import SectionHeading from "@/components/SectionHeading";
import ConfirmDialog from "@/components/ConfirmDialog";
import { api } from "@/lib/api";
import { toast } from "@/components/Feedback";
import { relativeTime } from "@/lib/fmt";
import type { CpaService, CpaServiceTestResult, Account } from "@/lib/types";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import type { RemoteAccount } from "@/lib/types";
import { Server, TestTube2, Trash2, RefreshCw, AlertTriangle, Loader2 } from "lucide-react";

interface ServiceForm {
  label: string;
  base_url: string;
  api_key: string;
}

const emptyForm: ServiceForm = {
  label: "",
  base_url: "",
  api_key: "",
};

function expiryBadge(date: string | null) {
  if (!date) return null;
  const now = Date.now();
  const exp = new Date(date).getTime();
  if (Number.isNaN(exp)) return null;
  const diff = exp - now;
  if (diff <= 0) return <span className="ml-1 inline-flex items-center gap-0.5 rounded-md bg-red-100 px-1.5 py-0.5 text-[10px] font-medium text-red-700"><AlertTriangle className="size-3" />已过期</span>;
  if (diff <= 24 * 60 * 60 * 1000) return <span className="ml-1 inline-flex items-center gap-0.5 rounded-md bg-red-100 px-1.5 py-0.5 text-[10px] font-medium text-red-700"><AlertTriangle className="size-3" />今日到期</span>;
  if (diff <= 7 * 24 * 60 * 60 * 1000) return <span className="ml-1 inline-flex items-center gap-0.5 rounded-md bg-yellow-100 px-1.5 py-0.5 text-[10px] font-medium text-yellow-700"><AlertTriangle className="size-3" />即将到期</span>;
  return null;
}

const linkedColumns: Column<Account>[] = [
  {
    key: "label",
    header: "标签",
    render: (r) => <span className="font-medium text-moon-800">{r.label}</span>,
    tone: "primary",
  },
  {
    key: "subtype",
    header: "类型",
    render: (r) => (
      <span className="text-sm text-moon-600">
        {r.cpa_account_key ? "凭据型账号" : "Provider 通道"}
      </span>
    ),
  },
  {
    key: "provider",
    header: "提供商",
    render: (r) => (
      <span className="rounded-md bg-lunar-100/60 px-2 py-0.5 text-xs font-medium text-lunar-700">
        {r.cpa_provider}
      </span>
    ),
  },
  {
    key: "email",
    header: "邮箱",
    render: (r) => r.cpa_email ? <span className="text-sm text-moon-600">{r.cpa_email}</span> : <span className="text-sm text-moon-400">-</span>,
  },
  {
    key: "plan",
    header: "套餐",
    render: (r) => r.cpa_plan_type ? <span className="text-sm text-moon-600">{r.cpa_plan_type}</span> : <span className="text-sm text-moon-400">-</span>,
  },
  {
    key: "status",
    header: "状态",
    render: (r) => (
      <span className="inline-flex items-center">
        <StatusBadge status={r.enabled ? r.status : "disabled"} />
        {r.cpa_expired_at && expiryBadge(r.cpa_expired_at)}
      </span>
    ),
    tone: "status",
  },
];

export default function CpaServicePage() {
  const [service, setService] = useState<CpaService | null | undefined>(undefined);
  const [linkedAccounts, setLinkedAccounts] = useState<Account[]>([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState<ServiceForm>(emptyForm);
  const [testing, setTesting] = useState(false);
  const [showDelete, setShowDelete] = useState(false);
  const [showSync, setShowSync] = useState(false);
  const [remoteAccounts, setRemoteAccounts] = useState<RemoteAccount[]>([]);
  const [syncLoading, setSyncLoading] = useState(false);
  const [selectedKeys, setSelectedKeys] = useState<Set<string>>(new Set());

  function load() {
    setLoading(true);
    let cancelled = false;

    Promise.all([
      api.get<CpaService | null>("/cpa/service").catch(() => {
        if (!cancelled) toast("加载 CPA 服务失败", "error");
        return null;
      }),
      api.get<Account[]>("/accounts").catch(() => [] as Account[]),
    ]).then(([svc, accounts]) => {
      if (cancelled) return;
      setService(svc ?? null);
      setLinkedAccounts(
        (accounts ?? []).filter((a) => a.source_kind === "cpa"),
      );
    }).finally(() => {
      if (!cancelled) setLoading(false);
    });

    return () => { cancelled = true; };
  }

  useEffect(() => {
    const cancel = load();
    return cancel;
  }, []);

  function openEdit() {
    if (service) {
      setForm({
        label: service.label,
        base_url: service.base_url,
        api_key: "",
      });
    } else {
      setForm(emptyForm);
    }
    setShowForm(true);
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    try {
      await api.put("/cpa/service", {
        label: form.label,
        base_url: form.base_url,
        api_key: form.api_key || undefined,
        enabled: true,
      });
      toast(service ? "CPA 服务已更新" : "CPA 服务已配置");
      setShowForm(false);
      load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "操作失败", "error");
    }
  }

  async function testConnection() {
    setTesting(true);
    try {
      const result = await api.post<CpaServiceTestResult>("/cpa/service/test", {});
      if (result.reachable) {
        toast(
          `已连接 (${result.latency_ms}ms) - ${result.providers?.length ?? 0} 个提供商可用`,
        );
      } else {
        toast(result.error || "连接失败", "error");
      }
    } catch (err) {
      toast(err instanceof Error ? err.message : "测试失败", "error");
    } finally {
      setTesting(false);
    }
  }

  async function toggleEnabled() {
    if (!service) return;
    const next = !service.enabled;
    try {
      await api.post(`/cpa/service/${next ? "enable" : "disable"}`);
      toast(next ? "CPA 服务已启用" : "CPA 服务已停用");
      load();
    } catch {
      toast("更新 CPA 服务失败", "error");
    }
  }

  async function confirmDelete() {
    try {
      await api.delete("/cpa/service");
      toast("CPA 服务已删除");
      setShowDelete(false);
      load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "删除失败", "error");
      setShowDelete(false);
    }
  }

  if (loading) {
    return (
      <div className="space-y-8">
        <Skeleton className="h-24 rounded-[1.5rem]" />
        <Skeleton className="h-64 rounded-[1.5rem]" />
      </div>
    );
  }

  return (
    <div className="space-y-8">
      <PageHeader
        eyebrow="配置"
        title="CPA 服务"
        description={
          service
            ? "管理 CPA 控制面，负责发现账号、刷新凭据，并支撑 CPA 托管的路由单元。"
            : "连接一个 CPA（cli-proxy-api）实例，用于发现账号并创建 CPA 托管单元。"
        }
        actions={
          !service ? (
            <Button size="sm" onClick={openEdit}>
              <Server className="size-4" />
              配置 CPA 服务
            </Button>
          ) : undefined
        }
      />

      {service ? (
        <>
          <section className="overflow-hidden rounded-[1.6rem] border border-moon-200/70 bg-white/85">
            <div className="p-6">
              <div className="flex items-start justify-between">
                <div className="space-y-1">
                  <div className="flex items-center gap-3">
                    <h3 className="text-lg font-semibold text-moon-800">
                      {service.label}
                    </h3>
                    <StatusBadge
                      status={
                        service.status === "healthy"
                          ? "healthy"
                          : service.status === "error"
                            ? "error"
                            : "degraded"
                      }
                      label={service.status}
                    />
                    {!service.enabled && (
                      <span className="rounded-md bg-moon-200/60 px-2 py-0.5 text-xs font-medium text-moon-500">
                        已停用
                      </span>
                    )}
                  </div>
                </div>
              </div>

              <div className="mt-5 grid gap-x-8 gap-y-3 sm:grid-cols-2">
                <div>
                  <p className="text-xs uppercase tracking-[0.18em] text-moon-400">
                    Base URL
                  </p>
                  <code className="mt-1 block text-sm text-moon-700">
                    {service.base_url}
                  </code>
                </div>
                <div>
                  <p className="text-xs uppercase tracking-[0.18em] text-moon-400">
                    API Key
                  </p>
                  <p className="mt-1 text-sm text-moon-700">
                    {service.api_key_set ? service.api_key_masked : "未设置"}
                  </p>
                </div>
                <div>
                  <p className="text-xs uppercase tracking-[0.18em] text-moon-400">
                    状态
                  </p>
                  <p className="mt-1 text-sm text-moon-700">
                    {service.status === "healthy" ? "正常" : service.status === "error" ? "异常" : "降级"}
                  </p>
                </div>
                <div>
                  <p className="text-xs uppercase tracking-[0.18em] text-moon-400">
                    最近检查
                  </p>
                  <p className="mt-1 text-sm text-moon-700">
                    {service.last_checked_at
                      ? relativeTime(service.last_checked_at)
                      : "从未"}
                  </p>
                </div>
                {service.last_error && (
                  <div className="sm:col-span-2">
                    <p className="text-xs uppercase tracking-[0.18em] text-moon-400">
                      最近错误
                    </p>
                    <p className="mt-1 text-sm text-status-red">
                      {service.last_error}
                    </p>
                  </div>
                )}
              </div>

              <div className="mt-6 flex flex-wrap gap-2">
                <Button
                  size="sm"
                  variant="outline"
                  onClick={testConnection}
                  disabled={testing}
                >
                  <TestTube2 className="size-4" />
                  {testing ? "测试中..." : "测试连接"}
                </Button>
                <Button size="sm" variant="outline" onClick={openSyncDialog}>
                  <RefreshCw className="size-4" />
                  发现账号
                </Button>
                <Button size="sm" variant="outline" onClick={openEdit}>
                  编辑
                </Button>
                <Button size="sm" variant="outline" onClick={toggleEnabled}>
                  {service.enabled ? "停用" : "启用"}
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  className="text-destructive"
                  onClick={() => setShowDelete(true)}
                >
                  <Trash2 className="size-4" />
                  删除
                </Button>
              </div>
            </div>
          </section>

          <section className="space-y-4">
            <SectionHeading
              title="CPA 托管账号"
              description={`当前有 ${linkedAccounts.length} 个路由单元挂载到该 CPA 服务下，包括 Provider Channel 和凭据型账号。`}
            />
            <div className="overflow-hidden rounded-[1.6rem] border border-moon-200/70 bg-white/85">
              <DataTable
                columns={linkedColumns}
                rows={linkedAccounts}
                rowKey={(r) => r.id}
                empty="当前没有挂载任何 CPA 账号"
              />
            </div>
          </section>
        </>
      ) : (
        <section className="rounded-[1.6rem] border border-moon-200/70 bg-white/85 p-8">
          <div className="text-center">
            <Server className="mx-auto size-12 text-moon-300" />
            <h3 className="mt-4 text-lg font-semibold text-moon-800">尚未配置 CPA 服务</h3>
            <p className="mt-2 text-sm text-moon-500">
              CPA（CLI Proxy API）是外部代理服务，支持通过 ChatGPT Plus/Pro、Claude 等订阅账号访问 LLM 提供商。部署 CPA 后，可在此页面连接并管理。
            </p>
          </div>

          <div className="mt-6 rounded-2xl border border-moon-200/70 bg-moon-50/70 p-5 text-left text-sm text-moon-600">
            <p className="font-medium text-moon-700">部署指引</p>
            <ul className="mt-2 list-inside list-disc space-y-1">
              <li>
                使用 Docker Compose 一键启动：
                <code className="ml-1 rounded bg-moon-100 px-1.5 py-0.5 text-xs text-moon-700">docker compose up -d</code>
              </li>
              <li>
                CPA 镜像：
                <code className="ml-1 rounded bg-moon-100 px-1.5 py-0.5 text-xs text-moon-700">eceasy/cli-proxy-api</code>
              </li>
              <li>
                Docker Compose 部署会自动配置默认 CPA 连接，无需手动操作
              </li>
            </ul>

            <p className="mt-4 font-medium text-moon-700">Base URL 参考</p>
            <div className="mt-2 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1">
              <span className="text-moon-500">Docker Compose 部署</span>
              <code className="text-moon-700">http://cpa:8317</code>
              <span className="text-moon-500">本地开发</span>
              <code className="text-moon-700">http://127.0.0.1:8317</code>
            </div>

            <p className="mt-4 text-moon-500">
              如需手动配置外部 CPA 实例，请确保 CPA 服务已启动后再点击下方按钮。
            </p>
          </div>

          <div className="mt-6 text-center">
            <Button size="sm" onClick={openEdit}>
              配置 CPA 服务
            </Button>
          </div>
        </section>
      )}

      <Dialog open={showForm} onOpenChange={setShowForm}>
        <DialogContent className="max-w-lg">
          <form onSubmit={handleSubmit}>
            <DialogHeader>
              <DialogTitle>
                {service ? "编辑 CPA 服务" : "配置 CPA 服务"}
              </DialogTitle>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="cpa-label">标签</Label>
                <Input
                  id="cpa-label"
                  value={form.label}
                  onChange={(e) => setForm({ ...form, label: e.target.value })}
                  required
                  placeholder="本地 CPA"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="cpa-url">Base URL</Label>
                <Input
                  id="cpa-url"
                  value={form.base_url}
                  onChange={(e) =>
                    setForm({ ...form, base_url: e.target.value })
                  }
                  required
                  placeholder="http://127.0.0.1:8317"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="cpa-key">API Key</Label>
                <Input
                  id="cpa-key"
                  type="password"
                  value={form.api_key}
                  onChange={(e) =>
                    setForm({ ...form, api_key: e.target.value })
                  }
                  placeholder={
                    service ? "留空则保留当前密钥" : "sk-cpa-default"
                  }
                />
              </div>
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setShowForm(false)}
              >
                取消
              </Button>
              <Button type="submit">{service ? "保存" : "配置"}</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={showDelete}
        onOpenChange={setShowDelete}
        title="删除 CPA 服务"
        description="确认删除当前 CPA 服务吗？此操作不可撤销。"
        onConfirm={confirmDelete}
      />

      {/* Sync from CPA Dialog */}
      <Dialog open={showSync} onOpenChange={setShowSync}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>从 CPA 发现账号</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="rounded-2xl border border-moon-200/70 bg-moon-50/70 p-4 text-sm text-moon-500">
              该操作会扫描 CPA 凭据目录，并将选中的凭据型账号导入当前工作区。
            </div>
            {syncLoading && remoteAccounts.length === 0 ? (
              <div className="flex items-center justify-center py-8">
                <Loader2 className="size-6 animate-spin text-moon-400" />
              </div>
            ) : remoteAccounts.length === 0 ? (
              <p className="py-4 text-center text-sm text-moon-500">在 cpa-auth 目录中未发现可导入账号。</p>
            ) : (
              <div className="max-h-64 space-y-2 overflow-y-auto">
                {remoteAccounts.map((ra) => (
                  <label
                    key={ra.account_key}
                    className={`flex items-center gap-3 rounded-lg border p-3 transition ${ra.already_imported ? "cursor-not-allowed border-moon-100 bg-moon-50 opacity-60" : "cursor-pointer border-moon-200 hover:border-lunar-400"}`}
                  >
                    <input
                      type="checkbox"
                      disabled={ra.already_imported}
                      checked={selectedKeys.has(ra.account_key)}
                      onChange={(e) => {
                        const next = new Set(selectedKeys);
                        if (e.target.checked) next.add(ra.account_key);
                        else next.delete(ra.account_key);
                        setSelectedKeys(next);
                      }}
                      className="size-4"
                    />
                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-medium text-moon-800">{ra.email}</p>
                      <p className="text-xs text-moon-500">
                        {ra.provider} - {ra.plan_type || "未知套餐"}
                        {ra.expired_at && ` | 到期：${new Date(ra.expired_at).toLocaleDateString()}`}
                      </p>
                    </div>
                    {ra.already_imported && (
                      <span className="rounded-md bg-moon-100 px-2 py-0.5 text-[10px] font-medium text-moon-500">已导入</span>
                    )}
                  </label>
                ))}
              </div>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowSync(false)}>取消</Button>
            <Button
              disabled={selectedKeys.size === 0 || syncLoading}
              onClick={handleBatchImport}
            >
              {syncLoading ? <Loader2 className="size-4 animate-spin" /> : `导入（${selectedKeys.size}）`}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );

  function openSyncDialog() {
    setRemoteAccounts([]);
    setSelectedKeys(new Set());
    setSyncLoading(true);
    setShowSync(true);

    api.get<RemoteAccount[]>("/cpa/service/remote-accounts")
      .then((accs) => setRemoteAccounts(accs ?? []))
      .catch((err) => toast(err instanceof Error ? err.message : "扫描账号失败", "error"))
      .finally(() => setSyncLoading(false));
  }

  async function handleBatchImport() {
    if (!service || selectedKeys.size === 0) return;
    setSyncLoading(true);
    try {
      const result = await api.post<{ imported: number; skipped: number; errors: string[] }>("/accounts/cpa/import/batch", {
        service_id: service.id,
        account_keys: [...selectedKeys],
      });
      toast(`已导入 ${result.imported} 个，跳过 ${result.skipped} 个${result.errors?.length ? `，${result.errors.length} 个失败` : ""}`);
      setShowSync(false);
      load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "导入失败", "error");
    } finally {
      setSyncLoading(false);
    }
  }
}
