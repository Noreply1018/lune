# Lune v1 — Part 1: Backend Design

> Go backend specification. Single binary, SQLite persistence, transparent proxy gateway.

## 1. Architecture Overview

```
                    ┌──────────────────────────────────────────┐
                    │                Lune Binary                │
                    │                                          │
  Browser ────────► │  /admin/*     ──► Admin API Handler      │
                    │  /admin       ──► Embedded SPA (site)    │
                    │                                          │
  LLM Client ────► │  /v1/*        ──► Gateway Proxy          │──► Upstream Provider
                    │  /openai/v1/* ──► (same handler)         │
                    │                                          │
                    │  /healthz     ──► Health Endpoint        │
                    │  /readyz      ──► Readiness Endpoint     │
                    │                                          │
                    │  ┌──────────────────────────────┐        │
                    │  │   SQLite (source of truth)   │        │
                    │  │   + in-memory routing cache   │        │
                    │  └──────────────────────────────┘        │
                    │                                          │
                    │  ┌──────────────────────────────┐        │
                    │  │  Health Checker (goroutine)   │        │
                    │  └──────────────────────────────┘        │
                    └──────────────────────────────────────────┘
```

## 2. Domain Model

### 2.1 Account

One upstream OpenAI-compatible credential target.

| Field | Type | Description |
|---|---|---|
| `id` | INTEGER PK | auto-increment |
| `label` | TEXT NOT NULL | human-readable name |
| `base_url` | TEXT NOT NULL | upstream base URL incl. version path (e.g. `https://api.openai.com/v1`) |
| `api_key` | TEXT NOT NULL | upstream API key (stored plaintext, see rationale below) |
| `enabled` | BOOLEAN | whether routing may use this account |
| `status` | TEXT | runtime health: `healthy`, `degraded`, `error`, `disabled` |
| `quota_total` | REAL | manually maintained total quota |
| `quota_used` | REAL | manually maintained used quota |
| `quota_unit` | TEXT | display unit (e.g. "USD", "CNY") |
| `notes` | TEXT | freeform note |
| `model_allowlist` | TEXT | JSON array of allowed model names; empty = all allowed |
| `last_checked_at` | DATETIME | last health check time |
| `last_error` | TEXT | last error summary |
| `created_at` | DATETIME | |
| `updated_at` | DATETIME | |

**API key plaintext rationale**: Lune is a personal local-only tool. The SQLite file has the same trust boundary as the user's filesystem. Encryption would require a master key that adds complexity without meaningful security benefit.

**model_allowlist semantics**: when non-empty, the router only selects this account if the target model is in the list. When empty, all models are allowed.

**Quota fields**: administrative metadata. Displayed in UI, may trigger soft warnings, not hard-stop routing constraints.

### 2.2 Pool

A routing group of accounts.

**pools table:**

| Field | Type | Description |
|---|---|---|
| `id` | INTEGER PK | auto-increment |
| `label` | TEXT NOT NULL | |
| `strategy` | TEXT NOT NULL | v1: always `priority-first-healthy` |
| `enabled` | BOOLEAN | |
| `created_at` | DATETIME | |
| `updated_at` | DATETIME | |

**pool_members table:**

| Field | Type | Description |
|---|---|---|
| `id` | INTEGER PK | auto-increment |
| `pool_id` | INTEGER FK | references pools(id) |
| `account_id` | INTEGER FK | references accounts(id) |
| `priority` | INTEGER NOT NULL | lower = tried first |
| `weight` | INTEGER NOT NULL DEFAULT 1 | for weighted selection among same priority |
| UNIQUE | | (pool_id, account_id) |

**Routing strategy: `priority-first-healthy`**

1. sort members by ascending priority
2. at the lowest priority level, filter to accounts that are `enabled` AND (`healthy` or `degraded`)
3. if model_allowlist is non-empty, filter to accounts that allow the target model
4. among remaining candidates at this priority, select one by weighted random
5. if no candidate at this priority, move to next priority level
6. if no candidate at any priority, return error

### 2.3 ModelRoute

