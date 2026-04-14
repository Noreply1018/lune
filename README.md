# Lune

面向个人使用的 LLM API 网关。采用单一 Go 二进制发布，内置 React 管理后台，支持 OpenAI 兼容提供商直连与 CPA（CLI Proxy API）聚合两种账号来源。

**架构：** Client → Lune（鉴权 + 路由）→ LLM Provider / CPA Service

## 特性

- **双源账号** — OpenAI 兼容直连 + CPA 聚合，统一池化路由
- **账号池** — Priority-weighted 调度、自动重试、健康检查
- **模型路由** — alias → pool → account → upstream，支持混合 pool
- **CPA 管理** — 设备码登录、凭据热加载、远程账号批量导入、过期预警
- **体验增强** — Provider 模板自动填充、一键测试连接、Env Snippets、内置 Playground
- **分析** — 成本估算、延迟百分位追踪（p50/p95/p99）、账号级 Sparkline

## 快速开始

默认启动方式：

```bash
docker compose up -d
```

这会同时启动 Lune 和 CPA。启动后可访问：

- Admin UI: `http://127.0.0.1:7788/admin`
- Gateway API: `http://127.0.0.1:7788/v1`

常用状态查看：

```bash
docker compose ps
docker compose logs -f lune
```

## 启动与开发命令

### Docker

```bash
docker compose up -d           # 启动 Lune + CPA
docker compose restart lune    # 重启 Lune
docker compose restart cpa     # 重启 CPA
docker compose ps              # 查看容器状态
docker compose logs -f lune    # 查看 Lune 日志
docker compose logs -f cpa     # 查看 CPA 日志
```

Docker Compose 是推荐运行方式。`docker-compose.yml` 已经把 Lune 和 CPA 接好，容器内默认通过 `http://cpa:8317` 通信。

### 本地开发

```bash
go run ./cmd/lune              # 本地直接运行 Lune
./scripts/dev-restart.sh       # 按当前配置端口重启本地开发进程
air                            # 监听 Go 文件和 lune.yaml，自动重编译 + 重启
go build ./cmd/lune            # 构建
CGO_ENABLED=0 go build -o lune ./cmd/lune
```

这些命令主要用于仓库开发和调试，不是默认启动路径。

`./scripts/dev-restart.sh` 会按下面的优先级解析当前端口：

- `LUNE_PORT`
- 本地 `lune.yaml` 里的 `port`
- 默认值 `7788`

### Air

`air` 是一个 Go 开发期的开源 live reload 工具。保存 Go 文件或本地 `lune.yaml` 后，它会自动重新编译并重启当前进程。

安装 `air`：

```bash
go install github.com/air-verse/air@latest
```

仓库已提交 [`.air.toml`](/home/lh/projects/lune/.air.toml) 作为默认配置，行为如下：

- 构建产物输出到 `tmp/`
- 监听 `go` 文件和本地 `lune.yaml`
- 忽略 `data/`、`cpa-auth/`、`web/`、`internal/site/dist/` 等目录

### CLI

```bash
lune up
lune version
lune check
```

## 配置

配置优先级：`lune.yaml` → 环境变量覆盖

仓库提供 [lune.example.yaml](/home/lh/projects/lune/lune.example.yaml) 作为示例配置；本地使用时复制为 `lune.yaml` 后再按环境修改。`lune.yaml` 被 `.gitignore` 忽略，用来保存本机目录、地址和 key 等本地值。

示例配置：

```yaml
port: 7788
data_dir: ./data
cpa_auth_dir: ./cpa-auth
cpa_base_url: http://127.0.0.1:8317
cpa_api_key: sk-cpa-default
```

### 环境变量

| 变量 | 用途 | 默认值 |
|---|---|---|
| `LUNE_PORT` | HTTP 服务端口 | `7788` |
| `LUNE_DATA_DIR` | SQLite 数据目录 | `./data` |
| `LUNE_ADMIN_TOKEN` | 管理令牌覆盖 | 自动生成 |
| `LUNE_CPA_AUTH_DIR` | CPA 凭据文件目录 | `./cpa-auth` |
| `LUNE_CPA_BASE_URL` | Lune 连接 CPA 的地址 | Docker: `http://cpa:8317` |
| `LUNE_CPA_API_KEY` | Lune 使用的 CPA API Key | 同 `CPA_API_KEY` |
| `CPA_API_KEY` | CPA 服务 API Key | `sk-cpa-default` |

## Docker 与 CPA

CPA 是外部代理服务，但在本仓库里已经纳入 `docker-compose.yml`，默认通过 Docker Compose 与 Lune 一起启动。

需要区分两件事：

- Docker 启动 CPA：由 `docker compose up -d` 负责
- 配置默认 CPA 连接：由 `LUNE_CPA_BASE_URL` 或本地 `lune.yaml` 负责

在 Docker Compose 场景下，Lune 默认连 `http://cpa:8317`，无需再到后台手动新增默认 CPA Service。

如果你本地直接运行 `go run ./cmd/lune`，而 CPA 仍在宿主机或容器外暴露 `8317`，则本地 `lune.yaml` 可以写成：

```yaml
cpa_base_url: http://127.0.0.1:8317
cpa_api_key: sk-cpa-default
```

Lune 和 CPA 通过共享 `cpa-auth` 卷交换凭据文件，CPA 会热加载该目录的变更。

## HTTP 接口

### 健康检查

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/healthz` | 存活检查，始终 200 |
| GET | `/readyz` | 就绪检查，需至少一个 account |

### 管理界面

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/` | 重定向到 `/admin` |
| GET | `/admin`, `/admin/*` | 嵌入式 React SPA |

### 管理 API（`/admin/api/*`）

Localhost 免认证，远程需 Bearer admin token。

| 资源 | 接口 |
|---|---|
| Accounts | `GET/POST /accounts`, `PUT/DELETE /accounts/{id}`, `POST /accounts/{id}/enable\|disable` |
| Accounts 扩展 | `POST /accounts/test-connection` |
| Pools | `GET/POST /pools`, `PUT/DELETE /pools/{id}`, `POST /pools/{id}/enable\|disable` |
| Routes | `GET/POST /routes`, `PUT/DELETE /routes/{id}` |
| Tokens | `GET/POST /tokens`, `PUT/DELETE /tokens/{id}`, `POST /tokens/{id}/enable\|disable` |
| Settings | `GET/PUT /settings` |
| Stats | `GET /overview`, `GET /usage`, `GET /usage/latency`, `GET /export` |
| CPA Service | `GET/PUT/DELETE /cpa/service`, `POST /cpa/service/test\|enable\|disable` |
| CPA Login | `POST /accounts/cpa/login-sessions`, `GET /accounts/cpa/login-sessions/{id}`, `POST /accounts/cpa/login-sessions/{id}/cancel` |
| CPA Import | `GET /cpa/service/remote-accounts`, `POST /accounts/cpa/import`, `POST /accounts/cpa/import/batch` |

### 网关接口（`/v1/*`、`/openai/v1/*`）

Bearer access token 认证。透明代理到上游 provider。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/v1/models` | 可用模型列表（无需认证） |
| POST | `/v1/chat/completions` | 代理到上游（支持 streaming） |
| POST | `/v1/*` | 其他 OpenAI 兼容端点透传 |
