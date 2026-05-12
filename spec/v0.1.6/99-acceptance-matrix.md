# 99. 验收矩阵

## 目标

本文件只做跨问题最终核对。详细设计以各问题文档为准，避免验收项和设计规则分散后出现冲突版本。

除“本轮验证状态”和各章节“本轮已完成”明确列出的内容外，下面的验收清单仍表示最终目标或后续待验收项，不能解读为本轮全部已完成。

## 本轮验证状态

- 已通过：`go test ./...`、`npm run build`、`sh -n docker/entrypoint.sh`、`git diff --check`。
- 已通过：Docker build `lune:v0.1.6-pinning-final5`。
- 已通过：新容器 `lune-v016-final5-verify` 在 `127.0.0.1:11128` 完成 health/API 空状态 smoke test。
- 未执行：真实多账号 Codex 上游消费验证；本轮用单元测试和容器 smoke test 验证 pinning/fail-closed 逻辑。
- 保护项：旧容器 `lune-0.1.5` 保持运行，未被本轮测试改动。

## 01. CPA 账号身份与 Runtime Credential 绑定错位

本轮已完成：runtime binding 接入普通转发、pinning headers、缺失 binding fail closed、request log runtime identity、Activity/usage 可信统计、相关单元测试和容器 smoke test。

- 同一个 CPA runtime 中导入 3 个 Codex auth file 后，固定选择第一个 Lune CPA account，连续发起 10 次普通模型请求时，CPA actual credential 必须全部等于该账号对应凭据。
- 强制路由 `X-Lune-Account-Id` 指向某个 CPA account 时，CPA actual credential 必须等于该 account 对应凭据；否则请求 fail closed。
- 普通 Pool 自动路由选择第 N 个 CPA account 时，`request_logs.account_id` 与 `runtime_auth_id/runtime_auth_index` 能够一一对应。
- 当 CPA runtime 不支持 credential pinning 或 auth metadata 未就绪时，该账号不可接普通流量，错误 reason 明确。
- Activity 页面账号请求量基于已确认 runtime credential 归属；无法确认时显示不确定状态。
- 单元测试覆盖：CPA target 构建必须携带 runtime credential binding；缺失 binding 时 fail closed；日志同时保存 Lune account id 与 runtime auth id。
- 集成测试覆盖：模拟 CPA round-robin runtime，验证 Lune pinning 后不会被 CPA 默认 round-robin 打散。

## 02. 账号状态互相污染导致错误路由和错误 UI

本轮已完成：subscription 非 active 阻断、`auth_suspect` 可路由但降权、状态大小写口径对齐、Pool 可信 usage 统计和相关 router/store 测试。

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
- Pool 详情页账号卡片最多展示请求量、订阅、主问题三个 chip；订阅异常只占订阅槽位，主问题 chip 最多一个。
- 右上角 badge 使用严重程度色并只表达 `正常/降级/异常/待检查/已停用` 的组合路由摘要。
- 账号详情抽屉使用 `Overview / Playground / 诊断` 三个 tab。
- Active Pool 卡片在常见 chip 组合下高度稳定。

## 03. Streaming 失败与 Activity 记账不准确

本轮已完成：streaming outcome、retryable health impact、Activity flow 和 confirmed binding 统计口径已纳入回归测试。

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
- Activity flow 展示 `Pool -> Account -> Model`。
- failed routed requests 默认纳入 flow。
- 没有 selected account 的 rows 被排除或归入明确 `Unrouted` 语义。
- CPA actual credential 未确认时，Activity 不把账号级统计表述成精确实际消耗。

## 04. CPA 凭据生命周期、重复账号、删除与重登

本轮已完成：普通模型转发复用 runtime auth metadata，request log 可审计 runtime auth id/index；删除 auth file 语义和 UI 清理入口仍是后续项。

- Device Code 登录同一个已存在 CPA account key 时，更新现有账号，不创建重复账号。
- 删除 CPA 账号后重新登录同一个 account key，embedded CPA runtime 使用新 token。
- 重新登录后的首次 quota refresh 不继续使用旧 auth index。
- 同名 auth file 被覆盖后，`resolveAuthMetadata` 能检测 runtime metadata 与磁盘 metadata 不一致并触发 reload。
- `syncCpaMetadata` 不会用旧 `last_refresh` 覆盖更新后的 DB 登录状态。
- batch import 遇到已存在 CPA account key 时跳过或更新，不创建重复账号。
- quota HTTP 401 不会在普通模型请求仍健康时单独把账号展示为“需要重新登录”。
- 重复 CPA account key 出现在数据库中时，启动检查能给出明确诊断。
- 删除账号时，用户能明确知道 auth file 是删除还是保留；runtime 状态与该选择一致。

## 05. 部署、日志、Health、诊断能力不足

本轮已完成：内置 CPA 固定为 `v7.0.2-lune.1` 源码构建，Dockerfile/release build args 同步，entrypoint 监督 CPA 子进程，容器构建和 health/API 空状态 smoke test 通过。

- 待后续：Docker 部署默认有 healthcheck、日志轮转、合理停止宽限期，并且运行配置与文档一致。
- 已完成：Dockerfile 和 release workflow 固定内置 CPA 为 `v7.0.2-lune.1`；容器构建已通过，`LUNE_EMBEDDED_CPA_VERSION` 与补丁版本一致。
- 部分完成：entrypoint 生成配置和 health/API 空状态已通过容器 smoke test；management API、provider endpoint、reload signal 仍待容器级实测。
- 部分完成：CPA 子进程异常退出会触发 entrypoint 停止 Lune 并让容器失败；异常退出注入测试和 `/readyz` 增强仍待后续。
- 待后续：过去 24 小时同一账号同一错误重复 1,000 次时，SQLite 和 stdout 不线性写入 1,000 条等价错误。
- 待后续：管理端暴露在内网或 `0.0.0.0` 时，不能仅凭 private IP 绕过 admin token。
- 部分完成：错误诊断已避免记录凭据和完整请求体；深度调试开关和系统化安全审计仍待后续。
- 待后续：诊断页能展示 DB/WAL 大小、日志策略、运行版本、CPA 版本和 readiness 失败原因。
