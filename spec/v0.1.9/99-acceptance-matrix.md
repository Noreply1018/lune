# 99. v0.1.9 验收矩阵

状态：产品范围已确认。

## Pool 生命周期管理

| ID | 项目 | 验收标准 |
| --- | --- | --- |
| UI-01 | 侧边栏新建入口 | Pools 行右侧有低噪声 `+` 图标按钮，点击只打开新建 Pool 弹窗，不触发 Pools 行的展开/跳转逻辑 |
| UI-02 | 新建 Pool 弹窗 | 弹窗只要求输入 Pool 名称，展示 `OpenAI / Claude / Gemini / Codex` 等推荐命名提示 |
| UI-03 | 创建后导航 | 创建成功后刷新全局数据、展开 Pools 列表，并跳转到 `/admin/pools/{new_id}` |
| UI-04 | Overview 次级入口 | 首次空状态保留“添加账号”主按钮，并提供“只创建 Pool”的次级入口 |
| UI-05 | Add Account 保留 | Add Account 第二步里的“新建 Pool”归属路径仍存在，可继续随账号接入创建 Pool |
| UI-06 | Pool 详情入口位置 | “更多设置”位于现有“自检 Pool”和“健康优先/排序优先”按钮右侧 |
| UI-07 | 重命名 | “更多设置”弹窗可保存 Pool 名称，并保留当前 `priority / enabled / routing_policy` |
| UI-08 | 停用确认 | 停用 Pool 前弹出二次确认；确认后 Pool 变为 disabled，网关请求不再通过该 Pool 路由 |
| UI-09 | 启用操作 | disabled Pool 可在“更多设置”弹窗中直接启用，完成后刷新详情与侧边栏 |
| UI-10 | 删除确认 | 删除 Pool 前弹出二次确认，文案明确说明会删除 Pool token、Pool 内账号和这些账号的其他 Pool 归属 |
| API-01 | 独立创建 Pool | `POST /admin/api/pools` 创建 Pool 后自动生成默认 Pool token |
| API-02 | 删除 Pool 同删账号 | 删除 Pool 后，该 Pool 当前成员账号不存在 |
| API-03 | 共享账号级联 | 若账号也属于其他 Pool，删除来源 Pool 后账号被删除，其他 Pool member 关系也被级联移除 |
| DOC-01 | 范围文档 | `README.md`、`01-pool-lifecycle-management.md`、本验收矩阵共同描述四点范围且没有相互冲突 |
| DOC-02 | Codex quota 文档 | `README.md`、`02-codex-plus-quota-audit.md`、本验收矩阵共同记录 v0.1.8 真实问题、修复建议和测试口径 |
| DOC-03 | 真实探查状态回写文档 | `README.md`、`03-codex-real-probe-state-writeback.md`、本验收矩阵共同记录问题根源、审计结论、改进优化和真实容器测试矩阵 |
| REL-01 | 容器验收 | 涉及前端、运行配置和删除语义的实现完成后，使用新构建测试容器完成 Pool 创建、重命名、启停、删除同删账号矩阵 |
| REL-01B | Codex quota 容器验收 | 涉及 Codex quota 展示或运行态实现后，使用新构建测试容器和 fake CPA 完成 quota 401/403、primary-only、primary+secondary 和模型 429 代表性矩阵 |
| REL-01C | 真实探查状态回写容器验收 | 涉及 gateway、Playground、自检或状态派生实现后，使用新构建测试容器和 fake CPA 完成 quota 401 + 模型 200 / 认证失败 / 429 / 5xx / service key 错误矩阵 |
| REL-02 | 清理 | 测试容器和临时数据目录清理完成，不影响旧版本正在运行的容器 |
| REL-03 | 审计与提交 | 修改完成后 subagent 严格审计通过，并按项目规则提交 Git commit |

## Codex Plus Quota 矩阵