Maps client-facing model alias to a pool and upstream model name.

| Field | Type | Description |
|---|---|---|
| `id` | INTEGER PK | auto-increment |
| `alias` | TEXT NOT NULL UNIQUE | model name the client sends |
| `pool_id` | INTEGER FK | references pools(id) |
| `target_model` | TEXT NOT NULL | model name sent to upstream |
| `enabled` | BOOLEAN | |
| `created_at` | DATETIME | |
| `updated_at` | DATETIME | |

**Default pool (catch-all)**:

A system setting `default_pool_id` in `system_config`. When no ModelRoute matches the requested model:

- if `default_pool_id` is set and the referenced pool is enabled → route through it with the original model name (no rewriting)
- otherwise → return 404 "no route for model"

### 2.4 AccessToken

Client credential for calling the gateway.

| Field | Type | Description |
|---|---|---|
| `id` | INTEGER PK | auto-increment |
| `name` | TEXT NOT NULL | human-readable label |
| `token` | TEXT NOT NULL UNIQUE | Bearer token value (e.g. `sk-lune-xxxx`) |
| `enabled` | BOOLEAN | |
| `quota_tokens` | INTEGER DEFAULT 0 | total allowed token usage; 0 = unlimited |
| `used_tokens` | INTEGER DEFAULT 0 | accumulated input+output tokens |
| `created_at` | DATETIME | |
| `updated_at` | DATETIME | |
| `last_used_at` | DATETIME | |

**Quota enforcement**:

- if `quota_tokens > 0` AND `used_tokens >= quota_tokens` → reject with 429
- if `quota_tokens = 0` → unlimited
- usage accumulated from upstream response `usage.prompt_tokens + usage.completion_tokens`
- if upstream doesn't return usage → request proceeds, usage recorded as 0

### 2.5 RequestLog

| Field | Type | Description |
|---|---|---|
| `id` | INTEGER PK | auto-increment |
| `request_id` | TEXT NOT NULL | unique per-request identifier |
| `access_token_name` | TEXT | which client token was used |
| `model_alias` | TEXT | model name from client |
| `target_model` | TEXT | model name sent to upstream |
| `pool_id` | INTEGER | |
| `account_id` | INTEGER | |
| `status_code` | INTEGER | upstream response status |
| `latency_ms` | INTEGER | |
| `input_tokens` | INTEGER | prompt_tokens from upstream (nullable) |
| `output_tokens` | INTEGER | completion_tokens from upstream (nullable) |
| `stream` | BOOLEAN | whether streaming request |
| `request_ip` | TEXT | client IP |
| `success` | BOOLEAN | |
| `error_message` | TEXT | |
| `created_at` | DATETIME | |

Index: `idx_request_logs_created_at ON request_logs(created_at)`

### 2.6 SystemConfig

Key-value store for system settings.

| Field | Type | Description |
|---|---|---|
| `key` | TEXT PK | setting name |
| `value` | TEXT | setting value (JSON-encoded if complex) |

Known keys:

- `admin_token` — admin API token
- `default_pool_id` — catch-all pool ID
- `health_check_interval` — seconds between health checks (default 60)
- `request_timeout` — upstream request timeout in seconds (default 120)
- `max_retry_attempts` — per-request retry limit (default 3)

## 3. Persistence

### 3.1 SQLite

Single file: `{data_dir}/lune.db`

Created automatically on first startup. Schema migrations run at startup before the HTTP server starts.

Migration strategy: version-stamped SQL statements. A `schema_version` key in `system_config` tracks the current version. Each migration is idempotent.

### 3.2 Bootstrap config

Only two settings need to be known before the database is available:

| Setting | Env Var | Default |
|---|---|---|
| HTTP port | `LUNE_PORT` | 7788 |
| Data directory | `LUNE_DATA_DIR` | `./data` |

Optional file: `lune.yaml` in the working directory with the same two fields. Env vars take precedence.

All other configuration lives in SQLite and is managed via admin API.

### 3.3 Config cache

The gateway needs fast access to routing data (accounts, pools, routes). Reading SQLite on every request is unnecessary.

