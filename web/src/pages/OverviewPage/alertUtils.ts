import type { OverviewAlert } from "@/lib/types";

export const DISMISS_STORAGE_KEY = "lune.overview.alertDismissals.v1";

export type AlertKind =
  | "account_expiring"
  | "account_error"
  | "pool_unhealthy"
  | "other";

export type ParsedAlert = {
  label: string;
  detail: string;
  kind: AlertKind;
};

// Backend messages are hardcoded English. Parse label/detail out so the UI can speak Chinese.
export function parseAlert(alert: OverviewAlert): ParsedAlert {
  const msg = alert.message || "";
  const kind: AlertKind =
    alert.type === "account_expiring" || alert.type === "expiring"
      ? "account_expiring"
      : alert.type === "account_error" || alert.type === "error"
      ? "account_error"
      : alert.type === "pool_unhealthy"
      ? "pool_unhealthy"
      : "other";

  const m = msg.match(/^(?:Account|Pool)\s+"([^"]+)"\s*(.*)$/);
  if (m) {
    const label = m[1];
    let detail = m[2].trim();
    if (kind === "account_expiring") {
      const at = detail.match(/^expires at\s+(.+)$/i);
      detail = at ? at[1].trim() : detail;
    } else if (kind === "account_error") {
      const colon = detail.match(/^has error status(?::\s*(.*))?$/i);
      detail = colon && colon[1] ? colon[1].trim() : "";
    } else {
      detail = "";
    }
    return { label, detail, kind };
  }
  return { label: "", detail: msg, kind };
}

export function formatCn(alert: OverviewAlert, parsed: ParsedAlert): string {
  const label = parsed.label ? `「${parsed.label}」` : "";
  switch (parsed.kind) {
    case "account_expiring":
      return `账号${label}将于 ${parsed.detail} 到期`;
    case "account_error":
      return parsed.detail
        ? `账号${label}状态异常：${parsed.detail}`
        : `账号${label}状态异常`;
    case "pool_unhealthy":
      return `Pool${label}当前没有可路由的账号`;
    default:
      return alert.message;
  }
}

export function fingerprint(alert: OverviewAlert, parsed: ParsedAlert): string {
  return [alert.type, alert.pool_id ?? 0, parsed.label, parsed.detail].join("|");
}

export function loadDismissed(): Set<string> {
  try {
    const raw = localStorage.getItem(DISMISS_STORAGE_KEY);
    if (!raw) return new Set();
    const arr = JSON.parse(raw);
    return Array.isArray(arr) ? new Set(arr) : new Set();
  } catch {
    return new Set();
  }
}

export function saveDismissed(set: Set<string>) {
  try {
    localStorage.setItem(DISMISS_STORAGE_KEY, JSON.stringify([...set]));
  } catch {
    /* ignore storage errors */
  }
}

// Only account_expiring is silenceable. Critical types always show.
export function filterVisible(
  alerts: OverviewAlert[],
  dismissed: Set<string>,
): { alert: OverviewAlert; parsed: ParsedAlert }[] {
  return alerts
    .map((alert) => ({ alert, parsed: parseAlert(alert) }))
    .filter(({ alert, parsed }) => {
      if (parsed.kind !== "account_expiring") return true;
      return !dismissed.has(fingerprint(alert, parsed));
    });
}
