# 02. 账号状态互相污染导致错误路由和错误 UI

## 问题

v0.1.5 把多种来源的异常写进相同或相近字段：

- quota `wham/usage` 返回 `401/403`，会被当成 CPA 登录态失效。
- 后台 quota 刷新失败可能把账号写成 `needs_login`，而网关模型请求成功又把状态写回 `ok`，导致 UI 抖动。
- `/v1/models` discovery 成功被误用为账号整体 healthy，但它不能证明生成请求、quota、subscription 都可用。
- `status=healthy`、`cpa_credential_status=needs_login`、过期订阅、额度耗尽可以同时存在，路由却仍可能选择该账号。
- 宽泛的 `401/403/unauthorized/access denied/expired token` 文本匹配，会把额度、订阅、权限、上游临时拒绝误判为“需要重新登录”。

UI 问题也来自同一根因：

- 前端只能展示后端过载状态，因此 quota 401/403 往往显示为红色“需要重新登录”。
- discovery healthy、serving cooldown、quota blocked、subscription expired 之间没有清晰展示优先级。
- Pool 卡片 chip 文案过长时会造成高度不一致。

## 改进策略

把账号“能否接普通流量”的判断从单一健康字段，改成多维状态组合：

- `credential_status`：凭据、登录态、CPA runtime 凭据可用性。
- `quota_status`：额度是否可用，以及 quota 接口是否能成功刷新。
- `subscription_status`：订阅、套餐、Free/过期状态。
- `serving_status`：真实模型生成请求的短期服务能力与熔断状态。

这四类状态必须独立写入、独立展示，再由路由层组合成 routability。任何辅助探测接口都不能越权覆盖其他状态。

可信度顺序：

1. 已确认 runtime credential 归属的模型生成请求。
2. quota/subscription 等辅助探测。
3. discovery `/models`。

如果 runtime binding 未确认，模型调用结果只能写 request log，不能把某个 Lune CPA account 标记为 `serving_status=healthy` 或 `credential_status=ok`。

### 模型调用写入规则

生成请求 `200` 的前提是已确认 runtime credential 归属：

- 写入 `serving_status=healthy`。
- 清空 serving 维度的短期失败计数：`failure_count=0`。
- 将 `credential_status=auth_suspect` 或类似辅助探测可疑状态降回 `ok`。
- 不得清除或改写任何 quota 维度状态。
- 不得清除或改写任何 subscription 维度状态。

生成请求 `401/403` 不能只看 HTTP code，必须先判断错误归属：

- Lune 到 CPA 的 service API key 错误：写 `cpa_service.status=error` 或 runtime/service error，不写 account `needs_login`。
- CPA 到 ChatGPT/Codex 上游凭据失效：写 account `credential_status=needs_login`。
- OpenAI-compatible 直连 API key 错误：写 account `credential_status=needs_login` 或 account error，按 source kind 归类。

生成请求 `5xx/EOF/timeout`：

- 写入 `serving_status=cooldown`。
- 更新 `last_failure_at`、`cooldown_until`、`failure_count`。
- 不写 `credential_status=needs_login`。
- 不改变 quota/subscription 阻断状态。

### Quota 写入规则

quota 探测只能影响 quota 状态：

| quota 结果 | 写入 |
| --- | --- |
| `wham/usage` 200 且 `allowed=true` | `quota_status=ok` |
| `wham/usage` 200 但 `allowed=false` 或 `limit_reached=true` | `quota_status=blocked` |
| `wham/usage` 401/403 | `quota_status=error`，记录 normalized reason，不直接写 `needs_login` |
| quota 失败 + 后续模型调用也认证失败 | 升级 `credential_status=needs_login` |
| quota 失败 + 后续模型调用 200 | `credential_status=ok`，`quota_status` 仍为 `error` |

quota `error` 代表“额度接口不可用或不可信”，不是“账号不能生成”。

`quota_status=error` 可以继续路由，但必须考虑最近成功快照：

- `quota_status=ok`：可路由。
- `quota_status=blocked`：不可路由。
- `quota_status=error` 且最近成功 quota 快照新鲜、`allowed=true`：可路由，UI 显示“额度查询失败，使用最近成功快照”。
- `quota_status=error` 且没有成功快照或快照过期：默认可路由但降权，UI 显示“额度未知”。

默认选择“可路由但降权”，因为真实模型调用优先于辅助观测接口。

### Subscription 写入规则

subscription 探测只能影响 subscription 状态：

