# 05. CPA Auth JSON 多账号导入

状态：已实现并完成新容器验收。本文定义 v0.1.8 必须实现的 CPA auth JSON 多账号导入能力、导入结果审计、重复账号处理和验收矩阵。

来源：`spec/draft/10-v0.1.7-deferred-followups.md` 中的 “Auth JSON 批量或目录导入”，以及 v0.1.7 已完成的单文件 `Import auth JSON` 能力。v0.1.8 只纳入多文件批量导入，不纳入旧数据目录扫描。

## 问题

v0.1.7 已支持在 Add Account 中导入单个 CPA auth JSON。迁移多个 Codex / CPA 账号时，用户仍需要重复执行选择文件、确认、导入、返回 Pool 的流程。

这在 0.1.8 的目标场景里会变得更明显：

- 用户可能从旧容器或旧数据目录迁移多个 `cpa-auth/*.json`。
- Free / Go / Plus 账号会混合存在，需要逐个展示计划、access、quota 和导入结果。
- 批量导入会放大重复账号、同 key 覆盖、部分失败、runtime reload pending 和日志脱敏风险。
- 如果没有结果审计，用户无法确认哪些账号创建、哪些账号更新、哪些账号被跳过。

v0.1.8 需要把单文件导入扩展成受控的多文件导入，但不能扩大到目录扫描、压缩包导入或整库迁移。

## 范围

本功能只覆盖：

- 在现有 Add Account 的 CPA / Codex `Import auth JSON` 流程中选择多个 `.json` 文件。
- 每个文件独立校验、独立生成安全摘要、独立导入。
- 支持同一批次内部分成功、部分失败。
- 支持同账号幂等更新，不创建重复 Lune 账号。
- 支持导入结果页展示导入主状态 `created`、`updated`、`skipped`、`failed`，并单独展示 runtime sync 状态。
- 支持安全审计摘要，不记录完整 token 或完整 auth JSON。

本功能不覆盖：

- 目录上传或服务器端扫描旧 Lune 数据目录。
- `.zip`、`.tar`、`.gz` 等压缩包导入。
- 数据库迁移或 Pool 配置迁移。
- 从 `.login-sessions.json` 派生 auth file。
- 用户指定最终文件名或写入路径。
- 完整 CPA 凭据清理 UI。

## UI 流程

Add Account 的 CPA / Codex 分支保留二选一入口：

```text
Login with Codex
Import auth JSON
```

选择 `Import auth JSON` 后，v0.1.8 支持多文件选择：

1. 用户选择目标 Pool。
2. 用户选择一个或多个 `.json` 文件。
3. 前端调用只读预检接口，展示每个文件的安全摘要和预计动作。
4. 用户确认后，前端调用正式批量导入接口。
5. 结果页展示每个文件的最终状态。
6. 用户可以返回目标 Pool，确认账号已经加入 Pool。

前端流程必须是：

```text
选择文件 -> 安全预检 -> 确认导入 -> 结果页
```

安全预检只用于让用户在写入前确认将要处理的文件。预检不能写入 auth file、不能创建或更新账号、不能新增 Pool member、不能触发 CPA runtime reload，也不能把账号标记为可路由。

预检结果不是最终导入结果。正式导入时后端必须重新执行完整校验、重复检测、身份冲突检测、写入安全检查和 Pool membership 检查，不能信任前端提交的预检结果。

前端摘要只允许展示：

- 原始文件名的安全展示名。
- provider。
- plan type，例如 `Free`、`Go`、`Plus`。
- masked email。
- account id 后 6 位或 hash。
- 是否与现有 account key 重复。
- 将执行的动作：创建、更新、跳过或无法导入。

前端不得展示完整 JSON、`refresh_token`、`access_token`、`id_token`、完整 account id 或完整 auth file 路径。

## 后端接口

实现可以复用 v0.1.7 单文件接口并新增批量参数，也可以新增批量接口，但必须区分只读预检接口和正式导入接口。

只读预检接口语义如下：

```http
POST /admin/api/accounts/cpa/import-json-batch/preview
Content-Type: multipart/form-data

files=<auth json files>
pool_id=<target pool id>
```

