import { type FormEvent, useEffect, useState } from "react";
import ConfirmDialog from "@/components/ConfirmDialog";
import DataTable, { type Column } from "@/components/DataTable";
import PageHeader from "@/components/PageHeader";
import SectionHeading from "@/components/SectionHeading";
import StatusBadge from "@/components/StatusBadge";
import { api } from "@/lib/api";
import { toast } from "@/components/Feedback";
import { relativeTime } from "@/lib/fmt";
import type { Account, CpaService, CpaServiceTestResult, RemoteAccount } from "@/lib/types";
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
import {
  AlertTriangle,
  Loader2,
  RefreshCw,
  Server,
  TestTube2,
  Trash2,
} from "lucide-react";

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
  if (diff <= 0) {
    return (
      <span className="ml-1 inline-flex items-center gap-1 rounded-md bg-red-100 px-1.5 py-0.5 text-[10px] font-medium text-red-700">
        <AlertTriangle className="size-3" />
        已过期
      </span>
    );
  }
  if (diff <= 24 * 60 * 60 * 1000) {
    return (
      <span className="ml-1 inline-flex items-center gap-1 rounded-md bg-red-100 px-1.5 py-0.5 text-[10px] font-medium text-red-700">
        <AlertTriangle className="size-3" />
        今日到期
      </span>
    );
  }
  if (diff <= 7 * 24 * 60 * 60 * 1000) {
    return (
      <span className="ml-1 inline-flex items-center gap-1 rounded-md bg-yellow-100 px-1.5 py-0.5 text-[10px] font-medium text-yellow-700">
        <AlertTriangle className="size-3" />
        即将到期
      </span>
    );
  }
  return null;
}