Design:

```go
type RoutingCache struct {
    mu       sync.RWMutex
    version  uint64
    accounts []Account
    pools    []PoolWithMembers
    routes   []ModelRoute
    settings SystemSettings
}
```

- admin API writes call `cache.Invalidate()` (bumps version)
- gateway reads call `cache.Get()` which checks version; if stale, refreshes from SQLite under write lock
- health checker updates also bump version
- lock contention is minimal: reads are concurrent, writes are infrequent

## 4. CLI

### 4.1 `lune up`

Primary command. Starts the HTTP server.

Startup sequence:

1. parse bootstrap config (env vars > yaml > defaults)
2. ensure data directory exists
3. open SQLite, run migrations
4. resolve admin token: `LUNE_ADMIN_TOKEN` env > existing DB value > auto-generate
5. if auto-generated, store in DB and print to terminal
6. initialize routing cache from DB
7. start health checker goroutine
8. start HTTP server
9. print startup banner:
   ```
   Lune v1.0.0 is running

     Admin UI:    http://127.0.0.1:7788/admin
     Gateway API: http://127.0.0.1:7788/v1
     Admin Token: lune-xxxx (auto-generated)

   Press Ctrl+C to stop
   ```
10. trap SIGINT/SIGTERM for graceful shutdown

If the port is occupied → fail with: `Error: port 7788 is already in use`

### 4.2 `lune version`

Print version and exit:

```
lune v1.0.0 (commit abc1234, built 2026-04-12)
```

### 4.3 `lune check`

Validate without starting:

1. check data directory is writable
2. open SQLite, verify schema version
3. count accounts, pools, routes, tokens
4. print summary and exit

```
Lune check passed

  Database: data/lune.db (schema v3)
  Accounts: 5 (3 healthy, 1 error, 1 disabled)
  Pools:    2
  Routes:   8
  Tokens:   3
```

## 5. HTTP Server

### 5.1 Route table

| Method | Path | Auth | Handler |
|---|---|---|---|
| GET | `/healthz` | none | liveness: always 200 |
| GET | `/readyz` | none | readiness: 200 if DB accessible + ≥1 account |
| GET | `/v1/models` | none | list enabled model aliases |
| ANY | `/v1/*` | Bearer access token | gateway transparent proxy |
| ANY | `/openai/v1/*` | Bearer access token | alias → same gateway handler |
| GET | `/admin/api/overview` | localhost or Bearer admin | overview stats |
| GET | `/admin/api/accounts` | localhost or Bearer admin | list accounts |
| POST | `/admin/api/accounts` | localhost or Bearer admin | create account |
| PUT | `/admin/api/accounts/{id}` | localhost or Bearer admin | update account |
| POST | `/admin/api/accounts/{id}/enable` | localhost or Bearer admin | enable account |
| POST | `/admin/api/accounts/{id}/disable` | localhost or Bearer admin | disable account |
| DELETE | `/admin/api/accounts/{id}` | localhost or Bearer admin | delete account |
| GET | `/admin/api/pools` | localhost or Bearer admin | list pools |
| POST | `/admin/api/pools` | localhost or Bearer admin | create pool |
| PUT | `/admin/api/pools/{id}` | localhost or Bearer admin | update pool |
| POST | `/admin/api/pools/{id}/enable` | localhost or Bearer admin | enable pool |
| POST | `/admin/api/pools/{id}/disable` | localhost or Bearer admin | disable pool |
| DELETE | `/admin/api/pools/{id}` | localhost or Bearer admin | delete pool |
| GET | `/admin/api/routes` | localhost or Bearer admin | list routes |
| POST | `/admin/api/routes` | localhost or Bearer admin | create route |
| PUT | `/admin/api/routes/{id}` | localhost or Bearer admin | update route |
| DELETE | `/admin/api/routes/{id}` | localhost or Bearer admin | delete route |
| GET | `/admin/api/tokens` | localhost or Bearer admin | list tokens |
| POST | `/admin/api/tokens` | localhost or Bearer admin | create token |
| PUT | `/admin/api/tokens/{id}` | localhost or Bearer admin | update token |
| POST | `/admin/api/tokens/{id}/enable` | localhost or Bearer admin | enable token |
| POST | `/admin/api/tokens/{id}/disable` | localhost or Bearer admin | disable token |
| DELETE | `/admin/api/tokens/{id}` | localhost or Bearer admin | delete token |
| GET | `/admin/api/usage` | localhost or Bearer admin | usage stats |
| GET | `/admin/api/settings` | localhost or Bearer admin | system settings |
| PUT | `/admin/api/settings` | localhost or Bearer admin | update settings |
| GET | `/admin/api/export` | localhost or Bearer admin | full config export |
| GET | `/admin`, `/admin/*` | none | serve embedded SPA |

