export interface Account {
  id: number;
  label: string;
  base_url: string;
  api_key_set: boolean;
  api_key_masked: string;
  enabled: boolean;
  status: "unknown" | "healthy" | "degraded" | "error" | "disabled";
  notes: string;
  quota_display: string;
  models: string[];
  last_checked_at: string | null;
  last_error: string | null;
  created_at: string;
  updated_at: string;
  source_kind: "openai_compat" | "cpa";
  provider: string;
  cpa_service_id: number | null;
  cpa_provider: string;
  cpa_account_key: string;
  cpa_email: string;
  cpa_plan_type: string;
  cpa_openai_id: string;
  cpa_expired_at: string | null;
  cpa_last_refresh_at: string | null;
  cpa_disabled: boolean;
  cpa_credential_status?: "unknown" | "ok" | "initializing" | "runtime_pending" | "runtime_error" | "needs_login";
  cpa_credential_reason?: string;
  cpa_credential_last_error?: string;
  cpa_credential_checked_at?: string | null;
  cpa_subscription_expires_at?: string | null;
  cpa_subscription_fetched_at?: string | null;
  cpa_subscription_last_error?: string;
  codex_quota_json?: string;
  codex_quota_fetched_at?: string;
  probe_models: string[];
  last_probe_status?: "" | "healthy" | "degraded" | "error";
  last_probe_at?: string | null;
  last_probe_error?: string;
  runtime: {
    base_url: string;
    auth_mode: string;
  } | null;
}

export interface Pool {
  id: number;
  label: string;
  priority: number;
  enabled: boolean;
  account_count: number;
  healthy_account_count: number;
  routable_account_count: number;
  models: string[];
  created_at: string;
  updated_at: string;
}

export interface PoolMember {
  id: number;
  pool_id: number;
  account_id: number;
  position: number;
  enabled: boolean;
  account?: Account;
}

export interface AccessToken {
  id: number;
  name: string;
  token?: string;
  token_masked: string;
  pool_id: number;
  pool_label?: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
  last_used_at: string | null;
}

export interface AccessTokenCreated {
  id: number;
  name: string;
  token: string;
  pool_id: number;
}

export interface RevealedAccessToken extends AccessToken {
  token: string;
}

export interface RequestLog {
  id: number;
  request_id: string;
  access_token_name: string;
  model_requested: string;
  model_actual: string;
  pool_id: number;
  account_id: number;
  account_label: string;
  status_code: number;
  latency_ms: number;
  input_tokens: number | null;
  output_tokens: number | null;
  stream: boolean;
  request_ip: string;
  success: boolean;
  error_message: string | null;
  source_kind: string;
  attempt_count: number;
  created_at: string;
}

export interface Overview {
  pools_total: number;
  pools_healthy: number;
  accounts_total: number;
  accounts_healthy: number;
  models_total: number;
  requests_today: number;
  success_rate_today: number;
  avg_latency_today: number;
  alerts: OverviewAlert[];
}

export interface OverviewAlert {
  type:
    | "expiring"
    | "error"
    | "account_expiring"
    | "account_error"
    | "cpa_credential_error"
    | "pool_unhealthy"
    | string;
  message: string;
  pool_id?: number;
}

export interface PoolDetailResponse {
  pool: Pool;
  members: PoolMember[];
  tokens: AccessToken[];
  models: string[];
  stats: UsageStats;
}

export interface UsageStats {
  total_requests: number;
  success_rate: number;
  total_input_tokens: number;
  total_output_tokens: number;
  by_account: Array<{
    account_id: number;
    account_label: string;
    requests: number;
    successful_requests: number;
    success_rate: number;
    input_tokens: number;
    output_tokens: number;
  }>;
  by_token: Array<{
    token_name: string;
    requests: number;
    input_tokens: number;
    output_tokens: number;
  }>;
}

export interface UsageLogPage {
  items: RequestLog[];
  total: number;
  page: number;
  page_size: number;
}

export interface SystemSettings {
  admin_token_masked: string;
  health_check_interval: number;
  request_timeout: number;
  max_retry_attempts: number;
  gateway_max_body_mb: number;
  gateway_memory_body_mb: number;
  notification_expiring_days: number;
  data_retention_days: number;
}

