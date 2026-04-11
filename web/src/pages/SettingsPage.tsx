import { FormEvent, useEffect, useState } from "react";
import { luneGet, lunePut } from "../lib/api";
import { toast } from "../components/Feedback";

type Settings = {
  backend_url: string;
  port: number;
  data_dir: string;
};

export default function SettingsPage() {
  const [settings, setSettings] = useState<Settings | null>(null);
  const [backendUrl, setBackendUrl] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);

  function load() {
    setLoading(true);
    luneGet<{ settings: Settings }>("/admin/api/settings")
      .then((d) => {
        setSettings(d.settings);
        setBackendUrl(d.settings.backend_url);
      })
      .catch(() => toast("加载设置失败", "error"))
      .finally(() => setLoading(false));
  }

  useEffect(load, []);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setSaving(true);
    try {
      await lunePut("/admin/api/settings", { backend_url: backendUrl });
      toast("设置已保存");
      load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "保存失败", "error");
    } finally {
      setSaving(false);
    }
  }

  if (loading || !settings) {
    return (
      <p className="py-12 text-center text-sm text-paper-300">加载中...</p>
    );
  }

  return (
    <div className="space-y-6">
      <h2 className="text-xl font-semibold">设置</h2>

      <form
        onSubmit={handleSubmit}
        className="max-w-lg rounded-xl border border-paper-200 bg-paper-100 p-6 space-y-4"
      >
        <div>
          <label className="block text-xs text-paper-500 mb-1">
            后端引擎地址
          </label>
          <input
            type="url"
            value={backendUrl}
            onChange={(e) => setBackendUrl(e.target.value)}
            required
            className="block w-full rounded-md border border-paper-200 bg-paper-50 px-3 py-2 text-sm text-paper-800 focus:border-paper-500 focus:outline-none"
          />
        </div>

        <div>
          <label className="block text-xs text-paper-500 mb-1">
            监听端口
          </label>
          <input
            type="text"
            value={settings.port}
            disabled
            className="block w-full rounded-md border border-paper-200 bg-paper-100 px-3 py-2 text-sm text-paper-500"
          />
          <p className="mt-1 text-xs text-paper-400">
            修改端口需要重启服务
          </p>
        </div>

        <div>
          <label className="block text-xs text-paper-500 mb-1">
            数据目录
          </label>
          <input
            type="text"
            value={settings.data_dir}
            disabled
            className="block w-full rounded-md border border-paper-200 bg-paper-100 px-3 py-2 text-sm text-paper-500"
          />
        </div>

        <div className="flex justify-end">
          <button
            type="submit"
            disabled={saving}
            className="rounded-md bg-paper-700 px-4 py-2 text-sm font-medium text-paper-50 hover:bg-paper-800 disabled:opacity-50 transition-colors"
          >
            {saving ? "保存中..." : "保存"}
          </button>
        </div>
      </form>
    </div>
  );
}
