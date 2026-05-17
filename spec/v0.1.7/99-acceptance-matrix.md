# 99. 验收矩阵

## 目标

本文件只做跨问题最终核对。详细设计以各问题文档为准，避免 UI 文案、连接编辑语义、直连诊断结构和容器运行态审计口径分散后出现冲突版本。

## 本轮验证状态

- 已完成：v0.1.7 发布阻塞实现、本地测试、前端构建、测试镜像构建、embedded CPA 容器矩阵、external fake CPA Codex 429 容器矩阵和测试容器清理。可复核证据见 `100-release-evidence.md`。
- 已完成：最终 `gpt-5.5` subagent 严格审计通过，对应发布阻塞 Git 提交为 `a216d38 Complete v0.1.7 release blockers`。
- 已完成：只读审计正在运行的 `lune-0.1.6` 容器，并用临时 `noreply1018/lune:0.1.6` 容器复核 embedded CPA 启动路径。审计结论为：`docker inspect` 和 PID1 环境缺少 `LUNE_CPA_PROVIDER_PINNING_SUPPORTED` 不能证明 Lune 运行态未启用 pinning；真实 `lune up` 子进程和 embedded `CLIProxyAPI` 子进程均携带 `LUNE_CPA_PROVIDER_PINNING_SUPPORTED=1`。临时测试容器和测试 volume 已删除。
- 已完成：只读审计正在运行的 `lune-0.1.6` 容器中 Codex CPA 账号 `429` 与“服务冷却中”展示的矛盾。审计结论为：真实模型请求 `429` 只写入 `serving_status='cooldown'`，没有沉淀到 Codex quota 维度；因此用户在 Playground 看到额度/限流耗尽，而 UI 主状态显示冷却。

## v0.1.7 必须完成范围审计

以下事项已经审计为 v0.1.7 发布阻塞范围：

| 编号 | 必须完成项 | 审计结论 | 最低闭环 |
| --- | --- | --- | --- |
| MUST-01 | 账号详情抽屉 tab 与直连连接编辑 | 这是用户直接可见的 UI/配置闭环，属于 v0.1.7 体验目标 | tab 统一；直连 `Connection` 编辑区在 `Overview`；空 token 保留旧值；保存后刷新详情且不泄露 token |
| MUST-02 | 直连账号 Diagnostics 收敛 | 当前 CPA 模板会误导直连排障，必须在 v0.1.7 修正 | 主维度为 `Connection / Credential / Route / Serving / Models`；无完整 Raw；Advanced 脱敏 |
| MUST-03 | Settings Pool token 编辑 | Pool token 替换是凭据管理缺口，必须完成受控替换语义 | 独立 `Edit token`；不回填明文；空值保留；显式空字符串 400；成功刷新 masked 值和更新时间 |
| MUST-04 | 最小 provider pinning effective state 数据源 | Settings 要展示 `Provider pinning`，前端不能猜测静态容器环境 | 后端提供 effective runtime state 安全摘要；Settings 展示 enabled/disabled/unknown；来源不依赖 `docker inspect` / PID1 |
| MUST-05 | CPA 阻断原因分层 | 0.1.6 容器审计显示聚合“账号不可用”不足以排障 | UI/API 至少区分 pinning、runtime binding、model、subscription、account status、no healthy account |
| MUST-06 | Codex CPA `429` quota / cooldown 归类 | 这是已确认真实缺陷，不能只停留在 serving cooldown | 普通 Codex `429` 写入 quota/rate-limit evidence；明确 quota 文案提升为额度/限流问题；snapshot 与 real request evidence 分层 |
| MUST-07 | 可重复 Codex `429` fake/mock 验收 | 没有 deterministic 验收会让 `429` 修复不可复核 | 新容器 + mock upstream / fake CPA 覆盖 CT-429 关键矩阵；不得消耗真实 Codex 额度 |
| MUST-08 | Add Account auth JSON 单文件导入 | 已作为 v0.1.7 增补规格进入 Add Account 主流程 | 单文件导入、校验、写入、upsert、加入 Pool、刷新任务、安全拒绝和回滚语义闭环 |
| MUST-09 | 新容器验收与清理 | 本项目发布/运行态规则要求，不可只靠本地单测 | 使用新启动 v0.1.7 容器和隔离数据目录；不得影响旧容器；验收后删除测试容器和临时数据 |

## 真实容器测试口径

凡是实现本目录里的 UI 调整，都要在**新容器**中做最终验收：

- 容器必须是新启动的 v0.1.7 实例，不能复用正在运行的旧版本容器。
- 需要使用隔离数据目录，或者从现有数据目录复制出测试副本后再挂载。
- 需要保留可复核的启动命令、端口、镜像 tag 或 digest、以及验证摘要。
- 验收完成后必须删除测试容器和临时数据卷。

建议的最小容器验收覆盖：

