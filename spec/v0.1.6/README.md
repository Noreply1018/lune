# v0.1.6 规格整理

状态：draft

本目录把旧版单文件 `spec/v0.1.6.md` 拆成多份主题文档。拆分目标是把历史审计、已完成事项、待解决问题和新增设计约束放到各自位置，避免后续实现时被杂糅的上下文误导。除明确按本次新增规则收束的状态机语义外，旧文件中的主题和验收要求应完整迁移。

## 文档结构

- `01-status-routing.md`：账号状态机、模型调用可信度、quota/subscription/serving 路由规则。
- `02-cpa-credential-lifecycle.md`：CPA 登录、删除、重登、auth file、runtime reload、重复账号治理。
- `03-cpa-runtime-binding.md`：CPA provider 级转发导致的 runtime credential 归属错位，以及 per-request credential pinning 要求。
- `04-streaming-activity.md`：流式完成判定、Activity 失败记账、上游错误细节保留、Activity flow。
- `05-lightweight-runtime.md`：2C2G/2C4G VPS、低资源 profile、SQLite、健康检查、请求/响应内存控制、原生 systemd 部署。
- `06-ui-behavior.md`：UI 展示、Active Pool 卡片修复、状态文案、Activity 归因提示。
- `07-ops-observability.md`：Docker/Compose、healthcheck、日志降噪、审计日志、诊断页。
- `08-acceptance-tests.md`：跨主题验收标准和测试清单。

## 总体原则

1. 真实业务调用优先于辅助观测接口。已确认 runtime credential 归属的模型生成请求，是判断账号能否工作的最高可信信号。
2. 辅助接口只更新自己负责的状态。quota 接口不能单独把账号写成 `needs_login`，subscription 接口也不能覆盖 serving 健康。
3. 多维状态必须拆开保存、组合决策。`credential_status`、`quota_status`、`subscription_status`、`serving_status` 不能继续互相复用。
4. CPA 多账号场景必须先解决 runtime credential 归属。否则“模型调用成功”可能来自另一个 CPA auth file，反而会污染被选账号的状态。
5. UI 默认解释真实问题，不用红色“请重登”覆盖所有异常。只有真实凭据失效或已确认认证失败时，才显示需要重新登录。

## 历史内容归档方式

旧文件中已经完成的 v0.1.6 工作会保留为“已完成/已验证”条目；生产审计记录会按问题根源归入对应主题。每个主题文档都尽量明确：

- 问题根源
- 解决的问题
- 设计规则
- 验收标准
- 仍待决定的问题
