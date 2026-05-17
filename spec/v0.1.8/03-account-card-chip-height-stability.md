# 03. Pool 账号卡片 Chip 换行与高度稳定

状态：规划中。本文定义 v0.1.8 必须修复的 Pool 账号卡片 chip 换行异常、UI 收敛策略和验收矩阵。

来源：2026-05-17 对正在运行的 `lune-0.1.7` 容器进行只读审计。用户反馈：当前 0.1.7 容器里的前端展示卡片 chip 出现换行异常，导致同一 Pool 内卡片高度不一致。

## 问题

Pool 详情页账号卡片承担高密度摘要职责。v0.1.7 的卡片最多展示三类 chip：

```text
今日 N
订阅 / 到期状态
主问题
```

这套信息密度本身符合 v0.1.6 / v0.1.7 的规格口径，但当前前端布局允许 chip 区域自然换行：

- `AccountCard` 中 chip 容器使用 `flex flex-wrap`。
- chip 自身只有 `max-w-full truncate`，只能限制单个 chip 不超过整行宽度，不能限制多个 chip 的合计宽度。
- Active Pool 卡片只设置 `min-h-[9.4rem]`，不是固定高度。
- Active Pool 网格列宽为 `repeat(auto-fill, minmax(15.5rem, 1fr))`，窄列下三个 chip 很容易超过一行。

因此当某张卡片拥有三枚较长 chip，而另一张卡片只有一到两枚 chip 时，前者会折成两行并撑高卡片，造成同一网格中的卡片高度不一致。

这不是后端状态错误，也不是单个文案偶发问题，而是卡片 chip 区域缺少“单行摘要”的布局约束。

## 审计过程

本次审计遵守只读原则，没有修改运行中的容器、数据库、前端产物、配置或源码。

### 1. 运行中容器

审计时正在运行的相关容器：

```text
lune-0.1.7  noreply1018/lune:0.1.7  0.0.0.0:22222->7788/tcp
lune-0.1.5  noreply1018/lune:0.1.5  0.0.0.0:11111->7788/tcp
```

本次只读取 `lune-0.1.7` 的 Admin API 与静态前端资源，未停止、重启或写入任何容器。

### 2. 运行数据证据

通过 `GET http://127.0.0.1:22222/admin/api/pools/1` 读取 0.1.7 真实运行数据，至少三张启用卡片具备以下 chip 组合：

```text
member 1: 今日 5 | 30 天后到期 | 模型请求被限流
member 4: 今日 0 | 30 天后到期 | 模型请求被限流
member 5: 今日 0 | 30 天后到期 | 模型请求被限流
```

其中：

- `今日 N` 来自 24h account stats。
- `30 天后到期` 来自 Codex CPA subscription expiry。
- `模型请求被限流` 来自 v0.1.7 前端把 `cpa_quota_status='error'` 一律映射为限流文案；本轮 02 文档已经确认该运行态案例实际是 quota fetch `HTTP 401`，不是普通模型请求 `429` evidence。

这组内容在 `15.5rem` 最小卡片列宽、左侧拖拽留白、卡片内边距和 `gap-1.5` 共同作用下，存在稳定换行条件。

### 3. 源码证据

`AccountCard` 构造 chip 列表：

```text
requestChip -> subscriptionChip -> mainIssueChip
```

渲染区域当前为：

```text
flex flex-wrap items-center gap-1.5 text-[11px] text-moon-500
```

卡片外层为：

```text
min-h-[9.4rem]
```

Active Pool 网格为：

```text
repeat(auto-fill, minmax(15.5rem, 1fr))
```

构建后的 0.1.7 静态资源中也能看到同样的 `flex flex-wrap`、`max-w-full truncate`、`minmax(15.5rem, 1fr)` 和 `模型请求被限流` 文案，说明运行容器与当前源码口径一致。

## 审计结论

本问题的真实结论是：

