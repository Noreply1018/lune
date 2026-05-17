# v0.1.8 规格整理

状态：规划中。本目录沉淀 v0.1.8 的发布范围、CPA 分层可路由模型、路由策略设计、quota 文案分层、卡片展示稳定性、CPA auth JSON 多账号导入和验收矩阵；所有写入本目录的事项均视为 v0.1.8 必须完成并验收的范围。

本版本的基础目标是把 CPA 账号可路由规则从零散字段判断收束为分层判定模型。Codex Free / Go 账号进入 CPA 后，`subscription active` 不再能代表所有可用账号；v0.1.8 必须把计划身份、Codex access / entitlement、quota、真实 serving 状态拆开，避免把 Free 误判为订阅异常，也避免把 quota 查询失败误判为模型请求限流。

本版本还把 Pool 内账号调度从隐式“健康优先”升级为显式 Pool 级路由策略，让用户可以在“优先成功率”和“严格尊重账号排序”之间做选择。该能力不改变分层模型中的硬阻断规则，不允许绕过需要重登、access ineligible、额度明确阻断、runtime binding 不可用或服务冷却等不可接普通流量状态。

本版本还必须修正 Codex CPA quota 诊断文案的误归类：`cpa_quota_status='error'` 不能一律展示为“模型请求被限流”。只有 `cpa_quota_last_error` 明确来自普通模型请求 `HTTP 429 from model request` 时，才使用“模型请求被限流”；`HTTP 401`、`HTTP 403`、`request failed` 或其他 quota 辅助接口失败必须展示为“额度查询失败”或更具体的查询失败原因。

本版本同时修复 v0.1.7 Pool 账号卡片 chip 在三枚摘要组合下换行撑高的问题。卡片 chip 是摘要层，应优先保证网格高度稳定和快速扫描；完整解释放在 title、tooltip、详情抽屉或诊断区。

本版本还把 v0.1.7 的单文件 CPA auth JSON 导入扩展为多文件导入。该能力只覆盖用户显式选择多个 `.json` 文件，不扫描旧数据目录、不导入压缩包、不迁移数据库；导入结果必须逐项展示创建、更新、跳过、失败和 runtime sync 状态，并且只记录安全摘要。

## 文档结构

- `01-pool-routing-policy.md`：Pool 级路由策略，覆盖健康优先、排序优先、降级账号处理、重试、UI 切换和日志诊断。
- `02-codex-quota-error-labeling.md`：Codex CPA quota 辅助接口失败与模型请求 `429` 证据的文案分层，避免把 `HTTP 401` 误报为模型限流。
- `03-account-card-chip-height-stability.md`：Pool 账号卡片 chip 换行异常，覆盖 0.1.7 只读审计、单行摘要策略、短文案口径和视觉验收。
- `04-cpa-routability-layer-model.md`：CPA 账号分层可路由模型，覆盖 Credential、Runtime Binding、Access、Quota、Serving、Models，以及 Codex Free / Go 的接入和路由语义。
- `05-cpa-auth-json-multi-import.md`：CPA auth JSON 多账号导入，覆盖多文件上传、幂等更新、部分失败、结果审计和容器验收。
- `99-acceptance-matrix.md`：跨问题验收矩阵、测试要求和容器验证口径。

## 总体原则

1. CPA 可路由规则必须按 `Credential / Runtime Binding / Access / Quota / Serving / Models` 分层判断；router、Pool count、Route summary 和 Diagnostics 不得各自维护冲突规则。
2. `cpa_subscription_status` 只表达 paid subscription 元数据，不能继续作为 Codex CPA 的唯一 access gate。
3. Free / Go 是计划身份，不是异常状态；Free / Go 的 Codex access 由 `wham/usage` 成功、模型请求成功或 CPA management 明确权益证据判定。
4. Quota blocked 不等于 access ineligible；前者表示当前额度或限流阻断，后者表示账号不具备该 provider 使用资格。
5. Free / Go 首次接入后先显示 Access 待确认，并由后台异步执行 `wham/usage` 探测；不自动发真实模型请求消耗用户额度。
6. 已确认 eligible 的账号遇到短暂 access / quota 探测失败时先保留可用结论，只有明确拒绝证据才能降级为 ineligible。
7. 默认策略必须延续现有“健康优先”的调度意图；v0.1.8 同时修正模型兜底边界，明确不支持请求模型的账号不得被兜底选中。
8. Pool 中的账号拖拽顺序需要有清晰语义：健康优先下是同健康层级内的顺序；排序优先下是主要调度顺序。
9. “排序优先”只影响可接普通流量账号之间的选择顺序，不绕过硬阻断状态。
10. 降级需要分层：轻降级可以继续接流量；硬阻断必须跳过。
11. 路由结果必须可解释。用户把账号排第一却没有被选中时，UI / 日志需要能说明跳过原因。
12. 重试必须沿用当前 Pool 的路由策略，并排除已尝试失败的账号。
13. UI 控件要克制：在现有“自检 Pool”右侧增加策略切换按钮，不新增大段说明或新的复杂面板。
14. Quota 快照、quota 辅助接口失败和真实模型请求限流必须分层展示；前端不能只根据 `cpa_quota_status='error'` 推断“模型请求被限流”。
15. Pool 账号卡片 chip 是摘要层，不要求完整铺开所有文字；优先保证网格高度稳定和扫描效率。
16. 卡片右上角显示 Plan chip + 健康 chip；底部 chip 只显示 `今日 N` 和主问题，不再显示 subscription 到期。
17. Free 只有短周期额度时，quota 第二行显示 `Plan  短周期额度  无周额度`，用于保持与 Plus 卡片一致的高度。
18. 卡片 chip 行必须稳定为单行摘要；完整解释应放在 title、tooltip、详情抽屉或诊断区。
19. 解决 chip 高度异常必须约束布局根因，不能只依赖缩短某一个当前文案。
20. CPA auth JSON 多账号导入必须以 v0.1.7 单文件导入的安全边界为基础；多文件只扩大批处理能力，不扩大到目录扫描、压缩包或整库迁移。
21. 导入成功不等于账号可路由；导入后的 Credential、Runtime Binding、Access、Quota、Serving 和 Models 仍必须由分层刷新流程确认。