- `UI-01` 到 `UI-06` 可同容器验证；其中直连保存语义与 Settings token 替换语义必须通过 API 或 UI 交互复核。
- `UI-07` / `CPA-JSON-01` 的 Add Account auth JSON 导入必须使用 fixture 或假凭据做容器验收，不得在测试记录中保存真实 refresh token。
- `OPS-03` 的 Codex 429 细分矩阵必须使用可重复 mock upstream / fake CPA，避免消耗真实 Codex 额度；如只用单元或集成测试覆盖，不满足 v0.1.7 发布验收。

## 需要统一验证的点

| 编号 | 覆盖问题 | 验收方式 | 必须验证 |
| --- | --- | --- | --- |
| UI-01 | 账号详情抽屉 tab 文案 | 页面人工检查或组件截图 | `Overview / Playground / Diagnostics` 全英文一致 |
| UI-02 | 直连账号连接编辑区 | 页面交互测试 | 编辑区位于 `Overview` 顶部，不出现在 `Playground` / `Diagnostics` |
| UI-03 | token 表达语义 | 交互测试 | 默认不展示完整 token；空 token 表示保留旧值，不误导为丢失 |
| UI-04 | 直连账号诊断页 | 页面人工检查或组件测试 | 对 `openai_compat` 账号不显示 `Runtime Binding`、`Subscription`、`Quota` 作为主诊断维度；主诊断区只出现 `Connection`、`Credential`、`Route`、`Serving`、`Models` |
| UI-05 | 高级信息收敛 | 页面人工检查 | `Advanced` 区域仅保留 `Account ID`、`Runtime Base URL`、`Last Error`、`Last Checked At`、`Source Kind` 等直连排障字段，不展示完整 token 或 request body |
| UI-06 | Pool token 编辑 | 页面交互测试 | Settings 页面提供显式 token 替换入口；默认不回填完整 token；空值保留旧 token；保存后 masked 值与更新时间刷新；当前 reveal 展开状态不被错误切换；`Edit token` 与 `Regenerate` 使用独立弹窗和提交状态 |
| UI-07 | Add Account 导入 auth JSON | 页面交互测试 | CPA 分支显示 `Login with Codex` 与 `Import auth JSON` 两个互斥入口；导入页只展示安全摘要，不展示完整 JSON 或 token 字段 |
| OPS-01 | 0.1.6 embedded CPA pinning 运行态审计 | 真实运行容器只读审计 + 临时发布镜像容器复核 | 区分 `docker inspect` 静态环境、PID1 shell 环境和 `lune` / embedded CPA 子进程有效环境；确认 embedded 模式下 Lune effective pinning capability；测试容器用完删除 |
| OPS-02 | CPA 路由阻断原因展示 | API / 诊断页验收 | 对固定 fixture 的 CPA 账号，接口或页面必须分别展示 `provider_pinning_unsupported`、`runtime_auth_binding_unavailable`、`no_healthy_account`、`model_not_on_account`、subscription 非 active、账号 `status=error` 六类不同阻断原因之一，且不能统一折叠成同一个“账号不可用” |
| OPS-03 | Codex CPA `429` 与 quota/cooldown 归类 | 真实运行容器只读审计 + 新容器 mock upstream / fake CPA 验收 | 真实模型请求 `429` 不能只沉淀为 generic serving cooldown；UI/API 必须展示 quota / rate-limit evidence，并与 `wham/usage` 快照分层 |
| OPS-04 | Provider pinning effective state 数据源 | API / Settings 页面验收 | 后端返回 `enabled` / `disabled` / `unknown` 的 effective runtime state 安全摘要；Settings CPA runtime 区块展示 `Provider pinning`；前端不得基于静态容器 env 自行推断 |
| CPA-JSON-01 | CPA auth JSON 上传导入 | API / 容器验收 | 合法 Codex auth JSON 写入 `cpa_auth_dir`，创建或更新账号，加入目标 Pool，并触发模型、quota、订阅刷新 |
| CPA-JSON-02 | CPA auth JSON 安全拒绝 | API / 容器验收 | `.login-sessions.json`、非 JSON、缺 `refresh_token`、缺账号身份、unsupported provider 均被拒绝；用户上传文件名永不参与目标路径生成，且不泄露 token 内容 |

## Codex 429 验收矩阵

以下矩阵用于 `05-codex-quota-429-cooldown-state.md` 的最终验收。必须使用新容器、隔离数据目录和 mock upstream / fake CPA，避免消耗真实 Codex 额度。

