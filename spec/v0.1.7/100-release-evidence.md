# 100. 发布阻塞证据记录

## 状态

状态：实现、测试和真实容器矩阵已完成；最终 `gpt-5.5` subagent 严格审计待重新执行并通过后，才能提交发布阻塞改动。

本文件记录 v0.1.7 发布阻塞项的可复核证据。测试容器和临时目录按项目规则已清理，因此审计时不应依赖 `docker ps -a` 中的已删除容器残留。

## 本地验证

- 已通过：`go test ./internal/admin ./internal/store ./internal/gateway ./internal/router ./internal/health`
- 已通过：`go test ./...`
- 已通过：`npm run build`，工作目录 `web`
- 已通过：`docker build -t lune:v0.1.7-blocker-test --build-arg LUNE_VERSION=0.1.7-blocker-test .`

本轮构建产物已写入 `internal/site/dist/**`，对应 hash 文件删除和新增属于预期前端构建结果。

## Embedded CPA 容器矩阵

容器：`lune-v017-embedded-final`

启动摘要：

```sh
docker run -d \
  --name lune-v017-embedded-final \
  -p 127.0.0.1:17791:7788 \
  -v /tmp/lune-v017-embedded-final.u2yRnx:/app/data \
  -e LUNE_PORT=7788 \
  -e LUNE_DATA_DIR=/app/data \
  -e LUNE_CPA_AUTH_DIR=/app/data/cpa-auth \
  -e LUNE_GATEWAY_TMP_DIR=/app/data/tmp \
  lune:v0.1.7-blocker-test
```

验证结果：

- `/healthz` 返回 `{"status":"ok"}`。
- `/admin/api/cpa/service` 返回 `runtime_mode=embedded`、`running_version=v7.0.2-lune.1`、`provider_pinning_supported=true`、`provider_pinning_state=enabled`。
- Pool token：
  - 缺省 `token` 的 `PUT /admin/api/tokens/{id}` 保留旧 token。
  - 显式 `token:""` 返回 `400`，错误为 `token cannot be empty`。
  - 非空 token 替换成功，响应只返回 masked 值和 `updated_at`，不返回完整 token。
- CPA auth JSON：
  - 使用 fake Codex auth JSON fixture 导入成功。
  - 上传文件名 `../../x.json` 被忽略，生成的 account key 为 `codex-embedded-final@example.com-plus`。
  - 响应不包含 `refresh_token`、`access_token`、`id_token` 值或字段。
  - 重复导入同一账号返回同一 account id，Pool member 幂等。
  - `.login-sessions.json` 被拒绝。
  - Pool 详情中只出现一条导入账号成员。

清理结果：

- 已执行 `docker rm -f lune-v017-embedded-final`。
- 已删除 `/tmp/lune-v017-embedded-final.u2yRnx`。

## External Fake CPA 429 容器矩阵

容器：`lune-v017-ct429-matrix`

fake CPA：host 侧临时 HTTP server `host.docker.internal:19091`，只提供测试用 `/healthz`、`/v0/management/auth-files`、`/v0/management/api-call`、`/api/provider/codex/v1/models`、`/api/provider/codex/v1/chat/completions`。不使用真实 Codex 账号或额度。

启动摘要：

```sh
docker run -d \
  --name lune-v017-ct429-matrix \
  -p 127.0.0.1:17790:7788 \
  -v /tmp/lune-v017-ct429-matrix.gzWAsA:/app/data \
  -e LUNE_PORT=7788 \
  -e LUNE_DATA_DIR=/app/data \
  -e LUNE_CPA_AUTH_DIR=/app/data/cpa-auth \
  -e LUNE_GATEWAY_TMP_DIR=/app/data/tmp \
  -e LUNE_EMBEDDED_CPA=0 \
  -e LUNE_CPA_BASE_URL=http://host.docker.internal:19091 \
  -e LUNE_CPA_API_KEY=sk-fake-cpa \
  -e LUNE_CPA_MANAGEMENT_KEY=mgmt-fake \
  -e LUNE_CPA_PROVIDER_PINNING_SUPPORTED=1 \
  lune:v0.1.7-blocker-test
```

验证结果：

- `/healthz` 返回 `{"status":"ok"}`。
- `/admin/api/cpa/service` 返回 `runtime_mode=external`、`provider_pinning_state=enabled`。
- 导入 fake Codex auth JSON 后，账号进入可解释状态，runtime binding 可确认。
- 文案型 `too many requests` 429：
  - 普通请求 `model=gpt-cpa429-bare` 后，账号写入 `cpa_quota_status=blocked`。
  - `cpa_quota_last_error=HTTP 429 from model request: too many requests`。
  - `serving_status=cooldown`。
- `CT-429-02` 明确 quota 文案 `429`：
  - 普通请求 `model=gpt-cpa429-quota` 后，账号写入 `cpa_quota_status=blocked`。
  - `cpa_quota_last_error=HTTP 429 from model request: quota exceeded rate limit reached`。
  - request log 中 `runtime_binding_status=confirmed`。
