# 03. CPA Runtime Credential 绑定

## 目标

普通 CPA 网关请求必须绑定到 Lune account 对应的 CPA runtime credential。只有确认了实际执行的 CPA auth file，模型调用结果才能用于更新该账号的状态、请求量和额度归因。

核心规则：

> 已确认 runtime credential 归属的模型调用，才是最高可信信号。

如果 runtime credential 归属无法确认，Lune 不能声称支持同一个 CPA runtime 下多个 Codex account 的准确账号级路由与统计。

## 问题根源

2026-05-11 的生产审计发现，Lune 前端几乎所有请求量都显示在 Pool 第一位 Codex CPA 账号上，但后续其他 Codex 账号的远端额度持续减少。

证据：

- `request_logs.account_id=1` 占绝大多数请求。
- account `id=2/3/4/5` 的请求日志很少。
- 但后续账号的 `codex_quota_json` 显示额度真实消耗。
- 多数日志 `attempt_count=1`，说明主要不是 Lune 跨账号重试导致的错记。

根因是 v0.1.5 普通 CPA 网关链路存在两层独立调度：

```text
client
  -> Lune router 选择 accounts.id
  -> Lune 记录 request_logs.account_id
  -> /api/provider/codex/v1
  -> CPA runtime 自行选择 Codex auth file
```

Lune 转发到 CPA 时只使用 provider 级地址：

```text
{cpa_base_url}/api/provider/{cpa_provider}/v1
```

多个 Codex CPA account 的 `cpa_provider` 都是 `codex`，所以请求 URL 完全相同。转发请求没有携带 `cpa_account_key`、CPA `auth_index`、email、OpenAI account id 或任何能 pin 到某个 auth file 的字段。

CPA provider endpoint 在未绑定且未覆盖策略时，会落到自身默认 selector，在可用 Codex auth file 中选择凭据。这不是 Lune 的 Pool 顺序，也不是 Lune 的 `account_id`。

## 解决的问题

- 防止 Lune 把请求日志记到账号 A，但 CPA 实际使用账号 B。
- 防止模型调用成功错误修复另一个账号的 `serving_status` 或 `credential_status`。
- 防止 Activity / Pool 详情展示看似精确但实际不可信的账号请求量。
- 防止强制路由 `X-Lune-Account-Id` 静默落到 CPA round-robin。
- 让 quota 消耗、请求日志、路由选择和 runtime 凭据可审计地对应。

## 代码证据归档

v0.1.5 Lune 侧问题：

- `internal/router/router.go` 返回 `ResolvedRoute{PoolID, TargetModel, AccountID, Account}`，选择的是 Lune account row，不是 CPA auth file。
- `internal/gateway/handler.go` 的 `resolveTarget` 对 CPA 账号只拼 `/api/provider/{provider}/v1`。
- `internal/gateway/proxy.go` 的 `UpstreamTarget` 只有 `BaseURL/APIKey/AccountID`，没有 `CpaAccountKey`、`AuthIndex` 或 runtime credential id。
- 转发时只替换 `Authorization: Bearer <CPA API key>`。
- `X-Lune-Account-Id` 只被 Lune 用作强制路由输入；即使被转发到 CPA，也不是 CPA 可识别的 credential pinning 参数。
- `logRequest` 从 `resolved.AccountID` 写入 `request_logs.account_id`。
- `request_logs` 没有保存实际 CPA auth id / auth index。

v0.1.5 embedded CPA 配置问题：

- Dockerfile 固定内置 CPA 为 `eceasy/cli-proxy-api:v6.9.41`。
- Lune entrypoint 生成的 CPA `config.yaml` 只包含 `port`、`auth-dir`、management key 和 API key。
- 配置没有账号绑定能力，也没有 `routing.strategy` 覆盖。
- CPA 的 `auth_index` 主要用于 management API `/v0/management/api-call`，不是普通 provider 转发路径的公开绑定参数。

## 设计规则

### 普通模型转发必须使用已校验 runtime metadata

`resolveCpaRuntime` 获取到的 runtime auth metadata 不能只服务于 quota/subscription 刷新；普通模型转发也必须使用同一份已校验 metadata。

转发目标至少应携带：

- Lune account id。
- `cpa_account_key`。
- runtime auth id 或 `auth_index`。
- OpenAI account id 或其他可审计 runtime 身份。
- binding freshness/version。

### 缺失 binding 时 fail closed

当 CPA runtime 不支持 credential pinning，或 auth metadata 未就绪时：

