# 04. 轻量 VPS 运行时

状态：draft

来源：previous `spec/v0.1.6/05-lightweight-runtime.md`

## 目标

让小型长期运行 VPS 成为未来版本的一等部署目标。

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

未来版本不应为轻量 VPS 目标引入新的生产运行时依赖：

- 不引入 Node/Python/Java 生产运行依赖。
- 不把 frontend runtime 单独跑在 Node。
- 不把 SQLite 换成外部数据库。
- 不做 CPA on-demand suspend/resume。
- 不做 managed CPA binary update。

应保持：

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

最低要求：

- low-resource profile 使用更短 retention。
- 自动 pruning 保持启用。
- Settings 中展示 retention/prune 状态。

后续可考虑：

- 只记录失败请求。
- 关闭详细 Activity logs。
- async write queue 或 batching。

如果实现风险低，可提前做 batching；否则保留为后续优化。

## Request body 内存控制

request body replay 已支持 disk spillover。low-resource mode 应更早使用该路径：

- 降低默认 memory body threshold。
- 大 request replay 文件放在 `LUNE_GATEWAY_TMP_DIR`。
- startup 和 request completion 都清理 replay files。

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

这些是设计目标，不是保证值。进入正式版本规格前应在小 VPS 实测后更新。

| 模式 | idle memory target | active request target | idle CPU target |
| --- | ---: | ---: | --- |
| 仅 Native Lune | 40-100MB | 80-200MB | 接近 0 |
| Native Lune + CPA | 150-400MB | 250-700MB | 除检查外接近 0 |
| Docker Lune + CPA，机器未运行 Docker daemon | 300-700MB | 400MB-1GB | 除检查外接近 0 |
| Docker Lune + CPA，已有 Docker host | Native 加较小边际开销 | 接近 native 加容器开销 | 除检查外接近 0 |

## 草案验收

- native bundle 能在无 Docker 机器上安装 Lune，并支持 embedded CPA。
- `LUNE_RESOURCE_PROFILE=low` 改变默认值，但不覆盖用户显式配置。
- SQLite connection pool limits 已应用。
- low-resource profile 下 health-check concurrency 和 interval 更低。
- low-resource profile 下 request body memory threshold 更低。
- low-resource profile 下默认 log verbosity 更低。
- 文档明确 native/systemd 是 low-resource 推荐路径，Docker 是 convenience path。
- 文档包含 `GOMEMLIMIT`、`GOGC` 和内存目标。
