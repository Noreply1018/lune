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
  if (diff <= 0) return <span className="ml-1 inline-flex items-center gap-0.5 rounded-md bg-red-100 px-1.5 py-0.5 text-[10px] font-medium text-red-700"><AlertTriangle className="size-3" />Expired</span>;
  if (diff <= 24 * 60 * 60 * 1000) return <span className="ml-1 inline-flex items-center gap-0.5 rounded-md bg-red-100 px-1.5 py-0.5 text-[10px] font-medium text-red-700"><AlertTriangle className="size-3" />Expiring today</span>;
  if (diff <= 7 * 24 * 60 * 60 * 1000) return <span className="ml-1 inline-flex items-center gap-0.5 rounded-md bg-yellow-100 px-1.5 py-0.5 text-[10px] font-medium text-yellow-700"><AlertTriangle className="size-3" />Expiring soon</span>;
  return null;
}

const linkedColumns: Column<Account>[] = [
  {
    key: "label",
    header: "Label",
    render: (r) => <span className="font-medium text-moon-800">{r.label}</span>,
    tone: "primary",
  },
  {
    key: "provider",
    header: "Provider",
    render: (r) => (
      <span className="rounded-md bg-lunar-100/60 px-2 py-0.5 text-xs font-medium text-lunar-700">
        {r.cpa_provider}
      </span>
    ),
  },
  {
    key: "email",
    header: "Email",
    render: (r) => r.cpa_email ? <span className="text-sm text-moon-600">{r.cpa_email}</span> : <span className="text-sm text-moon-400">-</span>,
  },
  {
    key: "plan",
    header: "Plan",
    render: (r) => r.cpa_plan_type ? <span className="text-sm text-moon-600">{r.cpa_plan_type}</span> : <span className="text-sm text-moon-400">-</span>,
  },
  {
    key: "status",
    header: "Status",
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
    Promise.all([
      api.get<CpaService | null>("/cpa/service"),
      api.get<Account[]>("/accounts"),
    ])
      .then(([svc, accounts]) => {
        setService(svc ?? null);
        setLinkedAccounts(
          (accounts ?? []).filter((a) => a.source_kind === "cpa"),
        );
      })
      .catch(() => toast("Failed to load CPA service", "error"))
      .finally(() => setLoading(false));
  }

  useEffect(load, []);

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
      toast(service ? "CPA service updated" : "CPA service configured");
      setShowForm(false);
      load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "Operation failed", "error");
    }
  }

  async function testConnection() {
    setTesting(true);
    try {
      const result = await api.post<CpaServiceTestResult>("/cpa/service/test", {});
      if (result.reachable) {
        toast(
          `Connected (${result.latency_ms}ms) - ${result.providers?.length ?? 0} providers available`,
        );
      } else {
        toast(result.error || "Connection failed", "error");
      }
    } catch (err) {
      toast(err instanceof Error ? err.message : "Test failed", "error");
    } finally {
      setTesting(false);
    }
  }

  async function toggleEnabled() {
    if (!service) return;
    const next = !service.enabled;
    try {
      await api.post(`/cpa/service/${next ? "enable" : "disable"}`);
      toast(next ? "CPA service enabled" : "CPA service disabled");
      load();
    } catch {
      toast("Failed to update CPA service", "error");
    }
  }

  async function confirmDelete() {
    try {
      await api.delete("/cpa/service");
      toast("CPA service removed");
      setShowDelete(false);
      load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "Failed to delete", "error");
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
        eyebrow="Configure"
        title="CPA Service"
        description={
          service
            ? "Manage your CPA (cli-proxy-api) connection."
            : "Connect to a CPA (cli-proxy-api) instance for GPT/Codex account access."
        }
        actions={
          !service ? (
            <Button size="sm" onClick={openEdit}>
              <Server className="size-4" />
              Configure CPA Service
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
                        Disabled
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
                    {service.api_key_set ? service.api_key_masked : "Not set"}
                  </p>
                </div>
                <div>
                  <p className="text-xs uppercase tracking-[0.18em] text-moon-400">
                    Status
                  </p>
                  <p className="mt-1 text-sm text-moon-700">{service.status}</p>
                </div>
                <div>
                  <p className="text-xs uppercase tracking-[0.18em] text-moon-400">
                    Last Check
                  </p>
                  <p className="mt-1 text-sm text-moon-700">
                    {service.last_checked_at
                      ? relativeTime(service.last_checked_at)
                      : "Never"}
                  </p>
                </div>
                {service.last_error && (
                  <div className="sm:col-span-2">
                    <p className="text-xs uppercase tracking-[0.18em] text-moon-400">
                      Last Error
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
                  {testing ? "Testing..." : "Test Connection"}
                </Button>
                <Button size="sm" variant="outline" onClick={openSyncDialog}>
                  <RefreshCw className="size-4" />
                  Sync from CPA
                </Button>
                <Button size="sm" variant="outline" onClick={openEdit}>
                  Edit
                </Button>
                <Button size="sm" variant="outline" onClick={toggleEnabled}>
                  {service.enabled ? "Disable" : "Enable"}
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  className="text-destructive"
                  onClick={() => setShowDelete(true)}
                >
                  <Trash2 className="size-4" />
                  Remove
                </Button>
              </div>
            </div>
          </section>

          <section className="space-y-4">
            <SectionHeading
              title="Linked Accounts"
              description={`${linkedAccounts.length} provider channel${linkedAccounts.length === 1 ? "" : "s"} using this service.`}
            />
            <div className="overflow-hidden rounded-[1.6rem] border border-moon-200/70 bg-white/85">
              <DataTable
                columns={linkedColumns}
                rows={linkedAccounts}
                rowKey={(r) => r.id}
                empty="No CPA accounts linked"
              />
            </div>
          </section>
        </>
      ) : (
        <section className="rounded-[1.6rem] border border-moon-200/70 bg-white/85 p-10 text-center">
          <Server className="mx-auto size-12 text-moon-300" />
          <p className="mt-4 text-moon-500">No CPA service configured.</p>
          <Button size="sm" className="mt-4" onClick={openEdit}>
            Configure CPA Service
          </Button>
        </section>
      )}

      <Dialog open={showForm} onOpenChange={setShowForm}>
        <DialogContent className="max-w-lg">
          <form onSubmit={handleSubmit}>
            <DialogHeader>
              <DialogTitle>
                {service ? "Edit CPA Service" : "Configure CPA Service"}
              </DialogTitle>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="cpa-label">Label</Label>
                <Input
                  id="cpa-label"
                  value={form.label}
                  onChange={(e) => setForm({ ...form, label: e.target.value })}
                  required
                  placeholder="Local CPA"
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
                    service ? "Leave empty to keep current" : "sk-cpa-default"
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
                Cancel
              </Button>
              <Button type="submit">{service ? "Save" : "Configure"}</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={showDelete}
        onOpenChange={setShowDelete}
        title="Remove CPA Service"
        description="Are you sure you want to remove the CPA service? This cannot be undone."
        onConfirm={confirmDelete}
      />

      {/* Sync from CPA Dialog */}
      <Dialog open={showSync} onOpenChange={setShowSync}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>Sync from CPA</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-4">
            {syncLoading && remoteAccounts.length === 0 ? (
              <div className="flex items-center justify-center py-8">
                <Loader2 className="size-6 animate-spin text-moon-400" />
              </div>
            ) : remoteAccounts.length === 0 ? (
              <p className="py-4 text-center text-sm text-moon-500">No accounts found in cpa-auth directory.</p>
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
                        {ra.provider} - {ra.plan_type || "unknown"}
                        {ra.expired_at && ` | Expires: ${new Date(ra.expired_at).toLocaleDateString()}`}
                      </p>
                    </div>
                    {ra.already_imported && (
                      <span className="rounded-md bg-moon-100 px-2 py-0.5 text-[10px] font-medium text-moon-500">Imported</span>
                    )}
                  </label>
                ))}
              </div>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowSync(false)}>Cancel</Button>
            <Button
              disabled={selectedKeys.size === 0 || syncLoading}
              onClick={handleBatchImport}
            >
              {syncLoading ? <Loader2 className="size-4 animate-spin" /> : `Import (${selectedKeys.size})`}
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
      .catch((err) => toast(err instanceof Error ? err.message : "Failed to scan accounts", "error"))
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
      toast(`Imported ${result.imported}, skipped ${result.skipped}${result.errors?.length ? `, ${result.errors.length} errors` : ""}`);
      setShowSync(false);
      load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "Import failed", "error");
    } finally {
      setSyncLoading(false);
    }
  }
}
