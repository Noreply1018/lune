# Lune 🌙

> 一座安静的、为我自己点亮的 LLM 网关。
>
> 我每天用它对话、写代码、做实验。它在本机上跑着，单二进制，不打扰任何人。
> 如果你也需要一个这样的东西，欢迎带走。

[![License](https://img.shields.io/github/license/Noreply1018/lune?color=blue)](./LICENSE)
[![Release](https://img.shields.io/github/v/release/Noreply1018/lune?include_prereleases&sort=semver)](https://github.com/Noreply1018/lune/releases)
[![GHCR](https://img.shields.io/badge/ghcr.io-noreply1018%2Flune-2496ed?logo=docker)](https://github.com/Noreply1018/lune/pkgs/container/lune)

![Overview](./docs/screenshots/overview.png)

## 关于 Lune

Lune 是一个面向**个人使用**的 LLM API 网关：对下游暴露 OpenAI 兼容接口，对上游支持两种账号来源——OpenAI 兼容 Provider 直连，以及通过 **CPA 服务**（CLI Proxy API）聚合的通道。

> 做 Lune 是因为 OneAPI 对个人用户来说太重，LiteLLM 又缺一个能看的管理页面。
> 我想要一个能装进口袋、有一点自己风格、晚上不用想着它的东西。

**架构：** `Client → Lune（鉴权 + 路由）→ LLM Provider / CPA 服务`

**技术栈：** Go（网关）· TypeScript/React（管理界面）· SQLite（持久化）

## 它会做什么

- **双源账号** — OpenAI 兼容直连 + CPA 服务聚合，统一池化路由
- **账号池** — Priority-weighted 调度、自动重试、健康检查
- **模型路由** — alias → pool → account → upstream，支持混合 pool
- **CPA 服务管理** — Device Code 登录 OpenAI Codex、凭据热加载、远程账号批量导入、过期预警
- **体验细节** — Provider 模板自动填充、一键测试连接、Env Snippets、内置 Playground
- **观测** — 成本估算、延迟百分位追踪（p50/p95/p99）、账号级 Sparkline

![Pool Detail](./docs/screenshots/pool-detail.png)

## 关于 CPA 服务

你可能第一次听说 CPA——它是 [CLI Proxy API](https://github.com/router-for-me/CLIProxyAPI) 的简称，把 Claude Code、OpenAI Codex CLI 这类工具的登录凭据包装成统一的 OpenAI 兼容接口。

Lune 做的事，是把 CPA 当成一种**二等账号来源**：你可以直接把 API Key 填进 Lune（`openai_compat` 模式），也可以让 Lune 通过 CPA 托管一批 CLI 登录账号（`cpa` 模式）。两种账号并存在同一个池里，Lune 负责挑一条可用的路发出去。

什么时候用哪种？大致是：

- **手头有 API Key** → 直连，最短路径
- **只有 CLI 登录**（比如 ChatGPT Plus 的 Codex 访问）→ 走 CPA，让 Lune 帮你做凭据管理和过期预警

## 快速开始

适合只想跑起来的使用者，无需本地构建：

```bash
# 1. 下载 compose 文件和配置模板
curl -O https://raw.githubusercontent.com/Noreply1018/lune/main/docker-compose.prod.yml
curl -O https://raw.githubusercontent.com/Noreply1018/lune/main/.env.example
curl -O https://raw.githubusercontent.com/Noreply1018/lune/main/cpa-config.yaml

# 2. 复制并编辑环境变量
cp .env.example .env
# ⚠️ 生产使用务必修改 .env 里的 CPA_API_KEY、LUNE_CPA_MANAGEMENT_KEY
#    同时同步修改 cpa-config.yaml 里的 api-keys 和 remote-management.secret-key
#    两边必须一致，否则 Lune 无法调用 CPA 服务
#    仓库中的默认值仅用于本地示例，不应直接暴露在公网环境

# 3. 启动
docker compose -f docker-compose.prod.yml --env-file .env up -d
```

启动后访问 `http://127.0.0.1:7788/admin`。首次进入需用 `LUNE_ADMIN_TOKEN` 登录；若 `.env` 里留空，容器启动日志会打印自动生成的 token（`docker compose logs lune`）。

**升级：**

```bash
docker compose -f docker-compose.prod.yml --env-file .env pull
docker compose -f docker-compose.prod.yml --env-file .env up -d
```

**停止 / 查看日志：**

```bash
docker compose -f docker-compose.prod.yml --env-file .env down   # 停止（保留数据卷）
docker compose -f docker-compose.prod.yml logs -f lune           # 跟随 Lune 日志
docker compose -f docker-compose.prod.yml logs -f cpa            # 跟随 CPA 服务日志
```

## 配置

Docker Compose 场景（默认运行方式）通过根目录的 `.env` 文件配置——`docker compose` 会自动读取它并注入到容器环境变量里。

仓库提供 [.env.example](./.env.example) 作为模板。本地使用时复制一份：

```bash
cp .env.example .env
# 按需修改 LUNE_PORT / LUNE_ADMIN_TOKEN / CPA_API_KEY 等
```

`.env` 已被 `.gitignore` 忽略。下表是 Lune 能识别的**全部**环境变量——`.env.example` 只列出日常需要修改的几项，其余（如数据目录、CPA 容器地址）由 `docker-compose.yml` 直接固定在容器里，一般不需要动。

### 环境变量

| 变量 | 用途 | 默认值 |
|---|---|---|
| `LUNE_IMAGE_TAG` | 预构建镜像 tag（用于 `docker-compose.prod.yml`） | `latest` |
| `LUNE_PORT` | HTTP 服务端口 | `7788` |
| `LUNE_DATA_DIR` | SQLite 数据目录 | `./data` |
| `LUNE_ADMIN_TOKEN` | 管理令牌覆盖 | 自动生成 |
| `LUNE_CPA_AUTH_DIR` | CPA 凭据文件目录 | `./cpa-auth` |
| `LUNE_CPA_BASE_URL` | Lune 连接 CPA 的地址 | Docker: `http://cpa:8317` |
| `LUNE_CPA_API_KEY` | Lune 使用的 CPA API Key | 同 `CPA_API_KEY` |
| `LUNE_CPA_MANAGEMENT_KEY` | Lune 访问 CPA 管理 API 的密钥 | `lune-cpa-management-dev` |
| `CPA_API_KEY` | CPA 服务 API Key | `sk-cpa-default` |
| `LUNE_LOG_LEVEL` | 日志级别：`debug` / `info` / `warn` / `error` | `info` |
| `LUNE_LOG_FORMAT` | 日志格式：`text` / `json` | `text` |

## Docker 与 CPA 服务

CPA 服务是外部代理，但在本仓库里已经纳入 `docker-compose.yml`，默认通过 Docker Compose 与 Lune 一起启动。

在 Docker Compose 场景下：

- `./scripts/up.sh` 会同时启动 Lune 和 CPA 服务
- Lune 默认通过 `http://cpa:8317` 访问 CPA 服务
- CPA 服务连接会由 Compose 里的环境变量自动配置，无需在后台手动新增
- 双方通过共享 `cpa-auth` 卷交换凭据文件：Lune 直接对接 OpenAI Device Code 登录后将凭据写入该目录，CPA 服务热加载自动识别
- OAuth 授权页依赖容器内的系统 CA 证书；如果你自定义运行镜像，确保安装了 `ca-certificates`
- 如果公司网络会拦截 HTTPS，需要把企业根证书注入容器或宿主机信任链，不能通过关闭 TLS 校验绕过

## 本地开发

Lune 的日常开发流程也走 Docker Compose，无需在本机装 Go 或 Node——所有构建都在 `./scripts/rebuild.sh` 触发的镜像构建里完成。

```bash
./scripts/up.sh
```

这会同时启动 Lune 和 CPA 服务，并在终端打印当前访问地址。地址展示会优先取运行中的 Docker 端口映射，拿不到时再回退到 `LUNE_PORT` 环境变量或 `.env` 文件。

所有常用操作都统一通过 `scripts/` 下的 Docker 包装脚本完成：

```bash
./scripts/up.sh               # 启动 Lune + CPA，并打印访问地址
./scripts/restart.sh          # 默认重启 Lune
./scripts/restart.sh cpa      # 重启 CPA
./scripts/restart.sh all      # 重启全部服务
./scripts/logs.sh             # 默认跟随 Lune 日志
./scripts/logs.sh cpa         # 跟随 CPA 日志
./scripts/logs.sh all         # 跟随全部日志
./scripts/ps.sh               # 查看容器状态
./scripts/down.sh             # 停止并移除容器
./scripts/rebuild.sh          # 重新构建并启动
```

Docker Compose 是唯一推荐的运行和开发方式。欢迎直接提交 GitHub Issue 或 Pull Request。

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

Localhost 及私有网络免认证，远程需 Bearer admin token。

| 资源 | 接口 |
|---|---|
| Accounts | `GET/POST /accounts`, `PUT/DELETE /accounts/{id}`, `POST /accounts/{id}/enable\|disable` |
| Accounts 扩展 | `POST /accounts/test-connection`, `POST /accounts/{id}/discover-models`, `GET /accounts/{id}/models` |
| Pools | `GET/POST /pools`, `GET/PUT/DELETE /pools/{id}`, `POST /pools/{id}/enable\|disable` |
| Pool Members | `POST /pools/{id}/members`, `PUT /pools/{id}/members/reorder`, `PUT/DELETE /pools/{id}/members/{member_id}` |
| Pool Tokens | `GET /pools/{id}/tokens` |
| Tokens | `GET/POST /tokens`, `PUT/DELETE /tokens/{id}`, `POST /tokens/{id}/enable\|disable\|regenerate`, `POST /tokens/{id}/reveal` |
| Settings | `GET/PUT /settings`, `GET /settings/data-retention`, `POST /settings/data-retention/prune` |
| Notifications | `GET/PUT /notifications/settings`, `PUT /notifications/subscriptions/{event}`, `POST /notifications/test`, `GET /notifications/deliveries` |
| Stats | `GET /overview`, `GET /usage`, `GET /usage/latency`, `GET /export`, `POST /import` |
| CPA 服务 | `GET/PUT/DELETE /cpa/service`, `POST /cpa/service/test\|enable\|disable` |
| CPA 登录 | `POST /accounts/cpa/login-sessions`, `GET /accounts/cpa/login-sessions/{id}`, `POST /accounts/cpa/login-sessions/{id}/cancel` |
| CPA 导入 | `GET /cpa/service/remote-accounts`, `POST /accounts/cpa/import`, `POST /accounts/cpa/import/batch` |

### 网关接口（`/v1/*`、`/openai/v1/*`）

Bearer access token 认证。透明代理到上游 provider。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/v1/models` | 可用模型列表（无需认证） |
| POST | `/v1/chat/completions` | 代理到上游（支持 streaming） |
| POST | `/v1/*` | 其他 OpenAI 兼容端点透传 |

## License

Lune 以 [Apache-2.0](./LICENSE) 协议开源。你可以自由使用、修改和分发本项目，但需要遵守许可证中的声明保留与授权条件。
