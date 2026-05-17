# 02. Codex quota error 文案分层

状态：规划中。本文定义 v0.1.8 必须修正的 Codex CPA quota 错误文案归类问题：quota 辅助接口失败不能被误展示为“模型请求被限流”。

来源：2026-05-17 对正在运行的 `lune-0.1.7` 容器 `lune-0.1.7` 进行只读审计。用户反馈：当前有两个账号的额度请求都显示“模型请求被限流”，但从用户语义看这更像“模型额度未知”或额度查询失败，而不是模型请求真的被限流。

## 问题

v0.1.7 修复了 Codex CPA 普通模型请求 `429` 只进入 generic serving cooldown 的问题。该修复引入了一个新的前端归类缺口：UI 把 `cpa_quota_status='error'` 直接等同于“模型请求被限流”。

当前字段语义实际更宽：

- `cpa_quota_status='blocked'`：quota 辅助接口或真实请求证据明确表示阻断。
- `cpa_quota_status='error'`：quota 相关状态刷新或证据采集失败，也可能是裸 `HTTP 429 from model request` 这类较弱限流证据。
- `cpa_quota_last_error`：唯一能区分错误来源的安全摘要。

因此，`error` 不是“模型请求被限流”的充分条件。只有 `cpa_quota_last_error` 明确以 `HTTP 429 from model request` 开头时，才可以把它解释为真实模型请求层面的限流证据。`HTTP 401`、`HTTP 403`、`request failed` 或其他 quota 辅助接口错误，应展示为“额度查询失败”或更具体的“额度接口鉴权失败”。

## 运行态审计

本次审计只读取容器状态、HTTP API、日志和代码，没有修改容器、数据库、auth file、配置或仓库实现代码。

### 1. 容器与接口状态

目标容器：

```text
name: lune-0.1.7
image: noreply1018/lune:0.1.7
ports: 0.0.0.0:22222->7788/tcp
volume: lune-data:/app/data
```

`GET http://127.0.0.1:22222/admin/api/pools/1` 中，账号 `1` 和账号 `4` 的关键字段反复稳定为：

```text
cpa_credential_status: ok
cpa_subscription_status: active
cpa_quota_status: error
cpa_quota_last_error: HTTP 401
serving_status: healthy
last_probe_status: healthy
```

账号 `1` 仍保留较早的 `codex_quota_json` 成功快照：

```text
rate_limit.allowed: true
rate_limit.limit_reached: false
primary_window.used_percent: 1
```

账号 `4` 是导入后新账号，当前没有成功 quota snapshot，但同样是 `serving_status='healthy'` 和 `last_probe_status='healthy'`。

### 2. 日志证据

容器日志中反复出现的是 quota 辅助接口刷新失败：

```text
POST "/v0/management/api-call" 200
fetch codex quota account_id=1 err="HTTP 401"
fetch codex quota account_id=4 err="HTTP 401"
```

同一时间段真实模型请求与自检请求返回成功：

```text
POST "/api/provider/codex/v1/chat/completions" 200
request completed path=/v1/chat/completions status=200
POST "/admin/api/accounts/1/probe-result" 200
POST "/admin/api/accounts/4/probe-result" 200
```

审计期间没有发现账号 `1` 或账号 `4` 当前普通模型请求返回 `HTTP 429` 的日志证据。

## 根因

根因不在后端运行态写错了 `HTTP 429`，而在前端把宽泛的 quota `error` 状态误映射成模型请求限流。

已确认的前端映射点：

- `web/src/lib/lune.ts` 的 route summary 逻辑中，只要 `cpa_quota_status === "error"` 且存在 `cpa_quota_last_error`，就返回“模型请求被限流”，并写死说明“最近真实模型请求返回了 HTTP 429”。
- `web/src/lib/lune.ts` 的 `getCpaQuotaErrorMeta` 中，只要 `status === "error"`，就返回 label “模型请求被限流”。
- `web/src/components/AccountDetailSheet.tsx` 的 `quotaStatusLabel("error")` 也返回“模型请求被限流”。

这些映射在 v0.1.7 的 `HTTP 429 from model request` 场景中是合理的，但对 `HTTP 401` quota 辅助接口失败是错误解释。

## 改进策略

v0.1.8 必须把 quota `error` 按 `cpa_quota_last_error` 分层解释，不能只看状态枚举。

采用以下最小分类：

| 条件 | 展示标签 | 语义 | 路由影响 |
| --- | --- | --- | --- |
| `cpa_quota_status='blocked'` | 额度已用尽 | quota 明确阻断或明确额度/限流文案 | 硬阻断 |
| `cpa_quota_status='error'` 且 `cpa_quota_last_error` 以 `HTTP 429 from model request` 开头 | 模型请求被限流 | 普通模型请求返回 `429` 的真实请求证据 | 硬阻断或等价阻断证据 |
| `cpa_quota_status='error'` 且错误为 `HTTP 401` 或 `HTTP 403` | 额度接口鉴权失败 | quota 辅助接口无法读取，不代表模型请求不可用 | 轻降级，不单独硬阻断 |
| `cpa_quota_status='error'` 且错误为 `request failed` 或其他非 429 摘要 | 额度查询失败 | quota 辅助接口暂不可用或不可解释 | 轻降级，不单独硬阻断 |
| `cpa_quota_status='unknown'` | 额度未知 | 没有可用 quota 快照 | 轻降级，不单独硬阻断 |
| `cpa_quota_status='ok'` | 额度可用 | 最近 quota snapshot 没有明确阻断 | 不产生 quota penalty |

