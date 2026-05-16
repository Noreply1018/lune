# 99. 验收矩阵

## 目标

本文件只做跨问题最终核对。详细设计以各问题文档为准，避免验收项和设计规则分散后出现冲突版本。

除“本轮验证状态”和各章节“本轮已完成”明确列出的内容外，下面的验收清单仍表示最终目标或后续待验收项，不能解读为本轮全部已完成。

## 本轮验证状态

- 已通过：`go test ./...`、`npm --prefix web run build`、`sh -n docker/entrypoint.sh`、`git diff --check`、`docker compose -f docker-compose.yml config`、`docker compose -f docker-compose.prod.yml config`。
- 已通过：Docker build `lune:v0.1.6-fake-ct7`。
- 已通过：新容器 `lune-v016-fake-ct` 在 `127.0.0.1:11129` 完成 health、readyz、diagnostic request、usage exclusion smoke test。
- 已通过：新容器 `lune-v016-fake-ct2` 在 `127.0.0.1:11130` 完成 error sanitization / repeat folding / stdout suppression smoke test。
- 已通过：新容器 `lune-v016-final-smoke` 在 `127.0.0.1:11131` 完成 health 与 data-retention(DB/WAL/SHM) 字段 smoke test。
- 已通过：新容器 `lune-v016-delete-ct` 在 `127.0.0.1:11132` 完成 CPA 删除、auth file 清理与 reload signal smoke test。
- 已通过：新容器 `lune-v016-readyz-ct` 在 `127.0.0.1:11133` 完成 enabled CPA account + disabled CPA service 时 `/readyz` 返回 `503` 和明确原因。
- 已通过：新容器 `lune-v016-readyz-missing-ct` 在 `127.0.0.1:11134` 完成 enabled CPA account 缺失 CPA service、同时另有 healthy CPA service 时 `/readyz` 返回 `503` 和 `service missing` 原因。
- 已通过：新容器 `lune-v016-ct0206` 在 `127.0.0.1:11143` 使用 fake account、外部 mock upstream 和 seed 数据完成 `CT-02` 到 `CT-06` 的代表性容器验收；明确 seed `quota_status=blocked`、`subscription_status=expired`、`serving_status=cooldown` 和 provider pinning unsupported 的 CPA 账号后，自动路由跳过这些账号并落到健康账号，强制 CPA 账号在 pinning unsupported 时返回 `503 runtime_auth_binding_unavailable/provider_pinning_unsupported`，普通强制 cooldown 账号返回 `503 no_healthy_account`，diagnostic request 返回 `200` 且 request log 标记 `diagnostic=1` 并被普通 Usage 排除，stream 缺 `[DONE]` 记录 `stream closed before [DONE]` 且不惩罚账号，stream `500` JSON error 保留安全上游 message，stream `response.failed` 记录 `mock capacity exhausted` 并让后续独立请求绕开 cooldown 账号。
- 已通过：新容器 `lune-v016-ct07b` 在 `127.0.0.1:11144` 使用 fake cpa-auth 和 seed 数据补齐 `CT-07`：从 Pool 移除只删除 membership，不删除账号和 auth file；删除 CPA 账号删除 DB row、Pool membership、磁盘 auth file 并写入 reload signal；历史 request log 保留删除前 `account_label_snapshot=CT07B CPA Account`；重新导入同一 account key 写入 `fake-access-v2` auth file，不与旧文件冲突。
- 已通过：新容器 `lune-v016-ct8` 在 `127.0.0.1:11138` 完成 embedded CPA management `auth-files` metadata 可读、provider endpoint 携带 pinned auth headers 可访问、删除 fake CPA 账号后 auth file 删除并写入 `cpa-reload.signal`。
- 已通过：新容器 `lune-v016-ct9` 在 `127.0.0.1:11137` 注入 embedded CPA 子进程退出；entrypoint 触发容器退出并自动删除测试容器。
- 已通过：Compose 临时项目 `lunev016ct10` 使用专用容器 `lune-v016-ct10`、端口 `127.0.0.1:11139` 和专用卷 `lune-v016-ct10-data` 完成待发布 compose 实跑；healthcheck 进入 `healthy`，端口绑定、named volume、`json-file` 日志轮转、restart policy 和 `down --timeout 15` 停止清理均符合预期。
- 已通过：新容器 `lune-v016-ct111213` 在 `127.0.0.1:11140` 完成错误信息安全截断、重复错误折叠、stdout 降噪和 Usage 上限 smoke test；500 错误摘要被截断到 511 字节且敏感 token 被移除，约 1000 次 diagnostic 重复错误没有线性膨胀成 1000 条 request log，stdout 仅保留少量带 `suppressed_repeats` 的汇总行，`/admin/api/usage?page_size=500` 实际只返回 200 条记录。
- 已通过：新容器 `lune-v016-ct13b` 在 `127.0.0.1:11141` 使用 6000 条 fake request log seed 数据完成 Usage 大数据量复测；`/admin/api/usage?range=all&page_size=500` 返回 `page_size=200`、`items=200`、`total=5838`，SQLite `EXPLAIN QUERY PLAN` 确认 account/source/token 过滤分别使用 `idx_request_logs_usage_account_created`、`idx_request_logs_usage_source_created`、`idx_request_logs_usage_token_created`，model 过滤使用 `idx_request_logs_usage_model_requested_created` 与 `idx_request_logs_usage_model_actual_created` 的 multi-index plan。
- 已通过：新容器 `lune-v016-ct7-smoke` 在 `127.0.0.1:11145` 使用最终镜像 `lune:v0.1.6-fake-ct7` 完成 `/healthz` smoke test。
- 已通过：新容器 `lune-v016-real-ct01` 在 `127.0.0.1:11146` 使用镜像 `lune:v0.1.6-fake-ct7`、隔离数据副本和 embedded CPA 完成 `CT-01` 真实多账号 Codex 上游消费验证；原始数据目录以只读方式复制到测试副本，复用其中已由用户导入的真实 CPA 账号和凭据，未记录 token、auth file 内容、完整凭据、真实邮箱或本机持久路径，旧容器 `lune-0.1.5` 未被改动。启动命令摘要：`docker run -d --name lune-v016-real-ct01 -p 127.0.0.1:11146:7788 -v <isolated-data-copy>:/app/data lune:v0.1.6-fake-ct7`；环境变量摘要：`LUNE_PORT=7788`、`LUNE_DATA_DIR=/app/data`、`LUNE_CPA_AUTH_DIR=/app/data/cpa-auth`、`LUNE_GATEWAY_TMP_DIR=/app/data/tmp`、`LUNE_CPA_BASE_URL=http://127.0.0.1:8317`、`LUNE_CPA_API_KEY=<redacted>`、`LUNE_CPA_MANAGEMENT_KEY=<redacted>`、`CPA_API_KEY=<redacted>`。强制账号路由 `X-Lune-Account-Id=1/2/3` 均返回 `200` 且响应头 `X-Lune-Account` 分别为 `1/2/3`；对应 request log `id=1649/1650/1651` 均为 `runtime_binding_status=confirmed`，`account_id` 与各自脱敏 `runtime_auth_index/runtime_auth_id/runtime_account_key` 稳定对应。自动路由 3 次均返回 `200`，选择账号 `3`，request log `id=1652/1653/1654` 均使用账号 `3` 的 confirmed pinned runtime auth。账号 `1` 连续 10 次强制普通请求均返回 `200`，request log `id=1655` 到 `1664` 全部保持同一脱敏 runtime auth index、同一脱敏 runtime auth id 和同一脱敏 account key，验证 pinned runtime auth 稳定。测试容器已执行 `docker rm -f lune-v016-real-ct01` 删除，测试数据副本和临时 Docker volume 均已删除。
- 保护项：旧容器 `lune-0.1.5` 保持运行，未被本轮测试改动。

