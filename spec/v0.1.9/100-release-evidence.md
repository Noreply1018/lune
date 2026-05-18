# 100. v0.1.9 实现与验收证据

状态：实现验收已完成。

## 范围

- Pool 生命周期管理：独立创建、默认 token、重命名、启停、删除 Pool 同删账号、共享账号级联删除、删除最后 Pool 后账号 / Pool / token 清空。
- Codex Plus quota 展示与派生：`wham/usage` 401/403 不再显示“无周额度”，不单独降级 access，不覆盖模型请求成功证据；模型请求 429 作为 quota / rate-limit evidence。
- Codex 真实探查状态回写：区分普通强制账号、stateful probe、纯 diagnostic；真实认证失败写 `needs_login`，纯 diagnostic 不写状态；stateful probe 不计普通 usage 但保留审计日志。
- 空集合 API 响应：accounts / pools / tokens 空列表返回 `[]`，避免前端和自动化验收遇到 `null`。
- Pool 删除健壮性：删除 Pool 同删账号时对短暂 SQLite busy 做小间隔重试，避免异步模型发现并发写库导致删除 500。

## 本地自动化

- `go test ./internal/gateway ./internal/store ./internal/admin`：通过。
- `go test ./...`：通过。
- `npm --prefix web run build`：通过，并重新生成 `internal/site/dist`。
- `npm --prefix web run test:quota`：通过；验证 provider 大小写不敏感、quota window 数字字符串可解析、非法 `used_percent: "abc"` 被拒绝、缺省 `used_percent` 按 0 处理。
- Codex quota label fixture：通过；验证 Plus + quota fetch 失败显示“额度查询失败”、Plus primary-only 显示“周额度待同步”、Free primary-only 显示“无周额度”、Unknown primary-only 显示“周额度未知”。

## 测试镜像

- 构建命令：`docker build -t lune:v0.1.9-current-test --build-arg LUNE_VERSION=0.1.9-current-test .`
- 镜像：`lune:v0.1.9-current-test`
- Manifest digest：`sha256:7cf081529c5c819eebb0feb2568d646b9ca09eb2c5b04ab75aaa56ed9f367aa4`
- 运行容器：`lune-019-current-test`
- 端口：`127.0.0.1:23333:7788`
- 数据卷：`lune-data-019-current-test`
- 环境：`LUNE_ADMIN_TOKEN=admintest`、`LUNE_CPA_PROVIDER_PINNING_SUPPORTED=true`
- fake CPA：本机临时 Python 服务，`http://host.docker.internal:28888`

## 容器矩阵

使用上述镜像启动隔离容器完成干净矩阵，旧容器 `lune-0.1.8` 和 `lune-0.1.5` 未被改动。

最终矩阵输出：

```json
{
  "status": "ok",
  "source_pool_id": 1,
  "matrix_pool_id": 2,
  "cpa_account_ids": {
    "ok": 1,
    "authfail": 2,
    "authunavail": 3,
    "limited": 4,
    "fail": 5,
    "ordinary": 6,
    "diagnostic": 7,
    "diagstateful": 8,
    "diagsuccess": 9
  },
  "logs_checked": 10,
  "usage_summary": 1,
  "traffic_kinds": [
    "diagnostic",
    "ordinary",
    "stateful_probe"
  ],
  "pool_delete_status": 503
}
```

## Pool / API 验收映射

- Pool 创建：`POST /admin/api/pools` 创建 Source Pool 和 Matrix Pool 均返回 201。
- 默认 token：`GET /admin/api/pools/{id}/tokens` 返回 1 个默认 Pool token，`POST /admin/api/tokens/{id}/reveal` 返回真实 `sk-` token。
- Pool 重命名与策略：`PUT /admin/api/pools/{id}` 把 Matrix Pool 改为 `Matrix Pool Renamed`，`routing_policy=ordered` 被保留。
- Pool 启停：`POST /admin/api/pools/{id}/disable` 后 `enabled=false`，`POST /admin/api/pools/{id}/enable` 后 `enabled=true`。
- 共享账号级联删除：OpenAI-compatible 账号同时加入 Source Pool 和 Matrix Pool；删除 Source Pool 后，账号被删除，Matrix Pool membership 同步消失，`GET /admin/api/accounts` 返回 `[]`。
- 最后清空：删除 Matrix Pool 和 Source Pool 2 后，`GET /admin/api/accounts`、`GET /admin/api/pools`、`GET /admin/api/tokens` 均返回空数组 `[]`。

## CQ 验收映射

