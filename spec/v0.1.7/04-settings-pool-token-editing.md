# 04. Settings 页面编辑 Pool Token

状态：release-ready。实现和验收已经纳入 v0.1.7 发布阻塞矩阵；最终 `gpt-5.5` subagent 严格审计已通过。

来源：来自 Settings 页面里 Pool token 管理能力不完整的问题。当前 Settings 只能查看、复制、重命名和重新生成 token，不能在同一入口里直接把已有 pool token 替换成用户提供的新值，导致凭据变更仍然要借助更绕的流程。

## 问题

- Pool token 是网关路由的核心凭据，但 Settings 页面当前缺少“替换现有 token”的直接入口。
- 现有操作里有 `Reveal` 和 `Regenerate`，但没有覆盖“用户已经拿到新的合法 token，希望直接写回当前 Pool”的场景。
- 如果把 token 当普通文本字段处理，容易误导用户把空值理解为清空，或者在页面上看到完整明文。
- Settings 是集中管理凭据的地方，Pool token 的编辑语义必须和 token 展示、复制、重新生成严格区分，避免用户误操作。

## 改进策略

- 在 Settings 的 `Pool Credentials` 区块里，为每个 Pool token 增加显式的编辑入口。
- 编辑动作采用受控替换语义，而不是“自由明文绑定”：
  - 默认仍只显示 masked token。
  - 编辑框不回填完整 token。
  - `Token` 输入框只作为“新 token”输入位。
  - 空值表示保留旧 token，不代表清空。
  - 填入新值才会替换当前 token。
- 保留 `Copy`、`Reveal`、`Regenerate`、`Edit name`，但把“编辑 token”作为独立动作，避免和重命名混淆。
- `Edit token` 与 `Regenerate` 必须使用独立弹窗和独立提交状态，不能共用同一套 open state 或确认文案。
- 保存后刷新该 token 行的 masked 值和更新时间；该行当前的 reveal 展开状态不因保存自动切换，保证页面反馈一致。
- v0.1.7 不提供“清空 token”动作；如未来确有需求，必须单独设计高风险确认流程，不能复用编辑弹窗里的空输入。

## UI 表现

- `Pool Credentials` 区块内每行新增一个 `Edit token` 或同义的显式操作入口。
- 编辑态使用弹窗或抽屉，不直接在表格里展开完整 token。
- 编辑态默认只展示当前 token 状态摘要，不展示完整明文。
- 输入框旁应有辅助文案，明确说明空值会保留现有 token。
- 成功后应更新 token 行的 masked 显示和更新时间；如果 token 当前处于 reveal 状态，可保持用户已展开的上下文，但不必回填明文输入框。
- 编辑弹窗不出现清空按钮，也不把空输入解释为清空。

## 日志与诊断

- 编辑 token 的过程不应把完整 token 写入前端日志、浏览器控制台或错误提示。
- 后端应将 token 替换建模为 `PUT /admin/api/tokens/{id}` 的可选字段 `token`：
  - `token` 缺省表示保留旧值。
  - `token` 只在用户提交非空新值时发送。
  - `token` 为空字符串时应视为无效输入并返回 400，避免空值、缺省和显式替换三种语义混淆。
- 后端必须把 token 替换看作受控写入，只接受安全的明文提交，不回传完整旧值。
- 如果保存失败，错误信息只允许返回安全摘要，不暴露原值或数据库细节。
- 为空提交时应采用明确语义：保留旧 token，而不是清空。
- v0.1.7 的 API 和 UI 都不支持清空 Pool token；任何清空需求都必须走后续独立设计。

## 测试与验收

- Settings 页面中每个 Pool 可进入 token 编辑流程。
- 编辑态默认不展示完整 token。
- 空 token 保存不会清除已有凭据。
- 填入新 token 后，列表展示会刷新为新的 masked token。
- 保存后该 token 行的更新时间会更新，且当前 reveal 展开状态不会被错误切换。
- token 编辑不会影响同一行的重命名、复制、reveal 和 regenerate 流程。

## 真实容器测试要求

- 使用**新启动的 v0.1.7 容器**验证，不复用旧版本正在运行的容器。
- 使用隔离数据目录或只读复制出来的数据副本，避免测试时改坏真实凭据状态。
- 在容器里进入 Settings 页面，对一个已有 Pool token 执行编辑：
  - 验证默认不展示完整 token。
  - 验证空值保存不会清除旧 token。
  - 验证填入新 token 后会替换旧值并刷新 masked 展示。
  - 验证保存后更新时间更新，且原本展开的 reveal 状态不会被错误切换。
- 验证过程中不记录完整 token，不把凭据写入截图、日志或导出材料。
- 验证完成后删除测试容器和临时数据目录。

## 已决策记录

- 已确认 Settings 的 Pool Credentials 区块是承载 pool token 编辑的合适位置。
- 已确认 token 编辑不应复用“重命名”交互，也不应默认回填完整明文。
- 已决策：每个 Pool token 行应提供独立 `Edit token` 入口。
- 已决策：`Edit name`、`Edit token`、`Regenerate` 拆成独立弹窗 / 确认流程和独立提交状态。
- 已决策：前端空输入保留旧 token；只有非空新 token 才向后端发送 `token` 字段。
- 已决策：后端 `PUT /admin/api/tokens/{id}` 语义为 `token` 缺省保留旧值，显式空字符串返回 400，非空值替换旧 token。
- 已决策：更新响应只返回 masked token，不回传完整 token；前端替换成功后也不缓存新明文。
- 已决策：v0.1.7 不提供清空 Pool token；后续如需支持，必须另设高风险确认流程。

## 后续非阻塞项

- 替换成功后自动收起 reveal 状态已移入 `spec/draft/10-v0.1.7-deferred-followups.md`，不进入 v0.1.7 阻塞范围。