## 真实容器测试分层

所有 v0.1.6 待解决项的最终验收都必须在新 v0.1.6 镜像启动的临时容器中完成。除明确标注“需要用户亲自导入真实账号”的项目外，默认使用 fake account、mock upstream、mock CPA management/provider 或直接 seed 测试数据库完成，不消耗真实 Codex 额度。测试容器必须使用全新数据目录，不复用或影响旧版本正在运行的容器，用完必须删除。

验收记录必须保留：

- 镜像 tag 或 digest。
- 容器启动命令、端口、数据目录和环境变量摘要。
- mock upstream / mock CPA 的行为配置。
- 用户亲自导入真实账号的步骤记录；不得记录 token、auth file 内容或完整凭据。
- 关键 API 响应摘要、request log 摘要、必要 UI 截图或 Playwright 断言。
- 测试容器清理命令和结果。

| 编号 | 覆盖问题 | 容器测试方式 | 账号要求 | 必须验证 |
| --- | --- | --- | --- | --- |
| CT-01 | 真实多账号 Codex runtime binding | 新 v0.1.6 容器 + embedded CPA + 真实 Codex 上游请求 | 需要至少 3 个真实 Codex CPA 账号；测试记录只写账号 label/id 摘要，不记录 auth file 内容或完整凭据 | 3 个真实账号都必须分别通过强制路由或自动路由发起普通模型请求，并核对各自 `request_logs.account_id`、`runtime_auth_id`、`runtime_auth_index` 稳定对应；其中至少 1 个账号连续 10 次请求仍稳定使用同一 pinned runtime auth；强制路由和自动路由都使用 selected account 的 pinned auth；本轮已通过 |
| CT-02 | pinning/fail-closed 负向路径 | 新容器 + fake CPA management/provider，模拟支持 pinning、不支持 pinning、auth metadata 缺失、round-robin 默认选择 | fake account 即可 | 缺失 binding、不支持 pinning 或 metadata 未就绪时普通流量和强制账号路由 fail closed；request log 不把未确认 binding 计入可信账号 usage |
| CT-03 | quota/subscription/serving/credential 状态隔离 | 新容器 + fake accounts + mock upstream/mock CPA | fake account 即可 | quota `401/403` 不写 `needs_login`；quota blocked 跳过；subscription 非 active 跳过；模型 `200` 不清除 quota/subscription 阻断；明确上游错误才进入 serving cooldown |
| CT-04 | 管理员诊断入口 | 新容器 + fake accounts + mock upstream/mock CPA | fake account 即可 | `diagnostic=true` 可以绕过 quota/subscription/serving cooldown 强测；不能绕过凭据硬失败或 runtime binding 缺失；不更新普通路由健康，也不计入普通 usage |
| CT-05 | Streaming 标记与 Activity 记账 | 新容器 + mock SSE upstream / mock CPA SSE | fake account 即可 | 缺 `[DONE]`、缺 `response.completed`、`response.failed`、CPA stream 500 JSON error、SSE 读错和 downstream write error 都记录失败与安全错误摘要；stream bytes 写出后不 retry |
| CT-06 | 长输出 stream 中断不惩罚账号 | 新容器 + mock SSE upstream，构造长输出、gateway timeout、客户端断开 | fake account 即可 | 没有明确上游 `5xx`、capacity、rate limit、quota、auth 错误证据时，只记录 stream incomplete/timeout，不进入 serving cooldown；有明确上游错误时后续独立请求绕开 cooldown 账号 |
| CT-07 | CPA 删除、从 Pool 移除、重登与 reload | 新容器 + fake cpa-auth 文件 + fake/mock CPA management；如需真实 Device Code 重登另列手工记录 | 默认 fake account；真实 Device Code 重登可选且需要用户亲自操作 | 从 Pool 移除不删账号、不删 auth file；删除 CPA 账号删除 DB row、Pool membership 和磁盘 auth file；触发 reload signal；历史 Activity/usage/request log 保留；重新导入同 key 不与旧文件冲突 |
| CT-08 | management API / provider endpoint / reload signal | 新容器 + fake CPA management/provider 或可控 embedded CPA 测试配置 | fake account 即可；真实 provider 上游消费由 CT-01 覆盖 | management auth-files metadata 可读；provider endpoint 可带 pinned auth headers；reload signal 后 CPA 子进程或 metadata 状态刷新；失败时 readiness 或 API 给出明确错误 |
| CT-09 | CPA 子进程异常退出与 `/readyz` | 新容器，注入 CPA 子进程退出或阻断 management API | fake account 即可 | CPA 子进程异常退出会让容器失败或 `/readyz` not ready；readiness 响应包含 CPA 不可用原因；不会继续宣称可接普通 CPA 流量 |
| CT-10 | Docker Compose 运行配置 | 用待发布 Compose 启动新容器 | 不需要账号 | 镜像 tag/digest 固定；healthcheck、restart、stop grace period、端口绑定、volume、json-file log rotation 与文档一致；停止容器时能正常退出并清理 |
| CT-11 | 错误信息安全截断 | 新容器 + mock upstream 返回超长错误、伪 token、伪 auth header、伪 prompt/body | fake account 即可 | `request_logs.error_message` 最大 4KB；敏感字段被移除；Activity/API 只展示安全截断摘要和 normalized reason |
| CT-12 | 重复错误折叠、stdout 降噪、通知去重 | 新容器 + mock upstream 连续返回同一错误 1,000 次；通知去重用 store 单测覆盖 | fake account 即可 | SQLite/request log 或聚合表不线性写入 1,000 条等价错误；stdout 不被同类错误刷屏；`failed/dropped` 通知按窗口去重由 `internal/store/notifications_test.go` 覆盖 |
| CT-13 | Activity/Usage API 上限和索引 | 新容器，seed 大量 request_logs/usage 数据 | fake account 即可 | API 强制 `page_size` / `limit` 上限；常用过滤条件走索引或可接受查询计划；前端默认时间窗口和分页不拖垮实例 |
| CT-14 | 诊断页和账号诊断 tab | 新容器 + fake DB/WAL、fake runtime 状态、mock readiness failure | fake account 即可 | 诊断页展示 DB/WAL 大小、日志策略、image pinned CPA version、实际 running CPA version、readiness 失败原因；账号诊断 tab 展示 Runtime Binding、credential、quota、subscription、serving 五维状态摘要和建议操作 |

