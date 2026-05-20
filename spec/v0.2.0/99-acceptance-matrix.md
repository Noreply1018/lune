# 99. v0.2.0 验收矩阵

状态：验收矩阵已确认，等待实现。

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

## 关键反例

| ID | 场景 | 期望 |
| --- | --- | --- |
| NEG-01 | 源 JSON 合法但导入失败 | 错误不得被展示成“不是可识别 JSON”；必须指向真实阶段 |
| NEG-02 | runtime reload 失败 | 不得把已成功写入 DB 和 auth file 的账号标记成导入失败 |
| NEG-03 | 删除账号后 runtime 尚在重启 | 批量导入不得因 runtime reload 中途状态而产生 `pool_member_failed` |
| NEG-04 | 只读预检 | 不得写 auth file、不得创建账号、不得触发 reload |
| NEG-05 | 审计缺失 | 不允许正式导入失败明细只存在于浏览器当次响应中 |

## 容器验收要求

| ID | 项目 | 要求 |
| --- | --- | --- |
| REL-01 | 隔离容器 | 使用新构建镜像、新容器、新数据卷和临时端口验证 |
| REL-02 | 旧容器保护 | 不停止、不删除、不复用正在运行的旧版本容器和数据卷 |
| REL-03 | 真实 runtime | 涉及 embedded CPA reload 的矩阵必须在容器内运行，不能只用单元测试替代 |
| REL-04 | 清理 | 测试结束删除临时容器和临时数据卷 |
| REL-05 | 审计 | 修改完成后由 subagent 严格审计，审计通过后提交 Git commit |