> v0.1.7 的 Pool 账号卡片 chip 区域允许多行换行；当卡片同时展示请求量、订阅到期和较长主问题 chip 时，chip 会折到第二行并撑高卡片，导致同一网格中的卡片高度不一致。

具体判断：

- 异常是前端布局问题，不是 API 数据异常。
- 对真实 Codex 普通模型请求 `HTTP 429 from model request`，`模型请求被限流` 是 v0.1.7 新增的有效状态文案；但在本次 `HTTP 401` quota fetch error 运行态案例中，它是前端误归类后的展示文案。
- 只缩短单个文案可以缓解当前案例，但不能彻底防止未来其他三 chip 组合再次换行。
- 卡片 chip 是摘要层，不应为了完整展示所有 chip 文案牺牲卡片网格稳定性。

## 改进策略

v0.1.8 应把账号卡片 chip 区域收敛为稳定单行摘要，同时保留完整解释入口。Codex Free / Go 支持进入后，计划身份不再占用底部 chip，而是在右上角健康 chip 左侧单独展示。

采用组合方案：

1. chip 容器改为单行布局，禁止自然换行。
2. chip 行加 `overflow-hidden`，保证不会撑高卡片。
3. 每个 chip 使用 `min-w-0 truncate` 或等价约束，允许单个 chip 内部省略。
4. 保留 `title={chip.detail}`，让完整原因可通过 hover 查看。
5. 移除卡片底部 subscription 到期 chip；底部只保留 `今日 N` 和主问题 chip。
6. 对真实 `HTTP 429 from model request` 的高频较长文案做卡片级短文案收敛，例如把 `模型请求被限流` 缩短为 `请求限流` 或 `模型限流`；quota fetch `HTTP 401/403/request failed` 仍必须使用额度查询失败类文案。
7. 详情抽屉和诊断区保留完整表述，不把卡片短文案作为唯一信息源。

最低实现口径：

```text
右上角：Plan chip + 健康 chip
底部 chip 行：今日 N + 主问题；单行、不换行、溢出隐藏
chip 项：可收缩、内部省略、保留 title
文案：卡片显示短文案；详情页显示完整解释
```

不得只采用固定卡片高度作为唯一修复。固定高度可以掩盖网格错位，但内容仍可能挤压底部时间、按钮或配额条；窄屏下还可能引入裁切或重叠。

## UI 表现

修复后，Pool 详情页账号卡片需要满足：

- 同一 Active Pool 网格中，常见一到三枚 chip 组合不再导致卡片高度不一致。
- chip 区域只占一行。
- 右上角显示 plan chip 和健康 chip；底部 chip 只显示请求量与主问题。
- 底部两枚 chip 同时存在时，优先完整展示短 chip，较长 chip 可省略。
- 被省略的 chip 仍能通过 tooltip / title 或详情抽屉看到完整原因。
- Disabled Dock 卡片使用同一 chip 行约束，避免窄侧栏里出现高度跳变。
- 卡片底部操作按钮、更新时间和信号条不被 chip 挤压、覆盖或推离预期位置。

Codex plan chip：

| 计划 | chip 文案 | 颜色语义 |
| --- | --- | --- |
| Free | `Free` | 中性或青绿色，不使用红色 |
| Go | `Go` | 中性或青绿色 |
| Plus | `Plus` | 蓝紫 / 月相紫 |
| Pro | `Pro` | 深紫或强调色 |
| Unknown | `Unknown` | 灰色 |

Codex quota 区域必须保持固定两行高度。Plus / Pro 使用实际窗口：

```text
5h    [bar]    81%
7d    [bar]    64%
```

Free 只有短周期窗口时使用说明行补齐高度：

```text
5h    [bar]    81%
Plan  短周期额度  无周额度
```

Free quota 尚未查到时，第一行为 pending / unknown，第二行仍为：

```text
Plan  短周期额度  无周额度
```

如果未来 Free 返回第二个真实窗口，则展示真实窗口，不强行显示“无周额度”。

