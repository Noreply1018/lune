# 04. CPA 分层可路由模型与 Codex Free 接入

状态：已实现并完成新容器验收。本文定义 v0.1.8 必须实现的 CPA 账号分层可路由模型，重点解决 Codex Free / Go 账号可以接入、可以被正确判定可路由、可以显示计划与额度的问题。

来源：2026-05-17 对当前 Lune CPA 账号生命周期、Codex subscription / quota / router / 前端展示逻辑的代码审计，以及用户确认 v0.1.8 需要支持 Free 账号正常接入和路由。OpenAI 当前官方说明中，Codex 包含在 ChatGPT Free / Go / Plus / Pro / Business / Enterprise 等计划中，其中 Free / Go 属于限时包含；因此 Lune 不能继续把 Codex CPA 可用性等同于 paid subscription active。

## 问题

当前实现把 Codex CPA 账号是否可路由和 `cpa_subscription_status='active'` 绑定：

```text
Codex CPA 可路由必须 subscription active
```

这在只考虑 Plus / Pro 的阶段可以工作，但引入 Free / Go 后不再成立：

- Free / Go 是计划身份，不一定有 `chatgpt_subscription_active_until`。
- 缺少 subscription active until 不代表 Codex access 不可用。
- Free 可能只有短窗口额度，没有周额度；现有前端固定假设 `5h + 7d` 两个窗口。
- quota 查询失败、quota blocked、access ineligible 和 serving cooldown 是不同语义，不能合并为同一个“账号不可用”。
- 前端当前已经有 `free` subscription status 类型，但其语义是红色“订阅不可用”，这会误导用户。

v0.1.8 必须把 CPA 可路由规则从零散字段判断升级为统一的分层判定模型。

## 分层模型

CPA 账号普通流量可路由性由以下层级按顺序判定：

```text
Account / Pool
Credential
Runtime Binding
Access
Quota
Serving
Models
```

每层都必须输出稳定机器语义：

```text
status: pass | warn | block | pending | unknown
reason: stable machine code
message: human readable summary
source: auth_file | cpa_management | wham_usage | model_request | config | store
checked_at
```

`source` 表示证据来自哪条链路；`reason` 表示这条链路给出的稳定错误或状态类别。不得把 `quota_fetch`、`runtime_api_call` 这类错误类别写进 `source`。例如：

```text
source=wham_usage, reason=quota_fetch_auth_failed
source=cpa_management, reason=runtime_api_call_failed
source=model_request, reason=model_request_429
```

最终聚合规则：

```text
block -> 不可路由
pending / unknown -> 默认不可路由，除非该层策略明确允许 fail-open
warn -> 可路由但产生 penalty
pass -> 正常
```

v0.1.8 必须把每一层的 fail-open / fail-closed 策略写死，避免实现阶段把所有 `unknown` 都当成硬阻断，或把 access 待确认错误地放行：

| 层级 | `pending` / `unknown` 默认策略 | 说明 |
| --- | --- | --- |
| Account / Pool | block | Pool、账号或 Pool member 状态不明确时不接普通流量 |
| Credential | block | 凭据状态不可信时不接普通流量 |
| Runtime Binding | block | CPA provider pinning 或 runtime binding 不可信时 fail closed |
| Access | block | Free / Go / unknown 计划必须先有 access evidence，不能只凭计划身份放行 |
| Quota | warn | quota snapshot 缺失或刷新中不等于额度阻断；明确 blocked 或模型请求 429 evidence 才 block |
| Serving | block | `cooldown` 未过期或 `error` 阻断；健康或冷却已过期才 pass / warn |
| Models | conditional | 有明确模型列表且不包含请求模型时 block；模型列表为空或未知只在没有明确匹配账号时作为兜底 |

v0.1.8 最低闭环要求后端新增统一评估函数或等价共享逻辑：

```go
EvaluateAccountRoutability(account, options) RoutabilityDecision
```

返回值至少包含：

```go
type RoutabilityDecision struct {
    Routable bool
    Penalty int
    BlockingLayer string
    BlockingReason string
    Layers []RoutabilityLayer
}
```

router、Pool routable count、Route summary 和 Diagnostics 必须使用同一套规则或有测试锁定的等价规则，不能继续由 Go router、SQL 和前端 TypeScript 各自维护冲突判断。