实现时可以先不新增数据库字段。前端使用现有字段派生 meta：

```text
quota_error_kind =
  model_request_429 | quota_auth_failed | quota_fetch_failed | quota_unknown | quota_blocked | quota_ok
```

实现可以选择在前端派生该 meta，也可以选择由后端返回安全派生字段；无论采用哪种方式，v0.1.8 的最低闭环都是 UI、诊断页和路由摘要不再把所有 `error` 都叫作“模型请求被限流”。

为了避免把不同来源的 `401/403` 混成同一个根因，派生 meta 必须保留安全 `source/reason`。`source` 表示链路来源，`reason` 表示错误类别：

| `source` | `reason` | 示例 | UI 摘要 | Diagnostics 归因 | 路由影响 |
| --- | --- | --- | --- | --- |
| `model_request` | `model_request_429` | 普通 `/chat/completions` 返回 `HTTP 429 from model request` | 模型请求被限流 | Serving / Quota evidence | 硬阻断 |
| `wham_usage` | `quota_fetch_auth_failed` | `wham/usage` 返回 `HTTP 401/403` | 额度查询失败 / 额度接口鉴权失败 | Quota warn | 轻降级 |
| `wham_usage` | `quota_fetch_failed` | `wham/usage` 返回 `request failed` / 502 / timeout | 额度查询失败 | Quota warn | 轻降级 |
| `cpa_management` | `runtime_api_call_failed` | CPA management api-call 无法代理 quota 请求 | 额度查询失败 | Runtime Binding 或 Credential 细节中说明管理调用失败 | 不单独把 access 写成 ineligible |
| `store` | `quota_error_legacy_unknown` | 旧数据只有宽泛摘要 | 额度查询失败 | Quota warn，并展示原始安全摘要 | 轻降级，除非摘要是模型请求 429 |

如果 v0.1.8 暂时不新增数据库字段，后端或前端也必须从安全摘要中派生等价 `source/reason`，并在测试中锁定 `HTTP 401/403` 不会被写入 Access ineligible、Credential needs_login 或模型限流。`quota_fetch` 和 `runtime_api_call` 不作为 `source` 枚举值；它们只能作为 `reason` 的语义类别或展示文案。

## UI 表现

账号卡片、Route 摘要和 Diagnostics 的 Quota 维度必须统一文案。

当前运行态案例的理想展示：

```text
额度查询失败
Quota 辅助接口返回 HTTP 401；最近模型自检仍为 healthy，不能据此判断模型请求被限流。
```

如果是 `HTTP 429 from model request`，继续展示：

```text
模型请求被限流
最近普通模型请求返回 HTTP 429；额度快照和真实请求证据需要分开判断。
```

如果是 `blocked` 或明确 quota / rate-limit 文案，展示：

```text
额度已用尽
额度接口或真实请求明确拒绝继续使用，普通路由会跳过。
```

Diagnostics 的 Quota 区域至少展示：

- 当前展示标签。
- `cpa_quota_status` 原始值。
- `cpa_quota_last_error` 安全摘要。
- 最近 quota 检查时间。
- 最近 quota 尝试时间、最近成功时间和最近错误摘要；字段可以复用现有存储或新增 `quota_last_attempt_at`、`quota_last_success_at`、`quota_last_error` 等等价字段。
- 如果存在 `codex_quota_json`，继续展示 quota snapshot；snapshot 不应覆盖更晚的真实模型请求 `429` evidence。

当存在旧成功快照和较新的 fetch error 时，UI 必须同时表达：

```text
最新额度查询失败
上次成功快照来自 <time>
```

不得只展示旧 snapshot 而隐藏最新失败，也不得只展示失败而让用户无法判断是否仍有历史 quota 数据可参考。

## 路由语义

本问题主要是 UI 和诊断文案修正，不要求 v0.1.8 改变 v0.1.7 已定的硬阻断规则。

必须保持：

- `cpa_quota_status='blocked'` 继续硬阻断。
- Codex CPA `cpa_quota_status='error'` 且 `cpa_quota_last_error` 以 `HTTP 429 from model request` 开头时，继续按 v0.1.7 的模型请求限流证据处理。

必须避免：

- `HTTP 401` quota 查询失败被当成模型请求 `429`。
- `request failed` 被当成模型请求 `429`。
- `unknown` 被当成模型请求 `429`。

对 Pool 路由策略文档中的轻降级 / 硬阻断边界，本文进一步明确：

