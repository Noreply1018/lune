# 99. v0.2.0 验收矩阵

状态：验收矩阵已确认。本轮已完成 CPA 删除后重导入核心复现、生命周期审计补强、真实请求诊断 evidence 回写和隔离 `stateful-probe` 证据采集；真实账号状态矩阵和其余条目仍按本矩阵验收。

## CPA Auth JSON 删除后重导入

| ID | 场景 | 准备 | 操作 | 期望 |
| --- | --- | --- | --- | --- |
| IMP-01 | 干净库首次导入 | 新容器、新数据卷、空 Pool | 批量导入 4 个合法 Codex auth JSON | `created=4`、`failed=0`；Pool 有 4 个账号；auth 目录有 4 个文件 |
| IMP-02 | 选择 `.login-sessions.json` | 同 IMP-01，并额外选择 `.login-sessions.json` | 预检和正式导入 | `.login-sessions.json` 为 `invalid_auth_json`；合法账号文件不受影响 |
| IMP-03 | 删除后立即重导入 | 首次导入 4 个账号成功 | 连续删除 4 个 CPA 账号后立刻重新导入同一批文件 | 不出现 `pool_member_failed`；最终 4 个账号全部存在 |
| IMP-04 | 删除后等待重导入 | 首次导入成功后删除全部账号 | 等待 reload 收敛后再导入 | 4 个账号全部成功；作为 IMP-03 的对照组 |
| IMP-05 | 批次级 reload | 批量导入多个 CPA auth JSON | 检查容器日志 | 同一个批次至多触发一次导入后 reload；不因每个 item 成功而重启 runtime |
| IMP-06 | 删除 reload 协调 | 连续删除多个 CPA 账号 | 检查日志和最终 runtime auth files | 删除流程不会留下旧 auth binding；不会与随后的导入事务冲突 |
| IMP-07 | Pool membership 失败可审计 | 构造 Pool member 写入失败或 SQLite busy | 执行批量导入 | 响应和持久化审计都包含阶段、错误码和安全摘要 |
| IMP-08 | 失败回滚 | 构造单个 item DB 失败 | 检查 auth 目录和 DB | 失败 item 不残留新 auth file，不残留半写 DB account/member |
| IMP-09 | Runtime sync 失败不回滚 | 构造 CPA reload 失败 | 执行导入 | 主状态仍为 `created/updated`，runtime sync 为 `failed`，账号和 auth file 保留 |
| IMP-10 | 同账号幂等 | 已存在同 account key 账号 | 再次导入同一 auth JSON | 返回 `updated` 或等价幂等状态，不创建重复账号或重复 Pool member |
| IMP-11 | 同批重复 | 同一批上传重复 account key | 执行预检和正式导入 | 第一份处理，后续重复项 `skipped + duplicate_in_batch` |
| IMP-12 | 审计明细持久化 | 执行一次包含成功、失败、跳过的导入 | 请求结束后查询审计记录 | 能恢复 batch id、文件名、account key hash、状态、runtime sync、error code |
| IMP-13 | 安全脱敏 | 查询日志、导入审计、通知 payload | 检查敏感字段 | 不出现 refresh token、access token、id token、完整 auth JSON、完整 account key |
| IMP-14 | identity mismatch | 磁盘已有同 key 但身份字段不一致 | 导入身份不一致的 auth JSON | 返回 `identity_mismatch`；不覆盖已有 auth file；runtime 不消费错误文件 |
| IMP-15 | 更新失败恢复旧文件 | 已有账号和 auth file，构造更新后 DB/Pool member 失败 | 再次导入同 key 更新文件 | 新文件回滚，旧 auth file 恢复，DB 保持旧账号一致状态 |
| IMP-16 | runtime 原子视图 | 构造导入中途 watcher 扫描 auth 目录 | 执行批量导入并观察 runtime | runtime 不读取半写、staging 或已回滚文件，只消费已提交快照 |
| IMP-17 | 崩溃恢复 | 在导入写文件后、DB 提交前、reload 前分别终止容器 | 重启容器，等待启动流程自动 reconcile，再用只读 debug 验证 | operation 被标记 `interrupted` 或恢复完成；不遗留半写文件；pending reload 可恢复 |
| IMP-18 | 并发生命周期 | 双管理员同时导入、导入时删除账号/Pool、导入时 health/quota refresh 运行 | 并发执行矩阵 | 同一账号/Pool 生命周期状态串行化；reload 合并；无重复 member、无半写 runtime 状态 |

