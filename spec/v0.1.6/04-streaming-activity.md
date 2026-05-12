# 04. 流式记账与 Activity

## 目标

让流式代理失败在 Activity 中可见且可信，尤其是上游流在协议级完成事件前关闭的场景。同时把 Activity flow 从 `Pool -> Model` 升级为 `Pool -> Account -> Model`，便于检查路由行为。

旧版目标中，流式完成记账和 Activity flow 大部分已经完成。本文件保留已完成行为、设计语义和后续问题。

## 问题根源

Lune 过去直接把 streaming response 转发给客户端，并在 response body reader 结束时认为流已经结束。

这会漏掉多种失败：

- 上游流在 `[DONE]` 或 `response.completed` 前关闭。
- 上游流发出 `response.failed` 等语义失败事件。
- 响应头已经写给客户端后，上游读取失败。
- SSE 单行超过 scanner limit。
- 向下游客户端写入时中途失败。
- gateway request timeout 终止长流。

客户端 SDK 可能报错：

```text
stream disconnected before completion: stream closed before response.completed
```

也可能暴露上游容量错误：

```text
Selected model is at capacity. Please try a different model
```

过去 Activity 仍可能把这类请求记录为成功，因为 stream forwarding 路径没有把 read/write/completion error 返回给 gateway handler。

## 解决的问题

- EOF before completion 不能被记成成功。
- upstream semantic failure 不能被 HTTP 200 掩盖。
- 已经向客户端写出部分流后，不能再重试，但仍要记录失败。
- 大 SSE 行导致 scanner 失败时，应记录 stream read error。
- Activity 需要保留安全、可诊断的上游错误信息，而不是统一显示 `upstream error`。
- 后续独立请求应能绕开短期失败账号，而不是反复选择同一个失败账号。

## 流式完成判定

已完成方向：

- stream forwarding 返回结构化 metadata：usage、completion state、protocol marker、terminal error。
- OpenAI Chat Completions 通过 `[DONE]` 判定完成。
- OpenAI Responses 通过 `response.completed` 判定完成。
- `response.failed`、`response.incomplete`、scanner/read error、write error、EOF before completion 都视为 failed stream outcome。
- 从 stream events 中提取 upstream semantic error message，包括 model capacity error。
- 即使 HTTP status 已经写出 `200`，失败的 stream outcome 仍记录到 Activity。
- 能在失败前解析到 usage 时，保留 partial usage。
- 任何 stream bytes 已写给客户端后，不再重试。

仍需关注：

- 当前实现会记录 scanner failure，不再静默当成功；但 reader-based SSE forwarder 仍是后续优化。

## Activity 记录语义

对于收到成功 upstream HTTP status 但没有达到 completion marker 的流：

- `status_code` 保持 upstream HTTP status，通常是 `200`。
- `success=false`。
- `error_message` 解释终止条件，例如 `stream closed before response.completed`。
- upstream semantic failure 保留上游 message，例如容量错误。
- `latency_ms`、`pool_id`、`account_id`、`source_kind`、`attempt_count` 和已解析 usage 仍应记录。

## 容量失败语义

`Selected model is at capacity. Please try a different model` 不是 Lune 生成的错误，而是上游模型或账号容量失败通过客户端协议暴露出来。

Lune 的责任：

- 如果容量失败发生在 response headers 或 stream bytes 写出前，正常 retry 仍可能可行。
- 如果容量失败发生在 stream output 已开始后，Lune 不应 retry，因为下游响应已经 committed。
- 两种情况下 Activity 都应记录 routed pool/account/model 和 upstream capacity message。
- stream-level capacity failure 不应因为 HTTP status 是 `200` 就清除已有账号错误状态或标记账号 healthy。

## CPA stream failover gap

2026-05-06 的生产失败显示，`POST /api/provider/codex/v1/responses` 连续路由到 Pool 第一个账号，CPA error log 中真实失败是：

```text
Post "https://chatgpt.com/backend-api/codex/responses": EOF
```

但 Activity 只记录：

- `source_kind=cpa`
- `model_requested=gpt-5.5`
- `status_code=500`
- `error_message=upstream error`
- `stream=true`
- `attempt_count=1`

失败停留在 `account_id=1`，没有移动到 `account_id=2`。根因是 streaming retry branch 在 retryable HTTP status 上过早返回，早于失败账号被排除或标记短期不可用。

