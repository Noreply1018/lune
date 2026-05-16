# 07. 账号状态诊断后续增强

状态：draft

来源：从 `spec/v0.1.6/02-account-status-routing-trust.md` 移出的后续增强项。

本草案不属于当前 v0.1.6 交付范围。v0.1.6 只要求 credential、quota、subscription、serving、runtime binding 拆开保存和组合路由，管理员 diagnostic request 可绕过 quota/subscription/serving cooldown 强测一次，但不得改变普通路由健康或普通 usage。

## 后续方向

- quota 缓存补齐 `quota_last_attempt_at`、`quota_last_success_at`、`quota_last_error`。
- 增加状态写入来源、前值、后值、时间和单调版本语义。
- 账号详情 `诊断` tab 从当前五维状态摘要扩展为完整状态时间线。

## 需要解决的问题

- 如何避免状态时间线无限增长。
- 状态版本是否按 account 维度递增，还是按 credential/quota/subscription/serving/runtime binding 各自递增。
- UI 是否默认展示最近 N 条，完整时间线通过导出或高级面板查看。
- 状态写入审计日志与 notification dedup、Activity request log 如何关联。

## 进入正式规格前的最低验收

- fake account + mock upstream 能构造每个维度的状态迁移。
- API 返回前值、后值、来源、时间、reason 和版本。
- 诊断 tab 能展示最近状态时间线，且不会暴露 token、完整 request body 或完整 auth file。
- SQLite 查询有上限、索引或分页保护。
