# 03. Codex 真实探查状态回写

状态：产品方案已确认。

## 背景

v0.1.8 已经把 Codex CPA 账号拆成 Credential、Runtime Binding、Access、Quota、Serving、Models 等维度。这个方向是正确的：`wham/usage` 额度辅助接口 `401/403` 不能单独把账号判成“需要重新登录”，否则会重现 v0.1.6 以前的误判。

但 2026-05-17 对真实 `noreply1018/lune:0.1.8` 容器审计时发现另一类误判：多个 Codex CPA 账号显示“额度接口鉴权失败”，用户在 Playground 或 Pool 自检中真实测试模型也失败，账号详情 Diagnostics 的 Serving 维度仍显示“服务正常”，账号主问题没有升级为“需要重新登录”。

这说明 v0.1.8 只修正了“quota 401 不能单独判死”的前半句，却没有可靠实现“quota 失败后，真实模型认证失败必须判死”的后半句。

## 真实容器审计记录

审计时间：2026-05-17。

审计对象：

- 容器：`lune-0.1.8`
- 镜像：`noreply1018/lune:0.1.8`
- 端口：`127.0.0.1:22222 -> 7788`
- 数据卷：先后审计过历史 `lune-data` 和新建 `lune-data-018`
- 内置 CPA：`CLIProxyAPI v7.0.2-lune.1`

只读审计结论：

- 后台健康检查每分钟执行一次，但只做轻量探查：CPA service health、`/api/provider/codex/v1/models`、`wham/usage` quota、subscription / auth metadata。
- 后台健康检查不会定时发真实 `chat/completions`，这是正确边界，避免自动消耗额度或触发限流。
- 真实模型探查只来自普通业务流量、Playground、Pool 自检，或管理员显式 diagnostic request。
- Playground 和 Pool 自检都会通过 gateway 调用 `/v1/chat/completions`，并带 `X-Lune-Account-Id` 强制指定账号。
- v0.1.8 gateway 把所有带 `X-Lune-Account-Id` 的请求自动标记为 `diagnostic`。
- gateway 在检测到 CPA 上游账号认证失败时，只有 `!diagnostic` 才写入 `cpa_credential_status=needs_login`。
- 因此 Playground / Pool 自检的真实认证失败被 diagnostic 保护挡住，只会写 `last_probe_status=error`，不会升级 Credential 维度。

运行态表现：

- `cpa_quota_status=error`，`cpa_quota_last_error=HTTP 401`。
- Playground / Pool 自检返回 `401`、`429`、`503` 或 `auth_unavailable: no auth available` 等错误。
- `cpa_credential_status` 仍为 `ok`。
- `serving_status` 仍为 `healthy`，因为 `/models` 探查正常，且 diagnostic 请求不更新普通 serving health。
- UI 主问题停留在“额度接口鉴权失败”或“服务正常”，没有展示“需要重新登录”。

## 问题根源

根源不是后台没有每分钟真实跑模型。后台不应自动真实跑模型。

根源是 v0.1.8 把三个概念混成了一个布尔值：

| 概念 | 正确语义 | v0.1.8 实际问题 |
| --- | --- | --- |
| 强制账号路由 | 只尝试指定账号，不按 Pool 策略切到其他账号 | 被自动等同于 diagnostic |
| 纯诊断流量 | 排障用途，不计普通 usage，不修复普通状态 | 语义被扩展到 Playground / 自检 |
| 用户主动真实探查 | 用户显式要求验证账号能否跑模型，结果应成为账号证据 | 失败被 diagnostic 拦截，不写 Credential |

`diagnostic` 在 v0.1.8 中同时承担了以下职责：

- 不计入普通 usage。
- 绕过部分普通路由保护。
- 不更新 serving health。
- 不写 `needs_login`。

这些职责必须拆开。Playground / Pool 自检可以不计入普通 usage，也可以避免污染 serving cooldown，但不能因为“是诊断流量”就丢弃明确的账号凭据失败证据。

## 目标语义

