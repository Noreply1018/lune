# 更新日志

这里记录 Lune 每个版本中值得用户关注的变化。

Lune 目前仍处于早期 `0.x` 阶段。版本会尽量遵循语义化版本，但在默认体验、部署方式、配置形态还没有完全稳定前，minor 版本可能会调整产品边界。

## [未发布]

暂无。

## [0.1.9] - 2026-05-17

状态：已发布。代码实现、自动化测试、真实容器验收矩阵、subagent 严格审计、发布文档提交、`v0.1.9` tag、GHCR / Docker Hub 多架构镜像推送和 GitHub Release 均已完成。

### Pool 管理体验

- 侧边栏 Pools 行新增低噪声 `+` 入口，可直接新建空 Pool；弹窗提供 OpenAI / Claude / Gemini / Codex 等推荐命名提示。
- Overview 首次空状态保留“添加账号”主入口，并新增“只创建 Pool”的次级入口，复用同一个新建 Pool 弹窗。
- Add Account 内的“新建 Pool”快捷路径保留，用于接入账号时顺手创建归属 Pool。
- Pool 详情页在“自检 Pool”和路由策略按钮右侧新增“更多设置”，集中承载 Pool 重命名、启用/停用与删除。

### Pool 删除语义

- 删除 Pool 会删除该 Pool 当前包含的账号，并通过账号外键级联移除这些账号在其他 Pool 中的成员关系。
- 删除确认文案明确提示会删除 Pool Token、Pool 内账号以及这些账号的其他 Pool 归属，避免把删除误理解为只移除分组。
- 删除含 CPA 账号的 Pool 后会请求 CPA runtime reload，避免 runtime 继续持有已删除账号的旧 auth binding。
- 删除含 CPA 账号的 Pool 时若 runtime reload 失败，会明确返回 `cpa_reload_failed`，同时 Pool 与账号删除结果已落库。
- Pool 删除遇到短暂 SQLite busy 时会进行小间隔重试，降低与异步模型发现等后台写库任务并发时误报 500 的概率。

### Codex Plus Quota

- Codex Plus 账号在 quota 辅助接口 `wham/usage` 返回 `401/403`、且没有可解析周额度快照时，不再显示“无周额度”。
- Quota 查询失败统一展示为“额度查询失败”，并保留模型请求成功或 access eligible 的独立事实。
- Plus primary-only 或无 snapshot 时展示“周额度待同步 / 周额度未知”，不把辅助接口失败误判为无周额度。
- provider 大小写和 quota window 字符串数字解析更宽容；非法数字字符串不会被误解析成可用额度。
- 模型请求返回 429 时仍作为 quota / rate-limit evidence；quota fetch 401/403 不作为硬阻断。
- 纯 diagnostic 成功请求不会清理模型请求 429 evidence，也不会写入 access eligible 证据。

### 真实探查状态回写

- `X-Lune-Account-Id` 强制账号请求不再自动归类为 diagnostic，普通客户端强制账号认证失败会写入 `needs_login`。
- 新增 `X-Lune-Probe-Mode: stateful`，供 Playground 和 Pool 自检表示“用户主动真实探查”，允许写 credential / access / quota evidence，但不写普通 serving cooldown。
- 显式 `X-Lune-Diagnostic: true` 仍是纯诊断流量，不写 credential 状态，不计普通 usage。
- `request_logs` 新增 `force_account`、`stateful_probe`、`traffic_kind`，Activity 日志保留 ordinary / stateful probe / diagnostic 审计记录；usage summary 默认排除 diagnostic 和 stateful probe。
- Activity 页趋势、日报、Top errors 和流向统计只使用 ordinary usage；日志表仍保留 diagnostic / stateful probe 审计记录。
- CPA upstream `auth_unavailable / no auth available / credential unavailable` 可识别为账号认证不可用；service key / management key 错误不会误写账号 `needs_login`。

### API 一致性

- accounts / pools / tokens 空集合统一返回 `[]`，避免管理端和自动化验收把空列表读成 `null`。

### 验证

- `go test ./internal/store ./internal/admin ./internal/gateway`
- `npm --prefix web run test:quota`
- `npm --prefix web run build`
- `go test ./...`
- `docker build -t lune:v0.1.9-current-test --build-arg LUNE_VERSION=0.1.9-current-test .`
- 使用临时容器 `lune-019-current-test`、端口 `127.0.0.1:23333`、隔离数据卷 `lune-data-019-current-test` 和 fake CPA `http://host.docker.internal:28888` 完成 Pool 生命周期、共享账号级联删除、CPA auth JSON 导入、stateful probe 状态回写、usage 排除和日志审计矩阵。
- 测试容器、测试数据卷和 fake CPA 临时进程均已清理；旧版 `lune-0.1.8`、`lune-0.1.5` 容器未被停止或修改。
- 真实容器验收、审计和最终清理证据见 `spec/v0.1.9/100-release-evidence.md`。

