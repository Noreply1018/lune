# 02. Codex Plus 额度展示审计与修复规格

## 背景

v0.1.8 真实容器中出现一个明显误导：`a***@hotmail.com` 这类 `a` 开头 Codex CPA 账号明确是 Plus，模型请求也能成功，但账号卡片和详情中的 quota 区可能显示“无周额度”。这会让用户误以为 Lune 把 Plus 账号识别成 Free，或者 Plus 账号没有 7 天额度窗口。

本问题必须在 v0.1.9 修复。核心目标不是伪造周额度，而是准确表达三类事实：

1. 账号计划身份：Plus / Free / Go / Unknown。
2. Codex access：是否能实际使用 Codex 模型。
3. Quota 快照：额度辅助接口是否拿到了 primary / secondary window。

这三类状态不能再在 UI 中互相覆盖或互相推断。

## 0.1.8 真实容器审计记录

审计时间：2026-05-17。

审计对象：

- 容器：`lune-0.1.8`
- 镜像：`noreply1018/lune:0.1.8`
- 端口：`127.0.0.1:22222 -> 7788`
- 数据卷：`lune-data-018`
- 内置 CPA：`CLIProxyAPI v7.0.2-lune.1`

审计方式：

- 只读查看 `docker ps`、`docker logs`、容器 `/app/data/lune.db` 快照和运行时代码。
- 未修改容器数据、源码、配置或文档。
- 未停止、删除或重启 `lune-0.1.8`。

关键运行日志：

- `codex-a***@hotmail.com-plus.json` 被导入后，runtime auth index 已确认。
- 该账号普通模型请求两次返回 `200`。
- 同一账号额度刷新多次记录 `fetch codex quota account_id=2 err="HTTP 401"`。
- 日志中存在 `resolve cpa runtime: skip older auth file metadata`，但 DB 中 `cpa_openai_id` 与 auth JSON `account_id` 一致，未发现账号绑定错位证据。

DB 快照中的关键字段：

| 字段 | `c***@mail.com` 对照账号 | `a***@hotmail.com` 问题账号 |
| --- | --- | --- |
| `cpa_plan_type` | `plus` | `plus` |
| `cpa_subscription_status` | `active` | `active` |
| `cpa_subscription_expires_at` | `2026-06-08T17:20:09Z` | `2026-06-15T10:09:27Z` |
| `cpa_access_status` | `eligible` | `eligible` |
| `cpa_access_reason` | `subscription_active` | `subscription_active` |
| `codex_quota_json` | 有快照 | 空 |
| `cpa_quota_status` | `blocked` | `error` |
| `cpa_quota_last_error` | `quota blocked by upstream` | `HTTP 401` |
| `cpa_quota_checked_at` | `2026-05-17 14:05:48` | `2026-05-17 14:12:29` |
| `cpa_quota_backoff_until` | 空 | `2026-05-17 14:22:29` |
| `serving_status` | `healthy` | `healthy` |

问题账号的 request log：

| 时间 | account_id | status_code | success | runtime_binding_status | runtime_account_key |
| --- | --- | --- | --- | --- | --- |
| `2026-05-17 14:07:13` | `2` | `200` | `1` | `confirmed` | `codex-a***@hotmail.com-plus` |
| `2026-05-17 14:10:48` | `2` | `200` | `1` | `confirmed` | `codex-a***@hotmail.com-plus` |

对照账号的 quota snapshot 中包含：

- `plan_type: plus`
- `rate_limit.primary_window`
- `rate_limit.secondary_window`
- `rate_limit.allowed=false`
- `rate_limit.limit_reached=true`

问题账号没有 quota snapshot，所以不能从 Lune DB 得出“无周额度”的事实。

## 根因分层

### 真实运行态问题

`a***@hotmail.com` 账号的 Plus 身份、subscription active、access eligible、runtime binding confirmed 和模型请求成功都成立，但 CPA management 代理 `https://chatgpt.com/backend-api/wham/usage` 时返回 `HTTP 401`。

因此真实问题是：模型路径可用，但 quota 辅助查询路径不可用。Lune 必须把这类带明确 quota fetch error 的状态展示为“额度查询失败”，不能展示为“无周额度”；只有没有明确错误、但 paid plan 缺少 secondary window 时，才展示“周额度待同步”。

### UI 表达问题

`web/src/components/CodexQuotaBars.tsx` 当前存在两个误导点：

1. `CodexQuotaBarsCompact` 只要 `quota.secondary` 为空，就显示“无周额度”，未结合 `planType`。
2. `CodexQuotaBarsPendingCompact` 在完全没有 quota snapshot 时也固定显示“无周额度”。

这会导致 Plus 账号在 quota fetch 401、quota snapshot 空、parser 失败、secondary window 暂缺时，都可能被展示成“无周额度”。

### Parser 风险

`web/src/lib/codexQuota.ts` 还存在两个潜在风险：

1. `parseCodexQuota` 用 `account.cpa_provider !== "codex"` 做大小写敏感判断；如果后端返回 `Codex`，会把真实 Codex snapshot 当成不可解析。
2. `parseWindow` 只接受 JS number；如果上游返回字符串数字，secondary window 会被解析为 `null`。

这两项在本次容器问题中不是已证实主因，因为问题账号的 `codex_quota_json` 为空；但它们会制造同类“Plus 显示无周额度”的未来回归。

