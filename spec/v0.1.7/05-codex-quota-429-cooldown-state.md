# 05. Codex 额度耗尽与 Serving 冷却状态归类

状态：draft。0.1.6 真实运行态已完成只读审计；v0.1.7 的 `429` 产品表达按本轮确认口径沉淀。

来源：2026-05-16 对正在运行的 `lune-0.1.6` 容器进行只读审计。用户反馈：当前 0.1.6 容器里第二个 Codex CPA 账号在 Playground 直测时表现为额度已经耗尽，但账号卡片和详情页主状态仍显示为“服务冷却中”。

## 问题

当前 Lune 把 Codex CPA 账号的“额度快照状态”和“真实模型请求失败状态”拆成了两条独立状态线：

- `cpa_quota_status` 来自 Codex `wham/usage` 辅助接口。
- `serving_status` 来自真实 `/v1/chat/completions` 或同类模型请求。

这套拆分本身是合理的，但现有 v0.1.6 仍有一个状态归类缺口：真实模型请求返回 `429` 时，网关只把账号写入 `serving_status='cooldown'`，不会同步把 Codex quota 状态升级为 `blocked` 或“quota limited by real request”。因此会出现用户体验上的矛盾：

- Playground 实际请求已经返回 `429`，用户判断为“额度耗尽”。
- 管理页仍看到 `cpa_quota_status='ok'`。
- 卡片主问题 chip 和详情页 Route 摘要显示“服务冷却中”。
- Quota 诊断区如果只看 `wham/usage` 快照，会继续显示“额度可用”或只显示 5h 窗口剩余为 `0%`。

这不是用户误判，也不是单纯前端展示顺序错误，而是后端状态模型没有把真实请求中的 Codex 限流/额度耗尽证据沉淀到 quota 维度。

## 审计过程

本次审计遵守只读原则，没有修改正在运行的容器、数据库、auth file 或配置。

### 1. 运行中容器与账号数据

运行中的相关容器：

- `lune-0.1.6`：镜像 `noreply1018/lune:0.1.6`，端口 `127.0.0.1:22222->7788`。
- `lune-0.1.5`：镜像 `noreply1018/lune:0.1.5`，作为旧版本对照，未被停止或修改。

通过 `GET http://127.0.0.1:22222/admin/api/pools/1` 读取运行中 0.1.6 的真实 API 数据，第二个 Codex CPA 账号当时的关键字段为：

```text
cpa_quota_status: ok
rate_limit.allowed: true
rate_limit.limit_reached: false
rate_limit.primary_window.used_percent: 100
serving_status: cooldown
failure_count: 1
last_failure_at: 2026-05-16T07:48:51Z
cooldown_until: 2026-05-16T07:53:51Z
```

这说明 UI 显示“冷却”不是空穴来风：后端返回给前端的真实字段确实是 `serving_status='cooldown'`，而不是 `cpa_quota_status='blocked'`。

### 2. 容器日志证据

`lune-0.1.6` 容器日志在同一时间段出现真实模型请求限流：

```text
2026-05-16 07:48:51 POST "/api/provider/codex/v1/chat/completions" 429
2026-05-16T07:48:51Z request completed path=/v1/chat/completions status=429
```

该时间与账号字段中的 `last_failure_at=2026-05-16T07:48:51Z` 对齐。

### 3. 代码路径复核

网关当前把 `429` 视为 retryable status：

```go
func IsRetryableStatus(statusCode int) bool {
	return statusCode >= 500 || statusCode == 429
}
```

普通请求命中 `429` 后，如果不是 diagnostic 请求，会调用 `recordServingFailure`，最终写入：

```text
serving_status='cooldown'
failure_count=failure_count + 1
last_failure_at=<now>
cooldown_until=<now+5m>
last_error=<upstream error>
```

但 gateway 路径没有调用 `UpdateAccountCodexQuotaStatus`，因此不会把这次真实请求的 `429` 反写为 `cpa_quota_status='blocked'`。

Codex quota 辅助刷新路径只在 `wham/usage` 返回结构里出现以下信号时写 `blocked`：

- `allowed=false`
- `is_allowed=false`
- `limit_reached=true`
- `limited=true`
- `blocked=true`
- 或上述字段出现在 `rate_limit` / `limits` 嵌套对象里

如果 `wham/usage` 快照仍是 `allowed=true`、`limit_reached=false`，即使 `primary_window.used_percent=100`，当前逻辑也仍写 `cpa_quota_status='ok'`。

## 审计结论

本问题的真实结论是：