## 01. CPA 账号身份与 Runtime Credential 绑定错位

本轮已完成：runtime binding 接入普通转发、pinning headers、缺失 binding fail closed、request log runtime identity、Activity/usage 可信统计、相关单元测试、容器 smoke test 和 `CT-01` 真实多账号 Codex 上游验证。

- 同一个 CPA runtime 中导入 3 个 Codex auth file 后，3 个真实账号都必须分别通过强制路由或自动路由发起普通模型请求，并核对各自 `request_logs.account_id`、`runtime_auth_id`、`runtime_auth_index` 稳定对应；其中至少 1 个账号连续发起 10 次普通模型请求时，Lune 必须全部携带该账号对应的 pinned runtime auth。
- 强制路由 `X-Lune-Account-Id` 指向某个 CPA account 时，Lune 必须携带该 account 对应的 pinned runtime auth；否则请求 fail closed。
- 普通 Pool 自动路由选择第 N 个 CPA account 时，`request_logs.account_id` 与 `runtime_auth_id/runtime_auth_index` 能够一一对应。
- 当 CPA runtime 不支持 credential pinning 或 auth metadata 未就绪时，该账号不可接普通流量，错误 reason 明确。
- Activity 页面账号请求量基于已确认 runtime credential 归属；无法确认时显示不确定状态。
- 单元测试覆盖：CPA target 构建必须携带 runtime credential binding；缺失 binding 时 fail closed；日志同时保存 Lune account id 与 runtime auth id。
- 集成测试覆盖：模拟 CPA round-robin runtime，验证 Lune pinning 后不会被 CPA 默认 round-robin 打散。

