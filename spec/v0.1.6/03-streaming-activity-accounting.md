# 03. Streaming 失败与 Activity 记账不准确

## 问题

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

2026-05-06 的生产失败还显示，`POST /api/provider/codex/v1/responses` 连续路由到 Pool 第一个账号，CPA error log 中真实失败是：

```text
Post "https://chatgpt.com/backend-api/codex/responses": EOF
```

但 Activity 只记录 `status_code=500`、`error_message=upstream error`、`stream=true`、`attempt_count=1`，没有保留足够错误细节，也没有让后续独立请求避开短期失败账号。

## 改进策略

stream forwarding 返回结构化 metadata：usage、completion state、protocol marker、terminal error。

流式完成判定：

- OpenAI Chat Completions 通过 `[DONE]` 判定完成。
- OpenAI Responses 通过 `response.completed` 判定完成。
- `response.failed`、`response.incomplete`、scanner/read error、write error、EOF before completion 都视为 failed stream outcome。
- 从 stream events 中提取 upstream semantic error message，包括 model capacity error。
- 即使 HTTP status 已经写出 `200`，失败的 stream outcome 仍记录到 Activity。
- 能在失败前解析到 usage 时，保留 partial usage。
- 任何 stream bytes 已写给客户端后，不再重试。

对于收到成功 upstream HTTP status 但没有达到 completion marker 的流：

- `status_code` 保持 upstream HTTP status，通常是 `200`。
- `success=false`。
- `error_message` 解释终止条件，例如 `stream closed before response.completed`。
- upstream semantic failure 保留上游 message，例如容量错误。
- `latency_ms`、`pool_id`、`account_id`、`source_kind`、`attempt_count` 和已解析 usage 仍应记录。

容量失败语义：

- 如果容量失败发生在 response headers 或 stream bytes 写出前，正常 retry 仍可能可行。
- 如果容量失败发生在 stream output 已开始后，Lune 不应 retry，因为下游响应已经 committed。
- 两种情况下 Activity 都应记录 routed pool/account/model 和 upstream capacity message。
- stream-level capacity failure 不应因为 HTTP status 是 `200` 就清除已有账号错误状态或标记账号 healthy。

streaming upstream 返回 retryable status，例如 `500`，即使当前流不能安全 retry，也记录 account health impact。失败账号进入可被后续 route resolution 暂时避开的状态；后续独立请求能选择下一个健康 Pool member。仍然避免在 response bytes committed 后 retry。

EOF before completion 应记录 request-level failure，并进入 time-limited `serving_status=cooldown`，不应写永久 account `error` 或泛化 `degraded` 状态。

## UI 表现

Activity Flow 从两列升级为三列：

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

后续可考虑：

- UI 是否需要单独展示 `stream_incomplete` badge，而不只是一条 failed request row。
- 是否展示 per-attempt retry path。需要 request log 存更丰富的 attempt history。
- 是否在 request log API 中增加 `pool_label`，避免 diagram 依赖额外 `/pools` fetch。
- CPA runtime binding 未确认时，flow 应如何标注 routed account 与 pinned runtime binding 的差异。

## 日志与诊断

Activity 应在收到结构化 upstream error body 时保留有用细节。泛化的 `upstream error` 不足以诊断。

要求：

- retryable upstream HTTP response 包含 JSON error body 时，提取最佳 message 写入 `request_logs.error_message`。
- CPA errors 保留安全 message，例如上游 EOF。
- message 有长度上限，避免 SQLite 或 Activity API 膨胀。
- 不暴露凭据或完整 request body。
- upstream body 有安全 message 时，优先使用 concise normalized message。
- 无法安全提取时，保留 generic fallback。

仍待明确：

- `request_logs.error_message` 的最大长度。
- 更系统的 normalized error token 设计，用于重复错误折叠。
- stream timeout 是否需要独立于 non-stream request timeout 的设置。

## 测试与验收

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
- stream bytes 已写出后不 retry，但仍更新 request log 和 serving cooldown。
- Activity flow 展示 `Pool -> Account -> Model`，并明确其基于 request log final route。
- CPA runtime binding 未确认时，Activity 不把账号级统计表述成精确实际消耗。

### Docker 容器验收

03 的最终验收必须包含真实 Docker 容器中的 streaming 行为验证。实现完成后必须用新 v0.1.6 镜像启动一次临时 Docker 容器，使用全新数据目录和可控 mock upstream / mock CPA 响应验证 SSE 转发、timeout、Activity 记账和后续路由行为。测试容器不得复用或影响上一版本正在运行的容器，结束后必须删除。

容器验收至少覆盖：

- mock upstream 返回 Chat Completions stream，但缺少 `[DONE]`：客户端收到 stream 后，Activity 必须记录 `success=false`，错误摘要包含缺少 `[DONE]`，状态码保留 upstream HTTP status。
- mock upstream 返回 Responses stream，但缺少 `response.completed`：Activity 必须记录失败，错误摘要能解释 `stream closed before response.completed`。
- mock upstream 返回 Responses stream 中的 `response.failed` 容量错误：Activity 必须保留上游容量 message，账号进入 `serving_status=cooldown`，不得把该账号标记为生成健康。
- mock upstream 在 stream 中途 EOF 或超过 gateway timeout：Activity 必须记录失败，保留已解析 usage 或 partial metadata，并更新 request log 与 serving cooldown。
- stream bytes 已写出后不得重试当前请求；如果后续独立请求发生在失败账号 cooldown 期间，路由必须选择另一个健康账号。
- mock CPA provider 返回 streaming HTTP `500` 且 body 为 JSON error：Activity 必须记录提取后的安全上游 message，而不是泛化为 `upstream error`。
- Activity Flow 必须展示 `Pool -> Account -> Model`，failed routed requests 默认纳入 flow；没有 selected account 的路由拒绝请求不得混入普通账号路径。
- CPA runtime binding 未确认或 binding 失败的 streaming 请求不得计入可信账号请求量/usage。

验收记录应保留：镜像 tag 或 digest、容器启动命令、mock upstream/CPA SSE 行为配置、关键 Activity API 响应摘要、UI 截图或 Playwright 断言、清理测试容器的命令结果。

## 已完成事项

- stream forwarding 已返回结构化 metadata。
- EOF before completion、semantic failure、scanner/read/write error 已按 failed stream outcome 处理。
- upstream semantic failure message 已进入 Activity。
- streaming retryable status 已记录 account health impact。
- Activity Flow 已升级到 `Pool -> Account -> Model`。
- `request_logs` 已增加 runtime identity 字段，Activity 能区分 Lune routed account 与 confirmed CPA runtime credential。
- failed routed requests 默认纳入 flow；没有 selected account 的 rows 不再混入普通账号路径。
- 账号级 usage/request 统计已按 confirmed CPA binding 过滤，避免 streaming 失败或 CPA 默认调度造成错误归因。
- 已通过 `go test ./...` 覆盖 gateway、router、store、stats 相关 streaming/accounting 回归。

## 待解决事项

- reader-based SSE forwarder 仍是后续优化。
- `request_logs.error_message` 的最大长度待明确。
- normalized error token 设计待完善。
- stream timeout 是否独立配置待决策。
- 是否展示 `stream_incomplete` badge、per-attempt retry path、`pool_label` 待后续设计。
