# 02. 通用可审计性体系

状态：产品方向已确认，作为 v0.2.0 基础能力要求。

## 问题：解决难审计

Lune 大多数真实问题发生在 Docker 容器中，现场状态分散在 SQLite、auth 文件、运行时内存、Admin API、gateway request log、CPA runtime log 和浏览器当次响应里。很多问题不是单点错误，而是由 runtime reload、异步健康刷新、DB 写入、外部服务响应和用户连续操作共同触发。

这导致审计时经常遇到三类困难：

1. 现场状态在容器里，宿主机未必有直接读取权限或工具。
2. 关键错误只短暂存在于 HTTP response、前端状态或内存中，请求结束后无法恢复。
3. 问题依赖时序，单看最终 DB 或最终日志无法解释真实原因。

本次 CPA auth JSON 批量导入失败就是典型例子：UI 显示多个 item 失败，但 v0.1.9 未持久化导入 item 明细；日志只显示接口 `200` 和 auth file `CREATE/REMOVE`；必须通过隔离容器复现才抓到真实错误码 `pool_member_failed`。

v0.2.0 需要从“日志 + 现象审计”升级为“操作事件 + 阶段错误 + 关联 ID + 隔离复现 + 脱敏审计包”的通用体系。

## 总体原则

- 关键操作失败后，必须能在请求结束后恢复失败阶段和安全错误码。
- 审计数据必须默认脱敏，不记录 token、完整 auth JSON、完整 account key、完整 API key。
- 审计能力必须优先服务真实容器场景，不能只停留在单元测试。
- 审计和复现默认只读或隔离执行，不得伤害正在运行的正式/旧版本容器。
- 新功能 spec 必须写清自身可审计性要求，不得只描述成功路径。

## 1. 关键操作事件

所有会改变 Lune 状态或影响运行时行为的操作，都必须记录操作事件或等价审计记录。

覆盖范围至少包括：

- 添加、更新、删除账号。
- CPA auth JSON 单文件和批量导入。
- 删除 Pool、启停 Pool、修改 Pool routing policy。
- runtime reload、runtime binding 解析、auth index 同步。
- health refresh、models refresh、quota refresh、subscription refresh。
- diagnostic request、stateful probe、普通 force-account 请求。
- token 创建、reveal、删除、启停。
- 系统设置和通知配置变更。

每个操作事件至少包含：

| 字段 | 要求 |
| --- | --- |
| `operation_id` | 稳定关联 ID，贯穿 API、日志、DB 和结果页 |
| `operation_type` | 如 `cpa_import_batch`、`delete_account`、`runtime_reload` |
| `actor/source` | UI、API、health checker、entrypoint、system 等安全来源 |
| `target` | pool id、account id、account key hash、service id 等脱敏目标 |
| `started_at / finished_at` | 操作时间窗口 |
| `status` | `running / succeeded / failed / partial` |
| `error_code` | 机器可读错误码 |
| `safe_error_message` | 可展示、可审计、已脱敏的错误摘要 |
| `correlation_id` | 与 request log / runtime log / notification 关联的 ID |

## 2. 阶段错误模型

错误不得只记录为“失败”。涉及多步骤的操作必须标出失败阶段。

通用阶段命名示例：

| 阶段 | 语义 |
| --- | --- |
| `parse_input` | 解析请求、文件、JSON、表单 |
| `validate_identity` | 校验账号身份、provider、重复和冲突 |
| `write_file` | 写入或恢复 auth file、配置文件、临时文件 |
| `upsert_account` | 创建或更新账号 |
| `insert_pool_member` | 写 Pool membership |
| `commit_db` | 提交数据库事务 |
| `request_runtime_reload` | 写 reload signal 或调用 reload API |
| `wait_runtime_ready` | 等待 runtime 健康或 auth index 可见 |
| `refresh_models` | 模型发现 |
| `refresh_quota` | quota 刷新 |
| `refresh_subscription` | subscription 刷新 |
| `dispatch_notification` | 通知分发 |

错误记录应能表达：

```text
operation=cpa_import_batch
stage=insert_pool_member
status=failed
error_code=sqlite_busy
safe_message=database is locked while inserting pool member
target=account_key_hash:sha256:...
```

阶段错误模型是跨功能要求。未来新增功能时，必须在 spec 中列出该功能的关键阶段和失败映射。

## 3. 统一关联 ID

每次用户操作或系统批处理都必须生成 `operation_id` 或等价关联 ID。该 ID 必须贯穿：

- Admin API access log。
- 业务日志。
- DB 审计表。
- runtime reload 日志。
- health checker 日志。
- request_logs。
- notification delivery。
- 前端结果页或错误页。

目标是审计时可以按一个 ID 收敛证据：

```bash
docker logs lune | rg op_...
```

关联 ID 不应包含邮箱、token、account key 或其他敏感信息。

## 4. 最近操作快照

Lune 不需要无限保存所有细节，但必须保留最近关键操作的脱敏快照。

v0.2.0 的最近操作快照必须至少保留：

- 最近 200 条 operation。
- 每条 operation 最多 1000 个 item。
- 可配置保留天数，默认不低于现有 request log 保留窗口中的较短值。

快照应覆盖：

- 操作摘要。
- 每个 item 的最终状态。
- 每个 item 的阶段错误。
- runtime sync 状态。
- 关联 request id / operation id。
- 安全目标摘要。