预检接口要求：

- 单次最多文件数为 20 个。
- 单文件大小沿用单文件导入限制，最大 256 KiB。
- 总请求大小必须有上限，避免一次上传过多凭据。
- 每个文件都按 v0.1.7 单文件规则做只读校验。
- 返回每个文件的安全摘要、重复检测结果和预计动作。
- 不写入 auth file，不创建或更新账号，不新增 Pool member，不触发 runtime reload，不触发分层刷新。
- 请求级条件失败时返回安全错误，例如 Pool 不存在、CPA service 未配置；响应不得包含完整 auth JSON。

正式批量导入接口语义如下：

```http
POST /admin/api/accounts/cpa/import-json-batch
Content-Type: multipart/form-data

files=<auth json files>
pool_id=<target pool id>
enabled=<optional bool, default true>
```

接口要求：

- 单次最多文件数为 20 个。
- 单文件大小沿用单文件导入限制，最大 256 KiB。
- 总请求大小必须有上限，避免一次上传过多凭据。
- 每个文件都按 v0.1.7 单文件规则重新校验，不能复用或信任前端预检结果。
- 任一文件失败不得阻断其他文件导入，除非请求级条件失败，例如 Pool 不存在、CPA service 未配置、auth dir 不可写。
- 响应按文件返回安全结果，不返回完整 auth JSON。

响应必须至少包含以下安全结果结构。`status` 只表达导入主结果；`runtime_sync` 单独表达 CPA runtime auth index / metadata 是否已同步：

```json
{
  "pool_id": 1,
  "summary": {
    "created": 2,
    "updated": 1,
    "skipped": 1,
    "failed": 1,
    "pending_runtime_sync": 1
  },
  "items": [
    {
      "client_file_name": "codex-a.json",
      "status": "created",
      "provider": "codex",
      "plan_type": "free",
      "masked_email": "a***@example.com",
      "account_key_hash": "sha256:...",
      "account_id_suffix": "123456",
      "account_id": 10,
      "pool_member_id": 30,
      "runtime_sync": "pending"
    }
  ]
}
```

响应不得返回完整 `account_key`。需要跨日志或 UI 对齐同一账号时，返回 hash 或短摘要。

`summary.pending_runtime_sync` 是从 item 的 `runtime_sync='pending'` 派生出来的计数，不是 item 的主 `status`。例如新账号已创建但 runtime 尚未同步时，item 应返回 `status='created'` 且 `runtime_sync='pending'`。

## 导入语义

每个文件按以下步骤处理：

1. 验证 CPA service 已配置，且 `cpa_auth_dir` 可写。
2. 读取上传文件，验证大小和 JSON 对象结构。
3. 拒绝 `.login-sessions.json`、数组 JSON、空对象、非 JSON、unsupported provider、缺少必要 token 或缺少账号身份字段。
4. 生成稳定 account key 和目标 auth file 名称；不得使用用户上传文件名作为最终路径。
5. 检测同批次重复 account key。
6. 检测磁盘和数据库中已有 account key。
7. 对新账号执行可补偿写入、账号 upsert、加入目标 Pool、CPA reload / runtime sync、异步 refresh。
8. 对已有账号执行幂等更新，复用同一个 Lune account，并确保加入目标 Pool。
9. 对同批次重复文件，默认只处理第一份；同批次中排在后面的同 key 项主状态必须为 `skipped`，重复原因写入 `error_code='duplicate_in_batch'` 或等价安全错误摘要。
10. 对单项失败执行单项回滚，不影响同批次其他项。

批量导入不得承诺跨文件系统、数据库和 CPA runtime reload 的严格全局原子事务。实现必须采用可补偿事务口径：