推荐实现路径：

1. 后端实现统一 `RoutabilityDecision`，作为“账号能不能接普通流量”的唯一裁判。
2. router 直接消费该决策；Diagnostics 直接展示该决策的 layers。
3. Pool routable count 优先由同一评估逻辑统计。若实现阶段因性能或 SQL 查询成本暂时保留 SQL count，则必须新增金样测试，证明 SQL count 与统一决策在同一组 fixture 上完全一致。
4. 前端 Route summary 不再自行重新定义硬阻断规则；可以渲染后端返回的决策，也可以用共享字段派生展示，但必须有测试锁定与后端决策一致。

必须新增“统一裁判金样测试”：同一批 fixture 同时断言 router 选择、Pool `routable_account_count`、Route summary 和 Diagnostics layer。该测试必须覆盖 Free access、quota fetch error、模型请求 429、runtime binding、模型明确不支持、模型未知兜底和 serving cooldown。

## Access 与 Subscription 拆分

v0.1.8 必须引入 Codex access / entitlement 概念，不能继续把 subscription 当作唯一 gate。

推荐新增字段：

```sql
cpa_access_status TEXT NOT NULL DEFAULT 'unknown'
cpa_access_reason TEXT NOT NULL DEFAULT ''
cpa_access_last_error TEXT NOT NULL DEFAULT ''
cpa_access_checked_at TEXT NOT NULL DEFAULT ''
```

允许状态：

| 状态 | 语义 | 路由影响 |
| --- | --- | --- |
| `eligible` | 账号具备该 CPA provider 的普通模型使用资格 | pass |
| `limited` | 账号具备资格，但当前权益或策略受限，需要结合 quota 判断 | warn 或 block，由 quota 决定 |
| `ineligible` | 账号明确不具备该 provider 使用资格 | block |
| `pending` | access 证据正在同步或探查中 | block |
| `error` | access 探查失败，但没有明确不可用证据 | warn 或 block，取决于最近模型成功证据 |
| `unknown` | 没有可信 access 证据 | block |

`cpa_subscription_*` 字段继续保留，但语义收窄：

```text
cpa_subscription_* 只表示 paid subscription 元数据
cpa_access_* 表示 Codex / CPA provider 是否可用
```

Paid plan 的 `subscription active` 可以推导 `access_status=eligible`；Free / Go 不能依赖 subscription，需要使用 quota 或模型请求证据。

## Codex Access 证据

Codex `access_status=eligible` 可以由以下证据推导：

1. `chatgpt_subscription_active_until` 存在且未过期。
2. `wham/usage` 返回成功，并且没有 `allowed=false`、`limit_reached=true`、`blocked=true` 等明确拒绝。
3. 最近一次普通模型请求成功。
4. CPA management 明确返回该账号具备 Codex access。

证据优先级采用“强证据优先”，不是“最新一次探测覆盖一切”：

```text
明确 access denied / plan unsupported > 明确 wham/usage allowed > 最近模型请求成功 > paid subscription active > pending / unknown / transient error
```

已经确认 `eligible` 的账号，后续短暂 quota / access 探测失败时不立刻降级为 `unknown` 或 `ineligible`。只有明确拒绝证据才能把 `eligible` 打回 `ineligible`。`quota blocked`、模型请求 `429`、quota fetch `401/403/request failed` 都不能把 access 改成 `ineligible`。

Codex `access_status=ineligible` 可以由以下证据推导：

1. access / entitlement 接口明确返回无权限。
2. `wham/usage` 或模型请求返回明确“plan unsupported / not eligible / access denied”类安全摘要。
3. paid subscription 已过期且没有 Free / Go 可用证据。

以下情况不得推导为 `ineligible`：

- quota blocked。
- 模型请求 429。
- quota fetch HTTP 401 / 403。
- quota fetch request failed。
- subscription metadata pending。

这些情况应分别落入 quota、credential、runtime 或 access error / pending，而不是“没有资格”。

## Quota 语义

Quota 只表达当前额度与限流状态，不表达计划身份或 access 资格。

推荐状态：

