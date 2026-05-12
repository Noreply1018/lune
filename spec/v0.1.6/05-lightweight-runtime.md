# 05. 轻量 VPS 运行时

## 目标

让小型长期运行 VPS 成为 v0.1.6 的一等部署目标。

目标机器：

- 2 核 2GB RAM：应支持 native `lune + embedded CPA` 和 low-resource profile。
- 2 核 4GB RAM：native 或 Docker 都应较舒适。

优化方向不是重写技术栈，而是降低默认运行开销：

- 保持 Go 单二进制服务。
- 保持 Vite/React frontend build-time assets 嵌入 Go binary。
- 保持 SQLite。
- 保持 embedded CPA 默认可用。
- 聚焦 runtime defaults、部署形态、health check、日志、内存和 SQLite 压力。

## 问题根源

小 VPS 上的主要压力来自：

- Docker daemon/container 额外常驻开销。
- 过于频繁或并发过高的后台 health check。
- 请求日志逐条写 SQLite。
- 非流式 upstream response 全量读入内存。
- request body replay 默认内存阈值偏高。
- stdout access log 和重复错误日志放大磁盘压力。
- SQLite 连接池默认可能高于单进程小库需求。

## 解决的问题

- 提供不依赖 Docker 的 native systemd 推荐路径。
- 提供 low-resource profile，降低默认后台开销。
- 降低 SQLite idle connection 和并发 writer 压力。
- 降低 request/response 体造成的内存峰值。
- 保留 Docker 便利性，但明确它不是 2C2G 上最低开销路径。
- 给用户明确的 Go runtime 内存建议。

## 技术栈边界

v0.1.6 不做：

- 不引入 Node/Python/Java 生产运行依赖。
- 不把 frontend runtime 单独跑在 Node。
- 不把 SQLite 换成外部数据库。
- 不做 CPA on-demand suspend/resume。
- 不做 managed CPA binary update。

v0.1.6 保持：

- Go + SQLite。
- embedded frontend assets。
- embedded CPA default。
- Docker 支持。
- native binary/systemd 作为小 VPS 推荐路径。

## 部署模式

### Native systemd，2C2G 推荐

推荐布局：

- `/usr/local/bin/lune`
- `/usr/local/lib/lune/CLIProxyAPI`
- `/etc/lune/lune.env`
- `/var/lib/lune/lune.db`
- `/var/lib/lune/cpa-auth/`
- `/var/lib/lune/tmp/`
- `lune.service`

native bundle 不应要求目标服务器安装 Go、Node、npm 或 Docker。

### Docker Compose，保留便利路径

Docker 继续支持：

- 适合已经运行 Docker 的 VPS。
- 适合用户偏好单 compose workflow。
- 在小 VPS 文档中定位为便利路径，不是最低开销路径。

如果 VPS 专用于 Lune，native deployment 避免 Docker daemon/container overhead。

## Release artifacts

至少提供：

- `lune-linux-amd64`
- `lune-linux-arm64`
- `lune-bundle-linux-amd64.tar.gz`
- `lune-bundle-linux-arm64.tar.gz`

bundle 内容：

- `bin/lune`
- `bin/CLIProxyAPI`
- example `lune.env`
- example `lune.service`
- native systemd install script

install script 应：

- 创建所需目录。
- 安装 binary。
- config 缺失时写 example。
- 安装 service file。
- 不自动生成或写入用户 secrets；让用户显式配置。

## Low-resource profile

新增：

```env
LUNE_RESOURCE_PROFILE=low
```

启用后，默认值偏向较低 steady-state overhead 和较低内存峰值。

初始默认建议：

- `LUNE_LOG_LEVEL=warn`，除非用户显式设置。
- `health_check_interval=300s` 或 `600s`。
- `max_retry_attempts=2`。
- `gateway_memory_body_mb=1` 或 `2`。
- `gateway_max_body_mb=20` 到 `32`。
- `data_retention_days=7` 到 `14`。
- embedded CPA 默认仍启用。