| 编号 | 场景 | 验收方式 | 必须验证 |
| --- | --- | --- | --- |
| CT-429-01 | Codex CPA 模型请求返回裸 `429` | 新容器 + fake Codex CPA + mock upstream | 普通请求后账号进入短期 cooldown，同时 UI/API 记录 `HTTP 429 from model request` quota / rate-limit evidence，不只显示泛化“服务冷却中” |
| CT-429-02 | Codex CPA 模型请求返回明确 quota 文案 `429` | 新容器 + mock upstream body 包含 `quota` / `rate limit` / `limit reached` | `cpa_quota_status` 反映 `blocked` 或等价现有阻断状态，卡片和详情页主问题显示额度/限流 |
| CT-429-03 | 非 Codex 账号返回 `429` | openai_compat fake upstream | 只触发 serving cooldown，不写 Codex quota 字段，不误报 Codex 额度耗尽 |
| CT-429-04 | Diagnostic / Playground 强制账号返回 `429` | `X-Lune-Account-Id` 强制路由 | request log 保留直测失败证据；直测失败不污染普通 serving/quota 状态 |
| CT-429-05 | `wham/usage` 快照 ok 但模型请求 `429` | quota mock 返回 ok，模型 mock 返回 `429` | quota snapshot 与 real request evidence 分层保存，`wham/usage` 成功快照不覆盖模型请求 429 evidence |
| CT-429-06 | 冷却过期后的状态解释 | 等待或模拟 cooldown 到期 | 若 quota evidence 仍有效，主状态不应直接退回“可接流量”；若上游恢复，需要清除或降级旧 evidence |

## CPA Auth JSON 导入容器测试矩阵

以下矩阵用于 `06-cpa-auth-json-import.md` 的最终验收。必须使用新容器、隔离数据目录和测试 fixture；不得把真实 refresh token、access token、id token 写入测试日志或文档。

| 编号 | 场景 | 验收方式 | 必须验证 |
| --- | --- | --- | --- |
| CT-JSON-01 | 合法 Codex auth JSON 首次导入 | Add Account 上传 fixture | auth file 写入 `cpa_auth_dir`；账号创建并加入目标 Pool；Pool 页面可见 |
| CT-JSON-02 | 同账号 JSON 重复导入 | 再次上传同一 account key fixture | 更新已有账号，不新增重复账号；Pool member 幂等 |
| CT-JSON-03 | 拒绝 `.login-sessions.json` | 上传 login sessions fixture | 返回明确错误；不写 auth file；不创建账号 |
| CT-JSON-04 | 拒绝缺 token JSON | 上传缺 `refresh_token` 的 fixture | 返回 400；响应和日志不包含上传内容 |
| CT-JSON-05 | 防路径穿越 | 上传 filename 为 `../../x.json` 的文件 | 后端忽略用户文件名，生成路径仍在 `cpa_auth_dir` 内 |
| CT-JSON-06 | Runtime reload / auth index 同步 | 导入后等待 CPA management auth-files | RefreshAccount 能拿到 auth index；模型、quota、订阅刷新进入可解释状态 |
| CT-JSON-07 | 覆盖失败回滚 | 模拟 upsert 或 pool add 失败 | 新文件回滚；已有文件不丢失；前端显示安全错误 |

## 最低验收标准

- tab 语言统一，详情抽屉不再混用中文和英文 tab 名。
- 直连账号可以在详情抽屉里直接改 API 地址和 token。
- token 默认不回填完整明文，但 UI 不会让用户误判为凭据丢失。
- Settings 页面可以显式替换 pool token，且不会把完整 token 暴露到界面或日志里。
- Settings 页面替换 pool token 后，masked 值和更新时间会同步刷新，且不会把当前 reveal 状态误切换掉。
- 直连账号的诊断页与 CPA 账号诊断页结构不同，且更贴近直连场景。
- 直连账号相关保存和诊断行为不会泄露完整 token。
- 容器诊断不能只依据 `docker inspect` 或 PID1 环境判断 pinning 是否启用；必须以 Lune 进程加载后的 effective 配置为准。
- 账号不可用时，UI/API 给出具体阻断层，不能把所有 503 都表述成同一个“账号不可用”。
- Codex CPA 账号真实请求返回 `429` 时，状态归类不能停留在 generic cooldown；必须把 quota snapshot 与 real request evidence 分层展示。

## 已确认口径

已确认不再反复讨论的口径：

- 直连账号不保留完整 `Raw` 区域，只保留脱敏 curated `Advanced` 字段。
- `Route` 与 `Serving` 不合并为单一维度，只允许做视觉紧凑化。
- `API Base URL` 编辑区保留推荐格式提示。
- v0.1.7 不提供清空 Pool token 动作。
- Add Account 的 auth JSON 导入首版只支持单文件导入；不支持 `.login-sessions.json`、批量导入或整库迁移。
- 直连账号不新增独立连接测试入口，账号级直测统一使用 `Playground`。
- 连接信息保存成功后只刷新账号详情数据，不自动触发完整健康刷新；保存动作不等同于连接成功。
- Codex 裸 `HTTP 429` 在 UI 上命名为“模型请求被限流”；带 `quota`、`rate limit`、`limit reached` 文案的 `429` 显示为“额度 / 限流问题”。
- Runtime pinning 诊断作为管理员排障灯，放在 Settings 的 CPA runtime 区块，优先展示一句 `Provider pinning: enabled / disabled / unknown`，避免在首版展示过多容器 env / PID / 子进程细节。

- Settings CPA runtime 区块增加 pinning 状态时，前端排布要克制：不要把状态塞成大卡片，也不要挤压现有 CPA runtime 操作；优先做成同区块内的一行状态或紧凑 badge。
