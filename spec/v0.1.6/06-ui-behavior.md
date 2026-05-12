# 06. UI 行为与状态展示

## 目标

UI 应准确解释 Lune 当前知道什么、不知道什么，以及为什么某个账号可路由、降权或不可路由。不要把所有 CPA 异常都渲染成红色“需要重新登录”。

## 问题根源

v0.1.5 的 UI 问题主要来自后端状态语义过载：

- quota 401/403 被写成 `needs_login`，前端只能显示“需要重新登录”。
- `cpa_credential_status` 同时承载凭据、quota、subscription、runtime、sync pending 等状态。
- discovery healthy、serving cooldown、quota blocked、subscription expired 之间没有清晰展示优先级。
- Activity 账号统计只展示 Lune routed account，没有说明 CPA actual credential 是否确认。
- Pool 卡片 chip 文案过长时会造成高度不一致。

## 解决的问题

- 用户看到的是具体失败来源，而不是一律重登。
- quota 查询失败时仍能理解模型调用是否可用。
- 已确认 runtime credential 归属和未确认归属的请求统计能区分展示。
- Pool 详情页卡片保持稳定布局。
- 管理员能从 UI 上看出“浏览器登录态”和“Lune/CPA 本地凭据”不是同一个事实。

## 状态文案规则

### credential

| 状态 | 推荐文案 |
| --- | --- |
| `ok` | 凭据正常 |
| `auth_suspect` | 鉴权可疑 |
| `needs_login` | 需要重新登录 |
| `refresh_failed` | 凭据刷新失败 |
| `runtime_pending` | CPA runtime 初始化中 |
| `runtime_error` | CPA runtime 异常 |
| `unknown` | 凭据状态未知 |

### quota

| 状态 | 推荐文案 |
| --- | --- |
| `ok` | 额度可用 |
| `blocked` | 额度耗尽或受限 |
| `error` | 额度查询失败 |
| `pending` | 额度刷新中 |
| `unknown` | 额度未知 |

quota `error` 的详情应展示：

- 最近成功刷新时间。
- 最近尝试失败时间。
- normalized reason，例如 `token_invalidated`。
- 最近模型调用是否成功。

如果最近模型调用成功，文案应类似：

```text
额度查询失败
最近模型调用成功，quota 接口返回 token_invalidated。
```

只有连续模型调用也出现账号上游认证失败时，才切换为“需要重新登录”。

### subscription

| 状态 | 推荐文案 |
| --- | --- |
| `active` | 订阅有效 |
| `expired` | 订阅已过期 |
| `free` | 当前为 Free |
| `pending` | 订阅刷新中 |
| `error` | 订阅元数据获取失败 |
| `unknown` | 订阅状态未知 |

### serving

| 状态 | 推荐文案 |
| --- | --- |
| `healthy` | 生成请求正常 |
| `cooldown` | 生成请求短期熔断 |
| `error` | 生成请求持续失败 |
| `unknown` | 生成状态未知 |

`cooldown` 详情应展示 `cooldown_until` 和最近 normalized error。

## UI 优先级

账号摘要可以按以下顺序展示最重要阻断：

1. runtime credential binding unavailable。
2. `credential_status=needs_login/refresh_failed/runtime_error/runtime_pending/unknown`。
3. `subscription_status=expired/free/pending/error/unknown`。
4. `quota_status=blocked`。
5. `serving_status=cooldown/error`。
6. `quota_status=error` 或 `credential_status=auth_suspect`，展示为降权/未知而不是硬错误。

摘要展示不能覆盖详情。详情页应显示四类状态的独立值和最近更新时间。

## Activity 与账号统计

Activity / Pool 详情页必须区分：

- Lune routed account：router 选择的 account row。
- CPA actual credential：runtime 实际使用的 auth file。

展示规则：

- runtime credential 归属确认时，账号请求量可作为精确统计。
- 未确认时，应显示“实际 CPA 凭据未确认”或等价提示。
- 不能把所有请求量默认归到 Lune 首选账号。
- Flow 图 `Pool -> Account -> Model` 应说明它基于 request log final route；在 runtime binding 完成前，CPA actual credential 仍可能未知。

## Active Pool 卡片高度修复

### 问题根源

Pool 详情页 Active Pool 账号卡片偶发高度不一致：

- Active Pool 使用 CSS grid，但每个卡片外层还有 `SortableMember` wrapper；grid 拉伸的是 wrapper，不是内部 `AccountCard`。
- `AccountCard` 只设置最小高度，没有固定高度或 `h-full`。
- 卡片中部 status chip 使用 `flex-wrap`。
- Codex CPA 卡片同时展示“今日请求数 / 订阅到期 / 凭据状态”三个 chip 时，长文案容易换行。
- 触发组合包括 `今日 625`、`31 天后到期`、`需要重新登录`。

### 已完成修复

- Pool 详情页账号卡片中，CPA 凭据状态 chip 仅在卡片展示层把“需要重新登录”缩短为“请重登”。
- 保持“31 天后到期”文案不变。
- 保持账号详情页、Overview 告警、通知文案和底层 `getCpaCredentialMeta` 语义不变。
- 不引入固定高度、chip 裁切或聚合交互，作为最低风险修复。
- 已重新构建并同步 `internal/site/dist` 嵌入式前端产物。
- 已通过 `npm run build`。
- 已由 subagent 严格审计，确认只影响账号卡片 chip。

当前修复提交：

```text
95025da Shorten CPA login chip label
```

### 后续观察

如果未来仍因请求数字过长、更多 status chip 或窄屏宽度导致换行，再考虑：

- 固定 active card 高度。
- 让 `AccountCard` 继承 grid row 高度。
- chip 区域改为单行省略或聚合。

## 重复账号 UI

当多个启用账号拥有相同 `(cpa_service_id, cpa_account_key)` 时：

- 显示“重复导入”。
- 标注哪些账号行共享同一 credential key。
- 提供安全清理入口。
- 清理前说明是否删除 auth file、是否只禁用账号、是否从 Pool 移除。

## 删除账号 UI

删除 CPA 账号时必须明确：

- 是否删除数据库 account row。
- 是否删除 Pool membership。
- 是否删除磁盘 auth file。
- 是否触发 embedded CPA runtime reload。

如果 auth file 默认保留，UI 必须明说“凭据文件仍保留”；如果同步删除，也必须明说该操作会使后续重登需要重新授权。

## 验收标准

- quota 401/403 但模型调用成功时，UI 显示“额度查询失败”，不显示“需要重新登录”。
- `credential_status=needs_login/refresh_failed/runtime_pending/runtime_error/unknown` 时，UI 摘要展示对应凭据或 runtime 阻断原因。
- quota blocked 时显示额度阻断，并阻止普通路由。
- subscription expired/free/pending/error/unknown 与 quota blocked 分开展示。
- serving cooldown 显示冷却信息，不把它表述成登录失败。
- Activity 账号统计只在 runtime credential 归属确认时显示为精确值。
- Active Pool 卡片在常见 chip 组合下高度稳定。
- 删除 CPA 账号前，用户能明确知道 auth file 会被删除还是保留。
