# 99. 验收矩阵

## 目标

本文件只做跨问题最终核对。详细设计以各问题文档为准，避免 UI 文案、连接编辑语义、直连诊断结构和容器运行态审计口径分散后出现冲突版本。

## 本轮验证状态

- 待完成：`go test ./...`
- 待完成：`npm --prefix web run build`
- 待完成：必要时补充容器 smoke test，验证新 UI 与直连账号保存语义的一致性。
- 已完成：只读审计正在运行的 `lune-0.1.6` 容器，并用临时 `noreply1018/lune:0.1.6` 容器复核 embedded CPA 启动路径。审计结论为：`docker inspect` 和 PID1 环境缺少 `LUNE_CPA_PROVIDER_PINNING_SUPPORTED` 不能证明 Lune 运行态未启用 pinning；真实 `lune up` 子进程和 embedded `CLIProxyAPI` 子进程均携带 `LUNE_CPA_PROVIDER_PINNING_SUPPORTED=1`。临时测试容器和测试 volume 已删除。

## 真实容器测试要求

凡是实现本目录里的 UI 调整，都要在**新容器**中做最终验收：

- 容器必须是新启动的 v0.1.7 实例，不能复用正在运行的旧版本容器。
- 需要使用隔离数据目录，或者从现有数据目录复制出测试副本后再挂载。
- 需要保留可复核的启动命令、端口、镜像 tag 或 digest、以及验证摘要。
- 验收完成后必须删除测试容器和临时数据卷。

建议的最小容器验收覆盖：

- `UI-01` 和 `UI-02` 同容器验证：打开直连账号详情抽屉，确认 tab 语言统一、`Connection` 仅在 `Overview`，`Diagnostics` 不再混入 CPA 专属结构。
- `UI-03` 同容器验证：保存时留空 token 不会清掉旧 token，填入新 token 才替换。
- `UI-04` 和 `UI-05` 同容器验证：直连账号诊断页更适合直连场景，且高级信息收敛后仍能辅助排障。

## 需要统一验证的点

| 编号 | 覆盖问题 | 验收方式 | 必须验证 |
| --- | --- | --- | --- |
| UI-01 | 账号详情抽屉 tab 文案 | 页面人工检查或组件截图 | `Overview / Playground / Diagnostics` 全英文一致 |
| UI-02 | 直连账号连接编辑区 | 页面交互测试 | 编辑区位于 `Overview` 顶部，不出现在 `Playground` / `Diagnostics` |
| UI-03 | token 表达语义 | 交互测试 | 默认不展示完整 token；空 token 表示保留旧值，不误导为丢失 |
| UI-04 | 直连账号诊断页 | 页面人工检查或组件测试 | 对 `openai_compat` 账号不显示 `Runtime Binding`、`Subscription`、`Quota` 作为主诊断维度；主诊断区只出现 `Connection`、`Credential`、`Route`、`Serving`、`Models` |
| UI-05 | 高级信息收敛 | 页面人工检查 | `Advanced` 区域仅保留 `Account ID`、`Runtime Base URL`、`Last Error`、`Last Checked At`、`Source Kind` 等直连排障字段，不展示完整 token 或 request body |
| OPS-01 | 0.1.6 embedded CPA pinning 运行态审计 | 真实运行容器只读审计 + 临时发布镜像容器复核 | 区分 `docker inspect` 静态环境、PID1 shell 环境和 `lune` / embedded CPA 子进程有效环境；确认 embedded 模式下 Lune effective pinning capability；测试容器用完删除 |
| OPS-02 | CPA 路由阻断原因展示 | API / 诊断页验收 | 对固定 fixture 的 CPA 账号，接口或页面必须分别展示 `provider_pinning_unsupported`、`runtime_auth_binding_unavailable`、`no_healthy_account`、`model_not_on_account`、subscription 非 active、账号 `status=error` 六类不同阻断原因之一，且不能统一折叠成同一个“账号不可用” |

## 最低验收标准

- tab 语言统一，详情抽屉不再混用中文和英文 tab 名。
- 直连账号可以在详情抽屉里直接改 API 地址和 token。
- token 默认不回填完整明文，但 UI 不会让用户误判为凭据丢失。
- 直连账号的诊断页与 CPA 账号诊断页结构不同，且更贴近直连场景。
- 直连账号相关保存和诊断行为不会泄露完整 token。
- 容器诊断不能只依据 `docker inspect` 或 PID1 环境判断 pinning 是否启用；必须以 Lune 进程加载后的 effective 配置为准。
- 账号不可用时，UI/API 给出具体阻断层，不能把所有 503 都表述成同一个“账号不可用”。
