export interface Account {
  id: number;
  label: string;
  base_url: string;
  api_key_set: boolean;
  api_key_masked: string;
  enabled: boolean;
  status: "healthy" | "degraded" | "error" | "disabled";
  quota_total: number;
  quota_used: number;
  quota_unit: string;
  notes: string;
  model_allowlist: string[];
  last_checked_at: string | null;
  last_error: string | null;
  created_at: string;
  updated_at: string;
}

export interface Pool {
  id: number;
  label: string;
  strategy: string;
  enabled: boolean;
  members: PoolMember[];
  created_at: string;
  updated_at: string;
}

export interface PoolMember {
  id: number;
  account_id: number;
  account_label: string;
  account_status: string;
  priority: number;
  weight: number;
}

export interface ModelRoute {
  id: number;
  alias: string;
  pool_id: number;
  pool_label: string;
  target_model: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface AccessToken {
  id: number;
  name: string;
  token_masked: string;
  enabled: boolean;
  quota_tokens: number;
  used_tokens: number;
  created_at: string;
  updated_at: string;
  last_used_at: string | null;
}

export interface AccessTokenCreated {
  id: number;
  name: string;
  token: string;
  quota_tokens: number;
}

export interface RequestLog {
  id: number;
  request_id: string;
  access_token_name: string;
  model_alias: string;
  target_model: string;
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
  created_at: string;
}

export interface Overview {
  total_accounts: number;
  healthy_accounts: number;
  total_pools: number;
  total_tokens: number;
  requests_24h: number;
  success_rate_24h: number;
  token_usage_24h: {
    input: number;
    output: number;
  };
  account_health: Array<{
    id: number;
    label: string;
    status: string;
    last_checked_at: string | null;
    last_error: string | null;
  }>;
  recent_requests: RequestLog[];
}

export interface UsageStats {
  total_requests: number;
  total_input_tokens: number;
  total_output_tokens: number;
  by_account: Array<{
    account_id: number;
    account_label: string;
    requests: number;
    input_tokens: number;
    output_tokens: number;
  }>;
  by_token: Array<{
    token_name: string;
    requests: number;
    input_tokens: number;
    output_tokens: number;
  }>;
  logs: {
    items: RequestLog[];
    total: number;
    page: number;
    page_size: number;
  };
}

export interface SystemSettings {
  admin_token_masked: string;
  default_pool_id: number | null;
  health_check_interval: number;
  request_timeout: number;
  max_retry_attempts: number;
}
