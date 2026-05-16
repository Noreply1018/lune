# 01. CPA 账号身份与 Runtime Credential 绑定错位

## 问题

2026-05-11 的生产审计发现，Lune 前端几乎所有请求量都显示在 Pool 第一位 Codex CPA 账号上，但后续其他 Codex 账号的远端额度持续减少。

证据：

- `request_logs.account_id=1` 占绝大多数请求。
- account `id=2/3/4/5` 的请求日志很少。
- 后续账号的 `codex_quota_json` 显示额度真实消耗。
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

多个 Codex CPA account 的 `cpa_provider` 都是 `codex`，所以请求 URL 完全相同。转发请求没有携带 `cpa_account_key`、CPA `auth_index`、email、OpenAI account id 或任何能 pin 到某个 auth file 的字段。CPA provider endpoint 在未绑定且未覆盖策略时，会落到自身默认 selector，在可用 Codex auth file 中选择凭据。这不是 Lune 的 Pool 顺序，也不是 Lune 的 `account_id`。

## 改进策略

普通 CPA 网关请求必须绑定到 Lune account 对应的 CPA runtime credential。只有 Lune 已确认可 pin 的 runtime auth file，并在转发时要求 CPA 使用该 auth，模型调用结果才能用于更新该账号的状态、请求量和额度归因。

核心规则：

> 已确认 runtime credential 归属的模型调用，才是最高可信信号。

`resolveCpaRuntime` 获取到的 runtime auth metadata 不能只服务于 quota/subscription 刷新；普通模型转发也必须使用同一份已校验 metadata。转发目标至少应携带：

- Lune account id。
- `cpa_account_key`。
- runtime auth id 或 `auth_index`。
- OpenAI account id 或其他可审计 runtime 身份。
- binding freshness/version。

当 CPA runtime 不支持 credential pinning，或 auth metadata 未就绪时：

- 该 CPA account 不可接普通流量。
- 强制路由也必须失败，不能静默使用 provider 级 round-robin。
- 错误 reason 应明确为 `runtime_auth_binding_unavailable` 或等价值。
- 管理员诊断可以走独立入口，但必须标记为 `diagnostic=true`，且不得更新普通路由健康或普通 usage 统计。诊断入口可以绕过 quota/subscription/serving cooldown 等普通路由保护来强测一次，但不得绕过 auth file 缺失、`needs_login` 或 runtime binding 不存在等无法确认账号身份的硬失败。

如果当前 CPA provider endpoint 不支持公开 pinning 参数，应优先推动或适配 CPA 的 pinned auth 能力。可选方向：

- CPA provider endpoint 支持 per-request auth index 或 auth id。
- Lune 通过 CPA management api-call 或等价接口执行 pinned 请求。
- Lune 对 embedded CPA 写入明确 routing 配置，但这只能解决策略一致性，不能替代 per-account pinning。

不允许用“provider 名相同但期待 CPA 默认选择正确账号”的方式实现多账号路由。

## UI 表现

Activity / Pool 详情页必须区分：

- Lune routed account：router 选择的 account row。
- Lune confirmed runtime binding：Lune 已确认并要求 CPA pin 到的 runtime auth file。

展示规则：

- runtime credential 归属确认时，账号请求量可作为精确统计。
- 未确认时，应显示“CPA runtime binding 未确认”或等价提示。
- Flow 图 `Pool -> Account -> Model` 应说明它基于 request log final route；在 runtime binding 完成前，账号级统计仍不可信。
- 账号卡片主问题 chip 应支持 `Binding 未确认`，颜色使用紫色，表达绑定、归因或统计可信度问题。
- 账号详情抽屉的 `诊断` tab 展示 Runtime Binding 维度：当前状态、runtime auth index、最近检查时间、最近错误或安全摘要、建议操作。

`Binding 未确认` 的含义：Lune 选择了某个 CPA account，但还不能确认它已经解析到可用于 pinning 的 runtime auth file。此时请求量、额度归因和健康修复都不应当作精确账号事实。

## 日志与诊断

`request_logs` 应区分：

- `account_id`：Lune router 选择的账号行。
- `runtime_auth_id` / `runtime_auth_index`：Lune 已确认并要求 CPA pin 到的 runtime auth。

如果 Lune 未能确认可 pin 的 runtime auth，应写 `unknown` 或空值，并在 Activity 中显示 binding 不确定，不能把请求量当作精确账号统计。更精确的 CPA 侧执行确认属于后续观测增强，不属于本轮 v0.1.6 必需能力。

CPA provider 转发路径应记录安全截断后的诊断信息：

