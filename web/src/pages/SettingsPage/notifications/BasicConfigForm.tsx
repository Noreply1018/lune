import { Copy, Eye, EyeOff, RefreshCw } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useRouter } from "@/lib/router";
import { cn } from "@/lib/utils";

import { CHANNEL_TYPE_META, SECRET_PLACEHOLDER } from "./types";
import type { NotificationChannelDraft } from "./types";

export default function BasicConfigForm({
  draft,
  savingField,
  onDraftChange,
  onCommit,
}: {
  draft: NotificationChannelDraft;
  savingField: string | null;
  onDraftChange: (next: NotificationChannelDraft) => void;
  onCommit: (field: string, next?: NotificationChannelDraft) => void;
}) {
  const [revealedFields, setRevealedFields] = useState<Record<string, boolean>>(
    {},
  );
  const { navigate } = useRouter();
  const meta = CHANNEL_TYPE_META[draft.type];

  return (
    <section className="space-y-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <p className="text-sm font-medium text-moon-800">基础配置</p>
          <p className="text-xs leading-5 text-moon-400">
            类型固定不变，其余字段失焦后自动保存。
          </p>
        </div>
        <button
          type="button"
          className="text-sm text-moon-500 underline underline-offset-4"
          onClick={() => navigate(`/admin/activity?channel_id=${draft.id}`)}
        >
          查看 Activity
        </button>
      </div>

      <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_12rem]">
        <label className="space-y-2">
          <span className="text-xs font-medium tracking-[0.16em] text-moon-350">
            NAME
          </span>
          <div className="flex items-center gap-2">
            <Input
              value={draft.name}
              onChange={(event) =>
                onDraftChange({ ...draft, name: event.target.value })
              }
              onBlur={(event) =>
                onCommit("name", { ...draft, name: event.currentTarget.value })
              }
              placeholder="例如：Ops Feishu"
            />
            {savingField === "name" ? (
              <RefreshCw className="size-4 animate-spin text-moon-350" />
            ) : null}
          </div>
        </label>
        <div className="space-y-2">
          <span className="text-xs font-medium tracking-[0.16em] text-moon-350">
            TYPE
          </span>
          <div className="rounded-[1rem] border border-moon-200/60 bg-moon-50/80 px-3 py-2.5 text-sm text-moon-700">
            {meta.label}
          </div>
        </div>
      </div>

      <div className="grid gap-3 sm:grid-cols-2">
        {meta.fields.map((field) => {
          const value = draft.config[field.key] ?? "";
          const showSecret = revealedFields[field.key];
          const displayValue =
            field.secret &&
            draft.preservedSecrets[field.key] &&
            value.trim() === ""
              ? SECRET_PLACEHOLDER
              : value;

          return (
            <label
              key={field.key}
              className={cn(
                "space-y-2",
                field.multiline ? "sm:col-span-2" : "",
              )}
            >
              <span className="text-xs font-medium tracking-[0.16em] text-moon-350">
                {field.label}
              </span>
              <div className="relative">
                {field.multiline ? (
                  <textarea
                    value={displayValue}
                    onChange={(event) =>
                      onDraftChange({
                        ...draft,
                        preservedSecrets: {
                          ...draft.preservedSecrets,
                          [field.key]: false,
                        },
                        config: {
                          ...draft.config,
                          [field.key]: event.target.value,
                        },
                      })
                    }
                    onBlur={(event) =>
                      onCommit(field.key, {
                        ...draft,
                        preservedSecrets: {
                          ...draft.preservedSecrets,
                          [field.key]: false,
                        },
                        config: {
                          ...draft.config,
                          [field.key]: event.currentTarget.value,
                        },
                      })
                    }
                    className="min-h-28 w-full rounded-[1rem] border border-moon-200/65 bg-white/82 px-3 py-3 text-sm text-moon-700 outline-none transition focus:border-lunar-300/70"
                    placeholder={field.placeholder}
                  />
                ) : (
                  <Input
                    type={
                      field.secret && !showSecret ? "password" : "text"
                    }
                    value={displayValue}
                    onChange={(event) =>
                      onDraftChange({
                        ...draft,
                        preservedSecrets: {
                          ...draft.preservedSecrets,
                          [field.key]: false,
                        },
                        config: {
                          ...draft.config,
                          [field.key]: event.target.value,
                        },
                      })
                    }
                    onBlur={(event) =>
                      onCommit(
                        field.key,
                        field.secret &&
                          draft.preservedSecrets[field.key] &&
                          displayValue === SECRET_PLACEHOLDER
                          ? draft
                          : {
                              ...draft,
                              preservedSecrets: {
                                ...draft.preservedSecrets,
                                [field.key]: false,
                              },
                              config: {
                                ...draft.config,
                                [field.key]: event.currentTarget.value,
                              },
                            },
                      )
                    }
                    placeholder={field.placeholder}
                    className={field.secret ? "pr-22" : undefined}
                  />
                )}
                {field.secret ? (
                  <div className="absolute top-2 right-2 flex items-center gap-1">
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-sm"
                      className="rounded-full text-moon-450"
                      onClick={() =>
                        setRevealedFields((current) => ({
                          ...current,
                          [field.key]: !current[field.key],
                        }))
                      }
                    >
                      {showSecret ? (
                        <EyeOff className="size-4" />
                      ) : (
                        <Eye className="size-4" />
                      )}
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-sm"
                      className="rounded-full text-moon-450"
                      onClick={async () => {
                        if (!value.trim()) {
                          return;
                        }
                        await navigator.clipboard.writeText(value);
                      }}
                    >
                      <Copy className="size-4" />
                    </Button>
                  </div>
                ) : null}
              </div>
              {field.helper ? (
                <div className="flex items-center gap-2 text-xs text-moon-400">
                  <span>{field.helper}</span>
                  {savingField === field.key ? (
                    <RefreshCw className="size-3.5 animate-spin" />
                  ) : null}
                </div>
              ) : savingField === field.key ? (
                <RefreshCw className="size-3.5 animate-spin text-moon-350" />
              ) : null}
            </label>
          );
        })}
      </div>
    </section>
  );
}
