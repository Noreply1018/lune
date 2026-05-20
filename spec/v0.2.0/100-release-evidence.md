# 100. v0.2.0 发布证据

状态：已完成本轮代码修复、自动化测试、隔离容器复现、审计包收口和临时资源清理；真实账号矩阵仍按 `99-acceptance-matrix.md` 持续验收。

## 自动化测试

- `go test ./...`：通过。
- `npm --prefix web run build`：通过。
- `npm --prefix web run test:quota`：通过。
- `bash -n scripts/audit/*.sh && git diff --check`：通过。
- Docker build：通过。
  - 镜像：`lune:v0.2.0-current-audit`
  - 构建参数：`LUNE_VERSION=0.2.0-current-audit`

## 隔离容器验证

- 复现命令：`scripts/audit/repro.sh stateful-probe`
- 证据目录：`audit-output/repro-stateful-probe-20260520T110117Z`
- 结果：`request_log_collected`
- HTTP 状态：`503`
- `request_log_id=12`
- `account_log_matched=0`
- `diagnostic_evidence_count=0`

已验证的行为：

- `banned` 诊断信号可写回。
- `quota_probe_auth_failed_but_usable` 可在 quota 探测 401/403 且账号仍可用时写回。
- quota 401/403 不再误写 `needs_login`。
- quota 鉴权失败但可用时，诊断与调度保持 warning 但仍可路由。

## 清理

- 已删除临时测试容器：`lune-v020-stage`
- 已删除临时测试卷：`lune-v020-stage-data`
- 旧运行容器未改动：
  - `lune-v020-019-test`
  - `lune-0.1.9`
  - `lune-0.1.5`
