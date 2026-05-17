# 99. 验收矩阵

## 目标

本文件只做 v0.1.8 最终核对。Pool 路由策略详细设计以 `01-pool-routing-policy.md` 为准；Codex quota error 文案分层以 `02-codex-quota-error-labeling.md` 为准；Pool 账号卡片 chip 高度稳定以 `03-account-card-chip-height-stability.md` 为准。避免路由语义、quota 诊断文案、卡片 chip 展示、UI 切换和测试口径分散后出现冲突版本。

## 本轮验证状态

- 已完成：v0.1.8 路由策略规格沉淀。
- 已完成：只读审计正在运行的 `lune-0.1.7` 容器中 Codex CPA quota `HTTP 401` 被误展示为“模型请求被限流”的问题。审计结论为：账号 `1` 和 `4` 当前是 quota 辅助接口 `HTTP 401`，真实模型请求和自检为 `200/healthy`；根因为前端把所有 `cpa_quota_status='error'` 都映射成模型请求限流。
- 已完成：只读审计正在运行的 `lune-0.1.7` 容器中 Pool 账号卡片 chip 换行异常。审计结论为：`AccountCard` 的 chip 容器允许 `flex-wrap`，三 chip 组合 `今日 N / N 天后到期 / 模型请求被限流` 在 `15.5rem` 最小列宽下会换行撑高卡片；该运行态案例中的限流文案本身还受到 quota fetch `HTTP 401` 误归类影响，但高度异常的根因是前端 chip 行缺少单行约束。
- 未执行容器测试：本轮只新增规格文档，没有修改运行时、构建、发布、启动脚本或运行配置。
- 待实现后必须完成：单元测试、API 测试、前端测试、新容器验收和测试容器清理。

## v0.1.8 必须完成范围审计

以下事项属于 v0.1.8 本项发布阻塞范围：

| 编号 | 必须完成项 | 审计结论 | 最低闭环 |
| --- | --- | --- | --- |
| MUST-01 | Pool 级 `routing_policy` 数据字段 | 没有持久化字段就无法让用户按 Pool 控制策略 | 默认 `health_first`；支持 `ordered`；迁移旧数据；非法值拒绝 |
| MUST-02 | `health_first` 延续健康优先意图 | 默认策略不能破坏健康优先心智，但必须修正模型兜底边界 | 保持模型匹配 + penalty 分层，同层级按 position；明确不支持请求模型的账号不得被兜底选中 |
| MUST-03 | `ordered` 排序优先 | 用户明确需要优先使用排在前面的轻降级账号 | 按 position 选择第一个可接普通流量且模型匹配的账号；轻降级不自动后置 |
| MUST-04 | 硬阻断不可绕过 | 排序优先不能变成强行打不可用账号 | 禁用、凭据、quota blocked、Codex `HTTP 429 from model request` evidence、subscription、runtime binding、cooldown/error、模型明确不支持均跳过 |
| MUST-05 | retry 遵守策略 | 请求失败后的重试不能回到旧策略 | 排除已尝试账号后按当前 Pool 策略继续选择 |
| MUST-06 | Pool API 读写策略 | UI 需要可保存、可刷新、可恢复 | `GET /pools`、`GET /pools/{id}` 和更新接口包含策略字段 |
| MUST-07 | Pool 详情页策略切换按钮 | 用户指定入口位于“自检 Pool”右侧 | 单按钮切换；文案为 `健康优先` / `排序优先`；两种颜色不同 |
| MUST-08 | 路由可解释性 | 用户需要知道为什么排第一账号没有被选中 | request log、diagnostic response 或结构化调试日志至少能说明策略、首次账号、最终账号、尝试次数和主要跳过原因 |
| MUST-09 | 新容器验收与清理 | 本项目运行态规则要求 | 使用新容器和隔离数据；不得影响旧容器；验收后删除测试容器和临时数据 |
| MUST-10 | Codex quota error 文案分层 | v0.1.7 运行态审计确认 `HTTP 401` quota fetch error 被误报为模型限流 | `HTTP 429 from model request` 才显示“模型请求被限流”；`HTTP 401/403/request failed` 显示额度查询失败类文案 |
| MUST-11 | Quota 文案跨 UI 一致 | 卡片、Route 摘要和 Diagnostics 不能给同一字段不同解释 | 账号卡片、Route 摘要、Diagnostics Quota 维度共享同一派生逻辑或等价规则 |
| MUST-12 | 卡片 chip 单行摘要布局 | v0.1.7 真实容器已出现三 chip 换行导致卡片高度不一致 | chip 行不换行；溢出省略；长文案不撑高卡片 |
| MUST-13 | 卡片主问题短文案 | `模型请求被限流` 等长文案会放大换行风险 | 卡片层允许短文案；详情和诊断保留完整解释 |
| MUST-14 | 卡片高度视觉回归 | 单靠代码审查无法确认不同视口下无重叠 | 覆盖桌面、窄桌面、平板、手机截图或等价视觉检查 |