profile 只能改变默认值，不能覆盖用户在环境变量或数据库中的显式配置。

## Go runtime 建议

native 和 Docker 文档都建议小 VPS 显式设置：

```env
GOMEMLIMIT=256MiB
GOGC=100
```

4GB VPS 可建议：

```env
GOMEMLIMIT=384MiB
```

或：

```env
GOMEMLIMIT=512MiB
```

这些是部署建议，不应硬编码进 binary。

## SQLite 优化

小单进程部署应配置 SQLite handle：

- `SetMaxOpenConns(1)`
- `SetMaxIdleConns(1)`
- 保持 WAL。
- 保持 `busy_timeout`。
- 保持 foreign keys enabled。

目标是减少 idle connection overhead，并避免不必要的并发 SQLite writers。

## Health check 优化

low-resource profile 下 health check 必须更克制：

- 降低 health-check concurrency。
- 避免在获取 semaphore 前为每个 enabled account 创建 goroutine。
- 使用 bounded worker pool 或等价 fixed-concurrency 设计。
- low-resource 默认 concurrency 为 1 到 3 workers。
- 多 CPA 账号时，后台检查不能长期占用 idle CPU。
- CPA service health check 仍保留，但频率和并发不能让服务一直做后台工作。

## Request log 与 retention

请求日志对 Activity 有价值，但小 VPS 高流量下会增加 SQLite 写压力。

v0.1.6 最低要求：

- low-resource profile 使用更短 retention。
- 自动 pruning 保持启用。
- Settings 中展示 retention/prune 状态。

后续可考虑：

- 只记录失败请求。
- 关闭详细 Activity logs。
- async write queue 或 batching。

如果 v0.1.6 实现风险低，可提前做 batching；否则保留为后续优化。

## Request body 内存控制

request body replay 已支持 disk spillover。low-resource mode 应更早使用该路径：

- 降低默认 memory body threshold。
- 大 request replay 文件放在 `LUNE_GATEWAY_TMP_DIR`。
- startup 和 request completion 都清理 replay files。

## 大型非流式响应处理

### 问题根源

Lune 过去会把非流式 upstream response 全量读入内存，再写给客户端。

这有利于 retry 和 usage parsing，但对大型 non-stream responses 成本高，尤其是 `/v1/responses` 图像、文件类 workflow。

### 解决的问题

- 防止大型非流式响应造成内存峰值。
- 保留小响应的 usage parsing 和 retry 行为。
- 对大响应优先保护内存，不为了完整 body retry 牺牲稳定性。

### 设计规则

- 小型非流式响应继续缓冲到内存。
- 新增可配置 response buffer threshold。
- 超过阈值后，不持有完整 response body。
- 一旦确认不能安全 retry，直接流式写给客户端。
- 保留 Activity 所需的 bounded metadata。
- 小响应继续解析 usage。
- 大响应 usage 可能不可用，除非能从 bounded metadata 中安全解析。
- response-body streaming 与 request-body replay-to-disk 是两个不同问题，不能混淆。

### retry 语义

- upstream response headers 前的失败，retry 行为保持不变。
- response headers 或 body bytes 写给 downstream 后，不再 retry。
- 大型非流式响应跨过阈值后，内存保护优先于保留完整 body retry。

### 初始阈值

实现时再最终确认，初始建议：

- normal profile：8MB 到 16MB。
- low-resource profile：1MB 到 4MB。

v0.1.6 使用全局设置；per-route thresholds 作为后续增强。

## External CPA advanced mode

### 目标

保持 all-in-one embedded CPA 作为默认体验，同时提供清晰的 opt-in 外部 CPA 路径。

适用场景：

- 用户需要独立升级 CPA。
- 用户已有外部 CPA 部署。
- 用户需要自定义 CPA 配置。
- 用户需要独立调试 CPA。