### 5.2 Admin auth middleware

```go
func AdminAuth(next http.Handler, store Store) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // localhost is always trusted
        if isLocalhost(r) {
            next.ServeHTTP(w, r)
            return
        }
        // non-localhost: require Bearer admin token
        token := extractBearerToken(r)
        if token == "" || token != store.GetAdminToken() {
            http.Error(w, "unauthorized", 401)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

`isLocalhost` checks `r.RemoteAddr` against `127.0.0.1` and `::1`. Must handle the case where a reverse proxy sets `X-Forwarded-For` — but for v1, Lune is accessed directly, so raw `RemoteAddr` is sufficient.

### 5.3 Gateway auth middleware

```go
func GatewayAuth(next http.Handler, cache *RoutingCache) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        token := extractBearerToken(r)
        accessToken := cache.FindAccessToken(token)
        if accessToken == nil {
            writeGatewayError(w, 401, "invalid_token", "invalid access token")
            return
        }
        if !accessToken.Enabled {
            writeGatewayError(w, 403, "token_disabled", "access token is disabled")
            return
        }
        if accessToken.QuotaTokens > 0 && accessToken.UsedTokens >= accessToken.QuotaTokens {
            writeGatewayError(w, 429, "quota_exhausted", "token quota exhausted")
            return
        }
        ctx := context.WithValue(r.Context(), accessTokenKey, accessToken)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

## 6. Gateway Proxy

### 6.1 Transparent proxy design

The gateway does not whitelist specific endpoints. Any request to `/v1/*` is forwarded to the selected upstream account.

```
Client: POST /v1/chat/completions
        POST /v1/embeddings
        POST /v1/images/generations
        POST /v1/responses
        GET  /v1/models  (handled locally, not proxied)
        ...any future /v1/* endpoint

Lune:   strips /v1/ prefix → path_suffix = "chat/completions"
        forwards to: {account.base_url}/{path_suffix}
        e.g.: https://api.openai.com/v1/chat/completions
```

### 6.2 Request flow

```
1. Extract Bearer token → validate (middleware)
2. Read request body
3. Parse JSON, extract "model" field
4. Resolve route:
   a. Look up ModelRoute by alias
   b. If found → use route's pool_id and target_model
   c. If not found → use default_pool with original model name
   d. If no default pool → 404
5. Get pool from cache
   a. If pool disabled → 503
6. Select account from pool (priority-first-healthy)
   a. If no usable account → 503
7. Rewrite "model" field in body if target_model differs from alias
8. Build upstream request:
   - URL: {account.base_url}/{path_suffix}
   - Method: same as client
   - Headers: copy client headers, replace Authorization with account's api_key
   - Body: (possibly rewritten) client body
9. Send upstream request
10. If error → try next account (up to max_retry_attempts)
11. Stream response back to client
12. After response completes:
    - Parse usage from response (if available)
    - Record request log
    - Accumulate token usage on access token
```

### 6.3 Body rewriting

Only the `model` field is rewritten. The rest of the body is passed through unchanged.

Implementation: if target_model == alias, no rewrite needed (common case). Otherwise, use efficient JSON patching — find the `"model"` key and replace its value, without parsing the entire body into a Go struct.

### 6.4 Streaming

Streaming is a v1 requirement.

For streaming requests (`"stream": true` in body):