## [0.1.8] - 2026-05-17

状态：已发布。代码实现、自动化测试、真实容器验收矩阵、subagent 严格审计、发布文档提交、`v0.1.8` tag、GHCR / Docker Hub 多架构镜像推送和 Docker Hub 描述同步均已完成。

### Pool 路由策略

- Pool 新增 `routing_policy`，默认 `health_first`，可切换为 `ordered`。
- `health_first` 优先选择健康账号；`ordered` 按 Pool 成员顺序优先，但仍不会绕过禁用、凭据异常、Access 未确认、quota blocked、模型请求 429、runtime binding、serving cooldown/error 或模型明确不支持等硬阻断。
- Pool API 会返回策略字段，非法策略返回 `400`，旧客户端不传策略时保留原值。
- Router 的模型匹配语义收紧：模型明确不匹配不再作为 unknown fallback。

### Codex CPA 可路由分层

- 新增 `cpa_access_status / reason / last_error / checked_at`，把 Codex Access 与 paid subscription、quota、credential、runtime、serving 状态拆开。
- Codex Free / Go 账号不再因为缺少 paid subscription active 被直接判死；`wham/usage` 成功或真实模型请求成功可把 Access 提升为 `eligible`。
- Access `pending / unknown / ineligible` 为硬阻断；quota `pending / unknown / error` 为轻降级；quota `blocked` 和真实模型请求 `HTTP 429 from model request` 为硬阻断。
- 已确认 `eligible` 的账号遇到 quota fetch `401/403/request failed` 不会被降级为 `ineligible`。
- Pool `routable_account_count` 与 Router 使用等价的统一裁判口径。

### Route trace 与审计可见性

- `request_logs` 新增 `route_trace`，记录每次尝试的 `routing_policy`、`selected_account_id`、候选账号和 `skip_reason`。
- 全池不可用、retry、serving cooldown、already attempted、账号不健康、模型明确不匹配等路径都能在 trace 中复核原因。
- route trace 不记录 prompt、token、request body 或完整上游响应。

### CPA auth JSON 批量导入

- 新增 CPA auth JSON 批量预检与确认导入接口：
  - `POST /admin/api/accounts/cpa/import-json-batch/preview`
  - `POST /admin/api/accounts/cpa/import-json-batch`
- 预检严格只读：不写 auth file、不创建或更新账号、不新增 Pool member、不触发 runtime reload。
- 正式导入按文件返回 `created / updated / skipped / failed`，同批重复返回 `skipped + duplicate_in_batch`。
- 响应只返回 masked email、hash、account id suffix 等安全摘要，不泄露 refresh/access/id token 或完整 auth JSON。
- 导入主状态与 runtime sync 状态拆开，返回 `batch_id` 与 `synced/pending/failed_runtime_sync` 计数。
- DB account upsert 与 Pool membership 在同一数据库事务内完成；auth file 采用备份/恢复的可补偿事务口径。
- 新增 `cpa_import_batch` 通知事件。

### Codex quota 与刷新退避

- 新增 `cpa_quota_backoff_until` 与 `cpa_quota_backoff_count`，quota/access refresh 失败后进入退避窗口，避免持续打 CPA management / wham/usage。
- 退避窗口内跳过 quota fetch；成功刷新后清理退避。
- 前端和后端文案区分真实模型请求 429、quota fetch 401/403、management/api-call 查询失败。
- Free / primary-only quota 不再强制要求 7d secondary window；无周额度时按实际窗口展示。

### 管理界面

- AccountCard 右上角展示 Plan chip，底部 chip 收敛为单行请求量与主问题，减少卡片高度抖动。
- Pool 详情页可在自检按钮旁切换 `health_first / ordered`。
- Add Account 的 auth JSON 导入改为“预检文件 -> 确认导入”两步，不再一次点击直接写入。
- 批量导入结果页展示每个文件的最终主状态与 runtime sync 状态，不把导入成功误写成“立即可用”。

### 发布流程与文档维护

