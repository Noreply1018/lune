# 100. v0.2.0 发布证据

状态：已完成本轮代码修复、自动化测试、隔离容器复现、审计包收口和临时资源清理；真实账号矩阵已在隔离容器中验证 `c` 额度不足案例，`al` 和 quota 鉴权失败但可用案例尚未完成，v0.2.0 仍未达到严格发布完成标准。

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

## 真实账号状态矩阵进展

- `c` 额度不足案例：已在从 `lune-data-018` 复制出的隔离容器中验证。真实模型请求返回 `HTTP 429 / usage_limit_reached`，诊断写回 `stable_diagnostic_status=quota_exhausted`、`last_probe_status=quota_exhausted_signal`、`scheduler_status=ineligible`，证据包含 `routing_observation / model_request / quota_exhausted`。
- `al` 封号案例：现有可见 volume 中未发现能产出账号级 banned 语义的真实样本；仍为发布阻塞。
- quota 鉴权失败但可用案例：现有可见 volume 中可复现 quota probe `HTTP 401`，但同一账号业务请求返回 `HTTP 429 / usage_limit_reached` 或 `auth_unavailable`，不能证明“鉴权失败但可用”；仍为发布阻塞。

未完成项：

- `al`、quota 鉴权失败但可用两类真实账号矩阵尚未在一次隔离真实容器中跑通。
- 因此本版本证据只能证明代码和隔离复现链路已收敛，不能证明 v0.2.0 已严格发布完成。

## 清理

- 已删除临时测试容器：`lune-v020-stage`
- 已删除临时测试卷：`lune-v020-stage-data`
- 旧运行容器未改动：
  - `lune-v020-019-test`
  - `lune-0.1.9`
  - `lune-0.1.5`