| 状态 | 语义 | 路由影响 |
| --- | --- | --- |
| `ok` | 最近 quota 快照未显示阻断 | pass |
| `limited` | 已接近或处于临界状态，但未明确阻断 | warn |
| `blocked` | quota 明确阻断或真实模型请求明确额度/限流阻断 | block |
| `error` | quota 查询失败或裸 429 证据 | warn 或 block，取决于 error kind |
| `pending` | quota 正在同步 | warn |
| `unknown` | 没有 quota 快照 | warn |

v0.1.8 可以先复用现有 `cpa_quota_status`，但 UI 和路由必须区分：

- `HTTP 429 from model request`：模型请求层限流证据，普通路由阻断。
- 明确 quota / rate-limit / limit reached 文案：quota blocked，普通路由阻断。
- quota fetch HTTP 401 / 403：额度接口鉴权失败，不能等同模型限流。
- quota fetch request failed / 502 / timeout：额度查询失败，不能等同模型限流。

quota fetch `401/403` 不能自动改写 Access 或 Credential。实现必须保留来源链路和错误类别：

```text
source=wham_usage, reason=quota_fetch_auth_failed
  -> Quota warn，展示额度查询失败 / 额度接口鉴权失败

source=cpa_management, reason=runtime_api_call_failed
  -> Runtime Binding 或 Credential 诊断可提示管理调用失败，但不能伪装成模型限流

source=model_request, reason=model_request_429
  -> 只有真实普通模型请求 429 才进入模型请求限流 evidence
```

如果后端暂时只保存 `cpa_quota_last_error` 一个摘要字段，也必须在派生 meta 或 Diagnostics 中给出等价 `source/reason`，避免把 management api-call 失败、目标 wham 鉴权失败和真实模型请求失败混成同一类问题。

Free / Go quota 展示必须按实际返回窗口动态渲染，不能固定假设一定有 `5h` 和 `7d` 两个窗口。没有周额度窗口时，不渲染假的 7d pending bar。

## Codex Free / Go 路由规则

Codex Free / Go 账号可路由条件：

```text
account enabled
pool member enabled
credential pass 或 warn
runtime binding pass
access_status=eligible 或 limited
quota 未 block
serving 未 block
模型匹配或模型未知兜底
```

`plan_type=free` 本身不够成为可路由证据；它只是计划身份。Free 账号必须通过 `wham/usage` 成功、普通模型请求成功或 CPA management 明确 access 证据进入 `access_status=eligible`。

Free / Go 的常见状态解释：

| 输入状态 | 期望判定 |
| --- | --- |
| plan free，wham/usage allowed，quota 未阻断 | 可路由 |
| plan free，wham/usage 只有 primary window | 可路由，UI 只展示实际窗口 |
| plan free，无 subscription active until，access 未验证 | 不可路由，显示 access 待确认 |
| plan free，模型请求成功但 quota fetch 失败 | 可路由但 quota warn |
| plan free，模型请求 429 | quota / serving 阻断 |
| plan free，明确 access denied | access ineligible，阻断 |

## Refresh 流程

CPA 账号刷新应按以下顺序收敛：

1. 读取 auth file 和 CPA management auth index。
2. 更新 credential 状态。
3. 更新 runtime binding / auth index 状态。
4. 解析并保存 `plan_type`。
5. 刷新 access：
   - paid plan：subscription active 可推导 access eligible。
   - Free / Go / unknown：用 `wham/usage` 或轻量模型证据推导 access。
6. 刷新 quota，并保存原始 snapshot 与安全派生 meta。
7. 刷新模型列表。

`wham/usage` 同时可以提供 access 证据和 quota 证据，但持久化时必须拆成两个维度。

### 首次接入 Free / Go

Free / Go 账号首次添加采用混合流程：

```text
1. 先创建 / 更新账号和 auth file。
2. 初始 access 显示为待确认。
3. 后台异步执行一次 `wham/usage` 探测。
4. wham/usage 成功且未明确 blocked -> access eligible，并保存 quota snapshot。
5. wham/usage 明确 blocked -> access 不写 ineligible，quota 写 blocked。
6. wham/usage 401 / 403 / request failed / 5xx -> access 保持 pending/unknown/error，quota 显示查询失败；不判死账号。
7. 后续真实模型请求成功，可以把 access 提升为 eligible。
```