- CQ-01 / CQ-02 / CQ-03 / CQ-04 / CQ-05 / CQ-21 / CQ-22 / CQ-23 / CQ-24：fake CPA management `api-call` 对 `wham/usage` 返回 `HTTP 401`，导入的 Plus Codex 账号保持 `cpa_access_status=eligible`，quota 维度为 `cpa_quota_status=error`、`cpa_quota_last_error=HTTP 401`；前端 label fixture 显示“额度查询失败”，不显示“无周额度”或“模型请求被限流”。
- CQ-06 / CQ-07：前端 label fixture 对 Plus primary-only / 无 secondary window 返回“周额度待同步”。
- CQ-08 / CQ-09：前端 label fixture 对 Free primary-only 返回“无周额度”；Go plan 走同一 free/go 分支。
- CQ-10：前端 label fixture 对 Unknown primary-only 返回“周额度未知”。
- CQ-11 / CQ-12 / CQ-13 / CQ-14：本地 store/gateway 测试覆盖可解析 snapshot、blocked snapshot、旧 snapshot 遇 quota fetch 401/403 不降级 access、不被辅助接口错误覆盖的路径；全量 `go test ./...` 通过。
- CQ-15 / CQ-16 / CQ-17：本地 health/store 测试覆盖 empty body、invalid JSON、CPA management api-call 失败归为 quota error；全量 `go test ./...` 通过。
- CQ-18 / CQ-19 / CQ-20：Codex quota TypeScript fixture 覆盖 provider 大小写、字符串数字、非法字符串数字。
- CQ-25：容器矩阵中 `ok@example.com` 的 `gpt-ok` stateful probe 返回 200 后，账号保持 `cpa_access_status=eligible`，且 `cpa_quota_status=error / HTTP 401` 保留。
- CQ-26：容器矩阵中 `limited@example.com` 的 `gpt-limited` stateful probe 返回 429 后，写入 quota / rate-limit evidence，未写 `needs_login`，未写 serving cooldown。
- 补充：`diagsuccess@example.com` 先跑 stateful probe 429，再跑 `X-Lune-Diagnostic: true` 的 200 成功请求，quota evidence 保留，未被 diagnostic 成功清理为 `model_request_success`。

## RPW 验收映射

- RPW-01 / RPW-02：`ok@example.com` 使用 `X-Lune-Account-Id` + `X-Lune-Probe-Mode: stateful` 请求 `gpt-ok` 返回 200；credential 保持 `ok`，access 保持 `eligible`，ordinary usage 不增加。
- RPW-03 / RPW-04：`authfail@example.com` 的 stateful probe 请求 `gpt-authfail` 返回 401 + `refresh token invalid`，写 `cpa_credential_status=needs_login`，普通路由会跳过该账号。
- RPW-05：`authunavail@example.com` 的 stateful probe 请求 `gpt-authunavail` 返回 503 + `auth_unavailable: no auth available`，写 `needs_login`。
- RPW-06：gateway 单元测试覆盖 service key / management key / invalid authorization 错误不会误写账号 `needs_login`；全量 `go test ./...` 通过。
- RPW-07：`limited@example.com` 的 stateful probe 请求 `gpt-limited` 返回 429，写 quota evidence，不显示需要重登语义。
- RPW-08：gateway 单元测试覆盖普通非 diagnostic 5xx 写 serving failure / cooldown；全量 `go test ./...` 通过。
- RPW-09 / RPW-16：`fail@example.com` 的 stateful probe 请求普通 ChatGPT 503，credential 保持 `ok`，serving 保持 `healthy`；`authunavail@example.com` 的 503 + 明确 auth signal 写 `needs_login`。
- RPW-10：gateway 单元测试覆盖 stateful probe 下网络 / retryable failure 不写 serving cooldown；全量 `go test ./...` 通过。
- RPW-11 / RPW-15：`/admin/api/usage?range=all&limit=200` summary 为 `total_requests=1`，只统计 ordinary 强制账号请求；logs 保留 `ordinary`、`stateful_probe`、`diagnostic` 三类，`logs_checked=10`。
- RPW-12：`diagnostic@example.com` 带 `X-Lune-Diagnostic: true` 请求 `gpt-authfail` 返回 401，未写 `needs_login`，日志为 diagnostic traffic。
- RPW-13：`ordinary@example.com` 只带 `X-Lune-Account-Id` 请求 `gpt-authfail` 返回 401，写 `needs_login`，日志为 ordinary force-account traffic。
- RPW-14：本地 UI 派生和 gateway 状态测试覆盖 credential 硬失败优先于 quota warning；全量 `go test ./...` 和 `npm --prefix web run build` 通过。
- RPW-15 补充：`diagstateful@example.com` 同时带 `X-Lune-Diagnostic: true` 与 `X-Lune-Probe-Mode: stateful` 时，diagnostic 优先，未写 `needs_login`，日志 `traffic_kind=diagnostic` 且 `stateful_probe=false`。
- 补充：`diagsuccess@example.com` 先跑 stateful probe 429，再跑 diagnostic 成功请求，access/quota 旧证据保留，diagnostic 成功不清理旧 quota evidence。

## 清理

- 已删除测试容器：`docker rm -f lune-019-current-test`
- 已删除测试数据卷：`docker volume rm lune-data-019-current-test`
- 已停止 fake CPA 临时进程。
- 已确认 `127.0.0.1:23333` 与 `127.0.0.1:28888` 无监听进程。
- 清理后仅保留旧版本运行容器：
  - `lune-0.1.8`：`noreply1018/lune:0.1.8`，`127.0.0.1:22222->7788`
  - `lune-0.1.5`：`noreply1018/lune:0.1.5`，`0.0.0.0:11111->7788`
