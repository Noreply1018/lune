# Lune

Personal-use LLM API gateway. Single Go binary with embedded admin UI, transparent proxy to upstream OpenAI-compatible providers.

**Architecture:** Client → Lune (auth + routing) → LLM Provider

## Status

v1 is under development.

## Development

### Go

```bash
go build ./cmd/lune      # Build
go run ./cmd/lune         # Run
```

### Web UI

```bash
cd web
npm install
npm run build             # TypeScript check + Vite build → internal/site/dist/
npm run dev               # Vite dev server :5173
```

## Environment Variables

| Variable | Purpose | Default |
|---|---|---|
| `LUNE_PORT` | HTTP server port | `7788` |
| `LUNE_DATA_DIR` | SQLite database directory | `./data` |
| `LUNE_ADMIN_TOKEN` | Admin token override | auto-generated |
