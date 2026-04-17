import type {
  ChannelType,
  NotificationChannel,
  NotificationDeliveryMeta,
  NotificationEventType,
  NotificationSeverity,
  NotificationSubscription,
} from "@/lib/types";

export type {
  ChannelType,
  NotificationEventType,
  NotificationSeverity,
  NotificationSubscription,
};

export type NotificationChannelField = {
  key: string;
  label: string;
  placeholder?: string;
  helper?: string;
  secret?: boolean;
  multiline?: boolean;
};

export type NotificationChannelDraft = {
  id: number;
  name: string;
  type: ChannelType;
  enabled: boolean;
  config: Record<string, string>;
  preservedSecrets: Record<string, boolean>;
  subscriptions: NotificationSubscription[];
  title_template: string;
  body_template: string;
  retry_max_attempts: number;
  retry_schedule_seconds: number[];
  created_at: string;
  updated_at: string;
  last_delivery?: NotificationDeliveryMeta | null;
  recent_deliveries?: NotificationDeliveryMeta[];
};

export const DEFAULT_SUBSCRIPTION: NotificationSubscription = {
  event: "*",
  min_severity: "info",
  title_template: "",
  body_template: "",
};

export const DEFAULT_RETRY_SCHEDULE = [30, 120, 600, 1800, 7200];
export const SECRET_PLACEHOLDER = "***";
export const SEVERITY_OPTIONS: NotificationSeverity[] = [
  "info",
  "warning",
  "critical",
];

export const CHANNEL_TYPE_META: Record<
  ChannelType,
  {
    label: string;
    tone: string;
    description: string;
    fields: NotificationChannelField[];
    defaults: Record<string, string>;
  }
> = {
  generic_webhook: {
    label: "Generic Webhook",
    tone: "bg-moon-100/85 text-moon-600",
    description: "发送标准 JSON 负载，适合自建接收器或自动化流程。",
    fields: [
      {
        key: "url",
        label: "Webhook URL",
        placeholder: "https://example.com/webhook",
      },
      {
        key: "headers_json",
        label: "Headers JSON",
        placeholder: '{"Authorization":"Bearer ..."}',
        helper: "可选，必须是字符串值的 JSON 对象。",
        multiline: true,
      },
    ],
    defaults: { url: "", headers_json: "" },
  },
  wechat_work_bot: {
    label: "企微告警",
    tone: "bg-emerald-100/85 text-emerald-700",
    description: "直接发到企业微信机器人，适合团队群告警。",
    fields: [
      {
        key: "webhook_url",
        label: "Webhook URL",
        placeholder: "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=...",
      },
      {
        key: "format",
        label: "Format",
        helper: "支持 text 或 markdown。",
      },
      {
        key: "mention_list",
        label: "Mention List",
        placeholder: "user1,user2",
      },
      {
        key: "mention_mobile_list",
        label: "Mention Mobiles",
        placeholder: "13800000000",
      },
    ],
    defaults: {
      webhook_url: "",
      format: "markdown",
      mention_list: "",
      mention_mobile_list: "",
    },
  },
  feishu_bot: {
    label: "飞书机器人",
    tone: "bg-sky-100/90 text-sky-700",
    description: "支持 text 或 post，填写 secret 后自动完成签名。",
    fields: [
      {
        key: "webhook_url",
        label: "Webhook URL",
        placeholder: "https://open.feishu.cn/open-apis/bot/v2/hook/...",
      },
      {
        key: "secret",
        label: "签名密钥",
        helper: "可选，启用 HMAC 签名。",
        secret: true,
      },
      {
        key: "format",
        label: "Format",
        helper: "支持 text 或 post。",
      },
    ],
    defaults: { webhook_url: "", secret: "", format: "post" },
  },
  email_smtp: {
    label: "邮箱通知",
    tone: "bg-amber-100/90 text-amber-700",
    description: "通过 SMTP 投递邮件，适合个人 inbox 或团队邮箱。",
    fields: [
      { key: "host", label: "Host", placeholder: "smtp.example.com" },
      { key: "port", label: "Port", placeholder: "587" },
      { key: "username", label: "Username" },
      { key: "password", label: "Password", secret: true },
      { key: "from", label: "From", placeholder: "lune@example.com" },
      {
        key: "to_csv",
        label: "Recipients",
        placeholder: "ops@example.com,me@example.com",
      },
      {
        key: "tls_mode",
        label: "TLS Mode",
        helper: "starttls / tls / none",
      },
    ],
    defaults: {
      host: "",
      port: "587",
      username: "",
      password: "",
      from: "",
      to_csv: "",
      tls_mode: "starttls",
    },
  },
};

export function makeChannelDraft(
  channel: NotificationChannel,
): NotificationChannelDraft {
  return {
    ...channel,
    subscriptions:
      channel.subscriptions.length > 0
        ? channel.subscriptions
        : [DEFAULT_SUBSCRIPTION],
    config: decodeChannelConfig(channel.type, channel.config),
    preservedSecrets: decodePreservedSecrets(channel.type, channel.config),
    retry_max_attempts: channel.retry_max_attempts || 5,
    retry_schedule_seconds:
      channel.retry_schedule_seconds?.length > 0
        ? channel.retry_schedule_seconds
        : DEFAULT_RETRY_SCHEDULE,
  };
}

