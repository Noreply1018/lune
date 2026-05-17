import { useEffect, useState } from "react";
import { Loader2, Plus } from "lucide-react";
import { toast } from "@/components/Feedback";
import { useAdminUI } from "@/components/AdminUI";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { api } from "@/lib/api";
import { useRouter } from "@/lib/router";
import type { Pool } from "@/lib/types";

const POOL_NAME_SUGGESTIONS = ["OpenAI", "Claude", "Gemini", "Codex"];

export default function PoolCreateDialog() {
  const { createPoolOpen, closeCreatePool, refreshData } = useAdminUI();
  const { navigate } = useRouter();
  const [label, setLabel] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (createPoolOpen) {
      setLabel("");
      setSaving(false);
    }
  }, [createPoolOpen]);

  async function createPool() {
    const nextLabel = label.trim();
    if (!nextLabel) {
      toast("请输入 Pool 名称", "error");
      return;
    }
    setSaving(true);
    try {
      const existing = await api.get<Pool[]>("/pools");
      const created = await api.post<Pool>("/pools", {
        label: nextLabel,
        priority: (existing?.length ?? 0) + 1,
      });
      toast("Pool 已创建");
      closeCreatePool();
      refreshData();
      navigate(`/admin/pools/${created.id}`);
    } catch (err) {
      toast(err instanceof Error ? err.message : "创建 Pool 失败", "error");
    } finally {
      setSaving(false);
    }
  }

  return (
    <Dialog open={createPoolOpen} onOpenChange={(open) => !open && closeCreatePool()}>
      <DialogContent className="gap-5 rounded-[1.25rem] bg-white/95 p-5 sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="text-[1.05rem] text-moon-800">新建 Pool</DialogTitle>
          <DialogDescription>
            创建一个新的路由边界，默认会启用并自动生成 Pool Token。
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-2.5">
            <Label>Pool 名称</Label>
            <Input
              value={label}
              onChange={(event) => setLabel(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter" && !saving) void createPool();
              }}
              placeholder="例如 OpenAI / Claude / Gemini"
              autoFocus
            />
          </div>
          <div className="flex flex-wrap gap-2">
            {POOL_NAME_SUGGESTIONS.map((name) => (
              <button
                key={name}
                type="button"
                onClick={() => setLabel(name)}
                className="rounded-full bg-lunar-100/70 px-3 py-1.5 text-xs text-lunar-700 transition-colors hover:bg-lunar-100"
              >
                {name}
              </button>
            ))}
          </div>
        </div>

        <DialogFooter className="-mx-5 -mb-5 border-moon-200/60 bg-moon-50/70 px-5">
          <Button variant="outline" onClick={closeCreatePool} disabled={saving}>
            取消
          </Button>
          <Button onClick={createPool} disabled={saving || !label.trim()}>
            {saving ? <Loader2 className="size-4 animate-spin" /> : <Plus className="size-4" />}
            创建
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