## 通用可审计性

| ID | 场景 | 准备 | 操作 | 期望 |
| --- | --- | --- | --- | --- |
| AUD-01 | 关键操作事件 | 执行账号导入、账号删除、Pool 删除、runtime reload | 查询最近操作 | 每个操作都有 operation id、类型、目标摘要、时间、状态和安全错误摘要 |
| AUD-02 | 阶段错误 | 构造 DB busy、runtime reload 失败、上游认证失败 | 查询操作明细 | 能看到失败阶段、错误码和安全上下文 |
| AUD-03 | 关联 ID | 执行一次批量导入 | 对齐 API log、operation record、runtime reload log | 同一 `operation_id` 能贯穿多处证据；多个 `correlation_id` 可归属同一 operation |
| AUD-04 | 最近操作快照 | 连续执行多次成功和失败操作 | 查询 recent operations | 请求结束后仍可恢复最近操作和 item 明细 |
| AUD-05 | 只读 debug | 在容器内执行全部 debug 命令 | 执行 `summary`、`recent-operations`、`operation <id>`、`db-integrity`、`cpa-auth`、`cpa-runtime`、`account <id>` | 输出脱敏且不修改 DB、文件或 runtime 状态 |
| AUD-06 | stateful-probe 必须实现 | 目标 `lune` 容器存在 | 执行 `scripts/audit/repro.sh stateful-probe` | 通过工具容器导出脱敏 evidence，清理临时工具容器；不得返回 `scenario_not_implemented` |
| AUD-07 | 脱敏审计包 | 有账号、Pool、request log、operation record | 执行 `lune debug collect --redact` | 生成结构化审计包，不含敏感凭据 |
| AUD-08 | 数据保留 | 超过最近操作保留窗口 | 触发清理 | 清理按策略执行，清理本身可审计 |
| AUD-09 | 后续 spec 约束 | 新增涉及状态变更或 runtime 行为的 spec | 审阅 spec | 必须包含可审计性小节，说明阶段、错误、关联 ID、脱敏和容器矩阵 |
| AUD-10 | 工具容器构建 | 本机可访问 Docker daemon | 执行 `scripts/audit/build-tools.sh` | 成功构建 `lune-audit-tools:local`，不改变生产 `Dockerfile` runtime 工具集 |
| AUD-11 | 工具容器命令封装 | 目标 `lune` 容器存在 | 执行 `scripts/audit/run-tools.sh jq --version` 和 `sqlite3 --version` | 命令在工具容器内执行，容器退出后不遗留 |
| AUD-12 | 只读 evidence 采集 | 目标 `lune` 容器和 `lune-data` volume 存在 | 执行 `scripts/audit/collect.sh` | 输出 `audit-output/<timestamp>/`，数据卷只读挂载，正式容器不被重启、删除或写入 |
| AUD-13 | 核心复现场景必须实现 | 已在隔离容器完成删除后重导入矩阵 | 执行 `scripts/audit/repro.sh cpa-import-reimport` | 输出该真实容器的脱敏 evidence；不得返回 `scenario_not_implemented` |
| AUD-14 | 辅助未实现场景显式失败 | 调用非发布阻塞且尚无安全编排的辅助场景 | 执行对应 `scripts/audit/repro.sh <scenario>` | 返回 `scenario_not_implemented`，不得伪造成功或静默降级 |

## 账号真实状态诊断

