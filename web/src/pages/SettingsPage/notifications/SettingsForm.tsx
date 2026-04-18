import { useEffect, useState, type ReactNode } from "react";

import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";

import MobileChipInput from "./MobileChipInput";
import type { NotificationSettings } from "./types";

type SettingsFormProps = {
  settings: NotificationSettings;
  saving: boolean;
  urlError?: string | null;
  onChange: (next: NotificationSettings) => void;
  onCommit: (next: NotificationSettings) => void;
  testSlot?: ReactNode;
};

export default function SettingsForm({
  settings,
  saving,
  urlError,
  onChange,
  onCommit,
  testSlot,
}: SettingsFormProps) {
  const [localUrl, setLocalUrl] = useState(settings.webhook_url);

  useEffect(() => {
    setLocalUrl(settings.webhook_url);
  }, [settings.webhook_url]);

  function commitUrl() {
    const trimmed = localUrl.trim();
    if (trimmed === settings.webhook_url) {
      return;
    }
    const next = { ...settings, webhook_url: trimmed };
    onChange(next);
    onCommit(next);
  }

  return (
    <div className="space-y-5">
      <div className="grid gap-4 sm:grid-cols-2 sm:items-stretch">
        <div className="flex items-center justify-between gap-4 rounded-[1rem] border border-white/75 bg-white/75 px-4 py-3">
          <div>
            <p className="text-sm font-medium text-moon-800">启用通知</p>
            <p className="text-xs leading-5 text-moon-400">
              关闭后所有事件都不再投递，已配置的企微信息保留。
            </p>
          </div>
          <Switch
            checked={settings.enabled}
            disabled={saving}
            onCheckedChange={(checked) => {
              const next = { ...settings, enabled: checked };
              onChange(next);
              onCommit(next);
            }}
          />
        </div>
        {testSlot ?? null}
      </div>

      {!settings.enabled ? (
        <div className="rounded-[0.9rem] border border-amber-200/70 bg-amber-50/85 px-3 py-2 text-xs text-amber-800">
          通知已关闭。订阅和模板仍可编辑，开启后立刻生效。
        </div>
      ) : null}

      <section className="space-y-4 rounded-[1.1rem] border border-white/75 bg-white/75 px-4 py-4">
        <header className="space-y-1">
          <p className="text-sm font-medium text-moon-800">企微机器人</p>
          <p className="text-xs leading-5 text-moon-400">
            仅支持企业微信机器人 Webhook；需要其他渠道请另行部署中转。
          </p>
        </header>

        <div className="space-y-1.5">
          <label className="text-xs font-medium uppercase tracking-[0.18em] text-moon-450">
            Webhook URL
          </label>
          <Input
            value={localUrl}
            onChange={(event) => setLocalUrl(event.target.value)}
            onBlur={commitUrl}
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                event.preventDefault();
                commitUrl();
              }
            }}
            placeholder="https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=..."
            disabled={saving}
            className={urlError ? "border-status-red/60" : ""}
          />
          {urlError ? (
            <p className="text-xs text-status-red">{urlError}</p>
          ) : null}
        </div>

        <div className="space-y-1.5">
          <label className="text-xs font-medium uppercase tracking-[0.18em] text-moon-450">
            @手机号
          </label>
          <MobileChipInput
            value={settings.mention_mobile_list}
            onChange={(next) =>
              onChange({ ...settings, mention_mobile_list: next })
            }
            onCommit={(next) =>
              onCommit({ ...settings, mention_mobile_list: next })
            }
            disabled={saving}
          />
          <p className="text-[11px] leading-4 text-moon-400">
            企微会把 @手机号 链接到对应成员；@all 为 @所有人。
          </p>
        </div>
      </section>
    </div>
  );
}