- DB account upsert 与 Pool membership 必须在同一个数据库事务中完成；失败时回滚数据库变更。
- auth file 写入必须使用临时文件 + rename 或等价安全写入；覆盖已有文件前必须备份旧内容。
- DB 事务失败时，新 auth file 必须删除；已有 auth file 必须恢复备份。
- runtime reload / auth index sync 失败不回滚已经成功的导入主状态；item 保持 `created` 或 `updated`，并把 `runtime_sync` 写为 `pending` 或 `failed`。
- 异步 Access / Quota / Models refresh 失败不把导入主状态改成 `failed`，只进入刷新摘要或 Diagnostics。
- 单个 item 的补偿失败必须写入安全日志和审计摘要，但不得影响同批次其他 item 的已成功结果。
- 如果补偿失败导致 auth file 或 DB 状态可能不一致，该 item 必须返回或记录为 `status='failed'`，并使用 `error_code='compensation_failed'` 或等价安全错误码；不能只写日志而让结果页看起来成功。

并发导入必须靠数据库唯一约束、文件锁或等价互斥保护 account key。两个请求同时导入同一 account key 时，最终只能有一个 DB account、一个目标 Pool member 和一个最终 auth file；另一个请求必须返回 `updated`、`skipped` 或安全失败，不能产生重复账号。

同账号重复导入必须遵守：

- 不创建重复 DB account。
- 不创建重复 Pool member。
- 覆盖已有 auth file 前必须能恢复旧文件。
- 如果同 key 的身份字段冲突，必须拒绝覆盖。
- 如果同账号已经在其他 Pool，导入到目标 Pool 时复用账号并新增或复用目标 Pool membership。

## 结果状态

批量导入结果必须拆成两个维度：导入主状态和 runtime sync 状态。

导入主状态只允许以下四个值：

| 状态 | 语义 |
| --- | --- |
| `created` | 新 auth file 写入成功，新 Lune account 创建成功，并已加入目标 Pool |
| `updated` | 已有 account key 被安全更新，Lune account 复用，并已加入目标 Pool |
| `skipped` | 文件未导入，通常是同批次重复或用户确认前选择跳过 |
| `failed` | 文件导入失败，未写入最终 auth file，或已回滚 |

Runtime sync 状态只允许以下四个值：

| 状态 | 语义 |
| --- | --- |
| `synced` | CPA runtime auth index / metadata 已确认看到该 auth file |
| `pending` | 文件和账号已写入，但 CPA runtime auth index / metadata 尚未确认 |
| `failed` | runtime reload、auth index 同步或 metadata 确认失败 |
| `not_applicable` | 该 item 没有 runtime 同步动作，例如 `skipped` 或请求级失败 |

结果页可以把 `runtime_sync='pending'` 展示为“等待 runtime 同步”或等价文案，但不得把 `pending_runtime_sync` 当作 item 的主 `status`，以免遮蔽该文件本身是 `created` 还是 `updated`。

单项错误必须可行动：

- `不是可识别的 CPA auth JSON`
- `不支持 provider`
- `缺少 refresh_token`
- `缺少账号 email / account_id`
- `同批次重复账号`
- `目标 Pool 不存在`
- `CPA auth 目录不可写`
- `账号身份与已有 auth file 冲突`
- `写入后同步 CPA runtime 超时`

错误响应、toast、日志和审计摘要不得包含 token、完整 JSON 或完整 auth file 内容。

## 导入审计

v0.1.8 必须为批量导入保留安全审计摘要。审计摘要可以落在现有 notification / activity 机制中，也可以落在新增轻量 import result 记录中；最低要求是导入完成后管理员能看到本次导入摘要。

审计摘要至少包含：

- import batch id。
- target pool id / label。
- item count。
- created / updated / skipped / failed count。
- runtime_sync 的 synced / pending / failed / not_applicable count；如果需要汇总展示，可派生 `pending_runtime_sync` 计数。
- 每个 item 的 provider、plan type、masked email、account id suffix 或 hash。
- 每个 item 的导入主状态、runtime sync 状态和安全错误摘要。
- runtime reload / auth index sync 结果。
- refresh model / quota / access 的 pending / success / error 摘要。

不得记录：

- 原始 JSON。
- refresh token。
- access token。
- id token。
- 完整 request body。
- 完整 auth file。

## 与分层可路由模型的关系

批量导入只负责凭据进入 Lune 和目标 Pool，不直接把账号标记为可路由。

导入完成后必须触发或排队执行 `04-cpa-routability-layer-model.md` 定义的刷新流程：

