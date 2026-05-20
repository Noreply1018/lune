# 01. CPA Auth JSON 删除后重导入失败

状态：真实容器问题已复现，修复待实现。

## 问题详情

在 v0.1.9 真实容器 `noreply1018/lune:0.1.9` 中，用户从 `C:\Users\lh\lune-data\cpa-auth` 批量导入 4 个 Codex CPA auth JSON。首次现象是预检通过，但正式导入后几乎全部失败，最终只剩 1 个账号。

用户随后删除所有账号后再次通过 Add Account 批量导入，UI 仍复现同类问题：

- Step 3 预检结果中 4 个账号文件均显示 `created`。
- Step 4 结果页中第 1 个文件显示“已创建 / 等待 runtime 同步”。
- 其余文件显示“失败 / 不适用”。
- 容器最终只保留 1 个 auth file 和 1 个 Lune account。

涉及源文件共 4 个 Codex Plus auth JSON。文档中只保留 masked 示例：

- `codex-a***6@hotmail.com-plus.json`
- `codex-c***u@mail.com-plus.json`
- `codex-j***0@hotmail.com-plus.json`
- `codex-j***7@hotmail.com-plus.json`

`.login-sessions.json` 不是 CPA auth JSON，预期应拒绝导入；它不是本问题根因。

## 审计结果

### 源文件排查

使用只读挂载源目录的临时容器读取 Windows 路径下的 JSON 文件，确认 4 个账号文件均包含必要字段：

- `type=codex`
- `email`
- `account_id`
- `refresh_token`
- `access_token`
- `id_token`
- `expired`
- `last_refresh`

对当前 `lune-0.1.9` 调用只读预检接口，4 个账号文件均返回 `action=created` 或 `action=updated`，没有格式错误。因此源 JSON 不坏。

### 干净容器复现

使用隔离容器 `noreply1018/lune:0.1.9`、新数据卷、新端口，直接批量导入同一批 4 个账号：

- 预检：`created=4, failed=0`
- 正式导入：`created=4, failed=0`
- 最终 auth 目录保留 4 个 auth JSON。

这说明 v0.1.9 并非在干净库首次导入时必然失败。

### 删除后重导入复现

在隔离容器中执行以下序列：

1. 创建 Pool。
2. 首次导入 4 个账号，全部成功。
3. 调用 `DELETE /admin/api/accounts/{id}` 删除 4 个账号。
4. 立即重新批量导入同一批 4 个账号。

第二次正式导入抓到隐藏错误码：

```json
{
  "summary": {
    "created": 2,
    "failed": 2,
    "pending_runtime_sync": 2
  },
  "items": [
    {
      "client_file_name": "codex-j***0@hotmail.com-plus.json",
      "status": "failed",
      "runtime_sync": "not_applicable",
      "error_code": "pool_member_failed",
      "error_message": "failed to add imported account to pool"
    },
    {
      "client_file_name": "codex-j***7@hotmail.com-plus.json",
      "status": "failed",
      "runtime_sync": "not_applicable",
      "error_code": "pool_member_failed",
      "error_message": "failed to add imported account to pool"
    }
  ]
}
```

当前真实容器用户截图中的“失败 / 不适用”与该错误码一致。

### 日志证据

失败窗口内日志表现为：

- 正式接口 `POST /admin/api/accounts/cpa/import-json-batch` 返回 `200`。
- 第一个 auth file `CREATE` 后触发 embedded CPA reload。
- 后续文件出现 `CREATE` 后又 `REMOVE`，或 CPA watcher 看到 `CREATE` 但读取时报 `no such file or directory`。
- embedded CPA 反复重启，且停止旧 runtime 时出现 `context deadline exceeded`。

示例行为：

```text
auth file changed (CREATE): codex-carter...
request completed POST /admin/api/accounts/cpa/import-json-batch status=200
auth file changed (REMOVE): codex-carter...
auth file changed (CREATE): codex-james...
failed to read auth file codex-james...: no such file or directory
entrypoint restarting embedded CPA after Lune reload signal
```

这些日志与批量导入单项回滚路径吻合：auth file 已写入，但 DB account + Pool membership 事务失败，随后删除新 auth file。

## 根因判断