| subscription 结果 | 写入 |
| --- | --- |
| 明确 active / paid / plus / pro 且未过期 | `subscription_status=active` |
| 明确过期 | `subscription_status=expired` |
| 明确 Free / 无可用订阅 | `subscription_status=free` |
| 正在刷新且没有可用成功快照 | `subscription_status=pending` |
| subscription 接口 401/403/5xx/timeout 或 metadata 解析失败 | `subscription_status=error`，记录 normalized reason，不直接写 `needs_login` |
| 未知或尚未刷新 | `subscription_status=unknown` |

subscription `401/403` 不能单独写 `credential_status=needs_login`。只有 refresh token 明确失效、auth file 缺失/损坏、CPA runtime 明确要求重新认证，或后续模型调用也确认账号上游认证失败时，才升级为 `needs_login`。

### 普通路由决策

普通路由按“能否生成”决策：

- `serving_status=healthy` 且 `credential_status=ok`：正常可路由。
- `credential_status=auth_suspect`：可路由但降权。
- `credential_status=needs_login/refresh_failed/runtime_pending/runtime_error/unknown`：不可路由。
- `quota_status=error`：可路由但显示额度未知，是否降权取决于 freshness。
- `quota_status=blocked`：不可路由。
- `subscription_status=active`：可路由。
- `subscription_status=expired/free/pending/error/unknown`：不可路由；subscription 是资格维度，不能仅凭模型调用成功覆盖。
- `serving_status=cooldown`：不可路由，直到冷却结束。
- runtime credential binding 未确认：CPA 普通流量必须 fail closed，不能静默转发到 provider 级 round-robin。

强制账号路由 `X-Lune-Account-Id` 也不能绕过不可接普通流量状态。管理员诊断请求可以走单独入口绕过，但必须标记 `diagnostic=true`，且不得更新普通路由健康。

## UI 表现

UI 应准确解释 Lune 当前知道什么、不知道什么，以及为什么某个账号可路由、降权或不可路由。不要把所有 CPA 异常都渲染成红色“需要重新登录”。

### 状态文案

| 维度 | 状态 | 推荐文案 |
| --- | --- | --- |
| credential | `ok` | 凭据正常 |
| credential | `auth_suspect` | 鉴权可疑 |
| credential | `needs_login` | 需要重新登录 |
| credential | `refresh_failed` | 凭据刷新失败 |
| credential | `runtime_pending` | CPA runtime 初始化中 |
| credential | `runtime_error` | CPA runtime 异常 |
| credential | `unknown` | 凭据状态未知 |
| quota | `ok` | 额度可用 |
| quota | `blocked` | 额度耗尽或受限 |
| quota | `error` | 额度查询失败 |
| quota | `pending` | 额度刷新中 |
| quota | `unknown` | 额度未知 |
| subscription | `active` | 订阅有效 |
| subscription | `expired` | 订阅已过期 |
| subscription | `free` | 当前为 Free |
| subscription | `pending` | 订阅刷新中 |
| subscription | `error` | 订阅元数据获取失败 |
| subscription | `unknown` | 订阅状态未知 |
| serving | `healthy` | 生成请求正常 |
| serving | `cooldown` | 生成请求短期熔断 |
| serving | `error` | 生成请求持续失败 |
| serving | `unknown` | 生成状态未知 |

quota `error` 的详情应展示最近成功刷新时间、最近尝试失败时间、normalized reason 和最近模型调用是否成功。如果最近模型调用成功，文案应类似：

```text
额度查询失败
最近模型调用成功，quota 接口返回 token_invalidated。
```

只有连续模型调用也出现账号上游认证失败时，才切换为“需要重新登录”。

### UI 优先级

账号摘要可以按以下顺序展示最重要阻断：

1. runtime credential binding unavailable。
2. `credential_status=needs_login/refresh_failed/runtime_error/runtime_pending/unknown`。
3. `subscription_status=expired/free/pending/error/unknown`。
4. `quota_status=blocked`。
5. `serving_status=cooldown/error`。
6. `quota_status=error` 或 `credential_status=auth_suspect`，展示为降权/未知而不是硬错误。

摘要展示不能覆盖诊断。账号详情抽屉的 `诊断` tab 应显示 Runtime Binding、credential、quota、subscription、serving 的独立值、最近更新时间和最近原因。

### 账号卡片 Chip 与 Badge

Pool 详情页账号卡片只展示高信息密度摘要，不把四维状态全部铺开。卡片 chip 最多三个槽位：