- Release workflow 升级到声明 Node 24 runtime 的 action 版本，并启用 `FORCE_JAVASCRIPT_ACTIONS_TO_NODE24=true`，消除 Node.js 20 action runtime 弃用提醒。
- Docker Hub 同步短描述收敛到 100 bytes 以内，避免发布时被 Docker Hub 截断。
- Docker 镜像默认 CPA 凭据目录变量改为 `LUNE_CPA_FILES_DIR`，避免 BuildKit 把目录变量误判为 secret；`LUNE_CPA_AUTH_DIR` 保留为兼容别名。
- README、Docker Hub 描述、Compose 示例和 `.env.example` 已同步新的 CPA 凭据目录变量口径。
- `spec/v0.1.8/100-implementation-evidence.md` 记录自动化测试、Docker build、真实容器矩阵和清理证据。

### 验证

- `go test ./...`
- `npm --prefix web run build`
- `docker build -t lune:v0.1.8-test --build-arg GOPROXY=https://goproxy.cn,direct --build-arg LUNE_VERSION=0.1.8-test ... .`
- 使用临时容器 `lune-v018-test`、端口 `127.0.0.1:23333`、隔离数据目录 `/tmp/lune-v018-test-*` 和临时网络 `lune-v018-net` 完成 `/healthz`、Pool routing_policy API、`health_first` retry/cooldown、`ordered` retry、route_trace 脱敏、CPA auth JSON batch preview/import、Access eligible 落库矩阵。
- fake upstream 容器 `lune-upstream-first` 返回 `503`，`lune-upstream-second` 返回 `200`。
- 测试容器、fake upstream、临时网络和临时数据目录均已删除；旧版 `lune-0.1.7`、`lune-0.1.5` 容器未被停止或修改。
- subagent 严格复审通过。

## [0.1.7] - 2026-05-16

状态：已发布。代码实现、测试矩阵、隔离容器 smoke、`gpt-5.5` subagent 严格审计、发布阻塞提交、`v0.1.7` tag、GHCR / Docker Hub 多架构镜像推送和 Docker Hub 描述同步均已完成。

### 账号详情与直连诊断

- 账号详情抽屉 tab 统一为 `Overview / Playground / Diagnostics`。
- 直连账号的 `Connection` 编辑区移到 `Overview`，可在详情抽屉里编辑 API 地址和 token。
- 直连账号保存时，空 `api_key` 表示保留旧 token，不会误清空已有凭据。
- 直连账号 `Diagnostics` 改为 `Connection / Credential / Route / Serving / Models` 主结构，不再混入 CPA 专属的 runtime binding、subscription、quota 作为核心诊断维度。
- 高级诊断信息保留直连排障所需字段，不展示完整 token 或请求内容。

### Pool token 管理

- Settings 页面将 `Edit name` 与 `Edit token` 拆成独立入口、独立弹窗和独立提交状态。
- Pool token 默认不回填完整明文；替换成功后刷新 masked 值和更新时间，响应不泄露完整 token。
- 后端 `PUT /admin/api/tokens/{id}` 语义收紧：缺省 `token` 保留旧值，显式 `token:""` 返回 400，非空 `token` 才替换。
- 显式空 token 请求不会发生 name 部分更新，避免前端校验被绕过时产生半成功状态。

### Codex 429 quota/cooldown 分层

- Codex CPA 普通模型请求返回 `429` 时，同时写入 serving cooldown 与 quota / rate-limit evidence。
- 明确包含 quota、rate limit、limit reached 等语义的 `429` 会写入 `cpa_quota_status=blocked`；裸 `429` 写入 `error` 和 `HTTP 429 from model request`。
- 非 Codex 账号 `429` 不写 Codex quota 字段；diagnostic 429 不污染普通 serving/quota 状态。
- `wham/usage` 成功快照不会覆盖模型请求 429 evidence，避免 quota snapshot 和真实模型请求证据互相抹平。

### 路由与恢复语义

- 普通路由会跳过仍有效的模型请求 429 evidence，避免账号在 quota/rate-limit 证据仍存在时被误判为可接流量。
- 强制账号直测可绕过普通健康拦截，用于恢复验证。
- 后续强制账号模型成功会清理 `error` 或 `blocked` 的模型请求 429 evidence。

### 文档与验收

- `spec/v0.1.7` 同步发布阻塞验证状态，并把剩余讨论改为后续非阻塞项。
- `100-release-evidence.md` 记录最终验证证据，区分 embedded CPA 容器、Codex 429 fake CPA 矩阵和 auth JSON 导入矩阵。
- 保留 0.1.6 运行态只读审计结论：不能只用 `docker inspect` 或 PID1 环境判断 embedded CPA pinning 是否生效，必须以 Lune 进程加载后的 effective runtime state 为准。

### 验证