超出保留窗口后可以按数据保留策略清理，但清理动作本身也应有审计事件。

## 5. 只读 `lune debug`

v0.2.0 必须在 Lune 二进制中提供只读 debug 子命令，方便容器内直接审计，而不是依赖临时安装 `sqlite3`、`jq` 或手写 SQL。

命令必须默认只读、默认脱敏、默认不修复。

必须提供以下只读命令；命令名是对外审计接口，不能在 v0.2.0 内省略：

```bash
lune debug summary
lune debug recent-operations
lune debug operation <operation_id>
lune debug db-integrity
lune debug cpa-auth
lune debug cpa-runtime
lune debug account <account_id>
```

这些命令要回答：

- DB 里有什么。
- 文件系统里有什么。
- runtime 当前看到什么。
- Lune 认为状态是什么。
- DB、文件和 runtime 是否一致。
- 最近谁改了目标对象。
- 失败发生在哪个阶段。

示例输出必须脱敏：

```text
hash              provider  email              plan  file  db   runtime
sha256:621ca...   codex     a***6@hotmail.com  plus  yes   yes  yes
```

## 6. 标准隔离复现脚本

仓库应提供标准化审计复现脚本，用于无法从现有现场恢复真实错因时创建隔离容器复现。

必须提供以下标准入口；实现可以拆分内部脚本，但该入口必须可用：

```bash
scripts/audit/repro.sh <scenario>
```

脚本职责：

1. 创建临时容器、临时 volume、临时端口。
2. 可选只读挂载用户提供的数据目录。
3. 调用 Admin API 或 Gateway API 复现场景。
4. 抓取 HTTP 响应、docker logs、DB 摘要和 auth 目录摘要。
5. 输出脱敏 evidence。
6. 删除临时容器和临时 volume。

初始场景至少包括：

- `cpa-import-reimport`
- `pool-delete`
- `runtime-reload`
- `quota-refresh`
- `stateful-probe`
- `routing-failover`

脚本可以依赖工具容器提供 `curl`、`jq`、`sqlite3`，不要求把这些工具打进生产镜像。

## 7. 脱敏审计包

v0.2.0 必须提供一键脱敏审计包导出能力，便于用户把现场证据交给维护者。

必须提供以下命令；输出内容可以随实现扩展，但脱敏和只读语义不能放宽：

```bash
lune debug collect --redact
```

输出结构：

```text
lune-audit-<timestamp>.tar.gz
  summary.txt
  recent-operations.json
  accounts.redacted.json
  pools.redacted.json
  cpa-auth-files.redacted.json
  runtime.redacted.json
  request-logs.redacted.json
  docker-env.redacted.txt
```

脱敏要求：

- 不包含 refresh token、access token、id token。
- 不包含完整 API key、Pool token、management key。
- 不包含完整 auth JSON。
- 不包含完整 account key。
- 邮箱默认 mask。
- request body 默认不导出；如需导出，只允许安全摘要。

## 8. 后续 spec 的可审计性要求

后续每个版本 spec 只要涉及状态变更、运行时行为、外部服务调用、异步任务或用户可见失败，都必须包含“可审计性”小节。

该小节必须回答：

| 问题 | 要求 |
| --- | --- |
| 失败后能否查到原因 | 请求结束后仍能查询失败阶段、错误码和安全摘要 |
| 是否有关联 ID | API、日志、DB、runtime 和前端结果页能用同一 ID 对齐 |
| 是否有阶段模型 | 列出关键阶段和阶段错误映射 |
| 是否脱敏 | 明确哪些字段不能进入日志、DB 和审计包 |
| 是否可复现 | 说明是否需要隔离容器矩阵或 fake upstream |
| 是否可清理 | 临时容器、临时数据卷、fake service 必须清理 |
| 是否影响旧容器 | 验证不伤害正在运行的旧版本容器 |

如果某功能没有可审计性要求，必须在 spec 中明确说明原因。涉及 Docker runtime、SQLite、CPA、quota、routing、Pool、账号生命周期的功能默认都必须提供可审计性要求。

## 测试要求

| ID | 场景 | 操作 | 期望 |
| --- | --- | --- | --- |
| AUD-01 | 操作事件持久化 | 执行成功和失败的账号导入、删除、runtime reload | `recent-operations` 可查询摘要、状态和关联 ID |
| AUD-02 | 阶段错误 | 构造 DB busy、runtime reload 失败、上游 401 | 审计记录包含阶段、错误码和安全摘要 |
| AUD-03 | 关联 ID | 执行一次批量导入 | API log、operation record、runtime reload log 可按同一 ID 对齐 |
| AUD-04 | 脱敏 | 导出审计记录和审计包 | 不包含 token、完整 auth JSON、完整 account key、完整 API key |
| AUD-05 | debug 只读 | 执行 `lune debug summary` 和 `lune debug cpa-auth` | 不修改 DB、文件或 runtime；输出稳定且脱敏 |
| AUD-06 | 隔离复现脚本 | 执行 `scripts/audit/repro.sh cpa-import-reimport` | 自动创建临时容器和 volume，输出 evidence，最终清理资源 |
| AUD-07 | 审计包导出 | 执行 `lune debug collect --redact` | 生成结构化脱敏包，可用于离线审计 |
| AUD-08 | 保留窗口 | 制造超过保留数量的操作 | 旧记录按策略清理，清理动作有审计事件 |
