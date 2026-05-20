# 03. 账号真实状态诊断与展示

状态：产品方向已确认，作为 v0.2.0 发布阻塞规格。

## 问题：账号状态不能只靠单一健康检查推断

v0.1.9 及之前版本容易把多种上游状态压扁成一个粗粒度结果：`active`、`disabled`、`error` 或前端上的“不可用”。这会导致两个问题：

1. 同一个账号可能同时存在多个事实：当前可发请求、历史额度接口鉴权失败、subscription 接口正常、模型列表正常、chat completion 因额度失败。
2. UI 或调度层如果只看最终标签，就会把“封号”“额度不足”“额度接口鉴权失败但账号仍可用”“临时上游错误”混在一起。

用户已经提供过真实案例：

- `al` 账号：真实情况是账号已被封号，不能被误判成单纯额度不足或临时网络错误。
- `c` 账号：真实情况是额度不足，但账号其他能力正常，不能被误判成封号。
- 历史 spec 中已提到的额度接口鉴权失败案例：quota/history 类接口鉴权失败，但账号不一定不可用；只要真实业务请求、subscription 或 models 等证据仍表明可用，就不能把它降级成封号。

v0.2.0 必须把这些真实状态作为回归样本，后续用真实容器测试矩阵持续验证 Lune 能分清楚多种状态，而不是只给出一个不可审计的失败标签。

## 借鉴 sub2api 后的解决思路

本规格借鉴 sub2api 的核心思路：不要把某一个接口的失败直接当成账号最终状态，而是采集多路证据，再用确定性的规则归类。

Lune v0.2.0 应建立“证据层”和“判定层”分离的账号诊断模型：

| 层级 | 职责 | 示例 |
| --- | --- | --- |
| 证据层 | 记录每个探测接口的原始安全结果 | auth refresh、models、subscription、quota、chat probe、account page hint |
| 判定层 | 根据多路证据推导机器可读状态 | `banned`、`quota_exhausted`、`quota_probe_auth_failed_but_usable` |
| 展示层 | 把状态翻译成 UI 标签和运维建议 | 已封号、额度不足、额度接口异常但可用 |

任何单个 probe 的失败都只能先进入证据层。只有当判定规则满足时，才允许改变账号的最终 `diagnostic_status` 或调度可用性。

## 状态模型

账号诊断必须拆成三个不同字段，不得把一次 probe 事件直接写成最终账号状态：

| 字段 | 语义 | 示例 |
| --- | --- | --- |
| `stable_diagnostic_status` | 最近一次有充分证据支持的稳定账号状态 | `usable`、`banned`、`quota_exhausted` |
| `last_probe_status` | 最近一次诊断或真实请求观察到的事件状态 | `succeeded`、`quota_probe_auth_failed`、`transient_error` |
| `scheduler_status` | 调度层当前采用的可用性决策 | `eligible`、`ineligible`、`eligible_with_warning` |

`diagnostic_status` 如作为 API 兼容字段，只能映射到 `stable_diagnostic_status`，不能用暂态 probe 事件覆盖。

稳定账号状态必须至少能表达以下值：

| 状态 | 语义 | 调度默认行为 |
| --- | --- | --- |
| `usable` | 关键业务请求和必要元数据正常 | 可调度 |
| `banned` | 上游明确表明账号被封、账号不可访问或认证主体失效 | 不可调度 |
| `quota_exhausted` | 账号身份有效，但业务请求因额度耗尽失败 | 不可调度，等待额度恢复或人工处理 |
| `quota_probe_auth_failed_but_usable` | quota/history 类接口鉴权失败，但业务请求或其他关键证据显示账号仍可用 | 可调度，但显示诊断告警 |
| `auth_invalid` | refresh token/access token 无效，无法恢复认证 | 不可调度 |
| `unknown` | 证据不足或首次探测未完成 | 不主动判死 |

最近 probe 事件状态必须至少能表达：

| 事件状态 | 语义 |
| --- | --- |
| `succeeded` | probe 成功或真实请求成功 |
| `quota_probe_auth_failed` | quota/history/usage 类接口鉴权失败 |
| `upstream_banned_signal` | 上游返回明确封号语义 |
| `quota_exhausted_signal` | 上游返回明确额度不足语义 |
| `auth_invalid_signal` | refresh/access token 无效且无法恢复 |
| `transient_error` | 5xx、超时、网络、429、代理等暂态问题 |