- `go test ./...`
- `npm --prefix web run build`
- `docker build -t lune:v0.1.7-blocker-test --build-arg LUNE_VERSION=0.1.7-blocker-test .`
- 使用临时 embedded CPA 容器 `lune-v017-embedded-final`、端口 `127.0.0.1:17791`、隔离数据目录 `/tmp/lune-v017-embedded-final.u2yRnx` 完成 `/healthz`、provider pinning effective state、Pool token 替换语义和 CPA auth JSON 导入矩阵。
- 使用临时 external fake CPA 容器 `lune-v017-ct429-matrix`、端口 `127.0.0.1:17790`、隔离数据目录 `/tmp/lune-v017-ct429-matrix.gzWAsA` 完成文案型 `too many requests` / quota 文案 `429`、`wham/usage` ok snapshot 分层和 cooldown 过期后 quota evidence 阻断矩阵。
- 使用临时 external fake CPA 容器 `lune-v017-ct429-bare`、端口 `127.0.0.1:17792`、隔离数据目录 `/tmp/lune-v017-ct429-bare.CVaCdR` 完成空 body 裸 `429` 分类矩阵，验证其写入 `cpa_quota_status=error` 和 `HTTP 429 from model request` evidence。
- 测试容器和临时数据已删除。
- 最终 `gpt-5.5` subagent 严格审计通过，发布阻塞改动已提交。

## [0.1.6] - 2026-05-16

状态：已发布。核心路由、runtime binding、可信记账、内置 CPA 构建、主要运维验收和真实多账号 Codex 上游验证均已完成。

### 重点变化

- 修复 CPA 多账号双调度归因问题：Lune 选中某个 CPA 账号后，普通模型请求会先确认该账号对应的 CPA runtime auth，再把请求 pin 到该 auth。
- 内置 CPA 从固定外部镜像切换为固定上游 commit + Lune provider pinning patch + 本地构建，版本标识为 `v7.0.2-lune.1`。
- CPA runtime 不支持逐请求 pinning、binding 未就绪或强制账号无法确认 runtime credential 时，普通流量会 fail closed，避免继续用 CPA 默认 round-robin 污染账号统计。
- 收紧 CPA 账号导入与登录生命周期，避免同一 CPA 凭据被重复导入为多条账号。
- 将 CPA 凭据、Codex 额度和网关 serving 冷却拆成独立路由状态，减少“需要重新登录”的误判。
- 普通路由和 `X-Lune-Account-Id` 强制账号路由都会遵守不可接流量状态；被 quota blocked、credential 异常或 serving cooldown 的账号不会继续接普通请求。
- Pool、Overview、Activity 和前端账号卡片的“可用/可信用量”口径对齐后端路由口径，避免管理界面显示可用但网关实际无法路由，或把 CPA 默认调度的消耗记到错误账号。

### CPA runtime binding 与可信记账

- 普通 CPA 网关转发已接入 runtime binding。转发前会解析 Lune account 对应的 CPA auth id / auth index / account key。
- CPA 转发会携带 pinning 与诊断 headers：`X-Lune-CPA-Account-Key`、`X-Lune-Runtime-Auth-Id`、`X-CLIProxyAPI-Pinned-Auth-Id`、`X-Lune-Runtime-Auth-Index`、`X-CLIProxyAPI-Pinned-Auth-Index`、`X-CPA-Auth-Index`、`ChatGPT-Account-Id`。
- `request_logs` 增加 runtime identity 字段，区分 Lune routed account 与 CPA runtime credential。
- Activity / usage / Pool 账号统计只把 confirmed CPA binding 作为可信账号级消耗；binding 未确认时不会把请求量当作精确账号事实。
- 状态写入只信任 confirmed runtime binding。未确认可 pin runtime auth 的模型调用不会把某个 Lune 账号标记为 healthy。
- `X-Lune-Account-Id` 强制路由也不能绕过 binding 和不可接普通流量状态。

### 内置 CPA 版本与构建

- Dockerfile 固定 `router-for-me/CLIProxyAPI@v7.0.2`，commit `1fca942b9c2c5bbdf78334eb4744a098983a05e9`。
- Lune patch 文件位于 `third_party/cliproxyapi/v7.0.2-lune-provider-pinning.patch`，构建时应用后生成 `v7.0.2-lune.1`。
- GitHub Actions release workflow 同步传入 `CPA_VERSION`、`CPA_COMMIT`、`CPA_PATCH_VERSION` build args，避免本地构建与发布镜像版本漂移。
- `LUNE_EMBEDDED_CPA_VERSION` 更新为 `v7.0.2-lune.1`。
- embedded CPA 会自动声明 `LUNE_CPA_PROVIDER_PINNING_SUPPORTED=1`；外部 CPA 默认不声明支持，相关 CPA 账号普通流量会 fail closed。
- entrypoint 现在监督 embedded CPA 子进程。CPA 异常退出时会停止 Lune 并让容器失败，避免 CPA 已死但容器继续看似运行。

