import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";

import { CHANNEL_TYPE_META } from "./types";
import type { NotificationChannelDraft } from "./types";

export default function CreateChannelPanel({
  draft,
  creating,
  error,
  onDraftChange,
  onCreate,
  onCancel,
}: {
  draft: NotificationChannelDraft;
  creating?: boolean;
  error?: string | null;
  onDraftChange: (next: NotificationChannelDraft) => void;
  onCreate: () => void;
  onCancel: () => void;
}) {
  const meta = CHANNEL_TYPE_META[draft.type];

  return (
    <div className="rounded-[1.45rem] border border-white/80 bg-[linear-gradient(180deg,rgba(255,255,255,0.95),rgba(245,242,249,0.82))] px-4 py-4 shadow-[0_28px_72px_-50px_rgba(33,40,63,0.28)]">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <p className="text-sm font-medium text-moon-800">创建 {meta.label}</p>
          <p className="text-xs leading-5 text-moon-400">
            先填写基础配置，再创建 channel。创建后会直接展开详情，继续补充订阅、模板和重试策略。
          </p>
        </div>
        <span className={cn("rounded-full px-2.5 py-1 text-[11px] tracking-[0.14em]", meta.tone)}>
          {meta.label}
        </span>
      </div>

      <div className="mt-4 grid gap-3 lg:grid-cols-[minmax(0,1fr)_12rem]">
        <label className="space-y-2">
          <span className="text-xs font-medium tracking-[0.16em] text-moon-350">
            NAME
          </span>
          <Input
            value={draft.name}
            onChange={(event) =>
              onDraftChange({ ...draft, name: event.target.value })
            }
            placeholder="例如：Ops Feishu"
          />
        </label>
        <div className="space-y-2">
          <span className="text-xs font-medium tracking-[0.16em] text-moon-350">
            INITIAL STATE
          </span>
          <div className="rounded-[1rem] border border-moon-200/60 bg-moon-50/80 px-3 py-2.5 text-sm text-moon-700">
            创建后默认保持停用
          </div>
        </div>
      </div>

      <div className="mt-4 grid gap-3 sm:grid-cols-2">
        {meta.fields.map((field) => (
          <label
            key={field.key}
            className={cn("space-y-2", field.multiline ? "sm:col-span-2" : "")}
          >
            <span className="text-xs font-medium tracking-[0.16em] text-moon-350">
              {field.label}
            </span>
            {field.multiline ? (
              <textarea
                value={draft.config[field.key] ?? ""}
                onChange={(event) =>
                  onDraftChange({
                    ...draft,
                    config: {
                      ...draft.config,
                      [field.key]: event.target.value,
                    },
                  })
                }
                className="min-h-28 w-full rounded-[1rem] border border-moon-200/65 bg-white/82 px-3 py-3 text-sm text-moon-700 outline-none transition focus:border-lunar-300/70"
                placeholder={field.placeholder}
              />
            ) : (
              <Input
                type={field.secret ? "password" : "text"}
                value={draft.config[field.key] ?? ""}
                onChange={(event) =>
                  onDraftChange({
                    ...draft,
                    config: {
                      ...draft.config,
                      [field.key]: event.target.value,
                    },
                  })
                }
                placeholder={field.placeholder}
              />
            )}
            {field.helper ? (
              <p className="text-xs leading-5 text-moon-400">{field.helper}</p>
            ) : null}
          </label>
        ))}
      </div>

      {error ? (
        <div className="mt-4 rounded-[1rem] border border-status-red/18 bg-status-red/6 px-3 py-3 text-sm text-status-red">
          {error}
        </div>
      ) : null}

      <div className="mt-4 flex flex-wrap items-center justify-between gap-3 border-t border-moon-200/45 pt-4">
        <p className="text-xs text-moon-400">
          只有创建成功后才会真正写入数据库，不会留下占位 channel。
        </p>
        <div className="flex items-center gap-2">
          <Button variant="outline" className="rounded-full" onClick={onCancel}>
            Cancel
          </Button>
          <Button className="rounded-full" onClick={onCreate} disabled={creating}>
            {creating ? "Creating..." : "Create Channel"}
          </Button>
        </div>
      </div>
    </div>
  );
}