> Playground 看到的“额度耗尽”是实际模型请求返回 `429` 的结果；管理页显示“冷却”也是当前后端字段的真实反映。缺陷在于 v0.1.6 没有把 Codex 真实模型请求的 quota / rate-limit 失败归类到 quota 维度，而是只写入了 serving cooldown。

当前状态口径的具体问题：

- `cpa_quota_status='ok'` 只代表最近一次 `wham/usage` 辅助接口没有明确给出 `blocked`。
- `primary_window.used_percent=100` 已经是强风险信号，但当前后端不把它视为 blocked。
- 模型请求 `429` 可能是 Codex 真实额度窗口耗尽，也可能是其他上游限流；当前 Lune 只记录成 generic serving cooldown。
- 前端卡片主问题 chip 在 quota danger 为空时显示 serving cooldown，符合当前字段，但没有表达“这次 cooldown 来自 429，可能是额度/限流”。

## 改进策略

v0.1.7 应补齐真实请求限流到账号状态的归类能力，同时保留 quota 快照与 serving health 的分层。

建议新增或调整以下状态口径：

1. 网关识别 CPA Codex 账号真实请求的 `429`。
2. 如果 upstream error body 或状态码能判定为 quota / rate-limit，应更新 quota 维度，而不是只写 serving cooldown。
3. 新增安全的 quota last evidence 字段，或复用 `cpa_quota_last_error` 写入摘要，例如 `HTTP 429 from model request`。
4. 对 Codex CPA 的真实 `429`，可将 `cpa_quota_status` 写为 `blocked`，或新增更精确状态如 `limited`。如果新增状态，需要同步类型定义、路由条件、UI 文案和迁移默认值。
5. `serving_status` 可以继续进入 cooldown，用于短期避让，但 UI 主问题应优先解释 quota / rate-limit 证据。
6. `wham/usage` 的 `used_percent=100` 应至少触发 warning / exhausted-window 展示；是否直接 blocking 需要谨慎，因为样本中 `allowed=true` 且 `limit_reached=false`。

建议优先采用保守实现：

- 模型请求 `429` 不直接等同于永久 quota blocked，但对 Codex CPA 写入 quota evidence。
- 若 error body 明确包含 quota、rate limit、usage limit、limit reached 等语义，再置为 `blocked`。
- 若只有裸 `HTTP 429`，置为 `error` 或新增 `limited`，并在 UI 中显示“模型请求被限流”，普通路由短期仍由 serving cooldown 避让。

## UI 表现

账号卡片和详情页需要同时表达两层事实：

- `serving_status='cooldown'`：短期路由避让仍然存在。
- 最近一次真实模型请求 `429`：该冷却可能来自 Codex 额度 / 限流，而不是普通上游 5xx。

建议 UI 主状态优先级调整为：

1. runtime binding fail closed。
2. credential 明确不可用。
3. subscription 非 active。
4. quota blocked / quota limited / real request 429 quota evidence。
5. serving cooldown / serving error。
6. quota error / quota unknown 降权。
7. auth suspect 降权。

对当前案例，理想展示不应只显示“服务冷却中”，而应显示类似：

```text
模型请求限流
最近普通模型请求返回 HTTP 429，账号已短期冷却；可能是 Codex 额度窗口耗尽。
Playground 直测返回 HTTP 429 时，只在诊断证据里展示，不触发普通路由冷却。
```

Quota 详情区应把 `wham/usage` 快照与真实请求证据拆开：

```text
Quota Snapshot: ok
5h window: 0% remaining
Last model request quota signal: HTTP 429 at 2026-05-16 07:48:51
Routing impact: cooldown until 2026-05-16 07:53:51
```

## 日志与诊断

日志与 API 诊断应保留足够证据，但不能泄露请求内容、auth file 或 token。

建议记录：

- account id。
- source kind / provider。
- upstream HTTP status。
- safe upstream error summary。
- 是否来自 diagnostic / Playground。
- 是否触发 serving cooldown。
- 是否触发 quota evidence 写入。
- runtime binding status / auth id 摘要。

不应记录：

- 完整 prompt。
- 完整 access token / refresh token。
- auth file 内容。
- 完整 CPA API key。

## 测试与验收

### 单元与集成测试

