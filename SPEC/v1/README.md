# Lune v1 Spec

## Overview

Lune v1 is a personal-use LLM gateway. Single Go binary, embedded admin UI, transparent proxy to upstream OpenAI-compatible providers. No one-api dependency.

**Architecture**: `Client → Lune → Provider`

**Spec Documents**:

| Document | Scope |
|---|---|
| [Part 0: Cleanup Manifest](./part0-cleanup.md) | What to delete before writing any v1 code. Complete removal of old concepts, code, and data. |
| [Part 1: Backend Design](./part1-backend.md) | Go backend: domain model, SQLite schema, CLI, HTTP server, gateway proxy, health checker, admin API. |
| [Part 2: Frontend Design](./part2-frontend.md) | React admin UI: pages, components, data fetching, UX patterns, type definitions. |

## Core Product Decisions

- **Single binary**: Go binary with embedded React SPA, SQLite database
- **No login wall**: localhost requests are trusted, no session/cookie needed
- **Transparent proxy**: `/v1/*` is forwarded to upstream as-is, no endpoint whitelist
- **Token-based quotas**: access tokens are metered by token usage (input+output), not request count
- **Default pool**: catch-all routing for unlisted models through a configurable default pool
- **Active health checks**: background goroutine checks accounts every 60s
- **SQLite as source of truth**: all mutable config in database, not JSON files
- **Clean break**: no migration from old Lune, no old code semantics carried over

## Implementation Phases

### Phase 1: Foundation and control plane
- SQLite schema + store layer
- Config cache with version-based invalidation
- Admin auth middleware (localhost trust)
- Admin CRUD APIs (accounts, pools, routes, tokens)
- Admin UI pages (CRUD)
- CLI: `lune up`, `lune version`, `lune check`

### Phase 2: Gateway and health
- Gateway auth middleware (access token validation)
- Model resolution + pool/account selection
- Transparent proxy with streaming
- Request logging with token usage parsing
- Token quota enforcement
- Active health checker
- Retry/fallback logic

### Phase 3: Polish and dashboard
- Overview dashboard
- Usage page with filtering
- Export API
- Error message polish
- Startup banner polish

## Success Criteria

v1 is complete when:

1. `lune up` starts the service in one command
2. `http://127.0.0.1:7788/admin` opens the admin UI with no login
3. User can configure accounts, pools, routes, and tokens through the UI
4. User can set `OPENAI_BASE_URL=http://localhost:7788/v1` + `OPENAI_API_KEY=sk-lune-xxx` in any project
5. Real LLM requests flow through the gateway with logging and token usage tracking
6. Account health is monitored and displayed in real-time

## Deferred Beyond v1

- Automatic provider balance scraping
- Provider-native adapters beyond OpenAI-compatible
- Remote admin auth hardening
- Per-token model/pool ACLs
- Richer routing policies
- Multi-user support
- API key encryption at rest
- Provider templates (one-click account setup)
- Built-in playground
- Cost estimation dashboard
- Latency sparklines
- One-click test connection
