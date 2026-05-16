# v0.1.7 规格整理

状态：release-ready。本目录已经沉淀 v0.1.7 必须闭环项、测试矩阵和本地/容器验证证据；最终 `gpt-5.5` subagent 严格审计和发布阻塞提交均已完成，等待正式 tag 发布公开镜像。

本目录按“问题闭环”组织 v0.1.7 规格。每份文档围绕一个真实 UI / 运行态问题展开，统一描述问题、改进策略、UI 表现、日志与诊断、测试与验收、已决策记录或审计记录、后续非阻塞项，避免详情抽屉、诊断页和交互规则散落在不同文件后互相遗漏。

## 文档结构

- `01-account-detail-tab-and-connection-editing.md`：账号详情抽屉 tab 语言不一致，以及直连账号连接信息编辑入口的位置与交互。
- `02-direct-account-diagnostics.md`：直连账号诊断页需要从 CPA 视角收敛为更适合直连场景的诊断结构。
- `03-0.1.6-cpa-pinning-runtime-audit.md`：0.1.6 真实容器审计中，容器配置与运行时子进程环境的可见性差异，以及由此引出的 pinning 误判。
- `04-settings-pool-token-editing.md`：Settings 页面里 Pool token 的替换编辑语义、展示原则和验收要求。
- `05-codex-quota-429-cooldown-state.md`：Codex CPA 账号真实模型请求 `429` 时，额度耗尽/限流证据只沉淀为 serving cooldown，导致 UI 显示“冷却”而不是额度问题。
- `06-cpa-auth-json-import.md`：Add Account 增加 `Import auth JSON` 入口，用于受控导入已有 CPA/Codex auth JSON 凭证并创建/更新账号。
- `99-acceptance-matrix.md`：跨问题验收矩阵和最终核对清单。
- `100-release-evidence.md`：v0.1.7 发布阻塞实现、测试、真实容器矩阵和清理证据。

## 总体原则

1. 账号详情抽屉的主目标是快速定位“能不能连、为什么不能连、怎么改”，不是把所有后端字段都暴露出来。
2. 直连账号和 CPA 账号的诊断结构不能共用同一套维度模板；直连账号优先围绕 base_url、token、路由可用性和服务状态展开。
3. 凭据类字段默认采用最小暴露原则，UI 应避免让用户误以为 token 丢失，也避免在无需变更时触发凭据重写。
4. 任何保存行为都不能隐式改动凭据；空值应有明确语义，要么保留旧值，要么显式清空，不允许模糊覆盖。
5. 不提供单独的完整 token reveal 动作；详情抽屉只展示脱敏状态和新的替换输入位。
6. 容器配置和运行时有效环境必须分开判断。`docker inspect`、PID1 shell 环境和 `lune` / embedded CPA 子进程环境不是同一层级，诊断页不能把前者当成后者。
7. Codex quota 快照和真实模型请求限流必须分层展示。`wham/usage` 显示 `ok` 不代表最近模型请求没有命中过 quota / rate-limit；真实请求 `429` 需要作为独立诊断证据沉淀。
8. CPA auth JSON 导入必须是受控凭据写入流程，不允许把完整 JSON、refresh token、access token 或 id token 暴露到 UI、日志、错误响应或测试记录。

## 已确认口径

- 账号详情抽屉 tab 使用 `Overview / Playground / Diagnostics`。
- 直连账号连接编辑收敛到 `Overview`，空 token 保存保留旧凭据。
- 直连账号 `Diagnostics` 收敛为 `Connection / Credential / Route / Serving / Models`。
- 直连账号不保留完整 `Raw` 区域，只保留脱敏 curated `Advanced` 字段。
- `Route` 与 `Serving` 不合并为单一维度，只允许做视觉紧凑化。
- 直连账号不新增独立连接测试入口，账号级直测统一使用 `Playground`。
- 连接信息保存成功后只刷新账号详情数据，不自动触发完整健康刷新；保存动作不等同于连接成功。
- Settings 的 Pool token 使用独立 `Edit token` 流程，和重命名、Reveal、Regenerate 分离。
- v0.1.7 不提供清空 Pool token 动作。
- Codex 裸 `HTTP 429` 在 UI 上命名为“模型请求被限流”；带 `quota`、`rate limit`、`limit reached` 文案的 `429` 显示为“额度 / 限流问题”；Playground 直测失败只作为诊断证据，不影响普通路由状态。
- Runtime pinning 诊断作为管理员排障灯，放在 Settings 的 CPA runtime 区块，优先展示一句 `Provider pinning: enabled / disabled / unknown`；前端排布要克制，不做大卡片、不挤压现有操作。
- Settings 的 provider pinning 状态必须有后端 effective runtime state 安全摘要作为数据源，不能由前端基于静态容器配置猜测。
- Codex `429` 归类必须有可重复的 mock upstream / fake CPA 验收场景，覆盖普通请求、明确 quota 文案、非 Codex 账号、Playground/diagnostic 直测和 quota snapshot 分层。
- 0.1.6 embedded CPA pinning 运行态误判已完成只读审计，并在验收矩阵中保留结论。

## 版本边界

v0.1.7 作为 UI 与诊断体验版本，优先收敛账号详情抽屉、直连账号诊断页、Pool token 编辑、Codex 429 状态归类、最小 provider pinning 状态数据源和 Add Account auth JSON 单文件导入。后续如需更严格的管理端鉴权、专门的 Raw diagnostics 区域、连接测试入口、完整 runtime diagnostics API、自动健康刷新、更细 quota state、批量/目录导入等能力，应进入后续版本或 `spec/draft/`，不再作为 v0.1.7 阻塞项。

凡是落实本目录中的 UI 或交互改动，只要涉及运行时行为、保存语义、路由结果或凭据展示，就必须在**新容器**里做实际验收，使用隔离数据目录或数据副本，不得影响正在运行的旧版本容器。本轮已经使用 `lune:v0.1.7-blocker-test` 启动隔离测试容器完成 embedded CPA、external fake CPA 429 和 auth JSON 导入矩阵；证据见 `100-release-evidence.md`。

## 单篇文档模板

每个问题文档尽量保持同一结构：

- `问题`：现象、影响、根因和必要的产品判断。
- `改进策略`：UI 结构、数据交互、保存语义和边界条件。
- `UI 表现`：卡片、badge、详情抽屉、诊断页等用户可见结果。
- `日志与诊断`：如涉及凭据保存、错误摘要或排障字段，则说明如何展示和隐藏。
- `测试与验收`：组件测试、接口测试、交互验收或容器验收要求。
- `已决策记录 / 审计记录`：已经明确的产品口径，或真实只读审计事实。
- `后续非阻塞项`：不影响当前产品口径收敛、可进入后续版本讨论的内容。
