# 100. v0.1.8 实现与验收证据

状态：已完成。本文记录 v0.1.8 本轮实现、自动化测试、真实容器测试和清理证据。

本轮最终复验证据目录：`/tmp/lune-v018-evidence-rerun`。

## 自动化测试

- `go test ./...`：通过。
- `npm run build`：通过，并刷新 `internal/site/dist`。
- Docker build：通过。
  - 镜像：`lune:v0.1.8-test`
  - 构建参数：`LUNE_VERSION=0.1.8-test`
  - 构建时使用 `GOPROXY=https://goproxy.cn,direct`，用于规避 Go proxy / GitHub 拉取链路偶发中断；一次 CPA GitHub clone 发生 TLS 中断后重试通过。

## 真实容器验收

新容器：

- `lune-v018-test`
- 端口：`127.0.0.1:23333 -> 7788`
- 镜像：`lune:v0.1.8-test`
- 隔离数据目录：`/tmp/lune-v018-test-*`
- 网络：`lune-v018-net`
- fake upstream：
  - `lune-upstream-first`：`503`
  - `lune-upstream-second`：`200`

旧容器未触碰：

- `lune-0.1.7`：`22222 -> 7788`
- `lune-0.1.5`：`11111 -> 7788`

验收结果：

- `/healthz` 返回 `{"status":"ok"}`。
- `GET /admin/api/pools` 与 `GET /admin/api/pools/{id}` 返回 `routing_policy`。
- 非法 `routing_policy=round_robin` 返回 `400`。
- `health_first` 下首次请求 first 账号返回 `503` 后 retry 到 second；后续请求因 first 进入 `serving_cooldown` 直接选择 second。
- `ordered` 下 first 账号排第一并返回 `503`，retry 后选择 second。
- `/usage` 中 `route_trace` 包含 `attempt`、`routing_policy`、`selected_account_id`、候选账号和 `skip_reason`，未包含 token、prompt、request body 或完整上游响应；证据文件为 `/tmp/lune-v018-evidence-rerun/usage.json`。
- CPA auth JSON batch preview 返回 `created=2/skipped=1/failed=1`，不泄露 refresh/access/id token。
- CPA auth JSON batch import 返回 `batch_id`、`created=2/skipped=1/failed=1/pending_runtime_sync=2`，不泄露 refresh/access/id token。
- 导入后的 Codex 账号为 `credential_status=ok`、`access_status=eligible`。
- 页面 smoke 截图覆盖 `1440x1000`、`1024x900`、`768x900`、`375x900`；DOM 检查确认策略按钮、Plan chip、单行底部 chip 和 quota 错误文案存在。

## 清理结果

验收完成后已删除：

- `lune-v018-test`
- `lune-upstream-first`
- `lune-upstream-second`
- `lune-v018-net`
- `/tmp/lune-v018-test-*`

保留的运行容器仅为旧版本：

- `lune-0.1.7`
- `lune-0.1.5`
