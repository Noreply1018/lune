# 04. CPA 凭据生命周期、重复账号、删除与重登

## 问题

CPA 账号过去是“数据库账号行”和“磁盘 auth file”松散同步，导致重复账号、同名重登污染和删除语义不明确。

2026-05-10 的生产审计暴露了两类问题。

### 重复账号

同一个 CPA credential key 被导入成两条 Lune 账号记录：

- account `id=6`：`cpa_account_key=codex-rebeccanicholson4700@outlook.com-plus`，`cpa_credential_status=ok`。
- account `id=8`：同一个 `cpa_account_key`，`cpa_credential_status=needs_login`，reason 为 `auth_failed`。
- 两条记录都挂在同一个 Pool。
- `id=6` 在请求日志中持续服务 `gpt-5.5` 且 HTTP 200。
- `id=8` 的失败状态来自后台 Codex quota 刷新路径 `wham/usage` 401，而不是真实模型生成请求失败。

根因是导入路径不幂等，Pool attach 也没有把“已有同 key 账号”当作同一个资源处理。

### 同名凭据重登污染

用户删除旧账号后重新登录同一邮箱，新账号仍显示 `needs_login`。

关键时间线：

- `2026-05-10 12:10:27` 删除旧 `id=6`。
- `2026-05-10 12:10:41` 删除旧 `id=8`。
- `2026-05-10 12:10:50` 开始新的 CPA Device Code 登录。
- `2026-05-10 12:11:38` 新登录成功，登录会话显示新的 `cpa_last_refresh_at`。
- `2026-05-10 12:11:39` 登录收尾阶段首次 quota 刷新返回 `HTTP 401`。
- 后台持续出现 `fetch codex quota account_id=8 err="HTTP 401"`。
- 磁盘 auth file 又显示旧 `last_refresh=2026-05-10T08:01:42Z`，数据库也被同步回旧值。

根因链路：

1. 删除 CPA 账号只删除 DB `accounts` 行，不删除同名 auth file，也不通知 embedded CPA runtime 释放或重载旧 auth index。
2. 重新 Device Code 登录会覆盖同名 auth file，但不会无条件重启或 reload embedded CPA runtime。
3. `resolveAuthMetadata` 只要看到 CPA management `/auth-files` 已存在 `auth_index`，就直接复用。
4. 它没有校验 runtime 内存凭据与磁盘新 token 是否一致。
5. CPA runtime 可能继续用旧内存 auth entry 调用 `wham/usage`，返回 401。
6. quota 401 被 Lune 写成 `needs_login`，UI 显示“需要重新登录”。
7. 如果 CPA runtime 又把旧内存凭据落回同名 auth file，`syncCpaMetadata` / `resolveCpaRuntime` 会把旧 metadata 写回 DB。

## 改进策略

把 CPA 账号改成可审计、可幂等、可重登的生命周期：

- 同一个 `cpa_service_id + cpa_account_key` 只能对应一条 Lune account。
- Device Code 登录、远程导入、批量导入必须复用同一套 upsert 逻辑。
- 删除、覆盖、重登同名 auth file 后，embedded CPA runtime 必须 reload 或重启到新状态。
- quota 初次失败不能立即把刚登录账号定性为 `needs_login`。
- 磁盘旧 metadata 不能覆盖数据库中更新后的登录状态。

### 唯一性与幂等导入

- 对 `source_kind='cpa' AND cpa_service_id IS NOT NULL AND cpa_account_key <> ''` 建立唯一约束：`UNIQUE(cpa_service_id, cpa_account_key)`。
- 启动检查必须检测重复 CPA key。若已有重复记录，应输出可诊断信息，不能静默忽略。
- Device Code 登录、远程单账号导入、批量导入统一走 CPA account upsert。
- 同一 `cpa_account_key` 重新登录时更新已有账号和 Pool membership，不创建重复账号。
- Pool attach 必须幂等；已有账号加入已有 Pool 时不创建新账号，也不产生用户不可见的重复状态。

### Auth metadata 校验

`resolveAuthMetadata` 不应只以“存在 auth index”为 ready 条件。它必须校验 runtime metadata 与磁盘 auth file 的关键版本信息，例如：

- account key。
- OpenAI account id。
- `last_refresh`。
- 文件 mtime。
- token 指纹或其他安全不可逆版本标识。

如果同名 auth file 被新 token 覆盖，但 runtime 仍返回旧 auth index，应触发 CPA reload，并在 reload 后重新读取 auth metadata。

### Runtime reload

- 删除、覆盖或重新登录同名 CPA auth file 后，embedded CPA runtime 必须强制 reload 或重启。
- 不能只在 auth index 缺失时触发 reload。
- 覆盖/重新登录后的 reload 已完成；删除语义仍需单独设计并补齐。

### Metadata 写回保护

`syncCpaMetadata` / `resolveCpaRuntime` 从磁盘 auth file 写回 DB 时，必须防止旧 metadata 覆盖更新后的登录状态。

最低要求：

- 基于 `last_refresh` 或单调版本比较新旧。
- 老版本 metadata 不得覆盖新版本 DB 字段。
- 写回时记录来源、前值、后值和版本。