直接失败码是 `pool_member_failed`，发生在 `UpsertCpaAccountAndPoolMemberFromImport` 的 DB 事务中。当前实现把底层 SQLite 错误吞成通用错误，导致用户和后续审计只能看到 `failed to add imported account to pool`。

触发条件是删除 CPA 账号后立即重新批量导入。此时存在多个交错动作：

- 删除账号会删除 auth file。
- 删除账号后会请求 embedded CPA reload。
- 批量导入每成功一个 item 后立即请求 embedded CPA reload。
- 导入成功后会异步触发模型发现、quota、subscription refresh。
- embedded CPA reload 由 entrypoint 独立执行，不受导入互斥锁保护。

因此，删除后的 runtime reload、导入中的 per-item reload、CPA watcher、异步健康刷新和 SQLite 写事务存在竞态窗口。该竞态会使部分 item 在 Pool membership 阶段失败并回滚。

等待一段时间让删除后的 reload 和异步刷新收敛后，再次导入同一批文件会全部成功。这进一步说明问题是删除后立即重导入的时序问题，不是 auth JSON 内容问题。

## 解决思路

### 1. 批量导入改为批次级 reload

批量导入期间不得每成功一个 item 就触发 embedded CPA reload。应改为：

1. 对所有文件完成解析、身份检查、auth file 写入、DB upsert 和 Pool membership。
2. 统计本批次成功写入或更新的 CPA account key。
3. 批次结束后只触发一次 CPA runtime reload。
4. 对成功 item 统一标记 `runtime_sync=pending` 或在可确认时标记 `synced`。

这样必须避免 CPA runtime 在批次中途扫描半完成状态。v0.2.0 不强制唯一实现，但必须满足结果型约束：runtime 只能消费 DB 已提交、auth file 已完整发布的原子视图。可接受实现包括 staging 目录后原子发布、manifest/symlink 快照切换、不可变 auth snapshot，或等价机制。单纯减少 reload signal 不足以证明满足该约束。

### 2. 删除账号 reload 与导入互斥

CPA auth file 删除、CPA auth file 批量导入和 embedded CPA reload 必须有一致的生命周期协调。v0.2.0 必须采用下列实现之一：

- 复用 `cpaImportMu`、新增 CPA auth lifecycle mutex，或实现账号/Pool 生命周期队列，覆盖删除账号、批量导入、诊断写回和 reload signal 写入。
- 删除账号时删除 auth file 和 DB account 后只请求一次 reload；如果连续删除多个账号，前端或后端应支持批量删除或 debounce reload。
- 导入开始前确认上一次 reload 已收敛，或把 reload 变成可排队、可合并的批次级动作。
- 同一账号或 Pool 的删除、导入、health refresh、quota refresh、subscription refresh 和诊断写回不得并发修改同一生命周期状态。

### 3. DB 错误必须可审计

`pool_member_failed` 必须保留底层安全错误摘要，至少包含：

- 阶段：复用通用阶段枚举，至少包括 `upsert_account`、`select_max_position`、`insert_pool_member`、`select_pool_member`、`commit_db`。如实现内部仍区分 `insert_account` / `update_account`，必须映射到 `upsert_account` 并保留安全子阶段。
- SQLite 错误类别：如 `sqlite_busy`、`constraint_failed`、`foreign_key`。
- 安全上下文：pool id、account id、account key hash、client file name。

不得记录完整 auth JSON、refresh token、access token、id token 或完整 account key。

### 4. 导入批次明细持久化

新增或等价实现导入审计记录，使正式导入响应在请求结束后仍可恢复：

- batch id
- pool id
- client file name
- account key hash
- action / final status
- runtime sync
- error code
- safe error message
- created account id / pool member id
- created at

Activity 或 Settings 不要求提供完整 UI，但 DB 或 admin API 必须能用于审计。

`runtime_sync` 必须是明确状态机，允许值至少包括：

| runtime_sync | 语义 | 允许的 item 主状态 |
| --- | --- | --- |
| `not_applicable` | item 未产生需要 runtime 同步的账号变更，如 invalid、skipped、duplicate | `skipped`、`failed` |
| `pending` | DB/auth file 已提交，等待 runtime reload 或确认 | `created`、`updated` |
| `synced` | runtime 已看到并加载该账号 | `created`、`updated` |
| `failed` | DB/auth file 已提交，但 runtime reload 或确认失败 | `created`、`updated` |

