# 01. Pool 生命周期管理

状态：产品方案已确认。

## 背景

当前前端已经有 `POST /pools` 调用能力，但入口藏在 Add Account 抽屉中。用户想先创建空 Pool、再逐步接入账号时，没有独立入口；用户想管理已有 Pool 的基础属性时，也需要在不同上下文里寻找操作。

v0.1.9 只补齐基础生命周期管理，不扩大为完整 Pool 管理后台。

## 1. 侧边栏新建 Pool

侧边栏 `Pools` 行右侧增加一个小型 `+` 图标按钮。按钮视觉层级低于底部 `Add Account` 主入口，语义是“管理动作”，不是首要接入路径。

交互要求：

- 点击 `+` 打开“新建 Pool”弹窗。
- 点击 `+` 不触发 Pools 行本身的展开/收起或跳转逻辑。
- 侧边栏折叠到 icon 模式时可以隐藏该 `+`，避免窄栏拥挤。
- 创建成功后刷新 pools、展开列表并跳转新 Pool 详情页。

弹窗内容：

- 标题：`新建 Pool`
- 输入：`Pool 名称`
- 推荐提示：`OpenAI / Claude / Gemini / Codex`
- 操作：`取消`、`创建`

不在弹窗中配置：

- `priority`
- `routing_policy`
- Pool token

## 2. Overview 空状态入口

Overview 首次空状态继续以“添加账号”为主按钮，因为多数首次用户需要完成账号接入。

同时增加次级入口“只创建 Pool”，用于先规划 Pool 的用户。该入口复用同一个“新建 Pool”弹窗，避免产生两个创建流程。

## 3. Pool 详情“更多设置”

Pool 详情页已有 Pool 级操作区：`自检 Pool` 和 `健康优先 / 排序优先`。新增按钮放在这两个按钮右侧：

```text
[自检 Pool] [健康优先/排序优先] [更多设置]
```

点击“更多设置”打开弹窗，弹窗内分三块：

1. 基础信息
   - 编辑 Pool 名称。
   - 保存时调用 Pool 更新接口，并保留当前 `priority / enabled / routing_policy`。

2. 运行状态
   - 展示当前 `已启用 / 已停用`。
   - 当前启用时显示“停用 Pool”，停用前二次确认。
   - 当前停用时显示“启用 Pool”，可以直接执行。
   - 停用说明：该 Pool 的网关请求会停止路由，token 保留，可恢复。

3. 危险操作
   - 显示“删除 Pool”。
   - 删除前二次确认。
   - 确认文案必须说明：删除 Pool 会删除 Pool token、Pool 内账号，以及这些账号在其他 Pool 中的归属。

## 4. 保留 Add Account 新建 Pool

Add Account 第二步中的“新建 Pool”继续存在。它解决的是“接入账号时选择或创建归属 Pool”的路径，不与独立创建入口冲突。

保留该入口可以避免用户在接入账号时被迫先退出流程去创建 Pool。

## 删除语义

删除 Pool 的语义按本版本收敛为“删除 Pool 及其账号”：

- 删除 Pool row。
- 删除该 Pool 的 Pool token。
- 删除该 Pool 当前包含的账号。
- 如果账号也属于其他 Pool，删除账号时通过外键级联移除其他 Pool membership。

该语义更接近“删除这一组接入资源”，不是“仅移除分组”。因此删除入口必须放在“危险操作”区域，且必须二次确认。

删除边界：

- 删除包含 CPA 账号的 Pool 后，必须触发 CPA runtime reload 或明确执行等价清理流程，避免运行时继续持有已删除账号的旧 auth binding。
- 删除最后一个 Pool 后，Overview 必须回到首次空状态，Sidebar 不显示已删除 Pool，Settings / Pool Credentials 不再显示已删除 Pool token。
- 删除当前正在查看的 Pool 后，前端必须跳转到可用的相邻 Pool；如果没有剩余 Pool，则跳转到 Overview。
- 删除 Pool 不要求删除磁盘上的历史 auth 文件；如果保留文件，必须保证它不会再通过 runtime auth index 绑定到已删除账号。

## API 对应关系

| 操作 | API |
| --- | --- |
| 创建 Pool | `POST /admin/api/pools` |
| 重命名 Pool | `PUT /admin/api/pools/{id}` |
| 启用 Pool | `POST /admin/api/pools/{id}/enable` |
| 停用 Pool | `POST /admin/api/pools/{id}/disable` |
| 删除 Pool | `DELETE /admin/api/pools/{id}` |

## 验收重点

- 侧边栏和 Overview 复用同一个创建弹窗。
- Add Account 内的新建 Pool 路径没有退化。
- “更多设置”位置紧跟 Pool 级操作按钮，而不是挤入标题区。
- 删除 Pool 的 UI 文案与后端实际删除语义一致。
- 删除共享账号场景必须覆盖：账号如果也属于其他 Pool，删除来源 Pool 后账号本身消失，其他 Pool 不再展示该账号。
- 删除含 CPA 账号场景必须覆盖 runtime reload / auth binding 清理语义。
- 删除最后一个 Pool 场景必须覆盖 Overview、Sidebar、Settings Pool token 一致性。
