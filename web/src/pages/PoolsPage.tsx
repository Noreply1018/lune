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
import { Checkbox } from "@/components/ui/checkbox";
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

type Pool = {
  id: string;
  strategy: string;
  enabled: boolean;
  members: string[];
};

type Account = {
  id: string;
  label: string;
  enabled: boolean;
};

type PoolForm = {
  id: string;
  strategy: string;
  enabled: boolean;
  members: string[];
};

const emptyForm: PoolForm = {
  id: "",
  strategy: "sticky-first-healthy",
  enabled: true,
  members: [],
};

export default function PoolsPage() {
  const [pools, setPools] = useState<Pool[]>([]);
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [editId, setEditId] = useState<string | null>(null);
  const [form, setForm] = useState<PoolForm>(emptyForm);

  function load() {
    setLoading(true);
    Promise.all([
      luneGet<{ pools: Pool[] }>("/admin/api/pools"),
      luneGet<{ accounts: Account[] }>("/admin/api/accounts"),
    ])
      .then(([p, a]) => {
        setPools(p.pools ?? []);
        setAccounts(a.accounts ?? []);
      })
      .catch(() => toast("加载失败", "error"))
      .finally(() => setLoading(false));
  }

  useEffect(load, []);

  function openCreate() {
    setEditId(null);
    setForm(emptyForm);
    setShowForm(true);
  }

  function openEdit(p: Pool) {
    setEditId(p.id);
    setForm({
      id: p.id,
      strategy: p.strategy,
      enabled: p.enabled,
      members: p.members ?? [],
    });
    setShowForm(true);
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    try {
      if (editId) {
        await lunePut("/admin/api/pools", form);
        toast("号池已更新");
      } else {
        await lunePost("/admin/api/pools", form);
        toast("号池已创建");
      }
      setShowForm(false);
      load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "操作失败", "error");
    }
  }

  function toggleMember(id: string) {
    setForm((prev) => ({
      ...prev,
      members: prev.members.includes(id)
        ? prev.members.filter((m) => m !== id)
        : [...prev.members, id],
    }));
  }

  const columns: Column<Pool>[] = [
    {
      key: "id",
      header: "ID",
      render: (r) => <span className="font-medium">{r.id}</span>,
    },
    {
      key: "strategy",
      header: "策略",
      render: (r) => (
        <code className="text-xs text-muted-foreground">{r.strategy}</code>
      ),
    },
    {
      key: "members",
      header: "成员",
      render: (r) => (
        <span className="text-xs text-muted-foreground">
          {r.members?.length ?? 0} 个账号
        </span>
      ),
    },
    {
      key: "enabled",
      header: "状态",
      render: (r) => (
        <StatusBadge
          status={r.enabled ? "ok" : "disabled"}
          label={r.enabled ? "启用" : "停用"}
        />
      ),
    },
    {
      key: "actions",
      header: "",
      render: (r) => (
        <Button variant="ghost" size="sm" onClick={() => openEdit(r)}>
          编辑
        </Button>
      ),
    },
  ];

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">号池</h2>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" onClick={load}>
            <RefreshCw className="size-4" />
            刷新
          </Button>
          <Button size="sm" onClick={openCreate}>
            <Plus className="size-4" />
            新建号池
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
              rows={pools}
              rowKey={(r) => r.id}
              empty="暂无号池"
            />
          </CardContent>
        </Card>
      )}

      <Dialog open={showForm} onOpenChange={setShowForm}>
        <DialogContent>
          <form onSubmit={handleSubmit}>
            <DialogHeader>
              <DialogTitle>
                {editId ? "编辑号池" : "新建号池"}
              </DialogTitle>
            </DialogHeader>

            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="pool-id">ID</Label>
                <Input
                  id="pool-id"
                  value={form.id}
                  onChange={(e) => setForm({ ...form, id: e.target.value })}
                  required
                  disabled={!!editId}
                  placeholder="如 default-pool"
                />
              </div>

              <div className="space-y-2">
                <Label>策略</Label>
                <Select
                  value={form.strategy}
                  onValueChange={(v) => v && setForm({ ...form, strategy: v })}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="sticky-first-healthy">
                      sticky-first-healthy
                    </SelectItem>
                    <SelectItem value="single">single</SelectItem>
                    <SelectItem value="fallback">fallback</SelectItem>
                  </SelectContent>
                </Select>
              </div>

              <div className="space-y-2">
                <Label>成员账号</Label>
                <div className="max-h-40 overflow-y-auto rounded-md border p-2 space-y-1">
                  {accounts.length === 0 ? (
                    <p className="text-xs text-muted-foreground py-2 text-center">
                      暂无账号，请先创建账号
                    </p>
                  ) : (
                    accounts.map((a) => (
                      <label
                        key={a.id}
                        className="flex items-center gap-2 rounded px-2 py-1 hover:bg-accent cursor-pointer"
                      >
                        <Checkbox
                          checked={form.members.includes(a.id)}
                          onCheckedChange={() => toggleMember(a.id)}
                        />
                        <span className="text-sm">{a.label}</span>
                        <code className="text-xs text-muted-foreground">
                          {a.id}
                        </code>
                      </label>
                    ))
                  )}
                </div>
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
