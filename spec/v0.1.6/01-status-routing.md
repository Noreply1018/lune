# 01. 状态机与路由规则

## 目标

把账号“能否接普通流量”的判断从单一健康字段，改成多维状态组合：

- `credential_status`：凭据、登录态、CPA runtime 凭据可用性。
- `quota_status`：额度是否可用，以及 quota 接口是否能成功刷新。
- `subscription_status`：订阅、套餐、Free/过期状态。
- `serving_status`：真实模型生成请求的短期服务能力与熔断状态。

这四类状态必须独立写入、独立展示，再由路由层组合成 routability。任何辅助探测接口都不能越权覆盖其他状态。

## 问题根源

v0.1.5 把多种来源的异常写进相同或相近字段：

- quota `wham/usage` 返回 `401/403`，会被当成 CPA 登录态失效。
- 后台 quota 刷新失败可能把账号写成 `needs_login`，而网关模型请求成功又把状态写回 `ok`，导致 UI 抖动。
- `/v1/models` discovery 成功被误用为账号整体 healthy，但它不能证明生成请求、quota、subscription 都可用。
- `status=healthy`、`cpa_credential_status=needs_login`、过期订阅、额度耗尽可以同时存在，路由却仍可能选择该账号。
- 宽泛的 `401/403/unauthorized/access denied/expired token` 文本匹配，会把额度、订阅、权限、上游临时拒绝误判为“需要重新登录”。

## 解决的问题

- 防止 quota 辅助接口单独决定账号是否需要重登。
- 防止 discovery 健康覆盖 serving 熔断。
- 防止一次模型调用成功清除 quota blocked 或 subscription expired。
- 让 UI 能展示“额度查询失败”“订阅刷新失败”“CPA runtime 异常”“需要重新登录”等不同原因。
- 让路由按真实可生成能力和明确阻断状态选择账号，而不是按一个过载的健康字段。

## 可信度顺序

CPA 多账号场景下，可信度必须带前提：

1. 已确认 runtime credential 归属的模型生成请求。
2. quota/subscription 等辅助探测。
3. discovery `/models`。

如果 runtime binding 未确认，模型调用结果只能写 request log，不能把某个 Lune CPA account 标记为 `serving_status=healthy` 或 `credential_status=ok`。否则账号 A 的日志可能被账号 B 的 CPA auth file 成功调用污染。

## 模型调用写入规则

### 生成请求 200

前提：已确认 runtime credential 归属。

- 写入 `serving_status=healthy`。
- 清空 serving 维度的短期失败计数：`failure_count=0`。
- 将 `credential_status=auth_suspect` 或类似辅助探测可疑状态降回 `ok`。
- 不得清除或改写任何 quota 维度状态，包括 `blocked`、`error`、`pending`、`unknown`。
- 不得清除或改写任何 subscription 维度状态，包括 `expired`、`free`、`error`、`pending`、`unknown`。
- 如果模型调用成功与 quota/subscription 阻断同时存在，说明“生成能力正常，但辅助状态仍需独立处理”；不能用业务成功反向污染额度或订阅状态。

### 生成请求 401/403

不能只看 HTTP code，必须先判断错误归属：

- Lune 到 CPA 的 service API key 错误：写 `cpa_service.status=error` 或 runtime/service error，不写 account `needs_login`。
- CPA 到 ChatGPT/Codex 上游凭据失效：写 account `credential_status=needs_login`。
- OpenAI-compatible 直连 API key 错误：写 account `credential_status=needs_login` 或 account error，按 source kind 归类。

只有确认是账号上游凭据问题时，才把 `credential_status` 写成 `needs_login`。

### 生成请求 5xx/EOF/timeout

- 写入 `serving_status=cooldown`。
- 更新 `last_failure_at`、`cooldown_until`、`failure_count`。
- 不写 `credential_status=needs_login`。
- 不改变 quota/subscription 阻断状态。

## Quota 写入规则

quota 探测只能影响 quota 状态：

| quota 结果 | 写入 |
| --- | --- |
| `wham/usage` 200 且 `allowed=true` | `quota_status=ok` |
| `wham/usage` 200 但 `allowed=false` 或 `limit_reached=true` | `quota_status=blocked` |
| `wham/usage` 401/403 | `quota_status=error`，记录 normalized reason，不直接写 `needs_login` |
| quota 失败 + 后续模型调用也认证失败 | 升级 `credential_status=needs_login` |
| quota 失败 + 后续模型调用 200 | `credential_status=ok`，`quota_status` 仍为 `error` |

quota `error` 代表“额度接口不可用或不可信”，不是“账号不能生成”。

## Quota freshness 规则

`quota_status=error` 可以继续路由，但必须考虑最近成功快照：

- `quota_status=ok`：可路由。
- `quota_status=blocked`：不可路由。
- `quota_status=error` 且最近成功 quota 快照新鲜、`allowed=true`：可路由，UI 显示“额度查询失败，使用最近成功快照”。
- `quota_status=error` 且没有成功快照或快照过期：默认可路由但降权，UI 显示“额度未知”。如未来提供保守配置，可改为阻断。

默认选择“可路由但降权”，因为真实模型调用优先于辅助观测接口。

这是相对旧表格的有意调整：旧版 2026-05-09 运行审计更保守，要求 quota stale 后阻断；本次按新增原则改为“quota 接口只负责额度展示和明确额度阻断，不单独决定账号是否能生成”。只有 `quota_status=blocked`、或 quota 失败后模型调用也确认认证失败时，才阻断或升级到 credential 问题。

