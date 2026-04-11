import { FormEvent, useEffect, useState } from "react";
import StatusBadge from "../components/StatusBadge";
import DataTable, { type Column } from "../components/DataTable";
import { luneGet, lunePost, lunePut } from "../lib/api";
import { toast } from "../components/Feedback";

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
        <code className="text-xs text-paper-500">{r.id}</code>
      ),
    },
    {
      key: "credential",
      header: "凭据",
      render: (r) => (
        <code className="text-xs text-paper-500">
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
        <div className="flex gap-2">
          <button
            onClick={() => openEdit(r)}
            className="rounded px-2 py-1 text-xs text-paper-500 border border-paper-200 hover:bg-paper-200/60 transition-colors"
          >
            编辑
          </button>
          <button
            onClick={() => toggleAccount(r)}
            className="rounded px-2 py-1 text-xs text-paper-500 border border-paper-200 hover:bg-paper-200/60 transition-colors"
          >
            {r.enabled ? "停用" : "启用"}
          </button>
        </div>
      ),
    },
  ];

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">账号</h2>
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
            新建账号
          </button>
        </div>
      </div>

      {loading ? (
        <p className="py-12 text-center text-sm text-paper-300">加载中...</p>
      ) : (
        <div className="rounded-xl border border-paper-200 bg-paper-100 p-1">
          <DataTable
            columns={columns}
            rows={accounts}
            rowKey={(r) => r.id}
            empty="暂无账号"
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
              {editId ? "编辑账号" : "新建账号"}
            </h3>

            <label className="block text-xs text-paper-500 mb-1">标签</label>
            <input
              value={form.label}
              onChange={(e) => setForm({ ...form, label: e.target.value })}
              required
              className="mb-4 block w-full rounded-md border border-paper-200 bg-paper-100 px-3 py-2 text-sm text-paper-800 focus:border-paper-500 focus:outline-none"
              placeholder="如 My Backend Key"
            />

            <label className="block text-xs text-paper-500 mb-1">
              凭据（API Key）
            </label>
            <input
              type="password"
              value={form.credential}
              onChange={(e) => setForm({ ...form, credential: e.target.value })}
              className="mb-1 block w-full rounded-md border border-paper-200 bg-paper-100 px-3 py-2 text-sm text-paper-800 focus:border-paper-500 focus:outline-none"
              placeholder={editId ? "留空则保持不变" : "直接输入 API Key"}
            />
            <p className="mb-4 text-xs text-paper-400">
              或通过环境变量名引用：
              <input
                value={form.credential_env}
                onChange={(e) =>
                  setForm({ ...form, credential_env: e.target.value })
                }
                className="ml-1 inline-block w-40 rounded border border-paper-200 bg-paper-100 px-2 py-0.5 text-xs focus:border-paper-500 focus:outline-none"
                placeholder="如 LUNE_BACKEND_KEY"
              />
            </p>

            <label className="block text-xs text-paper-500 mb-1">套餐</label>
            <select
              value={form.plan_type}
              onChange={(e) => setForm({ ...form, plan_type: e.target.value })}
              className="mb-4 block w-full rounded-md border border-paper-200 bg-paper-100 px-3 py-2 text-sm text-paper-800 focus:border-paper-500 focus:outline-none"
            >
              <option value="free">Free</option>
              <option value="plus">Plus</option>
              <option value="pro">Pro</option>
              <option value="team">Team</option>
            </select>

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