- 该 CPA account 不可接普通流量。
- 强制路由也必须失败，不能静默使用 provider 级 round-robin。
- 错误 reason 应明确为 `runtime_auth_binding_unavailable` 或等价值。
- 管理员诊断可以走独立入口，但必须标记为 diagnostic，且不得更新普通路由健康。

### 请求日志必须记录两类身份

`request_logs` 应区分：

- `account_id`：Lune router 选择的账号行。
- `runtime_auth_id` / `runtime_auth_index`：CPA 实际执行的凭据。

如果 CPA 没有回传实际凭据，应写 `unknown` 或空值，并在 Activity 中显示归属不确定，不能把请求量当作精确账号统计。

### Activity 展示必须按可信度降级

Activity / Pool 详情页应区分：

- Lune routed account。
- CPA actual credential。

只有 runtime credential 归属确认时，账号请求量才可作为精确统计。无法确认时，应显示不确定状态或降级提示，不应把所有请求量归到 Lune 首选账号。

### CPA 能力适配

如果当前 CPA provider endpoint 不支持公开 pinning 参数，应优先推动或适配 CPA 的 pinned auth 能力。可选方向：

- CPA provider endpoint 支持 per-request auth index 或 auth id。
- Lune 通过 CPA management api-call 或等价接口执行 pinned 请求。
- Lune 对 embedded CPA 写入明确 routing 配置，但这只能解决策略一致性，不能替代 per-account pinning。

不允许用“provider 名相同但期待 CPA 默认选择正确账号”的方式实现多账号路由。

## 与重试问题的边界

Lune 自身也有跨账号重试：非强制账号请求遇到网络错误、HTTP 429 或 5xx 时，可能排除当前账号并选择下一个 Pool member。

本问题不同：

- 审计样本中绝大多数请求 `attempt_count=1`。
- 错位发生在 Lune 记录 `account_id` 之后、CPA provider endpoint 内部选择 auth file 之前。
- 即使完全关闭 Lune 跨账号重试，只要普通转发没有 pinning，CPA 仍可能自行选择其他 auth file。

## 外部 CPA advanced mode 文档要求

外部 CPA advanced mode 必须明确说明：

- 多个 Lune CPA account 共用同一个外部 CPA runtime 时，必须要求 runtime 支持 per-request credential pinning。
- 如果外部 CPA runtime 不支持 pinning，账号级统计和额度归因不可信。
- 在 pinning 不可用的情况下，Lune 应 fail closed，或只允许作为单账号 provider 使用。

## 待解决事项

- 普通 CPA 网关请求绑定到 Lune account 对应的 CPA credential。
- `resolveCpaRuntime` 的 runtime auth metadata 接入普通模型转发。
- `UpstreamTarget` 或等价结构增加 runtime credential binding 字段。
- 缺失 binding 时普通路由和强制路由都 fail closed。
- `request_logs` 增加 runtime auth id / auth index 字段。
- Activity / Pool 详情页按 runtime 归属可信度展示请求量。
- CPA provider 转发路径增加安全诊断日志。
- 外部 CPA advanced mode 文档补充 pinning 限制。

## 诊断日志要求

CPA provider 转发路径应记录安全截断后的诊断信息：

- request id。
- Lune account id。
- `cpa_account_key`。
- runtime auth index。
- CPA selected auth id。
- 模型。
- 状态码。
- normalized error。

不得记录 token、完整 prompt、完整 request body 或完整 auth file。

## 验收标准

- 在同一个 CPA runtime 中导入 3 个 Codex auth file 后，固定选择第一个 Lune CPA account，连续发起 10 次普通模型请求时，CPA actual credential 必须全部等于该账号对应凭据。
- 强制路由 `X-Lune-Account-Id` 指向某个 CPA account 时，CPA actual credential 必须等于该 account 对应凭据；否则请求 fail closed。
- 普通 Pool 自动路由选择第 N 个 CPA account 时，`request_logs.account_id` 与 `runtime_auth_id/runtime_auth_index` 能够一一对应。
- 当 CPA runtime 不支持 credential pinning 或 auth metadata 未就绪时，该账号不可接普通流量，错误 reason 明确。
- Activity 页面账号请求量基于已确认 runtime credential 归属；无法确认时显示不确定状态。
- 单元测试覆盖：CPA target 构建必须携带 runtime credential binding；缺失 binding 时 fail closed；日志同时保存 Lune account id 与 runtime auth id。
- 集成测试覆盖：模拟 CPA round-robin runtime，验证 Lune pinning 后不会被 CPA 默认 round-robin 打散。
