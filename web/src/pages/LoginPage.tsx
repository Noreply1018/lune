import { FormEvent, useState } from "react";
import { setLuneToken } from "../lib/auth";
import { luneGet } from "../lib/api";
import { backendLogin } from "../lib/backend";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Loader2 } from "lucide-react";

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
      setLuneToken(luneToken);
      const data = await luneGet<{
        overview: { needs_bootstrap: boolean };
      }>("/admin/api/overview");

      if (!data.overview.needs_bootstrap && password) {
        try {
          await backendLogin(username, password);
        } catch {
          // Backend login failure is non-fatal
        }
      }

      window.location.href = "/admin";
    } catch (err) {
      setLuneToken("");
      setError(err instanceof Error ? err.message : "登录失败");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center px-4">
      <Card className="w-full max-w-sm">
        <CardContent className="p-8">
          <form onSubmit={handleSubmit}>
            <h1
              className="mb-6 text-center text-2xl font-semibold"
              style={{
                fontFamily:
                  '"Iowan Old Style","Palatino Linotype","Noto Serif SC",Georgia,serif',
              }}
            >
              Lune
            </h1>

            {error && (
              <div className="mb-4 rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
                {error}
              </div>
            )}

            <div className="space-y-2 mb-4">
              <Label htmlFor="admin-token">Admin Token</Label>
              <Input
                id="admin-token"
                type="password"
                value={luneToken}
                onChange={(e) => setLuneTokenLocal(e.target.value)}
                required
                placeholder="在终端中查看自动生成的 token"
              />
            </div>

            <details className="mb-4">
              <summary className="cursor-pointer text-xs text-muted-foreground hover:text-foreground transition-colors">
                后端引擎登录（可选）
              </summary>
              <div className="mt-3 space-y-3">
                <div className="space-y-2">
                  <Label htmlFor="username">用户名</Label>
                  <Input
                    id="username"
                    type="text"
                    value={username}
                    onChange={(e) => setUsername(e.target.value)}
                    placeholder="root"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="password">密码</Label>
                  <Input
                    id="password"
                    type="password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    placeholder="password"
                  />
                </div>
              </div>
            </details>

            <Button type="submit" disabled={loading} className="w-full">
              {loading && <Loader2 className="animate-spin" />}
              {loading ? "登录中..." : "登录"}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
