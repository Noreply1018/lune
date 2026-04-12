# Lune

Personal-use LLM API gateway. Single Go binary with embedded React admin UI, transparent proxy to upstream OpenAI-compatible providers.

**Architecture:** Client → Lune (auth + routing) → LLM Provider

## Quick Start

```bash
go run ./cmd/lune         # 启动服务
open http://127.0.0.1:7788/admin
```

## CLI

```bash
lune up                   # 启动 HTTP 服务（默认）
lune version              # 打印版本信息
lune check                # 校验数据库并打印摘要
```

## Configuration

优先级：`lune.yaml` → 环境变量覆盖

| Variable | Purpose | Default |
|---|---|---|
| `LUNE_PORT` | HTTP 服务端口 | `7788` |
| `LUNE_DATA_DIR` | SQLite 数据目录 | `./data` |
| `LUNE_ADMIN_TOKEN` | 管理令牌覆盖 | 自动生成 |

`lune.yaml` 示例：

```yaml
port: 7788
data_dir: ./data
```

## HTTP Endpoints

### Health

| Method | Path | Description |
|---|---|---|
| GET | `/healthz` | 存活检查，始终 200 |
| GET | `/readyz` | 就绪检查，需至少一个 account |

### Admin UI

| Method | Path | Description |
|---|---|---|
| GET | `/` | 重定向到 `/admin` |
| GET | `/admin`, `/admin/*` | 嵌入式 React SPA |

### Admin API (`/admin/api/*`)

Localhost 免认证，远程需 Bearer admin token。

| Resource | Endpoints |
|---|---|
| Accounts | `GET/POST /accounts`, `PUT/DELETE /accounts/{id}`, `POST /accounts/{id}/enable\|disable` |
| Pools | `GET/POST /pools`, `PUT/DELETE /pools/{id}`, `POST /pools/{id}/enable\|disable` |
| Routes | `GET/POST /routes`, `PUT/DELETE /routes/{id}` |
| Tokens | `GET/POST /tokens`, `PUT/DELETE /tokens/{id}`, `POST /tokens/{id}/enable\|disable` |
| Settings | `GET/PUT /settings` |
| Stats | `GET /overview`, `GET /usage`, `GET /export` |

### Gateway (`/v1/*`, `/openai/v1/*`)

Bearer access token 认证。透明代理到上游 provider。

| Method | Path | Description |
|---|---|---|
| GET | `/v1/models` | 可用模型列表（无需认证） |
| POST | `/v1/chat/completions` | 代理到上游（支持 streaming） |
| POST | `/v1/*` | 其他 OpenAI 兼容端点透传 |

## Development

### Go

```bash
go build ./cmd/lune                            # 构建
go run ./cmd/lune                              # 运行
CGO_ENABLED=0 go build -o lune ./cmd/lune      # 生产构建
```

### Web UI

```bash
cd web
npm install
npm run build      # 类型检查 + vite 构建 → internal/site/dist/
npm run dev        # vite 开发服务器 :5173（代理 /admin/api → :7788）
```

### Docker

```bash
docker compose up -d lune
```
