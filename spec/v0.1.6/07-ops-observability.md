# 07. 运维与观测

## 目标

让 v0.1.6 在生产排障时能快速回答：

- 当前运行配置是否符合仓库预期。
- 哪些账号在产生重复错误。
- 失败是 credential、quota、subscription、serving、runtime 还是部署问题。
- Docker/SQLite/stdout 日志是否在无上限增长。
- CPA 子进程是否健康。

## 问题根源

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

## 解决的问题

- 避免生产部署配置不可复现。
- 防止 Docker 日志无上限增长。
- 防止同一错误重复 1,000 次就写入 1,000 条等价日志。
- 让 readiness 能覆盖 CPA runtime、DB schema、账号/Pool/token 最小可用性。
- 防止内网或反向代理环境下管理端鉴权被 private IP 误信任绕过。
- 提供能快速排障的诊断页。

## Docker 与 Compose

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

## 管理端鉴权

管理端鉴权不得把所有 private IP 都视为可信本机来源。

问题场景：

- 内网部署。
- Docker bridge。
- 反向代理。
- 用户把端口绑定到 `0.0.0.0`。

规则：

- 默认要求 admin token。
- 如果支持 trusted proxy，必须显式配置。
- 文档必须说明把管理端暴露到公网或内网时的鉴权要求。

## CPA 子进程监督

当前 embedded CPA 与 Lune 同容器运行时，entrypoint 不能只等待 `lune up`。

要求：

- CPA 异常退出时，容器应退出或进入明确 unhealthy 状态。
- Lune `/readyz` 应能反映 CPA runtime 不可用。
- 诊断页展示 CPA version、runtime mode、最近健康检查时间和最近错误。

## Health endpoints

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

## 请求日志降噪

### SQLite request log

建议增加重复错误折叠：

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

## 安全错误信息

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

`request_logs.error_message` 应保留安全上游错误细节，避免所有 CPA 错误都变成 `upstream error`。

## 通知去重

通知去重应覆盖 failed/dropped，不只看近期成功投递。

规则：

- 长期故障进入 dropped 后仍要有冷却期。
- health check 下一轮不应立即重新入队同一通知。
- 去重 key 应包含账号、问题类型、normalized reason。

## Activity / Usage API 保护

日志量变大后，管理页查询不能拖垮实例。

要求：

- Activity/Usage API 限制 `page_size` / `limit` 上限。
- 常用过滤字段增加索引。
- 前端分页和时间窗口默认克制。

## 诊断页

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

## 已完成事项

- token 认证检查 `access_tokens.enabled`。
- 禁用 token 后，网关拒绝该 token 的非 `/v1/models` 请求。
- streaming 和 retryable upstream error 已开始保留更具体上游 message。

## 待解决事项

- Docker Compose 固定 tag/digest、healthcheck、restart、stop grace period。
- Docker json-file log rotation。
- 管理端 trusted private IP 策略收紧。
- CPA 子进程监督。
- `/readyz` 增强。
- request log 重复错误折叠。
- stdout access log 降噪。
- 通知去重覆盖 failed/dropped。
- Activity/Usage API 上限和索引。
- 诊断页。

## 验收标准

- Docker 部署默认有 healthcheck、日志轮转、合理停止宽限期，并且运行配置与文档一致。
- 如果 CPA 子进程异常退出，容器退出或 `/readyz` 变为 not ready。
- 过去 24 小时同一账号同一错误重复 1,000 次时，SQLite 和 stdout 不线性写入 1,000 条等价错误。
- 管理端暴露在内网或 `0.0.0.0` 时，不能仅凭 private IP 绕过 admin token。
- 错误诊断不落完整 prompt/request body；深度调试必须由显式 debug 开关启用，并有风险提示。
- 诊断页能展示 DB/WAL 大小、日志策略、运行版本、CPA 版本和 readiness 失败原因。