不得把 runtime sync 失败反向改写成导入失败；也不得在 DB/auth file 未提交的失败 item 上返回 `pending`。

### 5. 回滚与结果页语义保持一致

如果 auth file 已写入后 DB/Pool membership 失败：

- 新 auth file 必须删除。
- 已有 auth file 必须恢复。
- item 必须返回 `status=failed` 和可行动错误码。
- 结果页不得只显示“失败 / 不适用”，必须能展示安全原因，例如“加入 Pool 失败，已回滚”。

### 6. 崩溃与重启恢复

导入、删除和 reload 必须持久化 operation journal 或等价状态机。容器重启后必须能 reconcile：

- 启动时发现 `running` 操作超过安全窗口，必须标记为 `interrupted` 或继续完成可恢复阶段。
- 已写 staging 但未提交 DB 的 auth file 不得被 runtime 消费，启动时必须清理或隔离。
- DB 已提交但 runtime 未同步的账号必须保留为 `runtime_sync=pending` 或重新排队 reload。
- 已进入回滚阶段的 item 必须完成回滚或标记为 `failed` 并留下安全错误摘要。

reconcile 是启动流程或专用维护流程的写入行为，不属于 `lune debug`。`lune debug` 只能在 reconcile 后只读验证 operation、DB、auth file 和 runtime 状态。

## 测试矩阵

| ID | 场景 | 操作 | 期望 |
| --- | --- | --- | --- |
| CPA-IMP-01 | 干净库首次导入 | 新容器、新数据卷，导入 4 个合法 Codex auth JSON | `created=4, failed=0`；auth 目录保留 4 个文件；Pool 有 4 个账号 |
| CPA-IMP-02 | 包含 `.login-sessions.json` | 同批选择 `.login-sessions.json` 与 4 个账号文件 | `.login-sessions.json` 返回 `invalid_auth_json`；4 个账号文件成功 |
| CPA-IMP-03 | 删除后立即重导入 | 首次导入成功后，删除全部账号，立即再次导入同一批文件 | 不得出现 `pool_member_failed`；最终 4 个账号全部存在 |
| CPA-IMP-04 | 连续逐个删除后重导入 | 通过 UI 或 API 连续删除多个 CPA 账号，再立即导入 | 删除 reload 不得与导入写事务冲突；导入全部成功 |
| CPA-IMP-05 | 导入期间 runtime reload | 批量导入 4 个文件 | 只触发批次级 reload；日志中不应每个 item 都出现 reload signal |
| CPA-IMP-06 | SQLite busy 注入 | fake store 或测试夹具在 Pool membership 阶段制造短暂 busy | 有 bounded retry 或安全失败；错误明细可审计；auth file 回滚正确 |
| CPA-IMP-07 | Pool member 约束冲突 | 重复导入已有账号到同一 Pool | 返回 `updated` 或等价幂等结果，不创建重复 member |
| CPA-IMP-08 | 同批重复账号 | 同一批上传两个相同 account key 文件 | 第一份成功，第二份 `skipped + duplicate_in_batch` |
| CPA-IMP-09 | identity mismatch | 磁盘已有同 key 但身份字段不一致 | 返回 `identity_mismatch`；不覆盖 auth file |
| CPA-IMP-10 | runtime reload 失败 | DB 和 auth file 写入成功后 runtime reload 失败 | item 主状态仍为 `created/updated`，`runtime_sync=failed`；不回滚成功导入 |
| CPA-IMP-11 | 导入明细审计 | 完成一次含成功、跳过、失败的导入 | DB 或 admin API 可查询 batch 明细和每个 item 的 `error_code` |
| CPA-IMP-12 | 安全脱敏 | 查询导入审计明细和日志 | 不出现完整 token、完整 auth JSON、完整 account key |

## 验收证据要求

实现完成后必须使用容器实际验证，且不得伤害旧版本正在运行的容器：

- 使用新构建镜像和隔离数据卷。
- 使用临时端口。
- 使用源 auth JSON 的副本或只读挂载。
- 完成首次导入、删除后立即重导入、等待后重导入、包含 `.login-sessions.json` 的矩阵。
- 测试结束删除临时容器和临时数据卷。