首次接入后不自动发真实模型请求。原因是 Free 额度很少，自动 probe 可能消耗用户未预期的额度。真实模型请求成功只作为后续自然证据。

### 刷新节奏与降级

Access 和 quota 均采用缓存 + 定期刷新 + 失败退避：

- 不在每次普通请求前刷新 access 或 quota。
- `eligible` 可以继续路由，直到出现明确拒绝、quota blocked、模型请求 429、serving cooldown/error 或其他硬阻断。
- 短暂探测失败只更新错误摘要和 checked_at，不立刻清空已有 eligible 结论。
- 失败重试应使用退避策略，避免 quota / access 探测失败时持续打 CPA management。
- 具体刷新间隔和退避周期由实现阶段确定，但必须在测试矩阵中固定并覆盖。

实现阶段必须把 refresh/backoff 参数沉淀为可测试配置或常量，并在验收矩阵中声明实际值。最低要求：

- access / quota 刷新失败后进入退避窗口。
- 退避窗口内不会持续调用 CPA management 或 wham/usage。
- 退避窗口过后允许再次刷新。
- 刷新成功后清理或重置退避状态。
- 退避期间已确认 `eligible` 的账号不因短暂失败被降级为 `ineligible`。

## UI 表现

账号卡片和详情抽屉必须把计划身份、access 和 quota 分开展示。Free 不新增第三种卡片形态，继续使用现有 Codex CPA 卡片。

卡片结构：

```text
左上：来源，例如 Codex
右上：[Plan chip] [健康 chip]
中间：quota bars
底部：[今日 N] [主问题 chip]
```

底部 chip 不再显示 subscription 到期。Plus / Pro 到期信息转移到详情页 header、Access 细节或 Diagnostics，不再挤占卡片底部摘要。

Plan chip 规则：

| 计划 | chip 文案 | 颜色语义 |
| --- | --- | --- |
| Free | `Free` | 中性或青绿色，不使用红色 |
| Go | `Go` | 中性或青绿色 |
| Plus | `Plus` | 蓝紫 / 月相紫 |
| Pro | `Pro` | 深紫或强调色 |
| Unknown | `Unknown` | 灰色 |

Plus / Pro quota 示例：

```text
5h    [bar]    81%
7d    [bar]    64%
```

Free quota 示例：

```text
5h    [bar]    81%
Plan  短周期额度  无周额度
```

Free 只有一个真实 quota window 时，第二行固定显示 `Plan  短周期额度  无周额度`，用于保持所有 Codex 卡片高度一致，但不伪造 7d 窗口。如果未来 Free 返回第二个真实窗口，则显示真实窗口，不强行显示“无周额度”。

Free quota 尚未查到时，第一行显示 pending / unknown bar，第二行仍显示：

```text
Plan  短周期额度  无周额度
```

底部主问题 chip 只在有问题时出现，例如：

```text
Access 待确认
Access 不可用
额度查询失败
额度受限
请求限流
需要重登
Binding
```

如果账号可路由且没有主问题，不显示 `Access 可用` chip。可用性由健康 chip 和 quota 区域表达。

不允许：

```text
Free -> 红色“订阅不可用”
Free -> 因无 7d 窗口显示周额度 pending
quota fetch 401 -> 模型请求被限流
卡片底部继续显示“30 天后到期”这类 subscription chip
```

详情抽屉 Diagnostics 主维度：

```text
Credential
Runtime Binding
Access
Quota
Serving
Models
```

`Subscription` 只作为 paid plan 的 Access 细节出现，不再作为所有 Codex CPA 的主诊断 gate。

## 测试与验收

### 单元与集成测试