- `CT-429-05` quota snapshot 与模型请求 evidence 分层：
  - fake CPA `wham/usage` 返回 `{"allowed":true,"limit_reached":false,"used_percent":20}`。
  - 模型请求 429 后，`codex_quota_json` 仍保留 ok snapshot。
  - 模型请求 evidence 未被 ok snapshot 覆盖，`cpa_quota_status` 仍为 `blocked`。
- `CT-429-06` cooldown 过期后解释：
  - 将 `cooldown_until` 模拟为 `2000-01-01T00:00:00Z` 后重启测试容器。
  - 普通请求返回 `503 no_healthy_account`。
  - `cpa_quota_status` 仍为 `blocked`，`cpa_quota_last_error` 仍保留模型请求 `HTTP 429` evidence。
- 强制账号成功恢复：
  - 使用 `X-Lune-Account-Id` 强制请求 `model=gpt-cpa429-ok` 返回 `200`。
  - 该成功请求清理模型请求 429 evidence。

清理结果：

- 已执行 `docker rm -f lune-v017-ct429-matrix`。
- 已停止 host 侧 fake CPA 进程。
- 已删除 `/tmp/lune-v017-ct429-matrix.gzWAsA`。

## Empty-body Bare 429 容器补充矩阵

容器：`lune-v017-ct429-bare`

fake CPA：host 侧临时 HTTP server `host.docker.internal:19092`，模型请求返回 HTTP `429` 且 body 为空，用于验证真正无 quota/rate-limit 文案的裸 `429` 分类。

启动摘要：

```sh
docker run -d \
  --name lune-v017-ct429-bare \
  -p 127.0.0.1:17792:7788 \
  -v /tmp/lune-v017-ct429-bare.CVaCdR:/app/data \
  -e LUNE_PORT=7788 \
  -e LUNE_DATA_DIR=/app/data \
  -e LUNE_CPA_AUTH_DIR=/app/data/cpa-auth \
  -e LUNE_GATEWAY_TMP_DIR=/app/data/tmp \
  -e LUNE_EMBEDDED_CPA=0 \
  -e LUNE_CPA_BASE_URL=http://host.docker.internal:19092 \
  -e LUNE_CPA_API_KEY=sk-fake-cpa \
  -e LUNE_CPA_MANAGEMENT_KEY=mgmt-fake \
  -e LUNE_CPA_PROVIDER_PINNING_SUPPORTED=1 \
  lune:v0.1.7-blocker-test
```

验证结果：

- `/healthz` 返回 `{"status":"ok"}`。
- `/admin/api/cpa/service` 返回 `runtime_mode=external`、`provider_pinning_state=enabled`。
- 导入 fake Codex auth JSON 后，普通请求 `model=gpt-cpa429-empty` 返回空 body HTTP `429`。
- 账号写入 `cpa_quota_status=error`。
- `cpa_quota_last_error=HTTP 429 from model request`。
- 同时写入 `serving_status=cooldown` 和 `cooldown_until`。
- `codex_quota_json` 保留 ok snapshot：`{"allowed":true,"limit_reached":false,"used_percent":20}`。

清理结果：

- 已执行 `docker rm -f lune-v017-ct429-bare`。
- 已停止 host 侧 fake CPA 进程。
- 已删除 `/tmp/lune-v017-ct429-bare.CVaCdR`。

## 直连和非 Codex 429 容器覆盖

在前序隔离容器 `lune-v017-blocker-test` 中使用 openai_compat fake upstream 覆盖：

- 直连账号 `api_key:""` 保存保留旧 key，响应仍只返回 masked 状态。
- openai_compat fake upstream 返回 `429` 后，只进入 serving cooldown。
- 非 Codex 账号的 `cpa_quota_status` 保持 `unknown`，不写 Codex quota evidence。

该容器已删除，临时目录 `/tmp/lune-v017-blocker-test.qbUV8R` 已删除。

## 审计记录

- 第一轮 `gpt-5.5` subagent 严格审计结论为 FAIL，指出 auth JSON 失败回滚和 CT-429 容器矩阵证据缺口。
- 已修复：auth JSON 导入失败后回滚 auth file 并再次 reload runtime；已有账号覆盖导入失败时恢复账号 DB 快照。
- 已新增测试：`TestImportCpaAuthJSONRestoresExistingAccountAndFileWhenPoolAddFails`。
- 已补齐：裸 `429`、quota snapshot 分层、cooldown 过期后阻断的真实容器矩阵证据。
- 第二轮复审指出证据未落到仓库文档中，本文件用于补齐可复核证据入口。
- 后续复审指出 `too many requests` 不等同于真正裸 `429`；已补充 `lune-v017-ct429-bare` 空 body 429 容器证据。
- 最终 `gpt-5.5` subagent 严格审计尚未通过；通过前不得提交发布阻塞改动。

## 当前清理状态

最终核对时：

- `docker ps` 仅剩旧容器 `lune-0.1.6` 和 `lune-0.1.5`。
- 未占用测试端口 `17790`、`17791`、`19091`。
- 未占用测试端口 `17792`、`19092`。
- 上述 `/tmp/lune-v017-*` 测试目录均已删除。
