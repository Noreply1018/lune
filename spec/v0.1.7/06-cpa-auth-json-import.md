# 06. Add Account 导入 CPA Auth JSON

状态：release-ready。功能实现、本地测试和 embedded CPA 容器主路径验收已经完成；最终 `gpt-5.5` subagent 严格审计已通过。

来源：用户希望在 v0.1.7 的 Add Account 流程里增加一个选项，可以直接导入已有 Codex / CPA auth JSON 文件，用于从旧容器、旧数据目录或其他 CLI 登录环境迁移账号凭证。目标是避免用户手动进入容器复制 `/app/data/cpa-auth/*.json`，也避免误拷 `.login-sessions.json`、数据库文件或其他无关数据。

## 问题

当前迁移 CPA 账号凭证主要依赖两种方式：

- 在新容器里重新走 Device Code 登录。
- 手动把旧容器的 auth JSON 拷入新容器的 `/app/data/cpa-auth/`，再通过远程账号导入流程创建 Lune 账号。

这两种方式对普通用户都不够直接：

- 重新登录成本高，且可能打断已有账号状态。
- 手动拷文件需要理解 Docker volume、容器路径、文件命名和 import 关系。
- 只拷 auth JSON 不等于账号已加入 Pool；还必须触发 Lune 的 CPA import / upsert。
- 用户容易误拷 `.login-sessions.json`、日志目录、整个数据库，甚至覆盖新容器已有凭证。
- 凭证 JSON 内含 refresh token / access token，任何前端日志、错误回显或截图泄露都属于高风险。

因此，Add Account 需要提供一个受控的“导入 auth JSON”入口，把“上传凭证文件、验证结构、写入 cpa-auth 目录、创建/更新账号、加入 Pool、刷新模型/额度/订阅”收敛成一个明确流程。

## 改进策略

在 Add Account 的 CPA / Codex 分支增加一个新的接入方式：

```text
Login with Codex
Import auth JSON
```

`Import auth JSON` 用于迁移已有 CPA auth file，不替代 Device Code 登录。

### 前端流程

1. 用户在 Add Account 中选择 `CPA / Codex` 来源。
2. 用户选择接入方式：
   - `Login with Codex`：保持现有 Device Code 流程。
   - `Import auth JSON`：进入文件导入流程。
3. 用户选择目标 Pool。
4. 用户上传一个 `.json` 文件。
5. 前端只展示安全摘要：
   - provider / type。
   - email。
   - account id。
   - plan type。
   - expired at。
   - disabled。
6. 用户确认后提交到后端。
7. 后端完成写入、upsert、加入 Pool 和刷新任务后，前端进入成功页。

前端不得展示或长期保存完整 JSON 内容。上传后只应保留 `File` 对象和后端返回的安全摘要；不得在 toast、console、错误面板或调试区输出完整 token 字段。

### 后端流程

后端新增一个受控导入接口，建议语义为：

```http
POST /admin/api/accounts/cpa/import-json
Content-Type: multipart/form-data

file=<auth json>
pool_id=<target pool id>
label=<optional label>
notes=<optional notes>
enabled=<optional bool, default true>
```

后端处理步骤：

1. 验证 CPA service 已配置，且 `cpa_auth_dir` 可写。
2. 读取上传文件，限制大小，例如 256 KiB 或更小。
3. 解析 JSON，不允许非 JSON、数组 JSON 或空对象。
4. 拒绝 `.login-sessions.json` 结构和缺少账号身份的文件。
5. 验证 `type/provider` 在允许列表内，首版只支持 `codex`。
6. 验证必要凭据字段存在：
   - `refresh_token` 必须存在。
   - `access_token` 或 `id_token` 至少一个存在。
   - `email` 或 `account_id` 至少一个存在。
7. 从 token 或 JSON 字段提取账号信息：
   - provider。
   - email。
   - account id / ChatGPT account id。
   - plan type。
   - subscription active until。
   - expired / last refresh。
   - disabled。
