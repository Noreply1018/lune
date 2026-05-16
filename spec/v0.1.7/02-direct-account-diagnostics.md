# 02. 直连账号诊断页收敛

状态：release-blocker verification complete, final audit pending。实现和验收已经纳入 v0.1.7 发布阻塞矩阵；最终 `gpt-5.5` subagent 严格审计通过前仍不能视为正式发布完成。

来源：来自账号详情抽屉的诊断页问题。当前诊断页以 CPA 账号的五维模板为核心，直连账号虽然能勉强套用，但会出现 `Quota`、`Subscription`、`Runtime Binding` 等不适配字段，信息噪音大于帮助。

## 问题

- 直连账号没有 runtime auth file、CPA subscription、quota 采样这类概念，却仍被塞进同一套诊断维度。
- 诊断页里出现不适合直连账号的字段，会让用户误以为这些字段对直连账号有操作价值。
- 直连账号的真实排障重点是连接地址、凭据状态、路由结果、最近失败和服务能力，而不是 CPA 运行时绑定。

## 改进策略

- 直连账号的诊断页改成更贴合直连场景的结构。
- 诊断页核心维度调整为：
  - `Connection`：base_url、当前连接状态、最近检查时间。
  - `Credential`：API key 是否存在、最近凭据检查结果、最近错误。
  - `Route`：当前路由状态、路由原因、对正常请求的影响。
  - `Serving`：最近成功/失败、冷却状态、失败次数、最后错误。
  - `Models`：已发现模型及最近同步信息。
- 直连账号不展示 CPA 专属维度：
  - 不显示 `Runtime Binding`
  - 不显示 `Subscription`
  - 不显示 `Quota` 作为独立诊断维度
- `Advanced` 信息也要收敛，只保留真正有助于排障的字段。
- 不为直连账号保留完整 `Raw` 区域；高级用户排障依赖折叠的 `Advanced` 字段，字段必须经过筛选和脱敏。
- `Route` 与 `Serving` 继续作为两个诊断维度保留，不合并为单一状态；UI 可以把它们相邻展示或视觉压缩，但语义边界不能丢失。

## UI 表现

- 诊断页标题仍保留 `Diagnostics`，但内部结构针对直连账号重组。
- 直连账号的诊断卡片更少、更直接，不再出现多余的红色状态解释。
- 高级信息区域应优先展示 `Account ID`、`Runtime Base URL`、`Last Error` 等字段。
- 对直连账号来说，`Quota` 不再作为主诊断条目出现；如果保留历史字段，也应降级到非核心信息。
- 直连账号不展示“原始字段全集”式 Raw 面板；`Advanced` 是 curated debug view，只展示安全且能解释排障路径的字段。
- `Route` 用来回答“普通流量会不会选中这个账号”，`Serving` 用来回答“最近真实模型服务是否成功”；两者可以紧凑排列，但不在文案和状态模型上合并。

## 日志与诊断

- 直连账号的诊断页不应暗示存在 CPA runtime binding 或 subscription 失效这类语义。
- 需要保留的错误摘要仍应安全截断，不暴露完整 token。
- 若诊断页引用 runtime base URL，应明确它是连接视角，不是 CPA 绑定视角。
- `Advanced` 中禁止加入完整 token、完整请求体、完整 response body 或未经筛选的后端对象 dump。

## 测试与验收

- 直连账号诊断页不再出现 CPA 专属维度。
- 直连账号诊断页的主结构能覆盖连接、凭据、路由和服务能力。
- `Advanced` 区域不再强调无意义的 quota / subscription 字段。
- 诊断页能帮助用户判断“地址错了、token 失效了、路由为什么不通、上一次失败是什么”。
- 直连账号没有完整 `Raw` 区域；`Advanced` 只保留脱敏排障字段。
- `Route` 与 `Serving` 分别存在，并且文案能区分路由选择与真实服务失败。

## 真实容器测试要求

- 使用**新启动的 v0.1.7 容器**做验收，不能在旧版本运行容器上直接试改。
- 使用隔离数据目录或复制出来的测试数据，保证诊断页的变更不会污染生产或旧版本数据。
- 在容器里打开一个直连账号详情抽屉，切到 `Diagnostics`，确认诊断结构已经从 CPA 五维模板收敛为直连场景。
- 至少验证一组正常直连账号和一组异常直连账号，分别确认：
  - 正常账号突出连接、凭据、路由和服务能力。
  - 异常账号能把失败原因落在对应维度上，而不是回退成 CPA 专属状态。
- 验证直连账号诊断页不再把 `Quota`、`Subscription`、`Runtime Binding` 作为核心信息展示。
- 验证高级信息里没有把完整 token 明文暴露出来。
- 验证完成后删除测试容器、临时卷和测试数据副本。

## 已决策记录

- 已确认直连账号诊断页需要从 CPA 视角收敛。
- 已确认 `Quota` 不应继续作为直连账号的核心诊断项。
- 已决策：直连账号 `Diagnostics` 主结构按 `Connection / Credential / Route / Serving / Models` 收敛。
- 已决策：直连账号诊断主结构不展示 `Runtime Binding`、`Subscription`、`Quota` 作为 CPA 专属维度。
- 已决策：直连账号 `Advanced` 区域收敛到 `Account ID`、`Source Kind`、`Runtime Base URL`、`API Key` 脱敏状态、`Discovery Health`、`Route Badge`、`Serving Status`、`Failure Count`、`Cooldown Until`、`Last Checked`、`Last Error` 等排障字段。
- 已决策：诊断页不展示完整 token 或完整请求内容。
- 已决策：直连账号不保留完整 `Raw` 区域；只保留脱敏、筛选后的 `Advanced` 排障字段。
- 已决策：`Route` 与 `Serving` 不合并为单一维度；后续只能做视觉紧凑化，不能改变两者语义。

## 后续非阻塞项

- v0.1.7 当前不新增独立于 `Playground` 的连接测试入口；账号级直测仍使用 `Playground`。
- `Connection` 区块补充轻量 `GET /models` 连接测试已移入 `spec/draft/10-v0.1.7-deferred-followups.md`，不进入 v0.1.7 阻塞范围。