- set upstream request timeout to time-to-first-byte only
- once first byte arrives, keep connection open
- flush each chunk to client immediately (no buffering)
- parse the final chunk for `usage` data (if present)
- handle SSE format: `data: {json}\n\n` lines

For non-streaming requests:

- read full upstream response
- parse `usage` from response body
- forward to client

### 6.5 Usage parsing

After a response completes, extract token usage:

**Non-streaming**: parse response JSON, read `usage.prompt_tokens` and `usage.completion_tokens`.

**Streaming**: the last `data:` line before `data: [DONE]` may contain a `usage` field (OpenAI includes this when `stream_options.include_usage` is true). Capture it if present.

If usage is not available (upstream doesn't return it, or parsing fails), record 0 and proceed.

### 6.6 Header management

Upstream request headers:

- copy most client headers
- replace `Authorization` with `Bearer {account.api_key}`
- replace `Host` with upstream host
- remove hop-by-hop headers (`Connection`, `Keep-Alive`, etc.)

Client response headers:

- copy upstream response headers
- add `X-Lune-Request-Id: {request_id}` for traceability
- add `X-Lune-Account: {account_id}` (optional, for debugging)

### 6.7 Retry logic

On retryable failure, select the next account from the pool and retry.

| Upstream Response | Retryable? |
|---|---|
| HTTP 5xx | yes |
| HTTP 429 | yes |
| Connection timeout | yes |
| Connection refused | yes |
| DNS error | yes |
| HTTP 4xx (except 429) | no — return to client |
| HTTP 2xx | no — success |

Max retry attempts: configurable via `system_config`, default 3.

Each failed attempt:

- logs the failure
- updates account health status

If all attempts fail, return the last error to the client with HTTP 502.

## 7. Health Checker

### 7.1 Design

A single background goroutine, started after the HTTP server is listening.

```go
func (hc *HealthChecker) Run(ctx context.Context) {
    ticker := time.NewTicker(hc.interval)
    defer ticker.Stop()

    hc.checkAll(ctx)  // immediate first check
    for {
        select {
        case <-ticker.C:
            hc.checkAll(ctx)
        case <-ctx.Done():
            return
        }
    }
}
```

### 7.2 Per-account check

For each enabled account:

1. send `GET {base_url}/models` with `Authorization: Bearer {api_key}`
2. timeout: 10 seconds
3. classify:
   - HTTP 200, latency ≤ 5s → `healthy`
   - HTTP 200, latency > 5s → `degraded`
   - any error / non-200 / timeout → `error`
4. update DB: `status`, `last_checked_at`, `last_error`
5. bump routing cache version

Disabled accounts: always `disabled`, never health-checked.

### 7.3 Lazy updates from gateway

In addition to periodic checks, the gateway updates health on every request:

- successful upstream response → mark account `healthy` (if currently `degraded` or `error`)
- failed upstream response → mark account `degraded` or `error` depending on severity

This ensures that accounts used frequently have the most accurate health status.

## 8. Admin API Details

### 8.1 Response format

All admin API responses use consistent JSON envelope:

Success:
```json
{
  "data": { ... }
}
```

List:
```json
{
  "data": [ ... ],
  "total": 42
}
```

Error:
```json
{
  "error": {
    "message": "account not found",
    "code": "not_found"
  }
}
```

### 8.2 Account API

**POST /admin/api/accounts** — create account

Request:
```json
{
  "label": "OpenAI Main",
  "base_url": "https://api.openai.com/v1",
  "api_key": "sk-...",
  "enabled": true,
  "quota_total": 100,
  "quota_used": 0,
  "quota_unit": "USD",
  "model_allowlist": ["gpt-4o", "gpt-4o-mini"],
  "notes": "Personal account"
}
```

**GET /admin/api/accounts** — list accounts

Response includes all fields except `api_key` is masked: `"sk-...xxxx"` (last 4 chars).

Includes real-time `status`, `last_checked_at`, `last_error` from health checker.

### 8.3 Pool API

**POST /admin/api/pools** — create pool

Request:
```json
{
  "label": "OpenAI Pool",
  "strategy": "priority-first-healthy",
  "enabled": true,
  "members": [
    { "account_id": 1, "priority": 1, "weight": 10 },
    { "account_id": 2, "priority": 2, "weight": 5 }
  ]
}
```

Members are stored in `pool_members` table. The API accepts and returns them as a nested array.

### 8.4 Token API

**POST /admin/api/tokens** — create token

Request:
```json
{
  "name": "my-project",
  "token": "",
  "quota_tokens": 1000000
}
```

If `token` is empty, Lune generates one: `sk-lune-{random32hex}`.

Response includes the full token value on creation only. Subsequent GET requests mask it.

### 8.5 Export API

**GET /admin/api/export** — full config backup

Returns:
```json
{
  "exported_at": "2026-04-12T10:00:00Z",
  "accounts": [ ... ],
  "pools": [ ... ],
  "model_routes": [ ... ],
  "access_tokens": [ ... ],
  "settings": { ... }
}
```

API keys in accounts are masked. Token values in access_tokens are masked.

## 9. Error Responses (Gateway)

All gateway errors follow OpenAI error format:

```json
{
  "error": {
    "message": "human-readable explanation",
    "type": "gateway_error",
    "code": "machine_readable_code"
  }
}
```

| Scenario | HTTP | Code | Message |
|---|---|---|---|
| missing/invalid Bearer | 401 | `invalid_token` | "invalid access token" |
| token disabled | 403 | `token_disabled` | "access token is disabled" |
| token quota exhausted | 429 | `quota_exhausted` | "token quota exhausted" |
| no route, no default pool | 404 | `no_route` | "no route for model: {name}" |
| pool disabled | 503 | `pool_disabled` | "pool is disabled" |
| no healthy account | 503 | `no_healthy_account` | "no healthy account available" |
| all retries failed | 502 | `upstream_failed` | "all upstream attempts failed" |

## 10. Package Structure

```
cmd/
  lune/
    main.go              — CLI entry point (up, version, check)

internal/
  app/
    app.go               — application initialization and lifecycle
  store/
    store.go             — SQLite operations, schema migrations
    cache.go             — in-memory routing cache
  auth/
    admin.go             — admin auth middleware (localhost trust + Bearer)
    gateway.go           — gateway auth middleware (access token validation)
  admin/
    handler.go           — admin API handler (accounts, pools, routes, tokens, etc.)
  gateway/
    handler.go           — gateway request handler (routing + proxy)
    proxy.go             — HTTP transparent proxy (forwarding, streaming)
    usage.go             — upstream usage parsing
  router/
    router.go            — model resolution + pool/account selection
  health/
    checker.go           — background health check goroutine
  httpserver/
    server.go            — HTTP server setup, route registration, middleware
  site/
    embed.go             — embedded frontend assets
  webutil/
    response.go          — HTTP response helpers
```

## 11. Implementation Phases

### Phase 1: Foundation and control plane

1. SQLite schema + migrations
2. Store layer (CRUD for all entities)
3. Config cache
4. Admin auth middleware
5. Admin API handler (accounts, pools, routes, tokens, settings)
6. CLI: `lune up` (start server), `lune version`, `lune check`
7. Embedded frontend serving (placeholder)

**Exit criterion**: `lune up` starts, admin API works via curl, cache invalidation works.

### Phase 2: Gateway and health

1. Gateway auth middleware
2. Router (model resolution, pool selection, account selection)
3. Transparent proxy (request forwarding, body rewriting, streaming)
4. Usage parsing (non-streaming + streaming)
5. Request logging
6. Token quota enforcement + accumulation
7. Health checker goroutine
8. Retry/fallback logic

**Exit criterion**: a real LLM request can be sent through the gateway, logged, and token usage accumulated.

### Phase 3: Polish

1. Overview API (stats aggregation)
2. Usage API (breakdowns, pagination)
3. Export API
4. Startup banner polish
5. Error message review
6. `lune check` validation
7. Graceful shutdown

**Exit criterion**: all APIs complete, all acceptance scenarios pass.