状态命名是机器接口，不应直接依赖前端文案。前端可以展示更友好的中文标签，但必须保留稳定状态、最近 probe 事件、调度状态、证据摘要和最后诊断时间。

## 多路证据要求

每次账号诊断至少记录以下证据项；具体接口可按 provider 能力调整，但字段语义不能缺失：

| 证据项 | 目的 | 必须记录 |
| --- | --- | --- |
| `auth_refresh` | 判断 refresh token 是否仍可换取有效凭据 | HTTP 状态、错误码、安全摘要、时间 |
| `models_probe` | 判断账号能否读取可用模型或服务元数据 | HTTP 状态、错误码、模型摘要、时间 |
| `subscription_probe` | 判断账号主体、套餐或订阅状态 | HTTP 状态、错误码、安全摘要、时间 |
| `quota_probe` | 判断额度或 usage/history 接口状态 | HTTP 状态、错误码、额度摘要、时间 |
| `chat_probe` | 判断真实业务请求是否可用 | HTTP 状态、上游错误类型、安全摘要、时间 |
| `routing_observation` | 判断真实流量中是否出现额度/封禁/鉴权错误 | request id、错误码、账号摘要、时间 |

证据记录必须脱敏：

- 不记录 refresh token、access token、id token。
- 不记录完整请求体、完整响应体或完整 account key。
- 邮箱、账号 ID、上游 user id 必须 mask 或 hash。
- 上游错误只保留机器可读错误码和安全摘要。

## 判定规则

判定规则必须保守，优先避免把可用账号误杀。

### 封号

满足以下任一条件时，可以判定为 `banned`：

- `auth_refresh`、`subscription_probe` 或 `chat_probe` 返回明确的 account banned、account deactivated、account disabled、abuse lock、policy lock 等上游语义。
- 多个关键接口在同一诊断窗口内返回一致的账号主体不可用语义。
- 上游响应指向账号级封禁，而不是额度、模型不可用、地区限制、临时 5xx 或 quota/history 接口鉴权失败。

不得仅因为 `quota_probe` 鉴权失败就判定为 `banned`。

### 额度不足

满足以下条件时，可以判定为 `quota_exhausted`：

- 账号身份仍有效，至少一个身份或元数据接口能证明账号主体存在。
- `chat_probe` 或真实业务请求返回明确的额度不足、余额不足、usage limit、insufficient quota、billing hard limit 等语义。
- 错误不是 auth invalid、account banned、模型不存在或临时 5xx。

### 额度接口鉴权失败但可用

满足以下条件时，必须判定为 `quota_probe_auth_failed_but_usable`，不得判死账号：

- `quota_probe` 或 history/usage 类接口返回 401/403 或等价鉴权失败。
- `auth_refresh`、`models_probe`、`subscription_probe` 或 `chat_probe` 中至少一个关键证据显示账号仍可用。
- 没有任何关键证据明确表明账号封禁或 token 无效。

该状态应显示告警，提示“额度接口不可用，不能依赖 quota 数据”，但调度层默认仍可使用该账号。

### 暂态错误

网络错误、超时、上游 429、5xx、DNS、TLS、代理错误等不得直接覆盖上一稳定状态。必须记录为 `last_probe_status=transient_error`，并保留上一稳定诊断状态用于调度。

实现上应把这类事件写入 `last_probe_status=transient_error` 和 evidence。`stable_diagnostic_status` 继续保留上一稳定值；如果没有上一稳定值，则保持 `unknown`。

## 真实案例沉淀

以下案例必须进入 v0.2.0 的验收矩阵和后续容器回归测试。案例名称用于脱敏识别，不代表真实邮箱或完整账号 key。

| 案例 | 已知真实情况 | 必须采集的证据 | 期望判定 |
| --- | --- | --- | --- |
| `al` | 账号实际已被封号 | auth/subscription/chat 中至少一个明确封禁语义；quota 结果只能作为旁证 | `banned` |
| `c` | 账号实际额度不足，但账号主体和其他能力正常 | chat 或真实请求返回额度不足；models/subscription/auth 至少一个证明主体仍有效 | `quota_exhausted` |
| `quota-auth-failed-usable` | 历史额度接口鉴权失败案例，账号并非必然不可用 | quota/history 返回鉴权失败；models/subscription/chat 或真实流量证明可用 | `quota_probe_auth_failed_but_usable` |