v0.1.9 必须恢复 v0.1.6 已确认的状态语义：

| 组合状态 | 账号状态 | UI 主问题 |
| --- | --- | --- |
| quota fetch `401/403`，真实模型请求 `200` | `cpa_credential_status=ok`，quota 保持 warning | 额度接口鉴权失败 / 额度查询失败 |
| quota fetch `401/403`，真实模型请求明确账号认证失败 | `cpa_credential_status=needs_login` | 需要重新登录 |
| quota fetch `401/403`，真实模型请求 `429` 或 quota 文案限流 | quota 写入模型请求限流或 blocked 证据 | 额度已用尽 / 模型请求被限流 |
| quota fetch `401/403`，普通业务模型请求 `5xx` / timeout / EOF | serving 进入 error / cooldown，Credential 不变 | 服务异常 / 冷却 |
| quota fetch `401/403`，Playground / Pool 自检模型请求普通 `5xx` / timeout / EOF，且响应内容不能明确归因为账号认证缺失 | 只写 probe error，Credential 和 Serving 不变 | 自检失败 / Playground 错误 |
| quota fetch `401/403`，没有真实模型证据 | Credential 不变，quota warning | 额度接口鉴权失败 / 额度查询失败 |

## 改进方案

### 拆分请求语义

gateway 必须把请求语义拆成至少三个独立维度：

| 维度 | 建议来源 | 作用 |
| --- | --- | --- |
| `force_account` | `X-Lune-Account-Id` | 强制选择某个账号，不代表纯诊断 |
| `diagnostic` | `X-Lune-Diagnostic: true` 或 `/admin/api/accounts/{id}/diagnostic-request` | 排障流量，不计普通 usage，可禁止状态写入 |
| `stateful_probe` | `X-Lune-Probe-Mode: stateful` 或等价内部上下文 | 用户主动真实探查，不计普通 usage，但允许写 Credential / Access 证据 |

Playground 和 Pool 自检必须走 `force_account + stateful_probe`。纯诊断入口才走 `diagnostic`。

实现上必须避免继续用单个 `diagnostic` 布尔值承载全部语义。请求日志和 usage 统计至少需要能还原以下事实：

- 是否强制账号路由：`force_account=true/false` 或等价 trace 字段。
- 是否纯诊断：`diagnostic=true/false`，纯诊断默认禁止写 Credential / Access / Serving / quota evidence。
- 是否用户主动真实探查：`stateful_probe=true/false` 或 `traffic_kind=stateful_probe`。
- 普通 usage 统计必须排除纯诊断和 `stateful_probe`；但 `stateful_probe` 的 request log 仍应可被审计，且允许写 Credential / Access / quota evidence。

### 状态写入规则

真实模型请求完成后，gateway 必须按来源和错误归属写状态：

- 普通业务请求：
  - 明确 CPA 账号上游认证失败：写 `cpa_credential_status=needs_login`。
  - 成功：可写 `cpa_access_status=eligible`，不清除 quota fetch 401/403。
  - `429`：写 quota / rate-limit evidence。
  - `5xx` / timeout / EOF：写 serving failure / cooldown。
- `stateful_probe`：
  - 明确 CPA 账号上游认证失败：写 `cpa_credential_status=needs_login`。
  - 成功：写 `cpa_access_status=eligible`，可写 probe success，不计普通 usage。
  - `429`：必须写 quota / rate-limit evidence，并按现有模型请求 429 规则阻断普通路由；同时可写 probe error 摘要。
  - 普通 `5xx` / timeout / EOF：只写 probe error，不写 serving failure / cooldown，不写 `needs_login`。
  - `503`、`401` 或其他状态码只要响应内容能明确归因为账号 auth 缺失，例如 `auth_unavailable: no auth available`，必须优先归类为账号 credential 硬失败，写 `needs_login`。
- 纯 `diagnostic`：
  - 不计普通 usage。
  - 不修复普通 serving health。
  - 默认不写 Credential；如果实现允许写入，必须只在明确用户确认的“有状态诊断”入口写入。