- gateway 测试：Codex CPA 账号真实请求返回 `429` 时，应写入 serving cooldown，并写入 quota evidence。
- gateway 测试：非 Codex 账号返回 `429` 时，不应误写 Codex quota 字段。
- gateway 测试：diagnostic 请求返回 `429` 时，不应污染普通路由的 serving cooldown 或 quota evidence；只保留直测失败证据。
- health checker 测试：`wham/usage` 返回 `allowed=false` 或 `limit_reached=true` 时继续写 `blocked`。
- health checker 测试：`wham/usage` 返回 `used_percent=100`、`allowed=true`、`limit_reached=false` 时不应被静默展示为完全健康，至少应有 UI warning 或 quota evidence。
- 前端测试：当账号同时有 `serving_status='cooldown'` 和最近 quota / rate-limit evidence 时，卡片主问题应显示限流/额度相关文案，而不是 generic “服务冷却中”。
- 前端测试：详情页 diagnostics 同时展示 quota snapshot 与 real request evidence。

### 真实容器测试要求

必须使用新启动的测试容器，不复用正在运行的 `lune-0.1.5` 或 `lune-0.1.6` 容器。测试容器可使用 fake upstream 或 mock CPA provider，避免消耗真实账号额度。

最小容器验收矩阵：

| 编号 | 场景 | 准备 | 操作 | 期望 |
| --- | --- | --- | --- | --- |
| CT-429-01 | Codex CPA 模型请求返回裸 `429` | 新容器 + fake Codex CPA + mock upstream | 使用 Pool token 发普通 `/v1/chat/completions` | 账号进入短期 cooldown；quota evidence 记录 `HTTP 429 from model request`，UI 不只显示 generic 健康 |
| CT-429-02 | Codex CPA 模型请求返回明确 quota 文案的 `429` | 新容器 + mock upstream body 包含 `quota` / `rate limit` / `limit reached` | 发普通请求 | `cpa_quota_status` 或新增 quota state 反映 blocked/limited；卡片主问题显示限流/额度 |
| CT-429-03 | 非 Codex 账号返回 `429` | openai_compat fake upstream | 发普通请求 | 只触发 serving cooldown，不写 Codex quota 字段 |
| CT-429-04 | Diagnostic / Playground 强制账号返回 `429` | `X-Lune-Account-Id` 强制路由 | 使用 Playground 或 diagnostic request 触发直测 | request log 保留诊断证据；直测失败不污染普通 serving cooldown 或 quota evidence |
| CT-429-05 | `wham/usage` 快照 ok 但模型请求 429 | quota mock 返回 ok，模型 mock 返回 `429` | 刷新额度后再发模型请求 | Quota snapshot 与 real request evidence 分层保存，`wham/usage` 成功快照不覆盖模型请求 429 evidence |
| CT-429-06 | 冷却过期后状态解释 | 等待或模拟 cooldown 到期 | 刷新 Pool 页面 | 若 quota evidence 仍有效，主状态不应直接退回“可接流量”；若上游恢复，需要清除或降级旧 evidence |

容器测试完成后必须删除测试容器和临时 volume / 数据目录。本轮仅沉淀规格和产品口径，未启动新的 v0.1.7 容器，也不声称已执行完整 fake CPA 429 矩阵。

## 审计记录

- 已只读审计运行中的 `lune-0.1.6` 容器。
- 已确认目标账号 API 返回字段中 `cpa_quota_status='ok'` 且 `serving_status='cooldown'`。
- 已确认 quota 快照中 `rate_limit.allowed=true`、`rate_limit.limit_reached=false`、`primary_window.used_percent=100`。
- 已确认容器日志中真实模型请求在 `2026-05-16 07:48:51` 返回 `429`。
- 已确认 gateway 当前只把 `429` 归入 retryable serving failure，不会同步写入 `cpa_quota_status`。

## 后续非阻塞项

- 如后续需要更接近真实 Codex runtime 的端到端 429 验收，可补充专用 fake CPA 容器场景，但不应消耗真实账号额度。
- 如果未来新增更细的 quota state（例如 `limited`），需要同步类型、路由条件和 UI 文案；v0.1.7 当前保守复用 `blocked` / `error`。

## 后续非阻塞说明

v0.1.7 采用保守产品口径：普通 Codex 模型请求遇到 `429` 时，Lune 同时记录短期 `serving cooldown` 和一条 quota / rate-limit 证据；裸 `429` 不直接说“额度已用尽”，只有错误内容明确指向 quota / rate limit 时才提升为更强的额度问题。

- 裸 `HTTP 429` 在 UI 上命名为“模型请求被限流”。
- 带 `quota`、`rate limit`、`limit reached` 文案的 `429` 显示为“额度 / 限流问题”。
- Playground 人为直测失败只作为诊断证据展示，不影响普通路由状态。