const linkedColumns: Column<Account>[] = [
  {
    key: "label",
    header: "标签",
    render: (row) => <span className="font-medium text-moon-800">{row.label}</span>,
    tone: "primary",
  },
  {
    key: "provider",
    header: "提供商",
    render: (row) => (
      <span className="rounded-md bg-lunar-100/60 px-2 py-0.5 text-xs font-medium text-lunar-700">
        {row.cpa_provider}
      </span>
    ),
  },
  {
    key: "type",
    header: "类型",
    render: (row) => (
      <span className="text-sm text-moon-600">
        {row.cpa_account_key ? "凭据型账号" : "Provider 通道"}
      </span>
    ),
  },
  {
    key: "email",
    header: "邮箱",
    render: (row) =>
      row.cpa_email ? (
        <span className="text-sm text-moon-600">{row.cpa_email}</span>
      ) : (
        <span className="text-sm text-moon-400">-</span>
      ),
  },
  {
    key: "status",
    header: "状态",
    render: (row) => (
      <span className="inline-flex items-center">
        <StatusBadge status={row.enabled ? row.status : "disabled"} />
        {row.cpa_expired_at && expiryBadge(row.cpa_expired_at)}
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
      setLinkedAccounts((accounts ?? []).filter((account) => account.source_kind === "cpa"));
    }).finally(() => {
      if (!cancelled) setLoading(false);
    });

    return () => {
      cancelled = true;
    };
  }

  useEffect(() => {
    const cancel = load();
    return cancel;
  }, []);

  function openEdit() {
    if (service) {
      setForm({ label: service.label, base_url: service.base_url, api_key: "" });
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
        toast(`已连接 (${result.latency_ms}ms) - ${result.providers?.length ?? 0} 个提供商可用`);
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

  function openSyncDialog() {
    setRemoteAccounts([]);
    setSelectedKeys(new Set());
    setSyncLoading(true);
    setShowSync(true);

    api
      .get<RemoteAccount[]>("/cpa/service/remote-accounts")
      .then((accounts) => setRemoteAccounts(accounts ?? []))
      .catch((err) => toast(err instanceof Error ? err.message : "扫描账号失败", "error"))
      .finally(() => setSyncLoading(false));
  }

  async function handleBatchImport() {
    if (!service || selectedKeys.size === 0) return;
    setSyncLoading(true);
    try {
      const result = await api.post<{ imported: number; skipped: number; errors: string[] }>(
        "/accounts/cpa/import/batch",
        {
          service_id: service.id,
          account_keys: [...selectedKeys],
        },
      );
      toast(
        `已导入 ${result.imported} 个，跳过 ${result.skipped} 个${
          result.errors?.length ? `，${result.errors.length} 个失败` : ""
        }`,
      );
      setShowSync(false);
      load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "导入失败", "error");
    } finally {
      setSyncLoading(false);
    }
  }

  if (loading) {
    return (
      <div className="space-y-8">
        <Skeleton className="h-24 rounded-[1.5rem]" />
        <Skeleton className="h-72 rounded-[1.8rem]" />
        <Skeleton className="h-80 rounded-[1.8rem]" />
      </div>
    );
  }

  return (
    <div className="space-y-10">
      <PageHeader
        eyebrow="CPA Service / Control Plane"
        title="CPA 服务"
        description={
          service
            ? "查看控制平面状态、远端快照与托管账号。"
            : "连接一个 CPA 实例，用于发现账号并创建托管单元。"
        }
        meta={
          service ? (
            <>
              <span>{service.enabled ? "已启用" : "已停用"}</span>
              <span>{service.last_checked_at ? `最近检查 ${relativeTime(service.last_checked_at)}` : "尚未检查"}</span>
              <span>托管账号 {linkedAccounts.length}</span>
            </>
          ) : (
            <>
              <span>尚未配置</span>
            </>
          )
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
          <section className="grid gap-6 xl:grid-cols-[minmax(0,1.15fr)_minmax(320px,0.85fr)]">
            <div className="surface-section overflow-hidden">
              <div className="border-b border-moon-200/60 px-6 py-5">
                <div className="flex items-start justify-between gap-4">
                  <div>
                    <p className="eyebrow-label">服务健康</p>
                    <h2 className="mt-1 text-[1.15rem] font-semibold tracking-[-0.03em] text-moon-800">
                      {service.label}
                    </h2>
                    <p className="mt-2 text-sm text-moon-500">
                      控制平面服务的连接状态、凭据配置与最近检查信息。
                    </p>
                  </div>
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
                </div>
              </div>

              <div className="grid gap-4 px-6 py-5 md:grid-cols-2">
                <div className="rounded-[1.15rem] border border-white/72 bg-white/72 px-4 py-4">
                  <p className="kicker">Base URL</p>
                  <code className="mt-3 block text-sm text-moon-700">{service.base_url}</code>
                </div>
                <div className="rounded-[1.15rem] border border-white/72 bg-white/72 px-4 py-4">
                  <p className="kicker">API Key</p>
                  <p className="mt-3 text-sm text-moon-700">
                    {service.api_key_set ? service.api_key_masked : "未设置"}
                  </p>
                </div>
                <div className="rounded-[1.15rem] border border-white/72 bg-white/72 px-4 py-4">
                  <p className="kicker">状态</p>
                  <p className="mt-3 text-sm text-moon-700">
                    {service.status === "healthy" ? "正常" : service.status === "error" ? "异常" : "待检查"}
                  </p>
                </div>
                <div className="rounded-[1.15rem] border border-white/72 bg-white/72 px-4 py-4">
                  <p className="kicker">最近检查</p>
                  <p className="mt-3 text-sm text-moon-700">
                    {service.last_checked_at ? relativeTime(service.last_checked_at) : "从未"}
                  </p>
                </div>
              </div>

              {service.last_error && (
                <div className="px-6 pb-5">
                  <div className="rounded-[1.1rem] border border-red-200/70 bg-red-50/80 px-4 py-4 text-sm text-red-700">
                    {service.last_error}
                  </div>
                </div>
              )}

              <div className="flex flex-wrap gap-2 border-t border-moon-200/60 px-6 py-4">
                <Button size="sm" variant="outline" onClick={testConnection} disabled={testing}>
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
                <Button size="sm" variant="outline" className="text-destructive" onClick={() => setShowDelete(true)}>
                  <Trash2 className="size-4" />
                  删除
                </Button>
              </div>
            </div>

            <aside className="surface-card px-5 py-5">
              <p className="eyebrow-label">远端快照</p>
              <div className="mt-4 space-y-4">
                <div className="rounded-[1.15rem] border border-white/72 bg-white/72 px-4 py-4">
                  <p className="kicker">托管账号</p>
                  <p className="mt-3 text-[1.35rem] font-semibold tracking-[-0.04em] text-moon-800">
                    {linkedAccounts.length}
                  </p>
                </div>
                <div className="rounded-[1.15rem] border border-white/72 bg-white/72 px-4 py-4">
                  <p className="kicker">服务开关</p>
                  <p className="mt-3 text-sm font-medium text-moon-700">
                    {service.enabled ? "服务运行中" : "服务已停用"}
                  </p>
                </div>
                <div className="rounded-[1.15rem] border border-white/72 bg-white/72 px-4 py-4">
                  <p className="kicker">导入来源</p>
                  <p className="mt-3 text-sm font-medium text-moon-700">CPA Managed</p>
                </div>
              </div>
            </aside>
          </section>

          <section className="space-y-4">
            <SectionHeading
              title="托管账号"
              description={`当前有 ${linkedAccounts.length} 个路由单元挂在该控制平面下。`}
            />
            <div className="surface-card overflow-hidden">
              <DataTable
                columns={linkedColumns}
                rows={linkedAccounts}
                rowKey={(row) => row.id}
                empty="当前没有托管账号"
              />
            </div>
          </section>
        </>
      ) : (
        <section className="surface-section px-8 py-10 text-center">
          <Server className="mx-auto size-12 text-moon-300" />
          <h3 className="mt-4 text-lg font-semibold text-moon-800">尚未配置 CPA 服务</h3>
          <p className="mx-auto mt-2 max-w-2xl text-sm leading-7 text-moon-500">
            CPA 作为控制平面负责发现订阅账号、刷新凭据并托管上游执行单元。连接后，这里会变成服务状态页，而不是普通配置页。
          </p>

          <div className="mx-auto mt-6 max-w-3xl rounded-[1.5rem] border border-white/75 bg-white/72 px-5 py-5 text-left">
            <p className="text-sm font-medium text-moon-700">建议连接方式</p>
            <div className="mt-3 grid gap-3 md:grid-cols-2">
              <div className="rounded-[1.1rem] border border-moon-200/70 bg-moon-50/60 px-4 py-4 text-sm text-moon-500">
                <p className="font-medium text-moon-700">Docker Compose</p>
                <code className="mt-2 block rounded bg-white px-2 py-1 text-xs text-moon-700">
                  http://cpa:8317
                </code>
              </div>
              <div className="rounded-[1.1rem] border border-moon-200/70 bg-moon-50/60 px-4 py-4 text-sm text-moon-500">
                <p className="font-medium text-moon-700">本地开发</p>
                <code className="mt-2 block rounded bg-white px-2 py-1 text-xs text-moon-700">
                  http://127.0.0.1:8317
                </code>
              </div>
            </div>
          </div>

          <div className="mt-6">
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
              <DialogTitle>{service ? "编辑 CPA 服务" : "配置 CPA 服务"}</DialogTitle>
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
                  onChange={(e) => setForm({ ...form, base_url: e.target.value })}
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
                  onChange={(e) => setForm({ ...form, api_key: e.target.value })}
                  placeholder={service ? "留空则保留当前密钥" : "sk-cpa-default"}
                />
              </div>
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setShowForm(false)}>
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

      <Dialog open={showSync} onOpenChange={setShowSync}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>从 CPA 发现账号</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="rounded-2xl border border-moon-200/70 bg-moon-50/70 p-4 text-sm text-moon-500">
              扫描远端凭据目录，并将选中的账号导入当前工作区。
            </div>
            {syncLoading && remoteAccounts.length === 0 ? (
              <div className="flex items-center justify-center py-8">
                <Loader2 className="size-6 animate-spin text-moon-400" />
              </div>
            ) : remoteAccounts.length === 0 ? (
              <p className="py-4 text-center text-sm text-moon-500">未发现可导入账号。</p>
            ) : (
              <div className="max-h-64 space-y-2 overflow-y-auto">
                {remoteAccounts.map((account) => (
                  <label
                    key={account.account_key}
                    className={`flex items-center gap-3 rounded-lg border p-3 transition ${
                      account.already_imported
                        ? "cursor-not-allowed border-moon-100 bg-moon-50 opacity-60"
                        : "cursor-pointer border-moon-200 hover:border-lunar-400"
                    }`}
                  >
                    <input
                      type="checkbox"
                      disabled={account.already_imported}
                      checked={selectedKeys.has(account.account_key)}
                      onChange={(e) => {
                        const next = new Set(selectedKeys);
                        if (e.target.checked) next.add(account.account_key);
                        else next.delete(account.account_key);
                        setSelectedKeys(next);
                      }}
                      className="size-4"
                    />
                    <div className="min-w-0 flex-1">
                      <p className="text-sm font-medium text-moon-800">{account.email}</p>
                      <p className="text-xs text-moon-500">
                        {account.provider} - {account.plan_type || "未知套餐"}
                        {account.expired_at && ` | 到期：${new Date(account.expired_at).toLocaleDateString()}`}
                      </p>
                    </div>
                    {account.already_imported && (
                      <span className="rounded-md bg-moon-100 px-2 py-0.5 text-[10px] font-medium text-moon-500">
                        已导入
                      </span>
                    )}
                  </label>
                ))}
              </div>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowSync(false)}>
              取消
            </Button>
            <Button disabled={selectedKeys.size === 0 || syncLoading} onClick={handleBatchImport}>
              {syncLoading ? <Loader2 className="size-4 animate-spin" /> : `导入（${selectedKeys.size}）`}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