## 02. 账号状态互相污染导致错误路由和错误 UI

本轮已完成：subscription 非 active 阻断、`auth_suspect` 可路由但降权、状态大小写口径对齐、Pool 可信 usage 统计、管理员 diagnostic request 和相关 router/store/gateway/admin 测试；`CT-03` / `CT-04` 已通过 `lune-v016-ct0206` fake 容器代表性验证。

- quota `401/403` 且模型调用仍成功时，UI 显示“额度查询失败”，不显示“需要重新登录”。
- quota `allowed=false` 或 `limit_reached=true` 时，普通路由跳过该账号。
- subscription 接口 `401/403` 只写 `subscription_status=error`，不单独写 `credential_status=needs_login`。
- `subscription_status=expired/free/pending/error/unknown` 时，普通路由跳过该账号。
- 生成请求 `200` 不会清除或改写任何 quota/subscription 状态。
- 明确上游错误导致的生成失败进入 `serving_status=cooldown`，后续独立请求在冷却期内绕过该账号；stream 长输出、gateway timeout 或客户端断开在没有明确上游错误证据时只记录失败，不惩罚账号。
- 生成请求 `401/403` 只有确认是账号上游凭据问题时，才写 `credential_status=needs_login`。
- `auth_suspect` 默认可路由但降权，优先选择其他 `credential_status=ok` 账号。
- `credential_status=needs_login/refresh_failed/runtime_pending/runtime_error/unknown` 时，普通路由跳过该账号。
- CPA runtime credential binding 未确认时，不能把模型调用结果用于更新某个 CPA account 的健康状态。
- 强制账号路由不能绕过 `credential_status=needs_login/refresh_failed/runtime_pending/runtime_error/unknown`、`quota_status=blocked`、`subscription_status=expired/free/pending/error/unknown`、`serving_status=cooldown` 等不可接普通流量状态。
- 管理员诊断请求如需绕过普通路由限制，必须走单独入口并标记 `diagnostic=true`；可绕过 quota/subscription/serving cooldown 来强测一次，但不得绕过凭据和 runtime binding 硬失败，也不得更新普通路由健康或普通 usage 统计。
- Pool 详情页账号卡片最多展示请求量、订阅、主问题三个 chip；订阅异常只占订阅槽位，主问题 chip 最多一个。
- 右上角 badge 使用严重程度色并只表达 `正常/降级/异常/待检查/已停用` 的组合路由摘要。
- 账号详情抽屉使用 `Overview / Playground / 诊断` 三个 tab。
- Active Pool 卡片在常见 chip 组合下高度稳定。
- 容器验收覆盖：用新 v0.1.6 镜像、全新数据目录和可控 mock upstream / mock CPA 响应，验证 quota/subscription/serving/credential 状态互不串写、路由按组合状态跳过不可用账号、UI/API 展示具体原因；测试容器不得影响旧版本容器，用完必须删除。`lune-v016-ct0206` 已验证自动路由跳过明确 seed 的 quota blocked、subscription expired、serving cooldown 和 pinning unsupported 账号，diagnostic request 可绕过 cooldown 但不修复普通健康、不进入普通 Usage。