| ID | 场景 | 准备 | 操作 | 期望 |
| --- | --- | --- | --- | --- |
| DIA-01 | 多路证据持久化 | 配置任一 Codex CPA 账号 | 触发完整账号诊断 | 持久化 auth、models、subscription、quota、chat 或真实请求观察证据；请求结束后可通过 debug/API 恢复 |
| DIA-02 | `al` 封号案例 | 在隔离真实容器中通过 secret 配置脱敏案例 `al` | 触发完整诊断和一次安全业务探测 | 最终 `stable_diagnostic_status=banned`；证据包含明确封禁语义；不得显示为额度不足或暂态错误 |
| DIA-03 | `c` 额度不足案例 | 在隔离真实容器中通过 secret 配置脱敏案例 `c` | 触发完整诊断和一次安全业务探测 | 最终 `stable_diagnostic_status=quota_exhausted`；账号主体证据仍有效；不得显示为封号 |
| DIA-04 | 额度接口鉴权失败但可用 | 使用历史 quota/history 鉴权失败但账号可用案例 | 触发 quota probe、models/subscription/chat probe | 最终 `stable_diagnostic_status=quota_probe_auth_failed_but_usable`；调度默认仍可用；UI 显示额度接口告警 |
| DIA-05 | 单一 quota 失败不判死 | fake upstream 或真实账号返回 quota/history 401/403，其他关键 probe 正常 | 触发完整诊断 | 不得判定为 `banned`、`auth_invalid` 或全局不可用 |
| DIA-06 | 业务额度不足归一化 | fake upstream 或真实账号在 chat/真实请求返回 insufficient quota | 触发诊断或真实请求失败回写 | 判定为 `quota_exhausted`；错误归因到额度阶段；不污染 token/auth 状态 |
| DIA-07 | 暂态上游错误 | 构造 models/quota/chat 任一 probe 超时、429 或 5xx | 触发诊断 | 记录 `last_probe_status=transient_error`；保留上一 `stable_diagnostic_status`；不得覆盖为封号或额度不足 |
| DIA-08 | 调度行为一致 | 准备 `usable`、`banned`、`auth_invalid`、`quota_exhausted`、`quota_probe_auth_failed_but_usable`、`unknown` 账号 | 通过 Pool 发起请求 | 符合 scheduler 映射表；暂态错误沿用上一稳定调度；用户覆盖可审计且不改写机器诊断 |
| DIA-09 | UI/API 展示一致 | 已完成一次包含多种状态的诊断 | 查询账号列表、账号详情和 debug 输出 | 三处展示同一机器状态、调度状态、最后诊断时间和安全证据摘要 |
| DIA-10 | 诊断脱敏 | 查询诊断 DB、debug 输出、审计包和日志 | 检查敏感字段 | 不出现 token、完整 auth JSON、完整 account key、完整上游响应体 |
| DIA-11 | fake upstream 回放 | 无法长期保留真实账号时 | 回放 `al`、`c`、quota 鉴权失败可用三类上游语义 | fake upstream 结果与真实案例期望一致，但不能替代至少一次真实容器验收 |
| DIA-12 | auth invalid | 构造 refresh/access token 无效且无法恢复的账号 | 触发完整诊断 | 最终 `stable_diagnostic_status=auth_invalid`；不得误判为封号或额度不足；默认不可调度 |
| DIA-13 | 真实案例治理 | 准备 `al`、`c`、quota 鉴权失败但可用案例 | 检查验收输入和审计输出 | 凭据只来自 secret；记录脱敏 case id、采集时间、采集人或环境、当时真实状态、预期状态、可接受证据和过期策略；状态漂移时不得用 fake upstream 静默放行 |

## 关键反例

