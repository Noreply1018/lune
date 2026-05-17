# 100. v0.1.9 规格补充证据

状态：本轮仅补充规格文档，未修改运行时代码、启动配置、CI/CD、镜像或部署脚本。

本文件记录本轮规格补充的审计、校验和测试说明。涉及运行态的 v0.1.9 实现发布前，必须按项目规则补充自动化测试、真实容器验收、测试资源清理、subagent 严格审计和 Git 提交证据。

## 范围

- `README.md`
- `01-pool-lifecycle-management.md`
- `02-codex-plus-quota-audit.md`
- `99-acceptance-matrix.md`

## 本轮补充

- 新增 Codex Plus quota 展示审计规格，记录 v0.1.8 真实容器中 Plus 账号 `wham/usage` 返回 `HTTP 401`、模型请求仍成功、UI 不应显示“无周额度”的问题。
- README 纳入 Codex Plus quota 展示修正范围。
- 验收矩阵新增 Codex Plus quota 场景，覆盖无 snapshot、401/403、primary-only、旧快照、provider 大小写、字符串数字、access 不降级、路由语义和模型请求 429 反例。
- 真实邮箱和 runtime account key 已脱敏。

## 本地校验

- 已检查 `spec/v0.1.9` 下未保留未完成占位标记。
- 已检查 Markdown 表格列数，未发现列数错位。
- 已检查 `spec/v0.1.9` 中没有完整真实邮箱或完整 auth account key。

## 容器测试

未执行。原因：本轮只修改 `spec/v0.1.9` 规格文档，不影响构建、发布、部署、启动脚本、运行配置或运行时行为。
