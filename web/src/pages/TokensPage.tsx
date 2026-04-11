import { FormEvent, useEffect, useState } from "react";
import StatusBadge from "../components/StatusBadge";
import DataTable, { type Column } from "../components/DataTable";
import { oneapiGet, oneapiPost, oneapiPut, oneapiDelete } from "../lib/oneapi";
import { toast } from "../components/Feedback";
import { compact, shortDate } from "../lib/fmt";

type Token = {
  id: number;
  name: string;
  key: string;
  status: number;
  used_quota: number;
  remain_quota: number;
  unlimited_quota: boolean;
  created_time: number;
  expired_time: number;
};

type TokenForm = {
  name: string;
  remain_quota: number;
  unlimited_quota: boolean;
};

const emptyForm: TokenForm = {
  name: "",
  remain_quota: 500000,
  unlimited_quota: false,
};

export default function TokensPage() {
  const [tokens, setTokens] = useState<Token[]>([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [editId, setEditId] = useState<number | null>(null);
  const [form, setForm] = useState<TokenForm>(emptyForm);

  function load() {
    setLoading(true);
    oneapiGet<{ data: Token[] }>("/api/token/?p=0&page_size=100")
      .then((d) => setTokens(d.data ?? []))
      .catch(() => toast("加载令牌失败", "error"))
      .finally(() => setLoading(false));
  }

  useEffect(load, []);

  function openCreate() {
    setEditId(null);
    setForm(emptyForm);
    setShowForm(true);
  }

  function openEdit(t: Token) {
    setEditId(t.id);
    setForm({
      name: t.name,
      remain_quota: t.remain_quota,
      unlimited_quota: t.unlimited_quota,
    });
    setShowForm(true);
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    try {
      if (editId) {
        await oneapiPut("/api/token/", { id: editId, ...form });
        toast("令牌已更新");
      } else {
        await oneapiPost("/api/token/", form);
        toast("令牌已创建");
      }
      setShowForm(false);
      load();
    } catch {
      toast("操作失败", "error");
    }
  }

  async function deleteToken(id: number) {
    if (!confirm("确定删除此令牌？")) return;
    try {
      await oneapiDelete(`/api/token/${id}`);
      toast("已删除");
      load();
    } catch {
      toast("删除失败", "error");
    }
  }

  async function toggleToken(t: Token) {
    try {
      const newStatus = t.status === 1 ? 2 : 1;
      await oneapiPut("/api/token/", { ...t, status: newStatus });
      toast(newStatus === 1 ? "已启用" : "已停用");
      load();
    } catch {
      toast("操作失败", "error");
    }
  }

  const columns: Column<Token>[] = [
    { key: "name", header: "名称", render: (r) => <span className="font-medium">{r.name}</span> },
    {
      key: "key",
      header: "Key",
      render: (r) => (
        <code className="text-xs text-paper-500">
          {r.key ? `sk-...${r.key.slice(-6)}` : "-"}
        </code>
      ),
    },
    {
      key: "status",
      header: "状态",
      render: (r) => (
        <StatusBadge
          status={r.status === 1 ? "ok" : r.status === 3 ? "error" : "disabled"}
          label={r.status === 1 ? "正常" : r.status === 3 ? "过期" : "停用"}
        />
      ),
    },
    {
      key: "quota",
      header: "已用 / 剩余",
      render: (r) =>
        r.unlimited_quota
          ? `${compact(r.used_quota)} / unlimited`
          : `${compact(r.used_quota)} / ${compact(r.remain_quota)}`,
    },
    {
      key: "created",
      header: "创建时间",
      render: (r) => shortDate(new Date(r.created_time * 1000).toISOString()),
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
            onClick={() => toggleToken(r)}
            className="rounded px-2 py-1 text-xs text-paper-500 border border-paper-200 hover:bg-paper-200/60 transition-colors"
          >
            {r.status === 1 ? "停用" : "启用"}
          </button>
          <button
            onClick={() => deleteToken(r.id)}
            className="rounded px-2 py-1 text-xs text-clay-500 border border-clay-500/30 hover:bg-clay-500/10 transition-colors"
          >
            删除
          </button>
        </div>
      ),
    },
  ];

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">令牌</h2>
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
            新建令牌
          </button>
        </div>
      </div>

      {loading ? (
        <p className="py-12 text-center text-sm text-paper-300">加载中...</p>
      ) : (
        <div className="rounded-xl border border-paper-200 bg-paper-100 p-1">
          <DataTable
            columns={columns}
            rows={tokens}
            rowKey={(r) => r.id}
            empty="暂无令牌"
          />
        </div>
      )}

      {/* ── create / edit form ── */}
      {showForm && (
        <div className="fixed inset-0 z-40 flex items-center justify-center bg-paper-800/30">
          <form
            onSubmit={handleSubmit}
            className="w-full max-w-sm rounded-xl border border-paper-200 bg-paper-50 p-6 shadow-lg"
          >
            <h3 className="mb-4 text-lg font-semibold text-paper-700">
              {editId ? "编辑令牌" : "新建令牌"}
            </h3>

            <label className="block text-xs text-paper-500 mb-1">名称</label>
            <input
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              required
              className="mb-4 block w-full rounded-md border border-paper-200 bg-paper-100 px-3 py-2 text-sm text-paper-800 focus:border-paper-500 focus:outline-none"
            />

            <div className="mb-4 flex items-center gap-2">
              <input
                type="checkbox"
                checked={form.unlimited_quota}
                onChange={(e) =>
                  setForm({ ...form, unlimited_quota: e.target.checked })
                }
                className="rounded border-paper-200"
              />
              <label className="text-xs text-paper-500">无限额度</label>
            </div>

            {!form.unlimited_quota && (
              <>
                <label className="block text-xs text-paper-500 mb-1">额度</label>
                <input
                  type="number"
                  value={form.remain_quota}
                  onChange={(e) =>
                    setForm({ ...form, remain_quota: Number(e.target.value) })
                  }
                  className="mb-4 block w-full rounded-md border border-paper-200 bg-paper-100 px-3 py-2 text-sm text-paper-800 focus:border-paper-500 focus:outline-none"
                />
              </>
            )}

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
