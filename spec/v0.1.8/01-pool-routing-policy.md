# 01. Pool 路由策略：健康优先与排序优先

状态：规划中。本文定义 v0.1.8 必须实现的 Pool 级路由策略、UI 切换、日志解释和验收边界。

来源：用户确认当前账号池存在人工排序，但现有路由会把部分“降级但可用”的账号自动后置。用户希望在某些 Pool 中先使用被标记为“降级”的账号，只要该账号仍可接普通流量。

本文的策略切换必须建立在 `04-cpa-routability-layer-model.md` 的分层可路由模型之上。`ordered` 只改变轻降级账号与正常账号之间的优先级，不重新定义 Credential、Runtime Binding、Access、Quota、Serving 或 Models 的硬阻断语义。

## 问题

现有版本的账号选择逻辑是隐式健康优先：

- 请求使用 Pool token 后，只在该 token 绑定的 Pool 内选账号。
- Pool member 按 `position` 排序。
- 路由层先按模型匹配和 penalty 分层，再在每一层内按 `position` 选择。
- CPA `auth_suspect`、quota `error` 等轻降级状态会产生 penalty，因此会被正常账号插队。
- `status=degraded` 本身仍可路由，且当前没有单独降权。
- retry 会排除当前失败账号后重新走同一套路由逻辑。

这导致用户的心智和实际行为存在偏差：

```text
账号 1：降级但还能接流量
账号 2：正常
账号 3：正常
```

用户把账号 1 放在第一位，是想先用账号 1；但现有健康优先会先选择账号 2。该行为有利于成功率，但不适合用户明确想按顺序消耗账号、验证轻降级账号或控制账号使用节奏的场景。

## 改进策略

v0.1.8 为 Pool 增加显式路由策略：

```text
health_first
ordered
```

数据字段：

```text
pools.routing_policy TEXT NOT NULL DEFAULT 'health_first'
```

### 健康优先

`health_first` 是默认策略，延续现有健康优先的调度意图，并在 v0.1.8 收紧模型兜底边界：

1. 优先选择明确支持请求模型且无 penalty 的账号。
2. 再选择明确支持请求模型但有 penalty 的账号。
3. 再选择模型列表为空或未知且无 penalty 的账号。
4. 最后选择模型列表为空或未知但有 penalty 的账号。
5. 每个阶段内部仍按 Pool member `position` 排序。

当前实现的模型兜底阶段只区分是否要求模型命中，不能排除“模型列表非空但不包含请求模型”的账号。v0.1.8 必须修正这一点：明确不支持请求模型的账号，在 `health_first` 和 `ordered` 下都不能被选中。

适合场景：

- 用户优先追求成功率。
- 降级账号只作为兜底。
- 账号排序表达同健康层级内的优先级。

### 排序优先

`ordered` 是新增策略：

1. 第一轮按 Pool member `position` 扫描，选择第一个明确支持请求模型且可接普通流量的账号。
2. 如果第一轮没有结果，第二轮按 Pool member `position` 扫描，选择第一个模型列表为空或未知且可接普通流量的账号。
3. 轻降级账号不因为 penalty 被自动后置。
4. 明确不支持请求模型的账号不得被选中。

适合场景：

- 用户想严格按账号列表顺序消耗账号。
- 用户知道某个降级状态是轻微风险或临时误判。
- 用户希望手动排序成为主要调度控制。

示例：

```text
账号 1：auth_suspect，但支持模型且可接流量
账号 2：正常，支持模型
账号 3：正常，支持模型
```

`health_first`：

```text
先选账号 2；账号 2 不可用或失败后再考虑账号 3；最后才考虑账号 1。
```

`ordered`：

```text
先选账号 1；账号 1 不可用或失败后再选账号 2。
```

## 降级与硬阻断

本功能必须明确区分“轻降级”和“硬阻断”。

### 轻降级

轻降级代表账号仍可接普通流量，只是风险更高或观测不完整。`ordered` 下轻降级账号不自动后置。

轻降级包括：

- `status='degraded'` 且其他硬阻断条件不存在。
- CPA `cpa_credential_status='auth_suspect'`。
- CPA `cpa_quota_status='error'`，但不是 Codex 模型请求 `HTTP 429 from model request` 证据。
- 辅助探测失败，但真实模型生成能力没有被硬阻断。

### 硬阻断

硬阻断代表账号不可接普通流量。两种策略都必须跳过。

硬阻断包括：

- 账号 `enabled=false`。
- Pool member `enabled=false`。
- Pool `enabled=false`。
- 账号 `status` 不是 `healthy` 或 `degraded`。
- 非 diagnostic 普通请求中，`serving_status='cooldown'` 且冷却未过期。
- 非 diagnostic 普通请求中，`serving_status='error'`。
- CPA runtime binding 不可确认或 provider pinning 不支持。
- CPA `cpa_credential_status` 为 `needs_login`、`refresh_failed`、`runtime_pending`、`runtime_error`、`unknown` 或空值。
- CPA `cpa_quota_status='blocked'`。
- Codex CPA `cpa_quota_status='error'` 且 `cpa_quota_last_error` 以 `HTTP 429 from model request` 开头。
- CPA access 状态为 `ineligible`、`pending`、`unknown` 或等价硬阻断状态。
- Paid Codex CPA subscription 已过期且没有 Free / Go / quota / model success 等 access 可用证据。
- 账号模型列表明确存在，且不包含请求模型。

## 模型匹配

排序优先不能把模型匹配降级为“先打排第一再说”。否则会让明确不支持该模型的账号抢走请求。

模型选择规则：

