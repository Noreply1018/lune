import { FormEvent, useEffect, useState } from "react";
import StatusBadge from "../components/StatusBadge";
import DataTable, { type Column } from "../components/DataTable";
import { luneGet, lunePost, lunePut } from "../lib/api";
import { toast } from "../components/Feedback";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Plus, RefreshCw } from "lucide-react";

type Account = {
  id: string;
  platform_id: string;
  label: string;
  credential_type: string;
  credential: string;
  credential_env: string;
  plan_type: string;
  enabled: boolean;
  status: string;
};

type AccountForm = {
  id: string;
  label: string;
  credential: string;
  credential_env: string;
  plan_type: string;
  enabled: boolean;
};

const emptyForm: AccountForm = {
  id: "",
  label: "",
  credential: "",
  credential_env: "",
  plan_type: "plus",
  enabled: true,
};

export default function AccountsPage() {
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [editId, setEditId] = useState<string | null>(null);
  const [form, setForm] = useState<AccountForm>(emptyForm);

  function load() {
    setLoading(true);
    luneGet<{ accounts: Account[] }>("/admin/api/accounts")
      .then((d) => setAccounts(d.accounts ?? []))
      .catch(() => toast("加载账号失败", "error"))
      .finally(() => setLoading(false));
  }

  useEffect(load, []);

  function openCreate() {
    setEditId(null);
    setForm(emptyForm);
    setShowForm(true);
  }

  function openEdit(a: Account) {
    setEditId(a.id);
    setForm({
      id: a.id,
      label: a.label,
      credential: "",
      credential_env: a.credential_env,
      plan_type: a.plan_type,
      enabled: a.enabled,
    });
    setShowForm(true);
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    try {
      if (editId) {
        await lunePut(`/admin/api/accounts/${editId}`, form);
        toast("账号已更新");
      } else {
        await lunePost("/admin/api/accounts", form);
        toast("账号已创建");
      }
      setShowForm(false);
      load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "操作失败", "error");
    }
  }

  async function toggleAccount(a: Account) {
    try {
      const action = a.enabled ? "disable" : "enable";
      await lunePost(`/admin/api/accounts/${a.id}/${action}`);
      toast(a.enabled ? "已停用" : "已启用");
      load();
    } catch {
      toast("操作失败", "error");
    }
  }

  const columns: Column<Account>[] = [
    {
      key: "label",
      header: "标签",
      render: (r) => <span className="font-medium">{r.label}</span>,
    },
    {
      key: "id",
      header: "ID",
      render: (r) => (
        <code className="text-xs text-muted-foreground">{r.id}</code>
      ),
    },
    {
      key: "credential",
      header: "凭据",
      render: (r) => (
        <code className="text-xs text-muted-foreground">
          {r.credential || r.credential_env || "-"}
        </code>
      ),
    },
    {
      key: "plan_type",
      header: "套餐",
      render: (r) => <span className="text-xs">{r.plan_type}</span>,
    },
    {
      key: "status",
      header: "状态",
      render: (r) => (
        <StatusBadge
          status={
            !r.enabled
              ? "disabled"
              : r.status === "healthy" || r.status === "active" || r.status === "ready"
                ? "ok"
                : "error"
          }
          label={!r.enabled ? "停用" : r.status}
        />
      ),
    },
    {
      key: "actions",
      header: "",
      render: (r) => (
        <div className="flex gap-1">
          <Button variant="ghost" size="sm" onClick={() => openEdit(r)}>
            编辑
          </Button>
          <Button variant="ghost" size="sm" onClick={() => toggleAccount(r)}>
            {r.enabled ? "停用" : "启用"}
          </Button>
        </div>
      ),
    },
  ];

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">账号</h2>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" onClick={load}>
            <RefreshCw className="size-4" />
            刷新
          </Button>
          <Button size="sm" onClick={openCreate}>
            <Plus className="size-4" />
            新建账号
          </Button>
        </div>
      </div>

      {loading ? (
        <Skeleton className="h-48" />
      ) : (
        <Card>
          <CardContent className="p-1">
            <DataTable
              columns={columns}
              rows={accounts}
              rowKey={(r) => r.id}
              empty="暂无账号"
            />
          </CardContent>
        </Card>
      )}

      <Dialog open={showForm} onOpenChange={setShowForm}>
        <DialogContent>
          <form onSubmit={handleSubmit}>
            <DialogHeader>
              <DialogTitle>
                {editId ? "编辑账号" : "新建账号"}
              </DialogTitle>
            </DialogHeader>

            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="acc-label">标签</Label>
                <Input
                  id="acc-label"
                  value={form.label}
                  onChange={(e) => setForm({ ...form, label: e.target.value })}
                  required
                  placeholder="如 My Backend Key"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="acc-credential">凭据（API Key）</Label>
                <Input
                  id="acc-credential"
                  type="password"
                  value={form.credential}
                  onChange={(e) =>
                    setForm({ ...form, credential: e.target.value })
                  }
                  placeholder={editId ? "留空则保持不变" : "直接输入 API Key"}
                />
                <div className="flex items-center gap-2 text-xs text-muted-foreground">
                  <span>或通过环境变量名引用：</span>
                  <Input
                    value={form.credential_env}
                    onChange={(e) =>
                      setForm({ ...form, credential_env: e.target.value })
                    }
                    className="h-7 w-40 text-xs"
                    placeholder="如 LUNE_BACKEND_KEY"
                  />
                </div>
              </div>

              <div className="space-y-2">
                <Label>套餐</Label>
                <Select
                  value={form.plan_type}
                  onValueChange={(v) => v && setForm({ ...form, plan_type: v })}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="free">Free</SelectItem>
                    <SelectItem value="plus">Plus</SelectItem>
                    <SelectItem value="pro">Pro</SelectItem>
                    <SelectItem value="team">Team</SelectItem>
                  </SelectContent>
                </Select>
              </div>

              <div className="flex items-center gap-2">
                <Switch
                  checked={form.enabled}
                  onCheckedChange={(v) => setForm({ ...form, enabled: v })}
                />
                <Label>启用</Label>
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
              <Button type="submit">{editId ? "保存" : "创建"}</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
