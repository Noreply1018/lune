import { FormEvent, useState } from "react";
import { setLuneToken } from "../lib/auth";
import { luneGet } from "../lib/api";
import { oneapiLogin } from "../lib/oneapi";

export default function LoginPage() {
  const [luneToken, setLuneTokenLocal] = useState("");
  const [username, setUsername] = useState("root");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError("");
    setLoading(true);

    try {
      // 1. Validate Lune admin token
      setLuneToken(luneToken);
      await luneGet("/admin/api/overview");

      // 2. Login to One-API
      await oneapiLogin(username, password);

      // Both succeeded — navigate
      window.location.href = "/admin";
    } catch (err) {
      setLuneToken("");
      setError(err instanceof Error ? err.message : "登录失败");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-paper-50 px-4">
      <form
        onSubmit={handleSubmit}
        className="w-full max-w-sm rounded-xl border border-paper-200 bg-paper-100 p-8"
      >
        <h1
          className="mb-6 text-center text-2xl font-semibold text-paper-700"
          style={{
            fontFamily:
              '"Iowan Old Style","Palatino Linotype","Noto Serif SC",Georgia,serif',
          }}
        >
          Lune
        </h1>

        {error && (
          <div className="mb-4 rounded-md bg-clay-500/15 px-3 py-2 text-sm text-clay-600">
            {error}
          </div>
        )}

        <label className="block text-xs text-paper-500 mb-1">
          Lune Admin Token
        </label>
        <input
          type="password"
          value={luneToken}
          onChange={(e) => setLuneTokenLocal(e.target.value)}
          required
          className="mb-4 block w-full rounded-md border border-paper-200 bg-paper-50 px-3 py-2 text-sm text-paper-800 placeholder:text-paper-300 focus:border-paper-500 focus:outline-none"
          placeholder="admin token"
        />

        <label className="block text-xs text-paper-500 mb-1">
          One-API 用户名
        </label>
        <input
          type="text"
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          required
          className="mb-4 block w-full rounded-md border border-paper-200 bg-paper-50 px-3 py-2 text-sm text-paper-800 placeholder:text-paper-300 focus:border-paper-500 focus:outline-none"
          placeholder="root"
        />

        <label className="block text-xs text-paper-500 mb-1">
          One-API 密码
        </label>
        <input
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          required
          className="mb-6 block w-full rounded-md border border-paper-200 bg-paper-50 px-3 py-2 text-sm text-paper-800 placeholder:text-paper-300 focus:border-paper-500 focus:outline-none"
          placeholder="password"
        />

        <button
          type="submit"
          disabled={loading}
          className="w-full rounded-md bg-paper-700 px-4 py-2.5 text-sm font-medium text-paper-50 hover:bg-paper-800 disabled:opacity-50 transition-colors"
        >
          {loading ? "登录中..." : "登录"}
        </button>
      </form>
    </div>
  );
}
