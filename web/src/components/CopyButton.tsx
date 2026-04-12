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

  function handleCopy() {
    navigator.clipboard.writeText(value);
    setCopied(true);
    toast("Copied!");
    setTimeout(() => setCopied(false), 2000);
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
      {label && <span className="ml-1">{copied ? "Copied!" : label}</span>}
    </Button>
  );
}
