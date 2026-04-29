import type {
  NotificationEventType,
  NotificationSeverity,
  NotificationSettings,
  NotificationSubscription,
} from "@/lib/types";

export type {
  NotificationEventType,
  NotificationSeverity,
  NotificationSettings,
  NotificationSubscription,
};

export const MOBILE_PATTERN = /^\d{11}$/;
export const MENTION_ALL = "@all";

// PlaceholderMeta describes a template token the user can insert via the
// "插入字段" dropdown. `display` is the `[...]` label shown in the textarea,
// `template` is the Go template fragment persisted server-side, and `sample`
// is the local preview value resolved from the event's sample_vars.
export interface PlaceholderMeta {
  display: string;
  template: string;
  sample: string;
  description?: string;
}

const GENERIC_PLACEHOLDERS: PlaceholderMeta[] = [
  { display: "【事件】", template: "{{ .Event }}", sample: "" },
  { display: "【严重级别】", template: "{{ .Severity }}", sample: "" },
  { display: "【时间】", template: "{{ .TriggeredAt }}", sample: "" },
];

interface EventPlaceholderSpec {
  display: string;
  template: string;
  sampleKey: string;
  description?: string;
}

const EVENT_PLACEHOLDERS: Record<string, EventPlaceholderSpec[]> = {
  account_expiring: [
    {
      display: "【账号】",
      template: "{{ .Vars.account_label }}",
      sampleKey: "account_label",
    },
    {
      display: "【过期时间】",
      template: "{{ .Vars.expires_at }}",
      sampleKey: "expires_at",
    },
  ],
  cpa_credential_error: [
    {
      display: "【账号】",
      template: "{{ .Vars.account_label }}",
      sampleKey: "account_label",
    },
    {
      display: "【失效原因】",
      template: "{{ .Vars.last_error }}",
      sampleKey: "last_error",
    },
  ],
  account_error: [
    {
      display: "【账号】",
      template: "{{ .Vars.account_label }}",
      sampleKey: "account_label",
    },
    {
      display: "【最近错误】",
      template: "{{ .Vars.last_error }}",
      sampleKey: "last_error",
    },
  ],
  cpa_service_error: [
    {
      display: "【服务】",
      template: "{{ .Vars.service_label }}",
      sampleKey: "service_label",
    },
    {
      display: "【最近错误】",
      template: "{{ .Vars.last_error }}",
      sampleKey: "last_error",
    },
  ],
  test: [
    {
      display: "【实例】",
      template: "{{ .Vars.instance_id }}",
      sampleKey: "instance_id",
    },
    {
      display: "【管理后台】",
      template: "{{ .Vars.admin_url }}",
      sampleKey: "admin_url",
    },
  ],
};

export const EVENT_TRIGGER_DESCRIPTION: Record<string, string> = {
  account_expiring: "账号订阅或非 Codex CPA 凭据在阈值内即将过期",
  cpa_credential_error: "CPA 登录态失效，需要用户重新登录",
  account_error: "账号健康检查连续失败",
  cpa_service_error: "内置 CPA 运行状态检查失败",
  test: "手动点击 Send Test 触发",
};

export function placeholdersForEvent(
  event: NotificationEventType,
): PlaceholderMeta[] {
  const sampleVars = event.sample_vars ?? {};
  const generic = GENERIC_PLACEHOLDERS.map((item) => {
    if (item.template === "{{ .Event }}") {
      return { ...item, sample: event.event };
    }
    if (item.template === "{{ .Severity }}") {
      return { ...item, sample: event.default_severity };
    }
    if (item.template === "{{ .TriggeredAt }}") {
      return { ...item, sample: sampleTimestamp() };
    }
    return item;
  });
  const perEvent = (EVENT_PLACEHOLDERS[event.event] ?? []).map((spec) => ({
    display: spec.display,
    template: spec.template,
    sample: String(sampleVars[spec.sampleKey] ?? ""),
    description: spec.description,
  }));
  return [...generic, ...perEvent];
}

function sampleTimestamp(): string {
  return new Date().toISOString().replace(/\.\d+Z$/, "Z");
}

// toStorage translates the user-facing `[...]` placeholders back to the Go
// template fragments the backend renders. Unknown `[...]` substrings pass
// through unchanged.
export function toStorage(
  display: string,
  placeholders: PlaceholderMeta[],
): string {
  let result = display;
  for (const item of placeholders) {
    if (item.display === item.template) {
      continue;
    }
    result = result.split(item.display).join(item.template);
  }
  return result;
}

// toDisplay is the inverse of toStorage. Unknown `{{ ... }}` fragments pass
// through unchanged (shown literally in the textarea).
export function toDisplay(
  storage: string,
  placeholders: PlaceholderMeta[],
): string {
  let result = storage;
  for (const item of placeholders) {
    if (item.display === item.template) {
      continue;
    }
    result = result.split(item.template).join(item.display);
  }
  return result;
}

// renderPreview substitutes the placeholder samples into the current display
// value to produce the live preview string. Unknown placeholders stay as-is.
export function renderPreview(
  display: string,
  placeholders: PlaceholderMeta[],
): string {
  let result = display;
  for (const item of placeholders) {
    result = result.split(item.display).join(item.sample);
  }
  return result;
}

export function severityTone(severity: NotificationSeverity): string {
  switch (severity) {
    case "critical":
      return "bg-status-red/12 text-status-red";
    case "warning":
      return "bg-status-yellow/16 text-status-yellow";
    default:
      return "bg-moon-100/90 text-moon-500";
  }
}
