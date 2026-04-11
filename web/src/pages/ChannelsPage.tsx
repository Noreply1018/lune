import { useEffect, useState } from "react";
import StatusBadge from "../components/StatusBadge";
import DataTable, { type Column } from "../components/DataTable";
import { backendGet, backendPut } from "../lib/backend";
import { toast } from "../components/Feedback";
import { latency } from "../lib/fmt";

type Channel = {
  id: number;
  name: string;
  type: number;
  status: number;
  models: string;
  response_time: number;
  balance: number;
  used_quota: number;
  priority: number;
  weight: number;
};

export default function ChannelsPage() {
  const [channels, setChannels] = useState<Channel[]>([]);
  const [loading, setLoading] = useState(true);
  const [testing, setTesting] = useState<number | null>(null);

  function load() {
    setLoading(true);
    backendGet<{ data: Channel[] }>("/api/channel/?p=0&page_size=100")
      .then((d) => setChannels(d.data ?? []))
      .catch(() => toast("加载渠道失败", "error"))
      .finally(() => setLoading(false));
  }

  useEffect(load, []);

  async function testChannel(id: number) {
    setTesting(id);
    try {
      const res = await backendGet<{ success: boolean; message: string; time: number }>(
        `/api/channel/test/${id}`,
      );
      if (res.success) {
        toast(`测试通过 (${latency(res.time ?? 0)})`, "success");
      } else {
        toast(res.message || "测试失败", "error");
      }
      load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "测试失败", "error");
    } finally {
      setTesting(null);
    }
  }

  async function toggleChannel(ch: Channel) {
    try {
      const newStatus = ch.status === 1 ? 2 : 1;
      await backendPut("/api/channel/", { ...ch, status: newStatus });
      toast(newStatus === 1 ? "已启用" : "已停用");
      load();
    } catch {
      toast("操作失败", "error");
    }
  }

  const columns: Column<Channel>[] = [
    { key: "id", header: "ID", render: (r) => r.id, className: "w-12" },
    { key: "name", header: "名称", render: (r) => <span className="font-medium">{r.name}</span> },
    {
      key: "models",
      header: "模型",
      render: (r) => (
        <span className="text-xs text-paper-500 line-clamp-1 max-w-48 block">
          {r.models}
        </span>
      ),
    },
    {
      key: "status",
      header: "状态",
      render: (r) => (
        <StatusBadge
          status={r.status === 1 ? "ok" : r.status === 3 ? "error" : "disabled"}
          label={r.status === 1 ? "正常" : r.status === 3 ? "错误" : "停用"}
        />
      ),
    },
    {
      key: "response_time",
      header: "响应",
      render: (r) =>
        r.response_time ? latency(r.response_time) : "-",
    },
    {
      key: "priority",
      header: "优先级 / 权重",
      render: (r) => `${r.priority ?? 0} / ${r.weight ?? 0}`,
    },
    {
      key: "actions",
      header: "",
      render: (r) => (
        <div className="flex gap-2">
          <button
            onClick={() => testChannel(r.id)}
            disabled={testing === r.id}
            className="rounded px-2 py-1 text-xs text-paper-500 border border-paper-200 hover:bg-paper-200/60 disabled:opacity-50 transition-colors"
          >
            {testing === r.id ? "测试中..." : "测试"}
          </button>
          <button
            onClick={() => toggleChannel(r)}
            className="rounded px-2 py-1 text-xs text-paper-500 border border-paper-200 hover:bg-paper-200/60 transition-colors"
          >
            {r.status === 1 ? "停用" : "启用"}
          </button>
        </div>
      ),
    },
  ];

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">渠道</h2>
        <button
          onClick={load}
          className="rounded-md border border-paper-200 px-3 py-1.5 text-xs text-paper-500 hover:bg-paper-200/60 transition-colors"
        >
          刷新
        </button>
      </div>

      {loading ? (
        <p className="py-12 text-center text-sm text-paper-300">加载中...</p>
      ) : (
        <div className="rounded-xl border border-paper-200 bg-paper-100 p-1">
          <DataTable
            columns={columns}
            rows={channels}
            rowKey={(r) => r.id}
            empty="暂无渠道"
          />
        </div>
      )}
    </div>
  );
}