### CPA 账号幂等与 runtime reload

- 对 CPA 账号增加唯一性约束：同一 `(cpa_service_id, cpa_account_key)` 只能存在一条账号记录。
- 数据库迁移会在创建唯一索引前检测重复 CPA key，并输出可诊断错误，避免静默迁移失败。
- Device Code 登录、远程单账号导入、批量导入共用同一套 upsert 逻辑；同一 account key 重登会更新已有账号，而不是创建重复账号。
- Pool attach 改为幂等，重复导入或重登不会产生重复 Pool membership。
- 同名 auth file 被重登覆盖后会请求 embedded CPA runtime reload，并重新读取 runtime metadata。
- `resolveAuthMetadata` 会校验 runtime metadata 与磁盘 auth file 的关键身份信息；发现旧 auth index 时会触发 reload。
- `syncCpaMetadata` 和 runtime 解析路径会基于 `last_refresh` 防止旧 auth file metadata 覆盖 DB 中的新登录状态。
- 覆盖导入成功会清理旧的通用 health error，避免重登后账号仍因旧 `status=error` 被路由拒绝。

### quota、credential 与 serving 状态

- Codex quota 辅助接口 `wham/usage` 返回 `401/403` 时，不再直接把 CPA 账号标记为 `needs_login`。
- quota 查询失败会写入 `cpa_quota_status`、`cpa_quota_last_error` 和 `cpa_quota_checked_at`，前端按 quota 问题展示。
- 已有 quota 快照显示 `allowed=false` 或 `limit_reached=true` 时，会标记为 `blocked`，并在普通路由中跳过该账号。
- Codex subscription 非 `active` 时普通路由阻断；订阅资格不会被单次模型成功覆盖。
- `auth_suspect` 默认可路由但降权，优先选择 credential 确认正常的账号。
- 网关上游 5xx、EOF、timeout、网络错误等 serving 失败会进入账号级 cooldown，不再污染模型发现健康状态。
- discovery health 与 serving health 拆分：`/v1/models` 成功不会直接清除网关 serving cooldown。
- CPA gateway 鉴权失败只更新 CPA credential 状态，不再覆盖 discovery health。
- 禁用的 access token 会被网关拒绝。

### 管理界面

- 账号卡片和账号详情页增加 quota blocked / quota error 展示。
- 前端 `isAccountRoutable` 与后端路由条件对齐，用于 Pool 快照和可用账号统计。
- Overview 的账号健康、Pool 健康和 `pool_unhealthy` 告警改为使用 routable 口径。
- Pool API 和配置导入 helper 的 `routable_account_count` 使用同一套后端条件，包含 credential、quota 和 serving cooldown。
- Activity Flow 升级为 `Pool -> Account -> Model`，并按 confirmed CPA binding 解释 CPA 账号级可信统计。
- 账号详情增加 Runtime Binding 诊断字段，用于查看 runtime auth id/index、binding 状态和最近错误。

### 文档与规格

- `spec/v0.1.6` 从旧的分散主题整理为按问题闭环组织的 5 个主题文档和验收矩阵。
- 新增 `spec/draft/`，把 external CPA advanced mode、managed CPA update、大响应 streaming、轻量 VPS runtime 等未进入 v0.1.6 完成范围的内容移到后续草案。
- README 和 Docker Hub 描述同步 CPA pinning、fail-closed、内置 CPA `v7.0.2-lune.1` 语义。

### 已知后续项

- 管理端鉴权进一步收紧已移入 `spec/draft/06-admin-auth-hardening.md`，不作为 v0.1.6 当前发布闭环。

### 验证