## 路由策略测试矩阵

| 编号 | 场景 | 准备 | 操作 | 期望 |
| --- | --- | --- | --- | --- |
| RP-01 | 默认策略兼容 | 旧数据库或新建 Pool | 读取 Pool | `routing_policy='health_first'` |
| RP-02 | 健康优先选择正常账号 | 账号 1 为 `auth_suspect`，账号 2 正常，均支持模型 | 普通请求 | 选择账号 2 |
| RP-03 | 排序优先选择轻降级账号 | 账号 1 为 `auth_suspect`，账号 2 正常，均支持模型，策略为 `ordered` | 普通请求 | 选择账号 1 |
| RP-04 | 排序优先不绕过 disabled account | 账号 1 disabled，账号 2 正常，策略为 `ordered` | 普通请求 | 跳过账号 1，选择账号 2 |
| RP-05 | 排序优先不绕过 member disabled | 账号 1 member disabled，账号 2 正常，策略为 `ordered` | 普通请求 | 跳过账号 1，选择账号 2 |
| RP-06 | 排序优先不绕过 needs_login | 账号 1 CPA `needs_login`，账号 2 正常，策略为 `ordered` | 普通请求 | 跳过账号 1，选择账号 2 |
| RP-07 | 排序优先不绕过 quota blocked | 账号 1 CPA `cpa_quota_status='blocked'`，账号 2 正常，策略为 `ordered` | 普通请求 | 跳过账号 1，选择账号 2 |
| RP-08 | 排序优先不绕过 subscription 非 active | 账号 1 Codex CPA subscription 为 `expired` 或 `unknown`，账号 2 正常，策略为 `ordered` | 普通请求 | 跳过账号 1，选择账号 2 |
| RP-09 | 排序优先不绕过 serving cooldown | 账号 1 `serving_status='cooldown'` 且未过期，账号 2 正常，策略为 `ordered` | 普通请求 | 跳过账号 1，选择账号 2 |
| RP-10 | 排序优先允许 cooldown 过期账号 | 账号 1 cooldown 已过期，账号 2 正常，策略为 `ordered` | 普通请求 | 选择账号 1 |
| RP-11 | 排序优先不绕过明确模型不支持 | 账号 1 模型列表为 `gpt-a`，请求 `gpt-b`；账号 2 支持 `gpt-b` | 普通请求 | 跳过账号 1，选择账号 2 |
| RP-12 | 排序优先模型未知兜底 | 没有账号明确支持 `gpt-b`；账号 1 模型列表为空，账号 2 也为空 | 普通请求 | 选择账号 1 |
| RP-13 | 健康优先模型未知兜底保持现状 | 账号 1 轻降级且模型未知，账号 2 正常且模型未知 | 普通请求 | 选择账号 2 |
| RP-14 | 健康优先不选择明确不支持模型账号 | 账号 1 正常但模型列表为 `gpt-a`，请求 `gpt-b`；账号 2 模型列表为空或支持 `gpt-b` | 普通请求，策略为 `health_first` | 跳过账号 1，选择账号 2 或模型未知兜底账号 |
| RP-15 | status error 硬阻断 | 账号 1 `status='error'`，账号 2 正常，策略为 `ordered` | 普通请求 | 跳过账号 1，选择账号 2 |
| RP-16 | refresh_failed 硬阻断 | 账号 1 CPA `refresh_failed`，账号 2 正常，策略为 `ordered` | 普通请求 | 跳过账号 1，选择账号 2 |
| RP-17 | runtime_pending 硬阻断 | 账号 1 CPA `runtime_pending`，账号 2 正常，策略为 `ordered` | 普通请求 | 跳过账号 1，选择账号 2 |
| RP-18 | 空 credential status 硬阻断 | 账号 1 CPA credential status 为空或扫描后为 `unknown`，账号 2 正常，策略为 `ordered` | 普通请求 | 跳过账号 1，选择账号 2 |
| RP-19 | Pool disabled | Pool `enabled=false` | 普通请求 | 返回 `pool_disabled`，不选择任何账号 |
| RP-20 | retry 遵守 ordered | 账号 1 轻降级且排第一，上游返回 retryable 失败；账号 2 正常 | 普通非流式请求 | 第一次尝试账号 1，重试选择账号 2 |
| RP-21 | retry 遵守 health_first | 账号 1 轻降级，账号 2 正常且失败，账号 3 正常 | 普通非流式请求 | 第一次尝试账号 2，重试选择账号 3；正常账号耗尽后才考虑账号 1 |
| RP-22 | 强制账号不被策略改写 | 请求带 `X-Lune-Account-Id` 指向账号 1，策略为任意 | 普通或诊断请求 | 只尝试账号 1；失败后不切换其他账号 |
| RP-23 | runtime binding blocker 后续可用账号 | 账号 1 CPA 可路由字段正常但 provider pinning 不支持，账号 2 直连或可绑定 CPA 正常，策略为 `ordered` | 普通请求 | 跳过账号 1，选择账号 2 |
| RP-24 | runtime binding 全池阻断 | 全池只有 provider pinning 不支持的 CPA 可路由账号，策略为 `ordered` | 普通请求 | 返回 `runtime_auth_binding_unavailable` 或等价 runtime binding 错误，不静默转发 |
| RP-25 | Codex 模型请求 429 evidence 阻断 | 账号 1 `cpa_quota_status='error'` 且 last error 以 `HTTP 429 from model request` 开头，账号 2 正常，策略为 `ordered` | 普通请求 | 跳过账号 1，选择账号 2 |

