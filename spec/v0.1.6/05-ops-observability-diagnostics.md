# 05. 部署、日志、Health、诊断能力不足

## 问题

v0.1.6 在生产排障时需要快速回答：

- 当前运行配置是否符合仓库预期。
- 哪些账号在产生重复错误。
- 失败是 credential、quota、subscription、serving、runtime 还是部署问题。
- Docker/SQLite/stdout 日志是否在无上限增长。
- CPA 子进程是否健康。

2026-05-09 的生产审计中，运行容器与仓库 compose 预期存在明显差异：

- `RestartPolicy=no`。
- 没有 Docker healthcheck。
- 端口绑定为 `0.0.0.0:11111->7788`，仓库 compose 期望 `127.0.0.1`。
- 数据卷是宿主机 bind mount，而仓库 compose 使用 named volume。
- Docker json-file 日志没有 `max-size` / `max-file`。
- CPA 与 Lune 在同一容器内以后台子进程运行，entrypoint 只等待 `lune up`，没有严格监督 CPA 子进程。

运行日志也不可控：

- 过去 24 小时 stdout 约 34,055 行。
- 错误、鉴权、额度、订阅相关关键词约 27,350 行。
- `request_logs` 中共有 12,870 条请求，其中大量 `500/503`。
- 同类错误被逐请求线性写入 SQLite 和 stdout。
- 大量 `error_message` 被归一成 `upstream error`，又缺少可诊断细节。

## 改进策略

### Docker 与 Compose

v0.1.6 生产 Docker Compose 应：

- 固定镜像 tag 或 digest，避免 `latest` 导致排障不可复现。
- 增加 `restart: unless-stopped`。
- 增加 `healthcheck`。
- 增加 `stop_grace_period: 15s` 或更高。
- 默认端口继续绑定 `127.0.0.1`。
- 如用户显式绑定 `0.0.0.0`，文档要求反向代理鉴权或防火墙保护。
- 增加 Docker json-file log rotation，例如 `max-size` 和 `max-file`。
- 明确数据卷模式：named volume 或宿主机目录二选一，并给出对应备份命令。

SQLite 备份必须覆盖：

- `lune.db`
- `lune.db-wal`
- `lune.db-shm`

或使用 SQLite 在线备份。

### 内置 CPA 版本集成

v0.1.6 应把内置 CPA 从当前 Dockerfile 固定的 `eceasy/cli-proxy-api:v6.9.41@sha256:27a8090de418fd5ef96fae91ba6ba8579874806d573c5de3f8d13a1a4fe5ee91` 升级到已核对的上游新版本，并构建 Lune 专用补丁版本：

- 上游源码版本：`router-for-me/CLIProxyAPI@v7.0.2`，commit `1fca942b9c2c5bbdf78334eb4744a098983a05e9`
- Lune 补丁版本标识：`v7.0.2-lune.1`
- 补丁文件：`third_party/cliproxyapi/v7.0.2-lune-provider-pinning.patch`
- 参考官方镜像：`eceasy/cli-proxy-api:v7.0.2@sha256:e7b19291f121d20e13a34c96ef9980d2b207229c01ae1aaf1d6c4b848a6a0f4a`
- 核对时间：`2026-05-12`

集成规则：

- Dockerfile 必须固定 `CPA_VERSION=v7.0.2`、`CPA_COMMIT=1fca942b9c2c5bbdf78334eb4744a098983a05e9` 和 `CPA_PATCH_VERSION=v7.0.2-lune.1`，从上游 tag checkout 后校验 commit，应用补丁并编译内置 CPA。
- 所有构建入口必须同步更新 CPA 固定值，包括 GitHub Actions release workflow 中传入 Docker build 的 `CPA_VERSION`、`CPA_COMMIT` 与 `CPA_PATCH_VERSION` build-arg。
- 不允许改成 `latest`，也不允许使用未固定上游 tag 的源码构建。
- Runtime 必须继续通过 `LUNE_EMBEDDED_CPA_VERSION` 暴露镜像内置 CPA 版本。
- 只有 Lune 内置补丁 CPA 才能设置 `LUNE_CPA_PROVIDER_PINNING_SUPPORTED=1`。
- 诊断页展示的 CPA 版本必须能区分 image pinned version 与实际 running version。
- release notes 必须明确本次内置 CPA 从 `v6.9.41` 升级到 `v7.0.2-lune.1`，并提醒这是基于 CPA `7.x` 的 Lune 补丁构建。
- 如果 `v7.0.2` 的 provider endpoint、management API、auth-files metadata、配置字段或启动参数与 `v6.9.41` 不兼容，Lune 必须 fail closed，并在 readiness 或诊断页给出明确原因。

最低兼容性检查：

- embedded CPA 能用 entrypoint 生成的 `config.yaml` 正常启动。
- `/health` 或等价健康接口可达。
- management key 仍可访问 auth-files metadata。
- provider API key 仍可访问 provider endpoint。
- 现有 `auth-dir`、API key、management key 配置语义不变。
- CPA reload signal 触发后，新版本 CPA 子进程能被干净停止并重启。
- 对 Codex CPA account，quota/subscription/普通模型转发相关路径不因 CPA 版本变化静默退化。

