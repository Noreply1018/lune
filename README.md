# Lune

面向个人使用的 LLM API 网关。采用单一 Go 二进制发布，内置 React 管理后台，支持 OpenAI 兼容提供商直连与 CPA（CLI Proxy API）聚合两种账号来源。

**架构：** Client → Lune（鉴权 + 路由）→ LLM Provider / CPA Service

## 开源许可

Lune 以 [Apache-2.0](./LICENSE) 协议开源。你可以自由使用、修改和分发本项目，但需要遵守许可证中的声明保留与授权条件。

## 特性

- **双源账号** — OpenAI 兼容直连 + CPA 聚合，统一池化路由
- **账号池** — Priority-weighted 调度、自动重试、健康检查
- **模型路由** — alias → pool → account → upstream，支持混合 pool
- **CPA 管理** — Device Code 登录 OpenAI Codex、凭据热加载、远程账号批量导入、过期预警
- **体验增强** — Provider 模板自动填充、一键测试连接、Env Snippets、内置 Playground
- **分析** — 成本估算、延迟百分位追踪（p50/p95/p99）、账号级 Sparkline

## 快速开始（使用预构建镜像）

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
#    两边必须一致，否则 Lune 无法调用 CPA
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
docker compose -f docker-compose.prod.yml logs -f cpa            # 跟随 CPA 日志
```

## 本地开发

从源码启动：

```bash
./scripts/up.sh
```

这会同时启动 Lune 和 CPA，并在终端打印当前访问地址。地址展示会优先取运行中的 Docker 端口映射，拿不到时再回退到 `LUNE_PORT` 和本地 `lune.yaml`。

如果你打算参与开发或修复问题，欢迎直接提交 GitHub Issue 或 Pull Request。

## 启动与开发命令

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

Docker Compose 是唯一推荐的运行和开发方式。

## 配置

配置优先级：`lune.yaml` → 环境变量覆盖

仓库提供 [lune.example.yaml](./lune.example.yaml) 作为示例配置；本地使用时复制为 `lune.yaml` 后再按环境修改。`lune.yaml` 被 `.gitignore` 忽略，用来保存本机目录、地址和 key 等本地值。

示例配置：

```yaml
port: 7788
data_dir: ./data
cpa_auth_dir: ./cpa-auth
cpa_base_url: http://127.0.0.1:8317
cpa_api_key: sk-cpa-default
cpa_management_key: lune-cpa-management-dev
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
| `LUNE_CPA_MANAGEMENT_KEY` | Lune 访问 CPA 管理 API 的密钥 | `lune-cpa-management-dev` |
| `CPA_API_KEY` | CPA 服务 API Key | `sk-cpa-default` |

## Docker 与 CPA

CPA 是外部代理服务，但在本仓库里已经纳入 `docker-compose.yml`，默认通过 Docker Compose 与 Lune 一起启动。

在 Docker Compose 场景下：

- `./scripts/up.sh` 会同时启动 Lune 和 CPA
- Lune 默认通过 `http://cpa:8317` 访问 CPA
- 默认 CPA 连接会由 Compose 里的环境变量自动配置，无需在后台手动新增
- 默认 Compose 通过共享 `cpa-auth` 卷交换凭据文件，Lune 直接对接 OpenAI Device Code 登录后将凭据写入该目录，CPA 热加载自动识别
- OAuth 授权页依赖容器内的系统 CA 证书；如果你自定义运行镜像，确保安装了 `ca-certificates`
- 如果公司网络会拦截 HTTPS，需要把企业根证书注入容器或宿主机信任链，不能通过关闭 TLS 校验绕过

Lune 和 CPA 通过共享 `cpa-auth` 卷交换凭据文件。Lune 通过 OpenAI Device Code 流程登录后将凭据写入该目录，CPA 热加载自动识别。

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
