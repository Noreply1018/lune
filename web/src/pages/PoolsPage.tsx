import { FormEvent, useEffect, useState } from "react";
import StatusBadge from "../components/StatusBadge";
import DataTable, { type Column } from "../components/DataTable";
import { luneGet, lunePost, lunePut } from "../lib/api";
import { toast } from "../components/Feedback";

type Pool = {
  id: string;
  platform_id: string;
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
      render: (r) => <code className="text-xs text-paper-500">{r.strategy}</code>,
    },
    {
      key: "members",
      header: "成员",
      render: (r) => (
        <span className="text-xs text-paper-500">
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
        <button
          onClick={() => openEdit(r)}
          className="rounded px-2 py-1 text-xs text-paper-500 border border-paper-200 hover:bg-paper-200/60 transition-colors"
        >
          编辑
        </button>
      ),
    },
  ];

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">号池</h2>
        <div className="flex gap-2">
          <button
            onClick={load}
            className="rounded-md border border-paper-200 px-3 py-1.5 text-xs text-paper-500 hover:bg-paper-200/60 transition-colors"
          >
            刷新
          </button>
          <button
            onClick={openCreate}
            className="rounded-md bg-paper-700 px-3 py-1.5 text-xs text-paper-50 hover:bg-paper-800 transition-colors"
          >
            新建号池
          </button>
        </div>
      </div>

      {loading ? (
        <p className="py-12 text-center text-sm text-paper-300">加载中...</p>
      ) : (
        <div className="rounded-xl border border-paper-200 bg-paper-100 p-1">
          <DataTable
            columns={columns}
            rows={pools}
            rowKey={(r) => r.id}
            empty="暂无号池"
          />
        </div>
      )}

      {showForm && (
        <div className="fixed inset-0 z-40 flex items-center justify-center bg-paper-800/30">
          <form
            onSubmit={handleSubmit}
            className="w-full max-w-sm rounded-xl border border-paper-200 bg-paper-50 p-6 shadow-lg"
          >
            <h3 className="mb-4 text-lg font-semibold text-paper-700">
              {editId ? "编辑号池" : "新建号池"}
            </h3>

            <label className="block text-xs text-paper-500 mb-1">ID</label>
            <input
              value={form.id}
              onChange={(e) => setForm({ ...form, id: e.target.value })}
              required
              disabled={!!editId}
              className="mb-4 block w-full rounded-md border border-paper-200 bg-paper-100 px-3 py-2 text-sm text-paper-800 focus:border-paper-500 focus:outline-none disabled:opacity-50"
              placeholder="如 default-pool"
            />

            <label className="block text-xs text-paper-500 mb-1">策略</label>
            <select
              value={form.strategy}
              onChange={(e) => setForm({ ...form, strategy: e.target.value })}
              className="mb-4 block w-full rounded-md border border-paper-200 bg-paper-100 px-3 py-2 text-sm text-paper-800 focus:border-paper-500 focus:outline-none"
            >
              <option value="sticky-first-healthy">sticky-first-healthy</option>
              <option value="single">single</option>
              <option value="fallback">fallback</option>
            </select>

            <label className="block text-xs text-paper-500 mb-2">
              成员账号
            </label>
            <div className="mb-4 max-h-40 overflow-y-auto rounded-md border border-paper-200 bg-paper-100 p-2 space-y-1">
              {accounts.length === 0 ? (
                <p className="text-xs text-paper-300 py-2 text-center">
                  暂无账号，请先创建账号
                </p>
              ) : (
                accounts.map((a) => (
                  <label
                    key={a.id}
                    className="flex items-center gap-2 rounded px-2 py-1 hover:bg-paper-200/60 cursor-pointer"
                  >
                    <input
                      type="checkbox"
                      checked={form.members.includes(a.id)}
                      onChange={() => toggleMember(a.id)}
                      className="rounded border-paper-200"
                    />
                    <span className="text-sm text-paper-700">{a.label}</span>
                    <code className="text-xs text-paper-400">{a.id}</code>
                  </label>
                ))
              )}
            </div>

            <div className="mb-4 flex items-center gap-2">
              <input
                type="checkbox"
                checked={form.enabled}
                onChange={(e) =>
                  setForm({ ...form, enabled: e.target.checked })
                }
                className="rounded border-paper-200"
              />
              <label className="text-xs text-paper-500">启用</label>
            </div>

            <div className="flex justify-end gap-2">
              <button
                type="button"
                onClick={() => setShowForm(false)}
                className="rounded-md border border-paper-200 px-3 py-1.5 text-sm text-paper-500 hover:bg-paper-200/60 transition-colors"
              >
                取消
              </button>
              <button
                type="submit"
                className="rounded-md bg-paper-700 px-3 py-1.5 text-sm text-paper-50 hover:bg-paper-800 transition-colors"
              >
                {editId ? "保存" : "创建"}
              </button>
            </div>
          </form>
        </div>
      )}
    </div>
  );
}
