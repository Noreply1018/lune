# 08. Streaming 观测后续增强

状态：draft

来源：从 `spec/v0.1.6/03-streaming-activity-accounting.md` 移出的后续增强项。

本草案不属于当前 v0.1.6 交付范围。v0.1.6 已要求 stream completion marker、semantic failure、read/write error、明确上游错误 health impact、Activity Flow 和 confirmed binding 统计口径可用，并已通过 fake 容器代表性验证。

## 后续方向

- reader-based SSE forwarder。
- normalized error token 体系。
- 独立 stream timeout 配置。
- `stream_incomplete` badge。
- per-attempt retry path。
- request log API 增加 `pool_label`，避免 Activity flow 额外依赖 `/pools` fetch。

## 需要解决的问题

- reader-based forwarder 如何保持当前 completion marker、usage parsing 和错误提取语义。
- normalized error token 的枚举、版本策略和向后兼容。
- stream timeout 是否按 route、provider、model 或全局配置。
- per-attempt path 是否需要新表，还是扩展 request log JSON 字段。

## 进入正式规格前的最低验收

- fake SSE upstream 覆盖 complete、incomplete、semantic failure、timeout、downstream write error。
- Activity UI 清晰区分 stream incomplete 与明确上游失败。
- per-attempt 记录不会导致高流量下 SQLite 或 API 返回线性膨胀。
- 所有新增错误字段必须经过安全截断和脱敏。