| ID | 场景 | 期望 |
| --- | --- | --- |
| NEG-01 | 源 JSON 合法但导入失败 | 错误不得被展示成“不是可识别 JSON”；必须指向真实阶段 |
| NEG-02 | runtime reload 失败 | 不得把已成功写入 DB 和 auth file 的账号标记成导入失败 |
| NEG-03 | 删除账号后 runtime 尚在重启 | 批量导入不得因 runtime reload 中途状态而产生 `pool_member_failed` |
| NEG-04 | 只读预检 | 不得写 auth file、不得创建账号、不得触发 reload |
| NEG-05 | 审计缺失 | 不允许正式导入失败明细只存在于浏览器当次响应中 |
| NEG-06 | 只靠日志猜测 | 关键失败不得只能通过时间窗口和日志猜测根因 |
| NEG-07 | debug 修改现场 | `lune debug` 命令不得修复、重载、写 DB 或删除文件 |
| NEG-08 | 审计包泄密 | 审计包不得包含 token、完整 auth JSON、完整 account key 或完整 API key |
| NEG-09 | 工具容器污染生产镜像 | 不得为了审计把 `jq`、`sqlite3` 等工具打入生产 runtime 镜像 |
| NEG-10 | 未实现场景假成功 | 未完成安全编排的复现场景不得返回成功 |
| NEG-11 | quota/history 失败误判封号 | 只有额度接口鉴权失败时不得判定为 `banned` |
| NEG-12 | 额度不足误判封号 | 业务请求返回额度不足时不得展示为封号 |
| NEG-13 | 暂态错误覆盖稳定状态 | 5xx、超时、网络错误不得覆盖上一稳定诊断状态 |
| NEG-14 | 证据层缺失 | 账号状态不得只有最终标签而无法恢复 probe 明细 |
| NEG-15 | UI 和调度分裂 | UI 展示可用但调度层判不可用，或 UI 展示封号但调度仍使用，均不允许 |
| NEG-16 | 核心复现场景未实现 | `cpa-import-reimport` 不得返回 `scenario_not_implemented` 后仍通过发布验收 |
| NEG-17 | 暂态事件当最终状态 | 不得把 `transient_error` 写成最终 `diagnostic_status` |

## 容器验收要求

| ID | 项目 | 要求 |
| --- | --- | --- |
| REL-01 | 隔离容器 | 使用新构建镜像、新容器、新数据卷和临时端口验证 |
| REL-02 | 旧容器保护 | 不停止、不删除、不复用正在运行的旧版本容器和数据卷 |
| REL-03 | 真实 runtime | 涉及 embedded CPA reload 的矩阵必须在容器内运行，不能只用单元测试替代 |
| REL-04 | 清理 | 测试结束删除临时容器和临时数据卷 |
| REL-05 | 审计 | 修改完成后由 subagent 严格审计，审计通过后提交 Git commit |
| REL-06 | 后续 spec 审查 | 后续涉及状态变更、运行时、外部服务或异步任务的 spec 必须包含可审计性要求 |
| REL-07 | 真实账号状态矩阵 | `al`、`c`、quota 鉴权失败但可用案例必须至少在一次隔离真实容器验收中跑通 |
| REL-08 | fake upstream 不替代真实验收 | fake upstream 可用于 CI 回归，但不能作为真实账号状态识别的唯一证据 |
| REL-09 | 旧数据卷升级 | 使用 v0.1.9 真实或模拟数据卷副本启动 v0.2.0 | schema migration 成功；旧账号、Pool、request log 可读；旧账号诊断初始化为 `stable_diagnostic_status=unknown`、`last_probe_status=not_run` 或空值；`scheduler_status` 按旧启停/健康字段保守映射，旧启用账号不得仅因缺少诊断历史被自动禁用；新增 operation/diagnostic 表兼容空历史 |
| REL-10 | 重启恢复 | 在导入、删除、reload、诊断中断后重启容器 | 启动流程自动 reconcile 收敛；只读 debug 可验证；无长期 running；状态可审计 |
| REL-11 | 并发压力 | 同时运行导入、删除、health/quota refresh、诊断和 routing 请求 | 无数据竞争导致的半写状态；调度、UI、debug 结果一致 |