- `go test ./...`
- 在 `web/` 下执行 `npm run build`
- `sh -n docker/entrypoint.sh`
- `git diff --check`
- `docker build -t lune:v0.1.6-release-check .`
- 使用临时容器完成 health、readyz、diagnostic request、usage exclusion、error sanitization、repeat folding、stdout suppression、data-retention(DB/WAL/SHM)、CPA 删除 auth file 清理、reload signal、embedded CPA 子进程退出、Compose health/logging/stop grace 等 fake/smoke 验证。
- 使用临时容器 `lune-v016-ct0206` + fake account + mock upstream 完成 CT-02 到 CT-06 代表性容器验收：provider pinning unsupported fail closed、状态路由跳过、diagnostic request 不污染普通 usage、stream incomplete 不惩罚账号、明确 stream failure 后续绕开 cooldown 账号。
- 使用临时容器 `lune-v016-ct07b` + fake cpa-auth 完成 CT-07 补充验收：Pool 移除不删账号/auth file，删除账号删除 auth file 并写 reload signal，历史 request log 保留删除前 account label snapshot，重新导入同 key 不与旧 auth file 冲突。
- 使用隔离数据副本和真实 CPA 账号完成 CT-01 真实多账号上游消费验收：3 个真实 Codex CPA 账号强制路由均返回 `200` 且 request log 的 account/runtime binding 对应稳定；自动路由返回 `200` 且使用 selected account 的 confirmed pinned auth；其中 1 个账号连续 10 次普通请求保持同一 pinned runtime auth。
- 使用 6000 条 fake request log seed 数据完成 CT-13b Usage 大数据量复测；`/admin/api/usage?range=all&page_size=500` 返回 `page_size=200`，SQLite `EXPLAIN QUERY PLAN` 确认 account/source/token 过滤走对应 usage 索引，model 过滤走 requested/actual model 的 multi-index plan。
- 发布前本地镜像 `lune:v0.1.6-release-check` 使用临时容器完成 `/healthz` smoke test；测试容器和 `lune-v016-*` 测试卷已清理，旧版 `lune-0.1.5` 容器未被停止或修改。
- GitHub Actions Release workflow 已在 `v0.1.6` tag 上成功完成，向 GHCR 和 Docker Hub 发布 `0.1.6`、`0.1`、`latest` 镜像标签，并同步 Docker Hub 描述。
- 多轮 `gpt-5.5` subagent 严格只读审计；审计发现的验证范围表述、spec 状态口径、测试容器清理和 patch whitespace 检查问题均已修复并复审通过。

## [0.1.5] - 2026-04-30

状态：已发布。

### 重点变化

- 修复 Docker 单容器运行中新增 Codex CPA 账号后，CPA management 长时间不返回该账号 `auth_index`，导致额度/订阅刷新一直停留在待同步状态的问题。
- 内置 CPA 增加受控重启兜底：Lune 检测到本地 auth 文件存在但 CPA runtime 未索引时，会请求 entrypoint 重启内置 CPA，并在 runtime 恢复后继续刷新。
- 清理前端旧文案，不再把新增账号的短暂同步状态描述为用户侧错误。

### CPA 账号刷新

- `RefreshAccount` 在等待 `auth_index` 超时后，会通过 `/app/data/tmp/cpa-reload.signal` 请求内置 CPA 重启，并再次等待 `/v0/management/auth-files` 出现该账号。
- 该兜底只在内置 CPA 模式默认启用；外部 CPA 部署不会被 Lune 误判为可本地重启。
- entrypoint 现在直接管理 CPA 子进程 PID，避免重启兜底时留下旧 CPA 进程。
- 新增单元测试覆盖 auth index 长期缺失后触发 reload signal、CPA 恢复健康、随后额度刷新成功的路径。

### 管理界面

- Codex 额度缺失时使用稳定的双行 pending quota UI，避免账号卡片高度异常。
- 账号详情页同步使用 pending quota 组件，不再显示可能撑开布局的裸文本。
- 手动刷新 pending toast 改为“正在同步后补齐”，减少内部 runtime 语义外泄。
- web 包版本更新为 `0.1.5`。

### 文档

- README 固定版本示例更新为 `noreply1018/lune:0.1.5`。

### 验证

- `go test ./...`
- 在 `web/` 下执行 `npm run build`
- `git diff --check`
- `sh -n docker/entrypoint.sh`
- `docker build -t lune:cpa-reload-fix .`
- 使用复制出的 `/app/data` 启动独立测试容器，确认运行中新增 auth 文件可被 CPA 识别，并确认 reload signal 会重启 CPA 后重新扫描 auth files。

## [0.1.4] - 2026-04-30

状态：已发布。

### 重点变化

- 重构 CPA 账号初始化与刷新链路，统一模型、Codex 额度、ChatGPT 订阅刷新入口。
- 修复 Docker 单容器首次添加 Codex CPA 账号后，可能立刻显示 `CPA auth file not found` 并导致自检失败的问题。
- 清理旧的 CPA auth-file / auth-index 语义，避免把 CPA runtime 尚未热加载凭据误判为用户需要重新登录。

### CPA 账号刷新

