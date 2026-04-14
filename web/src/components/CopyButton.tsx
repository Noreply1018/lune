import { useState } from "react";
import { Copy, Check } from "lucide-react";
import { Button } from "@/components/ui/button";
import { toast } from "@/components/Feedback";
import { cn } from "@/lib/utils";

export default function CopyButton({
  value,
  label,
  className,
}: {
  value: string;
  label?: string;
  className?: string;
}) {
  const [copied, setCopied] = useState(false);

  async function handleCopy() {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      toast("已复制");
      setTimeout(() => setCopied(false), 2000);
    } catch {
      toast("复制失败", "error");
    }
  }

  return (
    <Button
      variant="ghost"
      size={label ? "sm" : "icon"}
      className={cn(label ? "" : "size-7", className)}
      onClick={handleCopy}
    >
      {copied ? (
        <Check className="size-3.5 text-status-green" />
      ) : (
        <Copy className="size-3.5" />
      )}
      {label && <span className="ml-1">{copied ? "已复制" : label}</span>}
    </Button>
  );
}