```text
Credential
Runtime Binding
Access
Quota
Serving
Models
```

结果页可以显示刷新状态，但不能把“导入成功”写成“账号可路由”。Free / Go 账号尤其必须等待 access evidence；没有 access evidence 时，账号应显示 `pending` 或 `unknown`，而不是订阅异常。

## 测试与验收

### 单元与集成测试

- 批量上传两个合法 Codex auth JSON，分别创建账号并加入目标 Pool。
- 批量预检两个合法 Codex auth JSON，只返回安全摘要和预计动作，不写入 auth file、不创建账号、不加入 Pool。
- 导入成功但 runtime 尚未确认时，item 返回 `status='created'` 或 `status='updated'`，同时返回 `runtime_sync='pending'`，不得返回 `status='pending_runtime_sync'`。
- 批量上传一个新账号和一个已有账号，结果分别为 `created` 和 `updated`。
- 同批次上传两个相同 account key，只处理第一份；第二份返回 `status='skipped'`，并用 `error_code='duplicate_in_batch'` 或等价安全错误摘要说明原因。
- `runtime_sync` 只允许 `synced`、`pending`、`failed`、`not_applicable`，这些值不得出现在 item 主 `status` 中。
- 批量中一个合法、一个缺少 `refresh_token`，合法项成功，非法项失败且不影响合法项。
- 上传 `.login-sessions.json` 被拒绝，不写入 auth file，不创建账号。
- 上传数组 JSON、空对象、非 JSON 被拒绝。
- unsupported provider 被拒绝。
- 用户上传文件名包含 `../` 时，后端不使用用户文件名生成目标路径。
- 目标 Pool 不存在时，请求级失败，不写入任何 auth file。
- 预检后、确认导入前状态变化时，正式导入重新校验并按最新状态返回 `created` / `updated` / `skipped` / `failed`。
- 覆盖已有 auth file 后 upsert 失败时恢复旧文件和旧账号快照。
- DB upsert 或 Pool member 添加失败时，数据库事务回滚；新 auth file 删除，已有 auth file 恢复备份。
- runtime reload / auth index sync 失败时，item 主状态仍为 `created` / `updated`，`runtime_sync` 为 `pending` 或 `failed`，不把成功导入回滚成 failed。
- 补偿失败导致文件或 DB 可能不一致时，item 返回 `status='failed'` 且 `error_code='compensation_failed'` 或等价安全错误码；结果页和审计摘要必须可见。
- 两个并发请求导入同一 account key 时，最终没有重复 DB account、重复 Pool member 或损坏 auth file；失败方或后完成方返回可解释安全状态。
- 导入成功后触发 refresh，Access / Quota / Models 至少进入 completed / pending / error 的可解释状态。
- 后端响应和日志不包含 token 字段值。

### 前端测试

- Add Account 的 CPA 分支 `Import auth JSON` 支持选择多个 `.json` 文件。
- 选择多个 `.json` 文件后先展示安全预检结果，确认前不创建账号、不写入 auth file。
- 多文件预览只展示安全摘要，不展示完整 JSON。
- 用户确认后才调用正式导入接口；导入结果页展示最终结果而不是沿用预检预计动作。
- 批量结果页按文件展示导入主状态 `created`、`updated`、`skipped`、`failed`，并单独展示 runtime sync 状态。
- runtime sync pending 时，结果页显示“等待 runtime 同步”或等价文案，但仍保留该 item 的导入主状态。
- 部分失败时，成功项仍显示为成功，并可前往目标 Pool 查看。
- 同批次重复账号有明确提示，不创建重复卡片。
- Free / Go / Plus 计划在导入预览和结果页显示为计划身份，不显示为订阅异常。
- 导入失败 toast 和错误面板不泄露 token 内容。

### 容器验收

实现后必须使用新启动的 v0.1.8 测试容器完成验收，不能复用正在运行的旧版本容器。

最小容器验收矩阵：