8. 按现有 `ScanAuthDirKeyed` / `ReadAuthFile` 规则生成稳定 `account_key` 和文件名，例如 `codex-email-plus.json`。
9. 写入临时文件后原子 rename 到 `cpa_auth_dir`，避免半写文件被 CPA 扫描。
10. 若同 key 已存在，按 upsert 语义覆盖凭证文件并更新已有 Lune 账号；不得创建重复账号。
11. 触发 embedded CPA reload signal 或等待 management auth-files 重新出现该 auth。
12. 调用现有 CPA import / upsert 逻辑创建或更新账号。
13. 将账号加入用户选定 Pool。
14. 异步刷新模型、Codex quota 和订阅信息。

后端响应只返回安全摘要和账号对象，不返回完整 auth JSON，不返回完整 token。

## UI 表现

Add Account 的 CPA 分支应避免把“登录”和“导入文件”混成一个按钮。建议使用明确的二选一分段控件或列表项：

- `Login with Codex`：用于新登录。
- `Import auth JSON`：用于迁移已有 auth file。

导入页面应包含：

- 一个 JSON 文件选择区域。
- 一个安全摘要预览区。
- 一个目标 Pool 确认区。
- 一个明确的风险说明：该文件包含账号凭据，只会写入当前 Lune 的 CPA auth 目录。
- 一个确认按钮，例如 `Import credential`。

错误状态要聚焦可行动作：

- `不是可识别的 CPA auth JSON`。
- `缺少 refresh_token`。
- `缺少账号 email / account_id`。
- `暂不支持 provider: <provider>`。
- `该凭据已存在，将更新现有账号`。
- `CPA runtime 尚未加载该凭据，正在同步`。

成功后应展示：

- 账号 label。
- email。
- provider。
- Pool。
- 导入状态。
- 模型/额度/订阅刷新状态：已完成、pending 或部分失败。

## 安全与边界

必须遵守以下边界：

- 不接受 `.login-sessions.json`。
- 不接受压缩包、目录上传或多个文件批量上传。
- 不允许用户指定最终文件名或路径。
- 文件名只能由后端根据 provider、email、plan/account 信息生成。
- 不允许路径穿越；所有写入必须落在配置的 `cpa_auth_dir` 内。
- 不在日志、错误响应、request log 或通知里输出完整 JSON、refresh token、access token、id token。
- 如果写入文件成功但账号 upsert 或加入 Pool 失败，需要回滚新写入文件；如果是覆盖已有文件，必须先安全备份或采用事务性策略，避免凭证丢失。
- 如果同账号已存在于其他 Pool，应复用账号并加入目标 Pool，而不是复制账号。
- 如果同 key 已存在但账号身份字段不一致，应拒绝覆盖，避免把 A 账号凭证写到 B 账号文件名下。

## 日志与诊断

建议记录以下安全摘要：

- import source: `uploaded_json`。
- generated account key。
- provider。
- email hash 或 masked email。
- account id 后 6 位或 hash。
- 是否创建新账号 / 更新已有账号。
- 是否写入 reload signal。
- RefreshAccount 的模型、quota、订阅结果摘要。

不得记录：

- 原始 JSON。
- refresh token。
- access token。
- id token。
- 完整 email 与完整 account id 同时出现在高频日志中。

诊断页可以展示：

```text
Credential source: uploaded JSON
Imported at: 2026-05-16 12:34:56
Provider: codex
Runtime sync: confirmed / pending / error
```

如果后端暂不新增字段，也可以先只通过 Activity / request log 之外的管理员 toast 展示导入结果，不强制进入首版 UI。

## 测试与验收

### 单元与集成测试

- 上传合法 Codex auth JSON 后，后端写入 `cpa_auth_dir`，创建账号并加入指定 Pool。
- 上传同一账号 JSON 时，更新已有账号，不创建重复账号。
- 上传 `.login-sessions.json` 被拒绝。
- 上传非 JSON、数组 JSON、空对象被拒绝。
- 上传缺少 `refresh_token` 的 JSON 被拒绝。
- 上传缺少 `email` 和 `account_id` 的 JSON 被拒绝。
- 上传 unsupported provider 被拒绝。
- 用户提交路径型文件名、带 `../` 的 filename 或奇怪扩展名时，后端不使用用户文件名生成目标路径。
- 写入文件成功但 upsert 失败时，新文件被回滚；覆盖已有文件时不会丢失旧凭证。
- 导入成功后触发 RefreshAccount，模型、quota、订阅至少进入 completed / pending / error 的可解释状态。
- 后端响应和日志不包含 `refresh_token`、`access_token`、`id_token` 字段值。