## Subscription 写入规则

subscription 探测只能影响 subscription 状态：

| subscription 结果 | 写入 |
| --- | --- |
| 明确 active / paid / plus / pro 且未过期 | `subscription_status=active` |
| 明确过期 | `subscription_status=expired` |
| 明确 Free / 无可用订阅 | `subscription_status=free` |
| 正在刷新且没有可用成功快照 | `subscription_status=pending` |
| subscription 接口 401/403/5xx/timeout 或 metadata 解析失败 | `subscription_status=error`，记录 normalized reason，不直接写 `needs_login` |
| 无历史数据或迁移后未知 | `subscription_status=unknown` |

subscription `401/403` 不能单独写 `credential_status=needs_login`。只有 refresh token 明确失效、auth file 缺失/损坏、CPA runtime 明确要求重新认证，或后续模型调用也确认账号上游认证失败时，才升级为 `needs_login`。

## Credential 状态路由

建议状态：

| 状态 | 普通流量 | UI 展示 |
| --- | --- | --- |
| `ok` | 正常路由 | 凭据正常 |
| `auth_suspect` | 可路由但降权 | 鉴权可疑，优先使用其他账号 |
| `needs_login` | 不可路由 | 需要重新登录 |
| `refresh_failed` | 不可路由 | 凭据刷新失败 |
| `runtime_pending` | 不可路由 | CPA runtime 初始化中 |
| `runtime_error` | 不可路由 | CPA runtime 异常 |
| `unknown` | 不可路由 | 凭据状态未知 |

`auth_suspect` 表达“辅助探测失败，但还没有真实业务调用认证失败”。默认可路由但降权：优先选择其他 `ok` 账号，只有没有更好账号时才使用。

诊断绕过不是普通流量能力。管理员诊断请求必须走单独入口，并带 `diagnostic=true` 标记。

## 普通路由决策

普通路由按“能否生成”决策：

- `serving_status=healthy` 且 `credential_status=ok`：正常可路由。
- `credential_status=auth_suspect`：可路由但降权。
- `credential_status=needs_login/refresh_failed/runtime_pending/runtime_error/unknown`：不可路由。
- `quota_status=error`：可路由但显示额度未知，是否降权取决于 freshness。
- `quota_status=blocked`：不可路由。
- `subscription_status=active`：可路由。
- `subscription_status=expired/free/pending/error/unknown`：不可路由；subscription 是资格维度，不能仅凭模型调用成功覆盖。
- `serving_status=cooldown`：不可路由，直到冷却结束。
- runtime credential binding 未确认：CPA 普通流量必须 fail closed，不能静默转发到 provider 级 round-robin。

强制账号路由 `X-Lune-Account-Id` 也不能绕过不可接普通流量状态。管理员诊断请求可以走单独入口绕过，但必须标记 `diagnostic=true`，且不得更新普通路由健康。

## UI 文案规则

UI 不应把 quota 401/403 直接显示成红色“请重登”。

建议展示：

- quota 失败但最近模型调用成功：`额度查询失败`。
- 详情：`最近模型调用成功，quota 接口返回 token_invalidated` 或归一化 reason。
- 连续模型调用也 401/403 且确认是账号上游凭据问题：切换为 `需要重新登录`。
- auth file 缺失/损坏、refresh token 明确失效、CPA 明确要求重新认证：直接显示 `需要重新登录`。

## 已完成事项

- 增加账号级 serving 熔断状态：`serving_status`、`failure_count`、`last_failure_at`、`last_success_at`、`cooldown_until`。
- discovery health 与 serving health 拆分；`/v1/models` 成功不再直接清除网关 serving error。
- 缩窄 `needs_login` 判定，不再仅凭任意 `403` 或宽泛文本写入。
- quota `allowed=false` 或 `limit_reached=true` 时设置 `quota_status=blocked`。
- token 认证检查 `access_tokens.enabled`。
- 禁用 token 后，网关拒绝该 token 的非 `/v1/models` 请求。

## 待解决事项

- 将旧 `cpa_credential_status` 迁移为 `credential_status`，旧字段只作为兼容别名读取。
- 路由选择同时检查账号启用、Pool member 启用、`serving_status`、`credential_status`、`subscription_status`、`quota_status`。
- 补齐显式 `subscription_status=active|expired|free|unknown|pending|error`。
- quota 缓存补齐 `quota_last_attempt_at`、`quota_last_success_at`、`quota_last_error`。
- 将 `codex_quota_fetch_interval` 接入 settings response、前端类型、Settings 页面控件和 stale 计算。
- 增加状态写入来源、前值、后值、时间和单调版本语义。

## 验收标准

- quota `401/403` 且模型调用仍成功时，UI 显示“额度查询失败”，不显示“需要重新登录”。
- quota `allowed=false` 或 `limit_reached=true` 时，普通路由跳过该账号。
- subscription 接口 `401/403` 只写 `subscription_status=error`，不单独写 `credential_status=needs_login`。
- `subscription_status=expired/free/pending/error/unknown` 时，普通路由跳过该账号。
- 生成请求 `200` 不会清除或改写任何 quota/subscription 状态。
- 生成请求 `5xx/EOF/timeout` 进入 `serving_status=cooldown`，后续独立请求在冷却期内绕过该账号。
- 生成请求 `401/403` 只有确认是账号上游凭据问题时，才写 `credential_status=needs_login`。
- CPA runtime credential binding 未确认时，不能把模型调用结果用于更新某个 CPA account 的健康状态。