## API 测试矩阵

| 编号 | 场景 | 验收方式 | 必须验证 |
| --- | --- | --- | --- |
| API-01 | Pool 列表返回策略 | `GET /pools` | 每个 Pool 包含 `routing_policy` |
| API-02 | Pool 详情返回策略 | `GET /pools/{id}` | `pool.routing_policy` 与数据库一致 |
| API-03 | 更新为排序优先 | Pool 更新接口 | 写入 `ordered` 成功，cache invalidate，后续请求按排序优先 |
| API-04 | 更新回健康优先 | Pool 更新接口 | 写入 `health_first` 成功，后续请求恢复健康优先 |
| API-05 | 非法策略拒绝 | Pool 更新接口传 `round_robin` 或空值 | 返回 400；数据库保持原值 |
| API-06 | 旧客户端兼容 | 更新 Pool label / enabled / priority 但不传策略 | 不把策略清空；保留原值或使用默认值 |
| API-07 | 旧客户端更新后 cache 生效 | 旧客户端更新 Pool 其他字段后立即发请求 | 策略保持原值，cache 刷新后路由行为仍符合该策略 |

## Codex Quota Error 文案矩阵

| 编号 | 场景 | 准备 | 操作 | 期望 |
| --- | --- | --- | --- | --- |
| QE-01 | 普通模型请求裸 `429` | `cpa_quota_status='error'`，`cpa_quota_last_error='HTTP 429 from model request'` | 打开账号卡片与 Diagnostics | 显示“模型请求被限流” |
| QE-02 | 明确 quota 阻断 | `cpa_quota_status='blocked'`，last error 包含 `quota` / `rate limit` / `limit reached` | 打开账号卡片与 Diagnostics | 显示“额度已用尽”或等价额度/限流阻断文案 |
| QE-03 | quota fetch 401 | `cpa_quota_status='error'`，`cpa_quota_last_error='HTTP 401'` | 打开账号卡片、Route 摘要和 Diagnostics | 显示“额度接口鉴权失败”或“额度查询失败”；不得显示“模型请求被限流” |
| QE-04 | quota fetch 403 | `cpa_quota_status='error'`，`cpa_quota_last_error='HTTP 403'` | 打开账号卡片、Route 摘要和 Diagnostics | 显示“额度接口鉴权失败”或“额度查询失败”；不得显示“模型请求被限流” |
| QE-05 | quota fetch request failed | `cpa_quota_status='error'`，`cpa_quota_last_error='request failed'` | 打开账号卡片与 Diagnostics | 显示“额度查询失败”；不得显示“模型请求被限流” |
| QE-06 | quota unknown | `cpa_quota_status='unknown'`，无 quota snapshot | 打开账号卡片与 Diagnostics | 显示“额度未知”；不得显示“模型请求被限流” |
| QE-07 | 旧 snapshot + 新 fetch error | 存在 `codex_quota_json` 旧成功快照，同时 `cpa_quota_status='error'` + `HTTP 401` | 打开 Diagnostics | 展示历史 quota snapshot，同时主问题说明最新额度查询失败 |
| QE-08 | 三处 UI 一致 | 同一账号处于 `HTTP 401` quota error | 对比卡片 chip、Route 摘要、Diagnostics Quota | 三处都不出现“模型请求被限流” |

