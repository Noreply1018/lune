import { FormEvent, useState } from "react";
import { lunePost } from "../lib/api";
import { toast } from "../components/Feedback";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Loader2, Copy, RefreshCw } from "lucide-react";

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

  function copyToken() {
    if (result) {
      navigator.clipboard.writeText(result);
      toast("已复制到剪贴板");
    }
  }

  if (result) {
    return (
      <div className="flex min-h-screen items-center justify-center px-4">
        <Card className="w-full max-w-md">
          <CardContent className="p-8">
            <h1
              className="mb-2 text-center text-2xl font-semibold"
              style={{
                fontFamily:
                  '"Iowan Old Style","Palatino Linotype","Noto Serif SC",Georgia,serif',
              }}
            >
              Lune
            </h1>
            <p className="mb-6 text-center text-sm text-muted-foreground">
              配置完成
            </p>

            <div className="mb-6 rounded-lg border border-sage-500/30 bg-sage-500/10 p-4">
              <p className="mb-2 text-xs font-medium text-sage-600">
                你的 API 访问令牌
              </p>
              <div className="flex items-start gap-2">
                <code className="flex-1 break-all text-sm">{result}</code>
                <Button
                  variant="ghost"
                  size="icon"
                  className="shrink-0"
                  onClick={copyToken}
                >
                  <Copy className="size-4" />
                </Button>
              </div>
              <p className="mt-2 text-xs text-muted-foreground">
                请妥善保存，后续不再显示完整值
              </p>
            </div>

            <Button onClick={onComplete} className="w-full">
              进入控制台
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="flex min-h-screen items-center justify-center px-4">
      <Card className="w-full max-w-md">
        <CardContent className="p-8">
          <form onSubmit={handleSubmit}>
            <h1
              className="mb-2 text-center text-2xl font-semibold"
              style={{
                fontFamily:
                  '"Iowan Old Style","Palatino Linotype","Noto Serif SC",Georgia,serif',
              }}
            >
              Lune
            </h1>
            <p className="mb-6 text-center text-sm text-muted-foreground">
              首次配置
            </p>

            {error && (
              <div className="mb-4 rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
                {error}
              </div>
            )}

            <div className="space-y-2 mb-4">
              <Label htmlFor="backend-url">后端引擎地址</Label>
              <Input
                id="backend-url"
                type="url"
                value={backendUrl}
                onChange={(e) => setBackendUrl(e.target.value)}
                required
                placeholder="http://localhost:3000"
              />
            </div>

            <div className="space-y-2 mb-4">
              <Label htmlFor="backend-key">后端 API Key</Label>
              <Input
                id="backend-key"
                type="password"
                value={backendKey}
                onChange={(e) => setBackendKey(e.target.value)}
                required
                placeholder="one-api 的令牌"
              />
            </div>

            <div className="space-y-2 mb-4">
              <Label htmlFor="access-token">
                Lune 访问令牌（供客户端使用）
              </Label>
              <div className="flex gap-2">
                <Input
                  id="access-token"
                  type="text"
                  value={accessToken}
                  onChange={(e) => setAccessToken(e.target.value)}
                  required
                  className="font-mono"
                />
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  onClick={() =>
                    setAccessToken(
                      "sk-lune-" +
                        crypto.randomUUID().replace(/-/g, "").slice(0, 32),
                    )
                  }
                >
                  <RefreshCw className="size-4" />
                </Button>
              </div>
            </div>

            <Button type="submit" disabled={loading} className="w-full">
              {loading && <Loader2 className="animate-spin" />}
              {loading ? "配置中..." : "完成配置"}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
