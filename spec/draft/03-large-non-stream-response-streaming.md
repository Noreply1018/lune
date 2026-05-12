# 03. 大型非流式响应 Streaming

状态：draft

来源：

- previous `spec/draft.md`
- previous `spec/v0.1.6/05-lightweight-runtime.md`

## 目标

当 upstream provider 返回大型非流式响应时降低内存压力，尤其是 `/v1/responses` 图像和文件类 workflow。

## 当前问题

Lune 当前会先把非流式 upstream response 缓冲到内存，再写给客户端。

这有利于 retry 行为和 usage parsing，但当 upstream response 很大时成本较高。

## 解决的问题

- 防止大型非流式响应造成内存峰值。
- 保留小响应的 usage parsing 和 retry 行为。
- 对大响应优先保护内存，不为了完整 body retry 牺牲稳定性。

## 草案方向

- 小型非流式响应继续缓冲到内存。
- 大型非流式响应改为直接写给客户端，不持有完整 body。
- 保留 Activity 和 usage tracking 所需的 bounded metadata。
- 不破坏 response headers 写出前发生错误时的 retry 语义。
- 明确区分 response-body streaming 和现有 request-body replay-to-disk。

## 设计规则

- 小型非流式响应继续缓冲到内存。
- 新增可配置 response buffer threshold。
- 超过阈值后，不持有完整 response body。
- 一旦确认不能安全 retry，直接流式写给客户端。
- 保留 Activity 所需的 bounded metadata。
- 小响应继续解析 usage。
- 大响应 usage 可能不可用，除非能从 bounded metadata 中安全解析。
- Response-body streaming 与 request-body replay-to-disk 是两个不同问题，不能混淆。

## Retry 语义

- Upstream response headers 前的失败，retry 行为保持不变。
- Response headers 或 body bytes 写给 downstream 后，不再 retry。
- 大型非流式响应跨过阈值后，内存保护优先于保留完整 body retry。

## 初始阈值

实现时再最终确认，初始建议：

- normal profile：8MB 到 16MB。
- low-resource profile：1MB 到 4MB。

Draft 阶段可先使用全局设置；per-route thresholds 作为后续增强。

## 草案验收

- 小型非流式 response 保持 usage parsing。
- 大型非流式 response 不全量驻留内存。
- upstream failure before response headers 仍可 retry。
- downstream 已写出后不 retry，并准确记录 Activity。

## 待决问题

- 多大的 response size threshold 应从内存缓冲切换到 streaming？
- usage 是否能从 bounded response tail 或 side channel 安全解析？
- 对 retry fidelity 更重要的 route，是否应禁用 large response streaming？
- large response streaming 应全局配置、按 route 配置，还是两者都支持？