### 规则

- default 仍为 embedded CPA。
- external CPA 必须 opt-in。
- external CPA 使用明确的 Lune environment variables。
- 文档必须标注为 advanced。
- quick start 不应优先展示 external CPA。
- Docker Compose 和 `.env.example` 应提供一致路径，不让用户从内部变量推断。
- Settings 应区分 `embedded` 与 `external` runtime mode。

### 建议配置形态

已有变量可作为基础：

```env
LUNE_EMBEDDED_CPA=0
LUNE_CPA_BASE_URL=
LUNE_CPA_API_KEY=
LUNE_CPA_MANAGEMENT_KEY=
```

文档必须明确回答：

- 如何禁用 embedded CPA。
- Lune 连接哪个 CPA base URL。
- provider requests 使用哪个 API key。
- CPA management operations 使用哪个 management key。

### minimum acceptance

- Docker 和 native bundle 默认 embedded CPA。
- external CPA 不需要 patch internal files 即可配置。
- Settings 连接外部 CPA 时显示 `external`。
- 测试覆盖 embedded vs external runtime-mode detection/config parsing。
- 如果多个 Lune CPA account 共用一个 external CPA runtime，而 runtime 不支持 per-request pinning，文档必须说明账号统计和额度归因不可信。

### non-goals

- 不恢复复杂旧双容器 quick start 作为默认路径。
- 不增加 migration wizard。
- 不做 CPA on-demand suspend/resume。
- 不做 managed CPA binary updates。

## Managed CPA update 非目标

managed CPA binary update 明确不属于 v0.1.6。

原因：

- 需要可信 release metadata。
- 需要 checksum 或 signature verification。
- 需要 rollback。
- 需要 update records。
- 需要 UI controls。
- 需要处理 container immutability。

v0.1.6 默认路径：

- Lune 升级时更新 embedded pinned CPA。
- external CPA 给高级用户提供独立升级路径。
- 不做 silent 或 automatic CPA binary replacement。

## Docker low-resource guidance

Docker 文档应包含：

```env
GOMEMLIMIT=256MiB
GOGC=100
LUNE_RESOURCE_PROFILE=low
LUNE_LOG_LEVEL=warn
```

Compose 可提供 advisory memory limit 示例：

```yaml
deploy:
  resources:
    limits:
      memory: 768M
```

该配置只是建议；服务不能依赖 Docker-specific resource controls 才能运行。

## 资源目标

这些是设计目标，不是保证值。v0.1.6 应在小 VPS 实测后更新。

| 模式 | idle memory target | active request target | idle CPU target |
| --- | ---: | ---: | --- |
| Native Lune only | 40-100MB | 80-200MB | Near 0 |
| Native Lune + CPA | 150-400MB | 250-700MB | Near 0 except checks |
| Docker Lune + CPA, no existing Docker daemon | 300-700MB | 400MB-1GB | Near 0 except checks |
| Docker Lune + CPA, existing Docker host | Native plus smaller marginal overhead | Similar to native plus overhead | Near 0 except checks |

## 验收标准

- native bundle 能在无 Docker 机器上安装 Lune，并支持 embedded CPA。
- `LUNE_RESOURCE_PROFILE=low` 改变默认值，但不覆盖用户显式配置。
- SQLite connection pool limits 已应用。
- low-resource profile 下 health-check concurrency 和 interval 更低。
- low-resource profile 下 request body memory threshold 更低。
- low-resource profile 下默认 log verbosity 更低。
- 文档明确 native/systemd 是 low-resource 推荐路径，Docker 是 convenience path。
- 文档包含 `GOMEMLIMIT`、`GOGC` 和内存目标。
- 小型非流式 response 保持 usage parsing。
- 大型非流式 response 不全量驻留内存。
- upstream failure before response headers 仍可 retry。
- downstream 已写出后不 retry，并准确记录 Activity。