### 前端测试

- Add Account 中 CPA 分支显示 `Login with Codex` 和 `Import auth JSON` 两个互斥入口。
- 导入 JSON 页面只展示安全摘要，不展示完整 JSON。
- 合法文件显示 email/provider/account id/expired/disabled 摘要。
- 非法文件显示明确错误，且不进入确认导入。
- 导入成功后进入成功页，并能前往目标 Pool。
- 导入失败时不泄露 token 内容。

### 真实容器测试矩阵

必须使用新启动的 v0.1.7 测试容器和隔离数据目录，不复用正在运行的旧容器。可以使用从测试 fixture 生成的 fake auth JSON；不得在测试记录里保存真实 refresh token。

| 编号 | 场景 | 操作 | 期望 |
| --- | --- | --- | --- |
| CT-JSON-01 | 合法 Codex auth JSON 首次导入 | Add Account 选择 `Import auth JSON`，上传 fixture，选择 Pool | `/app/data/cpa-auth/<account_key>.json` 写入成功；账号创建并加入 Pool；Pool 页面可见该账号 |
| CT-JSON-02 | 同账号 JSON 重复导入 | 再次上传同一 account key 的 JSON | 更新已有账号，不新增重复账号；Pool member 幂等 |
| CT-JSON-03 | 拒绝 `.login-sessions.json` | 上传 login sessions fixture | 返回明确错误，不写入 auth file，不创建账号 |
| CT-JSON-04 | 拒绝缺 token JSON | 上传缺 `refresh_token` 的 JSON | 返回 400；响应和日志不包含上传内容 |
| CT-JSON-05 | 防路径穿越 | 上传 filename 为 `../../x.json` 的文件 | 后端忽略用户文件名，目标路径仍在 `cpa_auth_dir` 内 |
| CT-JSON-06 | Runtime reload / auth index 同步 | 导入后等待 CPA management auth-files | RefreshAccount 能拿到 auth index；模型/额度/订阅刷新进入可解释状态 |
| CT-JSON-07 | 覆盖失败回滚 | 模拟 upsert 或 pool add 失败 | 新文件回滚；已有文件不丢失；前端显示安全错误 |

容器测试完成后必须删除测试容器和临时数据目录。实现后已经使用 `lune:v0.1.7-blocker-test` 的 embedded CPA 隔离容器完成 CT-JSON-01 到 CT-JSON-06 主路径验收；CT-JSON-07 覆盖失败回滚由单元测试覆盖新文件回滚和已有账号/文件恢复。证据见 `100-release-evidence.md`。

## 完成记录

- 已确认该功能适合放在 Add Account 的 CPA 分支中，而不是 Settings 或手动运维文档里。
- 已明确首版只支持单文件 JSON 导入，不做批量导入、不导入数据库、不导入 `.login-sessions.json`。
- 已明确导入后必须走现有 CPA import / RefreshAccount 流程，不能只把文件落盘就算成功。
- 已实现 `POST /admin/api/accounts/cpa/import-json`，支持 multipart 单文件导入、校验、写入、upsert、加入 Pool、runtime reload、异步刷新和失败回滚。
- 已补充失败回滚测试：新文件失败回滚；已有账号覆盖导入失败时恢复旧 auth file 和账号快照。

## 后续非阻塞项

- 批量导入多个 auth JSON 已移入 `spec/draft/10-v0.1.7-deferred-followups.md`，不进入 v0.1.7 阻塞范围。
- “从旧 Lune 数据目录扫描导入”的高级运维工具已移入 `spec/draft/10-v0.1.7-deferred-followups.md`，不进入 v0.1.7 阻塞范围。