export function buildChannelPayload(draft: NotificationChannelDraft) {
  return {
    name: draft.name.trim(),
    type: draft.type,
    enabled: draft.enabled,
    config: buildChannelConfig(draft),
    subscriptions:
      draft.subscriptions.filter((item) => item.event.trim()).length > 0
        ? draft.subscriptions.filter((item) => item.event.trim())
        : [DEFAULT_SUBSCRIPTION],
    title_template: draft.title_template.trim(),
    body_template: draft.body_template.trim(),
    retry_max_attempts: draft.retry_max_attempts,
    retry_schedule_seconds: draft.retry_schedule_seconds,
  };
}

export function defaultChannelName(
  type: ChannelType,
  existing: NotificationChannel[],
) {
  const label = CHANNEL_TYPE_META[type].label;
  const names = new Set(existing.map((item) => item.name));
  if (!names.has(label)) {
    return label;
  }
  let index = 2;
  while (names.has(`${label} ${index}`)) {
    index += 1;
  }
  return `${label} ${index}`;
}

export function eventLabel(
  eventTypes: NotificationEventType[],
  event: string,
) {
  if (event === "*") {
    return "全部事件";
  }
  return (
    eventTypes.find((item) => item.event === event)?.label ?? event
  );
}

export function formatDeliverySummary(delivery?: NotificationDeliveryMeta | null) {
  if (!delivery) {
    return "尚无投递记录";
  }
  const mark =
    delivery.status === "success"
      ? "✓"
      : delivery.status === "failed"
        ? "✗"
        : "•";
  const code = delivery.upstream_code ? ` ${delivery.upstream_code}` : "";
  return `${mark}${code}`;
}

export function deliveryTone(delivery?: NotificationDeliveryMeta | null) {
  if (!delivery) {
    return "bg-moon-300";
  }
  switch (delivery.status) {
    case "success":
      return "bg-status-green";
    case "failed":
      return "bg-status-red";
    default:
      return "bg-status-yellow";
  }
}

export function parseRetryInput(value: string) {
  return value
    .split(",")
    .map((item) => Number(item.trim()))
    .filter((item) => Number.isFinite(item) && item > 0);
}

export function retryInputValue(values: number[]) {
  return values.join(", ");
}

function decodeChannelConfig(
  type: ChannelType,
  config: Record<string, unknown>,
) {
  const meta = CHANNEL_TYPE_META[type];
  const next = { ...meta.defaults };
  for (const field of meta.fields) {
    if (field.key === "headers_json") {
      const headers = config.headers;
      next.headers_json =
        headers && typeof headers === "object"
          ? JSON.stringify(headers, null, 2)
          : "";
      continue;
    }
    if (field.key === "to_csv") {
      next.to_csv = Array.isArray(config.to)
        ? config.to.map((item) => String(item)).join(", ")
        : "";
      continue;
    }
    const raw = config[field.key];
    next[field.key] =
      raw == null ? meta.defaults[field.key] ?? "" : String(raw);
  }
  return next;
}

function decodePreservedSecrets(
  type: ChannelType,
  config: Record<string, unknown>,
) {
  const meta = CHANNEL_TYPE_META[type];
  const next: Record<string, boolean> = {};
  for (const field of meta.fields) {
    if (field.secret && config[field.key] === SECRET_PLACEHOLDER) {
      next[field.key] = true;
    }
  }
  return next;
}

function buildChannelConfig(draft: NotificationChannelDraft) {
  const value = draft.config;
  const preserveSecret = (key: string, current: string) =>
    draft.preservedSecrets[key] && current.trim() === ""
      ? SECRET_PLACEHOLDER
      : current;

  switch (draft.type) {
    case "generic_webhook": {
      let headers: Record<string, string> | undefined;
      if (value.headers_json.trim()) {
        let parsed: unknown;
        try {
          parsed = JSON.parse(value.headers_json);
        } catch {
          throw new Error("Headers JSON 必须是合法 JSON 对象");
        }
        if (
          !parsed ||
          Array.isArray(parsed) ||
          typeof parsed !== "object" ||
          Object.values(parsed as Record<string, unknown>).some(
            (item) => typeof item !== "string",
          )
        ) {
          throw new Error(
            'Headers JSON 必须是 { "Header": "value" } 形式的字符串对象',
          );
        }
        headers = parsed as Record<string, string>;
      }
      return {
        schema: 1,
        url: value.url.trim(),
        ...(headers ? { headers } : {}),
      };
    }
    case "wechat_work_bot":
      return {
        schema: 1,
        webhook_url: value.webhook_url.trim(),
        format: value.format.trim() || "markdown",
        mention_list: splitCsv(value.mention_list),
        mention_mobile_list: splitCsv(value.mention_mobile_list),
      };
    case "feishu_bot":
      return {
        schema: 1,
        webhook_url: value.webhook_url.trim(),
        secret: preserveSecret("secret", value.secret),
        format: value.format.trim() || "post",
      };
    case "email_smtp":
      return {
        schema: 1,
        host: value.host.trim(),
        port: Number(value.port || "0"),
        username: value.username.trim(),
        password: preserveSecret("password", value.password),
        from: value.from.trim(),
        to: splitCsv(value.to_csv),
        tls_mode: value.tls_mode.trim() || "starttls",
      };
  }
}

function splitCsv(value: string) {
  return value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}
