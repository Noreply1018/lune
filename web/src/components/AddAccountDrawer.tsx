import { useEffect, useMemo, useRef, useState } from "react";
import { Loader2, Plus, ScanQrCode, Sparkles } from "lucide-react";
import { CPA_PROVIDERS, DIRECT_PROVIDER_TEMPLATES, PROVIDER_POOL_RECOMMENDATION } from "@/copy/admin";
import { toast } from "@/components/Feedback";
import { useAdminUI } from "@/components/AdminUI";
import { useRouter } from "@/lib/router";
import { api } from "@/lib/api";
import type { CpaService, LoginSession, Pool } from "@/lib/types";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

const NEW_POOL_VALUE = "__new_pool__";

type SourceKind = "cpa" | "openai_compat";

type DirectForm = {
  label: string;
  provider: string;
  base_url: string;
  api_key: string;
  notes: string;
};

const EMPTY_DIRECT_FORM: DirectForm = {
  label: "",
  provider: "openai",
  base_url: "https://api.openai.com/v1",
  api_key: "",
  notes: "",
};

export default function AddAccountDrawer() {
  const { addAccountOpen, closeAddAccount, preferredPoolId, refreshData } = useAdminUI();
  const { navigate } = useRouter();
  const pollRef = useRef<number | null>(null);

  const [step, setStep] = useState<1 | 2 | 3 | 4>(1);
  const [sourceKind, setSourceKind] = useState<SourceKind>("openai_compat");
  const [provider, setProvider] = useState("openai");
  const [pools, setPools] = useState<Pool[]>([]);
  const [cpaService, setCpaService] = useState<CpaService | null>(null);
  const [selectedPoolId, setSelectedPoolId] = useState<string>("");
  const [newPoolLabel, setNewPoolLabel] = useState("");
  const [directForm, setDirectForm] = useState<DirectForm>(EMPTY_DIRECT_FORM);
  const [loading, setLoading] = useState(false);
  const [loginSession, setLoginSession] = useState<LoginSession | null>(null);
  const [result, setResult] = useState<{ poolId: number; label: string; source: string } | null>(null);

  useEffect(() => {
    if (!addAccountOpen) return;

    setLoading(true);
    Promise.all([
      api.get<Pool[]>("/pools").catch(() => []),
      api.get<CpaService | null>("/cpa/service").catch(() => null),
    ])
      .then(([poolData, service]) => {
        setPools(poolData ?? []);
        setCpaService(service ?? null);
      })
      .finally(() => setLoading(false));
  }, [addAccountOpen]);

  useEffect(() => {
    if (!addAccountOpen) {
      setStep(1);
      setSourceKind("openai_compat");
      setProvider("openai");
      setDirectForm(EMPTY_DIRECT_FORM);
      setLoginSession(null);
      setResult(null);
      setSelectedPoolId("");
      setNewPoolLabel("");
      if (pollRef.current) {
        window.clearInterval(pollRef.current);
        pollRef.current = null;
      }
      return;
    }

    const recommended = PROVIDER_POOL_RECOMMENDATION[provider] ?? "";
    const preferred = preferredPoolId ? String(preferredPoolId) : "";
    const matched = pools.find((pool) => pool.label === recommended);
    setSelectedPoolId(preferred || (matched ? String(matched.id) : recommended ? NEW_POOL_VALUE : ""));
    setNewPoolLabel(recommended);
  }, [addAccountOpen, pools, preferredPoolId, provider]);

  useEffect(() => {
    if (sourceKind !== "openai_compat") return;
    const template = DIRECT_PROVIDER_TEMPLATES.find((item) => item.value === provider);
    if (!template) return;
    setDirectForm((current) => ({
      ...current,
      provider,
      base_url: template.base_url,
      label:
        current.label ||
        (template.label === "自定义" ? "" : `My ${template.label}`),
    }));
  }, [provider, sourceKind]);

  useEffect(() => {
    if (!loginSession || loginSession.status === "succeeded" || loginSession.status === "failed" || loginSession.status === "expired") {
      return;
    }

    if (pollRef.current) {
      window.clearInterval(pollRef.current);
    }

    pollRef.current = window.setInterval(async () => {
      try {
        const session = await api.get<LoginSession>(`/accounts/cpa/login-sessions/${loginSession.id}`);
        setLoginSession(session);
        if (session.status === "succeeded" && session.account_id) {
          const poolId = await ensurePool();
          setResult({
            poolId,
            label: session.account?.label ?? "CPA account",
            source: `CPA · ${provider}`,
          });
          setStep(4);
          refreshData();
          if (pollRef.current) {
            window.clearInterval(pollRef.current);
            pollRef.current = null;
          }
        }
        if (session.status === "failed" || session.status === "expired") {
          if (pollRef.current) {
            window.clearInterval(pollRef.current);
            pollRef.current = null;
          }
        }
      } catch {
        if (pollRef.current) {
          window.clearInterval(pollRef.current);
          pollRef.current = null;
        }
      }
    }, 2500);

    return () => {
      if (pollRef.current) {
        window.clearInterval(pollRef.current);
        pollRef.current = null;
      }
    };
  }, [loginSession, provider, refreshData]);

  const recommendedPoolLabel = useMemo(
    () => PROVIDER_POOL_RECOMMENDATION[provider] ?? "",
    [provider],
  );

  async function ensurePool(): Promise<number> {
    if (selectedPoolId && selectedPoolId !== NEW_POOL_VALUE) {
      return Number(selectedPoolId);
    }

    if (!newPoolLabel.trim()) {
      throw new Error("请输入 Pool 名称");
    }

    const created = await api.post<Pool>("/pools", {
      label: newPoolLabel.trim(),
      priority: pools.length + 1,
    });
    setPools((current) => [...current, created]);
    setSelectedPoolId(String(created.id));
    return created.id;
  }

  function nextFromStepOne() {
    if (sourceKind === "cpa") {
      setProvider("codex");
    }
    setStep(2);
  }

  async function submitDirect() {
    setLoading(true);
    try {
      const poolId = await ensurePool();
      await api.post("/accounts", {
        label: directForm.label.trim(),
        source_kind: "openai_compat",
        provider: directForm.provider,
        base_url: directForm.base_url.trim(),
        api_key: directForm.api_key.trim(),
        notes: directForm.notes.trim(),
        enabled: true,
        pool_id: poolId,
      });
      refreshData();
      setResult({
        poolId,
        label: directForm.label.trim(),
        source: `直连 · ${directForm.provider}`,
      });
      setStep(4);
    } catch (err) {
      toast(err instanceof Error ? err.message : "创建账号失败", "error");
    } finally {
      setLoading(false);
    }
  }

  async function startCpaLogin() {
    if (!cpaService) {
      toast("请先在 Settings 配置 CPA Service", "error");
      return;
    }

    setLoading(true);
    try {
      const poolId = await ensurePool();
      const session = await api.post<LoginSession>("/accounts/cpa/login-sessions", {
        service_id: cpaService.id,
        pool_id: poolId,
      });
      setLoginSession(session);
    } catch (err) {
      toast(err instanceof Error ? err.message : "启动登录失败", "error");
    } finally {
      setLoading(false);
    }
  }

  function complete() {
    if (!result) {
      closeAddAccount();
      return;
    }
    closeAddAccount();
    navigate(`/admin/pools/${result.poolId}`);
  }

  return (
    <Sheet open={addAccountOpen} onOpenChange={(open) => !open && closeAddAccount()}>
      <SheetContent
        side="right"
        className="w-full max-w-[34rem] border-l border-white/75 bg-[linear-gradient(180deg,rgba(252,251,248,0.98),rgba(247,245,241,0.98))] p-0 sm:max-w-[34rem]"
      >
        <SheetHeader className="border-b border-moon-200/60 px-6 py-5">
          <SheetTitle>Add Account</SheetTitle>
          <SheetDescription>
            先选接入方式，再确定归属 Pool，完成后立即可用。
          </SheetDescription>
        </SheetHeader>

        <div className="space-y-6 overflow-y-auto px-6 py-6">
          <div className="flex items-center gap-2 text-xs uppercase tracking-[0.2em] text-moon-400">
            {[1, 2, 3, 4].map((index) => (
              <span
                key={index}
                className={`inline-flex size-7 items-center justify-center rounded-full border ${
                  step >= index
                    ? "border-lunar-300 bg-lunar-100/70 text-lunar-700"
                    : "border-moon-200/70 bg-white/60 text-moon-400"
                }`}
              >
                {index}
              </span>
            ))}
          </div>

          {step === 1 ? (
            <section className="space-y-5">
              <div className="space-y-2">
                <Label>接入方式</Label>
                <div className="grid gap-3 sm:grid-cols-2">
                  <button
                    type="button"
                    onClick={() => setSourceKind("cpa")}
                    className={`rounded-[1.4rem] border px-4 py-4 text-left ${
                      sourceKind === "cpa"
                        ? "border-lunar-300 bg-lunar-100/55"
                        : "border-moon-200/60 bg-white/70"
                    }`}
                  >
                    <p className="text-sm font-semibold text-moon-800">CPA</p>
                    <p className="mt-1 text-sm text-moon-500">通过 CPA 登录接入。</p>
                  </button>
                  <button
                    type="button"
                    onClick={() => setSourceKind("openai_compat")}
                    className={`rounded-[1.4rem] border px-4 py-4 text-left ${
                      sourceKind === "openai_compat"
                        ? "border-lunar-300 bg-lunar-100/55"
                        : "border-moon-200/60 bg-white/70"
                    }`}
                  >
                    <p className="text-sm font-semibold text-moon-800">直连</p>
                    <p className="mt-1 text-sm text-moon-500">连接 OpenAI 兼容上游。</p>
                  </button>
                </div>
              </div>

              <div className="space-y-2">
                <Label>Provider</Label>
                <Select value={provider} onValueChange={(value) => setProvider(value ?? "openai")}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {(sourceKind === "cpa" ? CPA_PROVIDERS : DIRECT_PROVIDER_TEMPLATES).map((item) => (
                      <SelectItem key={item.value} value={item.value}>
                        {item.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {sourceKind === "cpa" ? (
                  <p className="text-xs text-moon-400">
                    当前 Device Code 登录路径优先支持 Codex。
                  </p>
                ) : null}
              </div>
            </section>
          ) : null}

          {step === 2 ? (
            <section className="space-y-5">
              <div className="rounded-[1.4rem] border border-lunar-200/60 bg-lunar-100/45 px-4 py-4">
                <p className="eyebrow-label">Recommended</p>
                <p className="mt-2 text-lg font-semibold text-moon-800">
                  {recommendedPoolLabel || "手动选择 Pool"}
                </p>
                <p className="mt-1 text-sm text-moon-500">
                  接入方式和 Provider 决定默认归属，但你仍可改到其他 Pool。
                </p>
              </div>

              <div className="space-y-2">
                <Label>归属 Pool</Label>
                <Select
                  value={selectedPoolId || NEW_POOL_VALUE}
                  onValueChange={(value) => setSelectedPoolId(value ?? NEW_POOL_VALUE)}
                >
                  <SelectTrigger>
                    <SelectValue placeholder="选择 Pool" />
                  </SelectTrigger>
                  <SelectContent>
                    {pools.map((pool) => (
                      <SelectItem key={pool.id} value={String(pool.id)}>
                        {pool.label}
                      </SelectItem>
                    ))}
                    <SelectItem value={NEW_POOL_VALUE}>+ 新建 Pool</SelectItem>
                  </SelectContent>
                </Select>
              </div>

              {(selectedPoolId === NEW_POOL_VALUE || !selectedPoolId) ? (
                <div className="space-y-2">
                  <Label>新 Pool 名称</Label>
                  <Input
                    value={newPoolLabel}
                    onChange={(event) => setNewPoolLabel(event.target.value)}
                    placeholder="例如 OpenAI / Claude / Gemini"
                  />
                </div>
              ) : null}
            </section>
          ) : null}

          {step === 3 && sourceKind === "openai_compat" ? (
            <section className="space-y-4">
              <div className="space-y-2">
                <Label>Label</Label>
                <Input
                  value={directForm.label}
                  onChange={(event) => setDirectForm((current) => ({ ...current, label: event.target.value }))}
                />
              </div>
              <div className="space-y-2">
                <Label>Base URL</Label>
                <Input
                  value={directForm.base_url}
                  onChange={(event) => setDirectForm((current) => ({ ...current, base_url: event.target.value }))}
                />
              </div>
              <div className="space-y-2">
                <Label>API Key</Label>
                <Input
                  value={directForm.api_key}
                  type="password"
                  onChange={(event) => setDirectForm((current) => ({ ...current, api_key: event.target.value }))}
                />
              </div>
              <div className="space-y-2">
                <Label>Notes</Label>
                <Input
                  value={directForm.notes}
                  onChange={(event) => setDirectForm((current) => ({ ...current, notes: event.target.value }))}
                />
              </div>
            </section>
          ) : null}

          {step === 3 && sourceKind === "cpa" ? (
            <section className="space-y-5">
              <div className="rounded-[1.5rem] border border-white/75 bg-white/78 px-4 py-4">
                <p className="eyebrow-label">Device Code Login</p>
                {loginSession ? (
                  <div className="space-y-4">
                    <p className="mt-2 text-sm text-moon-500">
                      在浏览器中完成授权，然后等待 Lune 自动收口。
                    </p>
                    <div className="rounded-[1.2rem] bg-moon-100/60 px-4 py-4">
                      <p className="text-xs uppercase tracking-[0.18em] text-moon-400">Verification URI</p>
                      <p className="mt-2 break-all text-sm text-moon-700">{loginSession.verification_uri}</p>
                      <p className="mt-3 text-xs uppercase tracking-[0.18em] text-moon-400">User Code</p>
                      <p className="mt-2 text-xl font-semibold tracking-[0.14em] text-moon-800">
                        {loginSession.user_code}
                      </p>
                    </div>
                    {loginSession.status === "failed" || loginSession.status === "expired" ? (
                      <p className="text-sm text-status-red">
                        {loginSession.error_message || "登录未完成"}
                      </p>
                    ) : (
                      <p className="text-sm text-moon-500">
                        当前状态：{loginSession.status}
                      </p>
                    )}
                  </div>
                ) : (
                  <div className="space-y-3">
                    <p className="mt-2 text-sm leading-7 text-moon-500">
                      通过 Device Code 登录你的 CPA 账号。完成授权后，账号会自动加入当前 Pool。
                    </p>
                    {!cpaService ? (
                      <p className="text-sm text-status-red">Settings 中尚未配置 CPA Service。</p>
                    ) : null}
                  </div>
                )}
              </div>
            </section>
          ) : null}

          {step === 4 && result ? (
            <section className="space-y-5">
              <div className="rounded-[1.6rem] border border-lunar-200/65 bg-lunar-100/40 px-5 py-5">
                <div className="flex items-start gap-3">
                  <span className="inline-flex size-10 items-center justify-center rounded-full bg-white/80 text-lunar-700">
                    <Sparkles className="size-5" />
                  </span>
                  <div className="space-y-2">
                    <p className="eyebrow-label">Ready</p>
                    <h3 className="text-xl font-semibold tracking-[-0.03em] text-moon-800">
                      账号已接入
                    </h3>
                    <p className="text-sm text-moon-500">
                      {result.source} · {result.label}
                    </p>
                  </div>
                </div>
              </div>
            </section>
          ) : null}
        </div>

        <SheetFooter className="border-t border-moon-200/60 bg-white/70 px-6 py-4">
          {step > 1 && step < 4 ? (
            <Button variant="outline" onClick={() => setStep((current) => (current - 1) as 1 | 2 | 3)}>
              返回
            </Button>
          ) : null}

          {step === 1 ? (
            <Button onClick={nextFromStepOne}>
              <Plus className="size-4" />
              继续
            </Button>
          ) : null}

          {step === 2 ? (
            <Button onClick={() => setStep(3)} disabled={!selectedPoolId && !newPoolLabel.trim()}>
              <ScanQrCode className="size-4" />
              确认 Pool
            </Button>
          ) : null}

          {step === 3 && sourceKind === "openai_compat" ? (
            <Button
              onClick={submitDirect}
              disabled={loading || !directForm.label.trim() || !directForm.base_url.trim() || !directForm.api_key.trim()}
            >
              {loading ? <Loader2 className="size-4 animate-spin" /> : null}
              创建账号
            </Button>
          ) : null}

          {step === 3 && sourceKind === "cpa" && !loginSession ? (
            <Button onClick={startCpaLogin} disabled={loading || !cpaService}>
              {loading ? <Loader2 className="size-4 animate-spin" /> : null}
              开始登录
            </Button>
          ) : null}

          {step === 4 ? (
            <Button onClick={complete}>前往 Pool 详情</Button>
          ) : null}
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}