- router 测试：Codex Free + access eligible + quota ok 可以路由。
- router 测试：Codex Free + access unknown 不可路由。
- router 测试：Codex Free + quota blocked 不可路由。
- router 测试：Codex Plus + subscription active 推导 access eligible。
- router 测试：Codex Plus + subscription expired 且无 Free / quota / model success 证据时不可路由。
- health 测试：没有 `chatgpt_subscription_active_until` 的 Free auth file 不写成订阅异常。
- health 测试：wham/usage success 可以写入 access eligible。
- health 测试：wham/usage allowed=false 写入 quota blocked，不写 access ineligible。
- health 测试：Free 首次导入后 access 先为 pending/unknown，并异步 wham/usage 探测。
- health 测试：eligible 后一次 wham/usage 401/403/request failed 不立刻降级为 ineligible。
- health 测试：明确 access denied / plan unsupported 才写入 ineligible。
- health 测试：access / quota 刷新失败后进入退避，退避窗口内不会重复打 CPA management / wham/usage。
- health 测试：退避窗口过后允许再次刷新，刷新成功后清理或重置退避状态。
- gateway 测试：模型请求成功可以清除旧模型请求 429 evidence，并可作为 access eligible 证据。
- gateway 测试：模型请求 429 写 quota / serving 阻断，不写 access ineligible。

### 前端测试

- AccountCard：右上角健康 chip 左侧显示 plan chip；Free 为中性或青绿色，不能显示红色“订阅不可用”。
- AccountCard：底部 chip 只显示 `今日 N` 和主问题，不再显示到期 chip。
- AccountCard：Free 只有 primary quota window 时，第二行显示 `Plan  短周期额度  无周额度`。
- AccountCard：Free quota pending 时仍保持两行高度，第二行显示 `Plan  短周期额度  无周额度`。
- AccountDetailSheet：Diagnostics 包含 Access 维度。
- AccountDetailSheet：Subscription 仅作为 paid plan access 细节，不作为 Free 的阻断主项。
- quota parser：支持只有 primary window 的 snapshot。
- route summary：Free + access eligible 显示可路由，不被 subscription unknown 阻断。

### 容器验收

实现后必须使用新启动测试容器完成验收，不能复用正在运行的旧版本容器。

最小 fake CPA 矩阵：

| 编号 | 场景 | 期望 |
| --- | --- | --- |
| CT-FREE-01 | Free auth JSON，无 subscription active until，wham/usage allowed，只有 primary window | 账号可路由；卡片显示 Free 和单窗口额度 |
| CT-FREE-02 | Free auth JSON，wham/usage allowed=false | access 不写 ineligible；quota blocked；普通路由跳过 |
| CT-FREE-03 | Free auth JSON，quota fetch 401，模型请求成功 | access eligible；quota warn；普通路由可用但降权 |
| CT-FREE-04 | Free auth JSON，普通模型请求 429 | quota / serving 阻断；不写 access ineligible |
| CT-FREE-05 | Free 模型请求成功 | 可以作为 access eligible 自然证据 |
| CT-FREE-06 | Plus auth JSON，subscription active | access eligible；保持现有 paid 行为 |
| CT-FREE-07 | Plus auth JSON，subscription expired，无其他可用证据 | access ineligible；普通路由跳过 |
| CT-FREE-08 | UI 移动端和桌面 | Free 计划 chip、quota 单窗口、Access 诊断无重叠 |
| CT-FREE-09 | Free quota 只有 primary window | 卡片中间区域两行高度稳定，第二行为 `Plan  短周期额度  无周额度` |
| CT-FREE-10 | Free quota pending | 第一行为 pending，第二行为 `Plan  短周期额度  无周额度`，卡片高度与 Plus 一致 |
| CT-FREE-11 | 退避恢复 | 退避窗口过后 fake CPA 恢复成功 | 允许再次刷新；成功后清理或重置退避状态 |
| CT-FREE-12 | 测试清理 | 测试容器和临时数据已删除 |

## 已确认口径

- Free / Go 是计划身份，不是异常状态。
- `subscription active` 不是 Codex CPA 的通用可路由必要条件。
- Access 和 quota 必须拆开；quota blocked 不代表账号没有 Codex access。
- Free 可路由必须有 access evidence，不能只看 `plan_type=free`。
- Free 首次接入先待确认，后台异步 `wham/usage` 探测；不自动打真实模型请求。
- 已确认 eligible 的账号遇到短暂探测失败时先保留可用结论，不立刻降级。
- Free quota UI 必须按实际窗口动态展示，不能伪造 7d 窗口；只有一个窗口时第二行显示 `Plan  短周期额度  无周额度`。
- 卡片底部不再显示 subscription 到期 chip；计划身份使用右上角 plan chip。
- router、Pool count、Route summary 和 Diagnostics 必须共享分层规则或由测试保证等价。
