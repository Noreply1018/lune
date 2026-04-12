# Lune v1 — Part 0: Cleanup Manifest

> This document defines the complete removal scope before any v1 code is written.
> v1 is a clean break. No old code semantics may enter the new implementation.

## 1. Principle

The old Lune architecture is:

```
Client -> Lune -> one-api backend -> provider
```

The new Lune v1 architecture is:

```
Client -> Lune -> provider
```

Every artifact that exists to support the old architecture must be deleted. The codebase must be cleaned to a minimal skeleton before new code begins.

## 2. Concepts to Erase

The following concepts from the old Lune must not survive into v1 in any form — not in code, not in naming, not in comments, not in UI text:

| Old Concept | Why It Dies | v1 Replacement |
|---|---|---|
| one-api backend | no secondary backend in v1 | Lune talks directly to providers |
| backend proxy (`/backend/*`) | one-api relay path | removed entirely |
| channels | one-api abstraction for upstream providers | accounts + pools |
| platform (as a separate entity) | unnecessary layer when all upstreams are OpenAI-compatible | account is self-contained (has its own `base_url`) |
| adapter registry | multi-adapter pattern for different provider protocols | transparent HTTP proxy (one code path) |
| execution plan | over-engineered request routing model | simple pool → account selection |
| JSON config as source of truth | fragile, hard to manage from UI | SQLite is the sole source of truth |
| runtime config file watcher | watches JSON config file for changes | in-memory cache with version counter, invalidated by admin API writes |
| admin token login page | requires manual auth in local use | localhost is trusted by default |
| setup wizard / bootstrap flow | one-api initialization ceremony | empty database auto-created on startup, user configures via admin UI |
| platform registry + health polling | complex platform-level health abstraction | simple per-account health checker goroutine |
| `credential_env` (env var indirection for secrets) | unnecessary complexity for personal tool | `api_key` stored directly |
| `plan_type`, `risk_score`, `cooldown_until` on accounts | over-complex account state model | simple 4-state health: healthy/degraded/error/disabled |
| `cost_per_request`, call-count-based quotas | wrong granularity | token-usage-based quotas |
| `provider_id`, `platform_id` as foreign keys | platform entity removed | account is self-contained |
| `egress_proxy_env` on accounts | not needed for v1 personal use | removed |
| `api_cost_units`, `account_cost_units`, `account_cost_type` | over-complex cost model | simple input_tokens + output_tokens |
| `LUNE_BACKEND_URL` env var | pointed to one-api backend | removed |
| `LUNE_CONFIG` env var for JSON config path | JSON config no longer primary | replaced by `LUNE_DATA_DIR` and `LUNE_PORT` |
| `stream_smoothing`, `stream_heartbeat` server config | over-engineered streaming features | transparent stream passthrough |

## 3. Go Code to Delete

### 3.1 Entire packages to delete (remove the directory)

```
internal/adapter/           — old multi-adapter pattern (registry + openai_upstream)
internal/api/backendproxy/  — one-api reverse proxy handler
internal/config/            — old JSON config model (structs, loader, validator)
internal/runtimeconfig/     — file-watcher-based config hot-reload
internal/platform/          — platform registry + health polling
internal/execution/         — old execution types (Plan, PreparedExecution, etc.)
internal/metrics/           — old in-memory metrics
```

### 3.2 Packages to delete and rewrite from scratch

These packages will exist in v1 but with completely different implementations. Delete first, then write new:

```
internal/store/             — old SQLite schema (5 old tables, sync-from-config pattern)
internal/router/            — old routing logic (platform-aware, fallback parsing)
internal/proxy/             — old proxy service (adapter-based execution pipeline)
internal/auth/              — old auth middleware (config-based token lookup)
internal/api/admin/         — old admin handler (1162 lines, config-mutation pattern)
internal/api/public/        — old public handler (platform-aware model listing)
internal/app/               — old app initialization (config-first startup)
```

### 3.3 Files to delete in preserved packages

```
cmd/lune/main.go            — gut the contents, keep the file. Remove all old imports and initialization.
internal/httpserver/server.go — gut the route registration. Keep the server struct pattern.
```

### 3.4 Go dependencies to evaluate for removal

After deletion, run `go mod tidy` to remove unused dependencies. Specifically check:

- any one-api client libraries
- any config file parsing libraries that are no longer needed (if JSON config is gone)
- any platform-specific SDK imports

## 4. Frontend Code to Delete

### 4.1 Pages to delete