已完成行为：

- streaming upstream 返回 retryable status，例如 `500`，即使当前流不能安全 retry，也记录 account health impact。
- 失败账号进入可被后续 route resolution 暂时避开的状态。
- 后续独立请求能选择下一个健康 Pool member。
- 仍然避免在 response bytes committed 后 retry。
- 区分“当前已 committed stream 不能 retry”和“仍需更新 routing health”。

2026-05-09 runtime audit 后的结论：

- EOF before completion 应记录 request-level failure，并进入 time-limited `serving_status=cooldown`。
- 不应写永久 account `error` 或泛化 `degraded` 状态。

## 上游错误细节保留

Activity 应在收到结构化 upstream error body 时保留有用细节。泛化的 `upstream error` 不足以诊断。

已完成方向：

- retryable upstream HTTP response 包含 JSON error body 时，提取最佳 message 写入 `request_logs.error_message`。
- CPA errors 保留安全 message，例如上游 EOF。
- message 有长度上限，避免 SQLite 或 Activity API 膨胀。
- 不暴露凭据或完整 request body。
- upstream body 有安全 message 时，优先使用 concise normalized message。
- 无法安全提取时，保留 generic fallback。

仍待明确：

- `request_logs.error_message` 的最大长度。
- 更系统的 normalized error token 设计，用于重复错误折叠。

## Activity Flow 升级

旧 Activity Sankey 从两列升级为三列：

- Pool
- Account
- Model

数据来源：

- `pool_id`
- `account_id`
- `account_label`
- `model_requested`
- `model_actual`

构建两组 link：

- `pool -> account`
- `account -> model`

语义限制：

- 当前图描述每条 request log 的最终记录路由。
- 它不描述每一次 retry attempt，因为 request logs 只保存最终 account 和聚合 attempt count。
- 在 CPA runtime binding 未完成前，CPA account 级 flow 只能表示 Lune routed account；不能暗示实际 CPA credential 已确认。

已完成 UI 要求：

- section 改名为 `Pool -> Account -> Model`。
- 文案和 tooltip 更新为三步路径。
- 保留 rolling 24h source window。
- 增加 account 中间列，优先用 `account_label`，回退 `#account_id`。
- route rejected 且没有 selected account 的行排除或归到清晰 `Unrouted` 语义。
- 窄屏保持可读：允许横向滚动，扩大 SVG viewBox。
- failed routed requests 默认纳入 flow；仅排除没有 selected account 的 rows。

后续问题：

- stream timeout 是否需要独立于 non-stream request timeout 的设置。
- UI 是否需要单独展示 `stream_incomplete` badge，而不只是一条 failed request row。
- 是否展示 per-attempt retry path。需要 request log 存更丰富的 attempt history。
- 是否在 request log API 中增加 `pool_label`，避免 diagram 依赖额外 `/pools` fetch。
- CPA runtime binding 未确认时，flow 应如何标注 routed account 与 actual credential 的差异。

## 测试清单

已覆盖或应保持覆盖：

- Chat Completions stream 缺少 `[DONE]`，Activity 记录失败。
- Responses stream 缺少 `response.completed`，Activity 记录失败。
- Responses stream 发出 `response.failed`，Activity 记录失败并保留 upstream status。
- Responses stream 发出容量错误，Activity 保留该 message。
- Responses stream 发出 `[DONE]` 但没有 `response.completed`，仍记录失败。
- CPA stream 返回 HTTP 500 且 body 为 JSON error，Activity 记录提取后的上游 message。
- CPA stream 第一 Pool account 返回 HTTP 500 后，后续独立请求能避开该账号。
- streaming retryable status 即使发生在单次或最终 attempt，也记录 account health impact。
- SSE line 超过旧 1MB scanner limit，Activity 记录 stream read error。
- downstream write 失败后，Activity 记录失败。
- 长流超过 gateway timeout，Activity 记录 timeout 相关失败。

## 验收标准

- 流没有协议级完成 marker 时，不能记为成功。
- upstream semantic failure 的 message 出现在 Activity 中。
- stream bytes 已写出后不 retry，但仍更新 request log 和 serving cooldown。
- capacity failure 不会清除账号既有错误状态。
- Activity flow 展示 `Pool -> Account -> Model`，并明确其基于 request log final route。
- CPA actual credential 未确认时，Activity 不把账号级统计表述成精确实际消耗。