- request id。
- Lune account id。
- `cpa_account_key`。
- runtime auth index。
- 模型。
- 状态码。
- 安全截断后的错误摘要。

不得记录 token、完整 prompt、完整 request body 或完整 auth file。

## 测试与验收

- 在同一个 CPA runtime 中导入 3 个 Codex auth file 后，3 个真实账号都必须分别通过强制路由或自动路由发起普通模型请求，并核对各自 `request_logs.account_id`、`runtime_auth_id`、`runtime_auth_index` 稳定对应；其中至少 1 个账号连续发起 10 次普通模型请求时，Lune 必须全部携带该账号对应的 pinned runtime auth。
- 强制路由 `X-Lune-Account-Id` 指向某个 CPA account 时，Lune 必须携带该 account 对应的 pinned runtime auth；否则请求 fail closed。
- 普通 Pool 自动路由选择第 N 个 CPA account 时，`request_logs.account_id` 与 `runtime_auth_id/runtime_auth_index` 能够一一对应。
- 当 CPA runtime 不支持 credential pinning 或 auth metadata 未就绪时，该账号不可接普通流量，错误 reason 明确。
- Activity 页面账号请求量基于已确认 runtime credential 归属；无法确认时显示不确定状态。
- 单元测试覆盖：CPA target 构建必须携带 runtime credential binding；缺失 binding 时 fail closed；日志同时保存 Lune account id 与 runtime auth id。
- 集成测试覆盖：模拟 CPA round-robin runtime，验证 Lune pinning 后不会被 CPA 默认 round-robin 打散。
- 真实多账号 Codex 上游消费验证是正式 v0.1.6 发布验收项；本轮已用同一 CPA runtime 中的多个真实 Codex auth file 验证 Lune 选择账号与 pinned runtime auth 稳定一致。

容器验收分层：

- `CT-01` 使用真实 Codex CPA 账号，必须由用户亲自导入至少 3 个真实账号后执行；本轮已完成并通过。
- `CT-02` 使用 fake CPA management/provider 模拟 pinning 能力、metadata 缺失和默认 round-robin，不需要真实账号，用于覆盖 fail-closed 和可信 usage 负向路径。

执行要求：

- `CT-01` 和 `CT-02` 都必须使用新 v0.1.6 镜像、临时容器和全新数据目录，不得复用或影响上一版本正在运行的容器，结束后必须删除测试容器。
- 验收记录必须保留镜像 tag 或 digest、容器启动命令、真实账号导入步骤或 mock 配置、关键 request log/API 摘要、清理命令结果。
- `CT-01` 的真实账号记录只允许保存用户操作步骤、账号 label/id 摘要和 request log 关联字段，不得记录 token、auth file 内容或完整凭据。

## 已完成事项

- v0.1.5 问题证据已归档到本问题。
- Activity Flow 已升级为 `Pool -> Account -> Model`，但在 runtime binding 完成前只能表示 Lune routed account。
- 普通 CPA 网关请求已接入 Lune account 对应的 CPA runtime credential binding。
- 强制路由和自动路由都会在转发前解析 runtime binding；缺失 binding、runtime 不支持 pinning 或账号不可确认时 fail closed。
- CPA 转发会携带 `X-Lune-CPA-Account-Key`、`X-Lune-Runtime-Auth-Id`、`X-CLIProxyAPI-Pinned-Auth-Id`、`X-Lune-Runtime-Auth-Index`、`X-CLIProxyAPI-Pinned-Auth-Index`、`X-CPA-Auth-Index`、`ChatGPT-Account-Id` 等 pinning/诊断 headers。
- `request_logs` 已增加 runtime auth id / runtime auth index / runtime account key 等 runtime identity 字段。
- 状态写入只信任 confirmed runtime binding；未确认 runtime credential 的 CPA 请求不会把某个 Lune account 标记为生成健康。
- Activity/usage 的账号级可信统计已改为只统计 confirmed CPA binding 请求。
- 内置 CPA 已通过 Lune patch 支持 per-request provider pinning，并在 embedded 模式设置 `LUNE_CPA_PROVIDER_PINNING_SUPPORTED=1`。
- 已补充单元测试覆盖强制/自动路由 binding、缺失 binding fail closed、状态写入只信任 confirmed binding、runtime identity 日志写入。
- 已通过 Docker 容器 smoke test 验证新镜像健康接口和 API 空状态可用。
- 已通过 `CT-01` 真实多账号 Codex 上游消费验证：3 个真实 CPA 账号强制路由均返回 `200` 且 request log 的 account/runtime binding 对应稳定；自动路由使用 selected account 的 confirmed pinned auth；其中 1 个账号连续 10 次普通请求保持同一 pinned runtime auth。