卡片文案口径：

| 状态来源 | 详情或诊断完整表述 | 卡片 chip 短文案 |
| --- | --- | --- |
| Codex 模型请求裸 `429` evidence | 模型请求被限流 | 请求限流 |
| Codex 明确 quota / rate-limit 文案 | 额度 / 限流问题 | 额度受限 |
| Codex quota fetch `HTTP 401/403/request failed` | 额度查询失败 / 额度接口鉴权失败 | 额度查询失败 |
| Serving cooldown 且无 quota evidence | 服务冷却中 | 冷却中 |
| Credential needs login | 需要重新登录 | 需要重登 |
| Runtime binding issue | Binding 未确认 | Binding |
| Access pending / unknown | Access 待确认 | Access 待确认 |
| Access ineligible | Access 不可用 | Access 不可用 |

卡片短文案只用于摘要层。详情抽屉、诊断区、错误详情和日志摘要仍应使用完整中文说明。

## 日志与诊断

本问题本身不需要新增后端日志字段。修复重点是前端摘要层布局和文案收敛。

但验收时需要确认：

- chip 的 `title` 或等价 tooltip 不泄露 token、auth JSON、完整请求体或 prompt。
- `模型请求被限流` / `请求限流` 只能用于真实 `HTTP 429 from model request`，完整原因仍来自既有 `cpa_quota_last_error` 或详情抽屉诊断字段。
- 只读审计证据中涉及的真实账号邮箱、token mask 和请求统计不应写入自动化测试快照。

## 测试与验收

### 前端组件 / 页面测试

| 编号 | 场景 | 准备 | 操作 | 期望 |
| --- | --- | --- | --- | --- |
| CHIP-UI-01 | 两枚底部 chip：真实 429 | 右上 `Plus` + 健康，底部 `今日 0`、`请求限流` | 渲染 Active Pool 卡片 | chip 行保持一行，卡片高度不超过同类卡片 |
| CHIP-UI-01A | 两枚底部 chip：quota fetch 失败 | 右上 `Plus` + 健康，底部 `今日 0`、`额度查询失败` | 渲染 Active Pool 卡片 | chip 行保持一行，卡片高度不超过同类卡片 |
| CHIP-UI-02 | 长 chip | 右上 plan + 健康，底部 `今日 999.9k`、`Binding 未确认` | 渲染最小列宽卡片 | chip 行不换行；长 chip 内部省略；底部按钮不被挤压 |
| CHIP-UI-03 | 旧长文案兼容 | 主问题仍为 `模型请求被限流` | 渲染卡片 | 即使长文案未缩短，布局也不换行撑高 |
| CHIP-UI-04 | 单枚底部 chip | 只有 `今日 0` | 与两枚 chip 卡片同屏 | 卡片高度保持稳定，不因 chip 数量少产生异常塌陷 |
| CHIP-UI-05 | Disabled Dock 窄栏 | disabled 账号带 plan chip、健康 chip 和两枚底部 chip | 渲染右侧停泊区 | chip 行不换行；卡片不出现异常增高 |
| CHIP-UI-06 | 移动端窄屏 | 视口宽度 375px | 打开 Pool 详情页 | plan chip、健康 chip 和底部 chip 不换行、不重叠；文字省略合理 |
| CHIP-UI-07 | hover / title | chip 被省略 | hover chip 或检查 DOM title | 可看到完整 detail；不泄露敏感字段 |
| CHIP-UI-08 | Free 单窗口 quota | Free 账号只有 5h quota window | 渲染 Codex 卡片 | 第二行显示 `Plan  短周期额度  无周额度`，卡片高度与 Plus 一致 |
| CHIP-UI-09 | Free quota pending | Free 账号 quota 尚未同步 | 渲染 Codex 卡片 | 第一行 pending，第二行为 `Plan  短周期额度  无周额度` |

### 视觉回归矩阵