## 03. Streaming 失败与 Activity 记账不准确

本轮已完成：streaming outcome、retryable health impact、Activity flow 和 confirmed binding 统计口径已纳入回归测试；`CT-05` / `CT-06` 已通过 `lune-v016-ct0206` fake 容器代表性验证。

- Chat Completions stream 缺少 `[DONE]`，Activity 记录失败。
- Responses stream 缺少 `response.completed`，Activity 记录失败。
- Responses stream 发出 `response.failed`，Activity 记录失败并保留 upstream status。
- Responses stream 发出容量错误，Activity 保留 upstream message。
- Responses stream 发出 `[DONE]` 但没有 `response.completed`，仍记录失败。
- CPA stream 返回 HTTP 500 且 body 为 JSON error，Activity 记录提取后的上游 message。
- CPA stream 第一 Pool account 返回 HTTP 500 后，后续独立请求能避开该账号。
- streaming 明确上游错误即使发生在单次或最终 attempt，也按错误类型记录 account health impact。
- SSE line 超过旧 1MB scanner limit，Activity 记录 stream read error。
- downstream write 失败后，Activity 记录失败；没有明确上游错误证据时不惩罚账号。
- 长流超过 gateway timeout，Activity 记录 timeout 相关失败；默认不惩罚账号，避免把“大输出被截断”误判为账号坏。
- stream bytes 已写出后不 retry；request log 必须记录最终 stream outcome，只有明确上游错误才更新 serving cooldown。
- Activity flow 展示 `Pool -> Account -> Model`。
- failed routed requests 默认纳入 flow。
- 没有 selected account 的 rows 被排除或归入明确 `Unrouted` 语义。
- CPA runtime binding 未确认时，Activity 不把账号级统计表述成精确实际消耗。
- 容器验收覆盖：用新 v0.1.6 镜像、全新数据目录和可控 mock upstream / mock CPA SSE 响应，验证缺 completion marker、semantic failure、EOF/timeout、CPA stream 500 JSON error、stream 已写出后不 retry、无明确上游错误的长输出中断不惩罚账号、明确上游错误后续请求绕开 cooldown 账号、Activity Flow 和可信 usage 统计；测试容器不得影响旧版本容器，用完必须删除。`lune-v016-ct0206` 已验证 stream 缺 `[DONE]`、stream `500` JSON error、stream `response.failed` health impact，以及后续独立请求绕开 cooldown 账号。

