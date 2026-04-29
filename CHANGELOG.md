# 更新日志

这里记录 Lune 每个版本中值得用户关注的变化。

Lune 目前仍处于早期 `0.x` 阶段。版本会尽量遵循语义化版本，但在默认体验、部署方式、配置形态还没有完全稳定前，minor 版本可能会调整产品边界。

## [0.1.3] - 未发布

状态：发布收口已完成，等待 tag。

### 重点变化

- Lune 默认改为单镜像交付。
- 主镜像内置 CPA runtime，Docker 首次使用不再需要单独启动或配置第二个 CPA 容器。
- Pool 成为唯一的外部网关访问边界。
- Pool 详情页新增 Codex CLI 配置流，可生成可合并到 `~/.codex/config.toml` 的配置。
- 增强 CPA / Codex 账号状态展示，包括登录态、ChatGPT 订阅过期时间和额度刷新。

### 一体化 Docker 镜像

- 内置 CPA 固定为：
  `eceasy/cli-proxy-api:v6.9.41@sha256:27a8090de418fd5ef96fae91ba6ba8579874806d573c5de3f8d13a1a4fe5ee91`。
- 运行时镜像通过同一个 entrypoint 启动 Lune 和内置 CPA。
- CPA 配置在容器启动时根据 Lune 环境变量自动生成。
- 内置 CPA 日志统一加上 `[cpa]` 前缀，便于在单容器日志里区分来源。
- 默认 Docker 持久化目录收敛为一个挂载点：`/app/data`。
- SQLite 数据库位于 `/app/data/lune.db`。
- CPA 凭据文件位于 `/app/data/cpa-auth`。
- 网关大请求重放临时文件位于 `/app/data/tmp`。
- Docker Compose 默认不再暴露 CPA 端口 `8317`；Lune 在容器内部通过 `http://127.0.0.1:8317` 访问 CPA。
- 启动时会把旧 Compose 默认地址 `http://cpa:8317` 的默认/托管 CPA 配置迁移到新的内置 runtime 地址。

### Pool 级访问凭证

- 移除全局 access token 产品模型。
- 新建 Pool 时会自动创建一条默认启用的 Pool token。
- 启动时会对已有 Pool 做 token 对账，确保每个 Pool 都有可用凭证。
- 网关请求现在必须使用绑定到 Pool 的 token。
- `X-Lune-Account-Id` 强制路由只允许指定同一 Pool 内的账号。
- 配置导入/导出通过 Pool label 保留 token 与 Pool 的关联意图。
- Settings 和 Pool 详情页都改为围绕 Pool 上下文展示访问凭证。

### Codex CLI 配置流

- Pool 页面操作从 `Codex Setup` 改名为 `Codex CLI`。
- 删除未完成的 VS Code 配置占位。
- provider id 和环境变量名改为从 Pool 名称派生，不再使用 `pool-1` 这类数字命名。
- 生成配置的默认模型固定为 `gpt-5.5`。
- 生成的 `config.toml` 参照当前 Lune 的 Codex CLI 配置结构，包含 profile、sandbox、retry、stream timeout、tools 和 provider 设置。
- 环境变量步骤同时提供临时 `export` 命令，以及写入/更新 `~/.bashrc` 的命令。
- 写入 `~/.bashrc` 前会先移除同名旧 export，避免旧 token 残留。
- 弹窗已限制宽高，长内容在弹窗内部滚动，普通浏览器窗口可以正常显示。

### CPA 与 Codex 账号状态

- 将 CPA 凭据登录态与 ChatGPT 订阅过期时间拆开展示。
- 不再把 Codex 凭据过期误显示为账号订阅过期。
- 通过 CPA auth-files metadata 获取并存储 ChatGPT / Codex 订阅过期时间。
- 移除旧 `accounts/check` 订阅探测语义，订阅状态不再影响 CPA 登录态判断。
- 订阅 metadata 缺失时只记录独立的订阅获取错误，不再提示重新登录。
- 支持从账号卡片操作刷新 Codex 额度状态。
- 改进账号卡片和账号详情中的 Codex CPA 额度、凭据状态展示。
- 增加 CPA 凭据异常通知事件。

### 网关与运行时

- 增加大请求重放缓冲能力。
- 提高默认网关请求体上限，适配图片/文件较多的请求。
- 超过内存阈值的请求会写入临时目录，用于后续重试重放，避免完整请求体长期驻留内存。
- 更稳定地记录早期网关失败，包括请求体过大、请求体格式错误等情况。
- HTTP 请求日志包含状态码。
- 增加 `LUNE_GATEWAY_TMP_DIR` 运行时默认值。

### 管理界面

- 更新 Docker Desktop 首次使用说明和相关 UI 文案，默认按一体化镜像描述。
- Settings 中 CPA 区域改为围绕内置 runtime 展示，而不是要求用户先配置外部 CPA 服务。
- Add Account 继续聚焦账号来源和 Pool 选择，CPA 作为内置账号来源使用。
- Pool 详情页 token 处理已改进，Codex CLI 弹窗可以直接使用当前 Pool 凭证。
- 账号详情新增探测模型配置、Playground 和 Debug 信息。

### 修复

- 修复直接打开 `/admin/pools/:id` 时没有正确返回 SPA 的问题。
- 修复 Pool member account 扫描字段不完整，导致 CPA / Codex 新字段在 Pool 详情中丢失的问题。
- 修复旧数据库缺少 `request_logs.pool_id` 时 Pool stats 读取失败的问题。
- 修复 Docker release / build 中嵌入前端静态资源的收口问题。
- 修复 `web/package.json` 仍保留旧版本语义的问题；当前 web 包版本为 `0.1.3`。
- 修复文档和 UI 术语不一致的问题，统一使用 `Codex CLI`。

### 升级注意

- 默认路径只需要运行 Lune 容器，不需要再单独运行 CPA 容器。
- 持久化时挂载一个 volume 或宿主机目录到 `/app/data`。
- 不要暴露 `8317` 端口；它只用于容器内部 Lune 与内置 CPA 通信。
- 如果从旧的 Compose + CPA 双容器方案升级，请保留原 Lune 数据目录。Lune 会在合适时把默认 CPA 服务地址从 `http://cpa:8317` 迁移到 `http://127.0.0.1:8317`。
- 如果把 Lune 暴露到本机以外，请设置 `LUNE_ADMIN_TOKEN`，并配合可信反向代理、VPN、防火墙或等价网络边界。
- v0.1.3 不会在运行时替换内置 CPA 二进制；如果需要更新 CPA runtime，请升级 Lune 镜像。

### 验证

- `go test ./...`
- 在 `web/` 下执行 `npm run build`
- 通过 `./scripts/rebuild.sh` 完成 Docker 重建
- 手动确认运行中容器可以访问 `/admin` 和 `/admin/pools/:id`