## UI 测试矩阵

| 编号 | 场景 | 验收方式 | 必须验证 |
| --- | --- | --- | --- |
| UI-01 | 策略按钮位置 | Pool 详情页面检查 | 按钮位于现有“自检 Pool”右侧 |
| UI-02 | 健康优先状态 | 页面检查或组件测试 | 文案为 `健康优先`；按钮使用绿色或青绿色视觉 |
| UI-03 | 排序优先状态 | 点击策略按钮 | 文案变为 `排序优先`；按钮使用月相紫或靛紫视觉 |
| UI-04 | 单按钮往返切换 | 连续点击按钮 | 两种策略互相切换；页面数据刷新后保持服务端状态 |
| UI-05 | 保存失败反馈 | mock API 返回错误，组件断言或截图复核 | 按钮回到原状态或保持服务端状态；显示轻量错误反馈 |
| UI-06 | 无长说明 | 页面检查 | 不新增大段策略说明，不新增独立策略面板 |
| UI-07 | 三 chip 单行展示 | AccountCard 组件或页面测试 | `今日 N / N 天后到期 / 额度查询失败` 与真实 429 场景的 `今日 N / N 天后到期 / 请求限流` 都不换行、不撑高卡片 |
| UI-08 | 长 chip 省略 | AccountCard 组件或页面测试 | `Binding 未确认`、`模型请求被限流` 等长文案在卡片内省略，不挤压按钮区 |
| UI-09 | Disabled Dock 窄栏稳定 | Pool 详情页面测试 | 右侧 disabled 卡片三 chip 时不出现异常增高 |
| UI-10 | 移动端卡片稳定 | 375px 视口截图或页面测试 | chip 不重叠、不横向溢出页面、不遮挡底部操作 |
| UI-11 | 完整解释可达 | hover / title / 详情抽屉检查 | 被省略 chip 仍可查看完整原因，且不泄露敏感字段 |

## 卡片 Chip 测试矩阵

以下矩阵用于 `03-account-card-chip-height-stability.md` 的最终验收。