### 首次刷新与 quota 失败

登录完成后的首次 refresh 如果 quota 失败，不应立即写 `credential_status=needs_login`。

规则：

- quota 401/403 优先写 `quota_status=error`。
- 只有 refresh token 明确失效、auth file 缺失/损坏、CPA runtime 明确要求重新认证，或 quota 失败后模型调用也认证失败，才升级为 `credential_status=needs_login`。
- 刚登录账号应允许通过后续模型调用或明确认证探针确认真实状态。

### 删除语义

删除 CPA 账号必须明确 auth file 语义，不能继续隐式残留。

待选择的产品策略：

- 删除账号时同步删除 auth file。
- 删除账号时默认保留 auth file，但 UI/API 明确提示凭据文件仍保留。
- 提供删除账号与删除凭据文件两个选项。

无论选择哪种策略，都必须：

- 让 UI/API 文案明确结果。
- 对 embedded CPA runtime 执行 reload 或释放旧 auth entry。
- 写入安全审计日志。

## UI 表现

前端“需要重新登录”不能再作为所有 CPA 异常的默认文案。

应区分展示：

- quota 获取失败：`额度查询失败`。
- 订阅 metadata 获取失败：`订阅元数据获取失败`。
- CPA runtime 未就绪或异常：`CPA runtime 初始化中` 或 `CPA runtime 异常`。
- 浏览器登录正常但本地 CPA 凭据异常时，应解释两者不是同一个登录态。

当多个启用账号拥有相同 `(cpa_service_id, cpa_account_key)` 时：

- 显示“重复导入”。
- 标注哪些账号行共享同一 credential key。
- 提供安全清理入口。
- 清理前说明是否删除 auth file、是否只禁用账号、是否从 Pool 移除。

删除 CPA 账号时必须明确：

- 是否删除数据库 account row。
- 是否删除 Pool membership。
- 是否删除磁盘 auth file。
- 是否触发 embedded CPA runtime reload。

如果 auth file 默认保留，UI 必须明说“凭据文件仍保留”；如果同步删除，也必须明说该操作会使后续重登需要重新授权。

## 日志与诊断

登录、删除、reload、metadata sync 和 quota refresh 必须写入安全审计日志。

日志应包含：

- account id。
- account key。
- auth file version。
- runtime auth index。
- 状态前后值。
- 失败来源。
- HTTP 状态。
- normalized reason。

日志不得包含 refresh token、access token、完整 auth file、完整 prompt 或 request body。

启动检查发现重复 CPA account key 时，必须给出明确诊断，包含重复 key、涉及账号 id 和建议清理路径。

## 测试与验收

- Device Code 登录同一个已存在 CPA account key 时，更新现有账号，不创建重复账号。
- 删除 CPA 账号后重新登录同一个 account key，embedded CPA runtime 使用新 token。
- 重新登录后的首次 quota refresh 不继续使用旧 auth index。
- 同名 auth file 被覆盖后，`resolveAuthMetadata` 能检测 runtime metadata 与磁盘 metadata 不一致并触发 reload。
- `syncCpaMetadata` 不会用旧 `last_refresh` 覆盖更新后的 DB 登录状态。
- batch import 遇到已存在 CPA account key 时跳过或更新，不创建重复账号。
- quota HTTP 401 不会在普通模型请求仍健康时单独把账号展示为“需要重新登录”。
- 重复 CPA account key 出现在数据库中时，启动检查能给出明确诊断。
- 删除账号时，用户能明确知道 auth file 是删除还是保留；runtime 状态与该选择一致。

## 已完成事项

- 增加 CPA 账号唯一性约束方向：`UNIQUE(cpa_service_id, cpa_account_key)`。
- 增加启动前重复检测。
- Device Code 登录同一个已存在 CPA account key 时更新现有账号，不创建重复账号。
- 同一 `cpa_account_key` 重新登录时更新已有账号和 Pool membership。
- Pool attach 幂等。
- 覆盖/重新登录同名 auth file 后触发 reload。
- `resolveAuthMetadata` 检测 runtime metadata 与磁盘 metadata 不一致并触发 reload。
- `syncCpaMetadata` 不会用旧 `last_refresh` 覆盖更新后的 DB 登录状态。
- batch import 遇到已存在 CPA account key 时跳过或更新，不创建重复账号。
- quota HTTP 401 不会在普通模型请求仍健康时单独把账号展示为“需要重新登录”。
- 重复 CPA account key 出现在数据库中时，启动检查给出明确诊断。
- 普通模型转发已复用 runtime auth metadata，并将 runtime auth id/index 写入请求日志，便于后续确认重登后实际执行的是新 auth file。
- embedded CPA reload 和 runtime binding 失败会进入明确错误路径，不再静默回落到 provider 级默认调度。

## 待解决事项

- 删除 CPA 账号时的 auth file 语义仍需产品决策和实现。
- 删除 auth file 或删除账号后的 embedded CPA runtime reload 仍需补齐。
- UI 增加重复账号检测与安全清理入口。
- 登录、删除、reload、metadata sync、quota refresh 的安全审计日志仍需补齐。