回退规则：

- 如果 `v7.0.2` 在容器集成测试中失败，不能把失败版本合入默认镜像。
- 可临时选择同系列最新 `6.x` 版本作为保守备选，但必须重新固定所有构建入口的 tag、digest/build-arg，并记录选择原因。
- 任何 CPA 版本回退都必须保持 `LUNE_EMBEDDED_CPA_VERSION` 与实际内置版本一致。

### CPA 子进程监督

当前 embedded CPA 与 Lune 同容器运行时，entrypoint 不能只等待 `lune up`。

要求：

- CPA 异常退出时，容器应退出或进入明确 unhealthy 状态。
- Lune `/readyz` 应能反映 CPA runtime 不可用。
- 诊断页展示 CPA version、runtime mode、最近健康检查时间和最近错误。

### Health endpoints

建议区分：

- `/healthz`：轻量 liveness，进程活着即可。
- `/readyz`：readiness，检查是否可接普通流量。

`/readyz` 应至少检查：

- SQLite 可访问。
- DB schema 版本正确。
- CPA runtime 状态。
- CPA management API 可用性。
- 至少一个启用 Pool。
- 至少一个启用 access token。
- 如果配置要求 CPA，则至少一个可用 CPA service。

## UI 表现

诊断页应展示：

- SQLite DB/WAL/SHM 大小。
- 最近 prune 时间。
- data retention 配置。
- Docker log policy。
- 当前运行镜像 tag/commit。
- Lune version。
- CPA version。
- CPA runtime mode：embedded 或 external。
- 容器 restart policy。
- 端口绑定提示。
- 管理端鉴权模式。
- 当前 health/readiness 状态。
- 最近 credential/quota/subscription/serving 状态迁移摘要。

Activity/Usage 页面默认分页和时间窗口应克制，避免管理页查询拖垮实例。大量错误应以趋势、聚合或折叠形式展示，而不是逐条堆满 UI。

## 日志与诊断

### SQLite request log

建议增加重复错误折叠。

聚合 key：

- time bucket。
- token。
- pool。
- account。
- model。
- status code。
- normalized error。

聚合字段：

- `count`。
- first seen time。
- last seen time。
- sample request id。
- sample safe error message。

Activity 仍应能展示错误趋势，但不应因为同一账号同一错误高频重复而线性膨胀。

### stdout access log

生产默认：

- 只记录 warn/error。
- 成功请求采样或只记录慢请求。
- 高频 admin polling 和静态资源请求不刷屏。

可通过显式 debug 开关恢复更详细日志，并提示风险。

### 安全错误信息

错误日志默认不得保存：

- 完整 prompt。
- 完整 request body。
- refresh token。
- access token。
- 完整 auth file。

可记录：

- request id。
- account id。
- account key。
- status code。
- normalized error。
- 安全截断后的 upstream message。
- 状态迁移前后值。

`request_logs.error_message` 应保留安全上游错误细节，避免所有 CPA 错误都变成 `upstream error`。默认最多保存 4KB，写入前必须移除 token、auth header、完整 prompt、完整 request body 和完整 auth file；超过上限时截断并保留 normalized reason。

### 通知去重

通知去重应覆盖 failed/dropped，不只看近期成功投递。

规则：

- 长期故障进入 dropped 后仍要有冷却期。
- health check 下一轮不应立即重新入队同一通知。
- 去重 key 应包含账号、问题类型、normalized reason。

### Activity / Usage API 保护

日志量变大后，管理页查询不能拖垮实例。

要求：

- Activity/Usage API 限制 `page_size` / `limit` 上限。
- 常用过滤字段增加索引。
- 前端分页和时间窗口默认克制。

## 测试与验收

本节是最终验收要求；具体本轮已完成范围以“已完成事项”为准，未列入已完成的项目仍需后续验证或实现。

本节对应 `99-acceptance-matrix.md` 中的 `CT-08` 到 `CT-14`。除 `CT-01` 的真实多账号 Codex 上游验证外，本节运维、诊断和日志类容器测试默认使用 fake account、mock upstream、mock CPA management/provider 或 seed 测试数据，不需要真实 Codex 账号。

`CT-08` 到 `CT-14` 都必须用新 v0.1.6 镜像启动临时容器或待发布 Compose，使用全新数据目录，不得复用或影响上一版本正在运行的容器，结束后必须删除测试容器和测试数据。

