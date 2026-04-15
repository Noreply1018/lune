import { type FormEvent, useEffect, useState } from "react";
import ConfirmDialog from "@/components/ConfirmDialog";
import CopyButton from "@/components/CopyButton";
import DataTable, { type Column } from "@/components/DataTable";
import PageHeader from "@/components/PageHeader";
import SectionHeading from "@/components/SectionHeading";
import { api } from "@/lib/api";
import { toast } from "@/components/Feedback";
import { relativeTime } from "@/lib/fmt";
import type { AccessToken, AccessTokenCreated, Pool } from "@/lib/types";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Check, Globe, KeyRound, Layers, MoreHorizontal, Plus } from "lucide-react";

/* ── v3 token form ── */

interface TokenForm {
  name: string;
  pool_id: string; // "" = global, numeric string = pool-scoped
  enabled: boolean;
}

const emptyForm: TokenForm = { name: "", pool_id: "", enabled: true };

/* ── magic value for "global" in Select ── */
const GLOBAL_VALUE = "__global__";

export default function TokensPage() {
  const [tokens, setTokens] = useState<AccessToken[]>([]);
  const [pools, setPools] = useState<Pool[]>([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [editId, setEditId] = useState<number | null>(null);
  const [form, setForm] = useState<TokenForm>(emptyForm);
  const [deleteTarget, setDeleteTarget] = useState<AccessToken | null>(null);
  const [created, setCreated] = useState<AccessTokenCreated | null>(null);
  const [snippetTab, setSnippetTab] = useState(".env");

  /* ── data loading ── */

  function load() {
    setLoading(true);
    let cancelled = false;

    Promise.all([
      api.get<AccessToken[]>("/tokens"),
      api.get<Pool[]>("/pools"),
    ])
      .then(([tokensData, poolsData]) => {
        if (!cancelled) {
          setTokens(tokensData ?? []);
          setPools(poolsData ?? []);
        }
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

  /* ── form helpers ── */

  function openCreate() {
    setEditId(null);
    setForm(emptyForm);
    setShowForm(true);
  }

  function openEdit(token: AccessToken) {
    setEditId(token.id);
    setForm({
      name: token.name,
      pool_id: token.pool_id != null ? String(token.pool_id) : "",
      enabled: token.enabled,
    });
    setShowForm(true);
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    try {
      const poolId = form.pool_id ? Number(form.pool_id) : null;
      if (editId) {
        await api.put(`/tokens/${editId}`, {
          name: form.name,
          pool_id: poolId,
          enabled: form.enabled,
        });
        toast("令牌已更新");
        setShowForm(false);
        load();
      } else {
        const result = await api.post<AccessTokenCreated>("/tokens", {
          name: form.name,
          pool_id: poolId,
          enabled: form.enabled,
        });
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

  /* ── env snippets ── */

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

  /* ── type badge ── */

  function TypeBadge({ token }: { token: AccessToken }) {
    if (token.is_global) {
      return (
        <span className="inline-flex items-center gap-1.5 rounded-full border border-lunar-200/40 bg-lunar-50/60 px-2.5 py-0.5 text-xs font-medium text-lunar-700">
          <Globe className="size-3" />
          全局令牌
        </span>
      );
    }
    return (
      <span className="inline-flex items-center gap-1.5 rounded-full border border-moon-200/60 bg-moon-100/50 px-2.5 py-0.5 text-xs font-medium text-moon-600">
        <Layers className="size-3" />
        {token.pool_label ?? `池 #${token.pool_id}`}
      </span>
    );
  }

  /* ── table columns ── */

  const columns: Column<AccessToken>[] = [
    {
      key: "name",
      header: "名称",
      render: (row) => <span className="font-medium text-moon-800">{row.name}</span>,
      tone: "primary",
    },
    {
      key: "type",
      header: "类型",
      render: (row) => <TypeBadge token={row} />,
      tone: "secondary",
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
      key: "enabled",
      header: "状态",
      render: (row) => (
        <span
          className={
            row.enabled
              ? "inline-flex items-center gap-1.5 text-xs font-medium text-status-green"
              : "inline-flex items-center gap-1.5 text-xs font-medium text-moon-400"
          }
        >
          <span
            className={`size-1.5 rounded-full ${row.enabled ? "bg-status-green" : "bg-moon-400"}`}
          />
          {row.enabled ? "已启用" : "已停用"}
        </span>
      ),
      tone: "status",
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

  /* ── derived stats ── */

  const enabledTokens = tokens.filter((t) => t.enabled).length;
  const globalTokens = tokens.filter((t) => t.is_global).length;
  const poolTokens = tokens.filter((t) => !t.is_global).length;
  const recentUsed = tokens.filter((t) => t.last_used_at).length;

  /* ── pool selector helper ── */

  function handlePoolChange(value: string | null) {
    setForm({ ...form, pool_id: !value || value === GLOBAL_VALUE ? "" : value });
  }

  return (
    <div className="space-y-10">
      <PageHeader
        eyebrow="Tokens / Access"
        title="令牌"
        description="管理访问令牌的签发、作用域与客户端接入。"
        meta={
          <>
            <span>总数 {tokens.length}</span>
            <span>已启用 {enabledTokens}</span>
            <span>全局 {globalTokens}</span>
            <span>池令牌 {poolTokens}</span>
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
                令牌负责入口鉴权与作用域控制
              </h2>
              <p className="max-w-2xl text-sm leading-7 text-moon-500">
                全局令牌可访问所有池和路由的模型，池令牌仅限指定池内模型。创建后会自动生成令牌值，请立即复制保存。
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
              <p className="kicker">全局令牌</p>
              <p className="mt-3 text-[1.55rem] font-semibold tracking-[-0.05em] text-moon-800">
                {globalTokens}
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
              <p className="kicker">令牌类型说明</p>
              <ul className="mt-3 space-y-2 text-sm leading-6 text-moon-500">
                <li><strong className="text-moon-700">全局令牌</strong> - 可访问所有池和路由暴露的模型。</li>
                <li><strong className="text-moon-700">池令牌</strong> - 仅可访问绑定池内的模型。</li>
                <li>给不同调用方分配独立令牌并设置可读名称。</li>
              </ul>
            </div>
          </div>
        </aside>
      </section>

      <section className="space-y-4">
        <SectionHeading
          title="令牌列表"
          description="查看已签发令牌的类型、状态与最近使用情况。"
        />

        {loading ? (
          <Skeleton className="h-64 rounded-[1.5rem]" />
        ) : (
          <div className="surface-card overflow-hidden">
            <div className="flex flex-col gap-2 border-b border-moon-200/60 px-4 py-3 sm:flex-row sm:items-end sm:justify-between">
              <div>
                <p className="eyebrow-label">访问清单</p>
                <p className="mt-1 text-sm text-moon-500">
                  查看名称、类型、状态与最后一次使用时间。
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

      {/* ── create / edit dialog ── */}
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

              <div className="space-y-2">
                <Label>作用域</Label>
                <Select
                  value={form.pool_id || GLOBAL_VALUE}
                  onValueChange={handlePoolChange}
                >
                  <SelectTrigger className="w-full">
                    <SelectValue placeholder="选择作用域..." />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={GLOBAL_VALUE}>
                      全局令牌 - 可访问所有池
                    </SelectItem>
                    {pools.map((pool) => (
                      <SelectItem key={pool.id} value={String(pool.id)}>
                        {pool.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <p className="text-xs text-moon-400">
                  全局令牌可访问所有池和路由，池令牌仅限指定池内模型
                </p>
              </div>

              <div className="flex items-center justify-between">
                <Label htmlFor="token-enabled">启用</Label>
                <Switch
                  id="token-enabled"
                  checked={form.enabled}
                  onCheckedChange={(checked) => setForm({ ...form, enabled: !!checked })}
                />
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

      {/* ── creation success + snippets dialog ── */}
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

      {/* ── delete confirm ── */}
      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        title="删除令牌"
        description={`确认删除"${deleteTarget?.name ?? ""}"吗？该令牌会立即失效。`}
        onConfirm={confirmDelete}
      />
    </div>
  );
}