- 新增统一的 CPA runtime 解析流程：先确认 `/app/data/cpa-auth/<accountKey>.json` 本地凭据文件，再等待 CPA management `auth_index`，最后执行模型、额度、订阅刷新。
- 首次 Device Code 登录成功后会立即执行首轮强制刷新；如果 CPA runtime 还没有索引到新凭据，会短暂重试后标记为 `runtime_pending / auth_index_pending`，并由后台继续补偿。
- 手动刷新、定时健康检查、定时 Codex 额度刷新、定时订阅刷新现在共用同一套后端逻辑，不再各自重复查询 CPA auth-files。
- CPA management auth-files 匹配兼容 `id`、`name` 和 `provider-email-plan` 推导 key，降低不同 CPA runtime 返回格式差异造成的误判。
- Codex 订阅优先从本地 auth 文件 JWT 解析；CPA metadata 暂缺时只记录订阅 metadata pending，不再提示重新登录。
- 模型刷新失败不会清空既有 CPA 凭据错误状态；只有拿到 CPA `auth_index` 或明确完成凭据相关刷新后才把凭据标记为 `ok`。

### 管理界面

- CPA 凭据状态新增 `runtime_pending` 和 `runtime_error` 展示。
- `auth_index_pending` 会显示为 CPA runtime 尚未加载该凭证，不再显示为登录态失效。
- web 包版本更新为 `0.1.4`。

### 清理

- 删除旧的 `DiscoverModels`、`RefreshCodexQuota`、`RefreshCodexSubscription` 包装语义，后端收敛到 `RefreshAccount`。
- 删除不再使用的 `findAuthIndex` 和 `auth_index_missing` 语义。
- 合并健康检查中的模型发现代码，避免手动刷新和定时检查各自维护一套 `/models` 解析逻辑。

### 验证

- `go test ./...`
- 在 `web/` 下执行 `npm run build`

## [0.1.3] - 2026-04-29

状态：已发布。

### 重点变化

- Lune 默认改为单镜像交付。
- 主镜像内置 CPA runtime，Docker 首次使用不再需要单独启动或配置第二个 CPA 容器。
- Pool 成为唯一的外部网关访问边界。
- Pool 详情页新增 Codex CLI 配置流，可生成可合并到 `~/.codex/config.toml` 的配置。
- 增强 CPA / Codex 账号状态展示，包括登录态、ChatGPT 订阅过期时间和额度刷新。

### 一体化 Docker 镜像

- 内置 CPA 固定为：
  `eceasy/cli-proxy-api:v6.9.41@sha256:27a8090de418fd5ef96fae91ba6ba8579874806d573c5de3f8d13a1a4fe5ee91`。
- 运行时镜像通过同一个 entrypoint 启动 Lune 和内置 CPA。
- CPA 配置在容器启动时根据 Lune 环境变量自动生成。
- 内置 CPA 日志统一加上 `[cpa]` 前缀，便于在单容器日志里区分来源。
- 默认 Docker 持久化目录收敛为一个挂载点：`/app/data`。
- SQLite 数据库位于 `/app/data/lune.db`。
- CPA 凭据文件位于 `/app/data/cpa-auth`。
- 网关大请求重放临时文件位于 `/app/data/tmp`。
- Docker Compose 默认不再暴露 CPA 端口 `8317`；Lune 在容器内部通过 `http://127.0.0.1:8317` 访问 CPA。
- 启动时会把旧 Compose 默认地址 `http://cpa:8317` 的默认/托管 CPA 配置迁移到新的内置 runtime 地址。

### Pool 级访问凭证

- 移除全局 access token 产品模型。
- 新建 Pool 时会自动创建一条默认启用的 Pool token。
- 启动时会对已有 Pool 做 token 对账，确保每个 Pool 都有可用凭证。
- 网关请求现在必须使用绑定到 Pool 的 token。
- `X-Lune-Account-Id` 强制路由只允许指定同一 Pool 内的账号。
- 配置导入/导出通过 Pool label 保留 token 与 Pool 的关联意图。
- Settings 和 Pool 详情页都改为围绕 Pool 上下文展示访问凭证。

### Codex CLI 配置流

- Pool 页面操作从 `Codex Setup` 改名为 `Codex CLI`。
- 删除未完成的 VS Code 配置占位。
- provider id 和环境变量名改为从 Pool 名称派生，不再使用 `pool-1` 这类数字命名。
- 生成配置的默认模型固定为 `gpt-5.5`。
- 生成的 `config.toml` 参照当前 Lune 的 Codex CLI 配置结构，包含 profile、sandbox、retry、stream timeout、tools 和 provider 设置。
- 环境变量步骤同时提供临时 `export` 命令，以及写入/更新 `~/.bashrc` 的命令。
- 写入 `~/.bashrc` 前会先移除同名旧 export，避免旧 token 残留。
- 弹窗已限制宽高，长内容在弹窗内部滚动，普通浏览器窗口可以正常显示。

### CPA 与 Codex 账号状态