1. 如果账号模型列表明确包含请求模型，则视为模型匹配。
2. 如果账号模型列表为空，表示未知或尚未发现；仅在没有明确匹配账号时作为兜底。
3. 如果账号模型列表不为空且不包含请求模型，则视为明确不支持，不可被选中。

`health_first` 与 `ordered` 都必须遵守以上规则。

## 重试行为

重试继续沿用当前 Pool 的路由策略，并排除已尝试失败账号。

`health_first`：

```text
当前健康账号失败
排除该账号
继续在健康优先分层中寻找下一个账号
健康账号耗尽后再考虑轻降级账号
```

`ordered`：

```text
当前排序靠前账号失败
排除该账号
继续按列表顺序寻找下一个可接普通流量账号
```

强制账号请求 `X-Lune-Account-Id` 不参与 Pool 策略切换。该请求仍必须满足硬阻断规则，且失败后不得悄悄切到其他账号。

## UI 表现

Pool 详情页在现有“自检 Pool”按钮右侧增加一个策略切换按钮。

交互要求：

- 按钮展示当前策略名称。
- 点击后在 `健康优先` 和 `排序优先` 间切换。
- 不新增长说明文案，不新增独立设置页。
- 切换成功后刷新 Pool 详情数据。
- 保存失败时保留当前显示状态，并给出轻量错误反馈。

视觉要求：

| 策略 | 按钮文案 | 颜色 |
| --- | --- | --- |
| `health_first` | 健康优先 | 青绿色或绿色，表达稳定、优先成功率 |
| `ordered` | 排序优先 | 月相紫或靛紫，表达手动编排、按列表执行 |

按钮不承担健康状态提示，不使用红色或黄色。账号卡片、badge 和 chip 继续表达账号健康与阻断原因。

账号列表排序语义需要在 UI 行为中保持一致：

- `health_first` 下，拖拽排序改变同一健康层级内的先后。
- `ordered` 下，拖拽排序改变主要调度顺序。

## API 与数据交互

Pool API 需要读写 `routing_policy`。

最低接口要求：

- `GET /pools` 返回每个 Pool 的 `routing_policy`。
- `GET /pools/{id}` 返回 `pool.routing_policy`。
- `PUT /pools/{id}` 或独立策略更新接口可以修改 `routing_policy`。
- 仅接受 `health_first` 和 `ordered`；未知值返回 400。
- 迁移旧数据时填充默认值 `health_first`。
- Routing cache 必须包含策略字段，策略变更后必须 invalidate cache。

前端类型定义、Pool 详情页数据加载、保存动作和错误提示必须同步更新。

## 日志与诊断

路由结果需要可解释，尤其是 `ordered` 下排第一账号没有被选中时。

request log、diagnostic response 或结构化调试日志必须提供可复核载体，并沉淀：

- `routing_policy`：`health_first` / `ordered`。
- `selected_account_id`：最终选择账号。
- `attempt_count`：最终尝试次数。
- `account_penalty`：被选账号的 penalty 数值或等价摘要。
- `skip_reason`：排在前面的账号被跳过的主要原因。
- retry 关系：同一请求内各 attempt 的账号选择结果，至少能复核“首次账号、最终账号、尝试次数、当前策略”之间的关系。

最低可解释要求：

- route-level 失败能区分 `no_healthy_account`、`model_not_on_account`、`runtime_auth_binding_unavailable`、`pool_disabled`。
- 账号被跳过时，诊断页或调试日志能解释是硬阻断、模型不匹配、冷却、凭据、access、quota、subscription paid detail 还是 runtime binding。
- v0.1.8 不纳入 Activity 图表的完整 retry path 展示；Activity 可以继续展示最终路由账号，但日志或诊断载体必须能复核 retry 选择过程。

## 测试与验收

### 单元与集成测试

- router 测试：`health_first` 保持健康优先分层意图。
- router 测试：`health_first` 下明确不支持请求模型的账号不能被模型兜底选中。
- router 测试：`ordered` 下轻降级账号排第一时，先选择该账号。
- router 测试：`ordered` 下硬阻断账号排第一时跳过，选择后续可接账号。
- router 测试：`ordered` 下明确不支持请求模型的账号排第一时跳过。
- router 测试：`ordered` 下没有明确模型匹配账号时，允许模型列表为空账号兜底。
- gateway 测试：retry 在 `ordered` 下排除失败账号后继续按列表顺序选下一个账号。
- API 测试：Pool 策略字段默认 `health_first`，可更新为 `ordered`，非法值返回 400。
- cache 测试：策略更新后路由 cache 生效，不需要重启进程。
- 前端测试：Pool 详情页“自检 Pool”右侧存在策略切换按钮，点击后文案和颜色切换。

### 容器测试要求

实现本功能后必须使用新启动的测试容器完成验收，不能复用正在运行的旧版本容器。

容器验收至少覆盖：

- 迁移旧数据库后 Pool 默认策略为 `health_first`。
- 通过 UI 切换为 `ordered` 后，普通请求先命中排在前面的轻降级账号。
- 切回 `health_first` 后，同一组账号优先命中正常账号。
- 硬阻断账号排第一时，两种策略都跳过。
- 验收后删除测试容器和临时数据目录或 volume。

仅修改本规格文档时不执行容器测试；实现代码进入 v0.1.8 后必须执行。

## 已决策边界

- 不新增全局路由策略；策略属于 Pool。
- 不新增权重随机、轮询、成本优先或额度消耗优先。
- 不允许 `ordered` 绕过硬阻断。
- 不允许强制账号请求通过 retry 悄悄切换到其他账号。
- 不新增大段 UI 说明；策略切换入口保持在 Pool 详情页现有操作区。
