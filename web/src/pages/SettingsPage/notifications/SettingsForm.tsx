import { Input } from "@/components/ui/input";

import MobileChipInput from "./MobileChipInput";
import type { NotificationSettings } from "./types";

type SettingsFormProps = {
  settings: NotificationSettings;
  // webhookUrlDraft is the user's current input — may differ from
  // settings.webhook_url when they haven't blurred yet. Lives in the parent
  // so the enable switch and the Send Test button can both consult it.
  webhookUrlDraft: string;
  onWebhookUrlDraftChange: (next: string) => void;
  saving: boolean;
  urlError?: string | null;
  onChange: (next: NotificationSettings) => void;
  onCommit: (next: NotificationSettings) => void;
};

export default function SettingsForm({
  settings,
  webhookUrlDraft,
  onWebhookUrlDraftChange,
  saving,
  urlError,
  onChange,
  onCommit,
}: SettingsFormProps) {
  function commitUrl() {
    const trimmed = webhookUrlDraft.trim();
    if (trimmed === settings.webhook_url) {
      return;
    }
    const next = { ...settings, webhook_url: trimmed };
    onChange(next);
    onCommit(next);
  }

  return (
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
          value={webhookUrlDraft}
          onChange={(event) => onWebhookUrlDraftChange(event.target.value)}
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
  );
}
