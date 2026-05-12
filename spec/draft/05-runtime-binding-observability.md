# 05. Runtime Binding 观测增强

状态：draft

来源：从 `spec/v0.1.6/01-cpa-runtime-identity-binding.md` 移出的后续增强项。

本草案不属于当前 v0.1.6 交付范围。v0.1.6 只要求普通 CPA 转发能够 fail closed、携带 pinned runtime credential、记录 Lune 侧 runtime binding 字段，并让 Activity/Usage 不把未确认 binding 当作可信账号统计。

## 问题

v0.1.6 已解决“Lune 选择账号”和“CPA runtime 实际使用凭据”错位的主问题，但后续排障仍可以更精确：

- Lune 当前能记录自己要求 CPA pin 到哪个 runtime auth id / auth index。
- 更理想的是 CPA runtime 在响应中安全回传“实际选中的 auth id / auth index”，用于确认 pinning 是否真的生效。
- 现有错误 reason 仍分散在不同路径，后续可以形成稳定的 normalized reason taxonomy，便于 Activity 聚合、诊断页展示和日志检索。

## 改进策略

### Selected auth 回传

后续可让 CPA runtime 在 provider response 中回传安全诊断 header，例如：

- `X-CLIProxyAPI-Selected-Auth-Id`
- `X-CLIProxyAPI-Selected-Auth-Index`

Lune 记录时应区分：

- pinned auth：Lune 请求 CPA 使用的 auth id / auth index。
- selected auth：CPA runtime 实际声明使用的 auth id / auth index。

如果 selected auth 缺失，应保持未知状态，不得把缺失字段当作 confirmed actual credential。

### Normalized reason taxonomy

后续可把 runtime binding、CPA runtime、pinning 和上游安全错误统一成稳定 reason code，例如：

- `runtime_auth_binding_unavailable`
- `provider_pinning_unsupported`
- `auth_index_pending`
- `runtime_metadata_mismatch`
- `cpa_management_unreachable`
- `selected_auth_mismatch`
- `selected_auth_missing`

reason code 应稳定、短小、可聚合；面向用户的文案可以独立本地化或改写。

## UI 表现

Activity 和诊断页可以展示：

- pinned auth id / index。
- selected auth id / index。
- 是否匹配。
- normalized reason。
- 安全截断后的错误摘要。

当 pinned 和 selected 不一致时，应显示绑定不可信，并避免把请求量或额度消耗归为精确账号事实。

## 日志与诊断

日志可以新增字段：

- `pinned_runtime_auth_id`
- `pinned_runtime_auth_index`
- `selected_runtime_auth_id`
- `selected_runtime_auth_index`
- `runtime_binding_reason_code`

不得记录 token、完整 prompt、完整 request body 或完整 auth file。selected auth 字段也应使用 CPA runtime 已脱敏或不可逆的安全标识。

## 测试与验收

- CPA 回传 selected auth 与 pinned auth 一致时，Activity 标记 binding confirmed。
- CPA 回传 selected auth 与 pinned auth 不一致时，Activity 标记 binding mismatch，账号级统计降级。
- CPA 未回传 selected auth 时，Activity 显示 selected auth unknown，不把它当作 actual credential confirmed。
- normalized reason code 在 gateway、request log、Activity API 和 UI 中保持一致。
- reason code 可用于错误折叠和趋势统计。

## 待解决问题

- selected auth header 名称是否应由 CLIProxyAPI upstream 正式定义。
- selected auth id / index 是否需要同时支持，还是只保留一个稳定安全标识。
- normalized reason code 的完整枚举、版本策略和兼容策略。