1. 请求量：常驻，例如 `今日 123` 或 `24h 123`。
2. 订阅：常驻或由订阅异常替换，例如 `14 天后到期`、`3 天内到期`、`已过期`、`Free`、`订阅获取失败`、`订阅未知`、`订阅刷新中`。
3. 主问题：最多一个，只显示当前最重要的非订阅异常。订阅异常由第二槽表达，不在第三槽重复展示。

`不接流量` 不作为 chip。它是路由结果，应通过卡片整体状态、右上角状态 badge、tooltip 或详情页解释。正常态不显示 `凭据正常`、`额度可用`、`订阅有效`、`生成请求正常` 等 chip。

主问题 chip 按优先级选择一个：`Binding 未确认`、`需要重登`、`凭据刷新失败`、`CPA Runtime 异常`、`凭据同步中`、`凭据状态未知`、`额度已用尽`、`服务冷却中`、`服务异常`、`额度查询失败`、`额度未知`、`使用额度快照`、`鉴权待确认`。

chip 颜色表达问题类别，不表达完整严重等级：

- 红色：强阻断或高危险，例如 `需要重登`、`额度已用尽`、`已过期`、`Free`、`CPA Runtime 异常`、`凭据刷新失败`、`凭据状态未知`、`服务异常`。
- 黄色：降级、观测失败、临近风险或需要注意，例如 `额度查询失败`、`额度未知`、`使用额度快照`、`鉴权待确认`、`订阅获取失败`、`订阅未知`。
- 蓝色：系统处理中或短暂等待，例如 `凭据同步中`、`订阅刷新中`、`额度刷新中`、`服务冷却中`。
- 紫色：绑定、归因或统计可信度问题，例如 `Binding 未确认`。
- 灰色：普通信息或低强调提示，例如 `今日 123`、`24h 123`、`14 天后到期`。
- 绿色：整体正常或明确正向状态，卡片 chip 默认少用。

右上角状态 badge 表达组合后的路由摘要，保持五态：`正常`、`降级`、`异常`、`待检查`、`已停用`。

### 账号详情抽屉

账号详情抽屉不新增第四个 tab，也不继续把 `Debug` 作为面向用户的一等入口。tab 结构应调整为：

```text
Overview / Playground / 诊断
```

`Overview` 只保留日常摘要：短路由摘要、影响说明、请求量、成功率、延迟、quota bars、models、自检配置等。不在 Overview 展开完整四维状态表。

`Playground` 保留真实请求直测能力：选择模型并发送测试消息，展示响应、错误、延迟和 usage。失败时可以提供可复制 curl 或等价诊断材料，但不得泄露完整 token。

`诊断` 替代旧 `Debug`，展示路由结论、主因和影响，展开 Runtime Binding、凭据、订阅、额度、服务能力及建议操作。旧 `Debug` 中的底层字段放到 `高级信息` 折叠区。

### Active Pool 卡片高度

v0.1.6 规格后续统一使用 `需要重登` 作为卡片 chip 文案；历史 `请重登` 仅说明 v0.1.5 的已完成修复背景。若未来仍因请求数字过长、更多 status chip 或窄屏宽度导致换行，再考虑固定 active card 高度、让 `AccountCard` 继承 grid row 高度、chip 区域改为单行省略或聚合。

## 日志与诊断

状态写入应记录来源、前值、后值、时间和单调版本语义，便于解释为什么某个辅助接口不能覆盖真实业务调用结果。

账号详情 `诊断` tab 应展示：

- Runtime Binding、credential、quota、subscription、serving 的独立值。
- 最近更新时间。
- 最近 normalized reason。
- 路由判断和建议操作。

quota/subscription 错误详情可以展示安全截断后的 reason，但不得展示 token、完整 request body 或完整 auth file。

## 测试与验收

- quota `401/403` 且模型调用仍成功时，UI 显示“额度查询失败”，不显示“需要重新登录”。
- quota `allowed=false` 或 `limit_reached=true` 时，普通路由跳过该账号。
- subscription 接口 `401/403` 只写 `subscription_status=error`，不单独写 `credential_status=needs_login`。
- `subscription_status=expired/free/pending/error/unknown` 时，普通路由跳过该账号。
- 生成请求 `200` 不会清除或改写任何 quota/subscription 状态。
- 生成请求 `5xx/EOF/timeout` 进入 `serving_status=cooldown`，后续独立请求在冷却期内绕过该账号。
- 生成请求 `401/403` 只有确认是账号上游凭据问题时，才写 `credential_status=needs_login`。
- `auth_suspect` 默认可路由但降权，优先选择其他 `credential_status=ok` 账号。
- 强制账号路由不能绕过不可接普通流量状态。
- Pool 详情页账号卡片最多展示请求量、订阅、主问题三个 chip；右上角 badge 只表达组合路由摘要。
- 账号详情抽屉使用 `Overview / Playground / 诊断` 三个 tab。
- Active Pool 卡片在常见 chip 组合下高度稳定。