| 编号 | 场景 | 操作 | 期望 |
| --- | --- | --- | --- |
| CT-MJSON-00 | 只读预检 | Add Account 选择 `Import auth JSON`，一次选择两个 fixture，仅停留在预检结果页 | 展示安全摘要和预计动作；auth dir、DB account、Pool member 均无新增或更新；runtime/auth index/refresh 队列无新增任务 |
| CT-MJSON-01 | 两个合法 auth JSON 首次导入 | Add Account 选择 `Import auth JSON`，一次上传两个 fixture，选择 Pool | 两个 auth file 写入成功；两个账号创建并加入 Pool；Pool 页面可见 |
| CT-MJSON-02 | 新账号 + 已有账号混合导入 | 再次上传一个已有账号和一个新账号 | 已有账号为 `updated`，新账号为 `created`；没有重复 DB account 或 Pool member |
| CT-MJSON-03 | 同批次重复账号 | 一次上传两个同 account key fixture | 第一项成功，第二项 `status='skipped'` 且原因是 `duplicate_in_batch`；最终只有一个账号 |
| CT-MJSON-04 | 部分失败 | 一次上传一个合法 JSON 和一个缺 token JSON | 合法项成功；非法项失败；结果页展示部分失败；日志不泄露上传内容 |
| CT-MJSON-05 | 防路径穿越 | 上传 filename 为 `../../x.json` 的文件 | 后端忽略用户文件名，目标路径仍在 `cpa_auth_dir` 内 |
| CT-MJSON-06 | Runtime sync 与分层刷新 | 导入后等待 CPA management auth-files 和 refresh | Credential / Runtime Binding / Access / Quota / Models 进入可解释状态 |
| CT-MJSON-07 | 覆盖失败回滚 | 模拟 upsert 或 pool add 失败 | 新文件回滚；已有文件不丢失；前端显示安全错误 |
| CT-MJSON-08 | 预检后状态变化 | 预检显示新账号后，在确认前另一路径先导入同账号，再点击确认导入 | 正式导入重新校验，返回 updated / skipped / failed 等最新结果，不盲信预检 |
| CT-MJSON-09 | runtime pending 双状态 | runtime auth index 尚未确认 | item 主状态仍为 created / updated；runtime_sync 为 pending；页面不把 pending_runtime_sync 当主状态 |
| CT-MJSON-10 | runtime sync 枚举边界 | 构造 synced / pending / failed / not_applicable 四种 runtime sync 结果 | 四种值只出现在 runtime_sync 字段；item 主 status 仍只为 created / updated / skipped / failed |
| CT-MJSON-11 | 并发重复导入 | 两个客户端同时上传同一 account key fixture | 最终只有一个 DB account、一个目标 Pool member 和一个 auth file；两个响应均为可解释安全状态 |
| CT-MJSON-12 | runtime reload 失败不回滚导入 | 模拟 auth file 与 DB 写入成功但 runtime reload 失败 | item 主状态为 created / updated；runtime_sync 为 failed 或 pending；账号仍在 Pool 中可见但不可直接宣称可路由 |
| CT-MJSON-13 | 补偿失败可见 | 模拟回滚 auth file 失败或 DB / 文件状态可能不一致 | item 主状态为 failed，带 `compensation_failed` 或等价安全错误码；审计摘要可见 |
| CT-MJSON-14 | 测试清理 | 完成上述验收 | 删除测试容器和临时数据目录或 volume |

实现代码已进入 v0.1.8，本轮已执行新容器验收并清理测试容器。

## 已确认口径

- v0.1.8 纳入多文件 auth JSON 导入，不纳入目录扫描。
- 批量导入是 Add Account 的 CPA / Codex 导入能力增强，不新增独立迁移工具页。
- 导入成功不等于账号可路由；可路由性由分层模型刷新结果决定。
- 同账号导入必须幂等更新，不创建重复账号。
- 同批次重复 account key 默认不覆盖前一项。
- 并发重复 account key 导入必须由唯一约束、文件锁或等价互斥兜底，不能生成重复账号或损坏 auth file。
- 批量导入允许部分成功、部分失败。
- 导入采用可补偿事务口径：DB 事务、auth file 备份/恢复、runtime sync 独立状态三者分层处理。
- 批量结果和审计只记录安全摘要，不记录完整凭据。
