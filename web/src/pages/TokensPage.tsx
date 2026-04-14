import { type FormEvent, useEffect, useState } from "react";
import ConfirmDialog from "@/components/ConfirmDialog";
import CopyButton from "@/components/CopyButton";
import DataTable, { type Column } from "@/components/DataTable";
import PageHeader from "@/components/PageHeader";
import SectionHeading from "@/components/SectionHeading";
import UsageBar from "@/components/UsageBar";
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
import { Check, KeyRound, MoreHorizontal, Plus } from "lucide-react";

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
  const [snippetTab, setSnippetTab] = useState(".env");

  function load() {
    setLoading(true);
    let cancelled = false;

    api
      .get<AccessToken[]>("/tokens")
      .then((data) => {
        if (!cancelled) setTokens(data ?? []);
      })
      .catch(() => {
        if (!cancelled) toast("加载令牌失败", "error");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
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

  function openEdit(token: AccessToken) {
    setEditId(token.id);
    setForm({
      name: token.name,
      token: "",
      quota_tokens: token.quota_tokens,
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

  async function toggleToken(token: AccessToken) {
    const next = !token.enabled;
    setTokens((prev) =>
      prev.map((item) => (item.id === token.id ? { ...item, enabled: next } : item)),
    );
    try {
      await api.post(`/tokens/${token.id}/${next ? "enable" : "disable"}`);
    } catch {
      setTokens((prev) =>
        prev.map((item) => (item.id === token.id ? { ...item, enabled: !next } : item)),
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
      render: (row) => <span className="font-medium text-moon-800">{row.name}</span>,
      tone: "primary",
    },
    {
      key: "token",
      header: "令牌掩码",
      render: (row) => (
        <div className="flex items-center gap-1.5">
          <code className="text-xs text-moon-500">{row.token_masked}</code>
          <CopyButton value={row.token_masked} />
        </div>
      ),
      tone: "secondary",
    },
    {
      key: "usage",
      header: "配额压力",
      className: "min-w-[180px]",
      render: (row) => <UsageBar used={row.used_tokens} total={row.quota_tokens} />,
      tone: "numeric",
    },
    {
      key: "last_used",
      header: "最近使用",
      render: (row) => <span className="text-moon-500">{relativeTime(row.last_used_at)}</span>,
      align: "right",
      tone: "secondary",
    },
    {
      key: "actions",
      header: "",
      className: "w-10",
      render: (row) => (
        <DropdownMenu>
          <DropdownMenuTrigger
            render={<Button variant="ghost" size="icon" className="size-8" />}
          >
            <MoreHorizontal className="size-4" />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onClick={() => openEdit(row)}>编辑</DropdownMenuItem>
            <DropdownMenuItem onClick={() => toggleToken(row)}>
              {row.enabled ? "停用" : "启用"}
            </DropdownMenuItem>
            <DropdownMenuItem
              className="text-destructive"
              onClick={() => setDeleteTarget(row)}
            >
              删除
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      ),
    },
  ];

  const enabledTokens = tokens.filter((token) => token.enabled).length;
  const unlimitedTokens = tokens.filter((token) => token.quota_tokens === 0).length;
  const recentUsed = tokens.filter((token) => token.last_used_at).length;

  return (
    <div className="space-y-10">
      <PageHeader
        eyebrow="Tokens / Access"
        title="令牌"
        description="管理访问分发、配额边界与客户端接入。"
        meta={
          <>
            <span>总数 {tokens.length}</span>
            <span>已启用 {enabledTokens}</span>
            <span>不限额 {unlimitedTokens}</span>
          </>
        }
        actions={
          <Button size="sm" onClick={openCreate}>
            <Plus className="size-4" />
            新建令牌
          </Button>
        }
      />

      <section className="grid gap-6 xl:grid-cols-[minmax(0,1.16fr)_minmax(320px,0.84fr)]">
        <div className="surface-section px-5 py-5 sm:px-6 sm:py-6">
          <div className="flex items-start justify-between gap-4">
            <div className="space-y-2">
              <p className="eyebrow-label">访问分发</p>
              <h2 className="text-[1.15rem] font-semibold tracking-[-0.03em] text-moon-800 sm:text-[1.25rem]">
                令牌负责入口鉴权，也负责配额边界
              </h2>
              <p className="max-w-2xl text-sm leading-7 text-moon-500">
                这页不是单纯的凭据列表。创建后要能立即分发给客户端，平时则关注配额压力、最近使用和失效管理。
              </p>
            </div>
            <span className="flex size-12 items-center justify-center rounded-[1.2rem] border border-white/75 bg-white/70 text-lunar-700">
              <KeyRound className="size-5" />
            </span>
          </div>

          <div className="mt-6 grid gap-3 sm:grid-cols-3">
            <div className="rounded-[1.25rem] border border-white/72 bg-white/68 px-4 py-4">
              <p className="kicker">已启用</p>
              <p className="mt-3 text-[1.55rem] font-semibold tracking-[-0.05em] text-moon-800">
                {enabledTokens}
              </p>
            </div>
            <div className="rounded-[1.25rem] border border-white/72 bg-white/68 px-4 py-4">
              <p className="kicker">不限额</p>
              <p className="mt-3 text-[1.55rem] font-semibold tracking-[-0.05em] text-moon-800">
                {unlimitedTokens}
              </p>
            </div>
            <div className="rounded-[1.25rem] border border-white/72 bg-white/68 px-4 py-4">
              <p className="kicker">近期使用</p>
              <p className="mt-3 text-[1.55rem] font-semibold tracking-[-0.05em] text-moon-800">
                {recentUsed}
              </p>
            </div>
          </div>

          <div className="mt-5 rounded-[1.2rem] border border-white/72 bg-[linear-gradient(180deg,rgba(243,239,250,0.82),rgba(255,255,255,0.72))] px-4 py-4">
            <p className="text-sm leading-6 text-moon-500">
              创建成功后会立刻提供 `.env`、Shell、Python、Node.js 与 `curl` 片段，方便直接交付给调用方。
            </p>
          </div>
        </div>

        <aside className="surface-card px-5 py-5">
          <p className="eyebrow-label">分发状态</p>
          <div className="mt-4 space-y-4">
            <div className="rounded-[1.15rem] border border-white/72 bg-white/72 px-4 py-4">
              <p className="kicker">最近交付</p>
              <p className="mt-3 text-sm font-medium text-moon-700">
                {created ? "刚刚创建，可立即复制片段" : "当前没有待交付的新令牌"}
              </p>
            </div>
            <div className="rounded-[1.15rem] border border-white/72 bg-white/72 px-4 py-4">
              <p className="kicker">管理建议</p>
              <ul className="mt-3 space-y-2 text-sm leading-6 text-moon-500">
                <li>给不同调用方分配独立令牌。</li>
                <li>按项目或团队设置可读名称。</li>
                <li>有配额边界时优先使用限额模式。</li>
              </ul>
            </div>
          </div>
        </aside>
      </section>

      <section className="space-y-4">
        <SectionHeading
          title="令牌列表"
          description="查看已签发令牌、配额压力与最近使用情况。"
        />

        {loading ? (
          <Skeleton className="h-64 rounded-[1.5rem]" />
        ) : (
          <div className="surface-card overflow-hidden">
            <div className="flex flex-col gap-2 border-b border-moon-200/60 px-4 py-3 sm:flex-row sm:items-end sm:justify-between">
              <div>
                <p className="eyebrow-label">访问清单</p>
                <p className="mt-1 text-sm text-moon-500">
                  先确认名称和状态，再观察掩码、配额与最后一次使用时间。
                </p>
              </div>
              <p className="text-sm text-moon-500">
                已启用 {enabledTokens} / 总计 {tokens.length}
              </p>
            </div>
            <DataTable
              columns={columns}
              rows={tokens}
              rowKey={(row) => row.id}
              empty="暂未创建令牌"
            />
          </div>
        )}
      </section>

      <Dialog open={showForm} onOpenChange={setShowForm}>
        <DialogContent>
          <form onSubmit={handleSubmit}>
            <DialogHeader>
              <DialogTitle>{editId ? "编辑令牌" : "新建访问令牌"}</DialogTitle>
            </DialogHeader>

            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="token-name">名称</Label>
                <Input
                  id="token-name"
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  required
                  placeholder="frontend-app"
                />
              </div>

              {!editId && (
                <div className="space-y-2">
                  <Label htmlFor="token-value">
                    令牌值 <span className="font-normal text-moon-400">（留空自动生成）</span>
                  </Label>
                  <Input
                    id="token-value"
                    value={form.token}
                    onChange={(e) => setForm({ ...form, token: e.target.value })}
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
                <p className="text-xs text-moon-400">0 表示不限额</p>
              </div>
            </div>

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setShowForm(false)}>
                取消
              </Button>
              <Button type="submit">{editId ? "保存" : "创建"}</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={created !== null} onOpenChange={(open) => !open && setCreated(null)}>
        <DialogContent className="max-w-xl">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Check className="size-5 text-status-green" />
              令牌创建成功
            </DialogTitle>
          </DialogHeader>

          <div className="space-y-5 py-2">
            <p className="text-sm text-moon-500">
              请立即复制完整令牌。关闭后将无法再次查看明文。
            </p>

            <div className="rounded-[1.2rem] border border-white/72 bg-white/72 px-4 py-4">
              <div className="flex items-center gap-2">
                <code className="flex-1 break-all text-sm font-medium text-moon-800">
                  {created?.token}
                </code>
                <CopyButton value={created?.token ?? ""} label="复制" />
              </div>
            </div>

            <div className="space-y-3">
              <p className="eyebrow-label">快速接入</p>
              <div className="flex flex-wrap gap-1">
                {snippetTabs.map((tab) => (
                  <button
                    key={tab}
                    type="button"
                    onClick={() => setSnippetTab(tab)}
                    className={`rounded-full px-3 py-1.5 text-xs transition ${
                      snippetTab === tab
                        ? "bg-moon-800 text-moon-50"
                        : "bg-moon-100 text-moon-500 hover:bg-moon-200"
                    }`}
                  >
                    {tab}
                  </button>
                ))}
              </div>
              <div className="rounded-[1.2rem] bg-moon-800 p-4">
                <pre className="whitespace-pre-wrap text-xs leading-6 text-moon-100">
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