- Docker 部署默认有 healthcheck、日志轮转、合理停止宽限期，并且运行配置与文档一致。状态：已完成，见 `docker-compose.yml` / `docker-compose.prod.yml` 和 compose config 校验。
- Dockerfile 和 release workflow 固定内置 CPA 为 `v7.0.2-lune.1`，构建出的容器实际运行版本与 `LUNE_EMBEDDED_CPA_VERSION` 一致。状态：已完成，诊断页现同时展示来自镜像环境的 image pinned version 与从实际 `CLIProxyAPI version` 读取的 running version。
- entrypoint 生成配置和基础 health/API 空状态已通过容器 smoke test。
- management API、provider endpoint、reload signal 的深度行为已通过 fake 容器实测；真实 provider 上游消费由 `CT-01` 覆盖。
- 如果 CPA 子进程异常退出，容器退出或 `/readyz` 变为 not ready。状态：已完成，异常退出容器级注入测试已补。
- 过去 24 小时同一账号同一错误重复 1,000 次时，SQLite 和 stdout 不线性写入 1,000 条等价错误。状态：已完成，重复错误折叠和 stdout 降噪已通过 fake 容器 smoke test。
- 错误诊断不落完整 prompt/request body；深度调试必须由显式 debug 开关启用，并有风险提示。状态：已完成，`error_message` 安全截断与脱敏已落地。
- 诊断页能展示 DB/WAL 大小、日志策略、运行版本、CPA 版本和 readiness 失败原因。状态：已完成，DB/WAL/SHM 大小、运行版本、CPA 版本、readiness 状态和失败原因均已补齐。

容器验收至少覆盖：

- `CT-08`：用 fake CPA management/provider 验证 management auth-files metadata、provider endpoint pinned headers 和 reload signal；真实 provider 上游消费不在本项完成，由 `CT-01` 覆盖。
- `CT-09`：注入 CPA 子进程退出或 management API 不可用，确认容器失败或 `/readyz` 返回 not ready，并返回 CPA 不可用原因；本轮 `/readyz` 已补 CPA runtime 失效时的 503 路径，并通过 disabled CPA service 容器验证。
- `CT-10`：用待发布 Compose 启动容器，检查镜像 tag/digest、healthcheck、restart、stop grace period、端口绑定、volume 和 Docker json-file log rotation。
- `CT-11`：mock upstream 返回超长错误、伪 token、伪 auth header、伪 prompt/body，确认 `request_logs.error_message` 最多 4KB 且敏感信息被移除；Activity/Usage API 只展示安全截断摘要。
- `CT-12`：连续制造同一账号同一错误 1,000 次，确认 SQLite/request log 不线性写入 1,000 条等价错误，stdout 不刷屏；`failed/dropped` 通知按窗口去重由单测覆盖。
- `CT-13`：seed 大量 Activity/Usage 数据，确认 API 强制 `page_size` / `limit` 上限，常用过滤字段具备索引或可接受查询计划，前端默认分页和时间窗口不拖垮实例；本轮 6000 条 fake request log 容器复测已确认 `page_size=500` 被压到 `200`，account/source/token/model 常用过滤条件走新增索引或 SQLite multi-index plan。
- `CT-14`：构造 DB/WAL、日志策略、版本差异和 readiness failure，确认诊断页和账号 `诊断` tab 展示可排障信息；本轮已补 `DB/WAL/SHM`、image pinned CPA version、running CPA version、readiness 状态、readiness failure reason 和账号诊断 tab 的 Runtime Binding、credential、quota、subscription、serving 状态摘要。

验收记录应保留：镜像 tag 或 digest、Compose/容器启动命令、端口和数据目录、mock upstream/CPA 配置或 seed 数据规模、关键 API/readyz/diagnostic 响应摘要、日志和 DB/WAL 大小摘要、UI 截图或 Playwright 断言、清理测试容器和测试数据的命令结果。

## 已完成事项

- token 认证检查 `access_tokens.enabled`。
- 禁用 token 后，网关拒绝该 token 的非 `/v1/models` 请求。
- streaming 和 retryable upstream error 已开始保留更具体上游 message。
- Dockerfile 已固定 `CPA_VERSION=v7.0.2`、`CPA_COMMIT=1fca942b9c2c5bbdf78334eb4744a098983a05e9`、`CPA_PATCH_VERSION=v7.0.2-lune.1`，并改为 checkout upstream tag、校验 commit、应用 Lune patch 后本地构建内置 CPA。
- GitHub Actions release workflow 已同步 CPA build args，避免本地 Dockerfile 与发布镜像使用不同 CPA 版本。
- Lune provider pinning patch 已落在 `third_party/cliproxyapi/v7.0.2-lune-provider-pinning.patch`，用于 embedded CPA per-request auth pinning。
- entrypoint 已为 embedded CPA 设置 `LUNE_CPA_PROVIDER_PINNING_SUPPORTED=1`，并在 CPA 子进程异常退出时让容器失败。
- `LUNE_EMBEDDED_CPA_VERSION` 已更新为 `v7.0.2-lune.1`，便于运行时诊断区分 Lune patch 版 CPA。
- 已执行 `sh -n docker/entrypoint.sh`、Docker build 和新容器 smoke test；验证 health/API 空状态正常，旧版 `lune-0.1.5` 容器未被停止或修改。

## 待解决事项

- 暂无。