```
web/src/pages/LoginPage.tsx      — login form (replaced by auto-trust localhost)
web/src/pages/ChannelsPage.tsx   — channels list (one-api concept, removed)
web/src/pages/SetupWizard.tsx    — bootstrap wizard (one-api initialization)
web/src/pages/SettingsPage.tsx   — old settings (backend URL, etc.)
```

### 4.2 Auth module to rewrite

```
web/src/lib/auth.ts              — session token storage (sessionStorage-based login)
web/src/lib/api.ts               — API client (Bearer token from sessionStorage)
```

The new auth model: admin API requests from localhost need no auth header. The frontend simply calls `/admin/api/*` without injecting a token. The `api.ts` module should be a plain fetch wrapper.

### 4.3 Components to evaluate

```
web/src/components/Shell.tsx     — sidebar navigation. Keep structure, update nav items:
                                    remove: Channels, Settings (old), Login
                                    keep: Overview, Accounts, Pools, Tokens, Usage
                                    add: Routes
```

### 4.4 Frontend concepts to remove

- `lune_admin_token` in sessionStorage
- automatic redirect to `/login` on 401
- any reference to "backend", "backend URL", "channels", "one-api"
- `isAuthenticated()` / `getLuneToken()` / `setLuneToken()` / `logout()` functions

## 5. Config and Data Files to Delete

```
configs/                    — entire directory (old JSON config)
configs/config.json         — old config file
data/lune.db                — old database (incompatible schema)
```

The new data directory will be created fresh by `lune up`.

## 6. Docker and Deployment Files to Evaluate

```
docker-compose.yml          — review and simplify. Remove one-api/backend service definitions.
                               v1 is single binary, Docker is optional.
Dockerfile                  — review. Should build single Go binary + embedded frontend.
                               Remove any one-api or backend references.
```

## 7. Documentation to Delete or Rewrite

```
README.md                   — rewrite for v1 (remove all one-api references)
CLAUDE.md                   — update project description and commands
```

## 8. Environment Variables

### Old env vars to stop supporting

| Variable | Old Purpose | Action |
|---|---|---|
| `LUNE_CONFIG` | path to JSON config file | remove |
| `LUNE_BACKEND_URL` | one-api backend URL | remove |

### New env vars for v1

| Variable | Purpose | Default |
|---|---|---|
| `LUNE_PORT` | HTTP server port | 7788 |
| `LUNE_DATA_DIR` | SQLite database directory | `./data` |
| `LUNE_ADMIN_TOKEN` | override admin token (for programmatic access) | auto-generated |

## 9. Verification Checklist

After cleanup is complete, verify:

- [ ] `grep -r "one-api" --include="*.go"` returns zero results
- [ ] `grep -r "one-api" --include="*.ts" --include="*.tsx"` returns zero results
- [ ] `grep -r "backend" internal/ --include="*.go"` returns zero results (except generic HTTP terms)
- [ ] `grep -r "channel" internal/ --include="*.go"` returns zero results (except Go channel keyword)
- [ ] `grep -r "platform" internal/ --include="*.go"` returns zero results
- [ ] `grep -r "adapter" internal/ --include="*.go"` returns zero results
- [ ] `grep -r "execution" internal/ --include="*.go"` returns zero results (except new execution context if any)
- [ ] `grep -r "LUNE_CONFIG" --include="*.go"` returns zero results
- [ ] `grep -r "LUNE_BACKEND" --include="*.go"` returns zero results
- [ ] `grep -r "LoginPage\|ChannelsPage\|SetupWizard" web/src/` returns zero results
- [ ] `grep -r "lune_admin_token\|sessionStorage" web/src/` returns zero results
- [ ] `configs/` directory does not exist
- [ ] `go build ./cmd/lune` succeeds (with stub main)
- [ ] `cd web && npm run build` succeeds (with stub app)
- [ ] no import references to deleted packages remain

## 10. Execution Order

1. **Git commit** the current state as a reference point (tag: `pre-v1-cleanup`)
2. Delete all packages listed in 3.1
3. Delete all packages listed in 3.2
4. Gut files listed in 3.3
5. Delete frontend files listed in 4.1 and 4.2
6. Delete config/data files listed in 5
7. Clean up imports: `go mod tidy`
8. Create minimal stub `main.go` that compiles: `func main() { fmt.Println("lune v1") }`
9. Create minimal stub frontend that builds
10. Run verification checklist in Section 9
11. **Git commit** the cleaned state (tag: `v1-clean-slate`)
12. Begin v1 implementation from this clean base
