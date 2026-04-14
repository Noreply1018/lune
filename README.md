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

如果你只是想启动 Lune 本体：

```bash
go run ./cmd/lune         # 启动服务
```

如果你要直接体验带 CPA 的完整后台，推荐用下面这条作为日常开发入口：

```bash
docker compose up -d cpa && go run ./cmd/lune
```

这里有一个关键区别：

- `go run ./cmd/lune` 只会启动 Lune，不会启动 CPA 进程
- `docker compose up -d cpa` 才是把本地 CPA 容器拉起来
- 本地 `lune.yaml` 里的 `cpa_base_url` / `cpa_api_key` 只负责让 Lune 自动连上已经在运行的 CPA，不负责把它启动起来

## CLI

```bash
lune up                   # 启动 HTTP 服务（默认）
lune version              # 打印版本信息
lune check                # 校验数据库并打印摘要
```

## 配置

优先级：`lune.yaml` → 环境变量覆盖

| 变量 | 用途 | 默认值 |
|---|---|---|
| `LUNE_PORT` | HTTP 服务端口 | `7788` |
| `LUNE_DATA_DIR` | SQLite 数据目录 | `./data` |
| `LUNE_ADMIN_TOKEN` | 管理令牌覆盖 | 自动生成 |
| `LUNE_CPA_AUTH_DIR` | CPA 凭据文件目录 | `./cpa-auth` |

仓库提供 [lune.example.yaml](/home/lh/projects/lune/lune.example.yaml) 作为示例配置；本地使用时复制为 `lune.yaml` 后再按环境修改。`lune.yaml` 被 `.gitignore` 忽略，用来保存本机目录、地址和 key 等本地值。

`lune.example.yaml` 示例：

```yaml
port: 7788
data_dir: ./data
cpa_auth_dir: ./cpa-auth
```

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

## 开发

### 本地开发

```bash
docker compose up -d cpa && go run ./cmd/lune
```

这是最实用的本地开发方式：

- CPA 是外部 Docker 服务，不是这个 Go 进程里的一个子模块，所以 `go run ./cmd/lune` 不会顺手把 CPA 也启动
- 第一次开发时先执行一次 `docker compose up -d cpa`，后面通常只需要反复 `go run ./cmd/lune`
- 如果 Docker Desktop、WSL 或宿主机重启过，需要先确认 CPA 容器已经重新起来
- 本地 `lune.yaml` 可预配置 `http://127.0.0.1:8317` 和默认 key；只要本地 8317 上真的有 CPA 在跑，Lune 首次启动就会自动写入 `Default CPA`

如果你想确认 CPA 是否已经在运行，可以执行：

```bash
docker compose ps cpa
```

### 开发启动与重启

如果你只是想手动重启当前 Lune 进程，直接运行：

```bash
./scripts/dev-restart.sh
```

这个脚本会按下面的优先级自动解析当前端口，然后杀掉旧进程并重新启动：

- `LUNE_PORT`
- 本地 `lune.yaml` 里的 `port`
- 默认值 `7788`

如果你希望改完文件后自动重启，推荐使用 `air`。它是一个 Go 开发期的开源 live reload 工具：监听文件变化，自动重新编译并重启当前进程，不需要你手工 `Ctrl+C` 再 `go run`。

首次安装：

```bash
go install github.com/air-verse/air@latest
```

日常使用：

```bash
air
```

当前仓库已提交 [`.air.toml`](/home/lh/projects/lune/.air.toml) 作为默认配置：

- 构建产物输出到 `tmp/`
- 监听 `go` 文件和本地 `lune.yaml`
- 忽略 `data/`、`cpa-auth/`、`web/`、`internal/site/dist/` 等目录
- 保存 Go 文件或 `lune.yaml` 后会自动重新编译并重启

`air` 适合日常后端开发；如果你只是偶尔想强制刷新一次进程，用 `./scripts/dev-restart.sh` 更直接。

```bash
go build ./cmd/lune                            # 构建
CGO_ENABLED=0 go build -o lune ./cmd/lune      # 生产构建
```

### Web 管理界面

```bash
cd web
npm install
npm run build      # 类型检查 + vite 构建 → internal/site/dist/
npm run dev        # vite 开发服务器 :5173（代理 /admin/api → :7788）
```

### Docker 部署

```bash
docker compose up -d          # 启动 Lune + CPA（生产推荐）
docker compose up -d lune     # 仅启动 Lune（不含 CPA）
```

`docker compose up -d` 是最接近“开箱即用”的方式，因为它会把 Lune 和 CPA 一起拉起。

### CPA 部署

CPA（[cli-proxy-api](https://hub.docker.com/r/eceasy/cli-proxy-api)）是外部代理服务，支持通过 ChatGPT Plus/Pro、Claude 等订阅账号访问 LLM 提供商。它已经被纳入 `docker-compose.yml`，但它仍然是一个独立进程，不会被 `go run ./cmd/lune` 自动拉起。

需要区分两件事：

- 自动启动 CPA：只有 `docker compose up -d` 或 `docker compose up -d cpa` 会做这件事
- 自动配置默认 CPA 连接：Lune 启动时如果发现 `LUNE_CPA_BASE_URL` / 本地 `lune.yaml` 里有配置，就会自动写入默认 CPA Service

也就是说，`自动配置` 不等于 `自动启动`。前提始终是：对应地址上的 CPA 服务已经可达。

在 Docker Compose 场景下，执行 `docker compose up -d` 后，Lune 会自动配置默认 CPA 连接（`http://cpa:8317`），无需再去后台手动新增。

**配置项：**

| 变量 | 用途 | 默认值 |
|---|---|---|
| `CPA_API_KEY` | CPA 服务 API Key | `sk-cpa-default` |
| `LUNE_CPA_BASE_URL` | Lune 连接 CPA 的地址 | `http://cpa:8317`（Docker 内部） |
| `LUNE_CPA_API_KEY` | Lune 使用的 CPA API Key | 同 `CPA_API_KEY` |

**网络地址：**

| 场景 | CPA Base URL |
|---|---|
| Docker Compose 部署 | `http://cpa:8317`（容器间通信） |
| 本地开发 | `http://127.0.0.1:8317` |

Lune 和 CPA 通过共享 `cpa-auth` 卷交换凭据文件，CPA 会热加载该目录的变更。