export type NotificationSeverity = "info" | "warning" | "critical";

export interface NotificationSettings {
  enabled: boolean;
  webhook_url: string;
  mention_mobile_list: string[];
  created_at?: string;
  updated_at?: string;
}

export interface NotificationSubscription {
  event: string;
  subscribed: boolean;
  body_template: string;
  updated_at?: string;
}

export interface NotificationEventType {
  event: string;
  label: string;
  default_severity: NotificationSeverity;
  default_body_template: string;
  sample_vars: Record<string, unknown>;
}

export interface NotificationDelivery {
  id: number;
  channel_id: number;
  channel_name: string;
  channel_type: string;
  event: string;
  severity: NotificationSeverity;
  title: string;
  payload_summary: string;
  status: "success" | "failed" | "dropped";
  upstream_code: string;
  upstream_message: string;
  latency_ms: number;
  attempt: number;
  dedup_key: string;
  triggered_by: "system" | "test";
  created_at: string;
}

export interface ConfigImportResult {
  created_pools: number;
  updated_pools: number;
  created_tokens: number;
  skipped_tokens: number;
  updated_settings: number;
}

export interface ConfigImportPreview {
  schema_version: string;
  source_host: string;
  exported_at: string;
  include_secrets: boolean;
  created_pools: number;
  updated_pools: number;
  created_tokens: number;
  skipped_tokens: number;
  updated_settings: number;
  ignored_accounts: number;
  ignored_services: number;
}

export interface SystemNotification {
  type: string;
  severity: "warning" | "critical";
  title: string;
  message: string;
  account_id?: number;
  service_id?: number;
  expires_at?: string;
}

export interface DataRetentionSummary {
  retention_days: number;
  database_size_bytes: number;
  total_logs: number;
  oldest_log_at: string | null;
  newest_log_at: string | null;
  logs_size_bytes: number;
  total_notification_deliveries: number;
  notification_deliveries_oldest_at: string | null;
  notification_deliveries_newest_at: string | null;
  total_notification_outbox: number;
  outbox_pending_count: number;
  outbox_dropped_count: number;
  last_prune_at: string | null;
  last_prune_deleted_logs: number;
  last_prune_deleted_deliveries: number;
  last_prune_deleted_outbox: number;
}

export interface DataRetentionPreview {
  retention_days: number;
  logs_to_delete: number;
  logs_to_delete_size_bytes: number;
  deliveries_to_delete: number;
  outbox_to_delete: number;
  outbox_safety_days: number;
}

export interface CpaService {
  id: number;
  label: string;
  base_url: string;
  api_key_set: boolean;
  api_key_masked: string;
  enabled: boolean;
  status: "unknown" | "healthy" | "error";
  last_checked_at: string | null;
  last_error: string;
  created_at: string;
  updated_at: string;
  runtime_mode?: string;
  auth_dir?: string;
  current_version?: string;
  latest_version?: string;
  update_available?: boolean;
}

export interface CpaServiceTestResult {
  reachable: boolean;
  latency_ms: number;
  providers: string[];
  error: string;
}

export interface LoginSession {
  id: string;
  service_id?: number;
  pool_id?: number;
  provider?: string;
  status:
    | "pending"
    | "authorized"
    | "succeeded"
    | "expired"
    | "failed"
    | "cancelled";
  verification_uri?: string;
  user_code?: string;
  expires_at?: string;
  poll_interval_seconds?: number;
  account_id?: number;
  account?: Account;
  error_code?: string;
  error_message?: string;
}

export interface RemoteAccount {
  account_key: string;
  email: string;
  plan_type: string;
  provider: string;
  account_id: string;
  expired_at: string | null;
  disabled: boolean;
  already_imported: boolean;
}

export interface LatencyBucket {
  bucket: string;
  p50: number;
  p95: number;
  p99: number;
  count: number;
}

export interface TestConnectionResult {
  reachable: boolean;
  latency_ms: number;
  models: string[];
  error: string;
}

export interface BatchImportResult {
  imported: number;
  skipped: number;
  errors: string[];
}
