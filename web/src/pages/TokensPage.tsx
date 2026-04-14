import { type FormEvent, useEffect, useState } from "react";
import DataTable, { type Column } from "@/components/DataTable";
import CopyButton from "@/components/CopyButton";
import UsageBar from "@/components/UsageBar";
import ConfirmDialog from "@/components/ConfirmDialog";
import PageHeader from "@/components/PageHeader";
import SectionHeading from "@/components/SectionHeading";
import { api } from "@/lib/api";
import { toast } from "@/components/Feedback";
import { relativeTime } from "@/lib/fmt";
import type { AccessToken, AccessTokenCreated } from "@/lib/types";
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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Plus, MoreHorizontal, Check } from "lucide-react";

interface TokenForm {
  name: string;
  token: string;
  quota_tokens: number;
}

const emptyForm: TokenForm = { name: "", token: "", quota_tokens: 0 };

export default function TokensPage() {
  const [tokens, setTokens] = useState<AccessToken[]>([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [editId, setEditId] = useState<number | null>(null);
  const [form, setForm] = useState<TokenForm>(emptyForm);
  const [deleteTarget, setDeleteTarget] = useState<AccessToken | null>(null);
  const [created, setCreated] = useState<AccessTokenCreated | null>(null);

  function load() {
    setLoading(true);
    let cancelled = false;

    api.get<AccessToken[]>("/tokens")
      .then((d) => { if (!cancelled) setTokens(d ?? []); })
      .catch(() => { if (!cancelled) toast("加载令牌失败", "error"); })
      .finally(() => { if (!cancelled) setLoading(false); });

    return () => { cancelled = true; };
  }

  useEffect(() => {
    const cancel = load();
    return cancel;
  }, []);

  function openCreate() {
    setEditId(null);
    setForm(emptyForm);
    setShowForm(true);
  }

  function openEdit(t: AccessToken) {
    setEditId(t.id);
    setForm({
      name: t.name,
      token: "",
      quota_tokens: t.quota_tokens,
    });
    setShowForm(true);
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    try {
      if (editId) {
        await api.put(`/tokens/${editId}`, {
          name: form.name,
          quota_tokens: form.quota_tokens,
        });
        toast("令牌已更新");
        setShowForm(false);
        load();
      } else {
        const body: Record<string, unknown> = {
          name: form.name,
          quota_tokens: form.quota_tokens,
        };
        if (form.token) body.token = form.token;
        const result = await api.post<AccessTokenCreated>("/tokens", body);
        setShowForm(false);
        setCreated(result);
        load();
      }
    } catch (err) {
      toast(err instanceof Error ? err.message : "操作失败", "error");
    }
  }

  async function toggleToken(t: AccessToken) {
    const next = !t.enabled;
    setTokens((prev) =>
      prev.map((x) => (x.id === t.id ? { ...x, enabled: next } : x)),
    );
    try {
      await api.post(`/tokens/${t.id}/${next ? "enable" : "disable"}`);
    } catch {
      setTokens((prev) =>
        prev.map((x) => (x.id === t.id ? { ...x, enabled: !next } : x)),
      );
      toast("更新令牌失败", "error");
    }
  }

  async function confirmDelete() {
    if (!deleteTarget) return;
    try {
      await api.delete(`/tokens/${deleteTarget.id}`);
      toast("令牌已删除");
      load();
    } catch {
      toast("删除令牌失败", "error");
    } finally {
      setDeleteTarget(null);
    }
  }

  const [snippetTab, setSnippetTab] = useState(".env");

  const snippetTabs = [".env", "Shell", "Python", "Node.js", "curl"] as const;

  function getSnippet(token: string, tab: string): string {
    const host = `${window.location.protocol}//${window.location.host}`;
    const baseUrl = `${host}/v1`;
    switch (tab) {
      case ".env":
        return `OPENAI_BASE_URL=${baseUrl}\nOPENAI_API_KEY=${token}`;
      case "Shell":
        return `export OPENAI_BASE_URL="${baseUrl}"\nexport OPENAI_API_KEY="${token}"`;
      case "Python":
        return `from openai import OpenAI\n\nclient = OpenAI(\n    base_url="${baseUrl}",\n    api_key="${token}",\n)`;
      case "Node.js":
        return `import OpenAI from "openai";\n\nconst client = new OpenAI({\n  baseURL: "${baseUrl}",\n  apiKey: "${token}",\n});`;
      case "curl":
        return `curl ${baseUrl}/chat/completions \\\n  -H "Authorization: Bearer ${token}" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model": "gpt-4o", "messages": [{"role": "user", "content": "Hello"}]}'`;
      default:
        return "";
    }
  }

  const columns: Column<AccessToken>[] = [
    {
      key: "name",
      header: "名称",
      render: (r) => (
        <span className="font-medium text-moon-800">{r.name}</span>
      ),
      tone: "primary",
    },
    {
      key: "token",
      header: "令牌",
      render: (r) => (
        <div className="flex items-center gap-1">
          <code className="text-xs text-moon-500">{r.token_masked}</code>
          <CopyButton value={r.token_masked} />
        </div>
      ),
      tone: "secondary",
    },
    {
      key: "usage",
      header: "用量",
      className: "min-w-[160px]",
      render: (r) => (
        <UsageBar used={r.used_tokens} total={r.quota_tokens} />
      ),
      tone: "numeric",
    },
    {
      key: "last_used",
      header: "最近使用",
      render: (r) => (
        <span className="text-moon-500">
          {relativeTime(r.last_used_at)}
        </span>
      ),
      align: "right",
      tone: "secondary",
    },
    {
      key: "actions",
      header: "",
      className: "w-10",
      render: (r) => (
        <DropdownMenu>
          <DropdownMenuTrigger
            render={<Button variant="ghost" size="icon" className="size-8" />}
          >
            <MoreHorizontal className="size-4" />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onClick={() => openEdit(r)}>
              编辑
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => toggleToken(r)}>
              {r.enabled ? "停用" : "启用"}
            </DropdownMenuItem>
            <DropdownMenuItem
              className="text-destructive"
              onClick={() => setDeleteTarget(r)}
            >
              删除
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      ),
    },
  ];

  return (
    <div className="space-y-8">
      <PageHeader
        eyebrow="工作区"
        title="令牌"
        description="统一签发访问令牌、查看配额消耗，并管理令牌生命周期。"
        meta={
          <span>
            共 {tokens.length} 个令牌 • 已启用 {tokens.filter((token) => token.enabled).length} 个
          </span>
        }
        actions={
          <Button size="sm" onClick={openCreate}>
            <Plus className="size-4" />
            新建令牌
          </Button>
        }
      />

      <section className="space-y-4">
        <SectionHeading
          title="令牌列表"
          description="查看已签发令牌、掩码值和当前配额压力。"
        />

        {loading ? (
          <Skeleton className="h-64 rounded-[1.5rem]" />
        ) : (
          <div className="overflow-hidden rounded-[1.6rem] border border-moon-200/70 bg-white/85">
            <DataTable
              columns={columns}
              rows={tokens}
              rowKey={(r) => r.id}
              empty="暂未创建令牌"
            />
          </div>
        )}
      </section>

      {/* Create / Edit dialog */}
      <Dialog open={showForm} onOpenChange={setShowForm}>
        <DialogContent>
          <form onSubmit={handleSubmit}>
            <DialogHeader>
              <DialogTitle>
                {editId ? "编辑令牌" : "新建访问令牌"}
              </DialogTitle>
            </DialogHeader>

            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="token-name">名称</Label>
                <Input
                  id="token-name"
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  required
                  placeholder="my-project"
                />
              </div>

              {!editId && (
                <div className="space-y-2">
                  <Label htmlFor="token-value">
                    令牌值{" "}
                    <span className="font-normal text-moon-400">
                      （留空自动生成）
                    </span>
                  </Label>
                  <Input
                    id="token-value"
                    value={form.token}
                    onChange={(e) =>
                      setForm({ ...form, token: e.target.value })
                    }
                    placeholder="sk-lune-..."
                  />
                </div>
              )}

              <div className="space-y-2">
                <Label htmlFor="token-quota">Token 配额</Label>
                <Input
                  id="token-quota"
                  type="number"
                  value={form.quota_tokens}
                  onChange={(e) =>
                    setForm({
                      ...form,
                      quota_tokens: Number(e.target.value),
                    })
                  }
                />
                <p className="text-xs text-moon-400">
                  0 = 不限制
                </p>
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

      {/* Token created success dialog */}
      <Dialog open={created !== null} onOpenChange={(o) => !o && setCreated(null)}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Check className="size-5 text-status-green" />
              令牌创建成功
            </DialogTitle>
          </DialogHeader>

          <div className="space-y-4 py-2">
            <p className="text-sm text-moon-500">
              请立即复制该令牌。关闭后将无法再次查看完整明文。
            </p>

            <div className="flex items-center gap-2 rounded-lg bg-moon-100 px-4 py-3">
              <code className="flex-1 break-all text-sm font-medium text-moon-800">
                {created?.token}
              </code>
              <CopyButton value={created?.token ?? ""} label="复制" />
            </div>

            <div className="space-y-2">
              <p className="text-xs font-medium uppercase tracking-wider text-moon-400">
                快速接入
              </p>
              <div className="flex flex-wrap gap-1">
                {snippetTabs.map((tab) => (
                  <button
                    key={tab}
                    type="button"
                    onClick={() => setSnippetTab(tab)}
                    className={`rounded-md px-2.5 py-1 text-xs font-medium transition ${
                      snippetTab === tab
                        ? "bg-moon-800 text-moon-100"
                        : "bg-moon-100 text-moon-500 hover:bg-moon-200"
                    }`}
                  >
                    {tab}
                  </button>
                ))}
              </div>
              <div className="rounded-lg bg-moon-800 p-4">
                <pre className="whitespace-pre-wrap text-xs text-moon-100">
                  {created ? getSnippet(created.token, snippetTab) : ""}
                </pre>
              </div>
              <CopyButton
                value={created ? getSnippet(created.token, snippetTab) : ""}
                label={`复制 ${snippetTab}`}
              />
            </div>
          </div>

          <DialogFooter>
            <Button onClick={() => setCreated(null)}>完成</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        title="删除令牌"
        description={`确认删除“${deleteTarget?.name ?? ""}”吗？该令牌会立即失效。`}
        onConfirm={confirmDelete}
      />
    </div>
  );
}
