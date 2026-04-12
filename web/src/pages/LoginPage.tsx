import { FormEvent, useState } from "react";
import { setLuneToken } from "../lib/auth";
import { luneGet } from "../lib/api";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Loader2 } from "lucide-react";

export default function LoginPage() {
  const [luneToken, setLuneTokenLocal] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError("");
    setLoading(true);

    try {
      setLuneToken(luneToken);
      await luneGet<{
        overview: { needs_bootstrap: boolean };
      }>("/admin/api/overview");

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

            <p className="mb-4 text-sm text-muted-foreground">
              输入 Lune Admin Token 后即可访问全部管理功能。后端管理会话由
              Lune 在服务端自动处理。
            </p>

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
