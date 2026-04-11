import { FormEvent, useState } from "react";
import { lunePost } from "../lib/api";
import { toast } from "../components/Feedback";

export default function SetupWizard({
  onComplete,
}: {
  onComplete: () => void;
}) {
  const [backendUrl, setBackendUrl] = useState("http://localhost:3000");
  const [backendKey, setBackendKey] = useState("");
  const [tokenName] = useState("default");
  const [accessToken, setAccessToken] = useState(
    "sk-lune-" + crypto.randomUUID().replace(/-/g, "").slice(0, 32),
  );
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState<string | null>(null);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError("");
    setLoading(true);

    try {
      const data = await lunePost<{ access_token: string }>(
        "/admin/api/bootstrap",
        {
          backend_url: backendUrl,
          backend_key: backendKey,
          access_token: accessToken,
          token_name: tokenName,
        },
      );
      setResult(data.access_token);
      toast("引导完成");
    } catch (err) {
      setError(err instanceof Error ? err.message : "引导失败");
    } finally {
      setLoading(false);
    }
  }

  if (result) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-paper-50 px-4">
        <div className="w-full max-w-md rounded-xl border border-paper-200 bg-paper-100 p-8">
          <h1
            className="mb-2 text-center text-2xl font-semibold text-paper-700"
            style={{
              fontFamily:
                '"Iowan Old Style","Palatino Linotype","Noto Serif SC",Georgia,serif',
            }}
          >
            Lune
          </h1>
          <p className="mb-6 text-center text-sm text-paper-400">
            配置完成
          </p>

          <div className="mb-6 rounded-lg border border-sage-500/30 bg-sage-500/10 p-4">
            <p className="mb-2 text-xs font-medium text-sage-600">
              你的 API 访问令牌
            </p>
            <code className="block break-all text-sm text-paper-800">
              {result}
            </code>
            <p className="mt-2 text-xs text-paper-400">
              请妥善保存，后续不再显示完整值
            </p>
          </div>

          <button
            onClick={onComplete}
            className="w-full rounded-md bg-paper-700 px-4 py-2.5 text-sm font-medium text-paper-50 hover:bg-paper-800 transition-colors"
          >
            进入控制台
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-paper-50 px-4">
      <form
        onSubmit={handleSubmit}
        className="w-full max-w-md rounded-xl border border-paper-200 bg-paper-100 p-8"
      >
        <h1
          className="mb-2 text-center text-2xl font-semibold text-paper-700"
          style={{
            fontFamily:
              '"Iowan Old Style","Palatino Linotype","Noto Serif SC",Georgia,serif',
          }}
        >
          Lune
        </h1>
        <p className="mb-6 text-center text-sm text-paper-400">
          首次配置
        </p>

        {error && (
          <div className="mb-4 rounded-md bg-clay-500/15 px-3 py-2 text-sm text-clay-600">
            {error}
          </div>
        )}

        <label className="block text-xs text-paper-500 mb-1">
          后端引擎地址
        </label>
        <input
          type="url"
          value={backendUrl}
          onChange={(e) => setBackendUrl(e.target.value)}
          required
          className="mb-4 block w-full rounded-md border border-paper-200 bg-paper-50 px-3 py-2 text-sm text-paper-800 placeholder:text-paper-300 focus:border-paper-500 focus:outline-none"
          placeholder="http://localhost:3000"
        />

        <label className="block text-xs text-paper-500 mb-1">
          后端 API Key
        </label>
        <input
          type="password"
          value={backendKey}
          onChange={(e) => setBackendKey(e.target.value)}
          required
          className="mb-4 block w-full rounded-md border border-paper-200 bg-paper-50 px-3 py-2 text-sm text-paper-800 placeholder:text-paper-300 focus:border-paper-500 focus:outline-none"
          placeholder="one-api 的令牌"
        />

        <label className="block text-xs text-paper-500 mb-1">
          Lune 访问令牌（供客户端使用）
        </label>
        <div className="mb-4 flex gap-2">
          <input
            type="text"
            value={accessToken}
            onChange={(e) => setAccessToken(e.target.value)}
            required
            className="flex-1 rounded-md border border-paper-200 bg-paper-50 px-3 py-2 text-sm text-paper-800 font-mono focus:border-paper-500 focus:outline-none"
          />
          <button
            type="button"
            onClick={() =>
              setAccessToken(
                "sk-lune-" +
                  crypto.randomUUID().replace(/-/g, "").slice(0, 32),
              )
            }
            className="shrink-0 rounded-md border border-paper-200 px-3 py-2 text-xs text-paper-500 hover:bg-paper-200/60 transition-colors"
          >
            重新生成
          </button>
        </div>

        <button
          type="submit"
          disabled={loading}
          className="w-full rounded-md bg-paper-700 px-4 py-2.5 text-sm font-medium text-paper-50 hover:bg-paper-800 disabled:opacity-50 transition-colors"
        >
          {loading ? "配置中..." : "完成配置"}
        </button>
      </form>
    </div>
  );
}
