# 99. 验收矩阵

## 目标

本文件只做跨问题最终核对。详细设计以各问题文档为准，避免 UI 文案、连接编辑语义、直连诊断结构和容器运行态审计口径分散后出现冲突版本。

## 本轮验证状态

- 待完成：`go test ./...`
- 待完成：`npm --prefix web run build`
- 待完成：必要时补充容器 smoke test，验证新 UI 与直连账号保存语义的一致性。
- 已完成：只读审计正在运行的 `lune-0.1.6` 容器，并用临时 `noreply1018/lune:0.1.6` 容器复核 embedded CPA 启动路径。审计结论为：`docker inspect` 和 PID1 环境缺少 `LUNE_CPA_PROVIDER_PINNING_SUPPORTED` 不能证明 Lune 运行态未启用 pinning；真实 `lune up` 子进程和 embedded `CLIProxyAPI` 子进程均携带 `LUNE_CPA_PROVIDER_PINNING_SUPPORTED=1`。临时测试容器和测试 volume 已删除。
- 已完成：只读审计正在运行的 `lune-0.1.6` 容器中 Codex CPA 账号 `429` 与“服务冷却中”展示的矛盾。审计结论为：真实模型请求 `429` 只写入 `serving_status='cooldown'`，没有沉淀到 Codex quota 维度；因此用户在 Playground 看到额度/限流耗尽，而 UI 主状态显示冷却。

## 真实容器测试要求

凡是实现本目录里的 UI 调整，都要在**新容器**中做最终验收：

- 容器必须是新启动的 v0.1.7 实例，不能复用正在运行的旧版本容器。
- 需要使用隔离数据目录，或者从现有数据目录复制出测试副本后再挂载。
- 需要保留可复核的启动命令、端口、镜像 tag 或 digest、以及验证摘要。
- 验收完成后必须删除测试容器和临时数据卷。

建议的最小容器验收覆盖：

- `UI-01` 和 `UI-02` 同容器验证：打开直连账号详情抽屉，确认 tab 语言统一、`Connection` 仅在 `Overview`，`Diagnostics` 不再混入 CPA 专属结构。
- `UI-03` 同容器验证：保存时留空 token 不会清掉旧 token，填入新 token 才替换。
- `UI-06` 同容器验证：Settings 页面可编辑 pool token，且替换后 masked 展示立即刷新。
- `UI-04` 和 `UI-05` 同容器验证：直连账号诊断页更适合直连场景，且高级信息收敛后仍能辅助排障。
- `OPS-03` 同容器验证：Codex CPA 真实模型请求 `429` 时，UI/API 能区分 generic serving cooldown 与 quota / rate-limit 证据。

## 需要统一验证的点

| 编号 | 覆盖问题 | 验收方式 | 必须验证 |
| --- | --- | --- | --- |
| UI-01 | 账号详情抽屉 tab 文案 | 页面人工检查或组件截图 | `Overview / Playground / Diagnostics` 全英文一致 |
| UI-02 | 直连账号连接编辑区 | 页面交互测试 | 编辑区位于 `Overview` 顶部，不出现在 `Playground` / `Diagnostics` |
| UI-03 | token 表达语义 | 交互测试 | 默认不展示完整 token；空 token 表示保留旧值，不误导为丢失 |
| UI-04 | 直连账号诊断页 | 页面人工检查或组件测试 | 对 `openai_compat` 账号不显示 `Runtime Binding`、`Subscription`、`Quota` 作为主诊断维度；主诊断区只出现 `Connection`、`Credential`、`Route`、`Serving`、`Models` |
| UI-05 | 高级信息收敛 | 页面人工检查 | `Advanced` 区域仅保留 `Account ID`、`Runtime Base URL`、`Last Error`、`Last Checked At`、`Source Kind` 等直连排障字段，不展示完整 token 或 request body |
| UI-06 | Pool token 编辑 | 页面交互测试 | Settings 页面提供显式 token 替换入口；默认不回填完整 token；空值保留旧 token；保存后 masked 值与更新时间刷新；当前 reveal 展开状态不被错误切换；`Edit token` 与 `Regenerate` 使用独立弹窗和提交状态 |
| OPS-01 | 0.1.6 embedded CPA pinning 运行态审计 | 真实运行容器只读审计 + 临时发布镜像容器复核 | 区分 `docker inspect` 静态环境、PID1 shell 环境和 `lune` / embedded CPA 子进程有效环境；确认 embedded 模式下 Lune effective pinning capability；测试容器用完删除 |
| OPS-02 | CPA 路由阻断原因展示 | API / 诊断页验收 | 对固定 fixture 的 CPA 账号，接口或页面必须分别展示 `provider_pinning_unsupported`、`runtime_auth_binding_unavailable`、`no_healthy_account`、`model_not_on_account`、subscription 非 active、账号 `status=error` 六类不同阻断原因之一，且不能统一折叠成同一个“账号不可用” |
| OPS-03 | Codex CPA `429` 与 quota/cooldown 归类 | 真实运行容器只读审计 + 新容器 mock upstream 验收 | 真实模型请求 `429` 不能只沉淀为 generic serving cooldown；UI/API 必须展示 quota / rate-limit evidence，并与 `wham/usage` 快照分层 |