这些案例后续必须在真实容器里跑，而不是只写单元测试：

1. 导入或配置对应账号。
2. 触发一次完整账号诊断。
3. 触发一次真实 gateway 请求或安全 chat probe。
4. 查询只读 debug 输出、DB 诊断记录、request log 和前端/API 展示。
5. 验证最终状态、证据明细、调度行为和脱敏都符合预期。

### 真实案例治理

真实案例是发布验收输入，必须可治理、可脱敏、可替换：

- 凭据只能通过本地环境变量、CI secret 或人工验收环境注入，不得提交到仓库、日志、审计包或 fixture。
- 每个真实案例必须记录脱敏 case id、采集日期、采集人或环境、当时真实状态、预期状态、可接受证据和过期时间。
- 如果真实账号状态发生漂移、凭据过期或不再可用，不能静默改用 fake upstream 让发布通过；必须由维护者确认新的真实样本，或明确记录该发布阻塞项未通过。
- fake upstream 必须沉淀为 golden fixture，用于长期 CI 回归同样的上游响应语义；它只能证明归一化规则没有回退，不能替代至少一次真实容器验收。
- 真实案例验收只要求安全证据和最终归类可复核，不要求保存完整上游响应。

## 数据结构要求

v0.2.0 必须持久化最近诊断结果，且能在请求结束后恢复证据。

建议结构：

```text
account_diagnostics
  id
  account_id
  account_key_hash
  provider
  operation_id
  started_at
  finished_at
  stable_diagnostic_status
  previous_stable_diagnostic_status
  last_probe_status
  scheduler_status
  safe_summary

account_diagnostic_evidence
  diagnostic_id
  probe_type
  stage
  http_status
  upstream_error_code
  normalized_error_code
  safe_message
  observed_at
  request_log_id
```

不要求必须按以上表名实现，但 API、debug 命令和审计包必须能表达同等信息。

## API、UI 与调度要求

- Admin API 返回账号列表时必须包含 `diagnostic_status`、`scheduler_status`、`last_diagnosed_at` 和安全摘要。
- 账号详情页必须能展开最近一次诊断证据，显示各 probe 的状态、时间和安全错误码。
- 调度层不能只看 quota probe 结果；必须读取判定层输出。
- `quota_probe_auth_failed_but_usable` 默认仍可调度，但应降低置信度或显示运维告警。
- `banned`、`auth_invalid`、`quota_exhausted` 默认不可调度，除非用户显式覆盖。
- 手动 refresh、自动 health refresh、真实请求失败回写都必须走同一套归一化状态模型。
- `diagnostic_status` 对外兼容时必须等价于 `stable_diagnostic_status`；UI 可以额外显示 `last_probe_status`，但不得把暂态事件展示成最终账号状态。

## 可审计性要求

账号诊断必须复用 `02-general-auditability.md` 的操作事件、阶段错误和关联 ID。

每次完整诊断必须产生 `account_diagnostic` operation，至少记录：

- `operation_id`
- account hash
- provider
- 每个 probe 的阶段和结果
- 最终 `stable_diagnostic_status`
- 最近 `last_probe_status`
- `scheduler_status`
- 是否改变上一稳定状态
- 安全摘要

`lune debug account <account_id>` 和 `lune debug recent-operations` 必须能恢复最近诊断证据。`lune debug collect --redact` 必须包含账号诊断摘要和证据，不得泄露凭据。

## 反例

以下行为在 v0.2.0 中不允许：

- quota/history 接口 401/403 后直接把账号标记为封号。
- chat 返回额度不足后把账号标记为封号。
- 只有 transient 5xx 或网络错误时覆盖上一稳定状态。
- 把 `transient_error` 当成最终账号状态写入 `diagnostic_status`。
- UI 只显示“不可用”，不展示机器状态和证据摘要。
- request log 里能看到错误，但账号诊断记录里无法恢复原因。
- 调度层和 UI 使用两套不一致的状态判断。
