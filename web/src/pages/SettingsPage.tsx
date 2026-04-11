import { FormEvent, useEffect, useState } from "react";
import { luneGet, lunePut } from "../lib/api";
import { toast } from "../components/Feedback";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Loader2 } from "lucide-react";

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
      <div className="space-y-6">
        <Skeleton className="h-7 w-20" />
        <Skeleton className="h-64 max-w-lg" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <h2 className="text-xl font-semibold">设置</h2>

      <Card className="max-w-lg">
        <CardContent className="p-6">
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="backend-url">后端引擎地址</Label>
              <Input
                id="backend-url"
                type="url"
                value={backendUrl}
                onChange={(e) => setBackendUrl(e.target.value)}
                required
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="port">监听端口</Label>
              <Input
                id="port"
                type="text"
                value={settings.port}
                disabled
              />
              <p className="text-xs text-muted-foreground">
                修改端口需要重启服务
              </p>
            </div>

            <div className="space-y-2">
              <Label htmlFor="data-dir">数据目录</Label>
              <Input
                id="data-dir"
                type="text"
                value={settings.data_dir}
                disabled
              />
            </div>

            <div className="flex justify-end">
              <Button type="submit" disabled={saving}>
                {saving && <Loader2 className="animate-spin" />}
                {saving ? "保存中..." : "保存"}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
