# 08. 验收与测试清单

## 目标

把 v0.1.6 的跨主题验收集中到一处，便于实现后逐项核对。详细设计以各主题文档为准，本文件只保留测试和验收粒度。

## 状态机与路由

- quota `401/403` 且模型调用仍成功时，UI 显示“额度查询失败”，不显示“需要重新登录”。
- quota `allowed=false` 或 `limit_reached=true` 时，普通路由跳过该账号。
- subscription 接口 `401/403` 只写 `subscription_status=error`，不单独写 `credential_status=needs_login`。
- `subscription_status=expired/free/pending/error/unknown` 时，普通路由跳过该账号。
- 生成请求 `200` 不会清除或改写任何 quota/subscription 状态。
- 生成请求 `5xx/EOF/timeout` 进入 `serving_status=cooldown`，后续独立请求在冷却期内绕过该账号。
- 生成请求 `401/403` 只有确认是账号上游凭据问题时，才写 `credential_status=needs_login`。
- `auth_suspect` 默认可路由但降权，优先选择其他 `credential_status=ok` 账号。
- `credential_status=needs_login/refresh_failed/runtime_pending/runtime_error/unknown` 时，普通路由跳过该账号。
- CPA runtime credential binding 未确认时，不能把模型调用结果用于更新某个 CPA account 的健康状态。
- 强制账号路由不能绕过 `credential_status=needs_login/refresh_failed/runtime_pending/runtime_error/unknown`、`quota_status=blocked`、`subscription_status=expired/free/pending/error/unknown`、`serving_status=cooldown` 等不可接普通流量状态。
- 管理员诊断请求如需绕过普通路由限制，必须走单独入口并标记 `diagnostic=true`。

## CPA 凭据生命周期

- Device Code 登录同一个已存在 CPA account key 时，更新现有账号，不创建重复账号。
- 删除 CPA 账号后重新登录同一个 account key，embedded CPA runtime 使用新 token。
- 重新登录后的首次 quota refresh 不继续使用旧 auth index。
- 同名 auth file 被覆盖后，`resolveAuthMetadata` 能检测 runtime metadata 与磁盘 metadata 不一致并触发 reload。
- `syncCpaMetadata` 不会用旧 `last_refresh` 覆盖更新后的 DB 登录状态。
- batch import 遇到已存在 CPA account key 时跳过或更新，不创建重复账号。
- quota HTTP 401 不会在普通模型请求仍健康时单独把账号展示为“需要重新登录”。
- 重复 CPA account key 出现在数据库中时，迁移或启动检查能给出明确诊断。
- 删除账号时，用户能明确知道 auth file 是删除还是保留；runtime 状态与该选择一致。

## CPA runtime credential binding

- 同一个 CPA runtime 中导入 3 个 Codex auth file 后，固定选择第一个 Lune CPA account，连续发起 10 次普通模型请求时，CPA actual credential 必须全部等于该账号对应凭据。
- 强制路由 `X-Lune-Account-Id` 指向某个 CPA account 时，CPA actual credential 必须等于该 account 对应凭据；否则请求 fail closed。
- 普通 Pool 自动路由选择第 N 个 CPA account 时，`request_logs.account_id` 与 `runtime_auth_id/runtime_auth_index` 能够一一对应。
- 当 CPA runtime 不支持 credential pinning 或 auth metadata 未就绪时，该账号不可接普通流量，错误 reason 明确。
- Activity 页面账号请求量基于已确认 runtime credential 归属；无法确认时显示不确定状态。
- 单元测试覆盖：CPA target 构建必须携带 runtime credential binding；缺失 binding 时 fail closed；日志同时保存 Lune account id 与 runtime auth id。
- 集成测试覆盖：模拟 CPA round-robin runtime，验证 Lune pinning 后不会被 CPA 默认 round-robin 打散。

## Streaming 与 Activity

- Chat Completions stream 缺少 `[DONE]`，Activity 记录失败。
- Responses stream 缺少 `response.completed`，Activity 记录失败。
- Responses stream 发出 `response.failed`，Activity 记录失败并保留 upstream status。
- Responses stream 发出容量错误，Activity 保留 upstream message。
- Responses stream 发出 `[DONE]` 但没有 `response.completed`，仍记录失败。
- CPA stream 返回 HTTP 500 且 body 为 JSON error，Activity 记录提取后的上游 message。
- CPA stream 第一 Pool account 返回 HTTP 500 后，后续独立请求能避开该账号。
- streaming retryable status 即使发生在单次或最终 attempt，也记录 account health impact。
- SSE line 超过旧 1MB scanner limit，Activity 记录 stream read error。
- downstream write 失败后，Activity 记录失败。
- 长流超过 gateway timeout，Activity 记录 timeout 相关失败。
- stream bytes 已写出后不 retry，但仍更新 request log 和 serving cooldown。

## Activity Flow 与 UI

- Activity flow 展示 `Pool -> Account -> Model`。
- failed routed requests 默认纳入 flow。
- 没有 selected account 的 rows 被排除或归入明确 `Unrouted` 语义。
- CPA actual credential 未确认时，Activity 不把账号级统计表述成精确实际消耗。
- quota 失败但最近模型调用成功时，UI 显示“额度查询失败”，详情展示最近模型调用成功和 quota 失败 reason。
- serving cooldown 显示冷却信息，不表述成登录失败。
- subscription expired/free/pending/error/unknown 与 quota blocked 分开展示。
- Active Pool 卡片在常见 chip 组合下高度稳定。
- 删除 CPA 账号前，用户能明确知道 auth file 会被删除还是保留。

## 轻量 VPS 运行时

- native bundle 能在无 Docker 机器上安装 Lune，并支持 embedded CPA。
- `LUNE_RESOURCE_PROFILE=low` 改变默认值，但不覆盖用户显式配置。
- SQLite connection pool limits 已应用。
- low-resource profile 下 health-check concurrency 和 interval 更低。
- low-resource profile 下 request body memory threshold 更低。
- low-resource profile 下默认 log verbosity 更低。
- 文档明确 native/systemd 是 low-resource 推荐路径，Docker 是 convenience path。
- 文档包含 `GOMEMLIMIT`、`GOGC` 和内存目标。
- 小型非流式 response 保持 usage parsing。
- 大型非流式 response 不全量驻留内存。
- upstream failure before response headers 仍可 retry。
- downstream 已写出后不 retry，并准确记录 Activity。

## External CPA advanced mode

- Docker 和 native bundle 默认 embedded CPA。
- external CPA 不需要 patch internal files 即可配置。
- Settings 连接外部 CPA 时显示 `external`。
- 测试覆盖 embedded vs external runtime-mode detection/config parsing。
- 文档说明：多个 Lune CPA account 共用一个 external CPA runtime 时，runtime 必须支持 per-request credential pinning；否则账号统计和额度归因不可信。

## 运维与观测

- Docker 部署默认有 healthcheck、日志轮转、合理停止宽限期，并且运行配置与文档一致。
- 如果 CPA 子进程异常退出，容器退出或 `/readyz` 变为 not ready。
- 过去 24 小时同一账号同一错误重复 1,000 次时，SQLite 和 stdout 不线性写入 1,000 条等价错误。
- 管理端暴露在内网或 `0.0.0.0` 时，不能仅凭 private IP 绕过 admin token。
- 错误诊断不落完整 prompt/request body；深度调试必须由显式 debug 开关启用，并有风险提示。
- 诊断页能展示 DB/WAL 大小、日志策略、运行版本、CPA 版本和 readiness 失败原因。