## 04. CPA 凭据生命周期、重复账号、删除与重登

本轮已完成：普通模型转发复用 runtime auth metadata，request log 可审计 runtime auth id/index；删除 CPA 账号同步删除 auth file 并触发 embedded CPA reload signal；request log 保存删除前账号 label snapshot；容器 fake 删除、Pool 移除、历史保留和同 key 重新导入验证已通过。

- Device Code 登录同一个已存在 CPA account key 时，更新现有账号，不创建重复账号。
- 从 Pool 移除只解除 Pool membership，不删除账号记录，也不删除磁盘 auth file。
- 删除 CPA 账号会同步删除 auth file 并触发 embedded CPA runtime reload；重新登录同一个 account key 后，runtime 使用新 token。
- 重新登录后的首次 quota refresh 不继续使用旧 auth index。
- 同名 auth file 被覆盖后，`resolveAuthMetadata` 能检测 runtime metadata 与磁盘 metadata 不一致并触发 reload。
- `syncCpaMetadata` 不会用旧 `last_refresh` 覆盖更新后的 DB 登录状态。
- batch import 遇到已存在 CPA account key 时跳过或更新，不创建重复账号。
- quota HTTP 401 不会在普通模型请求仍健康时单独把账号展示为“需要重新登录”。
- 重复 CPA account key 出现在数据库中时，启动检查能给出明确诊断。
- 删除账号时，用户能明确知道 auth file 会被同步删除；runtime reload 后不再持有旧 auth entry；历史 Activity、usage 和 request log 保留。

## 05. 部署、日志、Health、诊断能力不足

本轮已完成：内置 CPA 固定为 `v7.0.2-lune.1` 源码构建，Dockerfile/release build args 同步，entrypoint 监督 CPA 子进程，Compose health/logging/stop grace 补齐，错误截断/折叠/API 上限与诊断字段落地，并通过 fake 容器 smoke test；failed/dropped 通知去重由 `internal/store/notifications_test.go` 覆盖。

- 已完成：Docker 部署默认有 healthcheck、日志轮转、合理停止宽限期，并且运行配置与文档一致。
- 已完成：Dockerfile 和 release workflow 固定内置 CPA 为 `v7.0.2-lune.1`；容器构建已通过，`LUNE_EMBEDDED_CPA_VERSION` 与补丁版本一致。
- 已完成：entrypoint 生成配置、health/API 空状态、management auth-files metadata、provider endpoint pinned headers 和 reload signal 均已通过 fake 容器验证。
- 已完成：CPA 子进程异常退出会触发 entrypoint 停止 Lune 并让容器失败；`/readyz` 已增强并完成 disabled/missing service 容器验证。
- 已完成：同一账号同一错误重复时，SQLite request log 和 stdout 不线性写入等价错误。
- 已完成：failed/dropped 通知按 dedup key 和时间窗口去重；该子项由 `go test ./internal/store` 覆盖。
- 已完成：错误诊断避免记录凭据、完整 prompt 和完整 request body，默认安全截断。
- 已完成：Activity/Usage API 强制 `page_size` / `limit` 上限，且大数据量下仍能返回受限分页结果。
- 已完成：诊断页能展示 DB/WAL/SHM 大小、image pinned CPA version、running CPA version、readiness 状态和 readiness 失败原因。