| 编号 | 场景 | 准备 | 操作 | 期望 |
| --- | --- | --- | --- | --- |
| CHIP-01 | v0.1.7 缺陷组合 | `今日 5`、`30 天后到期`、`额度查询失败` | 渲染 Active Pool 卡片 | chip 行保持一行，卡片高度稳定 |
| CHIP-02 | 长请求量 | `今日 999.9k`、`7 天内到期`、`Binding 未确认` | 渲染最小列宽卡片 | 长 chip 内部省略，不换行 |
| CHIP-03 | 旧长文案兼容 | 主问题仍为 `模型请求被限流` | 渲染卡片 | 即使文案未缩短，也不撑高卡片 |
| CHIP-04 | 单 chip 对照 | 一张卡片只有 `今日 0`，另一张三 chip | 同屏渲染 | 单 chip 与三 chip 卡片高度稳定 |
| CHIP-05 | Disabled Dock | disabled 账号三 chip | 渲染右侧停泊区 | 窄栏不出现两行 chip |
| CHIP-06 | 详情可解释 | chip 被省略 | hover chip 或打开详情抽屉 | 完整原因可读，且无 token / auth JSON / prompt 泄露 |

## 视觉回归矩阵

| 编号 | 视口 | 数据组合 | 必须验证 |
| --- | --- | --- | --- |
| VR-01 | 1440px 桌面 | Active Pool 四张卡片，chip 数量为 1 / 2 / 3 / 3 | 卡片高度稳定，chip 只占一行 |
| VR-02 | 1024px 窄桌面 | 三 chip + 长账号名 | 标题和 chip 均省略合理，按钮区无重叠 |
| VR-03 | 768px 平板 | Active Pool 单列或双列 | chip 不换行撑高，卡片间距稳定 |
| VR-04 | 375px 手机 | 三 chip + 最长主问题 | 无横向溢出、无遮挡、无文字重叠 |

## 日志与诊断测试矩阵

| 编号 | 场景 | 验收方式 | 必须验证 |
| --- | --- | --- | --- |
| LOG-01 | 记录策略 | 发普通请求后查看 request log、diagnostic response 或结构化调试日志 | 能看到本次使用 `health_first` 或 `ordered` |
| LOG-02 | 记录首次与最终账号 | 多账号 Pool 发请求并触发一次 retry | 能复核首次尝试账号、最终选择账号和当前策略 |
| LOG-03 | 记录重试次数 | 触发一次 retry | `attempt_count` 能反映实际尝试次数 |
| LOG-04 | 解释跳过原因 | 排第一账号硬阻断 | 诊断或日志能说明主要跳过原因 |
| LOG-05 | 不泄露敏感信息 | 触发凭据、quota、runtime 错误，本地测试和容器日志均复核 | 日志不包含完整 token、auth file、request body 或 prompt |

## 真实容器测试口径

实现本目录中的功能后，必须使用新容器完成最终验收：

- 容器必须是新启动的 v0.1.8 测试实例，不能复用正在运行的旧版本容器。
- 使用隔离数据目录，或从现有数据目录复制出测试副本后挂载。
- 需要保留可复核的启动命令、端口、镜像 tag 或 digest、验证摘要。
- 验收完成后删除测试容器和临时数据目录或 volume。

最小容器验收覆盖：

| 编号 | 场景 | 必须验证 |
| --- | --- | --- |
| CT-RP-01 | 旧数据库迁移 | 旧 Pool 自动获得 `health_first`，服务正常启动 |
| CT-RP-02 | UI 切换 ordered | Pool 详情按钮切换成功，刷新后仍为 `排序优先` |
| CT-RP-03 | ordered 真实路由 | 轻降级账号排第一时，普通请求命中该账号 |
| CT-RP-04 | health_first 真实路由 | 同一账号组切回健康优先后，普通请求命中正常账号 |
| CT-RP-05 | 硬阻断跳过 | 硬阻断账号排第一时，两种策略都跳过 |
| CT-RP-06 | 测试清理 | 测试容器和临时数据已删除 |

Codex quota error 文案最小容器验收覆盖：