## Codex 429 真实容器测试矩阵

以下矩阵用于 `05-codex-quota-429-cooldown-state.md` 的最终验收。必须使用新容器、隔离数据目录和 mock upstream / fake CPA，避免消耗真实 Codex 额度。

| 编号 | 场景 | 验收方式 | 必须验证 |
| --- | --- | --- | --- |
| CT-429-01 | Codex CPA 模型请求返回裸 `429` | 新容器 + fake Codex CPA + mock upstream | 普通请求后账号进入短期 cooldown，同时 UI/API 记录 `HTTP 429` quota / rate-limit evidence，不只显示泛化“服务冷却中” |
| CT-429-02 | Codex CPA 模型请求返回明确 quota 文案 `429` | 新容器 + mock upstream body 包含 `quota` / `rate limit` / `limit reached` | `cpa_quota_status` 或新增 quota state 反映 blocked/limited，卡片和详情页主问题显示额度/限流 |
| CT-429-03 | 非 Codex 账号返回 `429` | openai_compat fake upstream | 只触发 serving cooldown，不写 Codex quota 字段，不误报 Codex 额度耗尽 |
| CT-429-04 | Diagnostic / Playground 强制账号返回 `429` | `X-Lune-Account-Id` 强制路由 | request log 保留直测失败证据；是否污染普通 cooldown 与 quota evidence 按最终设计断言，不能隐式破坏普通路由状态 |
| CT-429-05 | `wham/usage` 快照 ok 但模型请求 `429` | quota mock 返回 `allowed=true`、`limit_reached=false`、`used_percent=100`，模型 mock 返回 `429` | UI 同屏展示 quota snapshot 与 real request evidence，不再把快照 ok 解释成真实请求可用 |
| CT-429-06 | 冷却过期后的状态解释 | 等待或模拟 cooldown 到期 | 若 quota evidence 仍有效，主状态不应直接退回“可接流量”；若上游恢复，需要清除或降级旧 evidence |

## 最低验收标准

- tab 语言统一，详情抽屉不再混用中文和英文 tab 名。
- 直连账号可以在详情抽屉里直接改 API 地址和 token。
- token 默认不回填完整明文，但 UI 不会让用户误判为凭据丢失。
- Settings 页面可以显式替换 pool token，且不会把完整 token 暴露到界面或日志里。
- Settings 页面替换 pool token 后，masked 值和更新时间会同步刷新，且不会把当前 reveal 状态误切换掉。
- 直连账号的诊断页与 CPA 账号诊断页结构不同，且更贴近直连场景。
- 直连账号相关保存和诊断行为不会泄露完整 token。
- 容器诊断不能只依据 `docker inspect` 或 PID1 环境判断 pinning 是否启用；必须以 Lune 进程加载后的 effective 配置为准。
- 账号不可用时，UI/API 给出具体阻断层，不能把所有 503 都表述成同一个“账号不可用”。
- Codex CPA 账号真实请求返回 `429` 时，状态归类不能停留在 generic cooldown；必须把 quota snapshot 与 real request evidence 分层展示。
