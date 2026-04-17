import { Eye, RefreshCw } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

import type { NotificationEventType, NotificationSeverity } from "./types";
import { SEVERITY_OPTIONS } from "./types";

export type PreviewResult = {
  rendered_title: string;
  rendered_body: string;
};

export default function PreviewPanel({
  eventTypes,
  event,
  severity,
  result,
  loading,
  hasPreviewed,
  onEventChange,
  onSeverityChange,
  onRun,
}: {
  eventTypes: NotificationEventType[];
  event: string;
  severity: NotificationSeverity;
  result: PreviewResult | null;
  loading: boolean;
  hasPreviewed: boolean;
  onEventChange: (value: string) => void;
  onSeverityChange: (value: NotificationSeverity) => void;
  onRun: () => void;
}) {
  return (
    <section className="space-y-3 rounded-[1.2rem] border border-moon-200/55 bg-[linear-gradient(180deg,rgba(255,255,255,0.86),rgba(245,243,249,0.7))] px-4 py-4">
      <div className="space-y-1">
        <p className="text-sm font-medium text-moon-800">渲染预览</p>
        <p className="text-xs leading-5 text-moon-400">
          选择事件与严重级别，预览当前 channel 最终会收到的 title 和 body。
        </p>
      </div>
      <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_10rem_auto]">
        <Select value={event} onValueChange={(value) => onEventChange(value ?? event)}>
          <SelectTrigger className="h-10 rounded-xl border-moon-200/65 bg-white/82">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {eventTypes.map((item) => (
              <SelectItem key={item.event} value={item.event}>
                {item.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select
          value={severity}
          onValueChange={(value) => onSeverityChange(value as NotificationSeverity)}
        >
          <SelectTrigger className="h-10 rounded-xl border-moon-200/65 bg-white/82">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {SEVERITY_OPTIONS.map((item) => (
              <SelectItem key={item} value={item}>
                {item}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Button
          variant="outline"
          className="rounded-full"
          onClick={onRun}
          disabled={loading}
        >
          {loading ? (
            <RefreshCw className="size-4 animate-spin" />
          ) : (
            <Eye className="size-4" />
          )}
          Preview
        </Button>
      </div>
      {result ? (
        <div className="grid gap-3 rounded-[1.2rem] border border-moon-200/55 bg-white/80 px-4 py-4 sm:grid-cols-2">
          <div>
            <p className="text-[11px] tracking-[0.16em] text-moon-350">TITLE</p>
            <p className="mt-2 text-sm text-moon-700">
              {result.rendered_title || "--"}
            </p>
          </div>
          <div>
            <p className="text-[11px] tracking-[0.16em] text-moon-350">BODY</p>
            <p className="mt-2 whitespace-pre-wrap text-sm leading-6 text-moon-600">
              {result.rendered_body || "--"}
            </p>
          </div>
        </div>
      ) : hasPreviewed ? (
        <div className="rounded-[1.2rem] border border-dashed border-moon-200/55 bg-white/72 px-4 py-4 text-xs leading-5 text-moon-450">
          未匹配此渠道：当前事件或严重级别未触发此 channel 的订阅。
        </div>
      ) : null}
    </section>
  );
}
