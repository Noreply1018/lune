# 100. v0.2.0 发布证据

状态：已完成本轮代码修复、自动化测试、隔离容器复现、审计包收口和临时资源清理；真实账号矩阵已在隔离容器中验证 `c` 额度不足与 quota 鉴权失败但可用案例，`al` 封号案例尚未完成，v0.2.0 仍未达到严格发布完成标准。

发布注意：远端已存在历史 `v0.2.0` tag，指向 `b81ee09a9d30e9fdd4d153c7f7557c5b5f55a605`。该 tag 早于当前证据收口，且 `al` 封号真实样本仍未完成；不得把该 tag 视为本轮严格发布完成证据，也不得继续推进新的 v0.2.0 release。

## 自动化测试

- `go test ./...`：通过，最近复跑时间 `2026-05-20T13:03Z` 前。
- `npm --prefix web run build`：通过，Vite 输出 `✓ built in 12.13s`。
- `npm --prefix web run test:quota`：通过，输出 `status=ok`。
- `bash -n scripts/audit/*.sh && git diff --check`：通过。
- Docker build：通过。
  - 镜像：`lune:v0.2.0-quota-fix`
  - 构建参数：默认 `dev / unknown` 构建元数据，仅用于隔离验证。

## 隔离容器验证

- 复现命令：`scripts/audit/repro.sh stateful-probe`
- 证据目录：`audit-output/repro-stateful-probe-20260520T110117Z`
- 结果：`request_log_collected`
- HTTP 状态：`503`
- `request_log_id=12`
- `account_log_matched=0`
- `diagnostic_evidence_count=0`

已验证的行为：

- `banned` 诊断代码路径可由构造信号写回；真实 `al` 封号样本尚未完成，不能作为真实矩阵通过证据。
- `quota_probe_auth_failed_but_usable` 可在 quota 探测 401/403 且账号仍可用时写回。
- quota 401/403 不再误写 `needs_login`。
- quota 鉴权失败但可用时，诊断与调度保持 warning 但仍可路由。
- 旧数据中残留 `needs_login` 的 Codex CPA 账号，在 stateful 真实模型请求成功后会恢复 `cpa_credential_status=ok / model_request_success`。

## 真实账号状态矩阵进展

- `c` 额度不足案例：已在从 `lune-data-018` 复制出的隔离容器中验证。真实模型请求返回 `HTTP 429 / usage_limit_reached`，诊断写回 `stable_diagnostic_status=quota_exhausted`、`last_probe_status=quota_exhausted_signal`、`scheduler_status=ineligible`，证据包含 `routing_observation / model_request / quota_exhausted`。证据目录：`audit-output/repro-stateful-probe-20260520T115801Z`。
- quota 鉴权失败但可用案例：已在从 `lune-data-018` 复制出的隔离容器中验证。启动后 `quota_probe / wham_usage` 返回 `HTTP 401 / quota_probe_auth_failed`，随后强制账号真实模型请求返回 `HTTP 200`，诊断写回 `stable_diagnostic_status=quota_probe_auth_failed_but_usable`、`last_probe_status=succeeded`、`scheduler_status=eligible_with_warning`，证据包含 `quota_probe / wham_usage` 和 `routing_observation / model_request`。证据目录：`audit-output/repro-quota-auth-usable-20260520T125927Z`。
- 旧 credential 残留恢复案例：已在从 `lune-0.1.5` 只读复制出的隔离容器中验证。账号 2/3/4 强制 stateful 真实模型请求均返回 `HTTP 200`，请求日志 `success=1`，`cpa_credential_status` 写回 `ok`、`cpa_credential_reason=model_request_success`；账号 2/3 的 quota 鉴权失败但可用诊断仍保持 `stable_diagnostic_status=quota_probe_auth_failed_but_usable`、`scheduler_status=eligible_with_warning`。证据目录：`audit-output/repro-credential-ok-20260520T131713Z`。
- `al` 封号案例：现有可见 volume 中未发现能产出账号级 banned 语义的真实样本；仍为发布阻塞。

未完成项：

- `al` 封号真实账号矩阵尚未在一次隔离真实容器中跑通。
- 因此本版本证据只能证明代码和隔离复现链路已收敛，不能证明 v0.2.0 已严格发布完成。

## 清理

- 已删除临时测试容器：`lune-v020-stage`
- 已删除临时测试卷：`lune-v020-stage-data`
- 已删除临时 quota 可用性验证容器与卷：`lune-v020-quota-usable-20260520T124820Z`
- 已删除临时 quota 可用性验证容器与卷：`lune-v020-quota-usable-20260520T125927Z`
- 已删除临时旧 credential 残留验证容器与卷：`lune-audit-credential-ok-20260520T131713Z-*`
- 旧运行容器未改动：
  - `lune-v020-019-test`
  - `lune-0.1.9`
  - `lune-0.1.5`