## 修复要求

### 状态语义

v0.1.9 必须明确以下语义：

- `cpa_plan_type=plus` 或 `cpa_subscription_status=active` 只说明账号是 paid plan，不说明 quota snapshot 一定可查询。
- `cpa_access_status=eligible` 说明账号可用，不说明 quota 辅助接口成功。
- `codex_quota_json` 为空时，UI 不得推断“无周额度”。
- `rate_limit.secondary_window` 缺失时，只有 Free / Go 等明确 primary-only 计划才可展示“无周额度”。
- Plus / Pro / Team / Unknown paid plan 缺少 secondary window 时，应展示“周额度待同步”“额度快照缺少周窗口”或“额度查询失败”，并保留错误来源。
- `HTTP 401/403` quota fetch error 不得降级为 `access ineligible`，也不得显示为“模型请求被限流”。

### quota meta 收敛策略

v0.1.9 必须提供唯一的 quota meta 派生入口，避免各 UI 组件靠原始字段各自猜测。实现可以选择以下两种路径之一，但只能有一个权威来源：

1. **后端派生字段**：API 返回结构化 meta，前端只消费 meta。
2. **前端集中派生函数**：不新增 API 字段，但必须提供一个统一派生函数，AccountCard、AccountDetail、Route Summary、Diagnostics 全部复用该函数，不得在组件内重复判断。

建议的 meta 字段如下：

| 字段 | 建议含义 |
| --- | --- |
| `quota_snapshot_status` | `available / missing / parse_error / stale` |
| `quota_primary_window_status` | `available / missing` |
| `quota_secondary_window_status` | `available / not_applicable / pending / missing / error` |
| `quota_error_source` | `wham_usage / cpa_management / model_request / parser / store` |
| `quota_error_reason` | `quota_fetch_auth_failed / runtime_api_call_failed / quota_fetch_failed / model_request_429 / quota_blocked / snapshot_missing / snapshot_parse_error` |

无论选择哪一路径，验收时必须能从 API 响应或统一派生结果中解释 `source` 与 `reason`，例如 `HTTP 401/403 from wham/usage` 必须归为 `source=wham_usage`、`reason=quota_fetch_auth_failed` 或等价结构化语义。

### 前端建议

建议重构 `CodexQuotaBars` 的输入：

- `CodexQuotaBarsCompact` 增加 `planType`、`quotaStatus`、`quotaError` 或统一 `quotaMeta`。
- `CodexQuotaBarsPendingCompact` 不再固定显示“无周额度”，应根据计划显示：
  - Free / Go primary-only：`无周额度`
  - Plus / Pro / Team：`周额度待同步`
  - Unknown：`周额度未知`
- `CodexQuotaBarsFull` 在没有 secondary window 且计划为 paid 时，详情文案必须解释“额度快照未返回 7 天窗口”，不得说“当前计划无周额度”。
- `parseCodexQuota` 对 `cpa_provider` 做大小写不敏感判断。
- `parseWindow` 支持安全解析字符串数字，但仍拒绝空值、NaN 和非有限数。

### 路由建议

这类问题不应影响普通模型路由：

- Plus + access eligible + serving healthy + quota fetch 401：账号可路由，但 quota 维度为 warning。
- quota blocked 或普通模型请求 `HTTP 429 from model request` evidence：继续硬阻断。
- 旧成功 snapshot + 新 quota fetch 401：保留旧 snapshot 展示，同时主问题提示最新查询失败；路由不因 401 降级为不可用。

## 用户可见文案口径

| 场景 | 卡片短文案 | 详情文案 |
| --- | --- | --- |
| Plus，无 quota snapshot，最近 `HTTP 401` | `额度查询失败` | `额度辅助接口鉴权失败，模型请求仍可能可用；未拿到 7 天额度快照。` |
| Plus，有 primary，无 secondary，无错误 | `周额度待同步` | `额度快照未返回 7 天窗口，不能判断周额度。` |
| Free / Go，只有 primary | `无周额度` | `当前计划未返回 7 天额度窗口。` |
| Unknown plan，只有 primary | `周额度未知` | `账号计划或 quota snapshot 不完整，不能判断是否存在 7 天窗口。` |
| 模型请求 429 | `请求限流` | `普通模型请求返回 429，路由会跳过该账号直到证据过期或状态刷新。` |

这些文案是本版本的唯一口径。AccountCard、AccountDetail、Route Summary、Diagnostics 可以根据空间缩短说明，但同一场景的短文案必须一致，不得在一处显示“额度查询失败”、另一处显示“周额度待同步”。

## 验收要求

v0.1.9 发布前必须覆盖：

- 单元测试：quota meta 派生、parser、route routability。
- 前端组件测试或等价渲染检查：卡片和详情页文案。如果本版本不引入前端测试框架，必须使用 Playwright 或等价 fixture 页面截图 / DOM 断言覆盖这些文案。
- fake CPA 容器矩阵：`wham/usage` 401、403、empty、invalid JSON、primary-only、primary+secondary。
- 真实容器验收：使用隔离测试容器，不影响旧容器；用完删除测试容器和临时数据。

仅修改本文档时不需要容器测试；实现相关运行态或 UI 改动时必须执行容器测试。