- `HTTP 429 from model request` 属于硬阻断或等价阻断证据。
- `HTTP 401` / `HTTP 403` / `request failed` quota fetch error 属于轻降级，不能在 `ordered` 策略下被自动视为不可接普通流量。

## 测试与验收

### 单元与组件测试

- 前端测试：`cpa_quota_status='error'` + `cpa_quota_last_error='HTTP 429 from model request'` 显示“模型请求被限流”。
- 前端测试：`cpa_quota_status='error'` + `cpa_quota_last_error='HTTP 401'` 显示“额度接口鉴权失败”或“额度查询失败”，不能出现“模型请求被限流”。
- 前端测试：`cpa_quota_status='error'` + `cpa_quota_last_error='request failed'` 显示“额度查询失败”。
- 前端测试：`cpa_quota_status='unknown'` 显示“额度未知”。
- 前端测试：卡片 chip、Route 摘要和 Diagnostics Quota 维度对同一账号给出一致文案。
- 前端测试：旧 quota snapshot + 较新 `HTTP 401` fetch error 时，Diagnostics 同时展示“最新查询失败”和“上次成功快照时间”。
- 前端 / 后端派生测试：quota fetch `HTTP 401/403` 派生为 `source=wham_usage` + `reason=quota_fetch_auth_failed` 或等价 meta，不能派生为 `model_request_429`、Access ineligible 或 Credential needs_login。
- Diagnostics 测试：当错误为 CPA management api-call 失败时，派生为 `source=cpa_management` + `reason=runtime_api_call_failed` 或等价 meta；Quota 区域展示额度查询失败，同时 Runtime Binding / Credential 细节能看到安全来源，不把该错误误称为真实模型请求限流。

### API / 后端测试

如果实现选择在后端返回派生字段，需要补充 API 测试：

- `HTTP 429 from model request` 派生为 `model_request_429`。
- wham/usage `HTTP 401` / `HTTP 403` 派生为 `source=wham_usage` + `reason=quota_fetch_auth_failed`。
- wham/usage 其他 error 派生为 `source=wham_usage` + `reason=quota_fetch_failed`。
- CPA management api-call 失败派生为 `source=cpa_management` + `reason=runtime_api_call_failed`。
- `blocked` 派生为 `quota_blocked`。
- `unknown` 派生为 `quota_unknown`。
- quota 刷新失败会更新最近尝试时间和安全错误摘要，不覆盖旧成功快照的成功时间。
- quota 刷新成功会更新最近成功时间，并清理或降级展示旧 fetch error。
- quota fetch `401/403/request failed` 不会写入 `cpa_access_status='ineligible'`，也不会写入 credential `needs_login`。

如果实现只在前端派生，后端测试不是必须项，但必须保留现有 gateway `429` evidence 测试，防止 v0.1.7 的真实模型请求 `429` 修复回退。

### 容器验收

实现后必须使用新启动的 v0.1.8 测试容器完成验收，不能复用正在运行的 `lune-0.1.7` 容器。

最小容器验收矩阵：

| 编号 | 场景 | 准备 | 操作 | 期望 |
| --- | --- | --- | --- | --- |
| CT-QE-01 | quota fetch `HTTP 401` | fake CPA management quota 返回 401，模型请求返回 200 | 打开 Pool 详情和账号 Diagnostics | 显示“额度接口鉴权失败”或“额度查询失败”；不显示“模型请求被限流” |
| CT-QE-02 | quota fetch `request failed` | fake CPA management quota 超时或 502 | 刷新账号 | 显示“额度查询失败”；账号不被误判为模型限流 |
| CT-QE-03 | 普通模型请求裸 `429` | fake upstream 对普通 `/chat/completions` 返回 429 | 发普通 Pool 请求 | 继续显示“模型请求被限流”，保留 v0.1.7 行为 |
| CT-QE-04 | 明确 quota 文案 `429` | fake upstream 返回 `quota` / `rate limit` / `limit reached` | 发普通 Pool 请求 | 显示“额度已用尽”或等价额度/限流阻断文案 |
| CT-QE-05 | `unknown` quota | 新账号无 quota snapshot 且无 last error | 打开账号卡片和 Diagnostics | 显示“额度未知”，不显示模型限流 |
| CT-QE-06 | 测试清理 | 完成上述验收 | 删除测试容器和临时 volume / 数据目录 | 不影响正在运行的旧版本容器 |

仅修改本规格文档时不执行容器测试；实现代码进入 v0.1.8 后必须执行。

## 已决策边界

- v0.1.8 不把 `cpa_quota_status='error'` 拆成新的数据库枚举，除非实现阶段发现前端派生无法保持一致。
- `HTTP 401` quota fetch error 不等于“模型额度未知”本身；更准确是“额度查询失败 / 额度接口鉴权失败”，因为还有可能存在旧 quota snapshot。
- `codex_quota_json` 成功快照和更晚的 quota fetch error 要同时可解释：旧 snapshot 可以展示为历史快照，但不能让用户误以为最新额度刷新成功。
- 本问题不改变 v0.1.7 对普通模型请求 `HTTP 429` 的证据沉淀规则。