### Docker 容器验收

02 的最终验收不能只依赖单元测试。实现完成后必须用新 v0.1.6 镜像启动一次临时 Docker 容器，使用全新数据目录和可控 mock upstream / mock CPA 响应验证状态隔离。测试容器不得复用或影响上一版本正在运行的容器，结束后必须删除。

容器验收至少覆盖：

- 启动新容器后，通过 API 创建测试 Pool、token 和至少两个测试账号；一个账号用于模拟 quota/subscription 辅助接口异常，另一个账号用于验证路由 fallback。
- 模拟 quota `401/403`，再让同一账号的已确认 runtime binding 模型请求返回 `200`：API/页面必须显示 `quota_status=error` 或等价“额度查询失败”，不得把 `credential_status` 改成 `needs_login`，也不得清除 quota error。
- 模拟 quota `allowed=false` 或 `limit_reached=true`：普通路由必须跳过该账号，强制账号路由也不能绕过该阻断。
- 模拟 subscription `401/403` 或 metadata 解析失败：只写 `subscription_status=error`，不得写 `credential_status=needs_login`；Codex 普通路由必须跳过该账号。
- 模拟真实模型请求 `5xx/EOF/timeout`：账号进入 `serving_status=cooldown`，后续独立请求路由到另一个健康账号；cooldown 不得表述成登录失败。
- 模拟真实模型请求 `200`：只修复 serving 维度和可疑 credential，不得清除 quota blocked/error 或 subscription expired/error。
- 在管理 UI 或 API 响应中确认账号卡片/详情能区分 `额度查询失败`、`订阅元数据获取失败`、`服务冷却中`、`需要重登`，而不是统一显示红色重登。
- 检查 `request_logs` / Activity 中的 routed account、状态结果和错误摘要能解释本次路由选择；未确认 runtime binding 的 CPA 请求不得被用来修复该账号健康。

验收记录应保留：镜像 tag 或 digest、容器启动命令、mock upstream/CPA 行为配置、关键 API 响应摘要、UI 截图或 Playwright 断言、清理测试容器的命令结果。

## 已完成事项

- 增加账号级 serving 熔断状态：`serving_status`、`failure_count`、`last_failure_at`、`last_success_at`、`cooldown_until`。
- discovery health 与 serving health 拆分；`/v1/models` 成功不再直接清除网关 serving error。
- 缩窄 `needs_login` 判定，不再仅凭任意 `403` 或宽泛文本写入。
- quota `allowed=false` 或 `limit_reached=true` 时设置 `quota_status=blocked`。
- token 认证检查 `access_tokens.enabled`。
- 禁用 token 后，网关拒绝该 token 的非 `/v1/models` 请求。
- Pool 详情页账号卡片中，CPA 凭据状态 chip 曾缩短为“请重登”以修复高度问题；后续统一为 `需要重登`。
- 路由选择已同时检查账号启用、Pool member 启用、serving、credential、subscription、quota 和 runtime binding。
- Codex subscription 非 `active` 时普通路由阻断；该资格维度不会被模型调用成功覆盖。
- `credential_status=auth_suspect` 已按可路由但降权处理，优先选择 `credential_status=ok` 的账号。
- CPA runtime credential binding 未确认时，普通 CPA 流量 fail closed，且不会用模型调用结果更新该账号健康。
- Pool 统计已按 confirmed CPA binding 计算可信 request/usage，避免 provider round-robin 导致账号请求量错记。
- 前端和后端状态大小写处理已对齐，降低状态值大小写不一致导致的 UI/路由误判。
- 已补充 router/store 单元测试覆盖 subscription 阻断、`auth_suspect` 降权、confirmed binding 可信统计。

## 待解决事项

- quota 缓存补齐 `quota_last_attempt_at`、`quota_last_success_at`、`quota_last_error`。
- 增加状态写入来源、前值、后值、时间和单调版本语义。
- 账号详情 `诊断` tab 仍可继续扩展为完整四维状态时间线。