- 将 CPA 凭据登录态与 ChatGPT 订阅过期时间拆开展示。
- 不再把 Codex 凭据过期误显示为账号订阅过期。
- 通过 CPA auth-files metadata 获取并存储 ChatGPT / Codex 订阅过期时间。
- 移除旧 `accounts/check` 订阅探测语义，订阅状态不再影响 CPA 登录态判断。
- 订阅 metadata 缺失时只记录独立的订阅获取错误，不再提示重新登录。
- 支持从账号卡片操作刷新 Codex 额度状态。
- 改进账号卡片和账号详情中的 Codex CPA 额度、凭据状态展示。
- 增加 CPA 凭据异常通知事件。

### 网关与运行时

- 增加大请求重放缓冲能力。
- 提高默认网关请求体上限，适配图片/文件较多的请求。
- 超过内存阈值的请求会写入临时目录，用于后续重试重放，避免完整请求体长期驻留内存。
- 更稳定地记录早期网关失败，包括请求体过大、请求体格式错误等情况。
- HTTP 请求日志包含状态码。
- 增加 `LUNE_GATEWAY_TMP_DIR` 运行时默认值。

### 管理界面

- 更新 Docker Desktop 首次使用说明和相关 UI 文案，默认按一体化镜像描述。
- Settings 中 CPA 区域改为围绕内置 runtime 展示，而不是要求用户先配置外部 CPA 服务。
- Add Account 继续聚焦账号来源和 Pool 选择，CPA 作为内置账号来源使用。
- Overview 右下角新增轻量的 Gateway Base URL 展示与复制入口，默认低透明度，鼠标靠近后变清晰。
- Pool 详情页 token 处理已改进，Codex CLI 弹窗可以直接使用当前 Pool 凭证。
- Pool 详情页账号卡片点击刷新后会立即显示刷新中状态，包括按钮旋转、卡片描边、底部文案和重复点击保护。
- Add Account 成功后会先刷新新账号的模型/额度/订阅信息，再让 Pool 详情页重新拉取权威数据，避免新账号以初始化占位状态提前插入 Active Pool。
- 账号详情新增探测模型配置、Playground 和 Debug 信息。

### 修复

- 修复直接打开 `/admin/pools/:id` 时没有正确返回 SPA 的问题。
- 修复 Pool member account 扫描字段不完整，导致 CPA / Codex 新字段在 Pool 详情中丢失的问题。
- 修复旧数据库缺少 `request_logs.pool_id` 时 Pool stats 读取失败的问题。
- 修复 Docker release / build 中嵌入前端静态资源的收口问题。
- 修复 `web/package.json` 仍保留旧版本语义的问题；当前 web 包版本为 `0.1.3`。
- 修复文档和 UI 术语不一致的问题，统一使用 `Codex CLI`。
- 修复一体化镜像重建/重启后 CPA runtime 可能短暂显示为 `error` 的启动竞态：健康检查现在会对 CPA 启动期的连接失败做短重试，避免把尚未 ready 的内置 CPA 误持久化为失败状态。
- CPA 健康检查只重试连接类失败；如果 CPA 已返回明确 HTTP 错误，会立即记录真实错误，避免错误配置场景下等待过久。
- 修复在 Pool 详情页新建账号后，新账号短暂以 pending quota / usage 占位状态进入 Active Pool，导致账号卡片高度和布局与刷新后不一致的问题。

### 升级注意

- 默认路径只需要运行 Lune 容器，不需要再单独运行 CPA 容器。
- 持久化时挂载一个 volume 或宿主机目录到 `/app/data`。
- 不要暴露 `8317` 端口；它只用于容器内部 Lune 与内置 CPA 通信。
- 如果从旧的 Compose + CPA 双容器方案升级，请保留原 Lune 数据目录。Lune 会在合适时把默认 CPA 服务地址从 `http://cpa:8317` 迁移到 `http://127.0.0.1:8317`。
- 如果把 Lune 暴露到本机以外，请设置 `LUNE_ADMIN_TOKEN`，并配合可信反向代理、VPN、防火墙或等价网络边界。
- v0.1.3 不会在运行时替换内置 CPA 二进制；如果需要更新 CPA runtime，请升级 Lune 镜像。

### 验证

- `go test ./...`
- 在 `web/` 下执行 `npm run build`
- 通过 `./scripts/rebuild.sh` 完成 Docker 重建
- 手动确认运行中容器可以访问 `/admin` 和 `/admin/pools/:id`
- 重建并重启容器后，直接确认 `/admin/api/cpa/service` 返回 `healthy`，无需手动点击 CPA runtime 测试按钮
- 手动确认 Pool 详情页新建账号后，账号卡片在刷新完成后再出现在 Active Pool，卡片视觉与刷新页面后一致