| 编号 | 视口 | 数据组合 | 必须验证 |
| --- | --- | --- | --- |
| VR-CHIP-01 | 1440px 桌面 | Active Pool 四张卡片，底部 chip 数量分别为 1 / 2 / 2 / 2，右上均有 plan + 健康 | 网格卡片高度一致或符合设计固定高度；chip 只占一行 |
| VR-CHIP-02 | 1024px 窄桌面 | 两枚底部 chip + 长账号名 | 标题 truncate、chip truncate、按钮区无重叠 |
| VR-CHIP-03 | 768px 平板 | Active Pool 单列或双列布局 | chip 不换行撑高；卡片间距稳定 |
| VR-CHIP-04 | 375px 手机 | plan chip + 健康 chip + 两枚底部 chip + 最长主问题 | 无文本溢出页面边界；无按钮遮挡 |

### 容器验收矩阵

实现本项后必须使用新容器做最终验收，不得复用正在运行的旧版本容器。

| 编号 | 场景 | 验收方式 | 必须验证 |
| --- | --- | --- | --- |
| CT-CHIP-01 | 0.1.7 缺陷数据迁移 | 使用隔离数据目录导入或构造旧三 chip Codex CPA 账号 | 新版本显示为 plan chip + 健康 chip + `今日 N / 额度查询失败`，不再换行撑高 |
| CT-CHIP-02 | 多账号同屏对比 | 构造一枚、两枚底部 chip 的 Active Pool 卡片，以及 Free / Plus quota 卡片 | 同屏卡片高度稳定，网格不出现某一张异常增高 |
| CT-CHIP-03 | Disabled Dock 对比 | 构造右侧 disabled 账号，包含 plan chip、健康 chip 和两枚底部 chip | 右侧窄栏不出现两行 chip 导致高度跳变 |
| CT-CHIP-04 | 移动视口检查 | 使用浏览器或 Playwright 截图检查 375px 页面 | chip 省略合理，无重叠、无横向溢出 |
| CT-CHIP-05 | 测试清理 | 验收完成后检查 Docker 状态和临时数据 | 测试容器、临时数据目录或 volume 已删除 |

## 最低验收标准

- Pool 账号卡片 chip 区域不能因为三枚 chip 自动换到第二行。
- 卡片右上角显示 plan chip + 健康 chip；底部只显示请求量与主问题。
- Free 只有短周期额度时，第二行显示 `Plan  短周期额度  无周额度`，并与 Plus 卡片保持高度一致。
- Active Pool 常见卡片组合高度稳定。
- Disabled Dock 窄栏卡片不因 chip 换行异常增高。
- 长文案在卡片摘要层可以省略，但完整解释仍可在 title、tooltip 或详情抽屉中查看。
- 修复不能引入按钮、标题、配额条、信号条或底部时间的重叠。
- 前端构建通过。
- 实现后必须用新容器和隔离数据完成真实页面验收，并删除测试容器。

## 已确认口径

- 卡片 chip 是摘要层，不要求完整展示所有文字。
- 计划身份不再作为底部 chip；Codex CPA 卡片右上角显示 plan chip。
- 底部 subscription 到期 chip 从卡片移除；到期信息保留到详情页或 Diagnostics。
- Free 账号无周额度不是异常；卡片 quota 第二行显示 `Plan  短周期额度  无周额度`。
- 解决根因必须约束 chip 行布局，不能只依赖缩短某一个当前文案。
- 对真实 `HTTP 429 from model request`，`模型请求被限流` 可以在卡片层缩短为 `请求限流` 或 `模型限流`，详情和诊断仍保留完整解释。
- 对 quota fetch `HTTP 401/403/request failed`，卡片、详情和诊断都不能继续使用“模型请求被限流”。
- 本问题不要求新增后端状态字段。
- 本问题不改变 Codex `429` 的后端归类规则，只修复其前端卡片摘要展示稳定性。