| 编号 | 场景 | 必须验证 |
| --- | --- | --- |
| CT-QE-01 | fake CPA management quota 返回 `HTTP 401`，模型请求返回 `200` | UI 显示额度查询失败类文案，不显示“模型请求被限流” |
| CT-QE-02 | fake CPA management quota 返回 `request failed` 或 `502` | UI 显示“额度查询失败”，不误判为模型限流 |
| CT-QE-03 | fake upstream 普通模型请求返回裸 `429` | 继续显示“模型请求被限流”，不回退 v0.1.7 修复 |
| CT-QE-04 | fake upstream 普通模型请求返回明确 `quota` / `rate limit` / `limit reached` 文案的 `429` | 显示“额度已用尽”或等价额度/限流阻断文案 |
| CT-QE-05 | 新账号无 quota snapshot 且无 last error | 显示“额度未知”，不显示“模型请求被限流” |
| CT-QE-06 | 测试清理 | 测试容器和临时数据已删除 |

卡片 chip 高度稳定最小容器验收覆盖：

| 编号 | 场景 | 必须验证 |
| --- | --- | --- |
| CT-CHIP-01 | 0.1.7 缺陷数据复现 | `今日 N / N 天后到期 / 额度查询失败` 在新版本中不再换行撑高 |
| CT-CHIP-02 | 多账号同屏对比 | 一枚、两枚、三枚 chip 卡片同屏高度稳定 |
| CT-CHIP-03 | Disabled Dock 对比 | 右侧 disabled 三 chip 卡片不出现高度跳变 |
| CT-CHIP-04 | 移动视口检查 | 375px 页面截图无重叠、无横向溢出 |
| CT-CHIP-05 | 测试清理 | chip 验收测试容器和临时数据已删除 |

## 最低验收标准

- Pool 有明确 `health_first` / `ordered` 两种策略。
- 默认策略延续健康优先调度意图，并修正明确不支持模型账号的兜底选择问题。
- 排序优先下，轻降级账号可以按用户排序先接流量。
- 排序优先下，硬阻断账号仍然不可接普通流量。
- Codex `HTTP 429 from model request` evidence 属于硬阻断或等价阻断证据，排序优先不能绕过。
- 模型明确不支持时不会因为排序靠前或健康层级更高而被选中。
- retry 按当前策略继续选择下一个账号。
- Pool 详情页可以在“自检 Pool”右侧通过单按钮切换策略。
- 策略切换有明确颜色区分，不新增复杂说明。
- 日志或诊断能解释主要路由结果。
- Codex quota `HTTP 401/403/request failed` 不再被显示成“模型请求被限流”。
- Codex 普通模型请求 `HTTP 429 from model request` 仍显示“模型请求被限流”。
- 卡片、Route 摘要和 Diagnostics 对 quota error 的文案一致。
- Pool 账号卡片 chip 区域不能因为三枚 chip 自动换到第二行。
- Active Pool 和 Disabled Dock 常见卡片组合高度稳定。
- 长 chip 可以在卡片摘要层省略，但完整解释仍可通过 title、tooltip 或详情抽屉查看。
- 卡片修复不能引入标题、配额条、信号条、底部时间或按钮重叠。
- 新容器验收完成并清理测试容器。

## 已确认口径

- `health_first` 是默认策略。
- `ordered` 只改变轻降级账号与正常账号之间的优先级，不改变硬阻断判断。
- Pool member 排序继续是唯一人工顺序来源。
- 不新增全局调度策略。
- 不新增随机、轮询、成本优先或额度消耗优先策略。
- v0.1.8 不纳入 Activity 图表完整 retry path 展示；retry 过程必须能通过日志或诊断载体复核。
- `cpa_quota_status='error'` 不是“模型请求被限流”的充分条件，必须结合 `cpa_quota_last_error`。
- v0.1.8 不强制新增 quota error 数据库枚举；可先用安全摘要派生 UI meta。
- 卡片 chip 是摘要层，不要求完整展示所有文字。
- 修复 chip 高度异常必须约束布局根因，不能只依赖缩短某一个当前文案。
- 对真实 `HTTP 429 from model request`，`模型请求被限流` 可以在卡片层缩短为 `请求限流` 或 `模型限流`，详情和诊断仍保留完整解释。
- 对 quota fetch `HTTP 401/403/request failed`，卡片、详情和诊断都不能继续使用“模型请求被限流”。
