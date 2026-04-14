# Lune v3 Spec — Draft

> 基于 v2 全面审计 + 行业调研形成的演进方向。不是需求文档，是思考素材。
> 每个章节独立，可挑可弃。

---

## 目录

1. [v2 现状评估](#1-v2-现状评估)
2. [网关核心升级](#2-网关核心升级)
3. [可观测性体系](#3-可观测性体系)
4. [前端体验重塑](#4-前端体验重塑)
5. [工程规范补债](#5-工程规范补债)
6. [新功能设想](#6-新功能设想)
7. [不做什么](#7-不做什么)
8. [里程碑建议](#8-里程碑建议)

---

## 1. v2 现状评估

### 做得好的

| 维度 | 评价 |
|------|------|
| 包结构 | 清晰的单向依赖图，internal/ 下 12 个包各司其职 |
| 路由算法 | priority-weighted 选择 + exclude list 重试，设计精良 |
| 缓存层 | RWMutex + atomic version 的 double-check locking，无锁读路径 |
| 部署形态 | 单二进制 + 内嵌前端 + SQLite，零外部依赖，docker 一行启动 |
| CPA 集成 | 设备码登录 → JWT 解析 → 凭据文件管理 → 远程账号扫描导入，完整闭环 |
| 前端类型安全 | TypeScript strict，types.ts 覆盖所有 API 实体 |

### 需要改进的

| 维度 | 现状 | 影响 |
|------|------|------|
| 测试覆盖 | 0 个 test 文件 | 重构无信心，回归无感知 |
| 日志系统 | 标准库 `log`，无级别无结构 | 线上排障极难 |
| 速率限制 | 无 | 单个 token 可打满上游 |
| 重试策略 | 线性立即重试，无退避 | 雪崩风险 |
| goroutine 管理 | 健康检查无并发上限 | 账号多时资源爆炸 |
| 异步错误 | `go func() { _ = store.InsertLog(log) }()` | 日志静默丢失 |
| 前端状态管理 | AccountsPage 25+ 个 useState | 代码膨胀，难维护 |
| 表单验证 | 仅 HTML5 required | 无字段级反馈 |
| 配置校验 | YAML 解析错误被 `_ =` 吞掉 | 配错无提示 |

---

## 2. 网关核心升级

### 2.1 速率限制

v2 的 access_token 只有 token 总量配额（`quota_tokens`），没有速率维度。

**方案：三层限流**

```
Layer 1: 全局限流（保护 Lune 自身）
  - 总 RPM / TPM 上限

Layer 2: per-token 限流
  - RPM（requests per minute）
  - TPM（tokens per minute）
  - 每日 token 预算上限

Layer 3: per-model 限流（可选）
  - 按模型设置 RPM/TPM 上限
  - 防止单一模型吃满所有配额
```

**实现思路：**
- 令牌桶算法（token bucket），滑动窗口计数
- 限流元数据存 SQLite（`token_rate_limits` 表），运行时计数器在内存
- 超限返回标准 `429 Too Many Requests` + `Retry-After` header
- access_tokens 表新增：`rpm_limit`、`tpm_limit`、`daily_token_budget`

**参考：** LiteLLM 的 per-user tpm_limit / rpm_limit 方案，Envoy AI Gateway 的 token-based rate limiting。

### 2.2 指数退避重试

v2 重试是立即线性重试，上游抖动时会加剧压力。

```go
// v2: 立即重试
for attempt := 0; attempt < maxRetries; attempt++ {
    // 直接重试，无间隔
}

// v3: 指数退避 + jitter
for attempt := 0; attempt < maxRetries; attempt++ {
    if attempt > 0 {
        base := time.Duration(1<<(attempt-1)) * 200 * time.Millisecond // 200ms, 400ms, 800ms
        jitter := time.Duration(rand.Int64N(int64(base / 2)))
        time.Sleep(base + jitter)
    }
    // ...
}
```

- 初始间隔 200ms，倍增到 ~1s，加随机 jitter 防止惊群
- 对 `429` 优先读 `Retry-After` header

### 2.3 熔断器（Circuit Breaker）

健康检查是 60 秒一次的粗粒度探活。v3 在路由层加轻量熔断：

```
状态机：closed → open → half-open → closed

触发条件：
  - 连续 N 次请求失败 → open（停止向该账号发送请求）
  - open 持续 M 秒 → half-open（允许一个试探请求）
  - 试探成功 → closed（恢复正常）
  - 试探失败 → 重回 open

与健康检查协作：
  - 健康检查成功 → 强制 reset 为 closed
  - 熔断状态反馈到 SelectAccount()，降低选中概率
```

这样在两次健康检查之间（60s 窗口），实际流量也能快速感知故障。

### 2.4 请求超时精细化

v2 只有一个 `request_timeout`（默认 120s）。流式长对话和短请求共用一个超时不合理。

```yaml
# lune.yaml v3
timeouts:
  connect: 5s          # TCP 连接超时
  read_header: 10s     # 等待上游响应头
  streaming: 300s      # 流式传输总时长
  non_streaming: 120s  # 非流式请求总时长
  idle: 90s            # HTTP keep-alive 空闲超时
```

### 2.5 Fallback Chain

v2 的重试是同一个 pool 内换账号。v3 支持跨 pool 降级：

```
Route: gpt-4o
  Primary Pool: premium-pool (高优先级账号)
  Fallback Pool: backup-pool (低成本账号)

流程：
  1. premium-pool 重试 N 次全部失败
  2. 自动降级到 backup-pool
  3. backup-pool 也失败 → 返回 502
```

**数据模型：** `model_routes` 新增 `fallback_pool_id` 字段。

### 2.6 请求/响应钩子

为高级用户提供请求生命周期钩子，不需要改 Lune 代码也能扩展：

```yaml
hooks:
  pre_request:
    - url: "http://localhost:9090/guardrails"   # 内容审核
      timeout: 3s
  post_response:
    - url: "http://localhost:9090/log-collector" # 外部日志收集
      timeout: 2s
      async: true                                # 异步，不阻塞响应
```

每个钩子是一个 HTTP 回调。pre_request 返回非 200 可以拦截请求，post_response 纯通知。

---

## 3. 可观测性体系

### 3.1 结构化日志

从 `log.Printf` 迁移到 Go 1.21+ 的 `log/slog`：

```go
// v2
logger.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))

// v3
slog.Info("request completed",
    "method", r.Method,
    "path", r.URL.Path,
    "duration_ms", time.Since(start).Milliseconds(),
    "status", statusCode,
    "request_id", requestID,
    "token_name", tokenName,
    "model", model,
    "input_tokens", usage.InputTokens,
    "output_tokens", usage.OutputTokens,
)
```

**配置：**
```yaml
logging:
  format: json    # json | text
  level: info     # debug | info | warn | error
```

- `json` 格式适合 Docker / Loki / ELK 采集
- `text` 格式适合本地开发
- 异步操作的错误必须记日志，不再 `_ = err`

### 3.2 Metrics 端点

暴露 Prometheus 兼容的 `/metrics` 端点：

```
# 请求维度
lune_gateway_requests_total{model, token_name, status_code, source_kind}
lune_gateway_request_duration_seconds{model, token_name, quantile}
lune_gateway_retries_total{model, reason}

# Token 维度
lune_gateway_input_tokens_total{model, token_name}
lune_gateway_output_tokens_total{model, token_name}

# 账号维度
lune_accounts_health{account_id, status}  # gauge: 1=healthy, 0=error
lune_accounts_total{source_kind, status}

# 速率限制
lune_ratelimit_rejected_total{token_name, reason}
```

**实现：** 用 Go 标准库 `expvar` 或轻量的 `prometheus/client_golang`。不引入重框架。

### 3.3 请求追踪增强

v2 已生成 `X-Lune-Request-Id`。v3 扩展追踪链：

```
请求头注入：
  X-Lune-Request-Id: req_xxxxxxxx    # Lune 生成
  X-Lune-Account: account-label       # 选中的账号（已有）
  X-Lune-Pool: pool-label             # 选中的池
  X-Lune-Route: model-alias           # 匹配的路由
  X-Lune-Attempt: 1                   # 第几次尝试
  X-Lune-Latency-Ms: 1234            # 总延迟

响应日志增强：
  - 记录每次重试的账号、状态码、延迟
  - 记录 TTFB（首字节时间）
```

### 3.4 成本估算

v2 的 UsagePage 只展示原始 token 数。v3 引入成本估算：

```sql
CREATE TABLE model_pricing (
    model_pattern TEXT PRIMARY KEY,   -- 支持通配符: "gpt-4o*", "claude-3.5-*"
    input_price_per_mtok  REAL,       -- $/1M input tokens
    output_price_per_mtok REAL,       -- $/1M output tokens
    updated_at TEXT
);
```

- Admin UI 提供内置价格表（主流模型预填充），支持用户自定义
- Usage 页面展示：总成本、per-token 成本、per-model 成本趋势图
- 可选：成本预警阈值（日/月），触发时通过 webhook 通知

---

## 4. 前端体验重塑

### 4.1 数据获取层重构

v2 每个页面手写 `useEffect` + `useState` + `fetch` + `catch`，加载/错误/刷新逻辑大量重复。

**引入 SWR：**

```tsx
// v2: 手动管理（每个页面 10-20 行样板代码）
const [accounts, setAccounts] = useState<Account[]>([]);
const [loading, setLoading] = useState(true);
useEffect(() => {
  api.get<Account[]>("/accounts")
    .then(setAccounts)
    .catch(() => toast("Failed", "error"))
    .finally(() => setLoading(false));
}, []);

// v3: 声明式
const { data: accounts, isLoading, error, mutate } = useSWR("/accounts", fetcher);
// 自动缓存、自动重验证、自动错误重试、页面切换回来自动刷新
```

**为什么选 SWR 而非 TanStack Query：**
- 项目不复杂，SWR 更轻量（~4KB gzip vs ~13KB）
- 内置 `useSWRMutation` 足够处理所有 CRUD 操作
- 与项目"最小依赖"哲学一致

**收益：**
- 消除每个页面 10-20 行重复的数据获取样板代码
- 页面切换时数据缓存，导航秒开
- 窗口聚焦自动刷新（`revalidateOnFocus`）
- 乐观更新支持（删除/启用/禁用操作即时反馈）

### 4.2 状态管理治理

AccountsPage 有 25+ 个 useState，是典型的"状态爆炸"。

**策略：将复杂页面拆分为子组件 + useReducer**

```
AccountsPage/
  index.tsx           -- 容器：数据获取、全局操作
  AccountTable.tsx    -- 列表展示 + 操作按钮
  AccountForm.tsx     -- 创建/编辑表单（独立状态）
  CpaImportDialog.tsx -- CPA 导入弹窗（独立状态）
  SourcePicker.tsx    -- 来源选择
```

每个子组件管理自己的局部状态（表单值、弹窗开关），父组件只持有列表数据和全局操作。

### 4.3 表单体验

v2 的表单验证只有 HTML5 `required`，没有字段级错误反馈。

**不引入表单库，用轻量自定义方案：**

```tsx
// 自定义 useForm hook
function useForm<T>(initial: T, validate: (v: T) => Partial<Record<keyof T, string>>) {
  const [values, setValues] = useState(initial);
  const [errors, setErrors] = useState<Partial<Record<keyof T, string>>>({});
  const [touched, setTouched] = useState<Partial<Record<keyof T, boolean>>>({});

  function handleChange(field: keyof T, value: T[keyof T]) {
    setValues(prev => ({ ...prev, [field]: value }));
    setTouched(prev => ({ ...prev, [field]: true }));
  }

  function handleSubmit(onSubmit: (v: T) => Promise<void>) {
    return async (e: FormEvent) => {
      e.preventDefault();
      const errs = validate(values);
      setErrors(errs);
      if (Object.keys(errs).length === 0) await onSubmit(values);
    };
  }

  return { values, errors, touched, handleChange, handleSubmit, reset: () => setValues(initial) };
}
```

- 字段失焦时显示校验错误（inline error）
- 提交时全量校验
- URL 格式、模型名格式、数值范围等自定义规则
- 保持零外部依赖

### 4.4 Error Boundary

v2 没有全局错误边界。页面组件 render 报错会白屏。

```tsx
// 全局 Error Boundary
function ErrorFallback({ error, reset }: { error: Error; reset: () => void }) {
  return (
    <div className="flex flex-col items-center justify-center gap-4 p-12">
      <h2 className="text-lg font-semibold">Something went wrong</h2>
      <pre className="max-w-lg rounded bg-muted p-4 text-sm">{error.message}</pre>
      <Button onClick={reset}>Try Again</Button>
    </div>
  );
}
```

包裹位置：App.tsx 的 `<Shell>` 内部，每个页面级别。

### 4.5 全局 API 错误拦截

v2 的 `api.ts` 只是抛异常，没有全局拦截。

```tsx
// v3: 统一拦截
async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(...);

  if (res.status === 401) {
    // Admin token 失效，展示重新认证提示
    showAuthDialog();
    throw new ApiError(401, "Unauthorized");
  }

  if (res.status === 429) {
    // 速率限制，展示友好提示 + Retry-After
    const retryAfter = res.headers.get("Retry-After");
    throw new ApiError(429, `Rate limited. Retry after ${retryAfter}s`);
  }

  // ...
}
```

### 4.6 UI/UX 细节增强

**空状态设计：**
```
当列表为空时，不是空白，而是引导性提示：
  "No accounts yet. Add your first LLM provider to get started."
  [+ Add Account]
```

**操作确认优化：**
- 删除操作：确认弹窗 + 输入资源名称确认（防误删）
- 批量操作：支持多选 + 批量启用/禁用/删除

**键盘导航：**
- 表格支持 ↑↓ 键导航
- `Cmd+K` 全局搜索（跨页面搜索账号、路由、token）

**实时状态指示器：**
- 侧边栏底部展示网关实时状态：当前 RPS、活跃连接数
- 健康检查结果实时推送（WebSocket 或 SSE）

### 4.7 Playground 增强

v2 的 Playground 是基础聊天界面。v3 增强：

```
功能增强：
  1. 多轮对话历史持久化（localStorage）
  2. 预设 Prompt 模板（System Prompt 编辑器）
  3. 模型参数调节面板（temperature、max_tokens、top_p）
  4. 响应对比模式：同一 prompt 发给两个模型，side-by-side 对比
  5. 导出对话为 JSON / Markdown
  6. 展示 token 用量和估算成本
  7. 支持发送图片（vision 模型）
```

### 4.8 暗色主题完善

v2 已有 `next-themes` 但部分组件可能未适配。全面检查：
- 所有硬编码颜色替换为 CSS 变量
- 图表配色暗色适配
- Skeleton 在暗色模式下的对比度

---

## 5. 工程规范补债

### 5.1 测试策略

v2 有 0 个测试文件。这是 v3 最重要的工程债务。

**分层测试：**

```
单元测试（优先）：
  internal/router/router_test.go
    - TestResolve_ExactMatch
    - TestResolve_DefaultPool
    - TestResolve_NoRoute
    - TestSelectAccount_PriorityOrdering
    - TestSelectAccount_WeightedDistribution
    - TestSelectAccount_ExcludeList
    - TestSelectAccount_UnhealthyFiltering

  internal/gateway/usage_test.go
    - TestParseUsageFromBody
    - TestParseUsageFromSSEChunk

  internal/gateway/proxy_test.go
    - TestIsRetryable
    - TestIsRetryableStatus
    - TestHopByHopHeaders

  internal/store/cache_test.go
    - TestRoutingCache_ConcurrentAccess
    - TestRoutingCache_Invalidation

  internal/cpa/jwt_test.go
    - TestParseAccountInfo

集成测试：
  internal/store/store_test.go
    - 完整 CRUD 流程（用临时 SQLite）
    - 迁移正确性

  internal/gateway/handler_test.go
    - httptest.Server 模拟上游
    - 重试行为验证
    - 流式转发验证

端到端：
  先不做，手动验证即可
```

**目标覆盖率：** router 和 gateway 核心路径 >80%。

### 5.2 CI/CD

```yaml
# .github/workflows/ci.yml
name: CI
on: [push, pull_request]
jobs:
  go:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: "1.25" }
      - run: go vet ./...
      - run: go test ./... -race -coverprofile=coverage.out
      - run: CGO_ENABLED=0 go build -o lune ./cmd/lune

  web:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with: { node-version: "22" }
      - run: cd web && npm ci && npm run build
```

### 5.3 接口抽象

v2 所有组件依赖具体类型，无法 mock 测试。

```go
// v3: 为核心依赖定义接口
type AccountStore interface {
    ListAccounts() ([]Account, error)
    GetAccount(id int64) (*Account, error)
    UpdateAccountHealth(id int64, status, lastError string) error
    // ...
}

type Router interface {
    Resolve(modelAlias string) (*ResolvedRoute, error)
    SelectAccount(poolID int64, targetModel string, exclude []int64) (*SelectedAccount, error)
}

type Cache interface {
    Get() *CacheSnapshot
    Invalidate()
}
```

Gateway handler 接受接口而非具体类型，测试时注入 mock。

### 5.4 配置校验

```go
func (cfg Config) Validate() error {
    var errs []string
    if cfg.Port < 1 || cfg.Port > 65535 {
        errs = append(errs, fmt.Sprintf("invalid port: %d", cfg.Port))
    }
    if cfg.DataDir == "" {
        errs = append(errs, "data_dir is required")
    }
    // ...
    if len(errs) > 0 {
        return fmt.Errorf("config validation failed:\n  %s", strings.Join(errs, "\n  "))
    }
    return nil
}
```

YAML 解析错误不再忽略，启动时校验失败直接退出并打印错误。

### 5.5 Goroutine 管理

健康检查并发加 semaphore：

```go
sem := make(chan struct{}, 10) // 最多 10 个并发检查
for _, acc := range accounts {
    wg.Add(1)
    go func(a store.Account) {
        defer wg.Done()
        sem <- struct{}{}
        defer func() { <-sem }()
        c.checkOne(ctx, a)
    }(acc)
}
```

异步日志写入加错误日志：

```go
go func() {
    if err := h.store.InsertLog(log); err != nil {
        slog.Error("failed to insert request log", "request_id", log.RequestID, "err", err)
    }
}()
```

### 5.6 数据库迁移事务化

v2 的迁移没有事务包裹：

```go
// v3: 每个迁移步骤包在事务里
for i := currentVersion; i < len(migrations); i++ {
    tx, err := s.db.Begin()
    if err != nil {
        return fmt.Errorf("begin migration %d: %w", i+1, err)
    }
    if _, err := tx.Exec(migrations[i]); err != nil {
        tx.Rollback()
        return fmt.Errorf("migration %d failed: %w", i+1, err)
    }
    if _, err := tx.Exec(
        `UPDATE system_config SET value = ? WHERE key = 'schema_version'`, i+1,
    ); err != nil {
        tx.Rollback()
        return fmt.Errorf("update schema version: %w", err)
    }
    if err := tx.Commit(); err != nil {
        return fmt.Errorf("commit migration %d: %w", i+1, err)
    }
}
```

---

## 6. 新功能设想

### 6.1 模型虚拟化

当前路由是 `alias → target_model` 一对一映射。v3 支持虚拟模型，一个 alias 背后可以是多个真实模型的策略组合：

```yaml
virtual_models:
  - alias: "best"
    strategy: "lowest-latency"      # 选择当前延迟最低的模型
    candidates:
      - model: "gpt-4o"
        pool: "openai-pool"
      - model: "claude-sonnet-4-20250514"
        pool: "anthropic-pool"

  - alias: "cheap"
    strategy: "lowest-cost"          # 选择最便宜的模型
    candidates:
      - model: "gpt-4o-mini"
        pool: "openai-pool"
      - model: "claude-haiku"
        pool: "anthropic-pool"
```

客户端只需要 `model: "best"`，Lune 自动选择当前最优的上游模型。

### 6.2 Webhook 通知

关键事件触发外部通知：

```yaml
webhooks:
  - url: "https://hooks.slack.com/xxx"
    events:
      - account.health_changed    # 账号健康状态变化
      - token.quota_exhausted     # Token 配额耗尽
      - daily_cost.threshold      # 日成本超过阈值
      - cpa.credential_expired    # CPA 凭据过期
```

也可以做简单的 Telegram / Discord bot 推送。

### 6.3 多用户与 RBAC

v2 是单 admin token。如果要给多人用，需要基础的访问控制：

```
角色：
  admin   -- 完全控制（当前行为）
  viewer  -- 只读 dashboard + usage
  user    -- 只能管理自己的 access token

实现方式：
  - admin_users 表：{ id, username, password_hash, role }
  - Session cookie 认证（替代当前的 Bearer token）
  - 保留 localhost 免认证（向后兼容）
```

**注：** 如果保持"个人自用"定位，这个可以不做。但如果有小团队共用场景，值得考虑。

### 6.4 配置导入

v2 有 `/admin/api/export` 但没有 import。v3 加配置导入：

```
POST /admin/api/import
{
  "accounts": [...],
  "pools": [...],
  "model_routes": [...],
  "access_tokens": [...]
}
```

- 基于 label 去重（已存在则跳过或更新）
- 支持 dry-run 模式（返回预览，不实际写入）
- 可用于跨实例迁移

### 6.5 API Playground 增强 — 多模型对比

这是一个对 LLM 网关特别有价值的功能：

```
用户输入一个 prompt → 同时发给 N 个模型
返回 side-by-side 对比：
  ┌─────────────────┬─────────────────┐
  │   gpt-4o        │  claude-sonnet  │
  │                 │                 │
  │  Response A...  │  Response B...  │
  │                 │                 │
  │  1.2s / 847tok  │  0.9s / 623tok  │
  │  ~$0.012        │  ~$0.008        │
  └─────────────────┴─────────────────┘
```

帮助用户直观评估不同模型的质量、速度和成本。

### 6.6 请求日志增强

v2 的 request_logs 记录基础信息。v3 增强：

```sql
-- 新增字段
ALTER TABLE request_logs ADD COLUMN ttfb_ms INTEGER;        -- 首字节延迟
ALTER TABLE request_logs ADD COLUMN retry_count INTEGER;     -- 重试次数
ALTER TABLE request_logs ADD COLUMN estimated_cost REAL;     -- 估算成本
ALTER TABLE request_logs ADD COLUMN upstream_status INTEGER;  -- 上游原始状态码
ALTER TABLE request_logs ADD COLUMN user_agent TEXT;          -- 客户端标识
```

Usage 页面相应增加：
- 成本趋势图（日/周/月）
- TTFB 分布（而非只有总延迟）
- 重试率指标
- 按 user_agent 分组（区分不同客户端）

### 6.7 健康检查增强

```
当前：GET /models → 200 即 healthy

v3 增强：
  1. 支持自定义健康检查端点（有些 provider 的 /models 需要认证）
  2. 延迟滑动窗口（最近 N 次检查的 p50/p95）
  3. 健康状态变更历史记录
  4. 可配置检查间隔（per-account 或 per-pool）
  5. 主动探测 vs 被动探测：
     - 主动：定时 HTTP 检查（现有）
     - 被动：根据实际请求结果更新状态（新增，配合熔断器）
```

### 6.8 CLI 增强

```bash
lune up                    # 启动（现有）
lune version               # 版本（现有）
lune check                 # 检查（现有）

# v3 新增
lune config validate       # 校验配置文件
lune export > backup.json  # 导出配置（CLI 方式，不需要启动服务）
lune import backup.json    # 导入配置
lune token create "name"   # 快速创建 access token
lune status                # 查看运行状态（连接到运行中的实例）
```

---

## 7. 不做什么

明确不在 v3 范围内的事：

| 排除项 | 原因 |
|--------|------|
| 替换 SQLite | 个人/小团队场景下 SQLite 够用，引入 PG 增加部署复杂度 |
| 语义缓存 | 需要向量数据库，复杂度高，收益不确定 |
| 引入 HTTP 框架（gin/chi） | 标准库 ServeMux 够用，v3 不增加路由复杂度 |
| 引入前端路由库 | 自定义路由工作良好，8 个页面无需 react-router 的复杂度 |
| 分布式部署 | 不是项目目标 |
| 国际化 | 管理后台面向自己，硬编码 UI 文案无问题 |
| 引入 ORM | 直接 SQL 对 SQLite 更透明，表不多不需要 |
| SSR | 管理后台不需要 SEO |

---

## 8. 里程碑建议

### M1: 工程基础（1-2 周）

重点：补测试、结构化日志、接口抽象。

```
 [ ] 核心包接口化（Router, Store, Cache）
 [ ] router_test.go — 路由解析和账号选择的单元测试
 [ ] gateway/usage_test.go — usage 解析测试
 [ ] 迁移到 slog（结构化日志 + 级别控制）
 [ ] 配置校验（启动时报错而非静默忽略）
 [ ] goroutine semaphore（健康检查并发上限）
 [ ] 异步操作错误日志（不再 _ = err）
 [ ] 迁移事务化
 [ ] CI 流水线（GitHub Actions）
```

### M2: 网关可靠性（1-2 周）

重点：重试升级、限流、熔断。

```
 [ ] 指数退避重试 + jitter
 [ ] per-token 速率限制（RPM/TPM）
 [ ] 429 + Retry-After 标准响应
 [ ] 轻量熔断器（per-account）
 [ ] 请求超时精细化（connect/streaming/non-streaming）
 [ ] Fallback pool 支持
 [ ] 请求日志增强（ttfb, retry_count, estimated_cost）
```

### M3: 可观测性（1 周）

重点：metrics 端点、成本追踪。

```
 [ ] /metrics Prometheus 端点
 [ ] model_pricing 表 + 内置价格数据
 [ ] Usage 页面成本趋势图
 [ ] 请求追踪头增强
```

### M4: 前端体验（1-2 周）

重点：数据层重构、状态治理、体验打磨。

```
 [ ] 引入 SWR — 统一数据获取 + 缓存
 [ ] AccountsPage 拆分子组件
 [ ] 自定义 useForm hook + 字段级校验
 [ ] 全局 Error Boundary
 [ ] API 错误拦截（401/429）
 [ ] 空状态设计
 [ ] Playground 增强（参数面板、对话持久化）
```

### M5: 新功能（按需）

```
 [ ] 模型虚拟化（lowest-latency / lowest-cost 策略）
 [ ] Webhook 通知
 [ ] 配置导入
 [ ] 多模型对比 Playground
 [ ] CLI 增强
```

---

> 以上为 v3 思考草稿，非最终方案。每个方向的取舍取决于实际使用中的痛点优先级。
# v3 Draft

## 中文文案策略

v3 建议引入正式的前端文案体系，但不要求一开始就做完整国际化。当前推荐分三层管理：

1. 公共词汇层
   负责导航名、状态词、按钮文案、空状态、确认弹窗基础语气。

2. 页面域文案层
   负责 Accounts、CPA Service、Overview、Usage 等页面自己的标题、说明、表格列名、提示语。

3. 未来 locale 层
   在需要中英切换时，再将公共词汇层和页面域文案层升级为 locale 字典。

## 默认表达原则

- 中文优先展示，优先让中文技术用户一眼看懂用途和动作。
- 保留高辨识度技术术语，不做生硬直译。
- 品牌名、模型名、接口地址、密钥样式、错误原文默认保留原始值。
- 运维后台文案优先强调状态、动作、结果，不写营销式语句。

## 术语保留建议

以下术语在 v3 里默认继续保留英文或中英混写：

- OpenAI
- CPA
- Token
- Provider
- API Key
- Base URL
- Device Code
- Playground

以下词建议以中文为主：

- Overview -> 总览
- Usage -> 用量
- Accounts -> 账号 / 账号单元
- Pools -> 池
- Routes -> 路由
- Status -> 状态
- Refresh -> 刷新
- Retry -> 重试
- Delete -> 删除

## 未来完整 i18n 迁移建议

- 先统一把页面硬编码文案迁移到文案模块，不直接散落在组件里。
- 再为文案模块增加 `zh-CN`、`en-US` 两套 locale 数据。
- 最后引入运行时 locale 选择和持久化，不在 v3 初期直接上复杂国际化框架。
