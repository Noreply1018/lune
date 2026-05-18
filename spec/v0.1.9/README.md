# v0.1.9 规格整理

状态：产品方案已确认。本文沉淀 v0.1.9 的 Pool 生命周期管理范围、Codex Plus 账号 quota 快照缺失时被误展示为“无周额度”的审计与修复要求，以及 Playground / Pool 自检真实模型探查失败未升级为“需要重新登录”的状态回写修复要求。

## 目标

v0.1.8 已经把 Pool 作为外部网关访问边界，但前端入口仍偏“账号接入优先”：用户只能在 Add Account 流程里顺手新建 Pool，不能先创建空 Pool 做规划，也不能在 Pool 详情页集中完成重命名、启停和删除。

v0.1.9 的目标是补齐 Pool 的基础生命周期管理，但不引入新的 Pool 列表页，不搬迁 token 管理，也不改变现有路由策略入口。

本版本还必须修复 v0.1.8 真实容器中发现的 Codex Plus quota 展示误导：账号已确认 `plus`、`subscription active`、`access eligible` 且普通模型请求成功，但 `wham/usage` quota 辅助查询返回 `HTTP 401`，导致没有 quota snapshot。UI 必须把这种状态展示为“额度查询失败”，不能展示为“无周额度”。

本版本同时必须修复另一个 v0.1.8 真实容器问题：后台 quota 辅助接口返回 `HTTP 401` 后，用户在 Playground 或 Pool 自检中真实调用模型也失败，但账号仍停留在“额度接口鉴权失败 / 服务正常”的组合状态，没有升级为 `cpa_credential_status=needs_login`。根因是 `X-Lune-Account-Id` 强制账号请求被后端一律归类为 `diagnostic`，而 gateway 对 diagnostic 请求禁止写入 credential 状态。v0.1.9 必须把“强制账号路由”“纯诊断流量”“用户主动真实探查”三种语义拆开。

## 四点范围

1. **侧边栏 Pools 行新增低噪声新建入口**
   - 在侧边栏 `Pools` 行右侧增加 `+` 图标按钮，和展开箭头并列。
   - 点击 `+` 只打开“新建 Pool”弹窗，不触发 Pools 展开/跳转逻辑。
   - 弹窗只收集 Pool 名称，并提供默认推荐命名提示，例如 `OpenAI / Claude / Gemini / Codex`。
   - 创建成功后刷新全局数据、展开 Pools 列表，并跳转到新建 Pool 的详情页。
   - 创建时不暴露 `priority`、`routing_policy`、token 配置；后端继续自动创建默认 Pool token。

2. **Overview 首次空状态增加“只创建 Pool”的次级入口**
   - 首次空状态保留“添加账号”为主按钮。
   - 增加低层级入口“只创建 Pool”，复用侧边栏的“新建 Pool”弹窗。
   - 该入口服务于先规划 Pool、后接入账号的用户，不替代 Add Account。

3. **Pool 详情页增加“更多设置”弹窗**
   - 在现有 Pool 详情页操作区，将“更多设置”放在“自检 Pool”和“健康优先/排序优先”按钮右侧。
   - 点击后打开弹窗，在弹窗内集中完成：
     - 重命名 Pool。
     - 启用 / 停用 Pool。
     - 删除 Pool。
   - 重命名保存时保留当前 `priority / enabled / routing_policy`。
   - 停用 Pool 前需要二次确认；启用 Pool 可以直接执行。
   - 删除 Pool 前必须二次确认，文案明确说明会删除 Pool、Pool token、Pool 内账号，以及这些账号在其他 Pool 中的归属。

4. **保留 Add Account 内的新建 Pool**
   - Add Account 第二步的“新建 Pool”归属路径继续保留。
   - 它的定位是“接入账号时顺手创建归属 Pool”的快捷路径。
   - 独立新建入口不替代 Add Account 内的快捷创建。

## Codex Plus quota 展示修正

v0.1.9 必须把以下状态拆开：

- 计划身份：Plus / Free / Go / Unknown。
- Codex access：是否已确认可用。
- quota snapshot：是否成功拿到 primary / secondary window。
- quota 辅助查询错误：例如 `HTTP 401/403`、空响应、非法 JSON、CPA management api-call 失败。
- 普通模型请求限流：`HTTP 429 from model request`。

验收口径：

- Plus / paid plan 在 quota snapshot 为空、quota fetch 401/403 或 secondary window 缺失时，不得显示“无周额度”。
- Free / Go 等明确 primary-only 计划仍可显示“无周额度”，但必须由计划和 snapshot 共同支持。
- `HTTP 401/403` quota fetch error 不得显示为“模型请求被限流”，不得把已确认 eligible 的账号降级为 ineligible。
- 旧成功 snapshot + 新 quota fetch 401/403 时，保留旧快照展示，同时主问题说明最新额度查询失败。

## 删除语义

删除 Pool 时，按照本版本确认的产品语义处理：

- 删除该 Pool。
- 删除该 Pool 绑定的 Pool token。
- 删除该 Pool 当前包含的账号。
- 如果这些账号也属于其他 Pool，账号删除会级联移除它们在其他 Pool 中的 member 关系。

这意味着“删除 Pool”是结构性操作，不是简单移除分组；前端必须把影响面写清楚，并要求二次确认。

删除 Pool 后还必须保证运行态和 UI 状态收敛：CPA runtime 不得继续绑定已删除账号；Sidebar、Overview、Settings Pool token 不得残留已删除 Pool 或 token；如果删除的是当前 Pool，前端跳转到剩余 Pool 或 Overview。

## 非范围

以下内容不纳入 v0.1.9：

- 独立 `/admin/pools` 列表或管理页。
- Pool `priority` 的显式编辑 UI。
- Pool token 管理搬迁；token 继续由现有详情页和 Settings 能力承接。
- 路由策略入口搬迁；`health_first / ordered` 继续保留在 Pool 详情页现有操作区。
- 删除 Pool 时提供“只移除 Pool、不删账号”的分支选项。
- 伪造上游未返回的周额度窗口。
- 为了拿 quota 状态而自动发真实模型请求消耗用户额度。
- 把后台健康检查改成定时真实模型调用；真实模型证据只来自普通业务流量、Playground 或 Pool 自检等用户显式触发路径。

## 文档结构

- `01-pool-lifecycle-management.md`：四点产品方案、交互和 API 行为细化。
- `02-codex-plus-quota-audit.md`：v0.1.8 真实容器审计记录、根因分层、修复建议和 quota 展示测试要求。
- `03-codex-real-probe-state-writeback.md`：Playground / Pool 自检真实模型探查的状态回写语义，覆盖问题根源、审计结论、改进方案和真实容器测试矩阵。
- `99-acceptance-matrix.md`：本范围的验收矩阵。
- `100-release-evidence.md`：发布验证时记录实际测试和审计证据。
