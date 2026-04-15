import CopyButton from "@/components/CopyButton";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

const SNIPPET_LABELS = ["OpenAI SDK", "Shell", "Cursor", "curl"] as const;

function getSnippets(baseUrl: string, token: string, model?: string) {
  const safeModel = model || "gpt-4o";

  return {
    "OpenAI SDK": `from openai import OpenAI\n\nclient = OpenAI(\n    api_key="${token}",\n    base_url="${baseUrl}",\n)\n\nresp = client.chat.completions.create(\n    model="${safeModel}",\n    messages=[{"role": "user", "content": "Hello"}],\n)`,
    Shell: `export OPENAI_API_KEY=${token}\nexport OPENAI_BASE_URL=${baseUrl}`,
    Cursor: `{\n  "openai.apiKey": "${token}",\n  "openai.baseUrl": "${baseUrl}"\n}`,
    curl: `curl ${baseUrl}/chat/completions \\\n  -H "Authorization: Bearer ${token}" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"${safeModel}","messages":[{"role":"user","content":"Hello"}]}'`,
  };
}

export default function EnvSnippetsDialog({
  open,
  onOpenChange,
  title,
  baseUrl,
  token,
  model,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  baseUrl: string;
  token: string;
  model?: string;
}) {
  const snippets = getSnippets(baseUrl, token, model);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-4xl rounded-[1.6rem] border border-white/75 bg-white/92 p-0 sm:max-w-4xl">
        <DialogHeader className="border-b border-moon-200/60 px-6 py-5">
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>
            复制当前地址与 Token，对接 SDK、CLI 或桌面工具。
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4 px-6 py-5">
          {SNIPPET_LABELS.map((label) => (
            <section key={label} className="surface-outline overflow-hidden">
              <div className="flex items-center justify-between border-b border-moon-200/50 px-4 py-3">
                <h3 className="text-sm font-semibold text-moon-700">{label}</h3>
                <CopyButton value={snippets[label]} label="复制" />
              </div>
              <pre className="overflow-x-auto px-4 py-4 text-xs leading-6 text-moon-700">
                <code>{snippets[label]}</code>
              </pre>
            </section>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  );
}