### 错误分类

CPA 账号上游认证失败分类必须覆盖 v0.1.8 真实日志中出现的语义：

- `refresh token`
- `invalid_grant`
- `token_invalidated`
- `upstream credential`
- `upstream authentication`
- `chatgpt`
- `codex credential`
- `auth_unavailable`
- `no auth available`
- `credential unavailable`
- CPA 明确返回的账号 auth 缺失 / 失效文案

同时必须继续排除 Lune 到 CPA 的 service key / management key 错误。service key 错误应写 CPA service / runtime 维度，不得把某个用户账号误标为 `needs_login`。

错误分类优先级：

1. 先识别 service key / management key / Lune 到 CPA 的服务级认证错误；这类错误不得写账号 `needs_login`。
2. 再识别账号级认证缺失或失效文案；即使 HTTP 状态码是 `503`，只要 body 明确包含账号 auth 不可用证据，也必须按账号 credential 硬失败处理。
3. 剩余 `5xx` / timeout / EOF 才按服务异常或 probe-only 错误处理。

### UI 展示优先级

账号卡片、Route Summary 和 Diagnostics 的主问题优先级必须统一：

1. Runtime Binding 不可确认。
2. Credential `needs_login / refresh_failed / runtime_error / runtime_pending / unknown`。
3. Quota `blocked` 或模型请求 429 evidence。
4. Serving `cooldown / error`。
5. Quota fetch warning，例如 `HTTP 401/403`。
6. Probe-only 错误摘要。

因此，一旦真实模型探查确认账号凭据失败，UI 必须展示“需要重新登录”，不能继续把 quota fetch `HTTP 401` 作为主问题。

## 后台健康检查边界

v0.1.9 不允许把每分钟健康检查改成自动真实模型调用。

后台健康检查继续只做：

- CPA service health。
- `/models` 模型发现。
- auth file / runtime metadata 同步。
- `wham/usage` quota refresh。
- subscription refresh。
- request log pruning。

真实模型证据只来自：

- 普通业务流量。
- Playground。
- Pool 自检。
- 管理员显式有状态探查入口。

## 真实容器测试矩阵

v0.1.9 涉及本规格的实现完成后，必须使用新构建测试容器和隔离数据卷完成以下矩阵。测试不得影响旧版本正在运行的容器；测试容器和临时数据必须在验收后删除。

