# v0.1.6 规格整理

状态：核心路由、runtime binding、可信记账与内置 CPA 构建闭环已实现并通过本轮验证；部分运维验收和诊断增强仍待后续。

本目录按“问题闭环”组织 v0.1.6 规格。每份文档围绕一个真实问题展开，统一描述问题、改进策略、UI 表现、日志与诊断、测试与验收、已完成事项和待解决事项，避免后端、UI、测试、运维规则分散在不同文件中后相互遗漏。

## 文档结构

- `01-cpa-runtime-identity-binding.md`：CPA 账号身份与 runtime credential 绑定错位。
- `02-account-status-routing-trust.md`：账号状态互相污染导致错误路由和错误 UI。
- `03-streaming-activity-accounting.md`：Streaming 失败与 Activity 记账不准确。
- `04-cpa-credential-lifecycle.md`：CPA 凭据生命周期、重复账号、删除与重登。
- `05-ops-observability-diagnostics.md`：部署、日志、health、诊断能力不足。
- `99-acceptance-matrix.md`：跨问题验收矩阵和最终核对清单。

## 总体原则

1. 真实业务调用优先于辅助观测接口。已确认 runtime credential 归属的模型生成请求，是判断账号能否工作的最高可信信号。
2. 辅助接口只更新自己负责的状态。quota 接口不能单独把账号写成 `needs_login`，subscription 接口也不能覆盖 serving 健康。
3. 多维状态必须拆开保存、组合决策。`credential_status`、`quota_status`、`subscription_status`、`serving_status` 不能继续互相复用。
4. CPA 多账号场景必须先解决 runtime credential 归属。否则“模型调用成功”可能来自另一个 CPA auth file，反而会污染被选账号的状态。
5. UI 默认解释真实问题，不用红色“请重登”覆盖所有异常。只有真实凭据失效或已确认认证失败时，才显示需要重新登录。

## 版本边界

v0.1.6 成功后按新容器和新数据目录运行，不承担旧容器中的数据、字段或语义迁移。规格中不再要求兼容旧 `cpa_credential_status`、旧 request log schema、旧 auth metadata 或历史 SQLite 数据；如需保留历史实例，应继续运行旧容器或手工导出需要的信息。

因此，本目录中的状态、字段和验收要求都按新 v0.1.6 runtime 的最终形态描述。实现时不需要增加旧数据迁移、旧字段兼容读取、旧语义转换或 migration wizard。

## 本轮完成摘要

本轮 v0.1.6 已完成并验证以下闭环：

- CPA 普通模型请求不再只按 provider 转发；Lune 会先解析目标账号对应的 runtime auth binding，再把 auth id/auth index 等 pinning 信息传给内置 CPA。
- CPA runtime 不支持 pinning、binding 未就绪或强制账号无法确认 runtime credential 时，普通流量 fail closed，避免 CPA 默认 round-robin 污染账号归因。
- `request_logs` 增加 runtime identity 字段，Activity/usage 的账号级可信统计只使用已确认 CPA binding 的请求。
- 账号状态、路由和 UI 口径补齐：Codex subscription 非 active 阻断；`auth_suspect` 可路由但降权；Pool 统计和前端状态大小写处理与后端一致。
- 内置 CPA 从外部固定镜像切换为固定 upstream commit、Lune provider pinning patch 和本地构建，版本标识为 `v7.0.2-lune.1`。
- embedded CPA 子进程退出时不再被静默忽略；entrypoint 会让容器失败，便于 Docker/health 发现问题。
- 已执行 `go test ./...`、`npm run build`、`sh -n docker/entrypoint.sh`、`git diff --check`、Docker build 和新容器 smoke test；旧版 `lune-0.1.5` 容器未被改动。

## 单篇文档模板

每个问题文档尽量保持同一结构：

- `问题`：现象、影响、根因和必要生产审计证据。
- `改进策略`：后端状态、路由、runtime、数据或部署行为。
- `UI 表现`：卡片、badge、详情抽屉、Activity、诊断页等用户可见结果。
- `日志与诊断`：request log、安全审计日志、health/readiness、诊断字段。
- `测试与验收`：单测、集成、UI、容器或手工验收。
- `已完成事项`：历史已完成内容。
- `待解决事项`：仍需实现或产品决策的内容。