## 已确认口径

- v0.1.8 引入 Pool 级路由策略字段，默认值为 `health_first`。
- `health_first` 表示健康优先：优先选择无 penalty 的正常账号，降级账号作为兜底，同层级内按 Pool member 顺序；v0.1.8 明确收紧模型兜底，不能选择明确不支持请求模型的账号。
- `ordered` 表示排序优先：按 Pool member 顺序选择第一个可接普通流量且匹配模型的账号，轻降级不再自动后置。
- `ordered` 不允许绕过硬阻断：禁用、member 禁用、凭据不可用、runtime binding 不可用、access ineligible / pending / unknown、quota blocked、Codex 模型请求 429 阻断证据、serving cooldown 未过期、serving error、明确不支持模型都必须跳过。
- Codex CPA 的 `subscription active` 是 paid plan 的 access 证据之一，不是 Free / Go 的必要条件。
- Codex Free / Go 账号在 `access_status=eligible` 且 quota / serving 未阻断时，可以作为普通可路由账号。
- Free / Go 首次接入后后台异步执行 `wham/usage` 探测；不自动发真实模型请求。
- 已确认 eligible 的账号遇到短暂探测失败时不立刻降级为 ineligible。
- Free / Go 卡片在右上角显示计划 chip，例如 `Free` / `Go`，不得把 Free 显示成红色“订阅不可用”。
- 卡片底部不再显示 subscription 到期 chip；底部只保留 `今日 N` 和主问题。
- Free 只有一个 quota window 时，第二行显示 `Plan  短周期额度  无周额度`。
- 模型匹配在两种策略下都不能被破坏；明确不支持请求模型的账号不得因为排序靠前或健康层级更高而被选中。
- 如果没有任何账号明确声明支持该模型，可以继续使用模型列表为空或未知的账号作为兼容兜底。
- UI 在现有“自检 Pool”右侧增加一个切换按钮：点一下切到“健康优先”，再点一下切到“排序优先”，两种状态颜色不同。
- 颜色要求：健康优先使用清爽绿色 / 青绿色，排序优先使用月相紫 / 靛紫；颜色表达策略模式，不表达账号健康等级。
- Codex CPA `cpa_quota_status='error'` 是宽泛错误状态，必须结合 `cpa_quota_last_error` 判断来源。
- `HTTP 429 from model request` 表示普通模型请求层面的限流证据，UI 可显示“模型请求被限流”。
- `HTTP 401`、`HTTP 403`、`request failed` 或其他非 `HTTP 429 from model request` 的 quota 错误，默认显示“额度查询失败”；其中 `401/403` 可进一步说明“额度接口鉴权失败”。
- v0.1.7 中 `今日 N / N 天后到期 / 模型请求被限流` 三 chip 组合会触发卡片高度不一致；该运行态案例中的限流文案本身还受到 02 文档所述 `HTTP 401` quota fetch error 误归类影响，v0.1.8 必须同时保证修正文案后 chip 行仍稳定。
- 卡片 chip 行采用单行、不换行、溢出省略的摘要口径。
- `模型请求被限流` 可以在卡片层缩短为 `请求限流` 或 `模型限流`；详情页和诊断区保留完整解释。
- `Import auth JSON` 支持一次选择多个 `.json` 文件；同账号幂等更新，不创建重复账号或重复 Pool member。
- 批量导入允许部分成功、部分失败；结果页必须逐项展示导入主状态 `created`、`updated`、`skipped`、`failed`，并单独展示 runtime sync 状态；`pending_runtime_sync` 只能作为 runtime pending 的汇总计数或展示标签，不作为 item 主状态。
- v0.1.8 不纳入旧 Lune 数据目录扫描、目录上传或压缩包导入。

## 版本边界

v0.1.8 当前范围聚焦 CPA 分层可路由模型、Pool 内账号选择策略、Codex CPA quota 诊断文案分层、Pool 账号卡片展示稳定性，以及 CPA auth JSON 多账号导入。不新增全局多 Pool 调度，不新增权重随机，不新增按成本、额度百分比或每日配额消耗的复杂调度算法；quota 文案修正可以先复用现有字段派生 UI meta，但 Codex Free / Go 接入必须有明确 access 语义；chip 高度问题不改变 Codex `429` 的后端归类规则；JSON 导入不做目录扫描或整库迁移。

凡是实现本目录中的路由、运行态行为、版本号、启动配置或 UI 运行逻辑改动，必须在新容器中完成实际测试，并删除测试容器。仅修改本目录规格文档时不需要容器测试，但需要说明未执行原因。