| ID | 场景 | 准备 | 操作 | 期望 |
| --- | --- | --- | --- | --- |
| RPW-01 | quota 401 + Playground 200 | fake CPA：`wham/usage` 返回 401，`chat/completions` 返回 200 | Playground 指定该账号测试 | 账号保持 `cpa_credential_status=ok`；`cpa_access_status=eligible`；主问题为额度查询失败；不显示需要重登 |
| RPW-02 | quota 401 + Pool 自检 200 | 同 RPW-01 | Pool 自检 | 自检成功；quota warning 保留；不计普通 usage |
| RPW-03 | quota 401 + Playground 上游认证失败 | fake CPA：`wham/usage` 返回 401，模型请求返回 401 + `refresh token invalid` | Playground 指定账号测试 | 写 `cpa_credential_status=needs_login`；UI 主问题为需要重新登录 |
| RPW-04 | quota 401 + Pool 自检上游认证失败 | 同 RPW-03 | Pool 自检 | 写 `needs_login`；`last_probe_status=error`；普通路由跳过该账号 |
| RPW-05 | quota 401 + `auth_unavailable` | 模型请求返回 503 或 401，body 含 `auth_unavailable: no auth available` | Playground 或自检 | 归类为账号认证不可用，写 `needs_login` 或等价 credential 硬失败 |
| RPW-06 | quota 401 + service key 错误 | CPA 返回 401，body 含 `invalid api key` / `management key` | 普通请求、Playground、自检分别验证 | 不写账号 `needs_login`；写 CPA service / runtime 错误或保留安全错误 |
| RPW-07 | quota 401 + 模型 429 | 模型请求返回 429，body 含 quota / rate limit / limit reached | 普通请求和自检 | 写 quota / rate-limit evidence；UI 显示额度已用尽或模型请求限流，不显示需要重登 |
| RPW-08 | quota 401 + 普通请求模型 5xx | 模型请求返回 500 / 502 | 普通请求 | 写 serving failure / cooldown；Credential 不变 |
| RPW-09 | quota 401 + stateful probe 模型 5xx | 模型请求返回 500 / 502 | Playground 和 Pool 自检分别验证 | 写 probe error；不得写 `needs_login`；不得写 serving failure / cooldown |
| RPW-10 | quota 401 + stateful probe timeout / EOF | fake CPA 模拟 timeout 或 EOF | Playground 和 Pool 自检分别验证 | 写 probe error；不得写 `needs_login`；错误摘要安全截断 |
| RPW-11 | stateful probe 不计普通 usage | Playground / 自检成功与失败各一次 | 查询 Activity / usage summary | 普通 usage 不统计 stateful probe；诊断或 probe 日志可被排除或单独过滤 |
| RPW-12 | 纯 diagnostic 不写状态 | 调用 `/admin/api/accounts/{id}/diagnostic-request` 或显式 `X-Lune-Diagnostic: true`，上游返回认证失败 | 查询账号 | 不写 `needs_login`，除非该入口明确声明为有状态诊断 |
| RPW-13 | 强制账号不等于 diagnostic | 普通客户端带 `X-Lune-Account-Id` 但不带 diagnostic header | 模型请求返回账号认证失败 | 写 `needs_login`；request log 能说明 force account |
| RPW-14 | 状态优先级 | 同一账号同时有 quota 401 和 credential needs_login | 打开卡片、Route Summary、Diagnostics | 主问题统一为需要重新登录，quota warning 作为次级信息 |
| RPW-15 | stateful probe 可审计 | Playground / 自检各发起一次 | 查询 request log 或 route trace | 能区分 `force_account`、`stateful_probe`、纯 `diagnostic`；stateful probe 不计普通 usage 但保留审计记录 |
| RPW-16 | 普通 5xx 与账号 auth 5xx 优先级 | 模型请求分别返回普通 503、503 + `auth_unavailable` | Playground 或自检 | 普通 503 只写 probe error；503 + 账号 auth 缺失写 `needs_login` |

## 自动化测试要求

必须覆盖以下自动化测试：

- gateway 单元测试：`X-Lune-Account-Id` 不再自动等同 `diagnostic`。
- gateway 单元测试：`stateful_probe` 模式下 CPA 上游认证失败写 `needs_login`。
- gateway 单元测试：纯 diagnostic 模式下同样错误不写 `needs_login`。
- gateway 单元测试：service key / management key 错误不写账号 `needs_login`。
- gateway 单元测试：`auth_unavailable` / `no auth available` 被归类为账号 credential 硬失败。
- 前端或集成测试：Playground / Pool 自检请求携带 stateful probe 语义。
- UI 派生测试：Credential 硬失败优先于 quota fetch warning。
- request log / usage 测试：`stateful_probe` 不计入普通 usage，但日志能审计 `force_account` 与 `stateful_probe` 语义。
- request log / usage 测试：纯 `diagnostic` 与 `stateful_probe` 可区分，纯 diagnostic 不写 Credential / Access / Serving / quota evidence。

## 发布证据要求

`100-release-evidence.md` 必须记录：

- 新构建镜像 tag 或 digest。
- 测试容器名、端口、隔离 volume / 数据目录。
- fake CPA 响应矩阵摘要。
- RPW-01 到 RPW-16 的通过证据。
- 普通 usage 是否排除 Playground / 自检的验证摘要。
- 测试容器和临时数据清理结果。

仅修改本文档时不需要执行容器测试；任何涉及 gateway、health checker、前端 Playground / 自检、状态派生或 UI 展示的实现改动，都必须执行上述真实容器矩阵。