| ID | 场景 | 准备 | 操作 | 期望 |
| --- | --- | --- | --- | --- |
| CQ-01 | 0.1.8 复现夹具 | Plus、subscription active、access eligible、runtime binding confirmed、`codex_quota_json=''`、quota error `HTTP 401` | 渲染 Pool 账号卡片 | 右上角显示 Plus；quota 区显示“额度查询失败”或“周额度待同步”；不显示“无周额度” |
| CQ-02 | 详情页复现夹具 | 同 CQ-01 | 打开账号详情 | Quota section 说明额度辅助接口鉴权失败，模型请求仍可能可用；不显示“当前计划无周额度” |
| CQ-03 | Diagnostics 复现夹具 | 同 CQ-01 | 打开 Diagnostics | Quota 维度为 warning；source/reason 可解释；Access 维度仍为 eligible |
| CQ-04 | Route Summary 复现夹具 | 同 CQ-01 | 打开 Pool 详情 Route Summary | 账号可路由但带 quota warning；不作为硬阻断 |
| CQ-05 | Plus quota fetch 403 | Plus、access eligible、无 snapshot、quota error `HTTP 403` | 渲染卡片、详情和 Diagnostics | 显示“额度接口鉴权失败”或“额度查询失败”；不显示“无周额度”或“模型请求被限流” |
| CQ-06 | Plus pending 无 snapshot | `cpa_plan_type=plus` 且没有 `codex_quota_json`，quota 仍在 pending / unknown | 渲染卡片和详情 | 第二行显示“周额度待同步”或“周额度未知”，不得显示“无周额度” |
| CQ-07 | Plus primary-only | Plus snapshot 只有 primary window | 渲染卡片和详情 | 显示“周额度待同步”或“周额度未知”；不显示“无周额度” |
| CQ-08 | Free primary-only | Free snapshot 只有 primary window | 渲染卡片和详情 | 显示“无周额度”，高度与 Plus 卡片一致 |
| CQ-09 | Go primary-only | Go snapshot 只有 primary window | 渲染卡片和详情 | 可显示“无周额度”，不得使用红色订阅异常 |
| CQ-10 | Unknown primary-only | Unknown plan snapshot 只有 primary window | 渲染卡片和详情 | 显示“周额度未知”，不推断无周额度 |
| CQ-11 | Plus secondary 正常 | Plus snapshot 包含 primary 和 secondary | 渲染卡片和详情 | 显示短周期和周额度两行，百分比和 reset 时间正确 |
| CQ-12 | Plus blocked snapshot | Plus snapshot 包含 secondary 且 `allowed=false` / `limit_reached=true` | 渲染卡片、详情、路由 | 显示额度已用尽；路由硬阻断 |
| CQ-13 | fetch 401 后旧 snapshot 保留 | 先成功写入 Plus snapshot，再模拟 wham 401 | 刷新额度后渲染 | 旧 snapshot 仍可见；主问题为最新额度查询失败；access 不降级 |
| CQ-14 | fetch 403 后旧 snapshot 保留 | 同 CQ-13，但错误为 403 | 刷新额度后渲染 | 旧 snapshot 仍可见；主问题为最新额度查询失败；access 不降级 |
| CQ-15 | empty quota response | wham 返回 200 但 body 为空 | 刷新额度 | quota status 为 error；展示额度查询失败，不显示无周额度 |
| CQ-16 | invalid quota JSON | wham 返回非法 JSON | 刷新额度 | quota status 为 error；展示额度查询失败，不显示无周额度 |
| CQ-17 | CPA management api-call 失败 | `/v0/management/api-call` 返回 502 或 request failed | 刷新额度 | source 为 cpa_management 或等价；展示额度查询失败 |
| CQ-18 | provider 大小写 | API 返回 `cpa_provider='Codex'` | 渲染卡片 | quota parser 正常工作 |
| CQ-19 | 字符串数字 | quota window 数值字段为字符串数字 | parser 单测 | 解析成功；百分比和 reset 正确 |
| CQ-20 | 非法字符串数字 | quota window 数值字段为 `abc` / 空字符串 | parser 单测 | 解析失败并进入“快照不可解析/额度未知”，不显示无周额度 |
| CQ-21 | Quota meta 来源 | `HTTP 401/403` 来自 `wham/usage` | 打开 Diagnostics 或读取派生 meta | 标记为 `source=wham_usage`、`reason=quota_fetch_auth_failed` 或等价语义 |
| CQ-22 | Access 不被 401 降级 | 账号已有 `cpa_access_status=eligible`，quota fetch 返回 401/403/request failed | 刷新额度并读取账号 | access 保持 eligible；路由仍可用但 quota 维度为 warning |
| CQ-23 | 路由不受 fetch 401 硬阻断 | Plus 账号 access eligible、serving healthy、quota fetch 401，Pool 中还有其他账号 | 普通请求 | 该账号仍可参与路由；仅 quota blocked 或模型请求 429 evidence 才硬阻断 |
| CQ-24 | 三处 UI 一致 | 同一 Plus 账号处于 quota fetch 401 | 对比 AccountCard、AccountDetailSheet、Route Summary / Diagnostics | 文案一致，均不出现“无周额度” |
| CQ-25 | 模型请求成功佐证 | 账号 quota fetch 401，但普通模型请求 200 | 查看 Activity 和账号详情 | Activity 显示成功请求；Quota 显示查询失败；二者不互相覆盖 |
| CQ-26 | 模型请求 429 反例 | 普通模型请求返回 429 evidence | 渲染卡片和详情 | 显示模型请求限流，不使用 quota fetch 401 文案 |

## 真实探查状态回写矩阵

| ID | 场景 | 准备 | 操作 | 期望 |
| --- | --- | --- | --- | --- |
| RPW-01 | quota 401 + Playground 200 | fake CPA：`wham/usage` 返回 401，`chat/completions` 返回 200 | Playground 指定该账号测试 | `cpa_credential_status=ok`；`cpa_access_status=eligible`；主问题为额度查询失败；不显示需要重登 |
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

## 关键反例

| ID | 场景 | 期望 |
| --- | --- | --- |
| NEG-01 | 点击侧边栏 `+` | 不跳转到第一个 Pool，不折叠 Pools 列表 |
| NEG-02 | 新建 Pool 名称为空 | 前端阻止提交或后端返回 `label is required`，不得创建空名称 Pool |
| NEG-03 | 删除含共享账号的 Pool | 共享账号本身被删除，其他 Pool 不再保留该账号卡片 |
| NEG-04 | 停用 Pool | 只改变 Pool enabled 状态，不删除 token、不删除账号 |
| NEG-05 | Add Account 新建 Pool | 原接入流程不因独立新建入口而退化 |
| NEG-06 | quota 401 单独存在 | 没有真实模型认证失败证据时，不得只凭 `wham/usage` 401 写 `needs_login` |
| NEG-07 | 纯 diagnostic 失败 | 纯诊断请求失败不得污染普通 usage，不得默认修复或打坏 serving health |
